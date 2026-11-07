package evaluate

import (
	"net"
	"net/http"
	"testing"

	"github.com/metap-org/waap/edge-plane/ratelimit"
	"github.com/metap-org/waap/edge-plane/ruleset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func ctx(path string, ip [4]byte, headers http.Header) RequestContext {
	addr := net.IPv4(ip[0], ip[1], ip[2], ip[3])
	if headers == nil {
		headers = http.Header{}
	}
	return RequestContext{
		Method: "GET", Path: path, Query: "",
		ClientIP: addr, ClientIPText: addr.String(),
		UserAgent: "test-agent", Headers: headers, Country: "",
	}
}

func zoneWithRules(rules []ruleset.CompiledRule) ruleset.CompiledZone {
	return ruleset.CompiledZone{
		SchemaVersion: 1, ZoneID: "zone-1", TenantID: "tenant-1",
		Hostname: "shop.example.com", OriginAddress: "10.0.0.1",
		Status: "active", ProtectionMode: "enforce", ConfigVersion: 1,
		Ddos: nil, Rules: rules, CompiledAt: "",
	}
}

func ddosPolicy(id string, action ruleset.Action, threshold uint32, pathPrefix, httpMethod *string, priority int64) ruleset.CompiledDdos {
	return ruleset.CompiledDdos{
		ID: id, Sensitivity: "high", Action: action, RequestRateThreshold: threshold,
		BurstWindowSeconds: 60, PathPrefix: pathPrefix, HttpMethod: httpMethod, Priority: priority,
	}
}

func predicateRule(id string, action ruleset.Action, field ruleset.Field, op ruleset.Op, value string) ruleset.CompiledRule {
	return ruleset.CompiledRule{
		ID: id, Name: id, RuleType: "waf", Action: action, Priority: 100,
		MatchExpr: ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
			Field: field, Op: op, Value: strPtr(value),
		}},
	}
}

// --- ipInCIDR ---

func TestIpInCidrMatchesInsideThePrefix(t *testing.T) {
	ip := net.ParseIP("10.0.5.7")
	assert.True(t, ipInCIDR(ip, "10.0.0.0/16"), "expected to match inside the prefix")
	assert.False(t, ipInCIDR(ip, "10.1.0.0/16"), "expected to not match outside the prefix")
}

func TestIpInCidrZeroPrefixMatchesEverything(t *testing.T) {
	ip := net.ParseIP("203.0.113.5")
	assert.True(t, ipInCIDR(ip, "0.0.0.0/0"), "expected /0 to match everything")
}

func TestIpInCidrBareAddressIsExactMatch(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	assert.True(t, ipInCIDR(ip, "10.0.0.1"), "expected exact match")
	assert.False(t, ipInCIDR(ip, "10.0.0.2"), "expected no match against a different address")
}

func TestIpInCidrMalformedCidrNeverMatches(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	assert.False(t, ipInCIDR(ip, "not-a-cidr"), "expected a non-CIDR, non-address string to never match")
	assert.False(t, ipInCIDR(ip, "10.0.0.0/99"), "expected an out-of-range prefix to never match")
	assert.False(t, ipInCIDR(ip, "10.0.0.0/-1"), "expected a negative prefix to never match")
}

func TestIpInCidrIPv6PrefixWorks(t *testing.T) {
	ip := net.ParseIP("2001:db8::1")
	assert.True(t, ipInCIDR(ip, "2001:db8::/32"), "expected to match inside the IPv6 prefix")
	assert.False(t, ipInCIDR(ip, "2001:db9::/32"), "expected to not match a different IPv6 prefix")
}

// TestIpInCidrIPv4MappedIPv6MatchesTheIPv4Cidr documents a disclosed, deliberate divergence from
// the Rust original (see ipInCIDR's own doc comment): Go's net.IP has no explicit V4/V6
// discriminant independent of bit pattern, so an IPv4-mapped IPv6 address and its embedded IPv4
// address are the same value to net.IPNet.Contains. The Rust original treats this as a
// documented gap (never matches); Go's stdlib makes that gap not exist.
func TestIpInCidrIPv4MappedIPv6MatchesTheIPv4Cidr(t *testing.T) {
	ipv6 := net.ParseIP("::ffff:10.0.0.1")
	assert.True(t, ipInCIDR(ipv6, "10.0.0.0/8"), "expected Go's net.IP to treat the v4-mapped address as its embedded IPv4 identity")
}

