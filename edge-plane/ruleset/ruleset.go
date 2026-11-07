// Package ruleset is the rule-set contract, as the edge sees it.
//
// This is a deliberate duplicate of control-plane/go/ruleset, not an oversight. Sharing it as a
// module would make edge-plane and control-plane one compilation unit and therefore one deploy
// cycle, which is the exact coupling the three-plane split exists to prevent. The contract is
// JSON on a Redis key — a wire format, like an HTTP API — and both sides owning their own parser
// for it is the normal shape for that, not duplication of logic.
//
// SchemaVersion is what keeps the two copies honest: a rule-set written by a newer control-plane
// than this binary understands is rejected wholesale and the previous snapshot is kept, so a
// contract change can never be half-applied at the edge.
//
// Note what is ABSENT here compared to the portal's model: no entity names, no workflow state
// machine, no tenant routing, no metap types at all. The edge knows hostnames, rules and actions.
// That is the whole vocabulary.
package ruleset

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the highest rule-set schema this binary can read.
const SchemaVersion uint32 = 3

const ZoneIndexKey = "waf:zones"
const EpochKey = "waf:ruleset-epoch"

func ZoneKey(hostname string) string        { return fmt.Sprintf("waf:zone:%s", hostname) }
func ZoneVersionKey(hostname string) string { return fmt.Sprintf("waf:zone-version:%s", hostname) }

type Action string

const (
	ActionAllow     Action = "allow"
	ActionLog       Action = "log"
	ActionChallenge Action = "challenge"
	ActionBlock     Action = "block"
)

// EventName is what the portal calls this in waf.security_events.action. Allow never produces
// an event, so it has no name here beyond the fallback.
func (a Action) EventName() string {
	switch a {
	case ActionBlock:
		return "blocked"
	case ActionChallenge:
		return "challenged"
	default: // ActionAllow, ActionLog
		return "logged"
	}
}

func (a *Action) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Action(s) {
	case ActionAllow, ActionLog, ActionChallenge, ActionBlock:
		*a = Action(s)
		return nil
	default:
		return fmt.Errorf("ruleset: unknown action %q", s)
	}
}

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

func (f *Field) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Field(s) {
	case FieldUriPath, FieldUriQuery, FieldMethod, FieldHeader, FieldSourceIp, FieldSourceIpCidr, FieldCountry, FieldUserAgent:
		*f = Field(s)
		return nil
	default:
		return fmt.Errorf("ruleset: unknown field %q", s)
	}
}

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
	// OpRegex: value is a regex pattern (validated by the control-plane at compile time — an
	// invalid pattern never reaches the edge). The edge compiles patterns lazily, cached by
	// pattern text (regexcache), never on the per-request hot path itself.
	OpRegex Op = "regex"
)

func (o *Op) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Op(s) {
	case OpEq, OpNotEq, OpContains, OpNotContains, OpStartsWith, OpEndsWith, OpIn, OpNotIn, OpContainsCi, OpRegex:
		*o = Op(s)
		return nil
	default:
		return fmt.Errorf("ruleset: unknown op %q", s)
	}
}

