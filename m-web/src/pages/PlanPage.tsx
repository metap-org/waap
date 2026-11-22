/**
 * Plan and usage (UC-70, UC-74): which plan the workspace is on, how much of it is used, and what
 * the other plans allow. A plan is assigned by a platform administrator (there is no payment
 * gateway), so this page explains rather than sells: it warns as a limit nears and says whom to ask.
 */
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertDescription,
  Badge,
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
import { ApiErrorMessage } from "@metap/platform-ui";
import { usePlanUsage, type PlanLimits } from "../api/waf";

/** Share of a limit used at which the page starts warning (UC-74). */
const WARN_AT = 0.8;

export function PlanPage() {
  const { t } = useTranslation();
  const usage = usePlanUsage();
  if (usage.isLoading) {
    return (
      <p className="text-sm text-muted-foreground">{t("waf.common.loading")}</p>
    );
  }
  if (usage.error) return <ApiErrorMessage error={usage.error} />;
  if (!usage.data) return null;
  const { plan, plans, zones, tier } = usage.data;
  const limit = plan.maxZones;
  const ratio = limit === 0 ? 0 : zones / limit;

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title={t("waf.plan.title")}
        description={t("waf.plan.description")}
      />
      <div className="grid gap-4">
        <SectionCard
          title={t("waf.plan.current", {
            name: t(`waf.plan.names.${plan.name}`),
          })}
          description={t("waf.plan.tierNote", { tier })}
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

        <SectionCard
          title={t("waf.plan.compare")}
          description={t("waf.plan.changeHint")}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.plan.colPlan")}</TableHead>
                <TableHead>{t("waf.plan.colZones")}</TableHead>
                <TableHead>{t("waf.plan.colRules")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {plans.map((p) => (
                <TableRow key={p.name}>
                  <TableCell className="font-medium">
                    {t(`waf.plan.names.${p.name}`)}
                    {p.name === plan.name ? (
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
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </SectionCard>
      </div>
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
