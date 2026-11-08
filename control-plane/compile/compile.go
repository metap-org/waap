// Package compile translates management-plane records into ruleset.CompiledZone. The "compile" half
// of this worker's name.
//
// Everything decided here is something the edge then never has to decide per request:
//   - a zone that must not be served at all is filtered out (Publishable);
//   - disabled rules and disabled DDoS policies are dropped, not carried with a flag;
//   - rules are sorted by priority once, so "first match wins" is a plain iteration at the edge;
//   - the portal's authoring shape for matchCondition is translated into the fixed compiled
//     grammar (ruleset.MatchExpr).
//
// That last one is where the still-open authoring-grammar question is absorbed. ParseMatch
// accepts what the portal writes today and is the single function to change when that question
// is answered — the edge contract does not move.
package compile

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/metap-org/waap/control-plane/management"
	"github.com/metap-org/waap/control-plane/ruleset"
)

// Publishable reports whether this zone should be served at the edge at all.
//
// Only "active" and "paused" are published. "pending" has never been verified, and "suspended"
// is terminal — publishing either would put a hostname into the edge's routing table that the
// product says is not protected. "paused" IS published because pausing means "stop enforcing",
// not "stop serving": the edge still needs the origin address to pass traffic through, and
// protectionMode handling is what makes it inert.
func Publishable(zone management.Record) bool {
	status, ok := zone.Str("status")
	if !ok && zone.Status != nil {
		status = *zone.Status
		ok = true
	}
	if !ok {
		return false
	}
	return status == "active" || status == "paused"
}

func actionFrom(raw string) ruleset.Action {
	switch raw {
	case "block":
		return ruleset.ActionBlock
	case "challenge":
		return ruleset.ActionChallenge
	case "allow":
		return ruleset.ActionAllow
	default:
		// "log" and anything unrecognised both land here. Defaulting an unknown action to Log
		// rather than Block is deliberate: a control-plane that doesn't understand a newer
		// action value must not start blocking traffic on a guess.
		return ruleset.ActionLog
	}
}

func fieldFrom(raw string) (ruleset.Field, bool) {
	switch raw {
	case "uri.path", "uriPath":
		return ruleset.FieldUriPath, true
	case "uri.query", "uriQuery":
		return ruleset.FieldUriQuery, true
	case "method", "http.method":
		return ruleset.FieldMethod, true
	case "header", "http.header":
		return ruleset.FieldHeader, true
	case "ip", "source.ip", "sourceIp":
		return ruleset.FieldSourceIp, true
	case "ip.cidr", "sourceIpCidr":
		return ruleset.FieldSourceIpCidr, true
	case "country", "geo.country":
		return ruleset.FieldCountry, true
	case "userAgent", "http.userAgent":
		return ruleset.FieldUserAgent, true
	default:
		return "", false
	}
}

func opFrom(raw string) (ruleset.Op, bool) {
	switch raw {
	case "eq", "equals":
		return ruleset.OpEq, true
	case "ne", "neq", "notEquals":
		return ruleset.OpNotEq, true
	case "contains":
		return ruleset.OpContains, true
	case "notContains":
		return ruleset.OpNotContains, true
	case "containsCi", "icontains":
		return ruleset.OpContainsCi, true
	case "startsWith":
		return ruleset.OpStartsWith, true
	case "endsWith":
		return ruleset.OpEndsWith, true
	case "in":
		return ruleset.OpIn, true
	case "notIn":
		return ruleset.OpNotIn, true
	case "regex":
		return ruleset.OpRegex, true
	default:
		return "", false
	}
}

