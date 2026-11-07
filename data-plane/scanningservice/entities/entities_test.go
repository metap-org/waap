package entities

import (
	"testing"

	"github.com/metap-org/metap/pkg/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnsExactlyItsOwnTwoEntities(t *testing.T) {
	registry := metadata.NewMetadataRegistry()
	require.NoError(t, registry.RegisterAllSubmitted(), "RegisterAllSubmitted")
	require.NoError(t, registry.ValidateReferences(), "ValidateReferences")

	var names []string
	for _, e := range registry.ListEntities() {
		names = append(names, e.Name)
	}
	for _, want := range []string{"waf.scan_jobs", "waf.scan_findings"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		assert.True(t, found, "missing entity: %s", want)
	}
	assert.Len(t, names, 2, "unexpected entity count: %v", names)
}
