/**
 * One zone's *combined* audit trail — its own create/update/transition history merged with every
 * DdosPolicy/FirewallRule/IpAccessList record that belongs to it, newest first, deleted ones
 * included. Those 3 entities declare `audit.parent: zoneId`, so the server returns the whole
 * feed one page at a time (`includeChildren`) instead of this tab fetching every child list and
 * then one audit request per child. Built entirely on `@metap/platform-ui`'s generic `AuditLog`;
 * this file only names each entity in the object column.
 */
import { useTranslation } from "react-i18next";
import { AuditLog, type AuditTrailEntryDto } from "@metap/platform-ui";
import { ENTITIES } from "../../api/waf";

/** Maps each entity in this feed to an existing `waf.zoneDetail.*` tab label — reused rather
 *  than duplicated, so the object name always matches the tab the record actually lives under. */
const ENTITY_LABEL_KEY: Record<string, string> = {
  [ENTITIES.zones]: "waf.zoneDetail.tabOverview",
  [ENTITIES.ddosPolicies]: "waf.zoneDetail.tabDdos",
  [ENTITIES.firewallRules]: "waf.zoneDetail.tabRules",
  [ENTITIES.ipAccessLists]: "waf.zoneDetail.tabAccessLists",
};

export function ZoneAuditTab({ zoneId }: { zoneId: string }) {
  const { t } = useTranslation();
  return (
    <AuditLog
      entityName={ENTITIES.zones}
      recordId={zoneId}
      includeChildren
      objectLabel={(entry: AuditTrailEntryDto) => {
        const key = ENTITY_LABEL_KEY[entry.entity];
        return key ? t(key) : entry.entity;
      }}
    />
  );
}
