package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/metadata"

	_ "github.com/metap-org/waap/m-alerting/entities"
	_ "github.com/metap-org/waap/m-billing/entities"
	_ "github.com/metap-org/waap/m-scanning/entities"
	_ "github.com/metap-org/waap/m-zones/entities"
)

// Every entity pattern must match something, or Apply would refuse at deploy time.
func TestPersonasMatchRegisteredEntities(t *testing.T) {
	require.NoError(t, Personas().Validate())
	reg := metadata.NewMetadataRegistry()
	require.NoError(t, reg.RegisterAllSubmitted())
	names := map[string]bool{}
	for _, d := range reg.Definitions() {
		names[d.Name] = true
	}
	for _, g := range Personas().Grants {
		for _, e := range g.Entities {
			if e == "waf.*" {
				require.NotEmpty(t, names)
				continue
			}
			require.True(t, names[e], "grant for %s names unknown entity %s", g.Role, e)
		}
	}
}
