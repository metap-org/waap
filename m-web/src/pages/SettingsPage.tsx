/**
 * Module 10 — tenant settings. Reads and writes `metap`'s tiered config surface
 * (`GET /admin/config`, `PUT/DELETE /admin/config/{key}`), which already carries
 * branding and session policy, so this screen is a thin editor over it rather than a new settings
 * store.
 *
 * What a key *is* stays declared in metap (`pkg/config`'s key registry): tier, default and
 * validator. This screen therefore renders whatever the backend says exists — adding a key there
 * makes it appear here with no frontend change — and only writes keys the backend marks as
 * tenant-writable, since anything else comes back as a rejection by design.
 */
import { ConfirmAction } from "../components/ConfirmAction";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import {
  apiFetch,
  LocaleSwitcher,
  useAsyncAction,
  useAuth,
} from "@metap/platform-ui";
import {
  Button,
  Input,
  PageHeader,
  SectionCard,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  toast,
} from "@metap/ui";
import { StatusBadge } from "../components/primitives";
import { ExportDataCard } from "../components/ExportDataCard";
import { DeleteWorkspaceCard } from "../components/DeleteWorkspaceCard";

type ConfigItem = {
  key: string;
  value: unknown;
  level: string;
  overridden: boolean;
  public: boolean;
  /** A credential: the server never sends its value (`value` is only a marker) and refuses to set
   *  it here (403 `config_key_not_writable`), so it is shown read-only without Save/Reset. */
  secret?: boolean;
};

/** What an editable cell shows for a value: text for a scalar, JSON for anything else (an object
 *  must never reach `String()`, which prints "[object Object]"). */
function displayValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

export function SettingsPage() {
  const { t } = useTranslation();
  const { status } = useAuth();
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const { busy, run } = useAsyncAction();

  const config = useQuery({
    queryKey: ["waf-tenant-config"],
    queryFn: () => apiFetch<{ data: ConfigItem[] }>("/admin/config"),
    select: (response) => response.data,
    enabled: status === "authenticated",
  });

  async function save(item: ConfigItem) {
    const raw = drafts[item.key];
    if (raw === undefined) return;
    await run(async () => {
      // Numbers and booleans must go over the wire as their real JSON types — the key's validator
      // in metap type-checks the value, so sending "30" where a number is expected is a 400.
      let value: unknown = raw;
      if (typeof item.value === "number") value = Number(raw);
      else if (typeof item.value === "boolean") value = raw === "true";
      else if (typeof item.value === "object" && item.value !== null)
        value = JSON.parse(raw);
      await apiFetch(`/admin/config/${encodeURIComponent(item.key)}`, {
        method: "PUT",
        body: JSON.stringify({ value }),
      });
      await config.refetch();
      setDrafts((current) => {
        const next = { ...current };
        delete next[item.key];
        return next;
      });
      toast(t("waf.settings.toastSaved", { key: item.key }), {
        variant: "default",
      });
    });
  }

  async function reset(item: ConfigItem) {
    await run(async () => {
      await apiFetch(`/admin/config/${encodeURIComponent(item.key)}`, {
        method: "DELETE",
      });
      await config.refetch();
      toast(t("waf.settings.toastReset", { key: item.key }), {
        variant: "default",
      });
    });
  }

  const items = config.data ?? [];
  const branding = items.filter((item) => item.key.startsWith("theme."));
  const rest = items.filter((item) => !item.key.startsWith("theme."));

  function renderRows(rows: ConfigItem[]) {
    return rows.map((item) => {
      const current =
        drafts[item.key] ?? (item.secret ? "" : displayValue(item.value));
      return (
        <TableRow key={item.key}>
          <TableCell>
            <div className="font-mono text-xs">{item.key}</div>
            <div className="mt-1 flex gap-1">
              <StatusBadge value={item.level} />
              {item.public ? (
                <span className="text-[10px] text-muted-foreground">
                  {t("waf.settings.public")}
                </span>
              ) : null}
            </div>
          </TableCell>
          <TableCell>
            {item.secret ? (
              <span className="text-sm text-muted-foreground">
                {t("waf.settings.secretManaged")}
              </span>
            ) : (
              <Input
                value={current}
                onChange={(e) =>
                  setDrafts({ ...drafts, [item.key]: e.target.value })
                }
              />
            )}
          </TableCell>
          <TableCell>
            {item.overridden
              ? t("waf.settings.sourceTenant")
              : t("waf.settings.sourceInherited")}
          </TableCell>
          <TableCell className="text-right">
            {item.secret ? null : (
              <>
                <Button
                  size="sm"
                  onClick={() => save(item)}
                  disabled={busy || drafts[item.key] === undefined}
                >
                  {t("waf.common.save")}
                </Button>
                {item.overridden ? (
                  <ConfirmAction
                    title={t("waf.settings.reset")}
                    description={t("waf.confirm.changeBody")}
                    confirmLabel={t("waf.settings.reset")}
                    onConfirm={() => reset(item)}
                  >
                    <Button
                      size="sm"
                      variant="ghost"
                      className="ml-1"

                      disabled={busy}
                    >
                      {t("waf.settings.reset")}
                    </Button>
                  </ConfirmAction>
                ) : null}
              </>
            )}
          </TableCell>
        </TableRow>
      );
    });
  }

  return (
    <div>
      <PageHeader
        title={t("waf.settings.title")}
        description={t("waf.settings.description")}
      />

      <div className="grid gap-4">
        <ExportDataCard />
        <SectionCard
          title={t("waf.settings.languageTitle")}
          description={t("waf.settings.languageDescription")}
        >
          <div className="max-w-xs">
            <LocaleSwitcher />
          </div>
        </SectionCard>

        <SectionCard
          title={t("waf.settings.brandingTitle")}
          description={t("waf.settings.brandingDescription")}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.settings.colKey")}</TableHead>
                <TableHead>{t("waf.settings.colValue")}</TableHead>
                <TableHead>{t("waf.settings.colSource")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.settings.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>{renderRows(branding)}</TableBody>
          </Table>
        </SectionCard>

        <SectionCard
          title={t("waf.settings.platformTitle")}
          description={t("waf.settings.platformDescription")}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.settings.colKey")}</TableHead>
                <TableHead>{t("waf.settings.colValue")}</TableHead>
                <TableHead>{t("waf.settings.colSource")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.settings.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>{renderRows(rest)}</TableBody>
          </Table>
        </SectionCard>
        <DeleteWorkspaceCard />
      </div>
    </div>
  );
}
