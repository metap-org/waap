//go:build integration

// E2E tests against a real Postgres: reconciles all 5 entities this service owns via the real
// reconciler package, then exercises GuardedZonesBackend's guards and the custom HTTP routes
// through a real CrudService + PermissionService + a real signed JWT, mirroring the rigor
// established for the rest of this migration.
package zones_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/metap-org/metap/pkg/control"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/metap/pkg/testkit"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones"
	"github.com/metap-org/waap/m-management/zones/entities"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/m-management/zones/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.NewPostgres(t)
	return pool
}

// setupRegistry reconciles all 5 of this service's entities against pool, same boot-time order
// main.go uses, and returns the resulting MetadataRegistry.
func setupRegistry(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	ctx := context.Background()
	registry := metadata.NewMetadataRegistry()
	defs := []metadata.EntityDefinition{
		entities.DomainEntity(), entities.AccountEntity(), entities.ZoneEntity(), entities.DdosPolicyEntity(),
		entities.FirewallRuleEntity(), entities.IPAccessListEntity(), entities.PolicyVersionEntity(),
	}
	for _, def := range defs {
		require.NoError(t, registry.Register(def), "register %s", def.Name)
		_, err := reconciler.ReconcileWithScope(ctx, pool, control.PlatformTenantID, def, nil, reconciler.ScopeAllTenants)
		require.NoError(t, err, "reconcile %s", def.Name)
	}
	return registry
}

func seedAdmin(t *testing.T, pool *pgxpool.Pool, tenantID, userID uuid.UUID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, "INSERT INTO metadata.user_roles (tenant_id, user_id, role) VALUES ($1, $2, 'admin')", tenantID, userID)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM metadata.user_roles WHERE tenant_id = $1", tenantID)
	})
}

func cleanupEntityRows(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) {
	t.Cleanup(func() {
		ctx := context.Background()
		for _, table := range []string{"waf.waf_accounts", "waf.waf_policy_versions", "waf.waf_ip_access_lists", "waf.waf_firewall_rules", "waf.waf_ddos_policies", "waf.waf_zones", "waf.waf_domains"} {
			pool.Exec(ctx, "DELETE FROM "+table+" WHERE tenant_id = $1", tenantID)
		}
	})
}

func genRSAKeyPair(t *testing.T) (privatePEM string, publicKey *rsa.PublicKey) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privBytes := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	return string(privPEM), &key.PublicKey
}

func newTestState(t *testing.T, pool *pgxpool.Pool, registry *metadata.MetadataRegistry) (*httpserver.AppState, string) {
	tenantRegistry := control.NewPostgresTenantRegistry(pool)
	router := control.NewRouter(pool, control.NewRegistryCache(tenantRegistry), control.EnvStore{})
	permissions := permission.NewPermissionService(control.NewPostgresPolicyStore(router))
	var mdPtr atomic.Pointer[metadata.MetadataRegistry]
	mdPtr.Store(registry)
	privPEM, pubKey := genRSAKeyPair(t)
	state := httpserver.NewAppState(pool, registry, &mdPtr, permissions, privPEM, pubKey, router)
	return state, privPEM
}

func adminContext(tenantID, userID uuid.UUID) *permission.RequestContext {
	return &permission.RequestContext{TenantID: tenantID.String(), UserID: userID.String(), Roles: []string{"admin"}, HasRoles: true}
}

// freshApex returns a random apex domain — waf.domains.apexDomain is globally unique (not
// tenant-scoped, same as the Rust original: the reconciler's unique index is a plain
// `WHERE deleted = false` partial index, no tenant_id in its key), so every test that touches
// it needs its own value rather than a shared literal like "example.com" to stay independent of
// both other tests in the same run and leftover rows from a previous run against this same
// persistent dev database.
func freshApex(t *testing.T) string {
	t.Helper()
	return strings.ReplaceAll(uuid.New().String(), "-", "") + ".test"
}

func TestZonesServiceReconcilesAllFiveEntities(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	for _, name := range []string{"waf.domains", "waf.zones", "waf.ddos_policies", "waf.firewall_rules", "waf.ip_access_lists", "waf.policy_versions"} {
		_, ok := registry.GetEntity(name)
		assert.True(t, ok, "expected %s to be registered", name)
	}
}

