// Package evaluate is the mitigation decision: given a request and its zone's compiled
// rule-set, what happens?
//
// This is the hot path. Every design choice here is about doing the least work per request:
// rules arrive pre-sorted and pre-filtered, the match grammar is a closed type (no string
// dispatch), and nothing here allocates unless a predicate actually needs a lowercase copy.
//
// Evaluation order (whitelist/blacklist only means something if it bypasses everything else,
// DdosPolicy included, the way real Cloudflare-style IP Access Rules work):
//
//  1. IpAccessList rules — allow (whitelist) first, then block (blacklist), regardless of any
//     rule's own priority. priority only tie-breaks within the same action group; a match here
//     stops evaluation immediately, same as any other rule match below.
//  2. Regular firewall rules — priority order, first match wins.
//  3. DDoS policy — a per-client request budget for the whole zone, checked ONLY when nothing
//     above matched. A request already decided by an IpAccessList entry or a FirewallRule is not
//     also counted against the DDoS budget.
//
// compileZone (control-plane) is what puts zone.Rules in this order — this package's own loop is
// a plain, unconditional "first match wins" walk that does not need to know a rule's origin to
// respect the contract above.
//
// Monitor mode is applied last, at the boundary, by Decision.EffectiveAction — so the decision
// itself always records what WOULD have happened, which is what makes monitor mode useful rather
// than just "off".
package evaluate

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/metap-org/waap/edge-plane/ratelimit"
	"github.com/metap-org/waap/edge-plane/regexcache"
	"github.com/metap-org/waap/edge-plane/ruleset"
)

// RequestContext holds everything a predicate can test, extracted from the request once per
// request rather than re-parsed per rule.
type RequestContext struct {
	Method string
	Path   string
	Query  string
	// No Host field: a zone is one hostname, so which zone's rules are even being evaluated
	// already answers "what host was this" — a predicate never needs to ask it again.
	ClientIP     net.IP
	ClientIPText string
	UserAgent    string
	// Headers: already lower-cased key lookup via http.Header.Get.
	Headers http.Header
	// Country: ISO-3166 alpha-2, or empty when no geo source is configured. Country rules simply
	// never match in that case.
	Country string
}

// Decision records why a request was acted on.
type Decision struct {
	Action Action
	// TriggeredBy: "ddosPolicy" or "firewallRule" — matches waf.security_events.triggeredBy's
	// enum.
	TriggeredBy     string
	TriggeredByID   string
	TriggeredByName string
}

type Action = ruleset.Action

// EffectiveAction: what actually happens to the request. In monitor mode every verdict degrades
// to Log: the request is passed to the origin, but the event still reports the real action so a
// customer can see what enforcing would have done before turning it on.
func (d Decision) EffectiveAction(zone ruleset.CompiledZone) Action {
	if zone.Enforcing() {
		return d.Action
	}
	return ruleset.ActionLog
}

func headerValue(ctx *RequestContext, name string) string {
	return ctx.Headers.Get(name)
}

func fieldValue(ctx *RequestContext, predicate *ruleset.Predicate) string {
	switch predicate.Field {
	case ruleset.FieldUriPath:
		return ctx.Path
	case ruleset.FieldUriQuery:
		return ctx.Query
	case ruleset.FieldMethod:
		return ctx.Method
	case ruleset.FieldUserAgent:
		return ctx.UserAgent
	case ruleset.FieldSourceIp, ruleset.FieldSourceIpCidr:
		return ctx.ClientIPText
	case ruleset.FieldCountry:
		return ctx.Country
	case ruleset.FieldHeader:
		if predicate.Param == nil {
			return ""
		}
		return headerValue(ctx, *predicate.Param)
	default:
		return ""
	}
}

// ipInCIDR: is ip inside cidr? Hand-rolled rather than pulling in an IP-network library: this is
// one prefix comparison, and the edge's dependency list is something to keep short on purpose. A
// malformed CIDR never matches — a rule the operator wrote wrong must not silently widen.
//
// Disclosed divergence from the Rust original: Rust's IpAddr keeps an explicit V4/V6
// discriminant regardless of bit pattern, so it treats an IPv4-mapped IPv6 client address
// (::ffff:10.0.0.1) as a mismatch against a plain IPv4 CIDR — a documented gap, not a feature.
// Go's net.IP has no such discriminant (ParseIP/.To4() normalize a v4-mapped v6 address to its
// embedded IPv4 form), so net.IPNet.Contains treats the two as equivalent — a behavioral
// difference, but one that makes this stricter check MORE permissive in exactly the direction
// NAT64/dual-stack semantics already consider correct, not a new security gap.
func ipInCIDR(ip net.IP, cidr string) bool {
	if !strings.Contains(cidr, "/") {
		// A bare address is treated as an exact match, which is what someone writing "10.0.0.1"
		// in an IP-firewall rule means.
		return networkEq(ip, cidr)
	}
	// net.ParseCIDR also validates the prefix length range (rejecting e.g. /99 for IPv4, /-1 for
	// any family) — same guard the Rust original hand-rolls separately.
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return ipNet.Contains(ip)
}

func networkEq(ip net.IP, text string) bool {
	parsed := net.ParseIP(text)
	return parsed != nil && parsed.Equal(ip)
}

