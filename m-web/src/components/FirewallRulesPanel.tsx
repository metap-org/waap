/**
 * Module 4 — firewall rules. One engine for WAF custom rules, rate limiting and IP/geo firewall
 * (`docs/01-product-vision.md`'s deliberate decision not to ship three separate features), which
 * is why `ruleType` is a field on one entity rather than three entities.
 *
 * Priority is what makes this more than a CRUD list: rules are evaluated in priority order and the
 * first match wins, so the screen has to show that order and let it be changed. Reordering swaps
 * two rules' `priority` values rather than renumbering the whole list — two writes instead of N,
 * and no window where two rules share a priority.
 *
 * Shared by two contexts, distinguished only by whether `zoneId` is passed:
 * - `../pages/zone/ZoneRulesTab.tsx` — one zone's own rules (`zoneId` set).
 * - `../pages/GlobalRulesPage.tsx` — tenant-wide ("global") rules that apply to every zone
 *   (`zoneId` omitted). See `../../../c-config-distributor/management`'s
 *   `TenantWideRulesFor` for how a `zoneId: null` row gets merged into every zone at compile
 *   time.
 *
 * The global fetch can't ask the server to filter `zoneId IS NULL` through this hook:
 * `@metap/platform-ui`'s `useRecords` strips any filter value that is `""`/`undefined`
 * before building the query string, so the backend's own `?zoneId=` (empty string) → `IS
 * NULL` convention (metap's `pkg/query`) is unreachable through it. Fetching all
 * rows unfiltered and filtering client-side for `!rule.data.zoneId` is the workaround — changing
 * the shared hook is out of scope here (it would affect every other consumer app).
 */
import { ConfirmAction } from "./ConfirmAction";
import { useChangeProposal } from "./useChangeProposal";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  ButtonGroup,
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
  TagsInput,
  Toggle,
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
  ENTITIES,
  createRecord,
  deleteRecord,
  syncConfigState,
  updateRecord,
  useInvalidateWaf,
  useRecords,
  type WafRecord,
} from "../api/waf";
import {
  ApiErrorMessage,
  formatDateTime,
  useAsyncAction,
} from "@metap/platform-ui";
import { StatusBadge } from "./primitives";
import { ActionPicker, SectionLabel } from "./RuleFormParts";
import {
  FIELDS,
  FIELD_LABEL_KEYS,
  OPS,
  OP_LABEL_KEYS,
  builderToJson,
  emptyPredicate,
  jsonToBuilder,
  type BuilderCombinator,
  type BuilderState,
  type MatchField,
  type MatchOp,
  type Predicate,
} from "../lib/matchCondition";

type RuleData = {
  zoneId?: string;
  name?: string;
  ruleType?: string;
  priority?: number;
  action?: string;
  enabled?: boolean;
  matchCondition?: unknown;
  rateLimitThreshold?: number;
  rateLimitWindow?: number;
  expiresAt?: string | null;
};

const EMPTY: RuleData = {
  name: "",
  ruleType: "waf",
  action: "block",
  priority: 100,
  enabled: true,
  matchCondition: { field: "uri.path", op: "contains", value: "/admin" },
};

/** `zoneId` present -> this zone's own rules (unchanged behaviour). Omitted -> every tenant-wide
 *  rule, fetched unfiltered and filtered client-side (see the file header for why). */
