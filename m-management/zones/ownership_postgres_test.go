//go:build integration

// Domain ownership is proved by waf.domains.verifyDns and nothing else: these tests go through the
// guarded backend (what /api/{entity} serves) as a tenant admin, the most a customer can be.
package zones_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones"
	"github.com/metap-org/waap/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ownershipFixture struct {
	t   *testing.T
	api crud.RecordBackend
	rc  *permission.RequestContext
	// direct is the unguarded service, the path verifyDns itself writes through.
	direct crud.RecordBackend
	create func(entity string, data map[string]any) (crud.RecordDto, error)
	update func(r crud.RecordDto, entity string, data map[string]any) (crud.RecordDto, error)
}

func newOwnershipFixture(t *testing.T) ownershipFixture {
	t.Helper()
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	state, _ := newTestState(t, pool, registry)
	api := zones.Guarded(state.Crud, nil, state.Router)
	rc := adminContext(tenantID, userID)
	object := func(data map[string]any) crud.JsonObject {
		out := crud.JsonObject{}
		for k, v := range data {
			out[k] = jsonMarshal(t, v) // a json.RawMessage or nil goes through as read
		}
		return out
	}
	return ownershipFixture{
		t: t, api: api, rc: rc, direct: state.Crud,
		create: func(entity string, data map[string]any) (crud.RecordDto, error) {
			return api.Create(context.Background(), entity, object(data), rc, nil)
		},
		update: func(r crud.RecordDto, entity string, data map[string]any) (crud.RecordDto, error) {
			return api.Update(context.Background(), entity, r.ID, r.Version, object(data), rc, nil)
		},
	}
}

