package ruleset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func zoneWith(rules ...CompiledRule) CompiledZone {
	return CompiledZone{SchemaVersion: RulesetSchemaVersion, ZoneID: "z", Hostname: "h.example.com", Status: "active", Rules: rules, CompiledAt: "2026-01-01T00:00:00Z"}
}

func TestContentVersionFollowsTheContentOnly(t *testing.T) {
	a := zoneWith(CompiledRule{ID: "r1", Name: "one", Action: "block", Priority: 10, MatchExpr: Always()})
	later := a
	later.CompiledAt = "2030-01-01T00:00:00Z"
	assert.Equal(t, ContentVersion(a), ContentVersion(later), "recompiling the same content later is not a change")

	more := zoneWith(CompiledRule{ID: "r1", Name: "one", Action: "block", Priority: 10, MatchExpr: Always()}, CompiledRule{ID: "r2", Name: "two", Action: "block", Priority: 20, MatchExpr: Always()})
	assert.NotEqual(t, ContentVersion(a), ContentVersion(more), "a new rule is a new version")

	changed := zoneWith(CompiledRule{ID: "r1", Name: "one", Action: "log", Priority: 10, MatchExpr: Always()})
	assert.NotEqual(t, ContentVersion(a), ContentVersion(changed), "an edited rule is a new version")

	v := ContentVersion(a)
	assert.Positive(t, v)
	assert.Less(t, v, int64(1)<<53, "exact in any JSON consumer")
}
