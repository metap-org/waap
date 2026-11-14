package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStaysUnderBudgetUntilThresholdIsCrossed(t *testing.T) {
	limiter := New()
	window := 60 * time.Second
	// threshold 3: the 4th hit in the window is the first one over budget.
	assert.False(t, limiter.Check("k", 3, window), "expected hit 1 to be under budget")
	assert.False(t, limiter.Check("k", 3, window), "expected hit 2 to be under budget")
	assert.False(t, limiter.Check("k", 3, window), "expected hit 3 to be under budget")
	assert.True(t, limiter.Check("k", 3, window), "expected hit 4 to be over budget")
	assert.True(t, limiter.Check("k", 3, window), "expected to stay over budget for further hits in the same window")
}

func TestThresholdZeroTripsOnTheFirstHit(t *testing.T) {
	limiter := New()
	assert.True(t, limiter.Check("k", 0, 60*time.Second), "expected threshold 0 to trip immediately")
}

func TestDifferentKeysAreIndependent(t *testing.T) {
	limiter := New()
	window := 60 * time.Second
	assert.False(t, limiter.Check("a", 1, window), "expected key a's first hit to be under budget")
	assert.False(t, limiter.Check("b", 1, window), "expected key b's first hit to be under budget, independent of a")
	assert.True(t, limiter.Check("a", 1, window), "expected key a's second hit to be over budget")
}

func TestWindowResetsAfterItElapses(t *testing.T) {
	limiter := New()
	window := 30 * time.Millisecond
	assert.False(t, limiter.Check("k", 1, window), "expected first hit to be under budget")
	assert.True(t, limiter.Check("k", 1, window), "expected to still be inside the window, over budget")
	time.Sleep(60 * time.Millisecond)
	assert.False(t, limiter.Check("k", 1, window), "expected a new window to reset the counter")
}

func TestTrackedKeysCountsDistinctKeys(t *testing.T) {
	limiter := New()
	window := 60 * time.Second
	limiter.Check("a", 10, window)
	limiter.Check("b", 10, window)
	limiter.Check("a", 10, window) // repeat — must not add a second entry for "a".
	got := limiter.TrackedKeys()
	assert.Equal(t, 2, got, "expected 2 tracked keys, got %d", got)
}

func TestRuleKeyAndDdosKeyHaveDisjointPrefixes(t *testing.T) {
	// Same zone/client, different rule — the two key spaces must never collide, or a firewall
	// rule's budget could be silently shared with the zone's DDoS budget.
	rule := RuleKey("zone-1", "rule-1", "1.2.3.4")
	ddos := DdosKey("zone-1", "policy-1", "1.2.3.4")
	assert.NotEqual(t, ddos, rule, "expected rule and ddos keys to differ")
	assert.Equal(t, "r:", rule[:2], "expected rule key to start with r:, got %q", rule)
	assert.Equal(t, "d:", ddos[:2], "expected ddos key to start with d:, got %q", ddos)
}
