/**
 * Module 5's cross-zone view — the remediation queue. This is the Developer persona's screen
 * (`docs/03-personas-workflows.md`): they own findings and nothing else, so the actions here are
 * the finding workflow's, not the zone's.
 *
 * `open` can go three ways (confirm / accept the risk / mark false positive), which is why this
 * offers a set of actions per row rather than the single "next step" the incident queue uses.
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  EmptyState,
  PageHeader,
  SectionCard,
  StatTile,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@metap/ui";
import {
  ENTITIES,
  transitionRecord,
  useAggregate,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "../components/primitives";

type FindingData = {
  scanJobId?: string;
  severity?: string;
  category?: string;
  endpoint?: string;
  description?: string;
  remediationStatus?: string;
  lastSeenAt?: string;
};

/** Which transitions each state offers — mirrors `scan_finding_entity.rs`'s workflow. `action`
 *  doubles as a `waf.actions.<action>` translation key at the render site below. */
const ACTIONS: Record<string, { action: string }[]> = {
  open: [
    { action: "confirm" },
    { action: "accept" },
    { action: "markFalsePositive" },
  ],
  confirmed: [{ action: "markFixed" }],
  fixed: [],
  falsePositive: [],
  accepted: [],
};

const STATUSES = [
  "",
  "open",
  "confirmed",
  "fixed",
  "falsePositive",
  "accepted",
];

export function FindingsPage() {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const [status, setStatus] = useState("open");
  const [severity, setSeverity] = useState("");
  const { busy, run } = useAsyncAction();

  const findings = useRecords<FindingData>(
    ENTITIES.scanFindings,
    { remediationStatus: status || undefined, severity: severity || undefined },
    50,
  );
  const bySeverity = useAggregate(ENTITIES.scanFindings, {
    groupBy: "severity",
  });
  const countFor = (group: string) =>
    bySeverity.data?.find((row) => row.group === group)?.count ?? 0;

  async function act(finding: WafRecord<FindingData>, action: string) {
    await run(async () => {
      await transitionRecord(
        ENTITIES.scanFindings,
        finding.id,
        action,
        finding.version,
      );
      invalidate();
    });
  }

  return (
    <div>
      <PageHeader
        title={t("waf.findings.title")}
        description={t("waf.findings.description")}
      />

      <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label={t("waf.findings.statCritical")}
          value={countFor("critical")}
          tone="danger"
          loading={bySeverity.isLoading}
        />
        <StatTile
          label={t("waf.findings.statHigh")}
          value={countFor("high")}
          tone="danger"
          loading={bySeverity.isLoading}
        />
        <StatTile
          label={t("waf.findings.statMedium")}
          value={countFor("medium")}
          tone="warning"
          loading={bySeverity.isLoading}
        />
        <StatTile
          label={t("waf.findings.statLowInfo")}
          value={countFor("low") + countFor("info")}
          loading={bySeverity.isLoading}
        />
      </div>

      <SectionCard
        title={t("waf.findings.title2")}
        actions={
          <div className="flex flex-wrap gap-1">
            {STATUSES.map((value) => (
              <Button
                key={value || "all"}
                size="sm"
                variant={status === value ? "default" : "outline"}
                onClick={() => setStatus(value)}
              >
                {value ? t(`waf.status.${value}`) : t("waf.common.all")}
              </Button>
            ))}
            <span className="mx-1 w-px bg-border" aria-hidden />
            {["", "critical", "high", "medium", "low"].map((value) => (
              <Button
                key={value || "any"}
                size="sm"
                variant={severity === value ? "default" : "outline"}
                onClick={() => setSeverity(value)}
              >
                {value ? t(`waf.status.${value}`) : t("waf.common.any")}
              </Button>
            ))}
          </div>
        }
      >
        {findings.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : findings.error ? (
          <ApiErrorMessage error={findings.error} />
        ) : (findings.data ?? []).length === 0 ? (
          <EmptyState
            title={t("waf.findings.nothingToRemediate")}
            description={t("waf.findings.nothingToRemediateDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.findings.colSeverity")}</TableHead>
                <TableHead>{t("waf.findings.colCategory")}</TableHead>
                <TableHead>{t("waf.findings.colEndpoint")}</TableHead>
                <TableHead>{t("waf.findings.colStatus")}</TableHead>
                <TableHead>{t("waf.findings.colLastSeen")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.findings.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(findings.data ?? []).map((finding) => {
                const state =
                  finding.data.remediationStatus ?? finding.status ?? "";
                return (
                  <TableRow key={finding.id}>
                    <TableCell>
                      <StatusBadge value={finding.data.severity} />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{finding.data.category}</div>
                      {finding.data.description ? (
                        <div className="max-w-[420px] truncate text-xs text-muted-foreground">
                          {finding.data.description}
                        </div>
                      ) : null}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {finding.data.endpoint}
                    </TableCell>
                    <TableCell>
                      <StatusBadge value={state} />
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {shortDate(finding.data.lastSeenAt)}
                    </TableCell>
                    <TableCell className="text-right">
                      {(ACTIONS[state] ?? []).map((entry) => (
                        <Button
                          key={entry.action}
                          size="sm"
                          variant="outline"
                          className="ml-1"
                          disabled={busy}
                          onClick={() => act(finding, entry.action)}
                        >
                          {t(`waf.actions.${entry.action}`)}
                        </Button>
                      ))}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </SectionCard>
    </div>
  );
}