type Predicate struct {
	Field  Field    `json:"field"`
	Op     Op       `json:"op"`
	Value  *string  `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
	// Param is the header name for Field::Header, already lower-cased by the control-plane.
	Param *string `json:"param,omitempty"`
}

type MatchExprKind int

const (
	MatchAlways MatchExprKind = iota
	MatchPredicate
	MatchAll
	MatchAny
	MatchNot
)

type MatchExpr struct {
	Kind      MatchExprKind
	Predicate *Predicate
	Children  []MatchExpr
	Not       *MatchExpr
}

// UnmarshalJSON mirrors serde's default externally-tagged enum representation — the actual wire
// shape the control-plane (Rust or Go) publishes: a unit variant ("always") is a bare JSON
// string, every other variant is a single-key object keyed by its camelCase variant name.
func (m *MatchExpr) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		if asString != "always" {
			return fmt.Errorf("ruleset: unknown MatchExpr variant %q", asString)
		}
		*m = MatchExpr{Kind: MatchAlways}
		return nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("ruleset: MatchExpr is neither a string nor an object: %w", err)
	}
	if len(object) != 1 {
		return fmt.Errorf("ruleset: MatchExpr object must have exactly one key, got %d", len(object))
	}

	if raw, ok := object["predicate"]; ok {
		var p Predicate
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		*m = MatchExpr{Kind: MatchPredicate, Predicate: &p}
		return nil
	}
	if raw, ok := object["all"]; ok {
		var children []MatchExpr
		if err := json.Unmarshal(raw, &children); err != nil {
			return err
		}
		*m = MatchExpr{Kind: MatchAll, Children: children}
		return nil
	}
	if raw, ok := object["any"]; ok {
		var children []MatchExpr
		if err := json.Unmarshal(raw, &children); err != nil {
			return err
		}
		*m = MatchExpr{Kind: MatchAny, Children: children}
		return nil
	}
	if raw, ok := object["not"]; ok {
		var inner MatchExpr
		if err := json.Unmarshal(raw, &inner); err != nil {
			return err
		}
		*m = MatchExpr{Kind: MatchNot, Not: &inner}
		return nil
	}
	return fmt.Errorf("ruleset: unrecognized MatchExpr key")
}

type RateLimit struct {
	Threshold     uint32 `json:"threshold"`
	WindowSeconds uint32 `json:"windowSeconds"`
}

type CompiledRule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// RuleType/Priority are deserialized but never read by package evaluate: priority ordering
	// already happened at the control-plane (rules arrives pre-sorted), and RuleType is
	// informational, not behavioral. Kept on the struct rather than dropped from the wire
	// contract so a log line still shows them for debugging.
	RuleType  string     `json:"ruleType"`
	Action    Action     `json:"action"`
	Priority  int64      `json:"priority"`
	MatchExpr MatchExpr  `json:"matchExpr"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

// CompiledDdos is one DDoS policy, scoped. PathPrefix/HttpMethod both nil (or HttpMethod "any")
// means "applies to the whole zone".
type CompiledDdos struct {
	ID                   string  `json:"id"`
	Sensitivity          string  `json:"sensitivity"`
	Action               Action  `json:"action"`
	RequestRateThreshold uint32  `json:"requestRateThreshold"`
	BurstWindowSeconds   uint32  `json:"burstWindowSeconds"`
	PathPrefix           *string `json:"pathPrefix,omitempty"`
	HttpMethod           *string `json:"httpMethod,omitempty"`
	// Priority: already priority-sorted by the control-plane, same as CompiledRule above —
	// package evaluate walks CompiledZone.Ddos in order, it never sorts.
	Priority int64 `json:"priority"`
}

type CompiledZone struct {
	SchemaVersion uint32 `json:"schemaVersion"`
	ZoneID        string `json:"zoneId"`
	// TenantID is not read by any request-path logic (the edge never scopes anything by tenant —
	// that's a data-plane concept it deliberately doesn't know) or by cache/refresh logic. Kept
	// for wire-contract fidelity and debug logging.
	TenantID       string         `json:"tenantId"`
	Hostname       string         `json:"hostname"`
	OriginAddress  string         `json:"originAddress"`
	Status         string         `json:"status"`
	ProtectionMode string         `json:"protectionMode"`
	ConfigVersion  int64          `json:"configVersion"`
	Ddos           []CompiledDdos `json:"ddos"`
	Rules          []CompiledRule `json:"rules"`
	// CompiledAt: when the control-plane compiled this zone. Not read today; kept for the same
	// reason as TenantID above.
	CompiledAt string `json:"compiledAt"`
}

// Enforcing: monitor mode means "evaluate and report, change nothing". Handled here rather than
// by the control-plane rewriting every action to Log, so telemetry can still report what WOULD
// have happened — which is the entire value of monitor mode to a customer deciding whether to
// enforce.
func (z CompiledZone) Enforcing() bool {
	return z.ProtectionMode == "enforce" && z.Status == "active"
}
