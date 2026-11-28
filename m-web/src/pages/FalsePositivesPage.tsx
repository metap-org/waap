/**
 * Module 7 — false positive reports (UC-61, UC-62): "this request was blocked and should not have
 * been". The SOC opens a report, looks at what was blocked, and either dismisses it with a reply or
 * lets the client through (a whitelist entry on the zone) and resolves it.
 *
 * A report is entered here, from a request id someone forwarded. Intake from the block page itself
 * needs the edge to show a request id and a report form, which it does not yet.
 */
import { ConfirmAction } from "../components/ConfirmAction";
import { useChangeProposal } from "../components/useChangeProposal";
import { useState } from "react";
import { Link } from "react-router-dom";
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
  EmptyState,
  Input,
  Label,
  PageHeader,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
  toast,
} from "@metap/ui";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  createRecord,
  transitionRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { StatusBadge } from "../components/primitives";
import { isSharedAddress } from "./sharedAddress";

type ReportData = {
  zoneId?: string;
  requestId?: string;
  sourceIp?: string;
  requestPath?: string;
  description?: string;
  reporterContact?: string;
  status?: string;
  resolutionNote?: string;
  exceptionEntryId?: string;
};

const STATUSES = ["open", "dismissed", "resolved", ""];

export function FalsePositivesPage() {
  const { t } = useTranslation();
  const [status, setStatus] = useState("open");
  const [creating, setCreating] = useState(false);
  const [handling, setHandling] = useState<WafRecord<ReportData> | null>(null);
  const reports = useRecords<ReportData>(
    ENTITIES.falsePositives,
    { status: status || undefined },
    50,
  );
  const zones = useRecords<{ hostname?: string }>(ENTITIES.zones, {}, 100);
  const hostnameFor = (zoneId?: string) =>
    zones.data?.find((zone) => zone.id === zoneId)?.data.hostname ??
    zoneId ??
    "—";

  return (
    <div>
      <PageHeader
        title={t("waf.falsePositives.title")}
        description={t("waf.falsePositives.description")}
        actions={
          <Button onClick={() => setCreating(true)}>
            {t("waf.falsePositives.newReport")}
          </Button>
        }
      />
      <div className="mb-3 flex gap-1">
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
      </div>

      {reports.isLoading ? (
        <p className="text-sm text-muted-foreground">
          {t("waf.common.loading")}
        </p>
      ) : reports.error ? (
        <ApiErrorMessage error={reports.error} />
      ) : (reports.data ?? []).length === 0 ? (
        <EmptyState
          title={t("waf.falsePositives.none")}
          description={t("waf.falsePositives.noneDescription")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.falsePositives.colReport")}</TableHead>
              <TableHead>{t("waf.falsePositives.colZone")}</TableHead>
              <TableHead>{t("waf.falsePositives.colClient")}</TableHead>
              <TableHead>{t("waf.falsePositives.colStatus")}</TableHead>
              <TableHead>{t("waf.falsePositives.colReceived")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(reports.data ?? []).map((report) => (
              <TableRow key={report.id}>
                <TableCell className="max-w-[320px] truncate">
                  {report.data.description}
                </TableCell>
                <TableCell>
                  <Link
                    className="hover:underline"
                    to={`/zones/${report.data.zoneId}`}
                  >
                    {hostnameFor(report.data.zoneId)}
                  </Link>
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {report.data.sourceIp ?? "—"}
                </TableCell>
                <TableCell>
                  <StatusBadge value={report.data.status ?? report.status} />
                </TableCell>
                <TableCell className="whitespace-nowrap text-muted-foreground">
                  {shortDate(report.createdAt)}
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setHandling(report)}
                  >
                    {report.data.status === "open"
                      ? t("waf.falsePositives.handle")
                      : t("waf.falsePositives.view")}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {creating ? (
        <NewReportDialog
          zones={zones.data ?? []}
          onClose={() => setCreating(false)}
        />
      ) : null}
      {handling ? (
        <HandleDialog
          report={handling}
          hostname={hostnameFor(handling.data.zoneId)}
          onClose={() => setHandling(null)}
        />
      ) : null}
    </div>
  );
}

function NewReportDialog({
  zones,
  onClose,
}: {
  zones: WafRecord<{ hostname?: string }>[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [zoneId, setZoneId] = useState(zones[0]?.id ?? "");
  const [form, setForm] = useState({
    requestId: "",
    sourceIp: "",
    requestPath: "",
    description: "",
    reporterContact: "",
  });
  const set = (key: keyof typeof form) => (value: string) =>
    setForm((f) => ({ ...f, [key]: value }));

  async function submit() {
    await run(async () => {
      await createRecord(ENTITIES.falsePositives, {
        zoneId,
        ...Object.fromEntries(
          Object.entries(form).filter(([, v]) => v.trim() !== ""),
        ),
      });
      invalidate();
      onClose();
    });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("waf.falsePositives.newReport")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div>
            <Label htmlFor="fp-zone">{t("waf.falsePositives.colZone")}</Label>
            <Select
              id="fp-zone"
              value={zoneId}
              onValueChange={(value) => setZoneId(String(value))}
              options={zones.map((z) => ({
                value: z.id,
                label: z.data.hostname ?? z.id,
              }))}
            />
          </div>
          <div>
            <Label htmlFor="fp-desc">
              {t("waf.falsePositives.whatHappened")}
            </Label>
            <Textarea
              id="fp-desc"
              value={form.description}
              onChange={(e) => set("description")(e.target.value)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="fp-ip">{t("waf.falsePositives.colClient")}</Label>
              <Input
                id="fp-ip"
                className="font-mono"
                value={form.sourceIp}
                onChange={(e) => set("sourceIp")(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="fp-req">
                {t("waf.falsePositives.requestId")}
              </Label>
              <Input
                id="fp-req"
                className="font-mono"
                value={form.requestId}
                onChange={(e) => set("requestId")(e.target.value)}
              />
            </div>
          </div>
          <div>
            <Label htmlFor="fp-path">{t("waf.falsePositives.path")}</Label>
            <Input
              id="fp-path"
              className="font-mono"
              value={form.requestPath}
              onChange={(e) => set("requestPath")(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="fp-contact">
              {t("waf.falsePositives.contact")}
            </Label>
            <Input
              id="fp-contact"
              value={form.reporterContact}
              onChange={(e) => set("reporterContact")(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("shell.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={busy || !zoneId || !form.description.trim()}
          >
            {t("waf.falsePositives.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function HandleDialog({
  report,
  hostname,
  onClose,
}: {
  report: WafRecord<ReportData>;
  hostname: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const proposal = useChangeProposal(report.data.zoneId);
  const [note, setNote] = useState(report.data.resolutionNote ?? "");
  const open = (report.data.status ?? report.status) === "open";
  const ip = report.data.sourceIp?.trim() ?? "";

  async function decide(action: "dismiss" | "resolve", allowIp: boolean) {
    await run(async () => {
      let exceptionEntryId: string | undefined;
      if (allowIp) {
        // The exception is an ordinary whitelist entry on this zone, so it is visible and
        // removable under Whitelist/Blacklist like any other.
        const exception = {
          zoneId: report.data.zoneId,
          type: "whitelist",
          value: ip,
          enabled: true,
        };
        // Proposed instead of written: the report stays open until the change is published.
        const written = await proposal.write(async () => {
          const entry = await createRecord(ENTITIES.ipAccessLists, exception);
          exceptionEntryId = entry.data.id;
        }, [{ op: "create", entity: ENTITIES.ipAccessLists, data: exception }]);
        if (!written) return;
      }
      await transitionRecord(
        ENTITIES.falsePositives,
        report.id,
        action,
        report.version,
        { resolutionNote: note, exceptionEntryId },
      );
      invalidate();
      toast(t(`waf.falsePositives.toast.${action}`), { variant: "default" });
      onClose();
    });
  }

  return (
    <Dialog open onOpenChange={(value) => !value && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("waf.falsePositives.handleTitle")}</DialogTitle>
        </DialogHeader>
        <dl className="grid gap-2 text-sm">
          <Row label={t("waf.falsePositives.colZone")} value={hostname} />
          <Row
            label={t("waf.falsePositives.colClient")}
            value={ip || "—"}
            mono
          />
          <Row
            label={t("waf.falsePositives.requestId")}
            value={report.data.requestId ?? "—"}
            mono
          />
          <Row
            label={t("waf.falsePositives.path")}
            value={report.data.requestPath ?? "—"}
            mono
          />
          <Row
            label={t("waf.falsePositives.contact")}
            value={report.data.reporterContact ?? "—"}
          />
          <Row
            label={t("waf.falsePositives.whatHappened")}
            value={report.data.description ?? ""}
          />
        </dl>
        <p className="text-xs text-muted-foreground">
          {t("waf.falsePositives.lookUpHint")}
        </p>
        {open ? (
          <>
            <div>
              <Label htmlFor="fp-note">{t("waf.falsePositives.reply")}</Label>
              <Textarea
                id="fp-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
            {ip && isSharedAddress(ip) ? (
              <Alert>
                <AlertDescription>
                  {t("waf.falsePositives.sharedWarning", { ip })}
                </AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <ConfirmAction
                title={t("waf.falsePositives.dismiss")}
                description={t("waf.confirm.changeBody")}
                confirmLabel={t("waf.falsePositives.dismiss")}
                onConfirm={() => decide("dismiss", false)}
              >
                <Button variant="outline" disabled={busy}>
                  {t("waf.falsePositives.dismiss")}
                </Button>
              </ConfirmAction>
              <ConfirmAction
                title={t("waf.falsePositives.resolveOnly")}
                description={t("waf.confirm.changeBody")}
                confirmLabel={t("waf.falsePositives.resolveOnly")}
                onConfirm={() => decide("resolve", false)}
              >
                <Button variant="outline" disabled={busy}>
                  {t("waf.falsePositives.resolveOnly")}
                </Button>
              </ConfirmAction>
              <ConfirmAction
                title={t("waf.falsePositives.allowAndResolve")}
                description={t("waf.confirm.allowBody", { ip })}
                confirmLabel={t("waf.falsePositives.allowAndResolve")}
                destructive
                onConfirm={() => decide("resolve", true)}
              >
                <Button disabled={busy || !ip}>
                  {t("waf.falsePositives.allowAndResolve")}
                </Button>
              </ConfirmAction>
            </DialogFooter>
          </>
        ) : (
          <Row
            label={t("waf.falsePositives.reply")}
            value={report.data.resolutionNote || "—"}
          />
        )}
        {proposal.dialog}
      </DialogContent>
    </Dialog>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="grid grid-cols-[8rem_1fr] gap-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={mono ? "break-all font-mono text-xs" : "break-words"}>
        {value}
      </dd>
    </div>
  );
}
