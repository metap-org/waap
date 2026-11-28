package entities

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/model"
)

// Who may run each zone action is a decision: this pins it, so a change shows up in review.
func TestWhoMayRunTheZoneActions(t *testing.T) {
	var got []string
	for _, def := range []struct {
		name    string
		actions []string
	}{
		{model.Zones, describe(ZoneEntity().Actions)},
		{model.Domains, describe(DomainEntity().Actions)},
		{model.PolicyVersions, describe(PolicyVersionEntity().Actions)},
	} {
		for _, a := range def.actions {
			got = append(got, def.name+" "+a)
		}
	}
	sort.Strings(got)
	want := `waf.domains verifyDns update
waf.policy_versions rollback read
waf.zones applyPreset read
waf.zones exportData read
waf.zones planUsage read
waf.zones probeOrigin create
waf.zones resolveHostname create
waf.zones syncConfigState read
waf.zones testOrigin update
waf.zones validatePolicy read
waf.zones verifyDns update`
	assert.Equal(t, strings.Split(want, "\n"), got, "an action that reaches the network or changes the zone by itself needs more than read")
}

func describe(actions []metadata.EntityAction) []string {
	var out []string
	for _, a := range actions {
		who := a.Requires
		if len(a.Roles) > 0 {
			who = "roles=" + strings.Join(a.Roles, ",")
		}
		out = append(out, a.Name+" "+who)
	}
	return out
}