func domainData(apex string, extra map[string]any) map[string]any {
	data := map[string]any{
		model.FieldApexDomain: apex, model.FieldVerificationToken: "client-chosen-token",
		model.FieldVerificationMethod: model.VerificationMethodDNSTxt, model.FieldVerificationStatus: model.VerificationUnverified,
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func zoneData(hostname string, extra map[string]any) map[string]any {
	data := map[string]any{
		model.FieldHostname: hostname, "originAddress": "203.0.114.10", "protectionMode": "monitor",
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

// assertRefused: err is a 422 naming field.
func assertRefused(t *testing.T, err error, field string) {
	t.Helper()
	se, ok := crud.AsServiceError(err)
	if !assert.True(t, ok, "expected a refusal of %s, got err=%v", field, err) {
		return
	}
	assert.EqualValues(t, 422, se.Status)
	assert.Contains(t, se.FieldErrors, field)
}

func text(r crud.RecordDto, field string) string { return jsonString0(r.Data[field]) }

func TestAClientCannotCreateAVerifiedDomainOrChooseItsToken(t *testing.T) {
	f := newOwnershipFixture(t)

	_, err := f.create(model.Domains, domainData(freshApex(t), map[string]any{model.FieldVerificationStatus: model.VerificationVerified}))
	assertRefused(t, err, model.FieldVerificationStatus)

	// A chosen token would let any TXT value already published under _waf-verify prove ownership.
	made, err := f.create(model.Domains, domainData(freshApex(t), nil))
	require.NoError(t, err)
	assert.NotEqual(t, "client-chosen-token", text(made, model.FieldVerificationToken))
	assert.Contains(t, text(made, model.FieldVerificationToken), "waf-verify-")
	assert.Equal(t, model.VerificationUnverified, text(made, model.FieldVerificationStatus))
}

func TestAClientCannotChangeWhatProvesADomain(t *testing.T) {
	f := newOwnershipFixture(t)
	made, err := f.create(model.Domains, domainData(freshApex(t), nil))
	require.NoError(t, err)

	for field, value := range map[string]any{
		model.FieldVerificationStatus: model.VerificationVerified,
		model.FieldVerificationToken:  "client-chosen-token",
		// Renaming a verified domain would carry the proof to a name nobody proved.
		model.FieldApexDomain: freshApex(t),
	} {
		_, err := f.update(made, model.Domains, map[string]any{field: value})
		assertRefused(t, err, field)
	}
	_, err = f.update(made, model.Domains, map[string]any{model.FieldVerificationStatus: nil})
	assertRefused(t, err, model.FieldVerificationStatus)

	// A form that sends the record back unchanged still saves.
	kept, err := f.update(made, model.Domains, map[string]any{
		model.FieldApexDomain: text(made, model.FieldApexDomain), model.FieldVerificationToken: text(made, model.FieldVerificationToken),
		model.FieldVerificationStatus: model.VerificationUnverified, model.FieldVerificationMethod: "httpFile",
	})
	require.NoError(t, err)
	assert.Equal(t, "httpFile", text(kept, model.FieldVerificationMethod))
}

func TestAClientCannotMakeAZoneVerifiedOrMoveItOffItsDomain(t *testing.T) {
	f := newOwnershipFixture(t)
	apex := freshApex(t)

	_, err := f.create(model.Zones, zoneData("a."+apex, map[string]any{model.FieldVerificationStatus: model.VerificationVerified}))
	assertRefused(t, err, model.FieldVerificationStatus)

	zone, err := f.create(model.Zones, zoneData("a."+apex, nil))
	require.NoError(t, err)
	assert.NotEqual(t, model.VerificationVerified, text(zone, model.FieldVerificationStatus))

	_, err = f.update(zone, model.Zones, map[string]any{model.FieldVerificationStatus: model.VerificationVerified})
	assertRefused(t, err, model.FieldVerificationStatus)
	// A verified zone must not be renamed to a name under a domain it was not verified for.
	_, err = f.update(zone, model.Zones, map[string]any{model.FieldHostname: "a." + freshApex(t)})
	assertRefused(t, err, model.FieldHostname)
	other, err := f.create(model.Domains, domainData(freshApex(t), nil))
	require.NoError(t, err)
	_, err = f.update(zone, model.Zones, map[string]any{model.FieldDomainID: other.ID.String()})
	assertRefused(t, err, model.FieldDomainID)

	// A transition's data is a write too.
	_, err = f.api.Transition(context.Background(), model.Zones, zone.ID, "activate", zone.Version,
		crud.JsonObject{model.FieldVerificationStatus: jsonMarshal(t, model.VerificationVerified)}, f.rc, nil)
	assertRefused(t, err, model.FieldVerificationStatus)

	// Renaming under the same domain, and sending unchanged values back, still work.
	renamed, err := f.update(zone, model.Zones, map[string]any{
		model.FieldHostname: "b." + apex, model.FieldDomainID: text(zone, model.FieldDomainID),
		// as read: this zone has no status stored
		model.FieldVerificationStatus: zone.Data[model.FieldVerificationStatus],
	})
	require.NoError(t, err)
	assert.Equal(t, "b."+apex, text(renamed, model.FieldHostname))
}

// proveDomain marks zone's domain verified through the unguarded service.
func proveDomain(t *testing.T, f ownershipFixture, zone crud.RecordDto) {
	t.Helper()
	id := uuid.MustParse(text(zone, model.FieldDomainID))
	domain, err := f.direct.Get(context.Background(), model.Domains, id, f.rc)
	require.NoError(t, err)
	_, err = f.direct.Update(context.Background(), model.Domains, id, domain.Record.Version,
		crud.JsonObject{model.FieldVerificationStatus: jsonMarshal(t, model.VerificationVerified)}, f.rc, nil)
	require.NoError(t, err)
}

// A domain that has really been proved (written the way verifyDns writes it) still passes the
// proof on to new zones.
func TestAZoneUnderAProvedDomainStartsVerified(t *testing.T) {
	f := newOwnershipFixture(t)
	apex := freshApex(t)
	first, err := f.create(model.Zones, zoneData("a."+apex, nil))
	require.NoError(t, err)
	proveDomain(t, f, first)

	second, err := f.create(model.Zones, zoneData("b."+apex, map[string]any{model.FieldVerificationStatus: model.VerificationUnverified}))
	require.NoError(t, err)
	assert.Equal(t, model.VerificationVerified, text(second, model.FieldVerificationStatus))
}
