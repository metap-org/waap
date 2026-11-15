/**
 * Module 2 — the zone list. A thin configuration of the generated list: columns, tones, search,
 * pagination, filters and permission-aware buttons all come from `waf.zones`' metadata and
 * `GeneratedList`; this file only says where a row leads, that zones are created through the
 * onboarding wizard rather than the plain form, and how the 2 cells that carry more than one
 * field's value render (the origin under the hostname, a warning when a zone has no protection).
 */
import { Link } from "react-router-dom";
import { Button } from "@metap/ui";
import { GeneratedList } from "@metap/platform-ui";
import { useTranslation } from "react-i18next";
import { ENTITIES } from "../api/waf";

export function ZonesPage() {
  const { t } = useTranslation();
  return (
    <GeneratedList
      entityName={ENTITIES.zones}
      title={t("waf.zones.title")}
      rowHref={(zone) => `/zones/${zone.id}`}
      newHref={null}
      actions={
        <Button asChild>
          <Link to="/onboarding">{t("waf.common.addZone")}</Link>
        </Button>
      }
      cellRenderers={{
        hostname: (zone) => (
          <span>
            {String(zone.data.hostname ?? "")}
            {zone.data.originAddress ? (
              <span className="block text-xs font-normal text-muted-foreground">
                {String(zone.data.originAddress)}
              </span>
            ) : null}
          </span>
        ),
        hasConfig: (zone) =>
          zone.data.hasConfig ? (
            <span className="text-sm text-muted-foreground">
              {t("waf.common.configured")}
            </span>
          ) : (
            <span className="text-sm text-amber-600 dark:text-amber-500">
              {t("waf.common.none")}
            </span>
          ),
      }}
    />
  );
}
