//go:build integration

// E2E tests against a real Postgres: reconciles both entities this service owns, then exercises
// the full run-scan-job -> submit-findings lifecycle over a real HTTP server with a real signed
// JWT, mirroring the rigor established for the rest of this migration.
package scanning_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/metap-org/metap/pkg/jwks"
	"github.com/metap-org/metap/pkg/permission/policypg"
	"github.com/metap-org/metap/pkg/secrets"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/httpx/router"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/metap/pkg/tenancy"
	"github.com/metap-org/metap/pkg/testkit"
	"github.com/metap-org/waap/m-management/scanning"
	"github.com/metap-org/waap/m-management/scanning/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.NewPostgres(t)
	return pool
}

func setupRegistry(t *testing.T, pool *pgxpool.Pool) *metadata.MetadataRegistry {
	ctx := context.Background()
	registry := metadata.NewMetadataRegistry()
	for _, def := range []metadata.EntityDefinition{entities.ScanJobEntity(), entities.ScanFindingEntity()} {
		require.NoError(t, registry.Register(def), "register %s", def.Name)
		_, err := reconciler.ReconcileWithScope(ctx, pool, tenancy.PlatformTenantID, def, nil, reconciler.ScopeAllTenants)
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
		for _, table := range []string{"waf.waf_scan_findings", "waf.waf_scan_jobs"} {
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
	tenantRegistry := tenancy.NewPostgresTenantRegistry(pool)
	router := tenancy.NewRouter(pool, tenancy.NewRegistryCache(tenantRegistry), secrets.EnvStore{})
	permissions := permission.NewPermissionService(policypg.NewPostgresPolicyStore(router))
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

func TestScanningServiceReconcilesBothEntities(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	for _, name := range []string{"waf.scan_jobs", "waf.scan_findings"} {
		_, ok := registry.GetEntity(name)
		assert.True(t, ok, "expected %s to be registered", name)
	}
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

// TestRunScanJobWithNoScannerConfiguredJustQueues confirms that with SCANNER_URL unset, running
// a job transitions it to "queued" and reports dispatched: false rather than pretending a scan
// happened.
func TestRunScanJobWithNoScannerConfiguredJustQueues(t *testing.T) {
	os.Unsetenv("SCANNER_URL")
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, _ := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	jobData := crud.JsonObject{"zoneId": jsonMarshal(t, "zone-"+uuid.New().String()), "scanType": jsonMarshal(t, "quickScan")}
	jobResult, err := state.Crud.Create(ctx, "waf.scan_jobs", jobData, rc, nil)
	require.NoError(t, err, "create job: %v / %+v", err, jobResult)
	require.NoError(t, err)
	jobID := jobResult.ID

	actions := crud.NewActionRegistry()
	require.NoError(t, scanning.RegisterActions(scanning.Config{})(state, actions))
	run := func() (crud.JsonObject, error) {
		return actions.Invoke(ctx, crud.ActionCall{Entity: "waf.scan_jobs", Name: "run", ID: jobID, RC: rc})
	}

	data, err := run()
	require.NoError(t, err)
	require.Equal(t, json.RawMessage("false"), data["dispatched"], "expected dispatched=false with no scanner configured, got %+v", data)
	var job map[string]any
	require.NoError(t, json.Unmarshal(data["job"], &job))
	require.Equal(t, "queued", job["status"], "expected status=queued, got %+v", job)

	// Running it again while queued must be rejected with 409.
	_, err = run()
	var svcErr *crud.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, uint16(http.StatusConflict), svcErr.Status, "expected 409 for a job already queued")
}

// TestSubmitFindingsCompletesTheJobAndCreatesFindings exercises the scanner-callback endpoint
// directly (bypassing run-scan-job, since this endpoint accepts a job in any state the real
// scanner integration would reach it from) and confirms it creates ScanFinding rows, walks the
// job through start if still queued, and completes it with lastRunAt set.
func TestSubmitFindingsCompletesTheJobAndCreatesFindings(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)

	state, privPEM := newTestState(t, pool, registry)
	rc := adminContext(tenantID, userID)
	ctx := context.Background()

	jobData := crud.JsonObject{"zoneId": jsonMarshal(t, "zone-"+uuid.New().String()), "scanType": jsonMarshal(t, "fullScan")}
	jobResult, err := state.Crud.Create(ctx, "waf.scan_jobs", jobData, rc, nil)
	require.NoError(t, err, "create job: %v / %+v", err, jobResult)
	require.NoError(t, err)
	jobID := jobResult.ID

	// Walk the job to "queued" first via a real transition, matching what run-scan-job would
	// have done before a scanner calls back.
	queuedResult, err := state.Crud.Transition(ctx, "waf.scan_jobs", jobID, "run", jobResult.Version, nil, rc, nil)
	require.NoError(t, err, "transition to queued: %v / %+v", err, queuedResult)
	require.NoError(t, err)

	r := router.New()
	scanning.MountRoutes(r, state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: r}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	baseURL := "http://" + listener.Addr().String()

	token, err := jwks.MintRSAJWT(privPEM, tenantID, userID, 3600)
	require.NoError(t, err)

	findingsBody, _ := json.Marshal(map[string]any{
		"findings": []map[string]any{
			{"severity": "high", "category": "sqli", "endpoint": "/login", "description": "SQL injection in login form"},
			{"severity": "low", "category": "headers", "endpoint": "/"},
		},
		"succeeded": true,
	})
	resp := doPost(t, baseURL+"/internal/scan-jobs/"+jobID.String()+"/findings", token, findingsBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
	result := decodeJSON(t, resp)
	data := result["data"].(map[string]any)
	created := data["createdFindings"].([]any)
	require.Len(t, created, 2, "expected 2 created findings, got %+v", created)
	job := data["job"].(map[string]any)
	require.Equal(t, "completed", job["status"], "expected the job to be completed, got %+v", job)
	jobFields := job["data"].(map[string]any)
	lastRunAt, ok := jobFields["lastRunAt"].(string)
	require.True(t, ok, "expected lastRunAt to be set, got %+v", jobFields)
	require.NotEqual(t, "", lastRunAt, "expected lastRunAt to be set, got %+v", jobFields)

	// Both findings must actually exist, scoped to this job.
	findings, err := state.Crud.List(ctx, "waf.scan_findings",
		query.ListInput{Limit: 10, Filters: [][2]string{{"scanJobId", jobID.String()}}}, rc)
	require.NoError(t, err, "list findings")
	require.NoError(t, err)
	require.Len(t, findings.Data, 2, "expected 2 findings for this job, got %+v", findings)

	// A repeated callback (scanner retry) is refused up front: no duplicate findings, no 500 after
	// the rows were already written.
	again := doPost(t, baseURL+"/internal/scan-jobs/"+jobID.String()+"/findings", token, findingsBody)
	require.Equal(t, http.StatusConflict, again.StatusCode)
	assert.Equal(t, "scan_not_running", decodeJSON(t, again)["error"].(map[string]any)["code"])
	findings, err = state.Crud.List(ctx, "waf.scan_findings",
		query.ListInput{Limit: 10, Filters: [][2]string{{"scanJobId", jobID.String()}}}, rc)
	require.NoError(t, err)
	require.Len(t, findings.Data, 2, "the retry must not add findings")

	// One call may not write an unbounded number of rows.
	many := make([]map[string]any, 501)
	for i := range many {
		many[i] = map[string]any{"severity": "low", "category": "headers", "endpoint": "/"}
	}
	tooMany, _ := json.Marshal(map[string]any{"findings": many})
	big := doPost(t, baseURL+"/internal/scan-jobs/"+jobID.String()+"/findings", token, tooMany)
	require.Equal(t, http.StatusRequestEntityTooLarge, big.StatusCode)
}
