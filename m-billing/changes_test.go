package billingservice

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/metap-org/metap/pkg/crud"
)

func TestIsUpgradeOnlyWhenNothingGetsSmaller(t *testing.T) {
	small := Plan{MaxZones: 1, MaxRulesPerZone: 5}
	mid := Plan{MaxZones: 5, MaxRulesPerZone: 25}
	big := Plan{MaxZones: Unlimited, MaxRulesPerZone: Unlimited}
	assert.True(t, isUpgrade(small, mid))
	assert.True(t, isUpgrade(mid, big))
	assert.False(t, isUpgrade(mid, small))
	assert.False(t, isUpgrade(big, mid))
	assert.False(t, isUpgrade(mid, mid), "the same plan is not a move up")
	// More zones but fewer rules gives less of something: treated as a downgrade, so it waits.
	assert.False(t, isUpgrade(Plan{MaxZones: 5, MaxRulesPerZone: 25}, Plan{MaxZones: 10, MaxRulesPerZone: 10}))
}

func TestEffectivePlanSwitchesWhenThePendingMoveIsDue(t *testing.T) {
	sub := func(pending, from string) crud.JsonObject {
		o := crud.JsonObject{"planId": jsonStr("current")}
		if pending != "" {
			o["pendingPlanId"], o["pendingFrom"] = jsonStr(pending), jsonStr(from)
		}
		return o
	}
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	get := func(o crud.JsonObject) string {
		var s string
		_ = json.Unmarshal(effectivePlanID(o, now), &s)
		return s
	}
	assert.Equal(t, "current", get(sub("", "")))
	assert.Equal(t, "current", get(sub("next", "2026-12-01T00:00:00Z")), "not due yet")
	assert.Equal(t, "next", get(sub("next", "2026-11-22T12:00:00Z")), "due now")
	assert.Equal(t, "next", get(sub("next", "2026-11-01T00:00:00Z")), "overdue")
	assert.Equal(t, "current", get(sub("next", "garbage")), "an unreadable date never moves anyone")
}
