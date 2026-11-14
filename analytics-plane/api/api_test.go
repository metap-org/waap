package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/waap/analytics-plane/auth"
	"github.com/metap-org/waap/analytics-plane/events"
	"github.com/metap-org/waap/analytics-plane/store"
)

var tenantA = uuid.MustParse("2e87cf98-46a7-473f-bebc-20307e17d3b3")

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, token string) (auth.Identity, error) {
	if token != "good" {
		return auth.Identity{}, errors.New("bad token")
	}
	return auth.Identity{TenantID: tenantA, Subject: "u1", Token: token}, nil
}

type fakeStore struct {
	inserted []events.Event
	tenant   uuid.UUID
	listQ    events.ListQuery
	aggQ     events.AggregateQuery
	failNext error
}

func (f *fakeStore) Insert(_ context.Context, tenantID uuid.UUID, evs []events.Event) error {
	if f.failNext != nil {
		return f.failNext
	}
	f.tenant = tenantID
	f.inserted = append(f.inserted, evs...)
	return nil
}
func (f *fakeStore) List(_ context.Context, tenantID uuid.UUID, q events.ListQuery) (store.Page, error) {
	f.tenant, f.listQ = tenantID, q
	if _, err := events.PlanList("t", tenantID, q); err != nil {
		return store.Page{}, err
	}
	return store.Page{Records: f.inserted}, nil
}
func (f *fakeStore) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (events.Event, error) {
	for _, e := range f.inserted {
		if e.ID == id {
			return e, nil
		}
	}
	return events.Event{}, store.ErrNotFound
}
func (f *fakeStore) Aggregate(_ context.Context, tenantID uuid.UUID, q events.AggregateQuery) ([]map[string]any, error) {
	f.aggQ = q
	if _, err := events.PlanAggregate("t", tenantID, q); err != nil {
		return nil, err
	}
	return []map[string]any{{"group": "blocked", "count": 2.0}}, nil
}
func (f *fakeStore) Ping(context.Context) error { return nil }

func do(t *testing.T, h http.Handler, method, path, token, body string, headers ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestIngestAndQuery(t *testing.T) {
	st := &fakeStore{}
	srv := &Server{Store: st, Verifier: fakeVerifier{}, IngestToken: "secret", Now: func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }}
	h := srv.Handler()

	rec, _ := do(t, h, "GET", "/v1/security-events", "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	rec, _ = do(t, h, "GET", "/v1/security-events", "forged", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	body := `{"events":[
		{"zoneId":"z1","triggeredBy":"firewallRule","triggeredById":"r1","action":"blocked","sourceIp":"203.0.113.7","requestPath":"/x","occurredAt":"2026-11-08T11:00:00Z"},
		{"zoneId":"z1","triggeredBy":"nope","triggeredById":"r1","action":"blocked","sourceIp":"203.0.113.7","requestPath":"/x","occurredAt":"2026-11-08T11:00:00Z"}]}`
	rec, _ = do(t, h, "POST", "/v1/security-events/ingest", "good", body)
	require.Equal(t, http.StatusForbidden, rec.Code, "a user token alone can't ingest")

	rec, out := do(t, h, "POST", "/v1/security-events/ingest", "good", body, IngestHeader, "secret")
	require.Equal(t, http.StatusAccepted, rec.Code)
	data := out["data"].(map[string]any)
	require.EqualValues(t, 1, data["accepted"])
	require.Len(t, data["rejected"], 1)
	require.Equal(t, tenantA, st.tenant, "tenant comes from the token")

	big := `{"events":[` + strings.TrimSuffix(strings.Repeat(`{},`, MaxIngestEvents+1), ",") + `]}`
	rec, _ = do(t, h, "POST", "/v1/security-events/ingest", "good", big, IngestHeader, "secret")
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

	st.failNext = errors.New("clickhouse down")
	rec, _ = do(t, h, "POST", "/v1/security-events/ingest", "good", body, IngestHeader, "secret")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	st.failNext = nil

	rec, _ = do(t, h, "GET", "/v1/security-events?zoneId=z1&action=blocked&since=2026-11-01T00:00:00Z&limit=5&sort=occurredAt", "good", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, map[string]string{"zoneId": "z1", "action": "blocked"}, st.listQ.Filters)
	require.True(t, st.listQ.Ascending)
	require.Equal(t, 5, st.listQ.Limit)

	rec, out = do(t, h, "GET", "/v1/security-events?tenant_id=other", "good", "")
	require.Equal(t, http.StatusBadRequest, rec.Code, "an unknown filter is an error, not ignored")
	require.Equal(t, "invalid_query", out["error"].(map[string]any)["code"])

	id := st.inserted[0].ID
	rec, out = do(t, h, "GET", "/v1/security-events/"+id.String(), "good", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, id.String(), out["data"].(map[string]any)["id"])
	rec, _ = do(t, h, "GET", "/v1/security-events/"+uuid.NewString(), "good", "")
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec, out = do(t, h, "POST", "/v1/security-events/aggregate", "good", `{"groupBy":"action","timeField":"occurredAt","since":"2026-11-01T00:00:00Z","filters":{"zoneId":"z1"},"limit":10}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "action", st.aggQ.GroupBy)
	require.Equal(t, []any{map[string]any{"group": "blocked", "count": 2.0}}, out["data"])

	rec, _ = do(t, h, "POST", "/v1/security-events/aggregate", "good", `{"groupBy":"action","sql":"DROP TABLE"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "unknown spec keys are refused")
	rec, _ = do(t, h, "POST", "/v1/security-events/aggregate", "good", `{"metrics":["sum:action"]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec, _ = do(t, h, "GET", "/health", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestOversizedBodiesAreRefusedWith413(t *testing.T) {
	h := (&Server{Store: &fakeStore{}, Verifier: fakeVerifier{}}).Handler()

	rec, _ := do(t, h, "POST", "/v1/security-events/ingest", "good", `{"events":[{"zoneId":"`+strings.Repeat("z", maxBodyBytes)+`"}]}`)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"payload_too_large"`)

	rec, _ = do(t, h, "POST", "/v1/security-events/aggregate", "good", `{"groupBy":["`+strings.Repeat("x", 64<<10)+`"]}`)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestAPanicingStoreIsAJSON500NotADroppedConnection(t *testing.T) {
	h := (&Server{Store: &panicStore{}, Verifier: fakeVerifier{}}).Handler()
	rec, _ := do(t, h, "GET", "/v1/security-events", "good", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"internal_error"`)
}

type panicStore struct{ fakeStore }

func (*panicStore) List(context.Context, uuid.UUID, events.ListQuery) (store.Page, error) {
	panic("boom")
}
