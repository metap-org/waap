/**
 * The rule-builder's authoring grammar, matching exactly what
 * `../../../control-plane/waf-config-distributor/src/compile.rs`'s `parse_match` accepts (and
 * `zones-service`'s `firewall_rule_match_condition_guard` validates at save time before that) —
 * flat `{field, op, value|values, param}` objects combined with `all`/`any`. `not` and further
 * nesting are real, valid `matchCondition` shapes this grammar supports, but the builder UI
 * (`../components/FirewallRulesPanel.tsx`) only ever *produces* one flat combinator level;
 * anything it can't losslessly round-trip falls back to the raw JSON editor rather than silently
 * reshaping a rule someone wrote by hand.
 *
 * Pure logic, no React — shared by `FirewallRulesPanel.tsx` (used from both the per-zone
 * `ZoneRulesTab` and the tenant-wide `GlobalRulesPage`) and tested independently of any component.
 */

export const FIELDS = [
  "uri.path",
  "uri.query",
  "method",
  "header",
  "sourceIp",
  "sourceIpCidr",
  "country",
  "userAgent",
] as const;
export type MatchField = (typeof FIELDS)[number];

export const OPS = [
  "eq",
  "ne",
  "contains",
  "notContains",
  "containsCi",
  "startsWith",
  "endsWith",
  "in",
  "notIn",
  "regex",
] as const;
export type MatchOp = (typeof OPS)[number];

export type Predicate = {
  field: MatchField;
  op: MatchOp;
  value: string;
  values: string[];
  param: string;
};

export type BuilderCombinator = "all" | "any";

export type BuilderState = {
  combinator: BuilderCombinator;
  predicates: Predicate[];
};

export function emptyPredicate(): Predicate {
  return {
    field: "uri.path",
    op: "contains",
    value: "",
    values: [],
    param: "",
  };
}

export function predicateToJson(p: Predicate): Record<string, unknown> {
  const json: Record<string, unknown> = { field: p.field, op: p.op };
  if (p.op === "in" || p.op === "notIn") {
    json.values = p.values;
  } else {
    json.value = p.value;
  }
  if (p.field === "header") {
    json.param = p.param;
  }
  return json;
}

/** `null`/no predicates compiles to `null` (`compile.rs`'s `parse_match` treats that as
 *  `MatchExpr::Always` — "matches every request", the correct meaning for a bare IP-list or
 *  rate-limit-only rule). A single predicate skips the combinator wrapper entirely — the same
 *  shape a hand-written single-condition rule already used before this builder existed. */
export function builderToJson(builder: BuilderState): unknown {
  const [first, ...rest] = builder.predicates;
  if (!first) return null;
  if (rest.length === 0) return predicateToJson(first);
  return { [builder.combinator]: builder.predicates.map(predicateToJson) };
}

function isMatchField(value: unknown): value is MatchField {
  return (
    typeof value === "string" && (FIELDS as readonly string[]).includes(value)
  );
}

function isMatchOp(value: unknown): value is MatchOp {
  return (
    typeof value === "string" && (OPS as readonly string[]).includes(value)
  );
}

/** The inverse of `predicateToJson`, for one flat predicate object — `null` if `raw` isn't one
 *  (unknown field/op, wrong shape), which is what tells the caller to fall back to Advanced mode
 *  rather than misrepresent it. */
function jsonToPredicate(raw: unknown): Predicate | null {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw))
    return null;
  const obj = raw as Record<string, unknown>;
  if (!isMatchField(obj.field) || !isMatchOp(obj.op)) return null;
  const values = Array.isArray(obj.values)
    ? obj.values.filter((v) => typeof v === "string")
    : [];
  const value =
    typeof obj.value === "string" ? obj.value : String(obj.value ?? "");
  const param =
    typeof obj.param === "string"
      ? obj.param
      : typeof obj.header === "string"
        ? obj.header
        : "";
  return { field: obj.field, op: obj.op, value, values, param };
}

/** `undefined`/`null` -> an empty builder (matches every request). A single flat predicate -> one
 *  row, combinator defaults to `all` (irrelevant with one row). `{all: [...]}`/`{any: [...]}`
 *  where every child is itself a flat predicate -> that many rows under that combinator. Anything
 *  else (a `not`, a nested `all`/`any` inside another, an unrecognized field/op) returns `null` —
 *  the caller must fall back to the raw JSON editor rather than build a lossy approximation. */
export function jsonToBuilder(raw: unknown): BuilderState | null {
  if (raw === null || raw === undefined)
    return { combinator: "all", predicates: [] };
  if (typeof raw !== "object" || Array.isArray(raw)) return null;
  const obj = raw as Record<string, unknown>;
  const combinatorKey: BuilderCombinator | null = Array.isArray(obj.all)
    ? "all"
    : Array.isArray(obj.any)
      ? "any"
      : null;
  if (combinatorKey) {
    const items = obj[combinatorKey] as unknown[];
    const predicates = items.map(jsonToPredicate);
    if (predicates.some((p) => p === null) || predicates.length === 0)
      return null;
    return { combinator: combinatorKey, predicates: predicates as Predicate[] };
  }
  const single = jsonToPredicate(obj);
  return single ? { combinator: "all", predicates: [single] } : null;
}

export const FIELD_LABEL_KEYS: Record<MatchField, string> = {
  "uri.path": "fieldUriPath",
  "uri.query": "fieldUriQuery",
  method: "fieldMethod",
  header: "fieldHeader",
  sourceIp: "fieldSourceIp",
  sourceIpCidr: "fieldSourceIpCidr",
  country: "fieldCountry",
  userAgent: "fieldUserAgent",
};

export const OP_LABEL_KEYS: Record<MatchOp, string> = {
  eq: "opEq",
  ne: "opNe",
  contains: "opContains",
  notContains: "opNotContains",
  containsCi: "opContainsCi",
  startsWith: "opStartsWith",
  endsWith: "opEndsWith",
  in: "opIn",
  notIn: "opNotIn",
  regex: "opRegex",
};
