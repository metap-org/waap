//go:build integration

// Against a real Postgres: which plan billing-service resolves for a tenant, and its internal
// route. The rows live in the operator's tenant; the asking tenant is a different one.
package billing_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/metap-org/metap/pkg/permission/policypg"
	"github.com/metap-org/metap/pkg/secrets"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/metap/pkg/tenancy"
	"github.com/metap-org/metap/pkg/testkit"
	"github.com/metap-org/metap/pkg/web"

	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/billing/entities"
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

func newTestState(t *testing.T, pool *pgxpool.Pool, registry *metadata.MetadataRegistry) (*web.State, string) {
	tenantRegistry := tenancy.NewPostgresTenantRegistry(pool)
	router := tenancy.NewRouter(pool, tenancy.NewRegistryCache(tenantRegistry), secrets.EnvStore{})
	permissions := permission.NewPermissionService(policypg.NewPostgresPolicyStore(router))
	var mdPtr atomic.Pointer[metadata.MetadataRegistry]
	mdPtr.Store(registry)
	privPEM, pubKey := genRSAKeyPair(t)
	state := web.NewState(pool, registry, &mdPtr, permissions, privPEM, pubKey, router)
	return state, privPEM
}

func adminContext(tenantID, userID uuid.UUID) *permission.RequestContext {
	return &permission.RequestContext{TenantID: tenantID.String(), UserID: userID.String(), Roles: []string{"admin"}, HasRoles: true}
}

func setupRegistry(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	registry := metadata.NewMetadataRegistry()
	for _, def := range []metadata.EntityDefinition{entities.PlanEntity(), entities.SubscriptionEntity()} {
		require.NoError(t, registry.Register(def))
		_, err := reconciler.ReconcileWithScope(context.Background(), pool, tenancy.PlatformTenantID, def, nil, reconciler.ScopeAllTenants)
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
	src := &billing.PlanSource{Operator: operator, Backend: state.Crud}

	// No plans at all: the built-in fallback, enforced.
	res, err := src.Resolve(ctx, customer)
	require.NoError(t, err)
	assert.True(t, res.Enforced)
	assert.Equal(t, billing.FallbackPlan.Name, res.Plan.Name)

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
	res, err = (&billing.PlanSource{Backend: state.Crud}).Resolve(ctx, customer)
	require.NoError(t, err)
	assert.False(t, res.Enforced)
}

// A customer asks, the operator decides: an upgrade applies at once, a downgrade is scheduled and
// the customer keeps the bigger plan until it is due; only the right people may ask and decide.
func TestPlanChangeRequestsAreDecidedByTheOperator(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistryAll(t, pool)
	operator, customer, userID := uuid.New(), uuid.New(), uuid.New()
	seedAdmin(t, pool, operator, userID)
	t.Cleanup(func() {
		for _, table := range []string{"waf.waf_plan_change_requests", "waf.waf_subscriptions", "waf.waf_plans"} {
			pool.Exec(context.Background(), "DELETE FROM "+table+" WHERE tenant_id = $1", operator)
		}
	})
	state, _ := newTestState(t, pool, registry)
	ctx := context.Background()
	opRC, custRC := adminContext(operator, userID), adminContext(customer, userID)
	src := &billing.PlanSource{Operator: operator, Backend: state.Crud}
	now := time.Now()

	mk := func(name string, zones int, def bool) crud.RecordDto {
		r, err := state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": name, "maxZones": zones, "maxRulesPerZone": zones * 5, "isDefault": def, "enabled": true}), opRC, nil)
		require.NoError(t, err)
		return r
	}
	small, big, mid := mk("Small", 1, true), mk("Big", 10, false), mk("Mid", 5, false)
	planOf := func() string {
		r, err := src.Resolve(ctx, customer)
		require.NoError(t, err)
		return r.Plan.Name
	}
	assert.Equal(t, "Small", planOf())

	// Only the customer's admin may ask, and not for the plan it has.
	_, err := src.RequestPlanChange(ctx, &permission.RequestContext{TenantID: customer.String(), Roles: []string{"viewer"}, HasRoles: true}, big.ID.String(), "", "Acme")
	assert.Error(t, err, "a non-admin cannot ask")
	_, err = src.RequestPlanChange(ctx, custRC, small.ID.String(), "", "Acme")
	assert.Error(t, err, "already on it")
	_, err = src.RequestPlanChange(ctx, opRC, big.ID.String(), "", "Op")
	assert.Error(t, err, "the operator is not on a plan")

	asked, err := src.RequestPlanChange(ctx, custRC, big.ID.String(), "we grew", "Acme")
	require.NoError(t, err)
	_, err = src.RequestPlanChange(ctx, custRC, big.ID.String(), "again", "Acme")
	assert.Error(t, err, "one open request at a time")
	mine, err := src.MyPlanRequests(ctx, custRC)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, "pending", mine[0]["status"])
	assert.Equal(t, "Big", mine[0]["planName"])

	reqID := idOf(asked)
	_, err = src.DecidePlanChange(ctx, custRC, reqID, true, "", now)
	assert.Error(t, err, "a customer cannot decide")

	// Approved upgrade: applies at once.
	res, err := src.DecidePlanChange(ctx, opRC, reqID, true, "welcome", now)
	require.NoError(t, err)
	assert.Equal(t, "now", res["applied"])
	assert.Equal(t, "Big", planOf())
	_, err = src.DecidePlanChange(ctx, opRC, reqID, true, "", now)
	assert.Error(t, err, "decided once")

	// A downgrade is approved but waits: the customer keeps Big until it is due.
	asked, err = src.RequestPlanChange(ctx, custRC, small.ID.String(), "", "Acme")
	require.NoError(t, err)
	reqID = idOf(asked)
	res, err = src.DecidePlanChange(ctx, opRC, reqID, true, "", now)
	require.NoError(t, err)
	assert.Equal(t, "scheduled", res["applied"])
	assert.Equal(t, "Big", planOf(), "still the bigger plan until the period ends")

	// A rejected request changes nothing.
	asked, err = src.RequestPlanChange(ctx, custRC, mid.ID.String(), "", "Acme")
	require.NoError(t, err)
	reqID = idOf(asked)
	res, err = src.DecidePlanChange(ctx, opRC, reqID, false, "not now", now)
	require.NoError(t, err)
	assert.Equal(t, "rejected", res["status"])
	mine, _ = src.MyPlanRequests(ctx, custRC)
	assert.Equal(t, "not now", mine[0]["reply"])
}

