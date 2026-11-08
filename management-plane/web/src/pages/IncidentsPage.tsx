/**
 * Module 7 — incident management. The SOC queue: what correlation raised, who owns it, and how far
 * through the workflow it is.
 *
 * The status filter tabs and the per-row actions both read the workflow declared in
 * `incident_entity.rs` (open → acknowledged → mitigating → resolved), so this screen does not
 * encode its own idea of the lifecycle beyond which action is offered from which state.
 */
import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Button,
  EmptyState,
  PageHeader,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  toast,
} from "@metap/ui";
import {
  ENTITIES,
  correlateIncidents,
  transitionRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "../components/primitives";

export type IncidentData = {
  zoneId?: string;
  title?: string;
  severity?: string;
  status?: string;
  eventCount?: number;
  assignedTo?: string;
};

/** Next action per state — one row of the workflow graph, kept next to the UI that offers it.
 *  `action` doubles as a `waf.actions.<action>` translation key at every render site (this page
 *  and `IncidentDetailPage.tsx`, which imports this map). */
export const NEXT_ACTION: Record<string, { action: string }> = {
  open: { action: "acknowledge" },
  acknowledged: { action: "startMitigating" },
  mitigating: { action: "resolve" },
};

const STATUSES = ["", "open", "acknowledged", "mitigating", "resolved"];

export function IncidentsPage() {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const [status, setStatus] = useState("open");
  const { busy, run } = useAsyncAction();
  const incidents = useRecords<IncidentData>(
    ENTITIES.incidents,
    { status: status || undefined },
    50,
  );
  const zones = useRecords<{ hostname?: string }>(ENTITIES.zones, {}, 100);

  const hostnameFor = (zoneId?: string) =>
    zones.data?.find((zone) => zone.id === zoneId)?.data.hostname ??
    zoneId ??
    "—";

  async function advance(incident: WafRecord<IncidentData>) {
    const next = NEXT_ACTION[incident.data.status ?? incident.status ?? ""];
    if (!next) return;
    await run(async () => {
      await transitionRecord(
        ENTITIES.incidents,
        incident.id,
        next.action,
        incident.version,
      );
      invalidate();
    });
  }

  async function correlate() {
    await run(async () => {
      const result = await correlateIncidents();
      invalidate();
      toast(
        t("waf.incidents.toastCorrelated", {
          count: result.data.createdIncidents.length,
          events: result.data.scannedEvents,
        }),
        {
          variant: "default",
        },
      );
    });
  }

  return (
    <div>
      <PageHeader
        title={t("waf.incidents.title")}
        description={t("waf.incidents.description")}
        actions={
          <Button variant="outline" onClick={correlate} disabled={busy}>
            {t("waf.incidents.runCorrelation")}
          </Button>
        }
      />

      <div className="mb-3 flex gap-1">
        {STATUSES.map((value) => (
          <Button
            key={value || "all"}
            size="sm"
            variant={status === value ? "default" : "outline"}
            onClick={() => setStatus(value)}
          >
            {value ? t(`waf.status.${value}`) : t("waf.common.all")}
          </Button>
        ))}
      </div>

      {incidents.isLoading ? (
        <p className="text-sm text-muted-foreground">
          {t("waf.common.loading")}
        </p>
      ) : incidents.error ? (
        <ApiErrorMessage error={incidents.error} />
      ) : (incidents.data ?? []).length === 0 ? (
        <EmptyState
          title={t("waf.incidents.nothingHere")}
          description={t("waf.incidents.nothingHereDescription")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.incidents.colIncident")}</TableHead>
              <TableHead>{t("waf.incidents.colZone")}</TableHead>
              <TableHead>{t("waf.incidents.colSeverity")}</TableHead>
              <TableHead>{t("waf.incidents.colStatus")}</TableHead>
              <TableHead className="text-right">
                {t("waf.incidents.colEvents")}
              </TableHead>
              <TableHead>{t("waf.incidents.colRaised")}</TableHead>
              <TableHead className="text-right">
                {t("waf.incidents.colAction")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(incidents.data ?? []).map((incident) => {
              const state = incident.data.status ?? incident.status ?? "";
              const next = NEXT_ACTION[state];
              return (
                <TableRow key={incident.id}>
                  <TableCell>
                    <Link
                      className="font-medium hover:underline"
                      to={`/incidents/${incident.id}`}
                    >
                      {incident.data.title}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Link
                      className="hover:underline"
                      to={`/zones/${incident.data.zoneId}`}
                    >
                      {hostnameFor(incident.data.zoneId)}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={incident.data.severity} />
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={state} />
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {incident.data.eventCount ?? 0}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {shortDate(incident.createdAt)}
                  </TableCell>
                  <TableCell className="text-right">
                    {next ? (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => advance(incident)}
                        disabled={busy}
                      >
                        {t(`waf.actions.${next.action}`)}
                      </Button>
                    ) : (
                      <span className="text-xs text-muted-foreground">
                        {t("waf.incidents.done")}
                      </span>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
