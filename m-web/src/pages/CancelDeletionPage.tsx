/**
 * The page the "workspace deletion requested" email links to (UC-81). Public: the workspace is
 * suspended while the deletion waits, so nobody can sign in to cancel it. The token in the link
 * cancels the deletion once; the page asks first, because opening an email link should not undo
 * anything by itself (mail scanners open links).
 */
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, Button } from "@metap/ui";
import { apiFetch, useAsyncAction } from "@metap/platform-ui";

export function CancelDeletionPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const { busy, run } = useAsyncAction();
  const [done, setDone] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  async function cancel() {
    setError(null);
    try {
      await run(async () => {
        await apiFetch("/auth/workspace-deletion/cancel", {
          method: "POST",
          body: JSON.stringify({ token }),
        });
        setDone(true);
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <div className="mx-auto max-w-sm px-4 py-12">
      <h2 className="mb-4 text-xl font-semibold">
        {t("waf.cancelDeletion.title")}
      </h2>
      {done ? (
        <div className="flex flex-col gap-4">
          <Alert>
            <AlertDescription>{t("waf.cancelDeletion.done")}</AlertDescription>
          </Alert>
          <Link to="/login" className="text-sm text-primary hover:underline">
            {t("waf.cancelDeletion.signIn")}
          </Link>
        </div>
      ) : !token ? (
        <Alert variant="destructive">
          <AlertDescription>{t("waf.cancelDeletion.noToken")}</AlertDescription>
        </Alert>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            {t("waf.cancelDeletion.explain")}
          </p>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          ) : null}
          <Button onClick={() => void cancel()} disabled={busy}>
            {t("waf.cancelDeletion.button")}
          </Button>
        </div>
      )}
    </div>
  );
}
