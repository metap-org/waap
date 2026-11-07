//go:build integration

// E2E tests against a real Postgres: reconciles all 5 entities this service owns via the real
// reconciler package, then exercises GuardedZonesBackend's guards and the custom HTTP routes
// through a real CrudService + PermissionService + a real signed JWT, mirroring the rigor
// established for the rest of this migration.
package zonesservice_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/metap-org/metap/pkg/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/metap-org/metap/pkg/control"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/peripherals"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/reconciler"

	"github.com/metap-org/waap/data-plane/zonesservice"
	"github.com/metap-org/waap/data-plane/zonesservice/entities"
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
		entities.DomainEntity(), entities.ZoneEntity(), entities.DdosPolicyEntity(),
		entities.FirewallRuleEntity(), entities.IPAccessListEntity(),
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
		for _, table := range []string{"waf.waf_ip_access_lists", "waf.waf_firewall_rules", "waf.waf_ddos_policies", "waf.waf_zones", "waf.waf_domains"} {
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
	for _, name := range []string{"waf.domains", "waf.zones", "waf.ddos_policies", "waf.firewall_rules", "waf.ip_access_lists"} {
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
	backend := zonesservice.NewGuardedZonesBackend(state.Crud, "", "localhost:1", "localhost:1")
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
	require.True(t, result.IsOk, "Create failed: status=%d code=%s msg=%v fieldErrors=%v", result.Status, result.ErrorCode, result.Message, result.FieldErrors)
	domainIDRaw, ok := result.Data.Data["domainId"]
	require.True(t, ok, "expected domainId to be auto-filled, got %+v", result.Data.Data)
	require.NotEqual(t, 0, len(domainIDRaw), "expected domainId to be auto-filled, got %+v", result.Data.Data)

	// A new Domain row with this apex must now exist.
	domains, err := state.Crud.List(ctx, "waf.domains", query.ListInput{Limit: 10, Filters: [][2]string{{"apexDomain", apex}}}, rc)
	require.NoError(t, err, "List domains")
	require.True(t, domains.IsOk, "expected exactly 1 domain for %s, got %+v", apex, domains)
	require.Len(t, domains.Data, 1, "expected exactly 1 domain for %s, got %+v", apex, domains)

	// Creating a second zone under the same apex must reuse the same domain, not create a
	// second one.
	data2 := crud.JsonObject{
		"hostname": jsonMarshal(t, "api."+apex), "originAddress": jsonMarshal(t, "1.2.3.5"),
		"protectionMode": jsonMarshal(t, "monitor"),
	}
	result2, err := backend.Create(ctx, "waf.zones", data2, rc, nil)
	require.NoError(t, err, "Create second zone: %v / %+v", err, result2)
	require.True(t, result2.IsOk, "Create second zone: %v / %+v", err, result2)
	domains2, err := state.Crud.List(ctx, "waf.domains", query.ListInput{Limit: 10, Filters: [][2]string{{"apexDomain", apex}}}, rc)
	require.NoError(t, err, "expected the second zone to reuse the same domain, got %+v /", domains2)
	require.True(t, domains2.IsOk, "expected the second zone to reuse the same domain, got %+v / %v", domains2, err)
	require.Len(t, domains2.Data, 1, "expected the second zone to reuse the same domain, got %+v / %v", domains2, err)
}

func TestGuardedZonesBackendRejectsInvalidMatchCondition(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := zonesservice.NewGuardedZonesBackend(state.Crud, "", "localhost:1", "localhost:1")
	rc := adminContext(tenantID, userID)

	data := crud.JsonObject{
		"name": jsonMarshal(t, "bad rule"), "priority": jsonMarshal(t, 1),
		"matchCondition": jsonMarshal(t, map[string]any{"field": "bogus", "op": "eq", "value": "x"}),
		"ruleType":       jsonMarshal(t, "waf"), "action": jsonMarshal(t, "block"),
	}
	result, err := backend.Create(context.Background(), "waf.firewall_rules", data, rc, nil)
	require.NoError(t, err, "Create")
	require.False(t, result.IsOk, "expected invalid matchCondition to be rejected")
	require.EqualValues(t, 422, result.Status, "expected a 422 with a matchCondition field error, got status=%d fieldErrors=%v", result.Status, result.FieldErrors)
	require.NotNil(t, result.FieldErrors["matchCondition"], "expected a 422 with a matchCondition field error, got status=%d fieldErrors=%v", result.Status, result.FieldErrors)
}

func TestGuardedZonesBackendRejectsInvalidIPAccessListValue(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	backend := zonesservice.NewGuardedZonesBackend(state.Crud, "", "localhost:1", "localhost:1")
	rc := adminContext(tenantID, userID)

	data := crud.JsonObject{"type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, "not-an-ip")}
	result, err := backend.Create(context.Background(), "waf.ip_access_lists", data, rc, nil)
	require.NoError(t, err, "Create")
	require.False(t, result.IsOk, "expected invalid IP/CIDR to be rejected")
	require.EqualValues(t, 422, result.Status, "expected a 422 with a value field error, got status=%d fieldErrors=%v", result.Status, result.FieldErrors)
	require.NotNil(t, result.FieldErrors["value"], "expected a 422 with a value field error, got status=%d fieldErrors=%v", result.Status, result.FieldErrors)
}

func mustCreateDomain(t *testing.T, state *httpserver.AppState, rc *permission.RequestContext, apex string) string {
	data := crud.JsonObject{
		"apexDomain": jsonMarshal(t, apex), "verificationToken": jsonMarshal(t, "tok"),
		"verificationMethod": jsonMarshal(t, "dnsTxt"), "verificationStatus": jsonMarshal(t, "unverified"),
	}
	result, err := state.Crud.Create(context.Background(), "waf.domains", data, rc, nil)
	require.NoError(t, err, "create domain: %v / %+v", err, result)
	require.True(t, result.IsOk, "create domain: %v / %+v", err, result)
	return result.Data.ID.String()
}

// TestSyncConfigStateOverRealHTTP exercises the full chi routing + handler + CrudService stack
// over a real network socket with a real signed JWT, mirroring httpserver's own e2e pattern.
func TestSyncConfigStateOverRealHTTP(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
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
	require.True(t, zoneResult.IsOk, "create zone: %v / %+v", err, zoneResult)
	zoneID := zoneResult.Data.ID

	// hasConfig starts false; no ddos_policies/firewall_rules exist yet for this zone.
	r := chi.NewRouter()
	zonesservice.MountRoutes(r, state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: r}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	baseURL := "http://" + listener.Addr().String()

	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	// No related records yet — sync should report hasConfig: false, changed: false (it's
	// already correct on a freshly-created zone).
	resp := doPost(t, baseURL+"/api/waf.zones/"+zoneID.String()+"/sync-config-state", token)
	body := decodeJSON(t, resp)
	data := body["data"].(map[string]any)
	require.Equal(t, false, data["hasConfig"], "expected hasConfig=false changed=false on a fresh zone, got %+v", data)
	require.Equal(t, false, data["changed"], "expected hasConfig=false changed=false on a fresh zone, got %+v", data)

	// Add a DdosPolicy for this zone, then sync again — hasConfig should flip to true.
	ddosData := crud.JsonObject{
		"zoneId": jsonMarshal(t, zoneID.String()), "sensitivity": jsonMarshal(t, "medium"),
		"requestRateThreshold": jsonMarshal(t, 100), "burstWindow": jsonMarshal(t, 10),
		"action": jsonMarshal(t, "log"), "priority": jsonMarshal(t, 1),
	}
	_, err = state.Crud.Create(ctx, "waf.ddos_policies", ddosData, rc, nil)
	require.NoError(t, err, "create ddos policy")

	resp = doPost(t, baseURL+"/api/waf.zones/"+zoneID.String()+"/sync-config-state", token)
	body = decodeJSON(t, resp)
	data = body["data"].(map[string]any)
	require.Equal(t, true, data["hasConfig"], "expected hasConfig=true changed=true after adding a DdosPolicy, got %+v", data)
	require.Equal(t, true, data["changed"], "expected hasConfig=true changed=true after adding a DdosPolicy, got %+v", data)

	// Calling it again is idempotent: still true, but changed=false now since it's already
	// correct.
	resp = doPost(t, baseURL+"/api/waf.zones/"+zoneID.String()+"/sync-config-state", token)
	body = decodeJSON(t, resp)
	data = body["data"].(map[string]any)
	require.Equal(t, true, data["hasConfig"], "expected a repeat sync to be a no-op, got %+v", data)
	require.Equal(t, false, data["changed"], "expected a repeat sync to be a no-op, got %+v", data)
}

func doPost(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(""))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var v map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

func jsonMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