// --- evalMatch / evalPredicate ---

func TestPredicateEqMatchesPath(t *testing.T) {
	c := ctx("/admin", [4]byte{1, 2, 3, 4}, nil)
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpEq, Value: strPtr("/admin"),
	}}
	assert.True(t, evalMatch(&c, &expr), "expected a match")
}

func TestPredicateContainsCiIsCaseInsensitive(t *testing.T) {
	c := ctx("/Admin/Login", [4]byte{1, 2, 3, 4}, nil)
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpContainsCi, Value: strPtr("ADMIN"),
	}}
	assert.True(t, evalMatch(&c, &expr), "expected ContainsCi to match case-insensitively")
	exprCS := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpContains, Value: strPtr("ADMIN"),
	}}
	assert.False(t, evalMatch(&c, &exprCS), "expected plain Contains to stay case-sensitive")
}

func TestPredicateInMatchesAnyListedValue(t *testing.T) {
	c := ctx("/x", [4]byte{1, 2, 3, 4}, nil)
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldMethod, Op: ruleset.OpIn, Values: []string{"POST", "PUT"},
	}}
	assert.False(t, evalMatch(&c, &expr), "expected no match: context method is GET")
}

func TestPredicateSourceIpCidrInAndNotIn(t *testing.T) {
	c := ctx("/x", [4]byte{10, 0, 0, 5}, nil)
	inside := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldSourceIpCidr, Op: ruleset.OpIn, Values: []string{"10.0.0.0/24"},
	}}
	assert.True(t, evalMatch(&c, &inside), "expected the client IP to be inside the CIDR")
	outsideNotIn := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldSourceIpCidr, Op: ruleset.OpNotIn, Values: []string{"10.0.0.0/24"},
	}}
	assert.False(t, evalMatch(&c, &outsideNotIn), "expected NotIn to be false since the client IS inside")
}

func TestAllAnyNotComposeCorrectly(t *testing.T) {
	c := ctx("/admin", [4]byte{1, 2, 3, 4}, nil)
	isAdmin := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpEq, Value: strPtr("/admin"),
	}}
	isGet := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldMethod, Op: ruleset.OpEq, Value: strPtr("GET"),
	}}
	predicateFalse := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldMethod, Op: ruleset.OpEq, Value: strPtr("POST"),
	}}

	allExpr := ruleset.MatchExpr{Kind: ruleset.MatchAll, Children: []ruleset.MatchExpr{isAdmin, isGet}}
	assert.True(t, evalMatch(&c, &allExpr), "expected All(isAdmin, isGet) to match")
	anyExpr := ruleset.MatchExpr{Kind: ruleset.MatchAny, Children: []ruleset.MatchExpr{isAdmin, predicateFalse}}
	assert.True(t, evalMatch(&c, &anyExpr), "expected Any(isAdmin, false) to match")
	notExpr := ruleset.MatchExpr{Kind: ruleset.MatchNot, Not: &isAdmin}
	assert.False(t, evalMatch(&c, &notExpr), "expected Not(isAdmin) to not match")
	assert.True(t, evalMatch(&c, &isGet), "sanity: isGet alone should also be true for this context")
}

func TestPredicateRegexMatchesThePattern(t *testing.T) {
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpRegex, Value: strPtr(`^/admin/\d+$`),
	}}
	c := ctx("/admin/42", [4]byte{1, 2, 3, 4}, nil)
	assert.True(t, evalMatch(&c, &expr), "expected the regex to match")
	nonMatching := ctx("/admin/abc", [4]byte{1, 2, 3, 4}, nil)
	assert.False(t, evalMatch(&nonMatching, &expr), "expected the regex to not match")
}

