package analytics_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"

	"github.com/metap-org/waap/m-analytics"
)

// fakeAnalyticsAPI answers the two Analytics query routes the read model uses, recording what it
// was sent.
func fakeAnalyticsAPI(t *testing.T) (*httptest.Server, *[]*http.Request, *[]string) {
	t.Helper()
	var reqs []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		reqs, bodies = append(reqs, r), append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/security-events":
			_, _ = w.Write([]byte(`{"data":{"records":[{"id":"7b0f8a52-58b8-4d1e-9d55-3f3e1b0f0a01","zoneId":"z1","triggeredBy":"firewallRule",
				"triggeredById":"r1","triggeredByName":"Rule 1","action":"blocked","sourceIp":"203.0.113.7","requestPath":"/login",
				"occurredAt":"2026-11-08T11:00:00.123Z","ingestedAt":"2026-11-08T11:00:01Z"}],"nextCursor":"abc","hasMore":true}}`))
		case "/v1/security-events/aggregate":
			_, _ = w.Write([]byte(`{"data":[{"group":"blocked","count":4}]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_query","message":"cannot filter on \"x\""}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs, &bodies
}

func TestSecurityEventsComeFromAnalytics(t *testing.T) {
	srv, reqs, bodies := fakeAnalyticsAPI(t)
	backend := &analytics.Backend{Client: analytics.NewClient(srv.URL)}
	rc := &permission.RequestContext{ForwardedBearerToken: "caller-token"}
	ctx := context.Background()

	// The portal's zone events list, as the REST list route hands it over.
	sort := "-occurredAt"
	res, err := backend.List(ctx, analytics.SecurityEvents, query.ListInput{Limit: 50, Sort: &sort,
		Filters: [][2]string{{"zoneId", "z1"}, {"action", "blocked"}}}, rc)
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	var zone string
	require.NoError(t, json.Unmarshal(res.Data[0].Data["zoneId"], &zone))
	require.Equal(t, "z1", zone)
	require.NotNil(t, res.Page.NextCursor)
	require.Equal(t, "abc", *res.Page.NextCursor)
	r0 := (*reqs)[0]
	require.Equal(t, "Bearer caller-token", r0.Header.Get("Authorization"), "the caller's token goes to Analytics")
	require.Equal(t, "z1", r0.URL.Query().Get("zoneId"))
	require.Equal(t, "blocked", r0.URL.Query().Get("action"))
	require.Equal(t, "50", r0.URL.Query().Get("limit"))

	// The dashboards' aggregate call.
	since := "2026-11-01T00:00:00Z"
	group, timeField := "action", "occurredAt"
	limit := int64(10)
	rows, err := backend.Aggregate(ctx, analytics.SecurityEvents, query.AggregateSpec{GroupBy: &group, TimeField: &timeField,
		Since: &since, Filters: map[string]string{"zoneId": "z1"}, Limit: &limit}, rc)
	require.NoError(t, err)
	require.JSONEq(t, `{"group":"blocked","count":4}`, string(rows[0]))
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[1]), &sent))
	require.Equal(t, map[string]any{"groupBy": "action", "timeField": "occurredAt", "since": "2026-11-01T00:00:00Z",
		"filters": map[string]any{"zoneId": "z1"}, "limit": float64(10)}, sent)

	// Read-only.
	_, err = backend.Create(ctx, analytics.SecurityEvents, crud.JsonObject{}, rc, nil)
	require.Error(t, err)

	// An Analytics 400 surfaces with its code, not as a generic failure.
	_, err = backend.Get(ctx, analytics.SecurityEvents, uuid.MustParse("7b0f8a52-58b8-4d1e-9d55-3f3e1b0f0a01"), rc)
	se, ok := crud.AsServiceError(err)
	require.True(t, ok)
	require.Equal(t, "invalid_query", se.Code)
}
