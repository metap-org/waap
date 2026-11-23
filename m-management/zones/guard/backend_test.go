package guard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/crud/crudmock"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Unit tests for Backend's create/update guards against metap's generated
// crudmock.RecordBackend — no database, no sibling service. The real end-to-end behavior (real CrudService,
// reconciled tables, signed JWT) stays in zonesservice_postgres_test.go.

func newGuarded(t *testing.T) (*Backend, *crudmock.RecordBackend) {
	t.Helper()
	inner := crudmock.NewRecordBackend(t)
	return New(inner), inner
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
	expectDefaultAccount(inner, rc)
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

// A zone always lands in an account; these tests are about something else, so the tenant already
// has its default one.
func expectDefaultAccount(inner *crudmock.RecordBackend, rc *permission.RequestContext) {
	inner.EXPECT().List(mock.Anything, "waf.accounts", mock.Anything, rc).
		Return(crud.ListResult{Data: []crud.RecordDto{{ID: uuid.New()}}}, nil)
}

// A zone added under a domain that is already verified starts verified: the verify action only
// cascades onto zones that exist when it runs, so without this the new zone could never activate.
func TestCreateZoneUnderAVerifiedDomainStartsVerified(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	expectDefaultAccount(inner, rc)
	domainID := uuid.New()

	inner.EXPECT().List(mock.Anything, "waf.domains", mock.Anything, rc).Return(crud.ListResult{Data: []crud.RecordDto{{
		ID: domainID, Data: obj(t, map[string]any{"verificationStatus": "verified"}),
	}}}, nil)
	inner.EXPECT().Create(mock.Anything, "waf.zones", mock.MatchedBy(func(data crud.JsonObject) bool {
		return string(data["verificationStatus"]) == `"verified"`
	}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)

	_, err := g.Create(context.Background(), "waf.zones", obj(t, map[string]any{"hostname": "www.example.com"}), rc, nil)
	require.NoError(t, err)
}

func TestCreateZoneUnderAnUnverifiedDomainDoesNotMarkItVerified(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	expectDefaultAccount(inner, rc)

	inner.EXPECT().List(mock.Anything, "waf.domains", mock.Anything, rc).Return(crud.ListResult{Data: []crud.RecordDto{{
		ID: uuid.New(), Data: obj(t, map[string]any{"verificationStatus": "unverified"}),
	}}}, nil)
	inner.EXPECT().Create(mock.Anything, "waf.zones", mock.MatchedBy(func(data crud.JsonObject) bool {
		_, set := data["verificationStatus"]
		return !set
	}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)

	_, err := g.Create(context.Background(), "waf.zones", obj(t, map[string]any{"hostname": "www.example.com"}), rc, nil)
	require.NoError(t, err)
}

func TestCreateZoneCreatesAnUnverifiedDomainWhenNoneExists(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	expectDefaultAccount(inner, rc)
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

// A zone that scan jobs or incidents still point at cannot be deleted, and the answer names what
// blocks it. Both entities are read through the same backend as the caller.
func TestDeleteZoneIsBlockedByWhatStillReferencesIt(t *testing.T) {
	for _, blocking := range []string{"waf.scan_jobs", "waf.incidents"} {
		g, inner := newGuarded(t) // inner.Delete must not be reached
		rc := testRC()
		for _, entity := range []string{"waf.scan_jobs", "waf.incidents"} {
			result := crud.ListResult{}
			if entity == blocking {
				result.Data = []crud.RecordDto{{ID: uuid.New()}}
			}
			inner.EXPECT().List(mock.Anything, entity, mock.Anything, rc).Return(result, nil).Maybe()
		}

		_, err := g.Delete(context.Background(), "waf.zones", uuid.New(), 1, rc, nil)
		se, ok := crud.AsServiceError(err)
		require.True(t, ok, "got %v", err)
		assert.EqualValues(t, 409, se.Status, blocking)
		assert.Equal(t, "record_referenced", se.Code, blocking)
		assert.Contains(t, se.Message, blocking)
	}
}

// With nothing pointing at it, the zone is deleted.
func TestDeleteZoneGoesThroughWhenNothingReferencesIt(t *testing.T) {
	g, inner := newGuarded(t)
	rc := testRC()
	id := uuid.New()
	inner.EXPECT().List(mock.Anything, mock.Anything, mock.Anything, rc).Return(crud.ListResult{}, nil)
	inner.EXPECT().Delete(mock.Anything, "waf.zones", id, int32(1), rc, (*string)(nil)).Return(crud.RecordDto{ID: id}, nil)

	_, err := g.Delete(context.Background(), "waf.zones", id, 1, rc, nil)
	require.NoError(t, err)
}

// The account of a rule is its zone's, never the client's: an accountId in the request is dropped
// when the zone cannot be read or has none, and replaced by the zone's when it has one (audit 08 #7).
func TestRuleAccountIsTheZonesNotTheClients(t *testing.T) {
	rc := testRC()
	zoneID := uuid.New()
	zoneAccount := uuid.New()
	forged := uuid.New()
	rule := func() crud.JsonObject {
		return obj(t, map[string]any{"zoneId": zoneID.String(), "accountId": forged.String(), "action": "block"})
	}

	t.Run("zone not readable", func(t *testing.T) {
		inner := crudmock.NewRecordBackend(t)
		g := &accountGuard{passthrough{inner}}
		inner.EXPECT().Get(mock.Anything, "waf.zones", zoneID, rc).Return(crud.GetResult{}, assert.AnError)
		inner.EXPECT().Create(mock.Anything, "waf.firewall_rules", mock.MatchedBy(func(data crud.JsonObject) bool {
			_, has := data["accountId"]
			return !has
		}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)
		_, err := g.Create(context.Background(), "waf.firewall_rules", rule(), rc, nil)
		require.NoError(t, err)
	})

	t.Run("zone with no account", func(t *testing.T) {
		inner := crudmock.NewRecordBackend(t)
		g := &accountGuard{passthrough{inner}}
		inner.EXPECT().Get(mock.Anything, "waf.zones", zoneID, rc).
			Return(crud.GetResult{Record: crud.RecordDto{Data: crud.JsonObject{}}}, nil)
		inner.EXPECT().Create(mock.Anything, "waf.firewall_rules", mock.MatchedBy(func(data crud.JsonObject) bool {
			_, has := data["accountId"]
			return !has
		}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)
		_, err := g.Create(context.Background(), "waf.firewall_rules", rule(), rc, nil)
		require.NoError(t, err)
	})

	t.Run("zone with an account", func(t *testing.T) {
		inner := crudmock.NewRecordBackend(t)
		g := &accountGuard{passthrough{inner}}
		inner.EXPECT().Get(mock.Anything, "waf.zones", zoneID, rc).
			Return(crud.GetResult{Record: crud.RecordDto{Data: obj(t, map[string]any{"accountId": zoneAccount.String()})}}, nil)
		inner.EXPECT().Create(mock.Anything, "waf.firewall_rules", mock.MatchedBy(func(data crud.JsonObject) bool {
			return string(data["accountId"]) == `"`+zoneAccount.String()+`"`
		}), rc, (*string)(nil)).Return(crud.RecordDto{ID: uuid.New()}, nil)
		_, err := g.Create(context.Background(), "waf.firewall_rules", rule(), rc, nil)
		require.NoError(t, err)
	})
}
