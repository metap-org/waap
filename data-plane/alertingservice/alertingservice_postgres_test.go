//go:build integration

// E2E tests against a real Postgres: reconciles all four entities this service owns, then
// exercises the incident-correlation sweep, the alert-evaluation sweep (with a real webhook
// receiver), and the "send test alert" endpoint over a real HTTP server with a real signed JWT —
// mirroring the rigor established for the rest of this migration.
package alertingservice_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

	"github.com/metap-org/waap/data-plane/alertingservice"
	"github.com/metap-org/waap/data-plane/alertingservice/entities"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.NewPostgres(t)
	return pool
}

func setupRegistry(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	ctx := context.Background()
	registry := metadata.NewMetadataRegistry()
	for _, def := range []metadata.EntityDefinition{
		entities.AlertPolicyEntity(), entities.AlertNotificationEntity(),
		entities.SecurityEventEntity(), entities.IncidentEntity(),
	} {
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
		for _, table := range []string{"waf.waf_alert_notifications", "waf.waf_alert_policies", "waf.waf_incidents", "waf.waf_security_events"} {
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

func jsonMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func startServer(t *testing.T, state *httpserver.AppState) string {
	r := chi.NewRouter()
	alertingservice.MountRoutes(r, state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: r}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return "http://" + listener.Addr().String()
}

func doPost(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(http.MethodPost, url, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var v map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

func createSecurityEvent(t *testing.T, ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, zoneID, sourceIP string, occurredAt time.Time) uuid.UUID {
	t.Helper()
	data := crud.JsonObject{
		"zoneId":        jsonMarshal(t, zoneID),
		"triggeredBy":   jsonMarshal(t, "firewallRule"),
		"triggeredById": jsonMarshal(t, uuid.New().String()),
		"action":        jsonMarshal(t, "blocked"),
		"sourceIp":      jsonMarshal(t, sourceIP),
		"requestPath":   jsonMarshal(t, "/login"),
		"occurredAt":    jsonMarshal(t, occurredAt.UTC().Format(time.RFC3339)),
	}
	result, err := state.Crud.Create(ctx, "waf.security_events", data, rc, nil)
	require.NoError(t, err, "create security event: %v / %+v", err, result)
	require.True(t, result.IsOk, "create security event: %v / %+v", err, result)
	return result.Data.ID
}

func listAllInput() query.ListInput {
	return query.ListInput{Limit: 50}
}

func TestAlertingServiceReconcilesAllFourEntities(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	for _, name := range []string{"waf.alert_policies", "waf.alert_notifications", "waf.security_events", "waf.incidents"} {
		_, ok := registry.GetEntity(name)
		assert.True(t, ok, "expected %s to be registered", name)
	}
}

// TestCorrelateIncidentsOpensOneIncidentAboveThresholdAndDedupes confirms the correlation sweep
// groups by (zoneId, sourceIp), only opens an Incident once the group clears the threshold, and
// is idempotent on a second run over the same events (the dedup-by-title rule).
func TestCorrelateIncidentsOpensOneIncidentAboveThresholdAndDedupes(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	zoneID := "zone-" + uuid.New().String()
	sourceIP := "203.0.113.7"
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		createSecurityEvent(t, ctx, state, rc, zoneID, sourceIP, now)
	}

	baseURL := startServer(t, state)
	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"threshold": 3, "zoneId": zoneID})
	resp := doPost(t, baseURL+"/internal/incidents/correlate", token, body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	data := decodeJSON(t, resp)["data"].(map[string]any)
	created := data["createdIncidents"].([]any)
	require.Len(t, created, 1, "expected exactly 1 created incident, got %+v", data)

	// A second run over the same events must not open a duplicate — the open incident with this
	// source IP in its title already exists.
	resp2 := doPost(t, baseURL+"/internal/incidents/correlate", token, body)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "expected 200, got %d", resp2.StatusCode)
	data2 := decodeJSON(t, resp2)["data"].(map[string]any)
	created2 := data2["createdIncidents"].([]any)
	require.Len(t, created2, 0, "expected the second run to create nothing (dedup), got %+v", data2)
	skipped, _ := data2["skippedExisting"].(float64)
	require.EqualValues(t, 1, skipped, "expected skippedExisting=1 on the second run, got %+v", data2)
}

// TestEvaluateAlertsFiresAWebhookNotification confirms the alert-evaluation sweep fires a
// notification once an enabled AlertPolicy's threshold is cleared, posts to the configured
// webhook, and records a "sent" AlertNotification.
func TestEvaluateAlertsFiresAWebhookNotification(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	received := make(chan struct{}, 1)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	policyData := crud.JsonObject{
		"name":           jsonMarshal(t, "High volume policy"),
		"thresholdCount": json.RawMessage("3"),
		"windowMinutes":  json.RawMessage("15"),
		"channels":       jsonMarshal(t, map[string]string{"webhook": webhook.URL}),
		"enabled":        json.RawMessage("true"),
	}
	policyResult, err := state.Crud.Create(ctx, "waf.alert_policies", policyData, rc, nil)
	require.NoError(t, err, "create alert policy: %v / %+v", err, policyResult)
	require.True(t, policyResult.IsOk, "create alert policy: %v / %+v", err, policyResult)

	zoneID := "zone-" + uuid.New().String()
	for i := 0; i < 4; i++ {
		createSecurityEvent(t, ctx, state, rc, zoneID, fmt.Sprintf("198.51.100.%d", i), time.Now().UTC())
	}

	baseURL := startServer(t, state)
	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	resp := doPost(t, baseURL+"/internal/alerts/evaluate", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	data := decodeJSON(t, resp)["data"].(map[string]any)
	fired := data["fired"].([]any)
	require.Len(t, fired, 1, "expected exactly 1 firing, got %+v", data)
	fire := fired[0].(map[string]any)
	require.Equal(t, true, fire["delivered"], "expected delivered=true, got %+v", fire)

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "webhook was never called")
	}

	notifications, err := state.Crud.List(ctx, "waf.alert_notifications", listAllInput(), rc)
	require.NoError(t, err, "list notifications: %v / %+v", err, notifications)
	require.True(t, notifications.IsOk, "list notifications: %v / %+v", err, notifications)
	require.Len(t, notifications.Data, 1, "expected 1 sent notification, got %+v", notifications.Data)
	require.NotNil(t, notifications.Data[0].Data["deliveryStatus"], "expected 1 sent notification, got %+v", notifications.Data)
}

// TestTestAlertPolicySendsAndRecordsANotification confirms the "Send test alert" endpoint runs
// the real delivery path (not a stub) and writes a real AlertNotification row.
func TestTestAlertPolicySendsAndRecordsANotification(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	policyData := crud.JsonObject{
		"name":           jsonMarshal(t, "Test policy"),
		"thresholdCount": json.RawMessage("1"),
		"windowMinutes":  json.RawMessage("15"),
		"channels":       jsonMarshal(t, map[string]string{"webhook": webhook.URL}),
		"enabled":        json.RawMessage("true"),
	}
	policyResult, err := state.Crud.Create(ctx, "waf.alert_policies", policyData, rc, nil)
	require.NoError(t, err, "create alert policy: %v / %+v", err, policyResult)
	require.True(t, policyResult.IsOk, "create alert policy: %v / %+v", err, policyResult)
	policyID := policyResult.Data.ID

	baseURL := startServer(t, state)
	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	resp := doPost(t, baseURL+"/api/waf.alert_policies/"+policyID.String()+"/test", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	data := decodeJSON(t, resp)["data"].(map[string]any)
	require.Equal(t, true, data["delivered"], "expected delivered=true, got %+v", data)

	notifications, err := state.Crud.List(ctx, "waf.alert_notifications", listAllInput(), rc)
	require.NoError(t, err, "expected exactly 1 notification row, got %v / %+v", err, notifications)
	require.True(t, notifications.IsOk, "expected exactly 1 notification row, got %v / %+v", err, notifications)
	require.Len(t, notifications.Data, 1, "expected exactly 1 notification row, got %v / %+v", err, notifications)
}

// TestCleanupSecurityEventsDeletesOnlyExpiredRows confirms the retention sweep deletes rows
// older than the cutoff and leaves everything else alone — the one behavior that actually
// matters for a retention job: get the boundary right in both directions.
func TestCleanupSecurityEventsDeletesOnlyExpiredRows(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	zoneID := "zone-" + uuid.New().String()
	now := time.Now().UTC()
	expiredID := createSecurityEvent(t, ctx, state, rc, zoneID, "203.0.113.1", now.AddDate(0, 0, -45))
	recentID := createSecurityEvent(t, ctx, state, rc, zoneID, "203.0.113.2", now.AddDate(0, 0, -1))

	baseURL := startServer(t, state)
	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	resp := doPost(t, baseURL+"/internal/security-events/cleanup", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	data := decodeJSON(t, resp)["data"].(map[string]any)
	deleted, _ := data["deletedCount"].(float64)
	require.EqualValues(t, 1, deleted, "expected deletedCount=1, got %+v", data)
	days, _ := data["retentionDays"].(float64)
	require.EqualValues(t, alertingservice.DefaultSecurityEventRetentionDays, days, "expected retentionDays=%d, got %+v", alertingservice.DefaultSecurityEventRetentionDays, data)

	result, err := state.Crud.Get(ctx, "waf.security_events", expiredID, rc)
	require.NoError(t, err, "expected the expired event to be gone, got %v / %+v", err, result)
	require.False(t, result.IsOk, "expected the expired event to be gone, got %v / %+v", err, result)
	result, err = state.Crud.Get(ctx, "waf.security_events", recentID, rc)
	require.NoError(t, err, "expected the recent event to still exist, got %v / %+v", err, result)
	require.True(t, result.IsOk, "expected the recent event to still exist, got %v / %+v", err, result)
}
