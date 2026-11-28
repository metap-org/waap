/**
 * Modules 2/3/4 in one screen — the zone-centric IA `docs/07-portal-features.md` asks for: a zone
 * and everything hanging off it (DDoS policy, firewall rules, scans, its own event stream) behind
 * tabs, instead of the flat per-entity lists the generic CRUD harness gave.
 *
 * Zone lifecycle actions (activate/pause/resume/suspend) come from the workflow metadata itself,
 * not a hardcoded button list — `metap` returns the available transitions per record, so a
 * workflow change in `zone_entity.rs` shows up here without a frontend change.
 */
import { ConfirmAction } from "../components/ConfirmAction";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Alert,
  Button,
  PageHeader,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  toast,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@metap/ui";
import {
  ApiErrorMessage,
  useEntity,
  useEntityLabels,
  WorkflowVisualizeDialog,
  type TransitionAvailability,
  ApiError,
  guardErrorMessage,
  guardReason,
} from "@metap/platform-ui";
import {
  ENTITIES,
  transitionRecord,
  useInvalidateWaf,
  useRecord,
  verifyDomainDns,
  type ZoneData,
} from "../api/waf";
import { StatusBadge } from "../components/primitives";
import { ZoneOverviewTab } from "./zone/ZoneOverviewTab";
import { ZoneDdosTab } from "./zone/ZoneDdosTab";
import { ZoneRulesTab } from "./zone/ZoneRulesTab";
import { ZoneAccessListTab } from "./zone/ZoneAccessListTab";
import { ZoneScansTab } from "./zone/ZoneScansTab";
import { ZoneEventsTab } from "./zone/ZoneEventsTab";
import { ZoneAuditTab } from "./zone/ZoneAuditTab";
import { ZoneVersionsTab } from "./zone/ZoneVersionsTab";
import { ChangeRequestList } from "../components/ChangeRequestList";

/** Which transitions are offered from each status — mirrors `zone_entity.rs`'s workflow. The
 *  backend is still the authority (it re-evaluates the `activate` guard and rejects anything
 *  invalid); this only decides which buttons are worth showing. Each `action` name is also a
 *  `waf.actions.<action>`/`waf.zoneDetail.toast<Action>` translation key — see those usages below. */
const TRANSITIONS: Record<string, { action: string }[]> = {
  pending: [{ action: "activate" }],
  active: [{ action: "pause" }, { action: "suspend" }],
  paused: [{ action: "resume" }, { action: "suspend" }],
  suspended: [{ action: "activate" }],
};

const ZONE_ACTION_TOAST_KEY: Record<string, string> = {
  activate: "waf.zoneDetail.toastActivate",
  pause: "waf.zoneDetail.toastPause",
  resume: "waf.zoneDetail.toastResume",
  suspend: "waf.zoneDetail.toastSuspend",
};

