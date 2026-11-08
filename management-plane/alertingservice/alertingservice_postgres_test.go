//go:build integration

// E2E tests against a real Postgres: reconciles the three entities this service owns, then
// exercises the incident-correlation sweep, the alert-evaluation sweep (with a real webhook
// receiver), and the "send test alert" endpoint over a real HTTP server with a real signed JWT —
// mirroring the rigor established for the rest of this migration. Security events live in the
// Analytics plane, so a small fake of its query API (fakeAnalytics) stands in for it here.
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
	"sync"
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

	"github.com/metap-org/waap/management-plane/alertingservice"
	"github.com/metap-org/waap/management-plane/alertingservice/entities"
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
		entities.AlertPolicyEntity(), entities.AlertNotificationEntity(), entities.IncidentEntity(),
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
		for _, table := range []string{"waf.waf_alert_notifications", "waf.waf_alert_policies", "waf.waf_incidents"} {
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

// fakeAnalytics serves GET /v1/security-events the way the Analytics plane does (zoneId and
// since filters, newest first) from an in-memory list, and records what it was asked.
type fakeAnalytics struct {
	mu        sync.Mutex
	events    []map[string]any
	tokens    []string
	lastSince time.Time
}

func startAnalytics(t *testing.T) *fakeAnalytics {
	t.Helper()
	fa := &fakeAnalytics{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/security-events" {
			http.NotFound(w, r)
			return
		}
		fa.mu.Lock()
		defer fa.mu.Unlock()
		fa.tokens = append(fa.tokens, r.Header.Get("Authorization"))
		since, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
		fa.lastSince = since
		zone := r.URL.Query().Get("zoneId")
		records := []map[string]any{}
		for i := len(fa.events) - 1; i >= 0; i-- {
			e := fa.events[i]
			at, _ := time.Parse(time.RFC3339Nano, e["occurredAt"].(string))
			if (zone != "" && e["zoneId"] != zone) || at.Before(since) {
				continue
			}
			records = append(records, e)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"records": records, "nextCursor": nil, "hasMore": false}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANALYTICS_URL", srv.URL)
	return fa
}

func (fa *fakeAnalytics) add(zoneID, sourceIP string, occurredAt time.Time) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.events = append(fa.events, map[string]any{
		"id": uuid.NewString(), "zoneId": zoneID, "triggeredBy": "firewallRule", "triggeredById": uuid.NewString(),
		"action": "blocked", "sourceIp": sourceIP, "requestPath": "/login",
		"occurredAt": occurredAt.UTC().Format(time.RFC3339Nano), "ingestedAt": occurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func listAllInput() query.ListInput {
	return query.ListInput{Limit: 50}
}

func TestAlertingServiceReconcilesItsThreeEntities(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	for _, name := range []string{"waf.alert_policies", "waf.alert_notifications", "waf.incidents"} {
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

	fa := startAnalytics(t)
	zoneID := "zone-" + uuid.New().String()
	sourceIP := "203.0.113.7"
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		fa.add(zoneID, sourceIP, now)
	}
	// Outside the 15-minute window: must not count, even though there are enough of them.
	for i := 0; i < 5; i++ {
		fa.add(zoneID, "192.0.2.50", now.Add(-time.Hour))
	}

	baseURL := startServer(t, state)
	token, err := peripherals.MintJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{"threshold": 3, "zoneId": zoneID})
	resp := doPost(t, baseURL+"/internal/incidents/correlate", token, body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	data := decodeJSON(t, resp)["data"].(map[string]any)
	created := data["createdIncidents"].([]any)
	require.Len(t, created, 1, "expected exactly 1 created incident (the out-of-window IP never counts), got %+v", data)
	require.Equal(t, "Bearer "+token, fa.tokens[0], "the caller's own token is forwarded to Analytics")
	require.WithinDuration(t, now.Add(-15*time.Minute), fa.lastSince, time.Minute, "the window is pushed into the query")

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

	fa := startAnalytics(t)
	zoneID := "zone-" + uuid.New().String()
	for i := 0; i < 4; i++ {
		fa.add(zoneID, fmt.Sprintf("198.51.100.%d", i), time.Now().UTC())
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
