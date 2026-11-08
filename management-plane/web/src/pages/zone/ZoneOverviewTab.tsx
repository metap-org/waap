/** Zone overview — traffic shape for this zone plus its current posture, all from the aggregate
 *  API so the counts are real totals rather than "however many rows fit on one page". */
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { BarChart, SectionCard, StatTile, TimeSeries } from "@metap/ui";
import {
  ENTITIES,
  daysAgo,
  useAggregate,
  useRecords,
  type Zone,
} from "../../api/waf";
import { ApiErrorMessage, dayLabel, shortDate } from "@metap/platform-ui";
import { StatusBadge } from "../../components/primitives";

export function ZoneOverviewTab({ zone }: { zone: Zone }) {
  const { t } = useTranslation();
  const filters = { zoneId: zone.id };
  // Memoized (2026-09-04, see `DashboardPage.tsx`'s own fix for the full explanation) —
  // `daysAgo(...)` returns a fresh timestamp on every call, and it flows into `useAggregate`'s
  // `queryKey` below; computed inline it differed on every render, causing a permanent
  // cache-miss/refetch/re-render loop.
  const since7d = useMemo(() => daysAgo(7), []);
  const eventsPerDay = useAggregate(ENTITIES.securityEvents, {
    bucket: "day",
    timeField: "occurredAt",
    since: since7d,
    filters,
  });
  const byAction = useAggregate(ENTITIES.securityEvents, {
    groupBy: "action",
    timeField: "occurredAt",
    since: since7d,
    filters,
  });
  const incidents = useAggregate(ENTITIES.incidents, {
    groupBy: "status",
    filters,
  });
  const rules = useRecords(ENTITIES.firewallRules, filters, 100);
  const policies = useRecords(ENTITIES.ddosPolicies, filters, 5);

  const totalEvents = (byAction.data ?? []).reduce(
    (sum, row) => sum + (row.count ?? 0),
    0,
  );
  const blocked =
    byAction.data?.find((row) => row.group === "blocked")?.count ?? 0;
  const openIncidents =
    incidents.data?.find((row) => row.group === "open")?.count ?? 0;

  // One combined banner rather than gating each panel individually — see AnalyticsPage.tsx's own
  // version of this for the reasoning.
  const queryError =
    eventsPerDay.error ??
    byAction.error ??
    incidents.error ??
    rules.error ??
    policies.error;

  return (
    <div className="mt-4 grid gap-4">
      {queryError ? <ApiErrorMessage error={queryError} /> : null}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label={t("waf.zoneTabs.overview.statEvents7d")}
          value={totalEvents}
          loading={byAction.isLoading}
        />
        <StatTile
          label={t("waf.zoneTabs.overview.statBlocked7d")}
          value={blocked}
          tone={blocked > 0 ? "danger" : "default"}
        />
        <StatTile
          label={t("waf.zoneTabs.overview.statOpenIncidents")}
          value={openIncidents}
          tone={openIncidents > 0 ? "warning" : "success"}
        />
        <StatTile
          label={t("waf.zoneTabs.overview.statFirewallRules")}
          value={(rules.data ?? []).length}
          loading={rules.isLoading}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <SectionCard
            title={t("waf.zoneTabs.overview.events")}
            description={t("waf.zoneTabs.overview.last7Days")}
          >
            <TimeSeries
              ariaLabel={t("waf.zoneTabs.overview.eventsPerDayAria")}
              points={(eventsPerDay.data ?? []).map((row) => ({
                label: dayLabel(row.bucket),
                value: row.count ?? 0,
              }))}
            />
          </SectionCard>
        </div>
        <SectionCard title={t("waf.zoneTabs.overview.byAction")}>
          <BarChart
            height={180}
            ariaLabel={t("waf.zoneTabs.overview.eventsByActionAria")}
            data={(byAction.data ?? []).map((row) => ({
              label: row.group ?? "—",
              value: row.count ?? 0,
            }))}
          />
        </SectionCard>
      </div>

      <SectionCard
        title={t("waf.zoneTabs.overview.posture")}
        description={t("waf.zoneTabs.overview.postureDescription")}
      >
        <dl className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <dt className="text-xs uppercase text-muted-foreground">
              {t("waf.zoneTabs.overview.domainOwnership")}
            </dt>
            <dd className="mt-1">
              <StatusBadge value={zone.data.verificationStatus} />
            </dd>
          </div>
          <div>
            <dt className="text-xs uppercase text-muted-foreground">
              {t("waf.zoneTabs.overview.dnsRouting")}
            </dt>
            <dd className="mt-1">
              <StatusBadge value={zone.data.dnsRoutingStatus} />
            </dd>
          </div>
          <div>
            <dt className="text-xs uppercase text-muted-foreground">
              {t("waf.zoneTabs.overview.ddosPolicy")}
            </dt>
            <dd className="mt-1">
              {(policies.data ?? []).length > 0 ? (
                <StatusBadge
                  value={String(policies.data?.[0]?.data.sensitivity ?? "")}
                />
              ) : (
                <span className="text-muted-foreground">
                  {t("waf.zoneTabs.overview.none")}
                </span>
              )}
            </dd>
          </div>
          <div>
            <dt className="text-xs uppercase text-muted-foreground">
              {t("waf.zoneTabs.overview.lastDnsCheck")}
            </dt>
            <dd className="mt-1 text-muted-foreground">
              {shortDate(zone.data.lastDnsCheckAt)}
            </dd>
          </div>
        </dl>
      </SectionCard>
    </div>
  );
}
