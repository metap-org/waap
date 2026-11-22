/**
 * One zone's policy history (docs: UC-48, UC-49): every saved state of its DDoS policies, firewall
 * rules and IP lists, what changed between a version and the one before it, and a rollback.
 *
 * The versions are written by zones-service, not by this screen; a rollback is an action on a
 * version and itself becomes the newest one, so nothing here rewrites history.
 */
import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
  Badge,
  Button,
  EmptyState,
  SectionCard,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  toast,
} from "@metap/ui";
import {
  ApiErrorMessage,
  formatDateTime,
  useAsyncAction,
} from "@metap/platform-ui";
import {
  ENTITIES,
  rollbackToVersion,
  validatePolicy,
  type PolicyReport,
  useInvalidateWaf,
  useRecord,
  useRecords,
  type PolicySnapshot,
  type PolicyVersionData,
} from "../../api/waf";
import { diffSnapshots, type SnapshotDiff } from "./versionDiff";

const ENTITY_LABEL_KEY: Record<string, string> = {
  [ENTITIES.ddosPolicies]: "waf.zoneTabs.versions.entityDdos",
  [ENTITIES.firewallRules]: "waf.zoneTabs.versions.entityRules",
  [ENTITIES.ipAccessLists]: "waf.zoneTabs.versions.entityIpLists",
};

