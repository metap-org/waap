// Package ruleset is the cross-plane wire contract. Everything edge-plane is allowed to know
// about a zone's configuration is in this file — nothing else crosses the boundary.
//
// Two properties this shape is designed for:
//
//  1. edge-plane never learns management-plane's schema. It does not see metap's records table, its
//     JSONB field bag, entity names, or workflow state names. It sees a flat, already-decided
//     rule list. That is the whole point of compiling here rather than letting the edge read
//     config itself.
//  2. Evaluation at the edge is a lookup, not a decision. Anything that can be resolved once,
//     centrally — priority ordering, disabled rules, a paused zone, monitor-mode downgrading —
//     is resolved here so the hot path does the minimum work per request.
//
// SchemaVersion is checked by the edge on load. A rule-set written by a newer control-plane than
// the edge understands is ignored (the edge keeps serving its last good snapshot) rather than
// half-parsed — a config-distribution bug must not become an outage.
package ruleset

import (
	"encoding/json"
	"fmt"
)

// RulesetSchemaVersion is bumped whenever this file's shape changes incompatibly. The edge
// refuses anything higher than the version it was built against.
const RulesetSchemaVersion uint32 = 3

// ZoneKey is the Redis key holding one zone's compiled rule-set, keyed by hostname (what the
// edge has in hand from the Host header — no lookup table needed on the hot path).
func ZoneKey(hostname string) string { return fmt.Sprintf("waf:zone:%s", hostname) }

// ZoneVersionKey is the Redis key holding just this zone's config_version, so the edge's refresh
// ticker can ask "did anything change" for a fraction of the bytes of the full rule-set.
func ZoneVersionKey(hostname string) string { return fmt.Sprintf("waf:zone-version:%s", hostname) }

// ZoneIndexKey is the Redis SET of every hostname currently served. The edge uses it to notice
// zones that have appeared or disappeared; without it a deleted zone would keep being served
// from the edge's in-memory snapshot forever.
const ZoneIndexKey = "waf:zones"

// EpochKey is bumped on every completed full resync. An edge that has been disconnected long
// enough to distrust its incremental state can compare this and reload everything.
const EpochKey = "waf:ruleset-epoch"

// Action is what the edge does with a request. Ordered by severity — Block wins if two rules
// somehow both match, though "first match wins" means that shouldn't arise.
type Action string

const (
	ActionAllow     Action = "allow"
	ActionLog       Action = "log"
	ActionChallenge Action = "challenge"
	ActionBlock     Action = "block"
)

// Field is a request attribute a compiled predicate may test. A closed set, not an open string:
// the edge must never receive a field name it has to interpret at runtime, and an unknown field
// arriving from a newer control-plane is a rule the edge cannot honour — better to fail parsing
// the whole zone (and keep the last good snapshot) than to silently skip a security rule.
type Field string

const (
	FieldUriPath      Field = "uriPath"
	FieldUriQuery     Field = "uriQuery"
	FieldMethod       Field = "method"
	FieldHeader       Field = "header"
	FieldSourceIp     Field = "sourceIp"
	FieldSourceIpCidr Field = "sourceIpCidr"
	FieldCountry      Field = "country"
	FieldUserAgent    Field = "userAgent"
)

type Op string

const (
	OpEq          Op = "eq"
	OpNotEq       Op = "notEq"
	OpContains    Op = "contains"
	OpNotContains Op = "notContains"
	OpStartsWith  Op = "startsWith"
	OpEndsWith    Op = "endsWith"
	OpIn          Op = "in"
	OpNotIn       Op = "notIn"
	OpContainsCi  Op = "containsCi"
	OpRegex       Op = "regex"
)

