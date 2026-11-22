//go:build integration

// Against a real Postgres: which plan billing-service resolves for a tenant, and its internal
// route. The rows live in the operator's tenant; the asking tenant is a different one.
package billingservice_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/control"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/metap/pkg/testkit"

	"github.com/metap-org/waap/m-billing"
	"github.com/metap-org/waap/m-billing/entities"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.NewPostgres(t)
	return pool
}

func seedAdmin(t *testing.T, pool *pgxpool.Pool, tenantID, userID uuid.UUID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, "INSERT INTO metadata.user_roles (tenant_id, user_id, role) VALUES ($1, $2, 'admin')", tenantID, userID)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM metadata.user_roles WHERE tenant_id = $1", tenantID)
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

func setupRegistry(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	registry := metadata.NewMetadataRegistry()
	for _, def := range []metadata.EntityDefinition{entities.PlanEntity(), entities.SubscriptionEntity()} {
		require.NoError(t, registry.Register(def))
		_, err := reconciler.ReconcileWithScope(context.Background(), pool, control.PlatformTenantID, def, nil, reconciler.ScopeAllTenants)
		require.NoError(t, err, "reconcile %s", def.Name)
	}
	return registry
}

func obj(t *testing.T, kv map[string]any) crud.JsonObject {
	out := crud.JsonObject{}
	for k, v := range kv {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		out[k] = b
	}
	return out
}

func TestResolveFollowsSubscriptionThenDefaultThenFallback(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	operator, customer, userID := uuid.New(), uuid.New(), uuid.New()
	seedAdmin(t, pool, operator, userID)
	t.Cleanup(func() {
		for _, table := range []string{"waf.waf_subscriptions", "waf.waf_plans"} {
			pool.Exec(context.Background(), "DELETE FROM "+table+" WHERE tenant_id = $1", operator)
		}
	})
	state, _ := newTestState(t, pool, registry)
	ctx := context.Background()
	opRC := adminContext(operator, userID)
	src := &billingservice.PlanSource{Operator: operator, Backend: state.Crud}

	// No plans at all: the built-in fallback, enforced.
	res, err := src.Resolve(ctx, customer)
	require.NoError(t, err)
	assert.True(t, res.Enforced)
	assert.Equal(t, billingservice.FallbackPlan.Name, res.Plan.Name)

	small, err := state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": "Starter", "maxZones": 2, "maxRulesPerZone": 5, "isDefault": true, "enabled": true}), opRC, nil)
	require.NoError(t, err)
	big, err := state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": "Big", "maxZones": 0, "maxRulesPerZone": 0, "enabled": true}), opRC, nil)
	require.NoError(t, err)
	_, err = state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": "Retired", "maxZones": 9, "maxRulesPerZone": 9, "enabled": false}), opRC, nil)
	require.NoError(t, err)

	// The default plan applies to a customer with no subscription; a disabled plan is not offered.
	res, err = src.Resolve(ctx, customer)
	require.NoError(t, err)
	assert.Equal(t, "Starter", res.Plan.Name)
	assert.EqualValues(t, small.ID.String(), res.Plan.ID)
	require.Len(t, res.Plans, 2)
	assert.Equal(t, "Starter", res.Plans[0].Name)
	assert.Equal(t, "Big", res.Plans[1].Name, "unlimited sorts last")

	// A subscription overrides the default.
	_, err = state.Crud.Create(ctx, "waf.subscriptions", obj(t, map[string]any{"customerTenantId": customer.String(), "planId": big.ID.String()}), opRC, nil)
	require.NoError(t, err)
	res, err = src.Resolve(ctx, customer)
	require.NoError(t, err)
	assert.Equal(t, "Big", res.Plan.Name)
	assert.Zero(t, res.Plan.MaxZones)

	// The operator is never limited; a source with no operator enforces nothing.
	res, err = src.Resolve(ctx, operator)
	require.NoError(t, err)
	assert.False(t, res.Enforced)
	assert.True(t, res.Operator)
	res, err = (&billingservice.PlanSource{Backend: state.Crud}).Resolve(ctx, customer)
	require.NoError(t, err)
	assert.False(t, res.Enforced)
}

func TestInternalRouteNeedsTheSecret(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	state, _ := newTestState(t, pool, registry)
	operator := uuid.New()
	t.Setenv("OPERATOR_TENANT_ID", operator.String())
	t.Setenv("BILLING_INTERNAL_SECRET", "s3cret")

	r := chi.NewRouter()
	billingservice.MountRoutes(r, state)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	get := func(secret, tenant string) int {
		req, _ := http.NewRequest("GET", srv.URL+"/internal/plans/resolve?tenantId="+tenant, nil)
		if secret != "" {
			req.Header.Set("X-Internal-Secret", secret)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return resp.StatusCode
	}
	tenant := uuid.NewString()
	assert.Equal(t, 401, get("", tenant), "no secret")
	assert.Equal(t, 401, get("wrong", tenant))
	assert.Equal(t, 400, get("s3cret", "not-a-uuid"))
	assert.Equal(t, 200, get("s3cret", tenant))

	t.Setenv("BILLING_INTERNAL_SECRET", "")
	assert.Equal(t, 503, get("anything", tenant), "no secret configured: refuse rather than serve open")
}