export function ZoneVersionsTab({ zoneId }: { zoneId: string }) {
  const { t, i18n } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [open, setOpen] = useState<string | null>(null);
  const [report, setReport] = useState<PolicyReport | null>(null);
  const versions = useRecords<PolicyVersionData>(
    ENTITIES.policyVersions,
    { zoneId },
    50,
    true,
    { sort: "-number" },
  );
  const rows = versions.data ?? [];

  async function check() {
    await run(async () => {
      setReport((await validatePolicy(zoneId)).data);
    });
  }

  async function rollback(id: string) {
    await run(async () => {
      const result = await rollbackToVersion(id);
      invalidate();
      toast(t("waf.zoneTabs.versions.toastRolledBack", result.data), {
        variant: "default",
      });
    });
  }

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.zoneTabs.versions.title")}
        description={t("waf.zoneTabs.versions.description")}
        actions={
          <Button size="sm" variant="outline" onClick={check} disabled={busy}>
            {t("waf.zoneTabs.versions.check")}
          </Button>
        }
      >
        {report ? (
          <div className="mb-4 rounded-md border p-3 text-sm">
            <p className="font-medium">
              {report.errors > 0
                ? t("waf.zoneTabs.versions.checkErrors", {
                    count: report.errors,
                  })
                : report.findings.some((f) => f.severity === "warning")
                  ? t("waf.zoneTabs.versions.checkWarnings")
                  : t("waf.zoneTabs.versions.checkClean")}
              <span className="ml-2 font-normal text-muted-foreground">
                {t("waf.zoneTabs.versions.checkCounts", {
                  rules: report.rules,
                  entries: report.entries,
                })}
              </span>
            </p>
            <ul className="mt-2 space-y-1">
              {report.findings.map((f, i) => (
                <li key={`${f.code}-${f.id ?? i}`} className="flex gap-2">
                  <Badge
                    variant={
                      f.severity === "error"
                        ? "destructive"
                        : f.severity === "warning"
                          ? "secondary"
                          : "outline"
                    }
                  >
                    {t(`waf.zoneTabs.versions.severity.${f.severity}`)}
                  </Badge>
                  <span>
                    {t(`waf.zoneTabs.versions.finding.${f.code}`, {
                      subject: f.subject ?? "",
                    })}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {versions.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : versions.error ? (
          <ApiErrorMessage error={versions.error} />
        ) : rows.length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.versions.none")}
            description={t("waf.zoneTabs.versions.noneDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.zoneTabs.versions.colVersion")}</TableHead>
                <TableHead>{t("waf.zoneTabs.versions.colSource")}</TableHead>
                <TableHead>{t("waf.zoneTabs.versions.colSummary")}</TableHead>
                <TableHead>{t("waf.zoneTabs.versions.colWhen")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row, index) => {
                const previous = rows[index + 1];
                const expanded = open === row.id;
                return (
                  <Fragment key={row.id}>
                    <TableRow>
                      <TableCell className="font-mono">
                        v{row.data.number}
                        {index === 0 ? (
                          <Badge variant="outline" className="ml-2">
                            {t("waf.status.active")}
                          </Badge>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        {t(
                          `waf.zoneTabs.versions.source${capitalize(row.data.source)}`,
                        )}
                      </TableCell>
                      <TableCell>
                        {row.data.source === "rollback" && row.data.restoredFrom
                          ? t("waf.zoneTabs.versions.restoredFrom", {
                              number: row.data.restoredFrom,
                            })
                          : (row.data.summary ?? "—")}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-muted-foreground">
                        {formatDateTime(row.createdAt, i18n.language)}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setOpen(expanded ? null : row.id)}
                        >
                          {expanded
                            ? t("waf.zoneTabs.versions.hide")
                            : t("waf.zoneTabs.versions.compare")}
                        </Button>
                        {index > 0 ? (
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={busy}
                              >
                                {t("waf.zoneTabs.versions.rollback")}
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>
                                  {t("waf.zoneTabs.versions.rollbackTitle", {
                                    number: row.data.number,
                                  })}
                                </AlertDialogTitle>
                                <AlertDialogDescription>
                                  {t("waf.zoneTabs.versions.rollbackConfirm", {
                                    number: row.data.number,
                                  })}
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>
                                  {t("shell.cancel")}
                                </AlertDialogCancel>
                                <AlertDialogAction
                                  onClick={() => void rollback(row.id)}
                                >
                                  {t("waf.zoneTabs.versions.rollback")}
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
                        ) : null}
                      </TableCell>
                    </TableRow>
                    {expanded ? (
                      <TableRow>
                        <TableCell colSpan={5}>
                          <VersionDiff
                            versionId={row.id}
                            previousId={previous?.id}
                          />
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </Fragment>
                );
              })}
            </TableBody>
          </Table>
        )}
        <p className="mt-3 text-xs text-muted-foreground">
          {t("waf.zoneTabs.versions.tenantWideNote")}
        </p>
      </SectionCard>
    </div>
  );
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/** What changed between a version and the one before it. The snapshots are fetched only now: the
 *  list leaves them out because each holds every rule of the zone. */
function VersionDiff({
  versionId,
  previousId,
}: {
  versionId: string;
  previousId?: string;
}) {
  const { t } = useTranslation();
  const current = useRecord<PolicyVersionData>(
    ENTITIES.policyVersions,
    versionId,
  );
  const previous = useRecord<PolicyVersionData>(
    ENTITIES.policyVersions,
    previousId,
  );
  if (!previousId) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("waf.zoneTabs.versions.diffFirst")}
      </p>
    );
  }
  if (current.error || previous.error) {
    return <ApiErrorMessage error={current.error ?? previous.error} />;
  }
  const after = current.data?.data.snapshot;
  const before = previous.data?.data.snapshot;
  if (!after || !before) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("waf.zoneTabs.versions.diffLoading")}
      </p>
    );
  }
  const diff = diffSnapshots(before as PolicySnapshot, after as PolicySnapshot);
  if (diff.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("waf.zoneTabs.versions.diffNone")}
      </p>
    );
  }
  return (
    <ul className="space-y-1 text-sm">
      {diff.map((entry) => (
        <DiffLine key={`${entry.kind}-${entry.id}`} entry={entry} />
      ))}
    </ul>
  );
}

function DiffLine({ entry }: { entry: SnapshotDiff }) {
  const { t } = useTranslation();
  const tone =
    entry.kind === "added"
      ? "text-emerald-600 dark:text-emerald-400"
      : entry.kind === "removed"
        ? "text-destructive"
        : "text-amber-600 dark:text-amber-500";
  const label =
    entry.kind === "added"
      ? t("waf.zoneTabs.versions.diffAdded")
      : entry.kind === "removed"
        ? t("waf.zoneTabs.versions.diffRemoved")
        : t("waf.zoneTabs.versions.diffChanged");
  return (
    <li>
      <span className={`mr-2 font-medium ${tone}`}>{label}</span>
      <span className="text-muted-foreground">
        {t(ENTITY_LABEL_KEY[entry.entity] ?? entry.entity)}
      </span>{" "}
      <span className="font-mono text-xs">{entry.title}</span>
      {entry.fields.length > 0 ? (
        <ul className="ml-6 mt-1 space-y-0.5 font-mono text-xs text-muted-foreground">
          {entry.fields.map((f) => (
            <li key={f.field}>
              {f.field}: {f.before} → {f.after}
            </li>
          ))}
        </ul>
      ) : null}
    </li>
  );
}
