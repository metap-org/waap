package graphqlgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/management-plane/analytics"
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

func analyticsSchema(t *testing.T, base string) (*metadata.MetadataRegistry, map[string]crud.RecordBackend) {
	registry := metadata.NewMetadataRegistry()
	require.NoError(t, registry.Register(analytics.SecurityEventEntity()))
	return registry, map[string]crud.RecordBackend{analytics.SecurityEvents: &analytics.Backend{Client: analytics.NewClient(base)}}
}

func TestSecurityEventsComeFromAnalytics(t *testing.T) {
	srv, reqs, bodies := fakeAnalyticsAPI(t)
	registry, backends := analyticsSchema(t, srv.URL)
	schema, err := BuildSchema(registry, backends, RestURLs{})
	require.NoError(t, err)

	// The portal's ZoneEventsTab/IncidentDetailPage list call, as platform-ui shapes it.
	res := exec(schema, `query($f: Json) { wafSecurityEventsList(filter: $f, limit: 50, sort: "-occurredAt") {
		records { id zoneId action sourceIp triggeredByName occurredAt } nextCursor hasMore } }`,
		map[string]interface{}{"f": map[string]interface{}{"zoneId": "z1", "action": "blocked"}})
	require.Empty(t, res.Errors)
	conn := res.Data.(map[string]interface{})["wafSecurityEventsList"].(map[string]interface{})
	rec := conn["records"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "z1", rec["zoneId"])
	require.Equal(t, "2026-11-08T11:00:00.123Z", rec["occurredAt"])
	require.Equal(t, true, conn["hasMore"])
	require.Equal(t, "abc", conn["nextCursor"])
	r0 := (*reqs)[0]
	require.Equal(t, "Bearer caller-token", r0.Header.Get("Authorization"), "the caller's token goes to Analytics")
	require.Equal(t, "z1", r0.URL.Query().Get("zoneId"))
	require.Equal(t, "blocked", r0.URL.Query().Get("action"))
	require.Equal(t, "50", r0.URL.Query().Get("limit"))

	// DashboardPage/AnalyticsPage aggregate call.
	res = exec(schema, `query($s: Json!) { aggregate(entity: "waf.security_events", spec: $s) }`,
		map[string]interface{}{"s": map[string]interface{}{"groupBy": "action", "timeField": "occurredAt", "since": "2026-11-01T00:00:00Z", "filters": map[string]interface{}{"zoneId": "z1"}, "limit": 10}})
	require.Empty(t, res.Errors)
	agg := res.Data.(map[string]interface{})["aggregate"].(map[string]interface{})["data"].([]interface{})
	require.Equal(t, map[string]interface{}{"group": "blocked", "count": float64(4)}, agg[0])
	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte((*bodies)[1]), &sent))
	require.Equal(t, map[string]interface{}{"groupBy": "action", "timeField": "occurredAt", "since": "2026-11-01T00:00:00Z",
		"filters": map[string]interface{}{"zoneId": "z1"}, "limit": float64(10)}, sent)

	// Read-only: no mutations generated for it.
	res = exec(schema, `{ __type(name: "Mutation") { fields { name } } }`, nil)
	require.Empty(t, res.Errors)
	if typ, ok := res.Data.(map[string]interface{})["__type"].(map[string]interface{}); ok && typ != nil {
		for _, f := range typ["fields"].([]interface{}) {
			require.NotContains(t, f.(map[string]interface{})["name"], "SecurityEvents")
		}
	}

	// An Analytics 400 surfaces with its code, not as a generic failure.
	res = exec(schema, `{ wafSecurityEvents(id: "7b0f8a52-58b8-4d1e-9d55-3f3e1b0f0a01") { id } }`, nil)
	require.NotEmpty(t, res.Errors)
	require.Equal(t, "invalid_query", res.Errors[0].Extensions["code"])
}