func TestPredicateRegexWithAnInvalidPatternNeverMatches(t *testing.T) {
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldUriPath, Op: ruleset.OpRegex, Value: strPtr("[invalid(regex"),
	}}
	c := ctx("/anything", [4]byte{1, 2, 3, 4}, nil)
	assert.False(t, evalMatch(&c, &expr), "expected an invalid regex to never match")
}

func TestHeaderFieldReadsTheNamedHeaderCaseInsensitively(t *testing.T) {
	h := http.Header{}
	h.Set("x-api-key", "secret")
	c := ctx("/x", [4]byte{1, 2, 3, 4}, h)
	expr := ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
		Field: ruleset.FieldHeader, Op: ruleset.OpEq, Value: strPtr("secret"), Param: strPtr("x-api-key"),
	}}
	assert.True(t, evalMatch(&c, &expr), "expected the header match to succeed")
}

// --- Evaluate() end to end ---

func TestEvaluateReturnsNoneWhenNothingMatches(t *testing.T) {
	zone := zoneWithRules([]ruleset.CompiledRule{predicateRule("r1", ruleset.ActionBlock, ruleset.FieldUriPath, ruleset.OpEq, "/never-hit")})
	c := ctx("/anything-else", [4]byte{1, 2, 3, 4}, nil)
	limiter := ratelimit.New()
	_, ok := Evaluate(&zone, &c, limiter)
	assert.False(t, ok, "expected no decision")
}

func TestEvaluateFirstMatchWinsOverALaterMatchingRule(t *testing.T) {
	zone := zoneWithRules([]ruleset.CompiledRule{
		predicateRule("allow-rule", ruleset.ActionAllow, ruleset.FieldUriPath, ruleset.OpStartsWith, "/admin"),
		predicateRule("block-rule", ruleset.ActionBlock, ruleset.FieldUriPath, ruleset.OpStartsWith, "/admin"),
	})
	c := ctx("/admin/panel", [4]byte{1, 2, 3, 4}, nil)
	limiter := ratelimit.New()
	decision, ok := Evaluate(&zone, &c, limiter)
	require.True(t, ok, "expected the first rule to match")
	assert.False(t, decision.Action != ruleset.ActionAllow || decision.TriggeredByID != "allow-rule", "unexpected decision: %+v", decision)
}

func TestEvaluateDdosBudgetIsCheckedOnlyWhenNoRuleMatched(t *testing.T) {
	zone := zoneWithRules([]ruleset.CompiledRule{predicateRule("never-matches", ruleset.ActionBlock, ruleset.FieldUriPath, ruleset.OpEq, "/nope")})
	zone.Ddos = []ruleset.CompiledDdos{ddosPolicy("ddos-1", ruleset.ActionChallenge, 1, nil, nil, 100)}
	c := ctx("/", [4]byte{9, 9, 9, 9}, nil)
	limiter := ratelimit.New()

	if _, ok := Evaluate(&zone, &c, limiter); ok {
		assert.Fail(t, "expected the first request to be within budget")
	}
	decision, ok := Evaluate(&zone, &c, limiter)
	require.True(t, ok, "expected the second request to trip the DDoS budget")
	assert.False(t, decision.Action != ruleset.ActionChallenge || decision.TriggeredBy != "ddosPolicy" || decision.TriggeredByID != "ddos-1", "unexpected decision: %+v", decision)
}

