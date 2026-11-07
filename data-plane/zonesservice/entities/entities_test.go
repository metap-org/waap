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
