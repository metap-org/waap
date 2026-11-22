/**
 * Export (UC-80): downloads everything configured in this workspace as one JSON file. Built in the
 * browser from the server's answer, so nothing is stored or emailed; the file holds what the person
 * asking is allowed to see.
 */
import { useTranslation } from "react-i18next";
import { Button, SectionCard, toast } from "@metap/ui";
import { useAsyncAction } from "@metap/platform-ui";
import { exportTenantData } from "../api/waf";

export function ExportDataCard() {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction();

  async function download() {
    await run(async () => {
      const { data } = await exportTenantData();
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: "application/json",
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `waap-export-${new Date().toISOString().slice(0, 10)}.json`;
      link.click();
      URL.revokeObjectURL(url);
      const truncated = (data.truncated as string[] | undefined) ?? [];
      toast(
        truncated.length > 0
          ? t("waf.settings.exportTruncated", {
              entities: truncated.join(", "),
            })
          : t("waf.settings.exportDone"),
        { variant: truncated.length > 0 ? "destructive" : "default" },
      );
    });
  }

  return (
    <SectionCard
      title={t("waf.settings.exportTitle")}
      description={t("waf.settings.exportDescription")}
    >
      <Button variant="outline" onClick={download} disabled={busy}>
        {t("waf.settings.exportButton")}
      </Button>
    </SectionCard>
  );
}
