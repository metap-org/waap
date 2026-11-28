/**
 * Mitigation from an incident (UC-67): block the attacking source for a limited time, straight from
 * the case, and later keep it or let it go (UC-68).
 *
 * A mitigation is an ordinary blacklist entry on the zone with an expiry and the incident's id. The
 * control plane stops compiling it once it has expired, so the block lapses on its own without
 * anyone remembering to undo it; "keep" clears the expiry (it becomes a normal entry), "remove"
 * deletes it.
 */
import { ConfirmAction } from "./ConfirmAction";
import { useChangeProposal } from "./useChangeProposal";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  EmptyState,
  Input,
  Label,
  SectionCard,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  toast,
} from "@metap/ui";
import {
  ApiErrorMessage,
  formatDateTime,
  useAsyncAction,
} from "@metap/platform-ui";
import {
  ENTITIES,
  createRecord,
  deleteRecord,
  updateRecord,
  useInvalidateWaf,
  useRecords,
} from "../api/waf";

type EntryData = {
  value?: string;
  type?: string;
  expiresAt?: string | null;
  incidentId?: string;
};

const HOURS = ["1", "6", "24", "168"] as const;

/** The incident title names its source ("12 events from 203.0.113.7 in 15m"); the incident has no
 *  field of its own for it. Returns "" when the title carries none. */
export function sourceIpFromTitle(title: string | undefined): string {
  return /\bfrom\s+(\S+)\s+in\b/.exec(title ?? "")?.[1] ?? "";
}

export function IncidentMitigation({
  incidentId,
  zoneId,
  title,
  closed,
}: {
  incidentId: string;
  zoneId: string | undefined;
  title: string | undefined;
  /** A resolved incident can no longer start a mitigation, only tidy up the ones it made. */
  closed: boolean;
}) {
  const { t, i18n } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const proposal = useChangeProposal(zoneId);
  const [ip, setIp] = useState(sourceIpFromTitle(title));
  const [hours, setHours] = useState<string>("24");
  const entries = useRecords<EntryData>(
    ENTITIES.ipAccessLists,
    { incidentId },
    50,
    true,
  );

  async function block() {
    if (!zoneId) return;
    await run(async () => {
      const expiresAt = new Date(
        Date.now() + Number(hours) * 3600 * 1000,
      ).toISOString();
      const entry = {
        zoneId,
        type: "blacklist",
        value: ip.trim(),
        enabled: true,
        expiresAt,
        incidentId,
      };
      const written = await proposal.write(
        () => createRecord(ENTITIES.ipAccessLists, entry),
        [{ op: "create", entity: ENTITIES.ipAccessLists, data: entry }],
      );
      if (!written) return;
      invalidate();
      toast(
        t("waf.incidentDetail.mitigation.toastBlocked", { ip: ip.trim() }),
        {
          variant: "default",
        },
      );
    });
  }

  const now = Date.now();
  return (
    <SectionCard
      title={t("waf.incidentDetail.mitigation.title")}
      description={t("waf.incidentDetail.mitigation.description")}
    >
      {!closed ? (
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <div>
            <Label htmlFor="mitigation-ip">
              {t("waf.incidentDetail.mitigation.source")}
            </Label>
            <Input
              id="mitigation-ip"
              className="w-56 font-mono"
              placeholder="203.0.113.7 or 203.0.113.0/24"
              value={ip}
              onChange={(e) => setIp(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="mitigation-hours">
              {t("waf.incidentDetail.mitigation.duration")}
            </Label>
            <Select
              id="mitigation-hours"
              value={hours}
              onValueChange={(value) => setHours(String(value))}
              options={HOURS.map((h) => ({
                value: h,
                label: t(`waf.incidentDetail.mitigation.hours${h}`),
              }))}
            />
          </div>
          <ConfirmAction
            title={t("waf.incidentDetail.mitigation.block")}
            description={t("waf.confirm.blockBody", { ip: ip.trim() })}
            confirmLabel={t("waf.incidentDetail.mitigation.block")}
            destructive
            onConfirm={block}
          >
            <Button disabled={busy || !ip.trim() || !zoneId}>
              {t("waf.incidentDetail.mitigation.block")}
            </Button>
          </ConfirmAction>
        </div>
      ) : null}

      {entries.error ? (
        <ApiErrorMessage error={entries.error} />
      ) : (entries.data ?? []).length === 0 ? (
        <EmptyState title={t("waf.incidentDetail.mitigation.none")} />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.incidentDetail.mitigation.source")}</TableHead>
              <TableHead>{t("waf.incidentDetail.mitigation.state")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(entries.data ?? []).map((entry) => {
              const expiresAt = entry.data.expiresAt;
              const expired = !!expiresAt && Date.parse(expiresAt) <= now;
              return (
                <TableRow key={entry.id}>
                  <TableCell className="font-mono text-xs">
                    {entry.data.value}
                  </TableCell>
                  <TableCell className="text-sm">
                    {!expiresAt
                      ? t("waf.incidentDetail.mitigation.permanent")
                      : expired
                        ? t("waf.incidentDetail.mitigation.expired", {
                            when: formatDateTime(expiresAt, i18n.language),
                          })
                        : t("waf.incidentDetail.mitigation.until", {
                            when: formatDateTime(expiresAt, i18n.language),
                          })}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-right">
                    {expiresAt ? (
                      <ConfirmAction
                        title={t("waf.incidentDetail.mitigation.keep")}
                        description={t("waf.confirm.changeBody")}
                        confirmLabel={t("waf.incidentDetail.mitigation.keep")}
                        onConfirm={() =>
                          run(async () => {
                            const written = await proposal.write(
                              () =>
                                updateRecord(
                                  ENTITIES.ipAccessLists,
                                  entry.id,
                                  entry.version,
                                  { expiresAt: null },
                                ),
                              [
                                {
                                  op: "update",
                                  entity: ENTITIES.ipAccessLists,
                                  id: entry.id,
                                  data: {
                                    value: entry.data.value,
                                    expiresAt: null,
                                  },
                                },
                              ],
                            );
                            if (written) invalidate();
                          })
                        }
                      >
                        <Button size="sm" variant="ghost" disabled={busy}>
                          {t("waf.incidentDetail.mitigation.keep")}
                        </Button>
                      </ConfirmAction>
                    ) : null}
                    <ConfirmAction
                      title={t("waf.incidentDetail.mitigation.remove")}
                      description={t("waf.confirm.deleteBody")}
                      confirmLabel={t("waf.incidentDetail.mitigation.remove")}
                      destructive
                      onConfirm={() =>
                        run(async () => {
                          const written = await proposal.write(
                            () =>
                              deleteRecord(
                                ENTITIES.ipAccessLists,
                                entry.id,
                                entry.version,
                              ),
                            [
                              {
                                op: "delete",
                                entity: ENTITIES.ipAccessLists,
                                id: entry.id,
                              },
                            ],
                          );
                          if (written) invalidate();
                        })
                      }
                    >
                      <Button size="sm" variant="ghost" disabled={busy}>
                        {t("waf.incidentDetail.mitigation.remove")}
                      </Button>
                    </ConfirmAction>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {proposal.dialog}
    </SectionCard>
  );
}
