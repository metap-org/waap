package compile

import (
	"encoding/json"
	"testing"

	"github.com/metap-org/waap/control-plane/management"
	"github.com/metap-org/waap/control-plane/ruleset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func record(id string, status *string, data map[string]any) management.Record {
	fields := make(map[string]json.RawMessage, len(data))
	for k, v := range data {
		b, _ := json.Marshal(v)
		fields[k] = b
	}
	return management.Record{ID: id, Status: status, Data: fields}
}

func strPtr(s string) *string { return &s }

func zoneRecord(t *testing.T, status string, extra map[string]any) management.Record {
	data := map[string]any{
		"hostname":       "shop.example.com",
		"originAddress":  "10.0.0.1",
		"status":         status,
		"protectionMode": "enforce",
		"configVersion":  3,
	}
	for k, v := range extra {
		data[k] = v
	}
	return record("zone-1", strPtr(status), data)
}

func TestPublishableAcceptsActiveAndPausedOnly(t *testing.T) {
	assert.True(t, Publishable(zoneRecord(t, "active", nil)), "expected active to be publishable")
	assert.True(t, Publishable(zoneRecord(t, "paused", nil)), "expected paused to be publishable")
	assert.False(t, Publishable(zoneRecord(t, "pending", nil)), "expected pending to not be publishable")
	assert.False(t, Publishable(zoneRecord(t, "suspended", nil)), "expected suspended to not be publishable")
}

func TestActionFromUnknownValueDefaultsToLogNotBlock(t *testing.T) {
	cases := map[string]ruleset.Action{
		"something-new": ruleset.ActionLog,
		"":              ruleset.ActionLog,
		"block":         ruleset.ActionBlock,
		"challenge":     ruleset.ActionChallenge,
		"allow":         ruleset.ActionAllow,
	}
	for input, want := range cases {
		got := actionFrom(input)
		assert.Equal(t, want, got, "actionFrom(%q) = %q, want %q", input, got, want)
	}
}

func TestParseMatchNoneAndNullBothMeanAlways(t *testing.T) {
	expr, ok := ParseMatch(nil)
	require.True(t, ok, "expected Always for nil, got %+v ok=%v", expr, ok)
	require.Equal(t, ruleset.MatchAlways, expr.Kind, "expected Always for nil, got %+v ok=%v", expr, ok)
	expr, ok = ParseMatch(json.RawMessage("null"))
	require.True(t, ok, "expected Always for null, got %+v ok=%v", expr, ok)
	require.Equal(t, ruleset.MatchAlways, expr.Kind, "expected Always for null, got %+v ok=%v", expr, ok)
}

func TestParseMatchSimplePredicate(t *testing.T) {
	raw := rawJSON(t, map[string]any{"field": "uri.path", "op": "contains", "value": "/admin"})
	expr, ok := ParseMatch(raw)
	require.True(t, ok, "expected a Predicate, got %+v ok=%v", expr, ok)
	require.Equal(t, ruleset.MatchPredicate, expr.Kind, "expected a Predicate, got %+v ok=%v", expr, ok)
	p := expr.Predicate
	assert.False(t, p.Field != ruleset.FieldUriPath || p.Op != ruleset.OpContains || p.Value == nil || *p.Value != "/admin", "unexpected predicate: %+v", p)
}

func TestParseMatchHeaderFieldLowercasesTheHeaderName(t *testing.T) {
	raw := rawJSON(t, map[string]any{"field": "header", "op": "eq", "value": "1", "param": "X-Custom-Header"})
	expr, ok := ParseMatch(raw)
	require.True(t, ok, "expected a match")
	assert.False(t, expr.Predicate.Param == nil || *expr.Predicate.Param != "x-custom-header", "expected lowercased param, got %+v", expr.Predicate.Param)
}

func TestParseMatchHeaderFieldWithoutParamIsUnrepresentable(t *testing.T) {
	raw := rawJSON(t, map[string]any{"field": "header", "op": "eq", "value": "1"})
	_, ok := ParseMatch(raw)
	require.False(t, ok, "expected header field without param to be unrepresentable")
}

func TestParseMatchAllAnyNotCompose(t *testing.T) {
	raw := rawJSON(t, map[string]any{
		"all": []any{
			map[string]any{"field": "method", "op": "eq", "value": "POST"},
			map[string]any{"not": map[string]any{"field": "sourceIp", "op": "eq", "value": "10.0.0.1"}},
		},
	})
	expr, ok := ParseMatch(raw)
	require.True(t, ok, "expected All, got %+v ok=%v", expr, ok)
	require.Equal(t, ruleset.MatchAll, expr.Kind, "expected All, got %+v ok=%v", expr, ok)
	require.Len(t, expr.Children, 2, "expected 2 children, got %d", len(expr.Children))
	assert.Equal(t, ruleset.MatchNot, expr.Children[1].Kind, "expected second child to be Not, got %+v", expr.Children[1])
}

