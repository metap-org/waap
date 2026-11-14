package zonesservice

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/crud/crudmock"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
)

// Unit tests for GuardedZonesBackend's create/update guards against metap's generated
// crudmock.RecordBackend — no database, no gRPC. The real end-to-end behavior (real CrudService,
// reconciled tables, signed JWT) stays in zonesservice_postgres_test.go.

func newGuarded(t *testing.T) (*GuardedZonesBackend, *crudmock.RecordBackend) {
	t.Helper()
	inner := crudmock.NewRecordBackend(t)
	// Cross-service addresses are never dialled by these tests (only Delete uses them).
	return NewGuardedZonesBackend(inner, "127.0.0.1:1", "127.0.0.1:1"), inner
}

func testRC() *permission.RequestContext {
	return &permission.RequestContext{TenantID: uuid.NewString(), UserID: uuid.NewString(), Roles: []string{"admin"}, HasRoles: true}
}

func obj(t *testing.T, pairs map[string]any) crud.JsonObject {
	t.Helper()
	out := crud.JsonObject{}
	for k, v := range pairs {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		out[k] = b
	}
	return out
}

func TestCreateZoneReusesTheExistingDomainForItsApex(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	domainID := uuid.New()

	inner.EXPECT().List(mock.Anything, "waf.domains", mock.MatchedBy(func(in query.ListInput) bool {
		return len(in.Filters) == 1 && in.Filters[0] == [2]string{"apexDomain", "example.com"}
	}), rc).Return(crud.ListResult{Data: []crud.RecordDto{{ID: domainID}}}, nil)
	inner.EXPECT().Create(mock.Anything, "waf.zones", mock.MatchedBy(func(data crud.JsonObject) bool {
		return string(data["domainId"]) == `"`+domainID.String()+`"`
	}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)

	_, err := g.Create(context.Background(), "waf.zones", obj(t, map[string]any{"hostname": "shop.example.com"}), rc, nil)
	require.NoError(t, err)
}

func TestCreateZoneCreatesAnUnverifiedDomainWhenNoneExists(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	newDomainID := uuid.New()

	inner.EXPECT().List(mock.Anything, "waf.domains", mock.Anything, rc).Return(crud.ListResult{Data: []crud.RecordDto{}}, nil)
	inner.EXPECT().Create(mock.Anything, "waf.domains", mock.MatchedBy(func(data crud.JsonObject) bool {
		return string(data["apexDomain"]) == `"example.com"` && string(data["verificationStatus"]) == `"unverified"`
	}), rc, (*string)(nil)).Return(crud.RecordDto{ID: newDomainID}, nil)
	inner.EXPECT().Create(mock.Anything, "waf.zones", mock.MatchedBy(func(data crud.JsonObject) bool {
		return string(data["domainId"]) == `"`+newDomainID.String()+`"`
	}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)

	_, err := g.Create(context.Background(), "waf.zones", obj(t, map[string]any{"hostname": "example.com"}), rc, nil)
	require.NoError(t, err)
}

func TestCreateZoneSurfacesADomainLookupFailureAsIs(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	inner.EXPECT().List(mock.Anything, "waf.domains", mock.Anything, rc).
		Return(crud.ListResult{}, crud.NewServiceError(403, "forbidden"))

	_, err := g.Create(context.Background(), "waf.zones", obj(t, map[string]any{"hostname": "example.com"}), rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "got %v", err)
	assert.EqualValues(t, 403, se.Status)
	assert.Equal(t, "forbidden", se.Code)
}

func TestCreateZoneWithoutHostnameIsLeftToTheRealValidation(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	data := obj(t, map[string]any{"name": "no hostname"})
	inner.EXPECT().Create(mock.Anything, "waf.zones", data, rc, (*string)(nil)).
		Return(crud.RecordDto{}, crud.NewServiceErrorFields(400, "validation_failed", map[string][]string{"hostname": {"required"}}))

	_, err := g.Create(context.Background(), "waf.zones", data, rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, []string{"required"}, se.FieldErrors["hostname"])
}

func TestInvalidFirewallMatchConditionIsRejectedBeforeReachingCrud(t *testing.T) {
	g, _ := newGuarded(t) // no expectations: inner must not be called
	data := obj(t, map[string]any{"matchCondition": map[string]any{"nonsense": true}})

	for name, call := range map[string]func() (crud.RecordDto, error){
		"create": func() (crud.RecordDto, error) {
			return g.Create(context.Background(), "waf.firewall_rules", data, testRC(), nil)
		},
		"update": func() (crud.RecordDto, error) {
			return g.Update(context.Background(), "waf.firewall_rules", uuid.New(), 1, data, testRC(), nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := call()
			se, ok := crud.AsServiceError(err)
			require.True(t, ok, "got %v", err)
			assert.EqualValues(t, 422, se.Status)
			assert.Contains(t, se.FieldErrors, "matchCondition")
		})
	}
}

func TestInvalidIPAccessListValueIsRejectedBeforeReachingCrud(t *testing.T) {
	g, _ := newGuarded(t)
	data := obj(t, map[string]any{"value": "999.1.1.1/40"})

	_, err := g.Create(context.Background(), "waf.ip_access_lists", data, testRC(), nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "got %v", err)
	assert.EqualValues(t, 422, se.Status)
	assert.Equal(t, []string{"not a valid IP address or CIDR"}, se.FieldErrors["value"])

	_, err = g.Update(context.Background(), "waf.ip_access_lists", uuid.New(), 1, data, testRC(), nil)
	se, ok = crud.AsServiceError(err)
	require.True(t, ok, "got %v", err)
	assert.EqualValues(t, 422, se.Status)
}

func TestValidIPAccessListValuePassesThroughUnchanged(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	data := obj(t, map[string]any{"value": "10.0.0.0/8"})
	inner.EXPECT().Create(mock.Anything, "waf.ip_access_lists", data, rc, (*string)(nil)).
		Return(crud.RecordDto{ID: uuid.New()}, nil)

	_, err := g.Create(context.Background(), "waf.ip_access_lists", data, rc, nil)
	require.NoError(t, err)
}

func TestUnguardedEntitiesPassStraightThrough(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	data := obj(t, map[string]any{"threshold": 100})
	id := uuid.New()
	inner.EXPECT().Create(mock.Anything, "waf.ddos_policies", data, rc, (*string)(nil)).
		Return(crud.RecordDto{ID: id}, nil)
	inner.EXPECT().Update(mock.Anything, "waf.zones", id, int32(3), data, rc, (*string)(nil)).
		Return(crud.RecordDto{ID: id}, nil)

	_, err := g.Create(context.Background(), "waf.ddos_policies", data, rc, nil)
	require.NoError(t, err)
	_, err = g.Update(context.Background(), "waf.zones", id, 3, data, rc, nil)
	require.NoError(t, err)
}

// Without the caller's own token there is no identity to run the cross-service reference check
// as, so the delete is refused (fail closed) instead of skipping the check.
func TestDeleteZoneWithoutACallerTokenIsRefusedNotUnchecked(t *testing.T) {
	g, _ := newGuarded(t) // inner.Delete must not be reached
	rc := testRC()        // ForwardedBearerToken empty

	_, err := g.Delete(context.Background(), "waf.zones", uuid.New(), 1, rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "got %v", err)
	assert.EqualValues(t, 503, se.Status)
	assert.Equal(t, "reference_check_unavailable", se.Code)
}