func setupRegistryAll(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	registry := metadata.NewMetadataRegistry()
	for _, def := range []metadata.EntityDefinition{entities.PlanEntity(), entities.SubscriptionEntity(), entities.PlanChangeRequestEntity()} {
		require.NoError(t, registry.Register(def))
		_, err := reconciler.ReconcileWithScope(context.Background(), pool, tenancy.PlatformTenantID, def, nil, reconciler.ScopeAllTenants)
		require.NoError(t, err, "reconcile %s", def.Name)
	}
	return registry
}

func idOf(o crud.JsonObject) uuid.UUID {
	var s string
	_ = json.Unmarshal(o["id"], &s)
	id, _ := uuid.Parse(s)
	return id
}

// Two requests at once from one customer cannot both wait, and two deciders at once cannot both
// win (audit 08 #4): the database lets one waiting request per customer in.
func TestConcurrentPlanRequestsAndDecisionsLetOneThrough(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistryAll(t, pool)
	operator, customer, userID := uuid.New(), uuid.New(), uuid.New()
	seedAdmin(t, pool, operator, userID)
	t.Cleanup(func() {
		for _, table := range []string{"waf.waf_plan_change_requests", "waf.waf_subscriptions", "waf.waf_plans"} {
			pool.Exec(context.Background(), "DELETE FROM "+table+" WHERE tenant_id = $1", operator)
		}
	})
	state, _ := newTestState(t, pool, registry)
	ctx := context.Background()
	opRC, custRC := adminContext(operator, userID), adminContext(customer, userID)
	src := &billing.PlanSource{Operator: operator, Backend: state.Crud}
	mk := func(name string, zones int, def bool) crud.RecordDto {
		r, err := state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": name, "maxZones": zones, "maxRulesPerZone": zones * 5, "isDefault": def, "enabled": true}), opRC, nil)
		require.NoError(t, err)
		return r
	}
	small, big := mk("Small", 1, true), mk("Big", 10, false)

	const n = 6
	var wg sync.WaitGroup
	var asked atomic.Int32
	var requestID atomic.Value
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := src.RequestPlanChange(ctx, custRC, big.ID.String(), "", "Acme"); err == nil {
				asked.Add(1)
				requestID.Store(idOf(got))
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, asked.Load(), "only one request waits per customer")

	var decided atomic.Int32
	var approved atomic.Bool // which decision won the race: it is not the same on every run
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(approve bool) {
			defer wg.Done()
			if _, err := src.DecidePlanChange(ctx, opRC, requestID.Load().(uuid.UUID), approve, "", time.Now()); err == nil {
				decided.Add(1)
				approved.Store(approve)
			}
		}(i%2 == 0)
	}
	wg.Wait()
	assert.EqualValues(t, 1, decided.Load(), "only one decision is taken")

	// Decided: the customer can ask again, for a plan other than the one it is on now (asking for the
	// plan it already has is refused, which is what happens when the approval won the race).
	other := big
	if approved.Load() {
		other = small
	}
	_, err := src.RequestPlanChange(ctx, custRC, other.ID.String(), "", "Acme")
	assert.NoError(t, err)
}
