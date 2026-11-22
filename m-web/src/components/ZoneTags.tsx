/**
 * A zone's tags (UC-34): short free labels ("prod", "eu", "client-acme") for finding zones. Edited
 * in place on the zone's overview; the server trims, de-duplicates and bounds them.
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge, Button, Input, SectionCard } from "@metap/ui";
import { useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  updateRecord,
  useInvalidateWaf,
  type Zone,
} from "../api/waf";

export function ZoneTags({ zone }: { zone: Zone }) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const tags = zone.data.tags ?? [];

  async function save(next: string[]) {
    setError(null);
    try {
      await run(async () => {
        await updateRecord(ENTITIES.zones, zone.id, zone.version, {
          tags: next,
        });
        invalidate();
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <SectionCard
      title={t("waf.zoneTabs.overview.tagsTitle")}
      description={t("waf.zoneTabs.overview.tagsDescription")}
    >
      <div className="flex flex-wrap items-center gap-2">
        {tags.map((tag) => (
          <Badge key={tag} variant="secondary" className="gap-1">
            {tag}
            <button
              type="button"
              aria-label={t("waf.zoneTabs.overview.tagRemove", { tag })}
              className="ml-1 text-muted-foreground hover:text-foreground"
              disabled={busy}
              onClick={() => void save(tags.filter((x) => x !== tag))}
            >
              ×
            </button>
          </Badge>
        ))}
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const value = draft.trim();
            if (!value) return;
            setDraft("");
            void save([...tags, value]);
          }}
        >
          <Input
            className="h-8 w-40"
            value={draft}
            placeholder={t("waf.zoneTabs.overview.tagPlaceholder")}
            onChange={(e) => setDraft(e.target.value)}
          />
          <Button size="sm" variant="outline" disabled={busy || !draft.trim()}>
            {t("waf.zoneTabs.overview.tagAdd")}
          </Button>
        </form>
      </div>
      {error ? <p className="mt-2 text-xs text-destructive">{error}</p> : null}
    </SectionCard>
  );
}
