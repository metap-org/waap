/**
 * The service provider's side of plans: define them, and give each customer a subscription.
 * Shown on the Plan page of the operator's workspace only (rows written in any other workspace are never
 * read). A customer without a subscription gets the plan
 * marked as default; with none marked, a small built-in one.
 */
import { ConfirmAction } from "./ConfirmAction";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertDescription,
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
} from "@metap/ui";
import { ApiErrorMessage, useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  createRecord,
  decidePlanChange,
  deleteRecord,
  listCustomerTenants,
  updateRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";

type PlanData = {
  name?: string;
  maxZones?: number;
  maxRulesPerZone?: number;
  isDefault?: boolean;
  enabled?: boolean;
};
type SubscriptionData = {
  customerTenantId?: string;
  planId?: string;
  pendingPlanId?: string;
  pendingFrom?: string;
};
type RequestData = {
  customerName?: string;
  requestedPlanId?: string;
  status?: string;
  note?: string;
};

export function PlanManager() {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [editing, setEditing] = useState<WafRecord<PlanData> | "new" | null>(
    null,
  );
  const [customer, setCustomer] = useState("");
  const [planId, setPlanId] = useState("");
  const [error, setError] = useState<Error | null>(null);
  const plans = useRecords<PlanData>(ENTITIES.plans, {}, 50);
  const subs = useRecords<SubscriptionData>(ENTITIES.subscriptions, {}, 100);
  const requests = useRecords<RequestData>(
    ENTITIES.planChanges,
    { status: "pending" },
    50,
  );
  const tenants = useQuery({
    queryKey: ["records", "customer-tenants"],
    queryFn: async () => (await listCustomerTenants()).data.tenants,
  });
  const nameOf = (id?: string) =>
    tenants.data?.find((x) => x.id === id)?.name ?? id ?? "—";
  const planName = (id?: string) =>
    plans.data?.find((p) => p.id === id)?.data.name ?? "—";

  async function guard(action: () => Promise<unknown>) {
    setError(null);
    try {
      await run(async () => {
        await action();
        invalidate();
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  const assigned = new Set(
    (subs.data ?? []).map((s) => s.data.customerTenantId),
  );
  const free = (tenants.data ?? []).filter((x) => !assigned.has(x.id));

  return (
    <>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
      <SectionCard
        title={t("waf.plan.manage.plansTitle")}
        description={t("waf.plan.manage.plansDescription")}
        actions={
          <Button size="sm" onClick={() => setEditing("new")}>
            {t("waf.plan.manage.newPlan")}
          </Button>
        }
      >
        {plans.error ? (
          <ApiErrorMessage error={plans.error} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.plan.colPlan")}</TableHead>
                <TableHead>{t("waf.plan.colZones")}</TableHead>
                <TableHead>{t("waf.plan.colRules")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(plans.data ?? []).map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">
                    {p.data.name}
                    {p.data.isDefault
                      ? ` · ${t("waf.plan.manage.default")}`
                      : ""}
                    {p.data.enabled ? "" : ` · ${t("waf.plan.manage.off")}`}
                  </TableCell>
                  <TableCell>
                    {p.data.maxZones || t("waf.plan.unlimited")}
                  </TableCell>
                  <TableCell>
                    {p.data.maxRulesPerZone || t("waf.plan.unlimited")}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditing(p)}
                    >
                      {t("waf.plan.manage.edit")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>

      {(requests.data ?? []).length > 0 ? (
        <SectionCard
          title={t("waf.plan.manage.requestsTitle")}
          description={t("waf.plan.manage.requestsDescription")}
        >
          <ul className="divide-y text-sm">
            {(requests.data ?? []).map((r) => (
              <li
                key={r.id}
                className="flex items-center justify-between gap-3 py-2"
              >
                <span>
                  <span className="font-medium">
                    {r.data.customerName ?? "—"}
                  </span>
                  {" → "}
                  {planName(r.data.requestedPlanId)}
                  {r.data.note ? (
                    <span className="block text-xs text-muted-foreground">
                      {r.data.note}
                    </span>
                  ) : null}
                </span>
                <span className="flex gap-2">
                  <ConfirmAction
                    title={t("waf.plan.manage.approve")}
                    description={t("waf.confirm.approvePlanBody")}
                    confirmLabel={t("waf.plan.manage.approve")}
                    onConfirm={() =>
                      void guard(() => decidePlanChange(r.id, true, ""))
                    }
                  >
                    <Button size="sm" disabled={busy}>
                      {t("waf.plan.manage.approve")}
                    </Button>
                  </ConfirmAction>
                  <ConfirmAction
                    title={t("waf.plan.manage.reject")}
                    description={t("waf.confirm.changeBody")}
                    confirmLabel={t("waf.plan.manage.reject")}
                    destructive
                    onConfirm={() =>
                      void guard(() => decidePlanChange(r.id, false, ""))
                    }
                  >
                    <Button size="sm" variant="outline" disabled={busy}>
                      {t("waf.plan.manage.reject")}
                    </Button>
                  </ConfirmAction>
                </span>
              </li>
            ))}
          </ul>
        </SectionCard>
      ) : null}

      <SectionCard
        title={t("waf.plan.manage.subsTitle")}
        description={t("waf.plan.manage.subsDescription")}
      >
        <div className="mb-3 flex flex-wrap items-end gap-3">
          <div>
            <Label htmlFor="sub-customer">
              {t("waf.plan.manage.customer")}
            </Label>
            <Select
              id="sub-customer"
              value={customer}
              onValueChange={(v) => setCustomer(String(v))}
              options={free.map((x) => ({ value: x.id, label: x.name }))}
            />
          </div>
          <div>
            <Label htmlFor="sub-plan">{t("waf.plan.colPlan")}</Label>
            <Select
              id="sub-plan"
              value={planId}
              onValueChange={(v) => setPlanId(String(v))}
              options={(plans.data ?? [])
                .filter((p) => p.data.enabled)
                .map((p) => ({ value: p.id, label: p.data.name ?? p.id }))}
            />
          </div>
          <Button
            disabled={busy || !customer || !planId}
            onClick={() =>
              void guard(async () => {
                await createRecord(ENTITIES.subscriptions, {
                  customerTenantId: customer,
                  planId,
                });
                setCustomer("");
              })
            }
          >
            {t("waf.plan.manage.assign")}
          </Button>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.plan.manage.customer")}</TableHead>
              <TableHead>{t("waf.plan.colPlan")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(subs.data ?? []).map((s) => (
              <TableRow key={s.id}>
                <TableCell>{nameOf(s.data.customerTenantId)}</TableCell>
                <TableCell>
                  <Select
                    value={s.data.planId ?? ""}
                    onValueChange={(v) =>
                      void guard(() =>
                        updateRecord(ENTITIES.subscriptions, s.id, s.version, {
                          planId: String(v),
                        }),
                      )
                    }
                    options={(plans.data ?? [])
                      .filter((p) => p.data.enabled || p.id === s.data.planId)
                      .map((p) => ({
                        value: p.id,
                        label: p.data.name ?? p.id,
                      }))}
                  />
                  <span className="sr-only">{planName(s.data.planId)}</span>
                  {s.data.pendingPlanId ? (
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {t("waf.plan.manage.pending", {
                        plan: planName(s.data.pendingPlanId),
                        when: s.data.pendingFrom
                          ? new Date(s.data.pendingFrom).toLocaleDateString()
                          : "—",
                      })}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell className="text-right">
                  <ConfirmAction
                    title={t("waf.plan.manage.remove")}
                    description={t("waf.confirm.deleteBody")}
                    confirmLabel={t("waf.plan.manage.remove")}
                    destructive
                    onConfirm={() =>
                      void guard(() =>
                        deleteRecord(ENTITIES.subscriptions, s.id, s.version),
                      )
                    }
                  >
                    <Button size="sm" variant="ghost" disabled={busy}>
                      {t("waf.plan.manage.remove")}
                    </Button>
                  </ConfirmAction>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </SectionCard>

      {editing ? (
        <PlanDialog
          plan={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </>
  );
}

function PlanDialog({
  plan,
  onClose,
}: {
  plan: WafRecord<PlanData> | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [name, setName] = useState(plan?.data.name ?? "");
  const [zones, setZones] = useState(String(plan?.data.maxZones ?? 1));
  const [rules, setRules] = useState(String(plan?.data.maxRulesPerZone ?? 5));
  const [isDefault, setIsDefault] = useState(plan?.data.isDefault ?? false);
  const [enabled, setEnabled] = useState(plan?.data.enabled ?? true);
  const [error, setError] = useState<Error | null>(null);
  const valid =
    name.trim() !== "" &&
    Number.isInteger(Number(zones)) &&
    Number(zones) >= 0 &&
    Number.isInteger(Number(rules)) &&
    Number(rules) >= 0;

  async function save() {
    setError(null);
    const data = {
      name: name.trim(),
      maxZones: Number(zones),
      maxRulesPerZone: Number(rules),
      isDefault,
      enabled,
    };
    try {
      await run(async () => {
        if (plan)
          await updateRecord(ENTITIES.plans, plan.id, plan.version, data);
        else await createRecord(ENTITIES.plans, data);
        invalidate();
        onClose();
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {plan ? t("waf.plan.manage.edit") : t("waf.plan.manage.newPlan")}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div>
            <Label htmlFor="plan-name">{t("waf.plan.colPlan")}</Label>
            <Input
              id="plan-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="plan-zones">{t("waf.plan.colZones")}</Label>
              <Input
                id="plan-zones"
                inputMode="numeric"
                value={zones}
                onChange={(e) => setZones(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="plan-rules">{t("waf.plan.colRules")}</Label>
              <Input
                id="plan-rules"
                inputMode="numeric"
                value={rules}
                onChange={(e) => setRules(e.target.value)}
              />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            {t("waf.plan.manage.zeroHint")}
          </p>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={isDefault}
              onChange={(e) => setIsDefault(e.target.checked)}
            />
            {t("waf.plan.manage.defaultHint")}
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            {t("waf.plan.manage.enabled")}
          </label>
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("shell.cancel")}
          </Button>
          <Button disabled={busy || !valid} onClick={() => void save()}>
            {t("waf.accounts.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