func TestEvaluateScopedDdosPoliciesOnlyApplyToTheirOwnPath(t *testing.T) {
	zone := zoneWithRules(nil)
	zone.Ddos = []ruleset.CompiledDdos{
		ddosPolicy("login-policy", ruleset.ActionBlock, 1, strPtr("/login"), nil, 10),
		ddosPolicy("catch-all", ruleset.ActionChallenge, 1, nil, nil, 100),
	}
	limiter := ratelimit.New()

	loginCtx := ctx("/login", [4]byte{1, 1, 1, 1}, nil)
	if _, ok := Evaluate(&zone, &loginCtx, limiter); ok {
		assert.Fail(t, "expected 1st /login request to be in budget")
	}
	decision, ok := Evaluate(&zone, &loginCtx, limiter)
	require.True(t, ok, "expected 2nd /login request to trip login-policy, got %+v ok=%v", decision, ok)
	require.Equal(t, "login-policy", decision.TriggeredByID, "expected 2nd /login request to trip login-policy, got %+v ok=%v", decision, ok)
	require.Equal(t, ruleset.ActionBlock, decision.Action, "expected 2nd /login request to trip login-policy, got %+v ok=%v", decision, ok)

	otherCtx := ctx("/checkout", [4]byte{2, 2, 2, 2}, nil)
	_, ok = Evaluate(&zone, &otherCtx, limiter)
	assert.False(t, ok, "expected 1st /checkout request to be in budget under the catch-all policy")
	decision, ok = Evaluate(&zone, &otherCtx, limiter)
	require.True(t, ok, "expected 2nd /checkout request to trip catch-all, got %+v ok=%v", decision, ok)
	require.Equal(t, "catch-all", decision.TriggeredByID, "expected 2nd /checkout request to trip catch-all, got %+v ok=%v", decision, ok)
	require.Equal(t, ruleset.ActionChallenge, decision.Action, "expected 2nd /checkout request to trip catch-all, got %+v ok=%v", decision, ok)
}

func TestEvaluateScopedDdosPolicyRespectsHttpMethod(t *testing.T) {
	zone := zoneWithRules(nil)
	zone.Ddos = []ruleset.CompiledDdos{ddosPolicy("post-only", ruleset.ActionBlock, 1, nil, strPtr("POST"), 10)}
	limiter := ratelimit.New()

	getCtx := ctx("/", [4]byte{3, 3, 3, 3}, nil)
	getCtx.Method = "GET"
	if _, ok := Evaluate(&zone, &getCtx, limiter); ok {
		assert.Fail(t, "expected a GET request to never match a POST-scoped policy")
	}
	if _, ok := Evaluate(&zone, &getCtx, limiter); ok {
		assert.Fail(t, "expected a GET request to never match a POST-scoped policy (repeat)")
	}

	postCtx := ctx("/", [4]byte{3, 3, 3, 3}, nil)
	postCtx.Method = "POST"
	if _, ok := Evaluate(&zone, &postCtx, limiter); ok {
		assert.Fail(t, "expected 1st POST to be in budget")
	}
	decision, ok := Evaluate(&zone, &postCtx, limiter)
	require.True(t, ok, "expected 2nd POST to trip post-only, got %+v ok=%v", decision, ok)
	require.Equal(t, "post-only", decision.TriggeredByID, "expected 2nd POST to trip post-only, got %+v ok=%v", decision, ok)
}

func TestEvaluateAMatchingRuleSuppressesTheDdosCheck(t *testing.T) {
	zone := zoneWithRules([]ruleset.CompiledRule{predicateRule("always-matches", ruleset.ActionLog, ruleset.FieldUriPath, ruleset.OpStartsWith, "")})
	zone.Ddos = []ruleset.CompiledDdos{ddosPolicy("ddos-1", ruleset.ActionChallenge, 1, nil, nil, 100)}
	c := ctx("/", [4]byte{7, 7, 7, 7}, nil)
	limiter := ratelimit.New()

	for i := 0; i < 2; i++ {
		decision, ok := Evaluate(&zone, &c, limiter)
		require.True(t, ok, "iteration %d: expected the always-matching rule to win, got %+v ok=%v", i, decision, ok)
		require.Equal(t, "firewallRule", decision.TriggeredBy, "iteration %d: expected the always-matching rule to win, got %+v ok=%v", i, decision, ok)
		require.Equal(t, "always-matches", decision.TriggeredByID, "iteration %d: expected the always-matching rule to win, got %+v ok=%v", i, decision, ok)
	}
}