func TestParseMatchInRequiresNonEmptyValues(t *testing.T) {
	empty := rawJSON(t, map[string]any{"field": "sourceIp", "op": "in", "values": []string{}})
	if _, ok := ParseMatch(empty); ok {
		assert.Fail(t, "expected empty values with op=in to be unrepresentable")
	}

	filled := rawJSON(t, map[string]any{"field": "sourceIp", "op": "in", "values": []string{"1.2.3.4", "5.6.7.8"}})
	expr, ok := ParseMatch(filled)
	require.True(t, ok, "expected a match")
	assert.False(t, len(expr.Predicate.Values) != 2 || expr.Predicate.Values[0] != "1.2.3.4" || expr.Predicate.Values[1] != "5.6.7.8", "unexpected values: %+v", expr.Predicate.Values)
}

func TestParseMatchRegexWithAValidPatternCompiles(t *testing.T) {
	raw := rawJSON(t, map[string]any{"field": "uri.path", "op": "regex", "value": `^/admin/\d+$`})
	expr, ok := ParseMatch(raw)
	require.True(t, ok, "expected a Predicate, got %+v ok=%v", expr, ok)
	require.Equal(t, ruleset.MatchPredicate, expr.Kind, "expected a Predicate, got %+v ok=%v", expr, ok)
	assert.False(t, expr.Predicate.Field != ruleset.FieldUriPath || expr.Predicate.Op != ruleset.OpRegex || *expr.Predicate.Value != `^/admin/\d+$`, "unexpected predicate: %+v", expr.Predicate)
}

func TestParseMatchRegexWithAnInvalidPatternIsUnrepresentable(t *testing.T) {
	raw := rawJSON(t, map[string]any{"field": "uri.path", "op": "regex", "value": "[invalid(regex"})
	_, ok := ParseMatch(raw)
	require.False(t, ok, "expected an invalid regex to be unrepresentable")
}

func TestParseMatchUnknownFieldOrOpIsUnrepresentable(t *testing.T) {
	_, ok := ParseMatch(rawJSON(t, map[string]any{"field": "bogus", "op": "eq", "value": "x"}))
	assert.False(t, ok, "expected an unknown field to be unrepresentable")
	_, ok = ParseMatch(rawJSON(t, map[string]any{"field": "method", "op": "bogus", "value": "x"}))
	assert.False(t, ok, "expected an unknown op to be unrepresentable")
}

func TestParseMatchMissingValueForANonInOpIsUnrepresentable(t *testing.T) {
	_, ok := ParseMatch(rawJSON(t, map[string]any{"field": "method", "op": "eq"}))
	assert.False(t, ok, "expected a missing value to be unrepresentable")
}

func TestCompileRuleDropsDisabledRules(t *testing.T) {
	rule := record("rule-1", nil, map[string]any{
		"name": "r", "ruleType": "waf", "action": "block", "priority": 10, "enabled": false,
	})
	_, ok := compileRule(rule)
	assert.False(t, ok, "expected disabled rule to be dropped")
}

func TestCompileRuleWithUnrepresentableConditionIsDropped(t *testing.T) {
	rule := record("rule-1", nil, map[string]any{
		"name": "r", "ruleType": "waf", "action": "block", "priority": 10, "enabled": true,
		"matchCondition": map[string]any{"field": "bogus", "op": "eq", "value": "x"},
	})
	_, ok := compileRule(rule)
	assert.False(t, ok, "expected unrepresentable condition to be dropped")
}

func TestCompileRuleRateLimitTypeCarriesThresholdAndWindow(t *testing.T) {
	rule := record("rule-1", nil, map[string]any{
		"name": "login-rl", "ruleType": "rateLimit", "action": "challenge", "priority": 5,
		"enabled": true, "rateLimitThreshold": 20, "rateLimitWindow": 30,
	})
	compiled, ok := compileRule(rule)
	require.True(t, ok, "expected a compiled rule")
	assert.False(t, compiled.ID != "rule-1" || compiled.Action != ruleset.ActionChallenge, "unexpected compiled rule: %+v", compiled)
	assert.False(t, compiled.RateLimit == nil || compiled.RateLimit.Threshold != 20 || compiled.RateLimit.WindowSeconds != 30, "expected rate limit 20/30, got %+v", compiled.RateLimit)
	assert.Equal(t, ruleset.MatchAlways, compiled.MatchExpr.Kind, "expected Always with no condition given, got %+v", compiled.MatchExpr)
}