export function FirewallRulesPanel({ zoneId }: { zoneId?: string }) {
  const { t, i18n } = useTranslation();
  const invalidate = useInvalidateWaf();
  const scoped = useRecords<RuleData>(
    ENTITIES.firewallRules,
    zoneId ? { zoneId } : {},
    zoneId ? 100 : 200,
  );
  const rules = zoneId
    ? scoped
    : { ...scoped, data: scoped.data?.filter((r) => !r.data.zoneId) };
  const [editing, setEditing] = useState<WafRecord<RuleData> | null>(null);
  const [draft, setDraft] = useState<RuleData>(EMPTY);
  const [conditionText, setConditionText] = useState(
    JSON.stringify(EMPTY.matchCondition, null, 2),
  );
  // `null` builder means "this condition is too complex for the builder" — `mode` then stays
  // (or is forced to) "advanced" and the raw JSON editor is the only way to change it.
  const [builder, setBuilder] = useState<BuilderState>(
    jsonToBuilder(EMPTY.matchCondition) ?? {
      combinator: "all",
      predicates: [],
    },
  );
  const [conditionMode, setConditionMode] = useState<"builder" | "advanced">(
    "builder",
  );
  // Distinguishes "started in advanced mode because this rule's saved condition uses nested
  // groups the builder can't show" from "the user chose Advanced themselves" — only the former
  // needs the persistent inline notice, the latter is just a normal mode the user picked.
  const [autoFellBackToAdvanced, setAutoFellBackToAdvanced] = useState(false);
  const [open, setOpen] = useState(false);
  // "" = permanent, "keep" = leave an existing expiry as it is, else hours from now (UC-51).
  const [expiry, setExpiry] = useState("");
  const { busy, run } = useAsyncAction();
  const proposal = useChangeProposal(zoneId);

  function loadCondition(condition: unknown) {
    setConditionText(JSON.stringify(condition ?? null, null, 2));
    const parsed = jsonToBuilder(condition);
    if (parsed) {
      setBuilder(parsed);
      setConditionMode("builder");
      setAutoFellBackToAdvanced(false);
    } else {
      setConditionMode("advanced");
      setAutoFellBackToAdvanced(true);
    }
  }

  function updatePredicate(index: number, patch: Partial<Predicate>) {
    setBuilder((current) => ({
      ...current,
      predicates: current.predicates.map((p, i) =>
        i === index ? { ...p, ...patch } : p,
      ),
    }));
  }

  function addPredicate() {
    setBuilder((current) => ({
      ...current,
      predicates: [...current.predicates, emptyPredicate()],
    }));
  }

  function removePredicate(index: number) {
    setBuilder((current) => ({
      ...current,
      predicates: current.predicates.filter((_, i) => i !== index),
    }));
  }

  function switchToAdvanced() {
    setConditionText(JSON.stringify(builderToJson(builder), null, 2));
    setConditionMode("advanced");
    setAutoFellBackToAdvanced(false);
  }

  function switchToBuilder() {
    let parsed: unknown;
    try {
      parsed = JSON.parse(conditionText);
    } catch {
      toast(t("waf.zoneTabs.rules.toastInvalidJson"), {
        variant: "destructive",
      });
      return;
    }
    const next = jsonToBuilder(parsed);
    if (!next) {
      toast(t("waf.zoneTabs.rules.advancedFallbackNotice"), {
        variant: "destructive",
      });
      return;
    }
    setBuilder(next);
    setConditionMode("builder");
  }

  // Sorted here rather than by the list API: `priority` is not in the entity's sortable fields,
  // and the list is small (one zone's rules, or the tenant's global rules), so ordering
  // client-side is cheaper than widening the metadata for it.
  const ordered = [...(rules.data ?? [])].sort(
    (a, b) => (a.data.priority ?? 0) - (b.data.priority ?? 0),
  );

  function startCreate() {
    setEditing(null);
    const nextPriority =
      ordered.length > 0
        ? (ordered[ordered.length - 1]?.data.priority ?? 0) + 10
        : 100;
    setDraft({ ...EMPTY, priority: nextPriority });
    setExpiry("");
    loadCondition(EMPTY.matchCondition);
    setOpen(true);
  }

  function startEdit(rule: WafRecord<RuleData>) {
    setEditing(rule);
    setDraft(rule.data);
    setExpiry(rule.data.expiresAt ? "keep" : "");
    loadCondition(rule.data.matchCondition);
    setOpen(true);
  }

  async function save() {
    await run(async () => {
      let matchCondition: unknown;
      if (conditionMode === "builder") {
        matchCondition = builderToJson(builder);
      } else {
        try {
          matchCondition = JSON.parse(conditionText);
        } catch {
          toast(t("waf.zoneTabs.rules.toastInvalidJson"), {
            variant: "destructive",
          });
          return;
        }
      }
      const expiresAt =
        expiry === "keep"
          ? draft.expiresAt
          : expiry === ""
            ? null
            : new Date(Date.now() + Number(expiry) * 3600 * 1000).toISOString();
      const payload = zoneId
        ? { ...draft, matchCondition, zoneId, expiresAt }
        : { ...draft, matchCondition, expiresAt };
      const written = editing
        ? await proposal.write(
            () =>
              updateRecord(
                ENTITIES.firewallRules,
                editing.id,
                editing.version,
                payload,
              ),
            [
              {
                op: "update",
                entity: ENTITIES.firewallRules,
                id: editing.id,
                data: payload,
              },
            ],
          )
        : await proposal.write(
            () => createRecord(ENTITIES.firewallRules, payload),
            [{ op: "create", entity: ENTITIES.firewallRules, data: payload }],
          );
      if (!written) {
        setOpen(false);
        return;
      }
      // A global (tenant-wide) rule has no single zone to resync — the next periodic resync
      // picks it up, an accepted v1 trade-off (see `sync.rs`'s `zone_id_from_event`).
      if (zoneId) {
        await syncConfigState(zoneId);
      }
      invalidate();
      setOpen(false);
      toast(t("waf.zoneTabs.rules.toastSaved"), { variant: "default" });
    });
  }

  async function remove(rule: WafRecord<RuleData>) {
    await run(async () => {
      const written = await proposal.write(
        () => deleteRecord(ENTITIES.firewallRules, rule.id, rule.version),
        [{ op: "delete", entity: ENTITIES.firewallRules, id: rule.id }],
      );
      if (!written) return;
      if (zoneId) {
        await syncConfigState(zoneId);
      }
      invalidate();
      toast(t("waf.zoneTabs.rules.toastDeleted"), { variant: "default" });
    });
  }

  /** Swaps this rule's priority with its neighbour's — see the file header for why swap, not
   *  renumber. */
  async function move(rule: WafRecord<RuleData>, direction: -1 | 1) {
    const index = ordered.findIndex((r) => r.id === rule.id);
    const neighbour = ordered[index + direction];
    if (!neighbour) return;
    await run(async () => {
      const swap = [
        { rule, priority: neighbour.data.priority },
        { rule: neighbour, priority: rule.data.priority },
      ];
      // Change control refuses the first of the two, so neither is written and both are proposed.
      await proposal.write(
        async () => {
          for (const { rule: target, priority } of swap) {
            await updateRecord(
              ENTITIES.firewallRules,
              target.id,
              target.version,
              { priority },
            );
          }
        },
        swap.map(({ rule: target, priority }) => ({
          op: "update" as const,
          entity: ENTITIES.firewallRules,
          id: target.id,
          data: { name: target.data.name, priority },
        })),
      );
      invalidate();
    });
  }

  return (
    <div className="mt-4">
      <SectionCard
        title={t("waf.zoneTabs.rules.title")}
        description={t("waf.zoneTabs.rules.description")}
        actions={
          <Button size="sm" onClick={startCreate}>
            {t("waf.zoneTabs.rules.addRule")}
          </Button>
        }
      >
        {rules.isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t("waf.common.loading")}
          </p>
        ) : rules.error ? (
          <ApiErrorMessage error={rules.error} />
        ) : ordered.length === 0 ? (
          <EmptyState
            title={t("waf.zoneTabs.rules.noRulesYet")}
            description={t("waf.zoneTabs.rules.noRulesYetDescription")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">
                  {t("waf.zoneTabs.rules.colPriority")}
                </TableHead>
                <TableHead>{t("waf.zoneTabs.rules.colName")}</TableHead>
                <TableHead>{t("waf.zoneTabs.rules.colType")}</TableHead>
                <TableHead>{t("waf.zoneTabs.rules.colAction")}</TableHead>
                <TableHead>{t("waf.zoneTabs.rules.colEnabled")}</TableHead>
                <TableHead>{t("waf.zoneTabs.rules.colExpires")}</TableHead>
                <TableHead className="text-right">
                  {t("waf.zoneTabs.rules.colActions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {ordered.map((rule, index) => (
                <TableRow key={rule.id}>
                  <TableCell className="tabular-nums">
                    <div className="flex items-center gap-1">
                      {rule.data.priority}
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || index === 0}
                        onClick={() => move(rule, -1)}
                        aria-label={t("waf.zoneTabs.rules.moveUp")}
                      >
                        ↑
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || index === ordered.length - 1}
                        onClick={() => move(rule, 1)}
                        aria-label={t("waf.zoneTabs.rules.moveDown")}
                      >
                        ↓
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell className="font-medium">
                    {rule.data.name}
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={rule.data.ruleType} />
                  </TableCell>
                  <TableCell>
                    <StatusBadge value={rule.data.action} />
                  </TableCell>
                  <TableCell>
                    {rule.data.enabled
                      ? t("waf.common.yes")
                      : t("waf.common.no")}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {rule.data.expiresAt
                      ? Date.parse(rule.data.expiresAt) <= Date.now()
                        ? t("waf.zoneTabs.rules.expired")
                        : formatDateTime(rule.data.expiresAt, i18n.language)
                      : "—"}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => startEdit(rule)}
                    >
                      {t("waf.common.edit")}
                    </Button>
                    <ConfirmAction
                      title={t("waf.common.delete")}
                      description={t("waf.confirm.deleteBody")}
                      confirmLabel={t("waf.common.delete")}
                      destructive
                      onConfirm={() => remove(rule)}
                    >
                      <Button
                        size="sm"
                        variant="ghost"

                        disabled={busy}
                      >
                        {t("waf.common.delete")}
                      </Button>
                    </ConfirmAction>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>

      <Dialog open={open} onOpenChange={setOpen}>
        {/* Header and footer stay put; only the body scrolls. */}
        <DialogContent className="flex max-h-[90vh] max-w-3xl flex-col gap-0 p-0">
          <DialogHeader className="px-lg pb-md pt-lg">
            <DialogTitle>
              {editing
                ? t("waf.zoneTabs.rules.editRule")
                : t("waf.zoneTabs.rules.newRule")}
            </DialogTitle>
            <p className="text-sm text-muted-foreground">
              {t("waf.zoneTabs.rules.dialogHint")}
            </p>
          </DialogHeader>

          <div className="grid gap-6 overflow-y-auto border-y border-border px-lg py-lg">
            <div>
              <Label htmlFor="rule-name">{t("waf.zoneTabs.rules.name")}</Label>
              <Input
                id="rule-name"
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </div>

            <section>
              <SectionLabel
                aside={
                  <>
                    {conditionMode === "builder" &&
                    builder.predicates.length > 1 ? (
                      <ButtonGroup
                        aria-label={t("waf.zoneTabs.rules.combinatorLabel")}
                      >
                        {(["all", "any"] as const).map((value) => (
                          <Button
                            key={value}
                            type="button"
                            size="sm"
                            variant={
                              builder.combinator === value
                                ? "default"
                                : "outline"
                            }
                            aria-pressed={builder.combinator === value}
                            onClick={() =>
                              setBuilder((current) => ({
                                ...current,
                                combinator: value as BuilderCombinator,
                              }))
                            }
                          >
                            {t(
                              value === "all"
                                ? "waf.zoneTabs.rules.combinatorAll"
                                : "waf.zoneTabs.rules.combinatorAny",
                            )}
                          </Button>
                        ))}
                      </ButtonGroup>
                    ) : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={
                        conditionMode === "builder"
                          ? switchToAdvanced
                          : switchToBuilder
                      }
                    >
                      {conditionMode === "builder"
                        ? t("waf.zoneTabs.rules.switchToAdvanced")
                        : t("waf.zoneTabs.rules.switchToBuilder")}
                    </Button>
                  </>
                }
              >
                {t("waf.zoneTabs.rules.sectionWhen")}
              </SectionLabel>

              {conditionMode === "builder" ? (
                <div className="grid gap-2">
                  {builder.predicates.length === 0 ? (
                    <div className="rounded-md border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
                      {t("waf.zoneTabs.rules.noPredicatesYet")}
                    </div>
                  ) : null}
                  {builder.predicates.map((predicate, index) => (
                    // One condition reads left to right as a sentence: "If | path | contains | /x".
                    <div
                      key={index}
                      className="grid items-start gap-2 sm:grid-cols-[2.75rem_9.5rem_10.5rem_minmax(0,1fr)_auto]"
                    >
                      <span className="pt-2.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        {index === 0
                          ? t("waf.zoneTabs.rules.if")
                          : t(
                              builder.combinator === "any"
                                ? "waf.zoneTabs.rules.or"
                                : "waf.zoneTabs.rules.and",
                            )}
                      </span>
                      <Select
                        aria-label={t("waf.zoneTabs.rules.fieldLabel")}
                        value={predicate.field}
                        onValueChange={(value) =>
                          updatePredicate(index, { field: value as MatchField })
                        }
                        options={FIELDS.map((f) => ({
                          value: f,
                          label: t(`waf.zoneTabs.rules.${FIELD_LABEL_KEYS[f]}`),
                        }))}
                      />
                      <Select
                        aria-label={t("waf.zoneTabs.rules.operatorLabel")}
                        value={predicate.op}
                        onValueChange={(value) =>
                          updatePredicate(index, { op: value as MatchOp })
                        }
                        options={OPS.map((op) => ({
                          value: op,
                          label: t(`waf.zoneTabs.rules.${OP_LABEL_KEYS[op]}`),
                        }))}
                      />
                      {predicate.op === "in" || predicate.op === "notIn" ? (
                        <TagsInput
                          value={predicate.values}
                          onChange={(values) =>
                            updatePredicate(index, { values })
                          }
                          placeholder={t("waf.zoneTabs.rules.valuesLabel")}
                        />
                      ) : (
                        <Input
                          aria-label={t("waf.zoneTabs.rules.valueLabel")}
                          placeholder={
                            predicate.field === "sourceIpCidr"
                              ? t("waf.zoneTabs.rules.valuePlaceholderCidr")
                              : predicate.op === "regex"
                                ? t("waf.zoneTabs.rules.valuePlaceholderRegex")
                                : t("waf.zoneTabs.rules.valueLabel")
                          }
                          value={predicate.value}
                          onChange={(e) =>
                            updatePredicate(index, { value: e.target.value })
                          }
                        />
                      )}
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={t("waf.zoneTabs.rules.removePredicate")}
                        title={t("waf.zoneTabs.rules.removePredicate")}
                        onClick={() => removePredicate(index)}
                      >
                        ×
                      </Button>
                      {predicate.field === "header" ? (
                        <Input
                          className="sm:col-start-2 sm:col-end-5"
                          placeholder={t(
                            "waf.zoneTabs.rules.headerNamePlaceholder",
                          )}
                          aria-label={t("waf.zoneTabs.rules.headerNameLabel")}
                          value={predicate.param}
                          onChange={(e) =>
                            updatePredicate(index, { param: e.target.value })
                          }
                        />
                      ) : null}
                    </div>
                  ))}
                  <div>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={addPredicate}
                    >
                      {t("waf.zoneTabs.rules.addPredicate")}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="grid gap-2">
                  {autoFellBackToAdvanced ? (
                    <p className="text-xs text-destructive">
                      {t("waf.zoneTabs.rules.advancedFallbackNotice")}
                    </p>
                  ) : null}
                  <Textarea
                    id="rule-condition"
                    rows={8}
                    className="font-mono text-xs"
                    value={conditionText}
                    onChange={(e) => setConditionText(e.target.value)}
                  />
                </div>
              )}
            </section>

            <section>
              <SectionLabel>{t("waf.zoneTabs.rules.sectionThen")}</SectionLabel>
              <ActionPicker
                label={t("waf.zoneTabs.rules.action")}
                value={draft.action}
                onChange={(value) => setDraft({ ...draft, action: value })}
                options={[
                  {
                    value: "block",
                    label: t("waf.zoneTabs.rules.actionBlock"),
                    hint: t("waf.zoneTabs.rules.actionBlockHint"),
                  },
                  {
                    value: "challenge",
                    label: t("waf.zoneTabs.rules.actionChallenge"),
                    hint: t("waf.zoneTabs.rules.actionChallengeHint"),
                  },
                  {
                    value: "log",
                    label: t("waf.zoneTabs.rules.actionLog"),
                    hint: t("waf.zoneTabs.rules.actionLogHint"),
                  },
                  {
                    value: "allow",
                    label: t("waf.zoneTabs.rules.actionAllow"),
                    hint: t("waf.zoneTabs.rules.actionAllowHint"),
                  },
                ]}
              />
              {/* A rate-limit rule is the same rule with a counter in front: it acts only once a
                  client has sent more than this many requests within the window. `ipFirewall`/
                  `geoFirewall` rule types were removed (Increment 2): IP lists have their own
                  screen, and a country rule is an ordinary rule on the `country` field. */}
              <div className="mt-4 rounded-md border border-border p-3">
                <div className="flex items-center gap-2">
                  <Toggle
                    id="rule-rate-limit"
                    checked={draft.ruleType === "rateLimit"}
                    onCheckedChange={(checked) =>
                      setDraft({
                        ...draft,
                        ruleType: checked ? "rateLimit" : "waf",
                      })
                    }
                  />
                  <Label htmlFor="rule-rate-limit">
                    {t("waf.zoneTabs.rules.typeRateLimit")}
                  </Label>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("waf.zoneTabs.rules.rateLimitHint")}
                </p>
                {draft.ruleType === "rateLimit" ? (
                  <div className="mt-3 grid gap-3 sm:grid-cols-2">
                    <div>
                      <Label htmlFor="rl-threshold">
                        {t("waf.zoneTabs.rules.requests")}
                      </Label>
                      <Input
                        id="rl-threshold"
                        type="number"
                        value={draft.rateLimitThreshold ?? 100}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            rateLimitThreshold: Number(e.target.value),
                          })
                        }
                      />
                    </div>
                    <div>
                      <Label htmlFor="rl-window">
                        {t("waf.zoneTabs.rules.windowSeconds")}
                      </Label>
                      <Input
                        id="rl-window"
                        type="number"
                        value={draft.rateLimitWindow ?? 60}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            rateLimitWindow: Number(e.target.value),
                          })
                        }
                      />
                    </div>
                  </div>
                ) : null}
              </div>
            </section>

            {/* Priority is set for a new rule (after the last one) and rarely needs touching, so
                it sits behind a line that says what it currently is. */}
            <details className="group rounded-md border border-border">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-3 py-2 text-sm">
                <span className="font-medium">
                  {t("waf.zoneTabs.rules.sectionOptions")}
                </span>
                <span className="text-xs text-muted-foreground">
                  {t("waf.zoneTabs.rules.priority")} {draft.priority ?? 100} ·{" "}
                  {t(
                    expiry === ""
                      ? "waf.zoneTabs.rules.expiryNever"
                      : expiry === "keep"
                        ? "waf.zoneTabs.rules.expiryKeep"
                        : "waf.zoneTabs.rules.expiryTemporary",
                  )}
                </span>
              </summary>
              <div className="grid gap-3 border-t border-border p-3 sm:grid-cols-2">
                <div>
                  <Label htmlFor="rule-priority">
                    {t("waf.zoneTabs.rules.priority")}
                  </Label>
                  <Input
                    id="rule-priority"
                    type="number"
                    value={draft.priority ?? 100}
                    onChange={(e) =>
                      setDraft({ ...draft, priority: Number(e.target.value) })
                    }
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("waf.zoneTabs.rules.priorityHint")}
                  </p>
                </div>
                <div>
                  <Label htmlFor="rule-expiry">
                    {t("waf.zoneTabs.rules.expiry")}
                  </Label>
                  <Select
                    id="rule-expiry"
                    value={expiry}
                    onValueChange={(value) => setExpiry(String(value))}
                    options={[
                      {
                        value: "",
                        label: t("waf.zoneTabs.rules.expiryNever"),
                      },
                      ...(editing?.data.expiresAt
                        ? [
                            {
                              value: "keep",
                              label: t("waf.zoneTabs.rules.expiryKeep"),
                            },
                          ]
                        : []),
                      { value: "1", label: t("waf.zoneTabs.rules.expiry1h") },
                      { value: "6", label: t("waf.zoneTabs.rules.expiry6h") },
                      { value: "24", label: t("waf.zoneTabs.rules.expiry24h") },
                      { value: "168", label: t("waf.zoneTabs.rules.expiry7d") },
                    ]}
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("waf.zoneTabs.rules.expiryHint")}
                  </p>
                </div>
              </div>
            </details>
          </div>

          <DialogFooter className="mt-0 px-lg py-md">
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("waf.common.cancel")}
            </Button>
            <Button onClick={save} disabled={busy}>
              {t("waf.common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {proposal.dialog}
    </div>
  );
}
