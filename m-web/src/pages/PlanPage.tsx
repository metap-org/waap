/**
 * Plan and usage (UC-70, UC-74). A workspace sees which plan it is on, how much of it is used, and
 * what the other plans allow; the plan is the service provider's to change, so this page explains
 * rather than sells and warns as a limit nears.
 *
 * The service provider's own workspace (the one set as OPERATOR_TENANT_ID) also manages the plans
 * and gives each customer a subscription; see `PlanManager`.
 */
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Textarea,
  PageHeader,
  Progress,
  SectionCard,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@metap/ui";
import {
  ApiErrorMessage,
  useAsyncAction,
  useCurrentUser,
} from "@metap/platform-ui";
import {
  myPlanRequests,
  requestPlanChange,
  usePlanUsage,
  type PlanLimits,
} from "../api/waf";
import { PlanManager } from "../components/PlanManager";

/** Share of a limit used at which the page starts warning (UC-74). */
const WARN_AT = 0.8;

export function PlanPage() {
  const { t } = useTranslation();
  const usage = usePlanUsage();
  const me = useCurrentUser();
  const queryClient = useQueryClient();
  const { busy, run } = useAsyncAction();
  const [asking, setAsking] = useState<PlanLimits | null>(null);
  const [note, setNote] = useState("");
  const [askError, setAskError] = useState<Error | null>(null);
  const isAdmin = Boolean(me.data?.roles.includes("admin"));
  // Only the workspace's admin may ask about its plan change requests.
  const requests = useQuery({
    queryKey: ["records", "plan-requests"],
    queryFn: async () => (await myPlanRequests()).data.requests,
    enabled: isAdmin && usage.data?.enforced === true && !usage.data.operator,
  });
  const waiting = (requests.data ?? []).some((r) => r.status === "pending");

  async function ask() {
    if (!asking?.id) return;
    setAskError(null);
    try {
      await run(async () => {
        await requestPlanChange(asking.id as string, note.trim());
        await queryClient.invalidateQueries({
          queryKey: ["records", "plan-requests"],
        });
        setAsking(null);
        setNote("");
      });
    } catch (e) {
      setAskError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  if (usage.isLoading) {
    return (
      <p className="text-sm text-muted-foreground">{t("waf.common.loading")}</p>
    );
  }
  if (usage.error) return <ApiErrorMessage error={usage.error} />;
  if (!usage.data) return null;
  const { plan, plans, zones, enforced, operator } = usage.data;
  const limit = plan.maxZones;
  const ratio = limit === 0 ? 0 : zones / limit;

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title={t("waf.plan.title")}
        description={t("waf.plan.description")}
      />
      <div className="grid gap-4">
        {enforced ? (
          <SectionCard
            title={t("waf.plan.current", { name: plan.name })}
            description={t("waf.plan.changeHint")}
          >
            <div className="grid gap-4">
              <div>
                <div className="mb-1 flex justify-between text-sm">
                  <span>{t("waf.plan.zonesUsed")}</span>
                  <span className="tabular-nums">
                    {limit === 0
                      ? t("waf.plan.usedUnlimited", { used: zones })
                      : t("waf.plan.usedOf", { used: zones, limit })}
                  </span>
                </div>
                {limit !== 0 ? <Progress value={zones} max={limit} /> : null}
              </div>
              <p className="text-sm text-muted-foreground">
                {t("waf.plan.rulesLimit", {
                  limit:
                    plan.maxRulesPerZone === 0
                      ? t("waf.plan.unlimited")
                      : plan.maxRulesPerZone,
                })}
              </p>
              {limit !== 0 && ratio >= 1 ? (
                <Alert variant="destructive">
                  <AlertDescription>{t("waf.plan.atLimit")}</AlertDescription>
                </Alert>
              ) : limit !== 0 && ratio >= WARN_AT ? (
                <Alert>
                  <AlertDescription>{t("waf.plan.nearLimit")}</AlertDescription>
                </Alert>
              ) : null}
            </div>
          </SectionCard>
        ) : (
          <SectionCard title={t("waf.plan.noLimits")}>
            <p className="text-sm text-muted-foreground">
              {operator
                ? t("waf.plan.operatorNote", { zones })
                : t("waf.plan.noLimitsNote", { zones })}
            </p>
          </SectionCard>
        )}

        {plans.length > 0 && !operator ? (
          <SectionCard title={t("waf.plan.compare")}>
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
                {plans.map((p) => (
                  <TableRow key={p.id ?? p.name}>
                    <TableCell className="font-medium">
                      {p.name}
                      {p.id === plan.id ? (
                        <Badge variant="outline" className="ml-2">
                          {t("waf.plan.yours")}
                        </Badge>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      {limitText(p, "maxZones", t("waf.plan.unlimited"))}
                    </TableCell>
                    <TableCell>
                      {limitText(p, "maxRulesPerZone", t("waf.plan.unlimited"))}
                    </TableCell>
                    <TableCell className="text-right">
                      {isAdmin && p.id !== plan.id ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={waiting}
                          onClick={() => setAsking(p)}
                        >
                          {t("waf.plan.request.ask")}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {waiting ? (
              <p className="mt-2 text-xs text-muted-foreground">
                {t("waf.plan.request.waiting")}
              </p>
            ) : null}
            {(requests.data ?? []).length > 0 ? (
              <ul className="mt-3 space-y-1 text-sm">
                {(requests.data ?? []).map((r) => (
                  <li key={r.id}>
                    <Badge variant="outline" className="mr-2">
                      {t(`waf.plan.request.status.${r.status}`)}
                    </Badge>
                    {r.planName}
                    {r.reply ? (
                      <span className="text-muted-foreground">
                        {" "}
                        — {r.reply}
                      </span>
                    ) : null}
                  </li>
                ))}
              </ul>
            ) : null}
          </SectionCard>
        ) : null}

        {operator ? <PlanManager /> : null}
      </div>
      {asking ? (
        <Dialog open onOpenChange={(open) => !open && setAsking(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {t("waf.plan.request.title", { name: asking.name })}
              </DialogTitle>
            </DialogHeader>
            <p className="text-sm text-muted-foreground">
              {asking.maxZones !== 0 && zones > asking.maxZones
                ? t("waf.plan.request.overLimit", {
                    used: zones,
                    limit: asking.maxZones,
                  })
                : t("waf.plan.request.explain")}
            </p>
            <Textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={t("waf.plan.request.notePlaceholder")}
            />
            {askError ? (
              <Alert variant="destructive">
                <AlertDescription>{askError.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button variant="outline" onClick={() => setAsking(null)}>
                {t("shell.cancel")}
              </Button>
              <Button disabled={busy} onClick={() => void ask()}>
                {t("waf.plan.request.send")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </div>
  );
}

function limitText(
  plan: PlanLimits,
  key: "maxZones" | "maxRulesPerZone",
  unlimited: string,
) {
  return plan[key] === 0 ? unlimited : plan[key];
}
