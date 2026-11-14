/**
 * Module 5 — vulnerability scanning, zone view. The portal configures scan jobs and shows
 * findings; it does not run the scan (`docs/13-screen-api-map.md`'s 2026-08-30 correction — the
 * DAST engine is a separate execution concern, the same way `edge-plane` is).
 *
 * "Run now" therefore hands the job to whatever scanner backend is configured and reports back
 * honestly when none is: the job queues, and the response says nothing will pick it up yet.
 */
import { useState } from "react";
import { Link } from "react-router-dom";
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
  toast,
} from "@metap/ui";
import {
  ENTITIES,
  createRecord,
  runScanJob,
  useInvalidateWaf,
  useRecords,
} from "../../api/waf";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "../../components/primitives";

type ScanJobData = {
  zoneId?: string;
  scanType?: string;
  schedule?: string;
  status?: string;
  lastRunAt?: string;
};
type FindingData = {
  scanJobId?: string;
  severity?: string;
  category?: string;
  endpoint?: string;
  remediationStatus?: string;
};

export function ZoneScansTab({ zoneId }: { zoneId: string }) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const jobs = useRecords<ScanJobData>(ENTITIES.scanJobs, { zoneId }, 50);
  const [open, setOpen] = useState(false);
  // `run` (the hook's async wrapper) is renamed here — this file already has a domain-level
  // `run(jobId)` function below (triggers a scan run), which would otherwise shadow it.
  const { busy, run: runAsync } = useAsyncAction();
  const [draft, setDraft] = useState<ScanJobData>({
    scanType: "passive",
    schedule: "0 3 * * *",
  });

  // Findings hang off a scan job, not a zone (`docs/02-domain-model.md`: "một finding luôn thuộc
  // đúng một lần chạy scan"), so the zone's findings are the union of its jobs' findings — hence
  // the id list rather than a `zoneId` filter.
  const jobIds = new Set((jobs.data ?? []).map((job) => job.id));
  const findings = useRecords<FindingData>(
    ENTITIES.scanFindings,
    {},
    100,
    jobIds.size > 0,
  );
  const zoneFindings = (findings.data ?? []).filter((finding) =>
    jobIds.has(String(finding.data.scanJobId ?? "")),
  );

  async function createJob() {
    await runAsync(async () => {
      await createRecord(ENTITIES.scanJobs, { ...draft, zoneId });
      invalidate();
      setOpen(false);
      toast(t("waf.zoneTabs.scans.toastCreated"), { variant: "default" });
    });
  }

  async function run(jobId: string) {
    await runAsync(async () => {
      const result = await runScanJob(jobId);
      invalidate();
      toast(result.data.detail, {
        variant: result.data.dispatched ? "default" : "default",
      });
    });
  }

  return (
    <div className="mt-4 grid gap-4">
      <SectionCard
        title={t("waf.zoneTabs.scans.jobsTitle")}
        description={t("waf.zoneTabs.scans.jobsDescription")}
        actions={
          <Button size="sm" onClick={() => setOpen(true)}>
            {t("waf.zoneTabs.scans.newScanJob")}
          </Button>
        }
      >
        {jobs.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : jobs.error ? (
          <ApiErrorMessage error={jobs.error} />
        ) : (jobs.data ?? []).length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.scans.noJobs")}
            description={t("waf.zoneTabs.scans.noJobsDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.zoneTabs.scans.colType")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colSchedule")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colStatus")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colLastRun")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.zoneTabs.scans.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(jobs.data ?? []).map((job) => (
                <TableRow key={job.id}>
                  <TableCell className="font-medium">
                    {job.data.scanType}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {job.data.schedule || t("waf.zoneTabs.scans.manual")}
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={job.data.status ?? job.status} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {shortDate(job.data.lastRunAt)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => run(job.id)}
                      disabled={busy}
                    >
                      {t("waf.zoneTabs.scans.runNow")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>

      <SectionCard
        title={t("waf.zoneTabs.scans.findingsTitle")}
        description={t("waf.zoneTabs.scans.findingsDescription")}
        actions={
          <Button size="sm" variant="outline" asChild>
            <Link to="/findings">{t("waf.zoneTabs.scans.allFindings")}</Link>
          </Button>
        }
      >
        {findings.error ? (
          <ApiErrorMessage error={findings.error} />
        ) : zoneFindings.length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.scans.noFindings")}
            description={t("waf.zoneTabs.scans.noFindingsDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.zoneTabs.scans.colSeverity")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colCategory")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colEndpoint")}</TableHead>
                <TableHead>{t("waf.zoneTabs.scans.colRemediation")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {zoneFindings.map((finding) => (
                <TableRow key={finding.id}>
                  <TableCell>
                    <StatusBadge value={finding.data.severity} />
                  </TableCell>
                  <TableCell>{finding.data.category}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {finding.data.endpoint}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      value={finding.data.remediationStatus ?? finding.status}
                    />
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
            <DialogTitle>{t("waf.zoneTabs.scans.newScanJob")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div>
              <Label htmlFor="scan-type">
                {t("waf.zoneTabs.scans.scanType")}
              </Label>
              <Select
                id="scan-type"
                value={draft.scanType}
                onValueChange={(value) =>
                  setDraft({ ...draft, scanType: String(value) })
                }
                options={[
                  {
                    value: "passive",
                    label: t("waf.zoneTabs.scans.scanTypePassive"),
                  },
                  {
                    value: "active",
                    label: t("waf.zoneTabs.scans.scanTypeActive"),
                  },
                ]}
              />
            </div>
            <div>
              <Label htmlFor="scan-schedule">
                {t("waf.zoneTabs.scans.schedule")}
              </Label>
              <Input
                id="scan-schedule"
                className="font-mono"
                value={draft.schedule ?? ""}
                onChange={(e) =>
                  setDraft({ ...draft, schedule: e.target.value })
                }
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.zoneTabs.scans.scheduleHint")}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("waf.common.cancel")}
            </Button>
            <Button onClick={createJob} disabled={busy}>
              {t("waf.common.create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
