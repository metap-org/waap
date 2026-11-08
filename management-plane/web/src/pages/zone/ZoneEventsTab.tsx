/**
 * Module 6 at zone scope — the raw event stream behind the charts, plus the one action that turns
 * events into work: correlation.
 *
 * Correlation is a real backend call (`/internal/incidents/correlate`), the same one the scheduler
 * drives — a "correlate now" button that ran different logic than the scheduled sweep would prove
 * nothing about what the product actually does.
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  EmptyState,
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
  ENTITIES,
  correlateIncidents,
  useInvalidateWaf,
  useRecords,
} from "../../api/waf";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "../../components/primitives";

type EventData = {
  zoneId?: string;
  triggeredBy?: string;
  triggeredByName?: string;
  action?: string;
  sourceIp?: string;
  requestPath?: string;
  occurredAt?: string;
};

export function ZoneEventsTab({ zoneId }: { zoneId: string }) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const [action, setAction] = useState("");
  const { busy, run } = useAsyncAction();
  const events = useRecords<EventData>(
    ENTITIES.securityEvents,
    { zoneId, action: action || undefined },
    50,
  );

  async function correlate() {
    await run(async () => {
      const result = await correlateIncidents(zoneId);
      invalidate();
      toast(
        t("waf.zoneTabs.events.toastCorrelated", {
          scanned: result.data.scannedEvents,
          created: result.data.createdIncidents.length,
        }),
        { variant: "default" },
      );
    });
  }

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.zoneTabs.events.title")}
        description={t("waf.zoneTabs.events.description")}
        actions={
          <>
            <Select
              value={action}
              onValueChange={(value) => setAction(String(value))}
              options={[
                { value: "", label: t("waf.zoneTabs.events.allActions") },
                { value: "blocked", label: t("waf.status.blocked") },
                { value: "challenged", label: t("waf.status.challenged") },
                { value: "logged", label: t("waf.status.logged") },
              ]}
            />
            <Button
              size="sm"
              variant="outline"
              onClick={correlate}
              disabled={busy}
            >
              {t("waf.zoneTabs.events.correlateNow")}
            </Button>
          </>
        }
      >
        {events.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : events.error ? (
          <ApiErrorMessage error={events.error} />
        ) : (events.data ?? []).length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.events.noEvents")}
            description={t("waf.zoneTabs.events.noEventsDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.zoneTabs.events.colWhen")}</TableHead>
                <TableHead>{t("waf.zoneTabs.events.colAction")}</TableHead>
                <TableHead>{t("waf.zoneTabs.events.colSourceIp")}</TableHead>
                <TableHead>{t("waf.zoneTabs.events.colPath")}</TableHead>
                <TableHead>{t("waf.zoneTabs.events.colTriggeredBy")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(events.data ?? []).map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {shortDate(event.data.occurredAt)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={event.data.action} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {event.data.sourceIp}
                  </TableCell>
                  <TableCell className="max-w-[280px] truncate font-mono text-xs">
                    {event.data.requestPath}
                  </TableCell>
                  <TableCell>
                    {event.data.triggeredByName ??
                      event.data.triggeredBy ??
                      "—"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>
    </div>
  );
}
