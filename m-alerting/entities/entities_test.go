package entities

import (
	"testing"

	"github.com/metap-org/metap/pkg/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnsExactlyItsOwnThreeEntities(t *testing.T) {
	registry := metadata.NewMetadataRegistry()
	require.NoError(t, registry.RegisterAllSubmitted(), "RegisterAllSubmitted")
	require.NoError(t, registry.ValidateReferences(), "ValidateReferences")

	var names []string
	for _, e := range registry.ListEntities() {
		names = append(names, e.Name)
	}
	for _, want := range []string{"waf.incidents", "waf.alert_policies", "waf.alert_notifications"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		assert.True(t, found, "missing entity: %s", want)
	}
	assert.Len(t, names, 3, "unexpected entity count: %v", names)
}