// Predicate mirrors the Rust Predicate struct.
type Predicate struct {
	Field  Field    `json:"field"`
	Op     Op       `json:"op"`
	Value  *string  `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
	Param  *string  `json:"param,omitempty"`
}

// MatchExpr is a boolean tree over predicates, kept shallow in practice but expressed as a tree
// so a richer authoring grammar doesn't need a new schema version to compile into.
//
// Exactly one of the fields below is set, discriminated by Kind — Go has no tagged union, so
// this struct plays that role directly rather than through an interface, keeping JSON
// (de)serialization trivial and matching the Rust enum's wire shape via MarshalJSON/UnmarshalJSON.
type MatchExprKind string

const (
	MatchAlways    MatchExprKind = "always"
	MatchPredicate MatchExprKind = "predicate"
	MatchAll       MatchExprKind = "all"
	MatchAny       MatchExprKind = "any"
	MatchNot       MatchExprKind = "not"
)

type MatchExpr struct {
	Kind      MatchExprKind
	Predicate *Predicate
	Children  []MatchExpr
	Not       *MatchExpr
}

func Always() MatchExpr                   { return MatchExpr{Kind: MatchAlways} }
func PredicateExpr(p Predicate) MatchExpr { return MatchExpr{Kind: MatchPredicate, Predicate: &p} }
func All(children []MatchExpr) MatchExpr  { return MatchExpr{Kind: MatchAll, Children: children} }
func Any(children []MatchExpr) MatchExpr  { return MatchExpr{Kind: MatchAny, Children: children} }
func Not(inner MatchExpr) MatchExpr       { return MatchExpr{Kind: MatchNot, Not: &inner} }

// MarshalJSON mirrors serde's default externally-tagged enum representation, which is the
// actual wire contract edge-plane (still Rust, unported) reads from Redis: a unit variant
// (Always) serializes as a bare string, every other variant as a single-key object keyed by its
// camelCase variant name. This is NOT the authoring grammar compile.ParseMatch accepts — it is
// the compiled grammar every MatchExpr this package produces gets published in.
func (m MatchExpr) MarshalJSON() ([]byte, error) {
	switch m.Kind {
	case MatchAlways:
		return json.Marshal("always")
	case MatchPredicate:
		return json.Marshal(map[string]*Predicate{"predicate": m.Predicate})
	case MatchAll:
		return json.Marshal(map[string][]MatchExpr{"all": m.Children})
	case MatchAny:
		return json.Marshal(map[string][]MatchExpr{"any": m.Children})
	case MatchNot:
		return json.Marshal(map[string]*MatchExpr{"not": m.Not})
	default:
		return nil, fmt.Errorf("unknown MatchExpr kind %q", m.Kind)
	}
}

type RateLimit struct {
	Threshold     uint32 `json:"threshold"`
	WindowSeconds uint32 `json:"windowSeconds"`
}

// CompiledRule mirrors the Rust CompiledRule struct.
type CompiledRule struct {
	// ID is the FirewallRule record id — travels back up in telemetry as triggeredById so the
	// portal can show which rule fired without the edge knowing anything else about it.
	ID   string `json:"id"`
	Name string `json:"name"`
	// RuleType is informational at the edge (waf/rateLimit/ipFirewall/geoFirewall) — behaviour is
	// fully described by MatchExpr + RateLimit + Action. Carried so telemetry and logs can say
	// what kind of rule fired.
	RuleType  string     `json:"ruleType"`
	Action    Action     `json:"action"`
	Priority  int64      `json:"priority"`
	MatchExpr MatchExpr  `json:"matchExpr"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

// CompiledDdos is one DDoS policy, scoped. PathPrefix/HttpMethod both nil (or HttpMethod
// "any") means "applies to the whole zone".
type CompiledDdos struct {
	ID                   string  `json:"id"`
	Sensitivity          string  `json:"sensitivity"`
	Action               Action  `json:"action"`
	RequestRateThreshold uint32  `json:"requestRateThreshold"`
	BurstWindowSeconds   uint32  `json:"burstWindowSeconds"`
	PathPrefix           *string `json:"pathPrefix,omitempty"`
	HttpMethod           *string `json:"httpMethod,omitempty"`
	Priority             int64   `json:"priority"`
}

// CompiledZone is one zone, fully resolved. Disabled rules and disabled policies are dropped
// during compilation rather than carried with a flag — the edge should never hold a rule it must
// remember not to apply.
type CompiledZone struct {
	SchemaVersion uint32 `json:"schemaVersion"`
	ZoneID        string `json:"zoneId"`
	TenantID      string `json:"tenantId"`
	Hostname      string `json:"hostname"`
	OriginAddress string `json:"originAddress"`
	// Status is "active" or "paused". A "pending"/"suspended" zone is never published at all.
	Status string `json:"status"`
	// ProtectionMode is "enforce" or "monitor". In monitor mode the edge still evaluates and
	// still reports, but downgrades every action to Log.
	ProtectionMode string         `json:"protectionMode"`
	ConfigVersion  int64          `json:"configVersion"`
	Ddos           []CompiledDdos `json:"ddos"`
	Rules          []CompiledRule `json:"rules"`
	CompiledAt     string         `json:"compiledAt"`
}
