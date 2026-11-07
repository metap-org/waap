// Package regexcache compiles Op::Regex patterns once, not per request.
//
// Predicate.Value carries the raw pattern text over the wire (a compiled regexp can't be
// serialized), so there's no single point where a CompiledZone load could pre-compile every
// pattern once and hand back a ready-to-use object without restructuring the wire types. Instead,
// this caches by pattern text: the first request that hits a given pattern pays the compile cost,
// every later request (same pattern, any zone) hits the cache.
//
// The control-plane already validates a pattern compiles before publishing it and drops the rule
// otherwise, so a pattern arriving here should always be valid — but a defensively-wrong or
// hand-edited Redis value must still never panic the edge, so a bad pattern just never matches.
//
// Disclosed divergence: Go's regexp package is RE2-syntax (no backreferences/lookaround), not
// the Rust regex crate's own (also RE2-based, but not byte-identical) grammar — a pattern valid
// in one is not guaranteed valid, or to mean the same thing, in the other. Both control-plane
// ports (Rust's compile.rs, Go's compile package) already validate against their OWN regex
// engine before publishing, so this is a pre-existing, narrow cross-engine fidelity gap, not a
// new one introduced here.
package regexcache

import (
	"regexp"
	"sync"
)

var (
	mu    sync.Mutex
	cache = map[string]*regexp.Regexp{}
	// bad tracks patterns known not to compile, cached too, so a bad pattern doesn't re-attempt
	// compilation on every request that hits it. A separate set from cache because a *Regexp nil
	// value can't distinguish "not looked up yet" from "known bad" on its own.
	bad = map[string]bool{}
)

// Compiled returns (re, true) for a pattern that compiles, or (nil, false) for one that doesn't
// — cached either way.
func Compiled(pattern string) (*regexp.Regexp, bool) {
	mu.Lock()
	defer mu.Unlock()
	if re, ok := cache[pattern]; ok {
		return re, true
	}
	if bad[pattern] {
		return nil, false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		bad[pattern] = true
		return nil, false
	}
	cache[pattern] = re
	return re, true
}