// ParseMatch translates one authored matchCondition into the compiled grammar.
//
// Returns (expr, false) for anything it cannot represent — and the caller DROPS that rule rather
// than publishing it with a weakened condition. A firewall rule that matches more than its
// author wrote is worse than a firewall rule that is missing: the missing one shows up as "my
// rule isn't working", the broadened one silently blocks real traffic.
func ParseMatch(raw json.RawMessage) (ruleset.MatchExpr, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		// No condition at all is meaningful for IP-list and rate-limit rules: the rule applies
		// to every request and the rate-limit budget (or the value list) is the real condition.
		return ruleset.Always(), true
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return ruleset.MatchExpr{}, false
	}

	if allRaw, ok := object["all"]; ok {
		var items []json.RawMessage
		if json.Unmarshal(allRaw, &items) != nil {
			return ruleset.MatchExpr{}, false
		}
		parsed := make([]ruleset.MatchExpr, 0, len(items))
		for _, item := range items {
			expr, ok := ParseMatch(item)
			if !ok {
				return ruleset.MatchExpr{}, false
			}
			parsed = append(parsed, expr)
		}
		return ruleset.All(parsed), true
	}
	if anyRaw, ok := object["any"]; ok {
		var items []json.RawMessage
		if json.Unmarshal(anyRaw, &items) != nil {
			return ruleset.MatchExpr{}, false
		}
		parsed := make([]ruleset.MatchExpr, 0, len(items))
		for _, item := range items {
			expr, ok := ParseMatch(item)
			if !ok {
				return ruleset.MatchExpr{}, false
			}
			parsed = append(parsed, expr)
		}
		return ruleset.Any(parsed), true
	}
	if notRaw, ok := object["not"]; ok {
		inner, ok := ParseMatch(notRaw)
		if !ok {
			return ruleset.MatchExpr{}, false
		}
		return ruleset.Not(inner), true
	}

	fieldRaw, ok := object["field"]
	if !ok {
		return ruleset.MatchExpr{}, false
	}
	var fieldStr string
	if json.Unmarshal(fieldRaw, &fieldStr) != nil {
		return ruleset.MatchExpr{}, false
	}
	field, ok := fieldFrom(fieldStr)
	if !ok {
		return ruleset.MatchExpr{}, false
	}

	opRaw, ok := object["op"]
	if !ok {
		return ruleset.MatchExpr{}, false
	}
	var opStr string
	if json.Unmarshal(opRaw, &opStr) != nil {
		return ruleset.MatchExpr{}, false
	}
	op, ok := opFrom(opStr)
	if !ok {
		return ruleset.MatchExpr{}, false
	}

	var values []string
	if valuesRaw, ok := object["values"]; ok {
		var rawValues []json.RawMessage
		if json.Unmarshal(valuesRaw, &rawValues) == nil {
			for _, v := range rawValues {
				var s string
				if json.Unmarshal(v, &s) == nil {
					values = append(values, s)
				}
			}
		}
	}

	var value *string
	if valueRaw, ok := object["value"]; ok {
		// A number or boolean in a request-attribute comparison is still text at the edge (a
		// header value, a path segment), so normalise here rather than making the edge handle
		// three JSON types.
		var s string
		var n json.Number
		var b bool
		switch {
		case json.Unmarshal(valueRaw, &s) == nil:
			value = &s
		case json.Unmarshal(valueRaw, &n) == nil:
			str := n.String()
			value = &str
		case json.Unmarshal(valueRaw, &b) == nil:
			str := fmt.Sprintf("%v", b)
			value = &str
		}
	}

	if op == ruleset.OpIn || op == ruleset.OpNotIn {
		if len(values) == 0 {
			return ruleset.MatchExpr{}, false
		}
	} else if value == nil {
		return ruleset.MatchExpr{}, false
	}

	var param *string
	paramRaw, hasParam := object["param"]
	if !hasParam {
		paramRaw, hasParam = object["header"]
	}
	if hasParam {
		var s string
		if json.Unmarshal(paramRaw, &s) == nil {
			// Lower-cased here so the edge can compare against hyper's already-lowercased header
			// names without allocating per request.
			lower := strings.ToLower(s)
			param = &lower
		}
	}
	if field == ruleset.FieldHeader && param == nil {
		return ruleset.MatchExpr{}, false
	}

	// Op::Regex is validated here, not just at authoring time — a pattern that fails to compile
	// must never reach the edge (same "drop rather than publish broken" rule as every other
	// unrepresentable condition in this function). zones-service's own save-time validation
	// should already have caught this, but that is a best-effort UX nicety at a different layer,
	// not something this function may rely on.
	if op == ruleset.OpRegex {
		if value == nil {
			return ruleset.MatchExpr{}, false
		}
		if _, err := regexp.Compile(*value); err != nil {
			return ruleset.MatchExpr{}, false
		}
	}

	return ruleset.PredicateExpr(ruleset.Predicate{
		Field:  field,
		Op:     op,
		Value:  value,
		Values: values,
		Param:  param,
	}), true
}