func TestCompileRuleNonRateLimitTypeHasNoRateLimit(t *testing.T) {
	rule := record("rule-1", nil, map[string]any{
		"name": "r", "ruleType": "waf", "action": "block", "priority": 10, "enabled": true,
	})
	compiled, ok := compileRule(rule)
	assert.False(t, !ok || compiled.RateLimit != nil, "expected no rate limit, got %+v ok=%v", compiled, ok)
}

func TestCompileDdosDropsWhenDisabled(t *testing.T) {
	policy := record("policy-1", nil, map[string]any{
		"sensitivity": "high", "action": "block", "requestRateThreshold": 500, "burstWindow": 60, "enabled": false,
	})
	_, ok := compileDdos(policy)
	assert.False(t, ok, "expected disabled policy to be dropped")
}

func TestCompileDdosEnabledCarriesFieldsThrough(t *testing.T) {
	policy := record("policy-1", nil, map[string]any{
		"sensitivity": "high", "action": "block", "requestRateThreshold": 500, "burstWindow": 60, "enabled": true,
	})
	compiled, ok := compileDdos(policy)
	require.True(t, ok, "expected a compiled policy")
	assert.False(t, compiled.Sensitivity != "high" || compiled.Action != ruleset.ActionBlock ||
		compiled.RequestRateThreshold != 500 || compiled.BurstWindowSeconds != 60, "unexpected compiled policy: %+v", compiled)
}

func TestCompileZoneSortsRulesByPriorityAndDropsUnrepresentableOnes(t *testing.T) {
	z := zoneRecord(t, "active", nil)
	rules := []management.Record{
		record("r-high-priority-num", nil, map[string]any{"name": "second", "ruleType": "waf", "action": "log", "priority": 200, "enabled": true}),
		record("r-low-priority-num", nil, map[string]any{"name": "first", "ruleType": "waf", "action": "block", "priority": 10, "enabled": true}),
		record("r-disabled", nil, map[string]any{"name": "disabled", "ruleType": "waf", "action": "block", "priority": 1, "enabled": false}),
	}
	compiled, ok := CompileZone(z, "tenant-1", nil, rules, nil)
	require.True(t, ok, "expected a compiled zone")
	require.Len(t, compiled.Rules, 2, "expected the disabled rule to be dropped, got %d rules", len(compiled.Rules))
	assert.False(t, compiled.Rules[0].Name != "first" || compiled.Rules[1].Name != "second", "expected priority ascending order, got %+v", compiled.Rules)
	assert.False(t, compiled.SchemaVersion != ruleset.RulesetSchemaVersion || compiled.TenantID != "tenant-1" || compiled.ZoneID != "zone-1", "unexpected zone metadata: %+v", compiled)
}

func TestCompileZoneWithoutHostnameFails(t *testing.T) {
	z := record("zone-1", strPtr("active"), map[string]any{"status": "active"})
	_, ok := CompileZone(z, "tenant-1", nil, nil, nil)
	assert.False(t, ok, "expected a zone with no hostname to fail to compile")
}

func TestCompileZoneCarriesDdosOnlyWhenPresentAndEnabled(t *testing.T) {
	z := zoneRecord(t, "active", nil)
	ddos := record("ddos-1", nil, map[string]any{
		"sensitivity": "medium", "action": "challenge", "requestRateThreshold": 500, "burstWindow": 60, "enabled": true,
	})
	compiled, ok := CompileZone(z, "tenant-1", []management.Record{ddos}, nil, nil)
	require.True(t, ok, "expected 1 ddos policy, got %+v ok=%v", compiled.Ddos, ok)
	require.Len(t, compiled.Ddos, 1, "expected 1 ddos policy, got %+v ok=%v", compiled.Ddos, ok)

	compiledNoDdos, ok := CompileZone(z, "tenant-1", nil, nil, nil)
	assert.False(t, !ok || len(compiledNoDdos.Ddos) != 0, "expected no ddos policies, got %+v", compiledNoDdos.Ddos)
}

func TestCompileDdosPoliciesSortsByPriorityAndDropsDisabled(t *testing.T) {
	policies := []management.Record{
		record("high-priority-num", nil, map[string]any{"sensitivity": "low", "action": "log", "requestRateThreshold": 100, "burstWindow": 60, "priority": 200, "enabled": true}),
		record("low-priority-num", nil, map[string]any{"sensitivity": "high", "action": "block", "requestRateThreshold": 10, "burstWindow": 60, "priority": 10, "enabled": true}),
		record("disabled", nil, map[string]any{"sensitivity": "high", "action": "block", "requestRateThreshold": 10, "burstWindow": 60, "priority": 1, "enabled": false}),
	}
	compiled := compileDdosPolicies(policies)
	require.Len(t, compiled, 2, "expected the disabled policy to be dropped, got %d", len(compiled))
	assert.False(t, compiled[0].ID != "low-priority-num" || compiled[1].ID != "high-priority-num", "expected priority ascending order, got %+v", compiled)
}

