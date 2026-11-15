/**
 * Module 2 — the zone list. A thin configuration of the generated list: columns, tones, search,
 * pagination, filters and permission-aware buttons all come from `waf.zones`' metadata and
 * `GeneratedList`; this file only says where a row leads and that zones are created through the
 * onboarding wizard rather than the plain form.
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
    />
  );
}
