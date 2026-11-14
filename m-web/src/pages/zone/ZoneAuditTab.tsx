/**
 * One zone's *combined* audit trail — its own create/update/transition history merged with every
 * DdosPolicy/FirewallRule/IpAccessList record that belongs to it, newest first. The zone-centric
 * hub (`ZoneDetailPage.tsx`) shows those 3 child resources as flat lists (`ZoneDdosTab`/
 * `ZoneRulesTab`/`ZoneAccessListTab`), each edited inline — no per-record detail page of their
 * own to hang a single-record `AuditTrail` off of — so this tab is what answers "what changed on
 * this zone or anything under it", the same question a Zone-only audit tab would leave half
 * answered. Built entirely on `@metap/platform-ui`'s generic `useAuditEventsForRecords`/
 * `AuditTrailList` — this file only supplies which (entity, id) pairs belong to one zone and how
 * to label each entity's badge, no new backend/business-audit logic of its own.
 */
import { useTranslation } from "react-i18next";
import { Badge, Spinner } from "@metap/ui";
import {
  ApiErrorMessage,
  AuditTrailList,
  useAuditEventsForRecords,
  type AuditTrailEntryDto,
} from "@metap/platform-ui";
import { ENTITIES, useRecords } from "../../api/waf";

/** Maps each entity this tab aggregates to an existing `waf.zoneDetail.*` tab label — reused
 *  rather than duplicated, so the badge text always matches the tab name the record actually
 *  lives under. */
const ENTITY_BADGE_KEY: Record<string, string> = {
  [ENTITIES.zones]: "waf.zoneDetail.tabOverview",
  [ENTITIES.ddosPolicies]: "waf.zoneDetail.tabDdos",
  [ENTITIES.firewallRules]: "waf.zoneDetail.tabRules",
  [ENTITIES.ipAccessLists]: "waf.zoneDetail.tabAccessLists",
};

export function ZoneAuditTab({ zoneId }: { zoneId: string }) {
  const { t } = useTranslation();
  const ddos = useRecords(ENTITIES.ddosPolicies, { zoneId }, 50);
  const rules = useRecords(ENTITIES.firewallRules, { zoneId }, 100);
  const accessLists = useRecords(ENTITIES.ipAccessLists, { zoneId }, 100);

  const childListsReady =
    !ddos.isLoading && !rules.isLoading && !accessLists.isLoading;
  const childListsError = ddos.error ?? rules.error ?? accessLists.error;

  const targets = childListsReady
    ? [
        { entityName: ENTITIES.zones, recordId: zoneId },
        ...(ddos.data ?? []).map((r) => ({
          entityName: ENTITIES.ddosPolicies,
          recordId: r.id,
        })),
        ...(rules.data ?? []).map((r) => ({
          entityName: ENTITIES.firewallRules,
          recordId: r.id,
        })),
        ...(accessLists.data ?? []).map((r) => ({
          entityName: ENTITIES.ipAccessLists,
          recordId: r.id,
        })),
      ]
    : [];

  const {
    data: events,
    isLoading,
    error,
  } = useAuditEventsForRecords(targets, childListsReady);

  if (childListsError) {
    return <ApiErrorMessage error={childListsError} />;
  }
  if (!childListsReady || isLoading) {
    return <Spinner size="sm" />;
  }
  if (error) {
    return <ApiErrorMessage error={error} />;
  }

  return (
    <AuditTrailList
      events={events ?? []}
      renderBadge={(event: AuditTrailEntryDto) => {
        const key = ENTITY_BADGE_KEY[event.entity];
        return (
          <Badge variant="outline" className="font-normal">
            {key ? t(key) : event.entity}
          </Badge>
        );
      }}
    />
  );
}