func TestCompileDdosReadsScopeFields(t *testing.T) {
	policy := record("policy-1", nil, map[string]any{
		"sensitivity": "high", "action": "block", "requestRateThreshold": 500, "burstWindow": 60,
		"priority": 50, "pathPrefix": "/login", "httpMethod": "POST", "enabled": true,
	})
	compiled, ok := compileDdos(policy)
	require.True(t, ok, "expected a compiled policy")
	assert.False(t, compiled.PathPrefix == nil || *compiled.PathPrefix != "/login", "expected pathPrefix /login, got %+v", compiled.PathPrefix)
	assert.False(t, compiled.HttpMethod == nil || *compiled.HttpMethod != "POST", "expected httpMethod POST, got %+v", compiled.HttpMethod)
	assert.EqualValues(t, 50, compiled.Priority, "expected priority 50, got %d", compiled.Priority)
}

func TestCompileDdosEmptyScopeFieldsMeanUnscoped(t *testing.T) {
	policy := record("policy-1", nil, map[string]any{
		"sensitivity": "high", "action": "block", "requestRateThreshold": 500, "burstWindow": 60,
		"pathPrefix": "", "httpMethod": "", "enabled": true,
	})
	compiled, ok := compileDdos(policy)
	require.True(t, ok, "expected a compiled policy")
	assert.False(t, compiled.PathPrefix != nil || compiled.HttpMethod != nil, "expected unscoped (nil) pathPrefix/httpMethod, got %+v / %+v", compiled.PathPrefix, compiled.HttpMethod)
}

func accessListEntry(id, listType, value string, enabled bool) management.Record {
	return record(id, nil, map[string]any{"type": listType, "value": value, "enabled": enabled})
}

func TestCompileIPAccessListWhitelistCompilesToAllow(t *testing.T) {
	entry := accessListEntry("wl-1", "whitelist", "10.0.0.5", true)
	compiled, ok := compileIPAccessList(entry)
	require.True(t, ok, "expected a compiled entry")
	assert.False(t, compiled.Action != ruleset.ActionAllow || compiled.RuleType != "ipAccessList", "unexpected compiled entry: %+v", compiled)
	assert.False(t, compiled.MatchExpr.Kind != ruleset.MatchPredicate || compiled.MatchExpr.Predicate.Field != ruleset.FieldSourceIpCidr ||
		compiled.MatchExpr.Predicate.Op != ruleset.OpEq || *compiled.MatchExpr.Predicate.Value != "10.0.0.5", "unexpected match expr: %+v", compiled.MatchExpr)
}

func TestCompileIPAccessListBlacklistCompilesToBlock(t *testing.T) {
	entry := accessListEntry("bl-1", "blacklist", "10.0.0.0/24", true)
	compiled, ok := compileIPAccessList(entry)
	assert.False(t, !ok || compiled.Action != ruleset.ActionBlock, "expected block action, got %+v ok=%v", compiled, ok)
}

func TestCompileIPAccessListDropsDisabledEntries(t *testing.T) {
	entry := accessListEntry("wl-1", "whitelist", "10.0.0.5", false)
	_, ok := compileIPAccessList(entry)
	assert.False(t, ok, "expected disabled entry to be dropped")
}

func TestCompileZonePlacesIPAccessListsBeforeRegularRulesAllowBeforeBlock(t *testing.T) {
	z := zoneRecord(t, "active", nil)
	rules := []management.Record{
		record("r-1", nil, map[string]any{"name": "regular", "ruleType": "waf", "action": "block", "priority": 1, "enabled": true}),
	}
	accessLists := []management.Record{
		accessListEntry("bl-1", "blacklist", "10.0.0.1", true),
		accessListEntry("wl-1", "whitelist", "10.0.0.2", true),
	}
	compiled, ok := CompileZone(z, "tenant-1", nil, rules, accessLists)
	require.True(t, ok, "expected a compiled zone")
	require.Len(t, compiled.Rules, 3, "expected 3 rules, got %d", len(compiled.Rules))
	// Whitelist (allow) first regardless of input order, then blacklist (block), then the
	// regular rule last regardless of its own (lower) priority number.
	assert.False(t, compiled.Rules[0].ID != "wl-1" || compiled.Rules[1].ID != "bl-1" || compiled.Rules[2].ID != "r-1", "unexpected rule order: %+v", compiled.Rules)
}
