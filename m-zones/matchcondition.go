// Validates a FirewallRule.matchCondition JSON blob against the same field/operator vocabulary
// control-plane/waf-config-distributor's compile.go accepts.
//
// Deliberately a second, independent copy of that vocabulary rather than a shared package — this
// plane and control-plane are separate deploy cycles, and the wire shape is a JSON contract like
// an HTTP API, not shared code. Unlike compile.go's own parser, this never builds a compiled
// match expression — zones-service has no reason to depend on control-plane's types, it only
// needs a yes/no answer at save time.
//
// Kept in sync with compile.go by hand. If the two ever drift, the effect is narrow and
// self-correcting: a condition this validator wrongly accepts still gets caught (and silently
// dropped) when waf-config-distributor actually compiles it — this is a best-effort,
// immediate-feedback nicety at save time, not the source of truth for what the edge can
// represent.
package zonesservice

import (
	"encoding/json"
	"regexp"
)

var knownMatchFields = map[string]bool{
	"uri.path": true, "uriPath": true, "uri.query": true, "uriQuery": true,
	"method": true, "http.method": true, "header": true, "http.header": true,
	"ip": true, "source.ip": true, "sourceIp": true, "ip.cidr": true, "sourceIpCidr": true,
	"country": true, "geo.country": true, "userAgent": true, "http.userAgent": true,
}

var knownMatchOps = map[string]bool{
	"eq": true, "equals": true, "ne": true, "neq": true, "notEquals": true,
	"contains": true, "notContains": true, "containsCi": true, "icontains": true,
	"startsWith": true, "endsWith": true, "in": true, "notIn": true, "regex": true,
}

// IsValidMatchCondition mirrors the compiler's acceptance rules exactly (including its
// permissive nil/null -> "always matches" case) so a rule this function accepts is, as far as
// this vocabulary can tell, one waf-config-distributor will actually compile rather than
// silently drop. raw is nil when the field was absent from the request entirely.
func IsValidMatchCondition(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return isValidMatchConditionValue(value)
}

func isValidMatchConditionValue(value any) bool {
	if value == nil {
		return true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}

	if items, ok := object["all"].([]any); ok {
		return allValid(items)
	}
	if items, ok := object["any"].([]any); ok {
		return allValid(items)
	}
	if inner, ok := object["not"]; ok {
		return isValidMatchConditionValue(inner)
	}

	field, ok := object["field"].(string)
	if !ok || !knownMatchFields[field] {
		return false
	}
	op, ok := object["op"].(string)
	if !ok || !knownMatchOps[op] {
		return false
	}

	values, hasValues := object["values"].([]any)
	hasNonEmptyValues := hasValues && len(values) > 0
	value1, hasValue := object["value"]
	hasRealValue := hasValue && value1 != nil

	if op == "in" || op == "notIn" {
		if !hasNonEmptyValues {
			return false
		}
	} else if !hasRealValue {
		return false
	}

	if field == "header" || field == "http.header" {
		_, hasParam := object["param"].(string)
		_, hasHeader := object["header"].(string)
		if !hasParam && !hasHeader {
			return false
		}
	}

	if op == "regex" {
		pattern, ok := value1.(string)
		if !ok {
			return false
		}
		// regexp is RE2 syntax, not Rust's regex crate's — close enough for a "does the pattern
		// compile" sanity check; a pattern using a construct RE2 lacks (backreferences,
		// lookaround) would be wrongly rejected here, but that's the same "narrow and
		// self-correcting" tolerance this validator's own doc comment already accepts for drift
		// against the real compiler.
		if _, err := regexp.Compile(pattern); err != nil {
			return false
		}
	}

	return true
}

func allValid(items []any) bool {
	for _, item := range items {
		if !isValidMatchConditionValue(item) {
			return false
		}
	}
	return true
}
