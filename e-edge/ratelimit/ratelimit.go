// Package ratelimit holds per-client request budgets, in memory.
//
// Fixed windows, not a sliding log. A sliding window keeps a timestamp per request, which at
// edge volumes is unbounded memory driven by attacker traffic — the wrong failure mode for the
// component whose job is surviving a flood. A fixed window is one counter per key, resets on a
// boundary, and its known weakness (up to 2x the budget across a boundary) is acceptable for
// mitigation thresholds that are approximate to begin with.
//
// Per-node, not global. Each edge node counts what it sees. With N nodes behind round-robin DNS
// the effective global budget is roughly Nx the configured one. Making it exact needs a shared
// counter on the hot path (a Redis round trip per request), which costs more than the precision
// is worth here. Documented rather than pretended otherwise — an operator sizing a threshold
// needs to know this.
//
// Sharded by key hash so concurrent requests for different clients don't queue behind one mutex.
package ratelimit

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

const shards = 32

// idleEviction: how much idle time before a counter is dropped during pruning. Longer than any
// realistic window so an active client's counter is never pruned mid-window.
const idleEviction = 300 * time.Second

type counter struct {
	windowStart time.Time
	hits        uint32
	lastSeen    time.Time
}

type shard struct {
	mu      sync.Mutex
	entries map[string]*counter
}

type RateLimiter struct {
	shards [shards]*shard
}

func New() *RateLimiter {
	rl := &RateLimiter{}
	for i := range rl.shards {
		rl.shards[i] = &shard{entries: make(map[string]*counter)}
	}
	return rl
}

func (rl *RateLimiter) shardFor(key string) *shard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return rl.shards[h.Sum32()%shards]
}

// Check records one hit against key and reports whether the client is now OVER threshold within
// window.
//
// Counting happens before the verdict, so the request that crosses the line is itself treated as
// over budget — the alternative (allow the crossing request, block the next) makes a threshold
// of 1 meaningless.
func (rl *RateLimiter) Check(key string, threshold uint32, window time.Duration) bool {
	now := time.Now()
	s := rl.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.entries[key]
	if !ok {
		c = &counter{windowStart: now, hits: 0, lastSeen: now}
		s.entries[key] = c
	}
	if now.Sub(c.windowStart) >= window {
		c.windowStart = now
		c.hits = 0
	}
	c.hits++
	c.lastSeen = now
	return c.hits > threshold
}

// Prune drops counters nothing has touched recently. Called on a timer rather than during Check
// so the hot path never pays for cleanup, and so a flood from many distinct IPs cannot make
// every request walk a growing map.
func (rl *RateLimiter) Prune() {
	now := time.Now()
	for _, s := range rl.shards {
		s.mu.Lock()
		for key, c := range s.entries {
			if now.Sub(c.lastSeen) >= idleEviction {
				delete(s.entries, key)
			}
		}
		s.mu.Unlock()
	}
}

func (rl *RateLimiter) TrackedKeys() int {
	total := 0
	for _, s := range rl.shards {
		s.mu.Lock()
		total += len(s.entries)
		s.mu.Unlock()
	}
	return total
}

// RuleKey is the rate-limit key for a firewall rule: one budget per (zone, rule, client).
func RuleKey(zoneID, ruleID, clientIP string) string {
	return fmt.Sprintf("r:%s:%s:%s", zoneID, ruleID, clientIP)
}

// DdosKey is the rate-limit key for one DDoS policy: one budget per (zone, policy, client) — a
// zone can carry more than one scoped policy, each with its own independent budget.
func DdosKey(zoneID, policyID, clientIP string) string {
	return fmt.Sprintf("d:%s:%s:%s", zoneID, policyID, clientIP)
}
