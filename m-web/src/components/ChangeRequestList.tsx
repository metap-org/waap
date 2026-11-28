/**
 * Change requests of one zone, or of the whole tenant (`zoneId` omitted), and the one dialog where a
 * change is read and decided. Which buttons succeed is the server's call (who wrote it, the account's
 * change control, the caller's rights); the screen only leaves out what can never apply.
 */
import { useState } from "react";
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
  Label,
  SectionCard,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
  toast,
} from "@metap/ui";
import {
  ApiErrorMessage,
  formatDateTime,
  useAsyncAction,
  useCurrentUser,
  useTenantUsers,
} from "@metap/platform-ui";
import { ENTITIES, useInvalidateWaf, useRecord, useRecords } from "../api/waf";
import {
  changeAction,
  type ChangeRequest,
  type ChangeRequestData,
} from "../api/changeRequests";
import { ChangeCheckSummary, ChangeDiff, OperationList } from "./ChangeDetails";
import { ConfirmAction } from "./ConfirmAction";
import { StatusBadge } from "./primitives";

function useUserName() {
  const users = useTenantUsers();
  return (id: string | undefined) =>
    id ? (users.find((u) => u.id === id)?.email ?? id.slice(0, 8)) : "—";
}

export function ChangeRequestList({
  zoneId,
  zoneName,
}: {
  zoneId?: string;
  /** Names the zone of each row, for the list that spans zones. */
  zoneName?: (id: string) => string;
}) {
  const { t, i18n } = useTranslation();
  const userName = useUserName();
  const [open, setOpen] = useState<string | null>(null);
  const requests = useRecords<ChangeRequestData>(
    ENTITIES.changeRequests,
    zoneId ? { zoneId } : {},
    50,
    true,
  );
  const rows = requests.data ?? [];

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.changes.listTitle")}
        description={t("waf.changes.listDescription")}
      >
        {requests.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : requests.error ? (
          <ApiErrorMessage error={requests.error} />
        ) : rows.length === 0 ? (
          <EmptyState
            title={t("waf.changes.none")}
            description={t("waf.changes.noneDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("waf.changes.title")}</TableHead>
                {zoneId ? null : (
                  <TableHead>{t("waf.changes.scope")}</TableHead>
                )}
                <TableHead>{t("waf.changes.status")}</TableHead>
                <TableHead>{t("waf.changes.author")}</TableHead>
                <TableHead>{t("waf.changes.submittedAt")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell className="font-medium">
                    {row.data.title}
                  </TableCell>
                  {zoneId ? null : (
                    <TableCell>
                      {row.data.zoneId
                        ? (zoneName?.(row.data.zoneId) ?? row.data.zoneId)
                        : t("waf.changes.tenantWide")}
                    </TableCell>
                  )}
                  <TableCell>
                    <StatusBadge value={row.data.status} />
                  </TableCell>
                  <TableCell className="text-sm">
                    {userName(row.data.authorId)}
                  </TableCell>
                  <TableCell className="text-sm">
                    {row.data.submittedAt
                      ? formatDateTime(row.data.submittedAt, i18n.language)
                      : "—"}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setOpen(row.id)}
                    >
                      {t("waf.changes.open")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>
      {open ? <ChangeDialog id={open} onClose={() => setOpen(null)} /> : null}
    </div>
  );
}

function ChangeDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useTranslation();
  const record = useRecord<ChangeRequestData>(ENTITIES.changeRequests, id);
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-auto">
        {record.error ? (
          <ApiErrorMessage error={record.error} />
        ) : !record.data ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : (
          <ChangeBody change={record.data} onClose={onClose} />
        )}
      </DialogContent>
    </Dialog>
  );
}

function ChangeBody({
  change,
  onClose,
}: {
  change: ChangeRequest;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const userName = useUserName();
  const { busy, run } = useAsyncAction();
  const me = useCurrentUser().data;
  const [comment, setComment] = useState("");

  const data = change.data;
  const status = data.status;
  const isAuthor = data.authorId === me?.userId;
  const isAdmin = me?.roles.includes("admin") ?? false;
  const undecided =
    status === "draft" || status === "pendingReview" || status === "rejected";
  // A tenant-wide change is an admin's to decide.
  const mayDecide = status === "pendingReview" && (!!data.zoneId || isAdmin);

  async function act(
    action: "submit" | "approve" | "reject" | "cancel" | "revise",
    input?: Record<string, unknown>,
  ) {
    await run(async () => {
      // A submit whose check fails is refused, and the draft keeps why: reload either way.
      try {
        const outcome = await changeAction(change.id, action, input);
        toast(
          t(`waf.changes.toast.${action}`, {
            version: outcome.publishedVersion ?? "",
          }),
          { variant: "default" },
        );
      } finally {
        invalidate();
      }
    });
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {data.title} <StatusBadge value={status} />
        </DialogTitle>
      </DialogHeader>
      <p className="text-sm text-muted-foreground">
        {t("waf.changes.by", { author: userName(data.authorId) })}
        {data.decidedBy
          ? ` · ${t(`waf.changes.decision.${data.decision ?? "approved"}`, {
              who: userName(data.decidedBy),
            })}`
          : ""}
        {data.publishedVersion
          ? ` · ${t("waf.changes.version", { version: data.publishedVersion })}`
          : ""}
      </p>
      {data.decisionComment ? (
        <Alert>
          <AlertDescription>{data.decisionComment}</AlertDescription>
        </Alert>
      ) : null}

      <h3 className="text-sm font-medium">{t("waf.changes.diff")}</h3>
      <ChangeDiff diff={data.diff ?? []} />
      {data.checkResult ? (
        <ChangeCheckSummary check={data.checkResult} />
      ) : null}

      {isAuthor && undecided && data.operations.length > 1 ? (
        <>
          <h3 className="text-sm font-medium">{t("waf.changes.operations")}</h3>
          <OperationList
            operations={data.operations}
            busy={busy}
            onRemove={(index) =>
              void act("revise", {
                operations: data.operations.filter((_, i) => i !== index),
              })
            }
          />
        </>
      ) : null}

      {mayDecide ? (
        <div>
          <Label htmlFor="change-comment">{t("waf.changes.comment")}</Label>
          <Textarea
            id="change-comment"
            value={comment}
            onChange={(e) => setComment(e.target.value)}
          />
        </div>
      ) : null}

      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          {t("waf.changes.close")}
        </Button>
        {undecided && (isAuthor || isAdmin) ? (
          <ConfirmAction
            title={t("waf.changes.cancel")}
            description={t("waf.changes.cancelBody")}
            confirmLabel={t("waf.changes.cancel")}
            destructive
            onConfirm={() => act("cancel")}
          >
            <Button variant="outline" disabled={busy}>
              {t("waf.changes.cancel")}
            </Button>
          </ConfirmAction>
        ) : null}
        {isAuthor && (status === "draft" || status === "pendingReview") ? (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void act("submit")}
          >
            {t(
              status === "draft"
                ? "waf.changes.submit"
                : "waf.changes.resubmit",
            )}
          </Button>
        ) : null}
        {mayDecide && !isAuthor ? (
          <Button
            variant="outline"
            disabled={busy || !comment.trim()}
            onClick={() => void act("reject", { comment: comment.trim() })}
          >
            {t("waf.changes.reject")}
          </Button>
        ) : null}
        {mayDecide ? (
          <ConfirmAction
            title={t(
              isAuthor ? "waf.changes.confirmOwn" : "waf.changes.approve",
            )}
            description={t("waf.changes.approveBody")}
            confirmLabel={t(
              isAuthor ? "waf.changes.confirmOwn" : "waf.changes.approve",
            )}
            onConfirm={() => act("approve", { comment: comment.trim() })}
          >
            <Button disabled={busy}>
              {t(isAuthor ? "waf.changes.confirmOwn" : "waf.changes.approve")}
            </Button>
          </ConfirmAction>
        ) : null}
      </DialogFooter>
    </>
  );
}
