package policy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
	"github.com/stretchr/testify/assert"
)

func item(id string, fields map[string]any) versions.SnapshotItem {
	d := crud.JsonObject{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		d[k] = b
	}
	return versions.SnapshotItem{ID: id, Data: d}
}

func codes(r ValidationReport) []string {
	out := []string{}
	for _, f := range r.Findings {
		out = append(out, f.Severity+":"+f.Code)
	}
	return out
}

var now = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func goodRule(id, name string, priority int, extra map[string]any) versions.SnapshotItem {
	f := map[string]any{"name": name, "priority": priority, "enabled": true, "ruleType": "waf", "action": "block",
		"matchCondition": map[string]any{"field": "uri.path", "op": "eq", "value": "/x"}}
	for k, v := range extra {
		f[k] = v
	}
	return item(id, f)
}

func TestAnEmptyZoneIsWarnedAbout(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{}, now)
	assert.Equal(t, []string{"warning:noProtection"}, codes(r))
	assert.Zero(t, r.Errors)
}

func TestACleanPolicyHasNoFindings(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{model.FirewallRules: {goodRule("a", "a", 1, nil), goodRule("b", "b", 2, nil)}}, now)
	assert.Empty(t, r.Findings)
	assert.Equal(t, 2, r.Rules)
}

func TestAnInvalidConditionIsAnError(t *testing.T) {
	bad := item("x", map[string]any{"name": "bad", "priority": 1, "enabled": true, "matchCondition": map[string]any{"field": "bogus", "op": "eq", "value": "x"}})
	r := checkSnapshot(versions.Snapshot{model.FirewallRules: {bad}}, now)
	assert.Contains(t, codes(r), "error:ruleConditionInvalid")
	assert.Equal(t, 1, r.Errors)
}

func TestRulesSharingAPriorityAreWarnedAboutTogether(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{model.FirewallRules: {goodRule("a", "one", 5, nil), goodRule("b", "two", 5, nil), goodRule("c", "three", 9, nil)}}, now)
	assert.Equal(t, []string{"warning:rulePriorityShared"}, codes(r))
	assert.Equal(t, "one, two", r.Findings[0].Subject)
}

func TestDisabledAndExpiredRulesAreNotJudgedAsLive(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{model.FirewallRules: {
		goodRule("a", "off", 1, map[string]any{"enabled": false}),
		goodRule("b", "old", 1, map[string]any{"expiresAt": "2026-11-22T11:00:00Z"}),
	}}, now)
	assert.Equal(t, []string{"info:ruleExpired", "warning:noProtection"}, codes(r))
}

func TestARateLimitRuleWithoutAThresholdIsWarnedAbout(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{model.FirewallRules: {goodRule("a", "rl", 1, map[string]any{"ruleType": "rateLimit"})}}, now)
	assert.Equal(t, []string{"warning:rateLimitThresholdMissing"}, codes(r))
}

func TestAnAddressOnBothListsIsWarnedAbout(t *testing.T) {
	r := checkSnapshot(versions.Snapshot{model.IPAccessLists: {
		item("w", map[string]any{"type": "whitelist", "value": "203.0.113.5", "enabled": true}),
		item("b", map[string]any{"type": "blacklist", "value": "203.0.113.5", "enabled": true}),
		item("o", map[string]any{"type": "blacklist", "value": "198.51.100.1", "enabled": true}),
	}}, now)
	assert.Equal(t, []string{"warning:ipOnBothLists"}, codes(r))
	assert.Equal(t, "203.0.113.5", r.Findings[0].Subject)
	assert.Equal(t, 3, r.Entries)
}