func compileRule(rule management.Record) (ruleset.CompiledRule, bool) {
	if enabled, ok := rule.Bool("enabled"); ok && !enabled {
		return ruleset.CompiledRule{}, false
	}
	ruleType, ok := rule.Str("ruleType")
	if !ok {
		ruleType = "waf"
	}
	matchExpr, ok := ParseMatch(rule.Data["matchCondition"])
	if !ok {
		return ruleset.CompiledRule{}, false
	}
	var rateLimit *ruleset.RateLimit
	if ruleType == "rateLimit" {
		threshold, ok := rule.Int64("rateLimitThreshold")
		if !ok || threshold < 1 {
			threshold = 100
		}
		window, ok := rule.Int64("rateLimitWindow")
		if !ok || window < 1 {
			window = 60
		}
		rateLimit = &ruleset.RateLimit{Threshold: uint32(threshold), WindowSeconds: uint32(window)}
	}
	name, ok := rule.Str("name")
	if !ok {
		name = "unnamed"
	}
	action, _ := rule.Str("action")
	priority, ok := rule.Int64("priority")
	if !ok {
		priority = 1000
	}
	return ruleset.CompiledRule{
		ID:        rule.ID,
		Name:      name,
		RuleType:  ruleType,
		Action:    actionFrom(action),
		Priority:  priority,
		MatchExpr: matchExpr,
		RateLimit: rateLimit,
	}, true
}

// compileIPAccessList translates one IpAccessList row into the same CompiledRule shape
// compileRule produces for a FirewallRule — no new wire type.
//
// Always compiles to FieldSourceIpCidr + OpEq, whether value is a bare IP or a real CIDR — the
// edge's own evaluator already handles both for that field (a bare address via exact-match
// fallback, a real CIDR via prefix containment).
//
// ruleType "ipAccessList" is informational only (telemetry/display) — compileZone below is what
// actually gives these rows evaluate-before-everything-else priority, by where it places them
// in the compiled list, not anything the edge's evaluator reads off this field.
func compileIPAccessList(entry management.Record) (ruleset.CompiledRule, bool) {
	if enabled, ok := entry.Bool("enabled"); ok && !enabled {
		return ruleset.CompiledRule{}, false
	}
	listType, ok := entry.Str("type")
	if !ok {
		return ruleset.CompiledRule{}, false
	}
	value, ok := entry.Str("value")
	if !ok {
		return ruleset.CompiledRule{}, false
	}
	action := ruleset.ActionBlock
	if listType == "whitelist" {
		action = ruleset.ActionAllow
	}
	v := value
	return ruleset.CompiledRule{
		ID:       entry.ID,
		Name:     fmt.Sprintf("%s %s", listType, value),
		RuleType: "ipAccessList",
		Action:   action,
		// Not a real ordering signal — see this function's own doc comment and compileZone's
		// sort below for where the actual allow-before-block order comes from.
		Priority:  0,
		MatchExpr: ruleset.PredicateExpr(ruleset.Predicate{Field: ruleset.FieldSourceIpCidr, Op: ruleset.OpEq, Value: &v}),
	}, true
}

