/**
 * Module 6 in full — the analytics screen the dashboard only samples. Window, zone and metric are
 * all user-controlled here, and every panel is one aggregate request.
 *
 * This is the screen that justified adding aggregation to `metap` core: "top 10 source IPs across
 * every zone in the last 7 days" is a `GROUP BY … ORDER BY count DESC LIMIT 10` over the highest-
 * volume table in the product. Doing it by listing rows was never going to be correct, let alone
 * fast.
 */
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  BarChart,
  Button,
  EmptyState,
  PageHeader,
  SectionCard,
  Select,
  StatTile,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TimeSeries,
} from "@metap/ui";
import { ENTITIES, daysAgo, useAggregate, useRecords } from "../api/waf";
import { ApiErrorMessage, dayLabel, shortDate } from "@metap/platform-ui";

// `DEFAULT_WINDOW` named separately (rather than `WINDOWS[1]`) so the fallback below has a type
// TypeScript can see is never `undefined` — `noUncheckedIndexedAccess` makes any indexed access
// into `WINDOWS` come back as possibly-`undefined`, even for a fixed-length literal array.
// `labelKey` rather than a literal label — this map is module-level (outside any component, so it
// can't call `useTranslation` itself), resolved to text via `t(w.labelKey)` at each render site.
const DEFAULT_WINDOW = {
  value: "7",
  labelKey: "waf.analytics.windowLast7d",
  bucket: "day" as const,
};
const WINDOWS = [
  {
    value: "1",
    labelKey: "waf.analytics.windowLast24h",
    bucket: "hour" as const,
  },
  DEFAULT_WINDOW,
  {
    value: "30",
    labelKey: "waf.analytics.windowLast30d",
    bucket: "day" as const,
  },
];

