package billingservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortPlansPutsSmallestFirstAndUnlimitedLast(t *testing.T) {
	got := sortPlans([]Plan{{Name: "big", MaxZones: Unlimited}, {Name: "mid", MaxZones: 25}, {Name: "small", MaxZones: 1}})
	assert.Equal(t, []string{"small", "mid", "big"}, []string{got[0].Name, got[1].Name, got[2].Name})
}

func TestNothingIsEnforcedWithoutAnOperator(t *testing.T) {
	assert.False(t, (*PlanSource)(nil).enforced())
	assert.False(t, (&PlanSource{}).enforced())
}