func compileDdos(policy management.Record) (ruleset.CompiledDdos, bool) {
	if enabled, ok := policy.Bool("enabled"); ok && !enabled {
		return ruleset.CompiledDdos{}, false
	}
	var pathPrefix *string
	if p, ok := policy.Str("pathPrefix"); ok && p != "" {
		pathPrefix = &p
	}
	var httpMethod *string
	if m, ok := policy.Str("httpMethod"); ok && m != "" {
		httpMethod = &m
	}
	sensitivity, ok := policy.Str("sensitivity")
	if !ok {
		sensitivity = "medium"
	}
	action, _ := policy.Str("action")
	threshold, ok := policy.Int64("requestRateThreshold")
	if !ok || threshold < 1 {
		threshold = 500
	}
	burstWindow, ok := policy.Int64("burstWindow")
	if !ok || burstWindow < 1 {
		burstWindow = 60
	}
	priority, ok := policy.Int64("priority")
	if !ok {
		priority = 1000
	}
	return ruleset.CompiledDdos{
		ID:                   policy.ID,
		Sensitivity:          sensitivity,
		Action:               actionFrom(action),
		RequestRateThreshold: uint32(threshold),
		BurstWindowSeconds:   uint32(burstWindow),
		PathPrefix:           pathPrefix,
		HttpMethod:           httpMethod,
		Priority:             priority,
	}, true
}

// compileDdosPolicies returns every enabled policy, priority-sorted ascending — the edge's
// evaluator walks this in order, first scope-matching-and-over-budget policy wins.
func compileDdosPolicies(policies []management.Record) []ruleset.CompiledDdos {
	compiled := make([]ruleset.CompiledDdos, 0, len(policies))
	for _, p := range policies {
		if c, ok := compileDdos(p); ok {
			compiled = append(compiled, c)
		}
	}
	sort.SliceStable(compiled, func(i, j int) bool { return compiled[i].Priority < compiled[j].Priority })
	return compiled
}

func CompileZone(zone management.Record, tenantID string, ddosPolicies, rules, ipAccessLists []management.Record) (ruleset.CompiledZone, bool) {
	hostname, ok := zone.Str("hostname")
	if !ok {
		return ruleset.CompiledZone{}, false
	}

	// IpAccessList entries evaluate before every regular FirewallRule — allow first (so a
	// whitelist match bypasses everything, including DdosPolicy), then block — regardless of any
	// rule's own priority.
	// Allow-action entries first, block-action entries after, input order preserved within each
	// (a stable partition, not a general sort — there is no ordering signal within either
	// group).
	var accessRules, blocked []ruleset.CompiledRule
	for _, entry := range ipAccessLists {
		c, ok := compileIPAccessList(entry)
		if !ok {
			continue
		}
		if c.Action == ruleset.ActionAllow {
			accessRules = append(accessRules, c)
		} else {
			blocked = append(blocked, c)
		}
	}
	accessRules = append(accessRules, blocked...)

	regularRules := make([]ruleset.CompiledRule, 0, len(rules))
	for _, r := range rules {
		if c, ok := compileRule(r); ok {
			regularRules = append(regularRules, c)
		}
	}
	sort.SliceStable(regularRules, func(i, j int) bool { return regularRules[i].Priority < regularRules[j].Priority })

	dropped := (len(rules) - len(regularRules)) + (len(ipAccessLists) - len(accessRules))
	if dropped > 0 {
		// Loud on purpose: a rule silently disappearing between the portal and the edge is
		// exactly the failure mode nobody notices until an attack gets through.
		slog.Warn("some firewall rules or IP access list entries were not compiled (disabled, or an unrepresentable value)",
			"hostname", hostname, "dropped", dropped)
	}

	compiled := append(accessRules, regularRules...)

	status, ok := zone.Str("status")
	if !ok {
		status = "pending"
		if zone.Status != nil {
			status = *zone.Status
		}
	}
	originAddress, _ := zone.Str("originAddress")
	protectionMode, ok := zone.Str("protectionMode")
	if !ok {
		protectionMode = "monitor"
	}
	configVersion, _ := zone.Int64("configVersion")

	return ruleset.CompiledZone{
		SchemaVersion:  ruleset.RulesetSchemaVersion,
		ZoneID:         zone.ID,
		TenantID:       tenantID,
		Hostname:       hostname,
		OriginAddress:  originAddress,
		Status:         status,
		ProtectionMode: protectionMode,
		ConfigVersion:  configVersion,
		Ddos:           compileDdosPolicies(ddosPolicies),
		Rules:          compiled,
		CompiledAt:     time.Now().UTC().Format(time.RFC3339),
	}, true
}
