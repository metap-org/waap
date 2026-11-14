package entities

import (
	"testing"

	"github.com/metap-org/metap/pkg/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOwnsExactlyItsOwnFiveEntities is a regression guard for RegisterAllSubmitted — catches an
// entities/*.go file that forgot its init() SubmitEntity call, a name collision, or another
// service's entity accidentally ending up registered here.
func TestOwnsExactlyItsOwnFiveEntities(t *testing.T) {
	registry := metadata.NewMetadataRegistry()
	require.NoError(t, registry.RegisterAllSubmitted(), "RegisterAllSubmitted")
	require.NoError(t, registry.ValidateReferences(), "ValidateReferences")

	var names []string
	for _, e := range registry.ListEntities() {
		names = append(names, e.Name)
	}
	expected := []string{"waf.domains", "waf.zones", "waf.ddos_policies", "waf.firewall_rules", "waf.ip_access_lists"}
	for _, want := range expected {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		assert.True(t, found, "missing entity: %s", want)
	}
	assert.Len(t, names, len(expected), "unexpected entity count: %v", names)
}

// app.Main reconciles metadata.Registry.Definitions() when no explicit list is given, so the
// order it derives must put every Reference target before the entities pointing at it.
func TestSubmittedEntitiesReconcileInReferenceOrder(t *testing.T) {
	reg := metadata.NewMetadataRegistry()
	require.NoError(t, reg.RegisterAllSubmitted())
	pos := map[string]int{}
	for i, e := range reg.Definitions() {
		pos[e.Name] = i
	}
	require.Contains(t, pos, "waf.zones")
	require.Less(t, pos["waf.domains"], pos["waf.zones"])
	for _, child := range []string{"waf.ddos_policies", "waf.firewall_rules", "waf.ip_access_lists"} {
		require.Less(t, pos["waf.zones"], pos[child], child)
	}
}