func evalPredicate(ctx *RequestContext, predicate *ruleset.Predicate) bool {
	if predicate.Field == ruleset.FieldSourceIpCidr {
		var inside bool
		switch predicate.Op {
		case ruleset.OpIn, ruleset.OpNotIn:
			for _, cidr := range predicate.Values {
				if ipInCIDR(ctx.ClientIP, cidr) {
					inside = true
					break
				}
			}
		default:
			if predicate.Value != nil {
				inside = ipInCIDR(ctx.ClientIP, *predicate.Value)
			}
		}
		switch predicate.Op {
		case ruleset.OpNotEq, ruleset.OpNotIn, ruleset.OpNotContains:
			return !inside
		default:
			return inside
		}
	}

	actual := fieldValue(ctx, predicate)
	switch predicate.Op {
	case ruleset.OpEq:
		return predicate.Value != nil && *predicate.Value == actual
	case ruleset.OpNotEq:
		return predicate.Value == nil || *predicate.Value != actual
	case ruleset.OpContains:
		return predicate.Value != nil && strings.Contains(actual, *predicate.Value)
	case ruleset.OpNotContains:
		return predicate.Value == nil || !strings.Contains(actual, *predicate.Value)
	case ruleset.OpStartsWith:
		return predicate.Value != nil && strings.HasPrefix(actual, *predicate.Value)
	case ruleset.OpEndsWith:
		return predicate.Value != nil && strings.HasSuffix(actual, *predicate.Value)
	case ruleset.OpIn:
		for _, v := range predicate.Values {
			if v == actual {
				return true
			}
		}
		return false
	case ruleset.OpNotIn:
		for _, v := range predicate.Values {
			if v == actual {
				return false
			}
		}
		return true
	case ruleset.OpContainsCi:
		// The only allocating branch, and only for rules that ask for it.
		return predicate.Value != nil && strings.Contains(strings.ToLower(actual), strings.ToLower(*predicate.Value))
	case ruleset.OpRegex:
		if predicate.Value == nil {
			return false
		}
		re, ok := regexcache.Compiled(*predicate.Value)
		return ok && re.MatchString(actual)
	default:
		return false
	}
}

func evalMatch(ctx *RequestContext, expr *ruleset.MatchExpr) bool {
	switch expr.Kind {
	case ruleset.MatchAlways:
		return true
	case ruleset.MatchPredicate:
		return evalPredicate(ctx, expr.Predicate)
	case ruleset.MatchAll:
		for i := range expr.Children {
			if !evalMatch(ctx, &expr.Children[i]) {
				return false
			}
		}
		return true
	case ruleset.MatchAny:
		for i := range expr.Children {
			if evalMatch(ctx, &expr.Children[i]) {
				return true
			}
		}
		return false
	case ruleset.MatchNot:
		return !evalMatch(ctx, expr.Not)
	default:
		return false
	}
}

// Evaluate runs the zone's policy and rules against one request.
//
// Returns (Decision{}, false) when nothing matched — the overwhelmingly common case, and the one
// this function is optimised for: no allocation, no clone, just a walk down a short pre-sorted
// list.
func Evaluate(zone *ruleset.CompiledZone, ctx *RequestContext, limiter *ratelimit.RateLimiter) (Decision, bool) {
	for i := range zone.Rules {
		rule := &zone.Rules[i]
		if !evalMatch(ctx, &rule.MatchExpr) {
			continue
		}
		// A rate-limit rule matches on its condition AND on the client being over budget.
		// Counting only for requests whose condition matched is what makes "100 requests to
		// /login per minute" mean that, rather than 100 requests to anything.
		if rule.RateLimit != nil {
			overBudget := limiter.Check(
				ratelimit.RuleKey(zone.ZoneID, rule.ID, ctx.ClientIPText),
				rule.RateLimit.Threshold,
				time.Duration(rule.RateLimit.WindowSeconds)*time.Second,
			)
			if !overBudget {
				continue
			}
		}
		// Allow is a real verdict, not "keep looking": an allow rule above a block rule is how
		// an operator writes an exception, so it must stop evaluation like any other match.
		return Decision{Action: rule.Action, TriggeredBy: "firewallRule", TriggeredByID: rule.ID, TriggeredByName: rule.Name}, true
	}

	// Only reached when nothing above matched — see this package's doc comment for why DDoS is
	// the last resort rather than the first check. zone.Ddos is already priority-sorted by the
	// control-plane — first policy whose scope matches this request AND whose own budget is
	// exceeded wins, exactly the same "first match wins" contract zone.Rules above already has.
	for i := range zone.Ddos {
		ddos := &zone.Ddos[i]
		if !ddosScopeMatches(ddos, ctx) {
			continue
		}
		overBudget := limiter.Check(
			ratelimit.DdosKey(zone.ZoneID, ddos.ID, ctx.ClientIPText),
			ddos.RequestRateThreshold,
			time.Duration(ddos.BurstWindowSeconds)*time.Second,
		)
		if overBudget {
			return Decision{
				Action: ddos.Action, TriggeredBy: "ddosPolicy", TriggeredByID: ddos.ID,
				TriggeredByName: "DDoS policy (" + ddos.Sensitivity + ")",
			}, true
		}
	}

	return Decision{}, false
}

// ddosScopeMatches: does this policy's scope apply to the current request? Both
// PathPrefix/HttpMethod absent (or HttpMethod "any") means "every request" — the default shape a
// single, unscoped policy already had before scoped policies were added.
func ddosScopeMatches(ddos *ruleset.CompiledDdos, ctx *RequestContext) bool {
	if ddos.PathPrefix != nil && !strings.HasPrefix(ctx.Path, *ddos.PathPrefix) {
		return false
	}
	if ddos.HttpMethod != nil && *ddos.HttpMethod != "any" && *ddos.HttpMethod != ctx.Method {
		return false
	}
	return true
}