export function ZoneDetailPage() {
  const { t } = useTranslation();
  const { zoneId } = useParams<{ zoneId: string }>();
  const invalidate = useInvalidateWaf();
  // Split from one generic `busy` boolean (2026-09-04) — `pendingAction` now names which
  // transition is in flight (`null` when none), not just whether one is, so the "Visualize
  // workflow" dialog below can highlight the specific button a click came from the same way
  // `WorkflowActionBar`'s generic version already does. `dnsBusy` stays separate: a DNS re-check
  // isn't a workflow transition, so it has no `action` name to report through `pendingAction`.
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [dnsBusy, setDnsBusy] = useState(false);
  const busy = pendingAction !== null || dnsBusy;
  const zone = useRecord<ZoneData>(ENTITIES.zones, zoneId);
  // Both hooks, so both must run before the early returns below regardless of whether `zone.data`
  // has resolved yet — used only for the "Visualize workflow" dialog (`workflow`, `transitionInfo`
  // further down), a passive read of `zone_entity.rs`'s workflow shape.
  const entity = useEntity(ENTITIES.zones);
  const { transitionLabel } = useEntityLabels(ENTITIES.zones);

  if (zone.isLoading)
    return (
      <p className="text-sm text-muted-foreground">{t("waf.common.loading")}</p>
    );
  if (zone.error) return <ApiErrorMessage error={zone.error} />;
  if (!zone.data)
    return (
      <p className="text-sm text-muted-foreground">
        {t("waf.zoneDetail.notFound")}
      </p>
    );

  const record = zone.data;
  const status = record.data.status ?? record.status ?? "pending";
  const workflow = entity.data?.workflow;
  const availableTransitions = workflow
    ? workflow.transitions.filter((t) => t.from === status)
    : [];
  // What the server says about each transition of this record (permission + guard), so a blocked
  // action is disabled with its reason before anyone clicks it; an action the server doesn't list
  // stays enabled and the server still has the last word when it is clicked.
  const serverInfo = new Map(
    (record.capabilities?.transitions ?? []).map((info) => [info.action, info]),
  );
  const transitionInfo = new Map<string, TransitionAvailability>(
    (TRANSITIONS[status] ?? []).map((t) => [
      t.action,
      serverInfo.get(t.action) ?? { action: t.action, available: true },
    ]),
  );
  const blockedReasons = (TRANSITIONS[status] ?? [])
    .map((t) => transitionInfo.get(t.action))
    .filter(
      (info): info is TransitionAvailability =>
        !!info && !info.available && !!info.reason,
    );

  async function act(action: string) {
    if (!zoneId || !zone.data) return;
    setPendingAction(action);
    try {
      await transitionRecord(ENTITIES.zones, zoneId, action, zone.data.version);
      invalidate();
      const toastKey = ZONE_ACTION_TOAST_KEY[action];
      toast(toastKey ? t(toastKey) : action, { variant: "default" });
    } catch (e) {
      // The `activate` guard failing is the common case here, and its message names the missing
      // precondition (unverified hostname / no protection configured) — surfacing it verbatim is
      // more useful than a generic "could not activate".
      toast(
        e instanceof ApiError
          ? guardErrorMessage(t, e)
          : e instanceof Error
            ? e.message
            : String(e),
        { variant: "destructive" },
      );
    } finally {
      setPendingAction(null);
    }
  }

  async function recheckDns() {
    const domainId = record.data.domainId;
    if (!domainId) return;
    setDnsBusy(true);
    try {
      const result = await verifyDomainDns(domainId);
      invalidate();
      toast(
        t(
          result.data.ownershipVerified
            ? "waf.zoneDetail.toastHostnameVerified"
            : "waf.zoneDetail.toastTxtNotFound",
        ),
        {
          variant: "default",
        },
      );
    } finally {
      setDnsBusy(false);
    }
  }

  return (
    <div>
      <div className="mb-2 text-sm">
        <Link className="text-muted-foreground hover:underline" to="/zones">
          {t("waf.zoneDetail.backToZones")}
        </Link>
      </div>
      <PageHeader
        title={record.data.hostname ?? t("waf.zoneDetail.defaultTitle")}
        description={t("waf.zoneDetail.originConfig", {
          origin: record.data.originAddress ?? "—",
          version: record.data.configVersion ?? 0,
        })}
        actions={
          <>
            <StatusBadge value={status} />
            <StatusBadge value={record.data.protectionMode} />
            {record.data.verificationStatus !== "verified" ? (
              <Button
                size="sm"
                variant="outline"
                onClick={recheckDns}
                disabled={busy}
              >
                {t("waf.zoneDetail.recheckDns")}
              </Button>
            ) : null}
            {(TRANSITIONS[status] ?? []).map((transition) => {
              const info = transitionInfo.get(transition.action);
              const blocked = info ? !info.available : false;
              const button = (
                <ConfirmAction
                  title={t(`waf.actions.${transition.action}`)}
                  description={t("waf.confirm.zoneBody")}
                  confirmLabel={t(`waf.actions.${transition.action}`)}
                  destructive={transition.action === "suspend"}
                  onConfirm={() => act(transition.action)}
                >
                  <Button
                    size="sm"

                    disabled={busy || blocked}
                  >
                    {t(`waf.actions.${transition.action}`)}
                  </Button>
                </ConfirmAction>
              );
              return blocked && info?.reason ? (
                <Tooltip key={transition.action}>
                  <TooltipTrigger asChild>
                    <span>{button}</span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-xs">
                    {guardReason(t, info)}
                  </TooltipContent>
                </Tooltip>
              ) : (
                <span key={transition.action}>{button}</span>
              );
            })}
            {workflow ? (
              <WorkflowVisualizeDialog
                label={t("workflow.visualize")}
                workflow={workflow}
                currentState={status}
                availableTransitions={availableTransitions}
                transitionInfo={transitionInfo}
                pendingAction={pendingAction}
                onTransition={(action) => void act(action)}
                transitionLabel={transitionLabel}
              />
            ) : null}
          </>
        }
      />

      {/* Why an action is blocked, said once where it is seen (the button's tooltip repeats it). */}
      {blockedReasons.length > 0 ? (
        <Alert className="mb-4">
          {blockedReasons.map((info) => (
            <div key={info.action} className="text-sm">
              <span className="font-medium">
                {t(`waf.actions.${info.action}`)}:
              </span>{" "}
              {guardReason(t, info)}
            </div>
          ))}
        </Alert>
      ) : null}

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">
            {t("waf.zoneDetail.tabOverview")}
          </TabsTrigger>
          <TabsTrigger value="ddos">{t("waf.zoneDetail.tabDdos")}</TabsTrigger>
          <TabsTrigger value="rules">
            {t("waf.zoneDetail.tabRules")}
          </TabsTrigger>
          <TabsTrigger value="accessLists">
            {t("waf.zoneDetail.tabAccessLists")}
          </TabsTrigger>
          <TabsTrigger value="scans">
            {t("waf.zoneDetail.tabScans")}
          </TabsTrigger>
          <TabsTrigger value="events">
            {t("waf.zoneDetail.tabEvents")}
          </TabsTrigger>
          <TabsTrigger value="changes">
            {t("waf.zoneDetail.tabChanges")}
          </TabsTrigger>
          <TabsTrigger value="versions">
            {t("waf.zoneDetail.tabVersions")}
          </TabsTrigger>
          {entity.data?.audit?.enabled ? (
            <TabsTrigger value="audit">
              {t("waf.zoneDetail.tabAudit")}
            </TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="overview">
          <ZoneOverviewTab zone={record} />
        </TabsContent>
        <TabsContent value="ddos">
          <ZoneDdosTab zoneId={record.id} />
        </TabsContent>
        <TabsContent value="rules">
          <ZoneRulesTab zoneId={record.id} />
        </TabsContent>
        <TabsContent value="accessLists">
          <ZoneAccessListTab zoneId={record.id} />
        </TabsContent>
        <TabsContent value="scans">
          <ZoneScansTab zoneId={record.id} />
        </TabsContent>
        <TabsContent value="events">
          <ZoneEventsTab zoneId={record.id} />
        </TabsContent>
        <TabsContent value="changes">
          <ChangeRequestList zoneId={record.id} />
        </TabsContent>
        <TabsContent value="versions">
          <ZoneVersionsTab zoneId={record.id} />
        </TabsContent>
        {entity.data?.audit?.enabled ? (
          <TabsContent value="audit">
            <ZoneAuditTab zoneId={record.id} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
