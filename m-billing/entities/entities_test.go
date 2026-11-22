package entities

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/metadata"
)

// billing-service owns exactly the plan and subscription entities, and a subscription's plan
// reference resolves inside it.
func TestOwnsExactlyItsOwnEntities(t *testing.T) {
	registry := metadata.NewMetadataRegistry()
	require.NoError(t, registry.RegisterAllSubmitted())
	require.NoError(t, registry.ValidateReferences())
	var names []string
	for _, e := range registry.ListEntities() {
		names = append(names, e.Name)
	}
	require.ElementsMatch(t, []string{"waf.plans", "waf.subscriptions"}, names)
}
