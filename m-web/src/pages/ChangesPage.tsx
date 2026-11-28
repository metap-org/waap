/** Every change request the caller may see, tenant-wide ones included. */
import { useTranslation } from "react-i18next";
import { PageHeader } from "@metap/ui";
import { ENTITIES, useRecords, type ZoneData } from "../api/waf";
import { ChangeRequestList } from "../components/ChangeRequestList";

export function ChangesPage() {
  const { t } = useTranslation();
  const zones = useRecords<ZoneData>(ENTITIES.zones, {}, 100);
  return (
    <div>
      <PageHeader
        title={t("waf.changes.pageTitle")}
        description={t("waf.changes.pageDescription")}
      />
      <ChangeRequestList
        zoneName={(id) =>
          zones.data?.find((z) => z.id === id)?.data.hostname ?? id
        }
      />
    </div>
  );
}
