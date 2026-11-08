/**
 * Tenant-wide ("global") whitelist/blacklist entries — same entity as a zone's own
 * (`zone/ZoneAccessListTab.tsx`), but with no `zoneId`: an entry here applies to every zone,
 * evaluated ahead of everything else including DDoS policy (see
 * `../../../edge-plane/waf-edge/src/evaluate.rs`'s module doc comment). Standalone page, not a
 * sub-route of `/zones/:zoneId`, matching `GlobalRulesPage`'s own reasoning.
 */
import { useTranslation } from "react-i18next";
import { PageHeader } from "@metap/ui";
import { IpAccessListPanel } from "../components/IpAccessListPanel";

export function GlobalAccessListPage() {
  const { t } = useTranslation();
  return (
    <div>
      <PageHeader
        title={t("waf.globalAccessLists.title")}
        description={t("waf.globalAccessLists.description")}
      />
      <IpAccessListPanel />
    </div>
  );
}
