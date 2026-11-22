package zonesservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanForTierResolvesKnownTiersAndFallsBackToFree(t *testing.T) {
	assert.Equal(t, "business", PlanForTier("business").Name)
	assert.Equal(t, "business", PlanForTier("  Business ").Name, "case and spaces do not matter")
	assert.Equal(t, PlanFree, PlanForTier("trial").Name, "metap's default tier gets the free plan")
	assert.Equal(t, PlanFree, PlanForTier("").Name)
	assert.Equal(t, PlanFree, PlanForTier("platinum-gold").Name, "an unknown tier is never unlimited")
	assert.Equal(t, Unlimited, PlanForTier("enterprise").MaxZones)
}

func TestPlanCatalogIsOrderedByAllowance(t *testing.T) {
	prev := 0
	for _, p := range PlanCatalog() {
		if p.MaxZones == Unlimited {
			continue
		}
		assert.Greater(t, p.MaxZones, prev, p.Name)
		prev = p.MaxZones
	}
}
