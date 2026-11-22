/**
 * Asks before a click that changes something. Wrap the button that does it:
 *
 *   <ConfirmAction title="Delete rule?" description="…" confirmLabel="Delete" destructive onConfirm={…}>
 *     <Button>Delete</Button>
 *   </ConfirmAction>
 *
 * The wrapped element is only the trigger (it opens the dialog); `onConfirm` runs when the person
 * confirms, and never on the first click. Every one-click action in the portal goes through this,
 * so a stray click cannot delete a rule, flip a zone's state or put an address on a list.
 */
import type { ReactNode } from "react";
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
} from "@metap/ui";
import { useTranslation } from "react-i18next";

export function ConfirmAction({
  title,
  description,
  confirmLabel,
  destructive = false,
  onConfirm,
  children,
}: {
  title: string;
  description?: ReactNode;
  /** The confirm button's text; defaults to the generic "Confirm". */
  confirmLabel?: string;
  /** Styles the confirm button as destructive (delete, block, suspend). */
  destructive?: boolean;
  onConfirm: () => void | Promise<void>;
  /** The trigger: one element that accepts a click (a Button, an IconButton). */
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>{children}</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("shell.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className={
              destructive
                ? "bg-destructive text-destructive-foreground hover:bg-destructive/90"
                : undefined
            }
            onClick={() => void onConfirm()}
          >
            {confirmLabel ?? t("waf.common.confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
