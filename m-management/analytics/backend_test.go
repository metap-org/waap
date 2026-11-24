package analytics_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/auth/jwks"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/scope"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"

	"github.com/metap-org/waap/m-management/analytics"
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
	key, signer := newSigner(t)
	backend := &analytics.Backend{Client: analytics.NewClient(srv.URL), Tokens: &analytics.ReadTokens{Signer: signer, Grants: grants{wide: true}}}
	rc := callerRC()
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
	claims := readClaims(t, r0.Header.Get("Authorization"), key)
	require.Equal(t, analytics.ZoneAccessAll, claims["zoneAccess"], "a read token for what this caller may see goes to Analytics")
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

func newSigner(t *testing.T) (*jwks.JwksKeyPair, *jwks.TokenSigner) {
	t.Helper()
	key, err := jwks.GenerateJwksKeyPair("kid-test")
	require.NoError(t, err)
	return key, &jwks.TokenSigner{Kind: jwks.SignerJwks, Key: key}
}

func callerRC() *permission.RequestContext {
	return &permission.RequestContext{TenantID: uuid.NewString(), UserID: uuid.NewString(), ForwardedBearerToken: "login-token"}
}

// readClaims checks the bearer is a token for the Analytics API signed by key, and returns its claims.
func readClaims(t *testing.T, header string, key *jwks.JwksKeyPair) jwt.MapClaims {
	t.Helper()
	raw := strings.TrimPrefix(header, "Bearer ")
	require.NotEqual(t, "login-token", raw, "the caller's own login token is never what Analytics is sent")
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return key.PublicKey(), nil },
		jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithAudience(analytics.ReadAudience), jwt.WithExpirationRequired())
	require.NoError(t, err)
	return claims
}

// grants is the permission answer the tests choose: wide is the entity-level read (every zone),
// scoped says the caller holds a granting role on some zones, zones being which.
type grants struct {
	wide   bool
	scoped bool
	zones  []string
}

func (g grants) CanReadEntity(context.Context, *permission.RequestContext, string) (permission.PermissionDecision, error) {
	if g.wide {
		return permission.Allowed(), nil
	}
	return permission.Forbidden(), nil
}

func (g grants) ScopeFor(_ context.Context, rc *permission.RequestContext, resource, action string) (scope.Scope, permission.PermissionDecision, error) {
	if resource != analytics.EventsResource || action != "read" || !g.scoped {
		return scope.Scope{}, permission.Forbidden(), nil
	}
	return scope.ForTenant(rc.TenantID).Limit("zone", g.zones...), permission.Allowed(), nil
}

func TestTheReadTokenSaysWhichZonesTheCallerMaySee(t *testing.T) {
	srv, reqs, _ := fakeAnalyticsAPI(t)
	key, signer := newSigner(t)
	ctx := context.Background()
	read := func(g analytics.Grants) jwt.MapClaims {
		backend := &analytics.Backend{Client: analytics.NewClient(srv.URL), Tokens: &analytics.ReadTokens{Signer: signer, Grants: g}}
		_, err := backend.List(ctx, analytics.SecurityEvents, query.ListInput{Limit: 10}, callerRC())
		require.NoError(t, err)
		return readClaims(t, (*reqs)[len(*reqs)-1].Header.Get("Authorization"), key)
	}

	c := read(grants{wide: true})
	require.Equal(t, "all", c["zoneAccess"])
	require.NotContains(t, c, "zones")

	c = read(grants{scoped: true, zones: []string{"z1", "z2"}})
	require.Equal(t, "limited", c["zoneAccess"])
	require.Equal(t, []any{"z1", "z2"}, c["zones"])

	c = read(grants{scoped: true})
	require.Equal(t, "limited", c["zoneAccess"])
	require.Equal(t, []any{}, c["zones"], "limited to none is sent as none, never left out")

	exp, _ := c.GetExpirationTime()
	require.WithinDuration(t, time.Now().Add(analytics.ReadTTL), exp.Time, 5*time.Second)
}

// The events live outside metap, so the read check is the backend's own: a role the tenant granted
// neither the events entity nor the events resource on a zone gets a 403 from every read, and
// nothing is asked of Analytics (audit 08).
func TestSecurityEventsNeedTheReadPermission(t *testing.T) {
	srv, reqs, _ := fakeAnalyticsAPI(t)
	_, signer := newSigner(t)
	ctx := context.Background()
	for name, tokens := range map[string]*analytics.ReadTokens{
		"denied":    {Signer: signer, Grants: grants{}},
		"unwired":   {Signer: signer},
		"no tokens": nil,
	} {
		backend := &analytics.Backend{Client: analytics.NewClient(srv.URL), Tokens: tokens}
		rc := callerRC()
		_, err := backend.List(ctx, analytics.SecurityEvents, query.ListInput{Limit: 10}, rc)
		se, ok := crud.AsServiceError(err)
		require.True(t, ok, name)
		require.EqualValues(t, 403, se.Status, name)
		_, err = backend.Get(ctx, analytics.SecurityEvents, uuid.New(), rc)
		se, ok = crud.AsServiceError(err)
		require.True(t, ok, name)
		require.EqualValues(t, 403, se.Status, name)
		_, err = backend.Aggregate(ctx, analytics.SecurityEvents, query.AggregateSpec{}, rc)
		se, ok = crud.AsServiceError(err)
		require.True(t, ok, name)
		require.EqualValues(t, 403, se.Status, name)
	}
	require.Empty(t, *reqs, "a refused read never reaches Analytics")
}

func TestTheSweepsReadTheWholeTenantWithoutAPersonsGrant(t *testing.T) {
	key, signer := newSigner(t)
	token, err := (&analytics.ReadTokens{Signer: signer}).TenantWide(callerRC())
	require.NoError(t, err)
	require.Equal(t, "all", readClaims(t, "Bearer "+token, key)["zoneAccess"])
}

func TestTheNamesAnalyticsVerifiesAreTheOnesSigned(t *testing.T) {
	require.Equal(t, "waap-analytics", analytics.ReadAudience)
	require.Equal(t, "all", analytics.ZoneAccessAll)
	require.Equal(t, "limited", analytics.ZoneAccessLimited)
	require.Equal(t, "zoneAccess", analytics.ClaimZoneAccess)
	require.Equal(t, "zones", analytics.ClaimZones)
}
