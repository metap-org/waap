/**
 * Tenant-wide ("global") firewall rules — the same rule engine as a zone's own rules
 * (`zone/ZoneRulesTab.tsx`), but with no `zoneId`: a global rule is merged into every zone's
 * compiled rule-set at `control-plane`'s resync time (see
 * `../../../control-plane/waf-config-distributor/src/resync.rs`). Standalone page, not a
 * sub-route of `/zones/:zoneId`, since it isn't about one zone.
 */
import { useTranslation } from "react-i18next";
import { PageHeader } from "@metap/ui";
import { FirewallRulesPanel } from "../components/FirewallRulesPanel";

export function GlobalRulesPage() {
  const { t } = useTranslation();
  return (
    <div>
      <PageHeader
        title={t("waf.globalRules.title")}
        description={t("waf.globalRules.description")}
      />
      <FirewallRulesPanel />
    </div>
  );
}
