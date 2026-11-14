/**
 * One incident, with the events that produced it.
 *
 * The event list is filtered by the incident's zone rather than by a stored link: an `Incident`
 * has no `eventIds` field — `docs/02-domain-model.md` deliberately keeps `eventCount` as a
 * snapshot taken at creation, not a live relation — so this shows the zone's recent stream as
 * context rather than claiming to reconstruct the exact rows that were correlated.
 */
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Button,
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
import { useState } from "react";
import {
  ApiErrorMessage,
  AuditTrail,
  shortDate,
  useEntity,
  useEntityLabels,
  WorkflowVisualizeDialog,
  type TransitionAvailability,
} from "@metap/platform-ui";
import {
  ENTITIES,
  transitionRecord,
  useInvalidateWaf,
  useRecord,
  useRecords,
} from "../api/waf";
import { StatusBadge } from "../components/primitives";
import { NEXT_ACTION, type IncidentData } from "./IncidentsPage";

const INCIDENT_ACTION_TOAST_KEY: Record<string, string> = {
  acknowledge: "waf.incidentDetail.toastAcknowledge",
  startMitigating: "waf.incidentDetail.toastStartMitigating",
  resolve: "waf.incidentDetail.toastResolve",
};

export function IncidentDetailPage() {
  const { t } = useTranslation();
  const { incidentId } = useParams<{ incidentId: string }>();
  const invalidate = useInvalidateWaf();
  // `pendingAction`, not a plain boolean (2026-09-04, see `ZoneDetailPage.tsx`'s own fix for the
  // full explanation) — names the transition in flight so the "Visualize workflow" dialog below
  // can highlight the right button.
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const incident = useRecord<IncidentData>(ENTITIES.incidents, incidentId);
  const zoneId = incident.data?.data.zoneId;
  const zone = useRecord<{ hostname?: string }>(ENTITIES.zones, zoneId);
  const events = useRecords<{
    sourceIp?: string;
    action?: string;
    requestPath?: string;
    occurredAt?: string;
  }>(ENTITIES.securityEvents, { zoneId }, 25, Boolean(zoneId));
  // Both hooks, so both must run before the early returns below — see `ZoneDetailPage.tsx`'s own
  // identical comment.
  const entity = useEntity(ENTITIES.incidents);
  const { transitionLabel } = useEntityLabels(ENTITIES.incidents);

  if (incident.isLoading)
    return (
      <p className="text-sm text-muted-foreground">{t("waf.common.loading")}</p>
    );
  if (incident.error) return <ApiErrorMessage error={incident.error} />;
  if (!incident.data)
    return (
      <p className="text-sm text-muted-foreground">
        {t("waf.incidentDetail.notFound")}
      </p>
    );

  const record = incident.data;
  const state = record.data.status ?? record.status ?? "";
  const next = NEXT_ACTION[state];
  const workflow = entity.data?.workflow;
  const availableTransitions = workflow
    ? workflow.transitions.filter((t) => t.from === state)
    : [];
  // Same "no real per-transition capability data reaches this page" caveat as
  // `ZoneDetailPage.tsx` — `next` is the only transition this page ever offers, marked available.
  const transitionInfo = new Map<string, TransitionAvailability>(
    next ? [[next.action, { action: next.action, available: true }]] : [],
  );

  async function advance() {
    if (!incidentId || !next || !incident.data) return;
    setPendingAction(next.action);
    try {
      await transitionRecord(
        ENTITIES.incidents,
        incidentId,
        next.action,
        incident.data.version,
      );
      invalidate();
      const toastKey = INCIDENT_ACTION_TOAST_KEY[next.action];
      toast(toastKey ? t(toastKey) : next.action, { variant: "default" });
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), {
        variant: "destructive",
      });
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <div>
      <div className="mb-2 text-sm">
        <Link className="text-muted-foreground hover:underline" to="/incidents">
          {t("waf.incidentDetail.backToIncidents")}
        </Link>
      </div>
      <PageHeader
        title={record.data.title ?? t("waf.incidentDetail.defaultTitle")}
        description={t("waf.incidentDetail.raisedDescription", {
          date: shortDate(record.createdAt),
          count: record.data.eventCount ?? 0,
        })}
        actions={
          <>
            <StatusBadge value={record.data.severity} />
            <StatusBadge value={state} />
            {next ? (
              <Button
                size="sm"
                onClick={advance}
                disabled={pendingAction !== null}
              >
                {t(`waf.actions.${next.action}`)}
              </Button>
            ) : null}
            {workflow ? (
              <WorkflowVisualizeDialog
                label={t("workflow.visualize")}
                workflow={workflow}
                currentState={state}
                availableTransitions={availableTransitions}
                transitionInfo={transitionInfo}
                pendingAction={pendingAction}
                onTransition={() => void advance()}
                transitionLabel={transitionLabel}
              />
            ) : null}
          </>
        }
      />

      <div className="grid gap-4">
        <SectionCard title={t("waf.incidentDetail.details")}>
          <dl className="grid gap-3 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-xs uppercase text-muted-foreground">
                {t("waf.incidentDetail.zone")}
              </dt>
              <dd className="mt-1">
                <Link className="hover:underline" to={`/zones/${zoneId}`}>
                  {zone.data?.data.hostname ?? zoneId ?? "—"}
                </Link>
              </dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-muted-foreground">
                {t("waf.incidentDetail.assignedTo")}
              </dt>
              <dd className="mt-1">
                {record.data.assignedTo || (
                  <span className="text-muted-foreground">
                    {t("waf.incidentDetail.unassigned")}
                  </span>
                )}
              </dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-muted-foreground">
                {t("waf.incidentDetail.lastUpdate")}
              </dt>
              <dd className="mt-1 text-muted-foreground">
                {shortDate(record.updatedAt)}
              </dd>
            </div>
          </dl>
        </SectionCard>

        <SectionCard
          title={t("waf.incidentDetail.recentEvents")}
          description={t("waf.incidentDetail.recentEventsDescription")}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.incidentDetail.colWhen")}</TableHead>
                <TableHead>{t("waf.incidentDetail.colAction")}</TableHead>
                <TableHead>{t("waf.incidentDetail.colSourceIp")}</TableHead>
                <TableHead>{t("waf.incidentDetail.colPath")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(events.data ?? []).map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="whitespace-nowrap text-muted-foreground">
                    {shortDate(event.data.occurredAt)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={event.data.action} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {event.data.sourceIp}
                  </TableCell>
                  <TableCell className="max-w-[320px] truncate font-mono text-xs">
                    {event.data.requestPath}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </SectionCard>

        {entity.data?.audit?.enabled ? (
          <SectionCard title={t("waf.incidentDetail.auditTrail")}>
            <AuditTrail entityName={ENTITIES.incidents} recordId={record.id} />
          </SectionCard>
        ) : null}
      </div>
    </div>
  );
}