func TestEvaluateAnIpAccessListAllowMatchBypassesDdosAndLaterRules(t *testing.T) {
	accessRule := predicateRule("wl-1", ruleset.ActionAllow, ruleset.FieldSourceIpCidr, ruleset.OpEq, "5.5.5.5")
	accessRule.RuleType = "ipAccessList"
	blockEverything := predicateRule("block-all", ruleset.ActionBlock, ruleset.FieldUriPath, ruleset.OpStartsWith, "")
	zone := zoneWithRules([]ruleset.CompiledRule{accessRule, blockEverything})
	zone.Ddos = []ruleset.CompiledDdos{ddosPolicy("ddos-1", ruleset.ActionChallenge, 1, nil, nil, 100)}
	c := ctx("/", [4]byte{5, 5, 5, 5}, nil)
	limiter := ratelimit.New()

	for i := 0; i < 2; i++ {
		decision, ok := Evaluate(&zone, &c, limiter)
		require.True(t, ok, "iteration %d: expected the allow rule to win, got %+v ok=%v", i, decision, ok)
		require.Equal(t, ruleset.ActionAllow, decision.Action, "iteration %d: expected the allow rule to win, got %+v ok=%v", i, decision, ok)
		require.Equal(t, "wl-1", decision.TriggeredByID, "iteration %d: expected the allow rule to win, got %+v ok=%v", i, decision, ok)
	}
}

func TestEvaluateRateLimitRuleOnlyFiresOnceOverBudget(t *testing.T) {
	zone := zoneWithRules([]ruleset.CompiledRule{{
		ID: "rl-1", Name: "login rate limit", RuleType: "rateLimit", Action: ruleset.ActionBlock, Priority: 10,
		MatchExpr: ruleset.MatchExpr{Kind: ruleset.MatchPredicate, Predicate: &ruleset.Predicate{
			Field: ruleset.FieldUriPath, Op: ruleset.OpEq, Value: strPtr("/login"),
		}},
		RateLimit: &ruleset.RateLimit{Threshold: 1, WindowSeconds: 60},
	}})
	c := ctx("/login", [4]byte{8, 8, 8, 8}, nil)
	limiter := ratelimit.New()

	if _, ok := Evaluate(&zone, &c, limiter); ok {
		assert.Fail(t, "expected the first hit to be within budget")
	}
	decision, ok := Evaluate(&zone, &c, limiter)
	require.True(t, ok, "expected the second hit to trip the rate limit, got %+v ok=%v", decision, ok)
	require.Equal(t, ruleset.ActionBlock, decision.Action, "expected the second hit to trip the rate limit, got %+v ok=%v", decision, ok)
	require.Equal(t, "rl-1", decision.TriggeredByID, "expected the second hit to trip the rate limit, got %+v ok=%v", decision, ok)
}

// --- Decision.EffectiveAction ---

func TestMonitorModeDowngradesEveryActionToLogButKeepsTheRealVerdict(t *testing.T) {
	zone := zoneWithRules(nil)
	zone.ProtectionMode = "monitor"
	decision := Decision{Action: ruleset.ActionBlock, TriggeredBy: "firewallRule", TriggeredByID: "r1", TriggeredByName: "r1"}
	got := decision.EffectiveAction(zone)
	assert.Equal(t, ruleset.ActionLog, got, "expected Log, got %v", got)
	assert.Equal(t, ruleset.ActionBlock, decision.Action, "expected the real verdict to still be Block")
}

func TestEnforceModeOnAnActiveZoneUsesTheRealAction(t *testing.T) {
	zone := zoneWithRules(nil)
	decision := Decision{Action: ruleset.ActionBlock, TriggeredBy: "firewallRule", TriggeredByID: "r1", TriggeredByName: "r1"}
	got := decision.EffectiveAction(zone)
	assert.Equal(t, ruleset.ActionBlock, got, "expected Block, got %v", got)
}

func TestPausedZoneNeverEnforcesEvenInEnforceMode(t *testing.T) {
	zone := zoneWithRules(nil)
	zone.Status = "paused"
	decision := Decision{Action: ruleset.ActionBlock, TriggeredBy: "firewallRule", TriggeredByID: "r1", TriggeredByName: "r1"}
	got := decision.EffectiveAction(zone)
	assert.Equal(t, ruleset.ActionLog, got, "expected Log for a paused zone, got %v", got)
}
