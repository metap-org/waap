/**
 * Module 8 — alerting. Two halves that belong on one screen: the policies (configuration) and the
 * notification log (what actually happened), because the only way to trust an alert policy is to
 * see what it has delivered.
 *
 * "Send test" and "Evaluate now" both hit the real backend paths — the same delivery code a
 * scheduled evaluation uses — so a green result here means the channel genuinely works, not that
 * a mock succeeded.
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
  PageHeader,
  SectionCard,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
  Toggle,
  toast,
} from "@metap/ui";
import {
  ENTITIES,
  createRecord,
  evaluateAlerts,
  testAlertPolicy,
  updateRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { ApiErrorMessage, shortDate, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "../components/primitives";

type PolicyData = {
  name?: string;
  thresholdCount?: number;
  windowMinutes?: number;
  channels?: Record<string, unknown>;
  enabled?: boolean;
};

type NotificationData = {
  alertPolicyId?: string;
  channel?: string;
  deliveryStatus?: string;
  triggeredAt?: string;
};

const EMPTY: PolicyData = {
  name: "",
  thresholdCount: 50,
  windowMinutes: 15,
  enabled: true,
  channels: { webhook: "" },
};

export function AlertingPage() {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const policies = useRecords<PolicyData>(ENTITIES.alertPolicies, {}, 50);
  const notifications = useRecords<NotificationData>(
    ENTITIES.alertNotifications,
    {},
    25,
  );
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<WafRecord<PolicyData> | null>(null);
  const [draft, setDraft] = useState<PolicyData>(EMPTY);
  const [channelsText, setChannelsText] = useState(
    JSON.stringify(EMPTY.channels, null, 2),
  );
  const { busy, run } = useAsyncAction();

  function startCreate() {
    setEditing(null);
    setDraft(EMPTY);
    setChannelsText(JSON.stringify(EMPTY.channels, null, 2));
    setOpen(true);
  }

  function startEdit(policy: WafRecord<PolicyData>) {
    setEditing(policy);
    setDraft(policy.data);
    setChannelsText(JSON.stringify(policy.data.channels ?? {}, null, 2));
    setOpen(true);
  }

  async function save() {
    await run(async () => {
      let channels: unknown;
      try {
        channels = JSON.parse(channelsText);
      } catch {
        toast(t("waf.alerting.toastInvalidJson"), { variant: "destructive" });
        return;
      }
      const payload = { ...draft, channels };
      if (editing) {
        await updateRecord(
          ENTITIES.alertPolicies,
          editing.id,
          editing.version,
          payload,
        );
      } else {
        await createRecord(ENTITIES.alertPolicies, payload);
      }
      invalidate();
      setOpen(false);
      toast(t("waf.alerting.toastSaved"), { variant: "default" });
    });
  }

  async function sendTest(policy: WafRecord<PolicyData>) {
    await run(async () => {
      const result = await testAlertPolicy(policy.id);
      invalidate();
      toast(
        t(
          result.data.delivered
            ? "waf.alerting.toastDelivered"
            : "waf.alerting.toastNotDelivered",
          {
            detail: result.data.detail,
          },
        ),
        {
          variant: result.data.delivered ? "default" : "destructive",
        },
      );
    });
  }

  async function evaluateNow() {
    await run(async () => {
      const result = await evaluateAlerts();
      invalidate();
      toast(
        t("waf.alerting.toastEvaluated", {
          policies: result.data.policiesEvaluated,
          fired: result.data.fired.length,
        }),
        {
          variant: "default",
        },
      );
    });
  }

  const policyName = (id?: string) =>
    policies.data?.find((p) => p.id === id)?.data.name ?? id ?? "—";

  return (
    <div>
      <PageHeader
        title={t("waf.alerting.title")}
        description={t("waf.alerting.description")}
        actions={
          <>
            <Button variant="outline" onClick={evaluateNow} disabled={busy}>
              {t("waf.alerting.evaluateNow")}
            </Button>
            <Button onClick={startCreate}>{t("waf.alerting.newPolicy")}</Button>
          </>
        }
      />

      <div className="grid gap-4">
        <SectionCard
          title={t("waf.alerting.policiesTitle")}
          description={t("waf.alerting.policiesDescription")}
        >
          {policies.isLoading ? (
            <p className="text-sm text-muted-foreground">
              {t("waf.common.loading")}
            </p>
          ) : policies.error ? (
            <ApiErrorMessage error={policies.error} />
          ) : (policies.data ?? []).length === 0 ? (
            <EmptyState
              title={t("waf.alerting.noPolicies")}
              description={t("waf.alerting.noPoliciesDescription")}
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("waf.alerting.colName")}</TableHead>
                  <TableHead>{t("waf.alerting.colRule")}</TableHead>
                  <TableHead>{t("waf.alerting.colChannel")}</TableHead>
                  <TableHead>{t("waf.alerting.colEnabled")}</TableHead>
                  <TableHead className="text-right">
                    {t("waf.alerting.colActions")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(policies.data ?? []).map((policy) => (
                  <TableRow key={policy.id}>
                    <TableCell className="font-medium">
                      {policy.data.name}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {t("waf.alerting.ruleHint", {
                        threshold: policy.data.thresholdCount,
                        window: policy.data.windowMinutes,
                      })}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {Object.keys(policy.data.channels ?? {}).join(", ") ||
                        "—"}
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
                        onClick={() => sendTest(policy)}
                        disabled={busy}
                      >
                        {t("waf.alerting.sendTest")}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="ml-1"
                        onClick={() => startEdit(policy)}
                      >
                        {t("waf.common.edit")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>

        <SectionCard
          title={t("waf.alerting.logTitle")}
          description={t("waf.alerting.logDescription")}
        >
          {notifications.isLoading ? (
            <p className="text-sm text-muted-foreground">
              {t("waf.common.loading")}
            </p>
          ) : notifications.error ? (
            <ApiErrorMessage error={notifications.error} />
          ) : (notifications.data ?? []).length === 0 ? (
            <EmptyState title={t("waf.alerting.noDeliveries")} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("waf.alerting.colWhen")}</TableHead>
                  <TableHead>{t("waf.alerting.colPolicy")}</TableHead>
                  <TableHead>{t("waf.alerting.colChannel")}</TableHead>
                  <TableHead>{t("waf.alerting.colStatus")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(notifications.data ?? []).map((notification) => (
                  <TableRow key={notification.id}>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {shortDate(
                        notification.data.triggeredAt ?? notification.createdAt,
                      )}
                    </TableCell>
                    <TableCell>
                      {policyName(notification.data.alertPolicyId)}
                    </TableCell>
                    <TableCell>{notification.data.channel}</TableCell>
                    <TableCell>
                      <StatusBadge value={notification.data.deliveryStatus} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t("waf.alerting.editPolicy")
                : t("waf.alerting.newPolicyTitle")}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div>
              <Label htmlFor="policy-name">{t("waf.alerting.name")}</Label>
              <Input
                id="policy-name"
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="policy-threshold">
                  {t("waf.alerting.threshold")}
                </Label>
                <Input
                  id="policy-threshold"
                  type="number"
                  value={draft.thresholdCount ?? 0}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      thresholdCount: Number(e.target.value),
                    })
                  }
                />
              </div>
              <div>
                <Label htmlFor="policy-window">
                  {t("waf.alerting.window")}
                </Label>
                <Input
                  id="policy-window"
                  type="number"
                  value={draft.windowMinutes ?? 0}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      windowMinutes: Number(e.target.value),
                    })
                  }
                />
              </div>
            </div>
            <div>
              <Label htmlFor="policy-channels">
                {t("waf.alerting.channels")}
              </Label>
              <Textarea
                id="policy-channels"
                rows={4}
                className="font-mono text-xs"
                value={channelsText}
                onChange={(e) => setChannelsText(e.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.alerting.channelsHint")}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Toggle
                id="policy-enabled"
                checked={draft.enabled ?? true}
                onCheckedChange={(checked) =>
                  setDraft({ ...draft, enabled: checked })
                }
              />
              <Label htmlFor="policy-enabled">
                {t("waf.alerting.enabled")}
              </Label>
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
