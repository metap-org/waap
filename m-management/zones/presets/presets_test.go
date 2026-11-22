package presets

import (
	"encoding/json"
	"testing"

	"github.com/metap-org/waap/m-management/zones/rules"
	"github.com/stretchr/testify/assert"
)

// Every rule a preset ships is one the validator (and so the compiler) accepts: a preset that
// created rules which then never apply would be worse than none.
func TestEveryPresetRuleHasAValidCondition(t *testing.T) {
	for key, p := range presets {
		assert.Equal(t, key, p.key)
		assert.NotEmpty(t, p.ddosSensitivity, key)
		assert.Positive(t, p.ddosThreshold, key)
		priorities := map[int]bool{}
		for _, r := range p.rules {
			assert.True(t, rules.IsValidMatchCondition(json.RawMessage(r.condition)), "%s / %s", key, r.name)
			assert.False(t, priorities[r.priority], "%s: rules share priority %d", key, r.priority)
			priorities[r.priority] = true
			if r.rateThreshold > 0 {
				assert.Positive(t, r.rateWindow, "%s / %s", key, r.name)
			}
		}
	}
	assert.Len(t, presets, len(PresetKeys()))
}