func TestGuardedZonesBackendCreateZoneAutoResolvesDomainID(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1", apex)
	})

	data := crud.JsonObject{
		"hostname": jsonMarshal(t, "shop."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"),
		"protectionMode": jsonMarshal(t, "monitor"),
	}
	result, err := backend.Create(ctx, "waf.zones", data, rc, nil)
	require.NoError(t, err, "Create")
	require.NoError(t, err)
	domainIDRaw, ok := result.Data["domainId"]
	require.True(t, ok, "expected domainId to be auto-filled, got %+v", result.Data)
	require.NotEqual(t, 0, len(domainIDRaw), "expected domainId to be auto-filled, got %+v", result.Data)

	// A new Domain row with this apex must now exist.
	domains, err := state.Crud.List(ctx, "waf.domains", query.ListInput{Limit: 10, Filters: [][2]string{{"apexDomain", apex}}}, rc)
	require.NoError(t, err, "List domains")
	require.NoError(t, err)
	require.Len(t, domains.Data, 1, "expected exactly 1 domain for %s, got %+v", apex, domains)

	// Creating a second zone under the same apex must reuse the same domain, not create a
	// second one.
	data2 := crud.JsonObject{
		"hostname": jsonMarshal(t, "api."+apex), "originAddress": jsonMarshal(t, "1.2.3.5"),
		"protectionMode": jsonMarshal(t, "monitor"),
	}
	result2, err := backend.Create(ctx, "waf.zones", data2, rc, nil)
	require.NoError(t, err, "Create second zone: %v / %+v", err, result2)
	require.NoError(t, err)
	domains2, err := state.Crud.List(ctx, "waf.domains", query.ListInput{Limit: 10, Filters: [][2]string{{"apexDomain", apex}}}, rc)
	require.NoError(t, err, "expected the second zone to reuse the same domain, got %+v /", domains2)
	require.NoError(t, err)
	require.Len(t, domains2.Data, 1, "expected the second zone to reuse the same domain, got %+v / %v", domains2, err)
}

func TestGuardedZonesBackendRejectsInvalidMatchCondition(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	rc := adminContext(tenantID, userID)

	data := crud.JsonObject{
		"name": jsonMarshal(t, "bad rule"), "priority": jsonMarshal(t, 1),
		"matchCondition": jsonMarshal(t, map[string]any{"field": "bogus", "op": "eq", "value": "x"}),
		"ruleType":       jsonMarshal(t, "waf"), "action": jsonMarshal(t, "block"),
	}
	_, err := backend.Create(context.Background(), "waf.firewall_rules", data, rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "expected a ServiceError, got %v", err)
	require.EqualValues(t, 422, se.Status, "expected a 422 with a matchCondition field error, got status=%d fieldErrors=%v", se.Status, se.FieldErrors)
	require.NotNil(t, se.FieldErrors["matchCondition"], "expected a 422 with a matchCondition field error, got status=%d fieldErrors=%v", se.Status, se.FieldErrors)
}

