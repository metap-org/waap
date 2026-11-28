//go:build integration

// Lookups that rest on a list filter. Each of these once returned every record of the tenant,
// because the filter's field was not declared in the list view and was silently dropped.
package zones_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/zones"
	"github.com/metap-org/waap/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// txtResolver answers the ownership lookup of one domain with its token.
func txtResolver(t *testing.T, name, token string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answers := []map[string]string{}
		if strings.TrimSuffix(r.URL.Query().Get("name"), ".") == name {
			answers = append(answers, map[string]string{"data": `"` + token + `"`})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Answer": answers})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// Proving one domain verifies the zones under it, and no other zone of the tenant.
func TestVerifyingADomainLeavesOtherDomainsZonesUnverified(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	state, _ := newTestState(t, pool, registry)
	api := zones.Guarded(state.Crud, nil, state.Router)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	zone := func(host string) crud.RecordDto {
		r, err := api.Create(ctx, model.Zones, crud.JsonObject{"hostname": jsonMarshal(t, host),
			"originAddress": jsonMarshal(t, "203.0.114.10"), "protectionMode": jsonMarshal(t, "monitor")}, rc, nil)
		require.NoError(t, err)
		return r
	}
	mine, notMine := freshApex(t), freshApex(t)
	under, other := zone("a."+mine), zone("a."+notMine)

	domain, err := state.Crud.Get(ctx, model.Domains, uuid.MustParse(text(under, model.FieldDomainID)), rc)
	require.NoError(t, err)
	actions := crud.NewActionRegistry()
	cfg := zones.Config{DoHURL: txtResolver(t, "_waf-verify."+mine, text(domain.Record, model.FieldVerificationToken))}
	require.NoError(t, zones.RegisterActions(cfg, nil)(state, actions))
	_, err = actions.Invoke(ctx, crud.ActionCall{Entity: model.Domains, Name: model.ActionVerifyDNS, ID: domain.Record.ID, Input: crud.JsonObject{}, RC: rc})
	require.NoError(t, err)

	status := func(id uuid.UUID) string {
		got, err := state.Crud.Get(ctx, model.Zones, id, rc)
		require.NoError(t, err)
		return text(got.Record, model.FieldVerificationStatus)
	}
	assert.Equal(t, model.VerificationVerified, status(under.ID), "the zone under the proved domain")
	assert.NotEqual(t, model.VerificationVerified, status(other.ID), "a zone under a domain nobody proved")
}

// A zone with no account named goes to the tenant's default account, not to whichever account
// happens to be listed first.
func TestANewZoneGoesToTheDefaultAccount(t *testing.T) {
	f := newOwnershipFixture(t)
	other, err := f.create(model.Accounts, map[string]any{model.FieldName: "Client A", model.FieldSlug: "client-a"})
	require.NoError(t, err)
	zone, err := f.create(model.Zones, zoneData("a."+freshApex(t), nil))
	require.NoError(t, err)
	assert.NotEqual(t, other.ID.String(), text(zone, model.FieldAccountID))

	account, err := f.direct.Get(context.Background(), model.Accounts, uuid.MustParse(text(zone, model.FieldAccountID)), f.rc)
	require.NoError(t, err)
	assert.Equal(t, "default", text(account.Record, model.FieldSlug))

	// And an account with no zone running can be closed, whatever runs in the others.
	got, err := f.direct.Get(context.Background(), model.Zones, zone.ID, f.rc)
	require.NoError(t, err)
	_, err = f.direct.Update(context.Background(), model.Zones, zone.ID, got.Record.Version, crud.JsonObject{"hasConfig": jsonMarshal(t, true), "verificationStatus": jsonMarshal(t, "verified")}, f.rc, nil)
	require.NoError(t, err)
	got, err = f.direct.Get(context.Background(), model.Zones, zone.ID, f.rc)
	require.NoError(t, err)
	_, err = f.api.Transition(context.Background(), model.Zones, zone.ID, model.ZoneActivate, got.Record.Version, nil, f.rc, nil)
	require.NoError(t, err)
	_, err = f.api.Transition(context.Background(), model.Accounts, other.ID, "close", other.Version, nil, f.rc, nil)
	assert.NoError(t, err, "no zone of this account is running")
}
