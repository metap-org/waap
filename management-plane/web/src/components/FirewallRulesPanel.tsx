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
 *   (`zoneId` omitted). See `../../../control-plane/waf-config-distributor/src/dataplane.rs`'s
 *   `tenant_wide_rules_for` for how a `zoneId: null` row gets merged into every zone at compile
 *   time.
 *
 * The global fetch can't ask the server to filter `zoneId IS NULL` through this hook:
 * `@metap/platform-ui`'s `useGraphQLRecords` strips any filter value that is `""`/`undefined`
 * before building the GraphQL variables, so the backend's own `?zoneId=` (empty string) → `IS
 * NULL` convention (`metap-query`'s `query_planner.rs`) is unreachable through it. Fetching all
 * rows unfiltered and filtering client-side for `!rule.data.zoneId` is the workaround — changing
 * the shared hook is out of scope here (it would affect every other consumer app).
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
  SectionCard,
  Select,
  TagsInput,
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
import { ApiErrorMessage, useAsyncAction } from "@metap/platform-ui";
import { StatusBadge } from "./primitives";
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
  const { t } = useTranslation();
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
  const { busy, run } = useAsyncAction();

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
    loadCondition(EMPTY.matchCondition);
    setOpen(true);
  }

  function startEdit(rule: WafRecord<RuleData>) {
    setEditing(rule);
    setDraft(rule.data);
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
      const payload = zoneId
        ? { ...draft, matchCondition, zoneId }
        : { ...draft, matchCondition };
      if (editing) {
        await updateRecord(
          ENTITIES.firewallRules,
          editing.id,
          editing.version,
          payload,
        );
      } else {
        await createRecord(ENTITIES.firewallRules, payload);
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
      await deleteRecord(ENTITIES.firewallRules, rule.id, rule.version);
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
      await updateRecord(ENTITIES.firewallRules, rule.id, rule.version, {
        priority: neighbour.data.priority,
      });
      await updateRecord(
        ENTITIES.firewallRules,
        neighbour.id,
        neighbour.version,
        { priority: rule.data.priority },
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
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => startEdit(rule)}
                    >
                      {t("waf.common.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => remove(rule)}
                      disabled={busy}
                    >
                      {t("waf.common.delete")}
                    </Button>
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
            <DialogTitle>
              {editing
                ? t("waf.zoneTabs.rules.editRule")
                : t("waf.zoneTabs.rules.newRule")}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div>
              <Label htmlFor="rule-name">{t("waf.zoneTabs.rules.name")}</Label>
              <Input
                id="rule-name"
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="rule-type">
                  {t("waf.zoneTabs.rules.type")}
                </Label>
                <Select
                  id="rule-type"
                  value={draft.ruleType}
                  onValueChange={(value) =>
                    setDraft({ ...draft, ruleType: String(value) })
                  }
                  // `ipFirewall`/`geoFirewall` removed (Increment 2) — IP/CIDR whitelist and
                  // blacklist now live on their own `IpAccessListPanel` screen; a geo (country)
                  // rule is still expressible here as an ordinary `waf` rule with a
                  // `matchCondition` field of `country`.
                  options={[
                    { value: "waf", label: t("waf.zoneTabs.rules.typeWaf") },
                    {
                      value: "rateLimit",
                      label: t("waf.zoneTabs.rules.typeRateLimit"),
                    },
                  ]}
                />
              </div>
              <div>
                <Label htmlFor="rule-action">
                  {t("waf.zoneTabs.rules.action")}
                </Label>
                <Select
                  id="rule-action"
                  value={draft.action}
                  onValueChange={(value) =>
                    setDraft({ ...draft, action: String(value) })
                  }
                  options={[
                    {
                      value: "allow",
                      label: t("waf.zoneTabs.rules.actionAllow"),
                    },
                    { value: "log", label: t("waf.zoneTabs.rules.actionLog") },
                    {
                      value: "challenge",
                      label: t("waf.zoneTabs.rules.actionChallenge"),
                    },
                    {
                      value: "block",
                      label: t("waf.zoneTabs.rules.actionBlock"),
                    },
                  ]}
                />
              </div>
            </div>
            {draft.ruleType === "rateLimit" ? (
              <div className="grid gap-3 sm:grid-cols-2">
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
            <div>
              <div className="flex items-center justify-between">
                <Label>{t("waf.zoneTabs.rules.matchCondition")}</Label>
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
              </div>
              <p className="mb-2 text-xs text-muted-foreground">
                {t("waf.zoneTabs.rules.matchConditionHint")}
              </p>

              {conditionMode === "builder" ? (
                <div className="grid gap-2">
                  {builder.predicates.length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      {t("waf.zoneTabs.rules.noPredicatesYet")}
                    </p>
                  ) : null}
                  {builder.predicates.length > 1 ? (
                    <Select
                      value={builder.combinator}
                      onValueChange={(value) =>
                        setBuilder((current) => ({
                          ...current,
                          combinator: value as BuilderCombinator,
                        }))
                      }
                      options={[
                        {
                          value: "all",
                          label: t("waf.zoneTabs.rules.combinatorAll"),
                        },
                        {
                          value: "any",
                          label: t("waf.zoneTabs.rules.combinatorAny"),
                        },
                      ]}
                    />
                  ) : null}
                  {builder.predicates.map((predicate, index) => (
                    <div
                      key={index}
                      className="grid gap-2 rounded-md border border-border p-2 sm:grid-cols-[1fr_1fr_auto]"
                    >
                      <Select
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
                        value={predicate.op}
                        onValueChange={(value) =>
                          updatePredicate(index, { op: value as MatchOp })
                        }
                        options={OPS.map((op) => ({
                          value: op,
                          label: t(`waf.zoneTabs.rules.${OP_LABEL_KEYS[op]}`),
                        }))}
                      />
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => removePredicate(index)}
                      >
                        {t("waf.zoneTabs.rules.removePredicate")}
                      </Button>

                      {predicate.field === "header" ? (
                        <Input
                          className="sm:col-span-3"
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

                      {predicate.op === "in" || predicate.op === "notIn" ? (
                        <div className="sm:col-span-3">
                          <TagsInput
                            value={predicate.values}
                            onChange={(values) =>
                              updatePredicate(index, { values })
                            }
                            placeholder={t("waf.zoneTabs.rules.valuesLabel")}
                          />
                        </div>
                      ) : (
                        <Input
                          className="sm:col-span-3"
                          aria-label={t("waf.zoneTabs.rules.valueLabel")}
                          placeholder={
                            predicate.field === "sourceIpCidr"
                              ? t("waf.zoneTabs.rules.valuePlaceholderCidr")
                              : predicate.op === "regex"
                                ? t("waf.zoneTabs.rules.valuePlaceholderRegex")
                                : undefined
                          }
                          value={predicate.value}
                          onChange={(e) =>
                            updatePredicate(index, { value: e.target.value })
                          }
                        />
                      )}
                    </div>
                  ))}
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={addPredicate}
                  >
                    {t("waf.zoneTabs.rules.addPredicate")}
                  </Button>
                </div>
              ) : (
                <>
                  {autoFellBackToAdvanced ? (
                    <p className="mb-2 text-xs text-destructive">
                      {t("waf.zoneTabs.rules.advancedFallbackNotice")}
                    </p>
                  ) : null}
                  <Textarea
                    id="rule-condition"
                    rows={5}
                    className="font-mono text-xs"
                    value={conditionText}
                    onChange={(e) => setConditionText(e.target.value)}
                  />
                </>
              )}
            </div>
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
