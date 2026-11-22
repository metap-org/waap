/**
 * Accounts inside the workspace (UC-02): a way to group zones by team, project or customer, and to
 * give a person a role only inside one group. Every workspace has a "Default" account (created with
 * its first zone), so a workspace that does not need this never has to look at it.
 *
 * Membership here is a role held inside the account (metap's scoped roles): the person sees and
 * edits the zones of this account and nothing else. Closing is refused while a zone of the account
 * is running; the server's answer names the zones.
 */
import { ConfirmAction } from "../components/ConfirmAction";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
} from "@metap/ui";
import { ApiErrorMessage, apiFetch, useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  createRecord,
  transitionRecord,
  updateRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import { StatusBadge } from "../components/primitives";

type AccountData = { name?: string; slug?: string; status?: string };
type Account = WafRecord<AccountData>;

type ScopeMember = {
  userId: string;
  email: string;
  displayName: string;
  role: string;
};

const SCOPE = "account";

/** "Team B" -> "team-b": the slug is only an identifier; the name is what people read. */
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

export function AccountsPage() {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [editing, setEditing] = useState<Account | "new" | null>(null);
  const [members, setMembers] = useState<Account | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const accounts = useRecords<AccountData>(ENTITIES.accounts, {}, 100);
  const zones = useRecords<{ accountId?: string }>(ENTITIES.zones, {}, 100);
  const zoneCount = (id: string) =>
    (zones.data ?? []).filter((z) => z.data.accountId === id).length;

  async function close(account: Account) {
    setError(null);
    try {
      await run(async () => {
        await transitionRecord(
          ENTITIES.accounts,
          account.id,
          "close",
          account.version,
        );
        invalidate();
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <div>
      <PageHeader
        title={t("waf.accounts.title")}
        description={t("waf.accounts.description")}
        actions={
          <Button onClick={() => setEditing("new")}>
            {t("waf.accounts.new")}
          </Button>
        }
      />
      {error ? (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
      {accounts.isLoading ? (
        <p className="text-sm text-muted-foreground">
          {t("waf.common.loading")}
        </p>
      ) : accounts.error ? (
        <ApiErrorMessage error={accounts.error} />
      ) : (accounts.data ?? []).length === 0 ? (
        <EmptyState
          title={t("waf.accounts.none")}
          description={t("waf.accounts.noneDescription")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.accounts.colName")}</TableHead>
              <TableHead>{t("waf.accounts.colSlug")}</TableHead>
              <TableHead>{t("waf.accounts.colZones")}</TableHead>
              <TableHead>{t("waf.accounts.colStatus")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(accounts.data ?? []).map((account) => {
              const open = (account.data.status ?? account.status) !== "closed";
              return (
                <TableRow key={account.id}>
                  <TableCell className="font-medium">
                    {account.data.name}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {account.data.slug}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {zoneCount(account.id)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      value={account.data.status ?? account.status}
                    />
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setMembers(account)}
                    >
                      {t("waf.accounts.members")}
                    </Button>
                    {open ? (
                      <>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditing(account)}
                        >
                          {t("waf.accounts.rename")}
                        </Button>
                        <ConfirmAction
                          title={t("waf.accounts.close")}
                          description={t("waf.confirm.closeAccountBody")}
                          confirmLabel={t("waf.accounts.close")}
                          destructive
                          onConfirm={() => void close(account)}
                        >
                          <Button size="sm" variant="outline" disabled={busy}>
                            {t("waf.accounts.close")}
                          </Button>
                        </ConfirmAction>
                      </>
                    ) : null}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {editing ? (
        <EditDialog
          account={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
      {members ? (
        <MembersDialog account={members} onClose={() => setMembers(null)} />
      ) : null}
    </div>
  );
}

function EditDialog({
  account,
  onClose,
}: {
  account: Account | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();
  const [name, setName] = useState(account?.data.name ?? "");
  const [error, setError] = useState<Error | null>(null);

  async function save() {
    setError(null);
    try {
      await run(async () => {
        if (account) {
          await updateRecord(ENTITIES.accounts, account.id, account.version, {
            name: name.trim(),
          });
        } else {
          await createRecord(ENTITIES.accounts, {
            name: name.trim(),
            slug: slugify(name),
          });
        }
        invalidate();
        onClose();
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {account ? t("waf.accounts.rename") : t("waf.accounts.new")}
          </DialogTitle>
        </DialogHeader>
        <div>
          <Label htmlFor="account-name">{t("waf.accounts.colName")}</Label>
          <Input
            id="account-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("shell.cancel")}
          </Button>
          <Button
            disabled={busy || !name.trim() || (!account && !slugify(name))}
            onClick={() => void save()}
          >
            {t("waf.accounts.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MembersDialog({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { busy, run } = useAsyncAction();
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState("");
  const [error, setError] = useState<Error | null>(null);
  const key = ["account-members", account.id];
  const members = useQuery({
    queryKey: key,
    queryFn: () =>
      apiFetch<{ members: ScopeMember[] }>(
        `/admin/scope-members?scopeType=${SCOPE}&scopeId=${encodeURIComponent(account.id)}`,
      ),
  });
  const users = useQuery({
    queryKey: ["account-members", "users"],
    queryFn: () =>
      apiFetch<{ users: { id: string; email: string; status: string }[] }>(
        "/admin/users?limit=100",
      ),
  });
  const roles = useQuery({
    queryKey: ["account-members", "roles"],
    queryFn: () =>
      apiFetch<{ roles: { name: string; builtin: boolean }[] }>("/admin/roles"),
  });
  // `admin` and `platform_admin` are workspace-wide powers, not something to hand out per account.
  const grantable = (roles.data?.roles ?? []).filter((r) => !r.builtin);

  async function change(action: () => Promise<unknown>) {
    setError(null);
    try {
      await run(async () => {
        await action();
        await queryClient.invalidateQueries({ queryKey: key });
      });
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t("waf.accounts.membersOf", { name: account.data.name })}
          </DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {t("waf.accounts.membersHint")}
        </p>
        {members.error ? <ApiErrorMessage error={members.error} /> : null}
        {(members.data?.members ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.accounts.noMembers")}
          </p>
        ) : (
          <ul className="divide-y text-sm">
            {(members.data?.members ?? []).map((m) => (
              <li
                key={`${m.userId}-${m.role}`}
                className="flex items-center justify-between py-2"
              >
                <span>
                  {m.email}{" "}
                  <span className="text-muted-foreground">· {m.role}</span>
                </span>
                <ConfirmAction
                  title={t("waf.accounts.revoke")}
                  description={t("waf.confirm.revokeBody", {
                    email: m.email,
                    role: m.role,
                  })}
                  confirmLabel={t("waf.accounts.revoke")}
                  destructive
                  onConfirm={() =>
                    void change(() =>
                      apiFetch(
                        `/admin/users/${encodeURIComponent(m.userId)}/roles/${encodeURIComponent(m.role)}?scopeType=${SCOPE}&scopeId=${encodeURIComponent(account.id)}`,
                        { method: "DELETE" },
                      ),
                    )
                  }
                >
                  <Button size="sm" variant="ghost" disabled={busy}>
                    {t("waf.accounts.revoke")}
                  </Button>
                </ConfirmAction>
              </li>
            ))}
          </ul>
        )}
        <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <div>
            <Label htmlFor="member-user">{t("waf.accounts.person")}</Label>
            <Select
              id="member-user"
              value={userId}
              onValueChange={(v) => setUserId(String(v))}
              options={(users.data?.users ?? [])
                .filter((u) => u.status === "active")
                .map((u) => ({ value: u.id, label: u.email }))}
            />
          </div>
          <div>
            <Label htmlFor="member-role">{t("waf.accounts.role")}</Label>
            <Select
              id="member-role"
              value={role}
              onValueChange={(v) => setRole(String(v))}
              options={grantable.map((r) => ({ value: r.name, label: r.name }))}
            />
          </div>
          <Button
            disabled={busy || !userId || !role}
            onClick={() =>
              void change(() =>
                apiFetch(`/admin/users/${encodeURIComponent(userId)}/roles`, {
                  method: "POST",
                  body: JSON.stringify({
                    role,
                    scopeType: SCOPE,
                    scopeId: account.id,
                  }),
                }),
              )
            }
          >
            {t("waf.accounts.grant")}
          </Button>
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("waf.accounts.done")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
