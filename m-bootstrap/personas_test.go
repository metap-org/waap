package bootstrap

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/m-management/analytics"

	_ "github.com/metap-org/waap/m-management/alerting/entities"
	_ "github.com/metap-org/waap/m-management/billing/entities"
	_ "github.com/metap-org/waap/m-management/scanning/entities"
	_ "github.com/metap-org/waap/m-management/zones/entities"
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

// The Analytics read model is not stored by any service, so the bootstrap must register it itself:
// without it `waf.*` would not reach it and only an admin could read security events (the read
// model checks the permission).
func TestSecurityEventsCanBeGrantedToThePersonas(t *testing.T) {
	reg := metadata.NewMetadataRegistry()
	require.NoError(t, reg.RegisterAllSubmitted())
	_, ok := reg.GetEntity("waf.security_events")
	require.True(t, ok, "the read model is known to the bootstrap")
	var viewer bool
	for _, g := range Personas().Grants {
		if g.Role != RoleViewer {
			continue
		}
		for _, e := range g.Entities {
			viewer = viewer || e == "waf.security_events"
		}
	}
	require.True(t, viewer, "a Viewer reads the analytics")
}

// A role held only on one zone sees that zone's events through the events resource, not the entity.
func TestScopedAnalystsAndViewersMaySeeTheirZonesEvents(t *testing.T) {
	require.NoError(t, Personas().Validate())
	for _, role := range []string{RoleSecurityAnalyst, RoleViewer} {
		var granted bool
		for _, g := range Personas().Grants {
			if g.Role == role && slices.Contains(g.Resources, analytics.EventsResource) {
				granted = true
			}
		}
		require.True(t, granted, role)
	}
}
