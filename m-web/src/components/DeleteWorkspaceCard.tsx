/**
 * Delete the workspace (UC-81). Only the workspace's owner may; the server refuses anyone else, so
 * this card does not try to guess who that is. Typing the workspace's name and re-entering the
 * password (and an authentication code, for an owner who has one) is the confirmation. The
 * workspace is then suspended for 30 days and every admin is emailed a link that cancels the
 * deletion; the person is signed out, since the workspace is closed to them too.
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertDescription,
  Button,
  Input,
  Label,
  SectionCard,
} from "@metap/ui";
import { apiFetch, useAsyncAction } from "@metap/platform-ui";

type Result = { effectiveAt: string; cancelLink?: string };

export function DeleteWorkspaceCard() {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction();
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [mfaCode, setMfaCode] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState<Error | null>(null);

  async function submit() {
    setError(null);
    try {
      await run(async () => {
        const res = await apiFetch<Result>("/admin/workspace/deletion", {
          method: "POST",
          body: JSON.stringify({
            confirmName: name.trim(),
            password,
            ...(mfaCode.trim() ? { mfaCode: mfaCode.trim() } : {}),
          }),
        });
        setResult(res);
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <SectionCard
      title={t("waf.settings.deleteTitle")}
      description={t("waf.settings.deleteDescription")}
    >
      {result ? (
        <Alert>
          <AlertDescription>
            {t("waf.settings.deleteScheduled", {
              when: new Date(result.effectiveAt).toLocaleDateString(),
            })}
            {result.cancelLink ? (
              <span className="mt-2 block break-all font-mono text-xs">
                {result.cancelLink}
              </span>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : (
        <div className="grid max-w-md gap-3">
          <div>
            <Label htmlFor="delete-name">
              {t("waf.settings.deleteConfirmLabel")}
            </Label>
            <Input
              id="delete-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="delete-password">
              {t("waf.settings.deletePasswordLabel")}
            </Label>
            <Input
              id="delete-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="delete-mfa">
              {t("waf.settings.deleteMfaLabel")}
            </Label>
            <Input
              id="delete-mfa"
              inputMode="numeric"
              autoComplete="one-time-code"
              value={mfaCode}
              onChange={(e) => setMfaCode(e.target.value)}
            />
          </div>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          ) : null}
          <div>
            <Button
              variant="destructive"
              disabled={busy || !name.trim() || !password}
              onClick={() => void submit()}
            >
              {t("waf.settings.deleteButton")}
            </Button>
          </div>
        </div>
      )}
    </SectionCard>
  );
}
