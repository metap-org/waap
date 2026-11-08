/**
 * Whitelist/blacklist by IP or CIDR — `waf.ip_access_lists`, deliberately its own entity/screen
 * rather than a filtered view over `FirewallRule` (see
 * `../../../management-plane/services/zones-service/src/entities/ip_access_list_entity.rs`'s doc
 * comment for why, and `../../../edge-plane/waf-edge/src/evaluate.rs`'s module doc comment for
 * how a match here is evaluated ahead of everything else, DDoS policy included).
 *
 * Shared by two contexts, distinguished only by whether `zoneId` is passed — same shape as
 * `FirewallRulesPanel`:
 * - `../pages/zone/ZoneAccessListTab.tsx` — one zone's own entries (`zoneId` set).
 * - `../pages/GlobalAccessListPage.tsx` — tenant-wide entries that apply to every zone (`zoneId`
 *   omitted). Same client-side-filter workaround as `FirewallRulesPanel` for the same
 *   `useGraphQLRecords` empty-string-filter-stripping reason — see that component's file header.
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  Input,
  Label,
  SectionCard,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
  toast,
} from "@metap/ui";
import {
  ENTITIES,
  createRecord,
  deleteRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { ApiErrorMessage, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "./primitives";

type AccessListType = "whitelist" | "blacklist";

type AccessListData = {
  zoneId?: string;
  type?: AccessListType;
  value?: string;
  enabled?: boolean;
};

const EMPTY: AccessListData = { type: "whitelist", value: "", enabled: true };

/** `zoneId` present -> this zone's own entries. Omitted -> every tenant-wide entry, fetched
 *  unfiltered and filtered client-side (see the file header). */
export function IpAccessListPanel({ zoneId }: { zoneId?: string }) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const scoped = useRecords<AccessListData>(
    ENTITIES.ipAccessLists,
    zoneId ? { zoneId } : {},
    zoneId ? 100 : 200,
  );
  const entries = zoneId
    ? scoped
    : { ...scoped, data: scoped.data?.filter((r) => !r.data.zoneId) };
  const [draft, setDraft] = useState<AccessListData>(EMPTY);
  const [open, setOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkType, setBulkType] = useState<AccessListType>("whitelist");
  const [bulkText, setBulkText] = useState("");
  const { busy, run } = useAsyncAction();

  const rows = entries.data ?? [];

  function startCreate() {
    setDraft(EMPTY);
    setOpen(true);
  }

  async function save() {
    await run(async () => {
      const payload = zoneId ? { ...draft, zoneId } : { ...draft };
      await createRecord(ENTITIES.ipAccessLists, payload);
      invalidate();
      setOpen(false);
      toast(t("waf.accessLists.toastSaved"), { variant: "default" });
    });
  }

  async function remove(entry: WafRecord<AccessListData>) {
    await run(async () => {
      await deleteRecord(ENTITIES.ipAccessLists, entry.id, entry.version);
      invalidate();
      toast(t("waf.accessLists.toastDeleted"), { variant: "default" });
    });
  }

  async function saveBulk() {
    const values = bulkText
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
    if (values.length === 0) {
      setBulkOpen(false);
      return;
    }
    await run(async () => {
      const results = await Promise.allSettled(
        values.map((value) =>
          createRecord(ENTITIES.ipAccessLists, {
            ...(zoneId ? { zoneId } : {}),
            type: bulkType,
            value,
            enabled: true,
          }),
        ),
      );
      const failed = results.filter((r) => r.status === "rejected").length;
      invalidate();
      setBulkText("");
      setBulkOpen(false);
      if (failed > 0) {
        toast(
          t("waf.accessLists.toastBulkPartial", {
            succeeded: values.length - failed,
            failed,
          }),
          { variant: "destructive" },
        );
      } else {
        toast(t("waf.accessLists.toastBulkSaved", { count: values.length }), {
          variant: "default",
        });
      }
    });
  }

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.accessLists.title")}
        description={t("waf.accessLists.description")}
        actions={
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => setBulkOpen(true)}
            >
              {t("waf.accessLists.bulkAdd")}
            </Button>
            <Button size="sm" onClick={startCreate}>
              {t("waf.accessLists.addEntry")}
            </Button>
          </div>
        }
      >
        {entries.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : entries.error ? (
          <ApiErrorMessage error={entries.error} />
        ) : rows.length === 0 ? (
          <EmptyState
            title={t("waf.accessLists.noEntriesYet")}
            description={t("waf.accessLists.noEntriesYetDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.accessLists.colType")}</TableHead>
                <TableHead>{t("waf.accessLists.colValue")}</TableHead>
                <TableHead>{t("waf.accessLists.colEnabled")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.accessLists.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell>
                    <StatusBadge value={entry.data.type} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {entry.data.value}
                  </TableCell>
                  <TableCell>
                    {entry.data.enabled
                      ? t("waf.common.yes")
                      : t("waf.common.no")}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => remove(entry)}
                      disabled={busy}
                    >
                      {t("waf.common.delete")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("waf.accessLists.newEntry")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div>
              <Label htmlFor="access-list-type">
                {t("waf.accessLists.type")}
              </Label>
              <Select
                id="access-list-type"
                value={draft.type}
                onValueChange={(value) =>
                  setDraft({ ...draft, type: value as AccessListType })
                }
                options={[
                  {
                    value: "whitelist",
                    label: t("waf.accessLists.typeWhitelist"),
                  },
                  {
                    value: "blacklist",
                    label: t("waf.accessLists.typeBlacklist"),
                  },
                ]}
              />
            </div>
            <div>
              <Label htmlFor="access-list-value">
                {t("waf.accessLists.value")}
              </Label>
              <Input
                id="access-list-value"
                placeholder={t("waf.accessLists.valuePlaceholder")}
                value={draft.value ?? ""}
                onChange={(e) => setDraft({ ...draft, value: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("waf.common.cancel")}
            </Button>
            <Button onClick={save} disabled={busy}>
              {t("waf.common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={bulkOpen} onOpenChange={setBulkOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("waf.accessLists.bulkAdd")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div>
              <Label htmlFor="bulk-type">{t("waf.accessLists.type")}</Label>
              <Select
                id="bulk-type"
                value={bulkType}
                onValueChange={(value) => setBulkType(value as AccessListType)}
                options={[
                  {
                    value: "whitelist",
                    label: t("waf.accessLists.typeWhitelist"),
                  },
                  {
                    value: "blacklist",
                    label: t("waf.accessLists.typeBlacklist"),
                  },
                ]}
              />
            </div>
            <div>
              <Label htmlFor="bulk-values">
                {t("waf.accessLists.bulkValuesLabel")}
              </Label>
              <p className="mb-2 text-xs text-muted-foreground">
                {t("waf.accessLists.bulkValuesHint")}
              </p>
              <Textarea
                id="bulk-values"
                rows={6}
                className="font-mono text-xs"
                value={bulkText}
                onChange={(e) => setBulkText(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBulkOpen(false)}>
              {t("waf.common.cancel")}
            </Button>
            <Button onClick={saveBulk} disabled={busy}>
              {t("waf.common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