func TestGuardedZonesBackendRejectsInvalidIPAccessListValue(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	rc := adminContext(tenantID, userID)

	data := crud.JsonObject{"type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, "not-an-ip")}
	_, err := backend.Create(context.Background(), "waf.ip_access_lists", data, rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "expected a ServiceError, got %v", err)
	require.EqualValues(t, 422, se.Status, "expected a 422 with a value field error, got status=%d fieldErrors=%v", se.Status, se.FieldErrors)
	require.NotNil(t, se.FieldErrors["value"], "expected a 422 with a value field error, got status=%d fieldErrors=%v", se.Status, se.FieldErrors)
}

func mustCreateDomain(t *testing.T, state *httpserver.AppState, rc *permission.RequestContext, apex string) string {
	data := crud.JsonObject{
		"apexDomain": jsonMarshal(t, apex), "verificationToken": jsonMarshal(t, "tok"),
		"verificationMethod": jsonMarshal(t, "dnsTxt"), "verificationStatus": jsonMarshal(t, "unverified"),
	}
	result, err := state.Crud.Create(context.Background(), "waf.domains", data, rc, nil)
	require.NoError(t, err, "create domain: %v / %+v", err, result)
	require.NoError(t, err)
	return result.ID.String()
}

// TestSyncConfigStateAction runs the declared action syncConfigState through the action registry
// (what POST /api/waf.zones/{id}/actions/syncConfigState dispatches to), with a real CrudService.
func TestSyncConfigStateAction(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1", apex)
	})

	domainID := mustCreateDomain(t, state, rc, apex)
	zoneData := crud.JsonObject{
		"hostname": jsonMarshal(t, "sync."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"),
		"protectionMode": jsonMarshal(t, "monitor"), "domainId": jsonMarshal(t, domainID),
	}
	zoneResult, err := state.Crud.Create(ctx, "waf.zones", zoneData, rc, nil)
	require.NoError(t, err, "create zone: %v / %+v", err, zoneResult)
	require.NoError(t, err)
	zoneID := zoneResult.ID

	// hasConfig starts false; no ddos_policies/firewall_rules exist yet for this zone.
	actions := crud.NewActionRegistry()
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, actions))
	sync := func() map[string]any {
		t.Helper()
		out, err := actions.Invoke(ctx, crud.ActionCall{Entity: "waf.zones", Name: "syncConfigState", ID: zoneID, RC: rc})
		require.NoError(t, err)
		var data map[string]any
		raw, err := json.Marshal(out)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &data))
		return data
	}

	// No related records yet: hasConfig false and unchanged (already correct on a fresh zone).
	data := sync()
	require.Equal(t, false, data["hasConfig"], "expected hasConfig=false changed=false on a fresh zone, got %+v", data)
	require.Equal(t, false, data["changed"], "expected hasConfig=false changed=false on a fresh zone, got %+v", data)

	// Add a DdosPolicy for this zone, then sync again: hasConfig flips to true.
	ddosData := crud.JsonObject{
		"zoneId": jsonMarshal(t, zoneID.String()), "sensitivity": jsonMarshal(t, "medium"),
		"requestRateThreshold": jsonMarshal(t, 100), "burstWindow": jsonMarshal(t, 10),
		"action": jsonMarshal(t, "log"), "priority": jsonMarshal(t, 1),
	}
	_, err = state.Crud.Create(ctx, "waf.ddos_policies", ddosData, rc, nil)
	require.NoError(t, err, "create ddos policy")
	data = sync()
	require.Equal(t, true, data["hasConfig"], "expected hasConfig=true changed=true after adding a DdosPolicy, got %+v", data)
	require.Equal(t, true, data["changed"], "expected hasConfig=true changed=true after adding a DdosPolicy, got %+v", data)

	// Idempotent: still true, but nothing changed now.
	data = sync()
	require.Equal(t, true, data["hasConfig"], "expected a repeat sync to be a no-op, got %+v", data)
	require.Equal(t, false, data["changed"], "expected a repeat sync to be a no-op, got %+v", data)
}

func jsonMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// A suspended zone is not a dead end: it can be activated again, and the activate guard still
// applies (verified domain, some protection config), so suspending never becomes a way round it.
func TestSuspendedZoneCanBeActivatedAgain(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1 OR apex_domain = $1", apex)
	})

	backend := guard.New(state.Crud)
	zone, err := backend.Create(ctx, "waf.zones", crud.JsonObject{
		"hostname": jsonMarshal(t, "back."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor"),
	}, rc, nil)
	require.NoError(t, err)
	policy, err := backend.Create(ctx, "waf.ddos_policies", crud.JsonObject{
		"zoneId": jsonMarshal(t, zone.ID.String()), "sensitivity": jsonMarshal(t, "medium"),
		"requestRateThreshold": jsonMarshal(t, 100), "burstWindow": jsonMarshal(t, 10),
		"action": jsonMarshal(t, "log"), "priority": jsonMarshal(t, 1),
	}, rc, nil)
	require.NoError(t, err)

	actions := crud.NewActionRegistry()
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, actions))
	sync := func() {
		t.Helper()
		_, err := actions.Invoke(ctx, crud.ActionCall{Entity: "waf.zones", Name: "syncConfigState", ID: zone.ID, RC: rc})
		require.NoError(t, err)
	}
	version := func() int32 {
		t.Helper()
		got, err := state.Crud.Get(ctx, "waf.zones", zone.ID, rc)
		require.NoError(t, err)
		return got.Record.Version
	}
	status := func() string {
		got, err := state.Crud.Get(ctx, "waf.zones", zone.ID, rc)
		require.NoError(t, err)
		s, _ := got.Record.Data["status"].MarshalJSON()
		return string(s)
	}
	move := func(action string) error {
		_, err := state.Crud.Transition(ctx, "waf.zones", zone.ID, action, version(), nil, rc, nil)
		return err
	}

	// Ready to activate: the domain is verified and the zone has protection config.
	sync()
	_, err = state.Crud.Update(ctx, "waf.zones", zone.ID, version(), crud.JsonObject{"verificationStatus": jsonMarshal(t, "verified")}, rc, nil)
	require.NoError(t, err)
	require.NoError(t, move("activate"))
	require.NoError(t, move("suspend"))
	assert.Equal(t, `"suspended"`, status())

	// Suspended, then activated again.
	require.NoError(t, move("activate"), "a suspended zone must be able to come back")
	assert.Equal(t, `"active"`, status())

	// The guard still applies from suspended: with no protection config left, activating is refused.
	require.NoError(t, move("suspend"))
	_, err = backend.Delete(ctx, "waf.ddos_policies", policy.ID, policy.Version, rc, nil)
	require.NoError(t, err)
	sync()
	err = move("activate")
	require.Error(t, err, "a suspended zone with no config must not activate")
	assert.Equal(t, `"suspended"`, status())
}