export function AnalyticsPage() {
  const { t } = useTranslation();
  const [windowDays, setWindowDays] = useState("7");
  const [zoneId, setZoneId] = useState("");

  const selected =
    WINDOWS.find((w) => w.value === windowDays) ?? DEFAULT_WINDOW;
  // Memoized on `windowDays` (2026-09-04, see `DashboardPage.tsx`'s own fix for the full
  // explanation) — `daysAgo(...)` returns a fresh millisecond-precision timestamp on every call,
  // and this value flows into every `useAggregate` below as part of its `queryKey`; computed
  // inline it differed on every render, permanently cache-missing and re-triggering a refetch
  // that itself caused the next render — a self-sustaining request storm.
  const since = useMemo(() => daysAgo(Number(windowDays)), [windowDays]);
  const filters = zoneId ? { zoneId } : {};

  const zones = useRecords<{ hostname?: string }>(ENTITIES.zones, {}, 100);
  const overTime = useAggregate(ENTITIES.securityEvents, {
    bucket: selected.bucket,
    timeField: "occurredAt",
    since,
    filters,
  });
  const byAction = useAggregate(ENTITIES.securityEvents, {
    groupBy: "action",
    timeField: "occurredAt",
    since,
    filters,
  });
  const byTrigger = useAggregate(ENTITIES.securityEvents, {
    groupBy: "triggeredBy",
    timeField: "occurredAt",
    since,
    filters,
  });
  const topSources = useAggregate(ENTITIES.securityEvents, {
    groupBy: "sourceIp",
    timeField: "occurredAt",
    since,
    filters,
    limit: 10,
  });
  const incidentsBySeverity = useAggregate(ENTITIES.incidents, {
    groupBy: "severity",
    filters,
  });

  const totalEvents = (byAction.data ?? []).reduce(
    (sum, row) => sum + (row.count ?? 0),
    0,
  );
  const blocked =
    byAction.data?.find((row) => row.group === "blocked")?.count ?? 0;
  const blockRate =
    totalEvents > 0 ? Math.round((blocked / totalEvents) * 100) : 0;

  // One combined banner rather than gating each of the 6 panels above individually — every panel
  // here reads from the same handful of `securityEvents`/`incidents` aggregates, so a permission
  // or network failure almost always fails all of them together, and a single explanation is more
  // useful than 6 empty-looking charts with no indication why.
  const queryError =
    zones.error ??
    overTime.error ??
    byAction.error ??
    byTrigger.error ??
    topSources.error ??
    incidentsBySeverity.error;

  return (
    <div>
      <PageHeader
        title={t("waf.analytics.title")}
        description={t("waf.analytics.description")}
        actions={
          <>
            <Select
              value={zoneId}
              onValueChange={(value) => setZoneId(String(value))}
              options={[
                { value: "", label: t("waf.analytics.allZones") },
                ...(zones.data ?? []).map((zone) => ({
                  value: zone.id,
                  label: zone.data.hostname ?? zone.id,
                })),
              ]}
            />
            <div className="flex gap-1">
              {WINDOWS.map((w) => (
                <Button
                  key={w.value}
                  size="sm"
                  variant={windowDays === w.value ? "default" : "outline"}
                  onClick={() => setWindowDays(w.value)}
                >
                  {t(w.labelKey)}
                </Button>
              ))}
            </div>
          </>
        }
      />

      {queryError ? (
        <div className="mb-4">
          <ApiErrorMessage error={queryError} />
        </div>
      ) : null}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label={t("waf.analytics.statEvents")}
          value={totalEvents}
          loading={byAction.isLoading}
        />
        <StatTile
          label={t("waf.analytics.statBlocked")}
          value={blocked}
          tone={blocked > 0 ? "danger" : "default"}
        />
        <StatTile
          label={t("waf.analytics.statBlockRate")}
          value={`${blockRate}%`}
        />
        <StatTile
          label={t("waf.analytics.statDistinctSources")}
          value={(topSources.data ?? []).length}
          loading={topSources.isLoading}
        />
      </div>

      <div className="mt-4 grid gap-4">
        <SectionCard
          title={t("waf.analytics.eventsOverTime")}
          description={t(selected.labelKey)}
        >
          <TimeSeries
            height={220}
            ariaLabel={t("waf.analytics.eventsOverTimeAria")}
            points={(overTime.data ?? []).map((row) => ({
              label:
                selected.bucket === "hour"
                  ? shortDate(row.bucket)
                  : dayLabel(row.bucket),
              value: row.count ?? 0,
            }))}
          />
        </SectionCard>

        <div className="grid gap-4 lg:grid-cols-3">
          <SectionCard title={t("waf.analytics.byAction")}>
            <BarChart
              height={180}
              ariaLabel={t("waf.analytics.eventsByActionAria")}
              data={(byAction.data ?? []).map((row) => ({
                label: row.group ?? "—",
                value: row.count ?? 0,
              }))}
            />
          </SectionCard>
          <SectionCard
            title={t("waf.analytics.byTrigger")}
            description={t("waf.analytics.byTriggerDescription")}
          >
            <BarChart
              height={180}
              ariaLabel={t("waf.analytics.eventsByTriggerAria")}
              data={(byTrigger.data ?? []).map((row) => ({
                label: row.group ?? "—",
                value: row.count ?? 0,
              }))}
            />
          </SectionCard>
          <SectionCard title={t("waf.analytics.incidentsBySeverity")}>
            <BarChart
              height={180}
              ariaLabel={t("waf.analytics.incidentsBySeverityAria")}
              data={(incidentsBySeverity.data ?? []).map((row) => ({
                label: row.group ?? "—",
                value: row.count ?? 0,
              }))}
            />
          </SectionCard>
        </div>

        <SectionCard
          title={t("waf.analytics.topSources")}
          description={t("waf.analytics.topSourcesDescription", {
            window: t(selected.labelKey).toLowerCase(),
          })}
        >
          {(topSources.data ?? []).length === 0 ? (
            <EmptyState title={t("waf.analytics.noTraffic")} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("waf.analytics.colSourceIp")}</TableHead>
                  <TableHead className="text-right">
                    {t("waf.analytics.colEvents")}
                  </TableHead>
                  <TableHead className="text-right">
                    {t("waf.analytics.colShare")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(topSources.data ?? []).map((row) => (
                  <TableRow key={row.group ?? "unknown"}>
                    <TableCell className="font-mono text-xs">
                      {row.group ?? "—"}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {row.count}
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {totalEvents > 0
                        ? `${Math.round(((row.count ?? 0) / totalEvents) * 100)}%`
                        : "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>
      </div>
    </div>
  );
}
