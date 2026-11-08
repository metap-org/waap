/**
 * Module 3 — DDoS L7 policy. A zone can now carry more than one policy at once (Increment 4),
 * scoped by `pathPrefix`/`httpMethod` so a tighter budget can apply to a specific endpoint (e.g.
 * `/login`) than the rest of the site — `priority` breaks ties the same way
 * `FirewallRule.priority` already does. An absent scope (or `httpMethod: "any"`) applies to every
 * request, the same "one policy protects the whole zone" shape a single policy always had before
 * this became a list.
 *
 * Every write calls `sync-config-state` afterwards, which is what keeps `Zone.hasConfig` (and
 * therefore the `activate` guard) honest — deleting the last policy has to be able to take a zone
 * back to "not configured", not just adding the first one to "configured".
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
  Toggle,
  toast,
} from "@metap/ui";
import { ApiErrorMessage, useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  createRecord,
  deleteRecord,
  syncConfigState,
  updateRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../../api/waf";
import { StatusBadge } from "../../components/primitives";

type DdosData = {
  zoneId?: string;
  sensitivity?: string;
  action?: string;
  requestRateThreshold?: number;
  burstWindow?: number;
  enabled?: boolean;
  priority?: number;
  pathPrefix?: string;
  httpMethod?: string;
};

const EMPTY: DdosData = {
  sensitivity: "medium",
  action: "challenge",
  requestRateThreshold: 500,
  burstWindow: 60,
  enabled: true,
  priority: 100,
  httpMethod: "any",
};

const HTTP_METHODS = [
  "any",
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
  "HEAD",
  "OPTIONS",
] as const;

export function ZoneDdosTab({ zoneId }: { zoneId: string }) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const policies = useRecords<DdosData>(ENTITIES.ddosPolicies, { zoneId }, 50);
  const [editing, setEditing] = useState<WafRecord<DdosData> | null>(null);
  const [draft, setDraft] = useState<DdosData>(EMPTY);
  const [open, setOpen] = useState(false);
  const { busy, run } = useAsyncAction();

  const ordered = [...(policies.data ?? [])].sort(
    (a, b) => (a.data.priority ?? 0) - (b.data.priority ?? 0),
  );

  function startCreate() {
    setEditing(null);
    const nextPriority =
      ordered.length > 0
        ? (ordered[ordered.length - 1]?.data.priority ?? 0) + 10
        : 100;
    setDraft({ ...EMPTY, priority: nextPriority });
    setOpen(true);
  }

  function startEdit(policy: WafRecord<DdosData>) {
    setEditing(policy);
    setDraft({ ...EMPTY, ...policy.data });
    setOpen(true);
  }

  async function save() {
    await run(async () => {
      if (editing) {
        await updateRecord(
          ENTITIES.ddosPolicies,
          editing.id,
          editing.version,
          draft,
        );
      } else {
        await createRecord(ENTITIES.ddosPolicies, { ...draft, zoneId });
      }
      await syncConfigState(zoneId);
      invalidate();
      setOpen(false);
      toast(t("waf.zoneTabs.ddos.toastSaved"), { variant: "default" });
    });
  }

  async function remove(policy: WafRecord<DdosData>) {
    await run(async () => {
      await deleteRecord(ENTITIES.ddosPolicies, policy.id, policy.version);
      await syncConfigState(zoneId);
      invalidate();
      toast(t("waf.zoneTabs.ddos.toastRemoved"), { variant: "default" });
    });
  }

  if (policies.isLoading)
    return (
      <p className="mt-4 text-sm text-muted-foreground">
        {t("waf.zoneTabs.ddos.loading")}
      </p>
    );
  if (policies.error)
    return (
      <div className="mt-4">
        <ApiErrorMessage error={policies.error} />
      </div>
    );

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.zoneTabs.ddos.title")}
        description={t("waf.zoneTabs.ddos.description")}
        actions={
          <Button size="sm" onClick={startCreate}>
            {t("waf.zoneTabs.ddos.addPolicy")}
          </Button>
        }
      >
        {ordered.length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.ddos.noPolicy")}
            description={t("waf.zoneTabs.ddos.noPolicyDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-20">
                  {t("waf.zoneTabs.ddos.priority")}
                </TableHead>
                <TableHead>{t("waf.zoneTabs.ddos.sensitivity")}</TableHead>
                <TableHead>{t("waf.zoneTabs.ddos.action")}</TableHead>
                <TableHead>{t("waf.zoneTabs.ddos.scope")}</TableHead>
                <TableHead>{t("waf.zoneTabs.ddos.enabled")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.zoneTabs.ddos.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {ordered.map((policy) => (
                <TableRow key={policy.id}>
                  <TableCell className="tabular-nums">
                    {policy.data.priority}
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={policy.data.sensitivity} />
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={policy.data.action} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {policy.data.pathPrefix ? policy.data.pathPrefix : "*"}{" "}
                    {policy.data.httpMethod && policy.data.httpMethod !== "any"
                      ? policy.data.httpMethod
                      : ""}
                  </TableCell>
                  <TableCell>
                    {policy.data.enabled
                      ? t("waf.common.yes")
                      : t("waf.common.no")}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => startEdit(policy)}
                    >
                      {t("waf.common.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => remove(policy)}
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
            <DialogTitle>
              {editing
                ? t("waf.zoneTabs.ddos.editPolicy")
                : t("waf.zoneTabs.ddos.createPolicy")}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="sensitivity">
                {t("waf.zoneTabs.ddos.sensitivity")}
              </Label>
              <Select
                id="sensitivity"
                value={draft.sensitivity}
                onValueChange={(value) =>
                  setDraft({ ...draft, sensitivity: String(value) })
                }
                options={[
                  {
                    value: "low",
                    label: t("waf.zoneTabs.ddos.sensitivityLow"),
                  },
                  {
                    value: "medium",
                    label: t("waf.zoneTabs.ddos.sensitivityMedium"),
                  },
                  {
                    value: "high",
                    label: t("waf.zoneTabs.ddos.sensitivityHigh"),
                  },
                  {
                    value: "aggressive",
                    label: t("waf.zoneTabs.ddos.sensitivityAggressive"),
                  },
                ]}
              />
            </div>
            <div>
              <Label htmlFor="action">{t("waf.zoneTabs.ddos.action")}</Label>
              <Select
                id="action"
                value={draft.action}
                onValueChange={(value) =>
                  setDraft({ ...draft, action: String(value) })
                }
                options={[
                  { value: "log", label: t("waf.zoneTabs.ddos.actionLog") },
                  {
                    value: "challenge",
                    label: t("waf.zoneTabs.ddos.actionChallenge"),
                  },
                  { value: "block", label: t("waf.zoneTabs.ddos.actionBlock") },
                ]}
              />
            </div>
            <div>
              <Label htmlFor="threshold">
                {t("waf.zoneTabs.ddos.threshold")}
              </Label>
              <Input
                id="threshold"
                type="number"
                value={draft.requestRateThreshold ?? 0}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    requestRateThreshold: Number(e.target.value),
                  })
                }
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.zoneTabs.ddos.thresholdHint")}
              </p>
            </div>
            <div>
              <Label htmlFor="burst">
                {t("waf.zoneTabs.ddos.burstWindow")}
              </Label>
              <Input
                id="burst"
                type="number"
                value={draft.burstWindow ?? 0}
                onChange={(e) =>
                  setDraft({ ...draft, burstWindow: Number(e.target.value) })
                }
              />
            </div>
            <div>
              <Label htmlFor="priority">
                {t("waf.zoneTabs.ddos.priority")}
              </Label>
              <Input
                id="priority"
                type="number"
                value={draft.priority ?? 100}
                onChange={(e) =>
                  setDraft({ ...draft, priority: Number(e.target.value) })
                }
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.zoneTabs.ddos.priorityHint")}
              </p>
            </div>
            <div>
              <Label htmlFor="httpMethod">
                {t("waf.zoneTabs.ddos.httpMethod")}
              </Label>
              <Select
                id="httpMethod"
                value={draft.httpMethod ?? "any"}
                onValueChange={(value) =>
                  setDraft({ ...draft, httpMethod: String(value) })
                }
                options={HTTP_METHODS.map((m) => ({ value: m, label: m }))}
              />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="pathPrefix">
                {t("waf.zoneTabs.ddos.pathPrefix")}
              </Label>
              <Input
                id="pathPrefix"
                placeholder="/login"
                value={draft.pathPrefix ?? ""}
                onChange={(e) =>
                  setDraft({ ...draft, pathPrefix: e.target.value })
                }
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.zoneTabs.ddos.pathPrefixHint")}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Toggle
                id="enabled"
                checked={draft.enabled ?? true}
                onCheckedChange={(checked) =>
                  setDraft({ ...draft, enabled: checked })
                }
              />
              <Label htmlFor="enabled">{t("waf.zoneTabs.ddos.enabled")}</Label>
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
    </div>
  );
}
