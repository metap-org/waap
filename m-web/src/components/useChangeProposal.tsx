/**
 * Writing policy when an account may require a change request. Screens write as before; only when the
 * server answers `change_request_required` does this ask to send the same writes for approval. A
 * screen never has to know which accounts have change control on.
 *
 *   const proposal = useChangeProposal(zoneId);
 *   const written = await proposal.write(() => deleteRecord(...), [{ op: "delete", ... }]);
 *   if (!written) return;   // proposal.dialog has taken over
 *   ...
 *   return <>{...}{proposal.dialog}</>;
 */
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
  Input,
  Label,
  Select,
  toast,
} from "@metap/ui";
import { useAsyncAction, useCurrentUser } from "@metap/platform-ui";
import { ENTITIES, useInvalidateWaf, useRecords } from "../api/waf";
import {
  changeAction,
  needsChangeRequest,
  proposeChange,
  type ChangeCheck,
  type ChangeOperation,
  type ChangeRequestData,
} from "../api/changeRequests";
import { ChangeCheckSummary, OperationList } from "./ChangeDetails";

type Pending = { operations: ChangeOperation[]; onDone?: () => void };

export function useChangeProposal(zoneId: string | undefined) {
  const [pending, setPending] = useState<Pending | null>(null);

  /** Runs the direct write. False: it needs a change request, and the dialog is open. Any other
   *  failure is thrown, as the write itself would. */
  async function write(
    direct: () => Promise<unknown>,
    operations: ChangeOperation[],
    onDone?: () => void,
  ): Promise<boolean> {
    try {
      await direct();
      return true;
    } catch (error) {
      if (!needsChangeRequest(error)) throw error;
      setPending({ operations, onDone });
      return false;
    }
  }

  const dialog = pending ? (
    <ProposeDialog
      zoneId={zoneId}
      pending={pending}
      onClose={() => setPending(null)}
    />
  ) : null;

  return { write, propose: setPending, dialog };
}

const NEW = "new";

function ProposeDialog({
  zoneId,
  pending,
  onClose,
}: {
  zoneId: string | undefined;
  pending: Pending;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const me = useCurrentUser().data?.userId;
  const [title, setTitle] = useState("");
  const [target, setTarget] = useState(NEW);
  // Set once the draft exists but its check failed: it is kept, and shown with why.
  const [failed, setFailed] = useState<ChangeCheck | null>(null);

  const requests = useRecords<ChangeRequestData>(
    ENTITIES.changeRequests,
    zoneId ? { zoneId } : {},
    50,
    true,
  );
  // Only the author may add to a change, and only while it is not decided.
  const mine = (requests.data ?? []).filter(
    (r) =>
      r.data.authorId === me &&
      (r.data.status === "draft" || r.data.status === "pendingReview") &&
      (zoneId ? r.data.zoneId === zoneId : !r.data.zoneId),
  );
  const chosen = mine.find((r) => r.id === target);

  async function send(submit: boolean) {
    await run(async () => {
      const outcome = chosen
        ? await changeAction(chosen.id, "revise", {
            operations: [...chosen.data.operations, ...pending.operations],
          })
        : await proposeChange(zoneId, title.trim(), pending.operations);
      invalidate();
      if (outcome.check && !outcome.check.ok) {
        setFailed(outcome.check);
        return;
      }
      if (submit) {
        await changeAction(outcome.id, "submit");
        invalidate();
      }
      toast(
        t(
          submit
            ? "waf.changes.toast.submitted"
            : "waf.changes.toast.savedDraft",
        ),
        { variant: "default" },
      );
      pending.onDone?.();
      onClose();
    });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("waf.changes.proposeTitle")}</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {t(
            zoneId
              ? "waf.changes.proposeZoneHint"
              : "waf.changes.proposeTenantHint",
          )}
        </p>
        <OperationList operations={pending.operations} />
        {failed ? (
          <>
            <Alert variant="destructive">
              <AlertDescription>
                {t("waf.changes.checkFailedSaved")}
              </AlertDescription>
            </Alert>
            <ChangeCheckSummary check={failed} />
            <DialogFooter>
              <Link
                className="mr-auto text-sm underline"
                to="/changes"
                onClick={onClose}
              >
                {t("waf.changes.openList")}
              </Link>
              <Button onClick={onClose}>{t("shell.cancel")}</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            {mine.length > 0 ? (
              <div>
                <Label htmlFor="change-target">{t("waf.changes.addTo")}</Label>
                <Select
                  id="change-target"
                  value={target}
                  onValueChange={(value) => setTarget(String(value))}
                  options={[
                    { value: NEW, label: t("waf.changes.newChange") },
                    ...mine.map((r) => ({
                      value: r.id,
                      label: r.data.title,
                    })),
                  ]}
                />
              </div>
            ) : null}
            {chosen ? null : (
              <div>
                <Label htmlFor="change-title">{t("waf.changes.title")}</Label>
                <Input
                  id="change-title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={onClose}>
                {t("shell.cancel")}
              </Button>
              <Button
                variant="outline"
                disabled={busy || (!chosen && !title.trim())}
                onClick={() => void send(false)}
              >
                {t("waf.changes.saveDraft")}
              </Button>
              <Button
                disabled={busy || (!chosen && !title.trim())}
                onClick={() => void send(true)}
              >
                {t("waf.changes.submit")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