// A zone's rules are versioned after every change, an unchanged save records nothing, and a
// rollback restores the rules (re-creating a deleted one, removing an added one) and is itself a
// version.
func TestPolicyVersionsAndRollback(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	actions := crud.NewActionRegistry()
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, actions))
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE apex_domain = $1 OR data->>'apexDomain' = $1", apex)
	})

	zone, err := backend.Create(ctx, "waf.zones", crud.JsonObject{
		"hostname": jsonMarshal(t, "shop."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor"),
	}, rc, nil)
	require.NoError(t, err)
	zoneID := jsonMarshal(t, zone.ID.String())

	versions := func() []crud.RecordDto {
		sortDesc := "-number"
		res, err := state.Crud.List(ctx, "waf.policy_versions", query.ListInput{Limit: 50, Sort: &sortDesc, Filters: [][2]string{{"zoneId", zone.ID.String()}}}, rc)
		require.NoError(t, err)
		return res.Data
	}
	number := func(v crud.RecordDto) int {
		var n int
		require.NoError(t, json.Unmarshal(v.Data["number"], &n))
		return n
	}
	require.Empty(t, versions(), "a zone with no rules has no history")

	// First rule: a baseline (the empty zone) and the change.
	ip1, err := backend.Create(ctx, "waf.ip_access_lists", crud.JsonObject{"zoneId": zoneID, "type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, "203.0.113.5"), "enabled": jsonMarshal(t, true)}, rc, nil)
	require.NoError(t, err)
	got := versions()
	require.Len(t, got, 2)
	assert.Equal(t, 2, number(got[0]))
	assert.Equal(t, "baseline", jsonString0(got[1].Data["source"]))
	baseline := got[1]

	// Saving the same values again changes nothing, so records nothing.
	ip1, err = backend.Update(ctx, "waf.ip_access_lists", ip1.ID, ip1.Version, crud.JsonObject{"value": jsonMarshal(t, "203.0.113.5")}, rc, nil)
	require.NoError(t, err)
	assert.Len(t, versions(), 2)

	// A second entry, then delete the first: now only the second exists.
	ip2, err := backend.Create(ctx, "waf.ip_access_lists", crud.JsonObject{"zoneId": zoneID, "type": jsonMarshal(t, "whitelist"), "value": jsonMarshal(t, "198.51.100.7"), "enabled": jsonMarshal(t, true)}, rc, nil)
	require.NoError(t, err)
	_, err = backend.Delete(ctx, "waf.ip_access_lists", ip1.ID, ip1.Version, rc, nil)
	require.NoError(t, err)
	require.Len(t, versions(), 4)

	// Roll back to version 2 (only 203.0.113.5): the deleted entry comes back, the added one goes.
	var v2 crud.RecordDto
	for _, v := range versions() {
		if number(v) == 2 {
			v2 = v
		}
	}
	out, err := actions.Invoke(ctx, crud.ActionCall{Entity: "waf.policy_versions", Name: "rollback", ID: v2.ID, RC: rc})
	require.NoError(t, err)
	assert.EqualValues(t, 1, jsonNum(t, out["created"]))
	assert.EqualValues(t, 1, jsonNum(t, out["deleted"]))

	list, err := state.Crud.List(ctx, "waf.ip_access_lists", query.ListInput{Limit: 10, Filters: [][2]string{{"zoneId", zone.ID.String()}}}, rc)
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.JSONEq(t, `"203.0.113.5"`, string(list.Data[0].Data["value"]))
	_ = ip2

	after := versions()
	require.Len(t, after, 5)
	assert.Equal(t, "rollback", jsonString0(after[0].Data["source"]))
	assert.EqualValues(t, 2, jsonNum(t, after[0].Data["restoredFrom"]))

	// Rolling back to the baseline empties the zone, and hasConfig stays false.
	_, err = actions.Invoke(ctx, crud.ActionCall{Entity: "waf.policy_versions", Name: "rollback", ID: baseline.ID, RC: rc})
	require.NoError(t, err)
	list, err = state.Crud.List(ctx, "waf.ip_access_lists", query.ListInput{Limit: 10, Filters: [][2]string{{"zoneId", zone.ID.String()}}}, rc)
	require.NoError(t, err)
	assert.Empty(t, list.Data)
}

func jsonString0(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }

func jsonNum(t *testing.T, v any) float64 {
	t.Helper()
	switch x := v.(type) {
	case json.RawMessage:
		var f float64
		require.NoError(t, json.Unmarshal(x, &f))
		return f
	case float64:
		return x
	case int:
		return float64(x)
	}
	t.Fatalf("not a number: %T", v)
	return 0
}

// A temporary entry (an incident mitigation) can be made permanent by clearing expiresAt, and a
// malformed expiry is refused.
func TestIPAccessListExpiryCanBeClearedAndMustParse(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	_, err := backend.Create(ctx, "waf.ip_access_lists", crud.JsonObject{"type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, "203.0.113.9"), "expiresAt": jsonMarshal(t, "tomorrow")}, rc, nil)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok)
	require.NotNil(t, se.FieldErrors["expiresAt"])

	entry, err := backend.Create(ctx, "waf.ip_access_lists", crud.JsonObject{"type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, "203.0.113.9"), "enabled": jsonMarshal(t, true), "expiresAt": jsonMarshal(t, "2099-01-01T00:00:00Z"), "incidentId": jsonMarshal(t, "inc-1")}, rc, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, entry.Data["expiresAt"])

	cleared, err := backend.Update(ctx, "waf.ip_access_lists", entry.ID, entry.Version, crud.JsonObject{"expiresAt": json.RawMessage("null")}, rc, nil)
	require.NoError(t, err)
	got, err := state.Crud.Get(ctx, "waf.ip_access_lists", cleared.ID, rc)
	require.NoError(t, err)
	assert.Contains(t, []string{"", "null"}, string(got.Record.Data["expiresAt"]), "expiry cleared: %s", got.Record.Data["expiresAt"])
}

// Plan quotas: what a tenant may create is whatever billing-service says its plan allows. A create
// past it is refused with plan_limit_reached and nothing is removed; an unlimited plan lifts it;
// when nothing is enforced for the tenant, or billing cannot be reached, creates go through.
func TestPlanQuotasFollowWhatTheBillingDomainSays(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1 OR apex_domain = $1", apex)
	})

	// The plan lookup is whatever the billing domain says; the test swaps the answer. An empty
	// answer is a failing lookup.
	var answer atomic.Value // a billing.Resolution, or an error
	answer.Store(lookupAnswer{res: billing.Resolution{Enforced: true, Plan: billing.Plan{Name: "Small", MaxZones: 1, MaxRulesPerZone: 2}}})
	client := planLookupFunc(func(_ context.Context, tenant uuid.UUID) (billing.Resolution, error) {
		require.Equal(t, tenantID, tenant, "the lookup is asked about the caller's own tenant")
		a := answer.Load().(lookupAnswer)
		return a.res, a.err
	})
	backend := guard.New(state.Crud).WithPlanLimits(client)
	newZone := func(host string) (crud.RecordDto, error) {
		return backend.Create(ctx, "waf.zones", crud.JsonObject{"hostname": jsonMarshal(t, host+"."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor")}, rc, nil)
	}
	code := func(err error) string {
		if se, ok := crud.AsServiceError(err); ok {
			return se.Code
		}
		return ""
	}
	newRule := func(zone uuid.UUID, n int) error {
		_, err := backend.Create(ctx, "waf.firewall_rules", crud.JsonObject{
			"zoneId": jsonMarshal(t, zone.String()), "name": jsonMarshal(t, fmt.Sprintf("r%d", n)), "priority": jsonMarshal(t, n),
			"matchCondition": jsonMarshal(t, map[string]any{"field": "uri.path", "op": "eq", "value": "/x"}), "ruleType": jsonMarshal(t, "waf"), "action": jsonMarshal(t, "block"),
		}, rc, nil)
		return err
	}

	zone, err := newZone("a")
	require.NoError(t, err)
	assert.Equal(t, "plan_limit_reached", code(second(newZone("b"))))

	require.NoError(t, newRule(zone.ID, 1))
	require.NoError(t, newRule(zone.ID, 2))
	assert.Equal(t, "plan_limit_reached", code(newRule(zone.ID, 3)))

	// An unlimited plan lifts both; nothing enforced does too.
	answer.Store(lookupAnswer{res: billing.Resolution{Enforced: true, Plan: billing.Plan{Name: "Big"}}})
	require.NoError(t, newRule(zone.ID, 3))
	_, err = newZone("b")
	require.NoError(t, err)
	answer.Store(lookupAnswer{})
	_, err = newZone("c")
	require.NoError(t, err)

	// Billing down: the create goes through (fail open) instead of blocking onboarding.
	answer.Store(lookupAnswer{err: errors.New("lookup failed")})
	_, err = newZone("d")
	require.NoError(t, err)

	// The usage view reports the plan and the zone count.
	small := billing.Plan{Name: "Small", MaxZones: 1, MaxRulesPerZone: 2}
	answer.Store(lookupAnswer{res: billing.Resolution{Enforced: true, Plan: small, Plans: []billing.Plan{small}}})
	view, err := policy.Usage(ctx, state.Crud, client, rc)
	require.NoError(t, err)
	assert.True(t, view.Enforced)
	assert.Equal(t, "Small", view.Plan.Name)
	assert.EqualValues(t, 4, view.Zones)
	assert.Len(t, view.Plans, 1)
}

func second(_ crud.RecordDto, err error) error { return err }

type lookupAnswer struct {
	res billing.Resolution
	err error
}

type planLookupFunc func(context.Context, uuid.UUID) (billing.Resolution, error)

func (f planLookupFunc) Resolve(ctx context.Context, tenant uuid.UUID) (billing.Resolution, error) {
	return f(ctx, tenant)
}

// The export holds everything the caller sees for the tenant, across pages, and only that tenant's.
func TestExportTenantDataHoldsTheTenantsConfiguration(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	actions := crud.NewActionRegistry()
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, actions))
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1 OR apex_domain = $1", apex)
	})

	zone, err := backend.Create(ctx, "waf.zones", crud.JsonObject{"hostname": jsonMarshal(t, "a."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor")}, rc, nil)
	require.NoError(t, err)
	for i := 0; i < 55; i++ { // more than one page
		_, err = backend.Create(ctx, "waf.ip_access_lists", crud.JsonObject{"zoneId": jsonMarshal(t, zone.ID.String()), "type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, fmt.Sprintf("203.0.113.%d", i))}, rc, nil)
		require.NoError(t, err)
	}

	out, err := actions.Invoke(ctx, crud.ActionCall{Entity: "waf.zones", Name: "exportData", RC: rc})
	require.NoError(t, err)
	var doc struct {
		TenantID  string                       `json:"tenantId"`
		Entities  map[string][]json.RawMessage `json:"entities"`
		Truncated []string                     `json:"truncated"`
	}
	raw, _ := json.Marshal(out)
	require.NoError(t, json.Unmarshal(raw, &doc))
	assert.Equal(t, tenantID.String(), doc.TenantID)
	assert.Len(t, doc.Entities["waf.zones"], 1)
	assert.Len(t, doc.Entities["waf.ip_access_lists"], 55)
	assert.Len(t, doc.Entities["waf.accounts"], 1, "the default account")
	assert.Empty(t, doc.Truncated)
}

// What a person types is stored canonically, an invalid hostname is refused with a field error, tags
// are tidied, and the origin port must be a real one.
func TestZoneFieldsAreNormalisedOnCreateAndUpdate(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1 OR apex_domain = $1", apex)
	})
	base := func(host string, extra crud.JsonObject) crud.JsonObject {
		d := crud.JsonObject{"hostname": jsonMarshal(t, host), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor")}
		for k, v := range extra {
			d[k] = v
		}
		return d
	}

	zone, err := backend.Create(ctx, "waf.zones", base("HTTPS://Shop."+strings.ToUpper(apex)+":8443/x", crud.JsonObject{
		"tags": jsonMarshal(t, []string{" prod ", "Prod", "eu"}), "originPort": jsonMarshal(t, 8080),
	}), rc, nil)
	require.NoError(t, err)
	assert.Equal(t, "shop."+apex, jsonString0(zone.Data["hostname"]))
	assert.JSONEq(t, `["prod","eu"]`, string(zone.Data["tags"]))

	for _, bad := range []crud.JsonObject{
		base("203.0.113.5", nil), base("not a host", nil),
		base("ok."+apex, crud.JsonObject{"originPort": jsonMarshal(t, 70000)}),
		base("ok."+apex, crud.JsonObject{"tags": jsonMarshal(t, "not-a-list")}),
	} {
		_, err := backend.Create(ctx, "waf.zones", bad, rc, nil)
		se, ok := crud.AsServiceError(err)
		require.True(t, ok, "%v", err)
		assert.EqualValues(t, 422, se.Status)
	}

	// An update normalises too, and only the fields it names are touched.
	upd, err := backend.Update(ctx, "waf.zones", zone.ID, zone.Version, crud.JsonObject{"hostname": jsonMarshal(t, "WWW."+apex)}, rc, nil)
	require.NoError(t, err)
	assert.Equal(t, "www."+apex, jsonString0(upd.Data["hostname"]))
	assert.JSONEq(t, `["prod","eu"]`, string(upd.Data["tags"]))
}

// A preset gives a zone its starting protections through the guarded path (one policy version, not
// one per rule), turns hasConfig on, and applying it again adds nothing.
func TestApplyPresetIsOneVersionAndIdempotent(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)
	backend := guard.New(state.Crud)
	actions := crud.NewActionRegistry()
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, actions))
	rc := adminContext(tenantID, userID)
	ctx := context.Background()
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE data->>'apexDomain' = $1 OR apex_domain = $1", apex)
	})
	zone, err := backend.Create(ctx, "waf.zones", crud.JsonObject{"hostname": jsonMarshal(t, "a."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor")}, rc, nil)
	require.NoError(t, err)

	apply := func(key string) crud.JsonObject {
		out, err := actions.Invoke(ctx, crud.ActionCall{Entity: "waf.zones", Name: "applyPreset", ID: zone.ID, RC: rc, Input: crud.JsonObject{"preset": jsonMarshal(t, key)}})
		require.NoError(t, err)
		return out
	}
	count := func(entity string) int {
		res, err := state.Crud.List(ctx, entity, query.ListInput{Limit: 50, Filters: [][2]string{{"zoneId", zone.ID.String()}}}, rc)
		require.NoError(t, err)
		return len(res.Data)
	}
	versions := func() int {
		res, err := state.Crud.List(ctx, "waf.policy_versions", query.ListInput{Limit: 50, Filters: [][2]string{{"zoneId", zone.ID.String()}}}, rc)
		require.NoError(t, err)
		return len(res.Data)
	}

	out := apply("api")
	assert.JSONEq(t, `["DDoS policy","Block access to sensitive files","Rate limit sign-in"]`, string(out["added"]))
	assert.Equal(t, 1, count("waf.ddos_policies"))
	assert.Equal(t, 2, count("waf.firewall_rules"))
	assert.Equal(t, 2, versions(), "the zone as it was, and one version for the whole preset")
	got, err := state.Crud.Get(ctx, "waf.zones", zone.ID, rc)
	require.NoError(t, err)
	assert.Equal(t, "true", string(got.Record.Data["hasConfig"]))

	out = apply("api")
	assert.JSONEq(t, `[]`, string(out["added"]))
	assert.Equal(t, 2, count("waf.firewall_rules"))
	assert.Equal(t, 2, versions(), "nothing added, nothing recorded")

	_, err = actions.Invoke(ctx, crud.ActionCall{Entity: "waf.zones", Name: "applyPreset", ID: zone.ID, RC: rc, Input: crud.JsonObject{"preset": jsonMarshal(t, "nope")}})
	require.Error(t, err)
}
