package netcheck_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/metap/pkg/action/actiontest"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/zones/netcheck"
	"github.com/metap-org/waap/model"
)

// dohCNAME answers every question with one CNAME.
func dohCNAME(t *testing.T, target string) netcheck.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Answer":[{"data":"` + target + `."}]}`))
	}))
	t.Cleanup(srv.Close)
	return netcheck.Config{DoHURL: srv.URL, EdgeCNAMETarget: "edge.waf.local"}
}

func TestVerifyZoneKeepsWhetherTheHostnameRoutesToTheEdge(t *testing.T) {
	for _, c := range []struct {
		name, answer, status string
		routed               bool
	}{
		{"points at the edge", "shop.edge.waf.local", model.DNSRouted, true},
		{"points elsewhere", "other.example.net", model.DNSNotRouted, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			id := uuid.New()
			records := &actiontest.Records{}
			records.Store(model.Zones, id, map[string]any{"hostname": "shop.example.com"})
			res := actiontest.RunWith(netcheck.VerifyZone(dohCNAME(t, c.answer)),
				actiontest.Call(model.Zones, model.ActionVerifyDNS, id, nil, actiontest.Caller("analyst")), records, &actiontest.Sink{})
			require.NoError(t, res.Err)

			if c.routed {
				assert.JSONEq(t, `true`, string(res.Out["dnsRouted"]))
			} else {
				assert.JSONEq(t, `false`, string(res.Out["dnsRouted"]))
			}
			require.Len(t, records.Writes, 1)
			assert.Equal(t, id, records.Writes[0].ID)
			assert.JSONEq(t, `"`+c.status+`"`, string(records.Writes[0].Data[model.FieldDNSRoutingStatus]))
			assert.Contains(t, records.Writes[0].Data, model.FieldLastDNSCheckAt)
		})
	}
}

func TestVerifyZoneRefusesAZoneWithoutAHostname(t *testing.T) {
	id := uuid.New()
	records := &actiontest.Records{}
	records.Store(model.Zones, id, map[string]any{"originAddress": "1.2.3.4"})
	res := actiontest.RunWith(netcheck.VerifyZone(dohCNAME(t, "x")),
		actiontest.Call(model.Zones, model.ActionVerifyDNS, id, nil, actiontest.Caller()), records, &actiontest.Sink{})
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusBadRequest, se.Status)
	assert.Empty(t, records.Writes)
}

func TestTestZoneOriginKeepsTheAnswerOnTheZone(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "shop.example.com", r.Host, "the zone's hostname is the host header when none is set")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(origin.Close)

	id := uuid.New()
	records := &actiontest.Records{}
	records.Store(model.Zones, id, map[string]any{
		"hostname": "shop.example.com", "originAddress": strings.TrimPrefix(origin.URL, "http://"), "originProtocol": "http",
	})
	res := actiontest.RunWith(netcheck.TestZoneOrigin(netcheck.Config{AllowPrivateTargets: true}),
		actiontest.Call(model.Zones, model.ActionTestOrigin, id, nil, actiontest.Caller()), records, &actiontest.Sink{})
	require.NoError(t, res.Err)

	assert.JSONEq(t, `true`, string(res.Out["reachable"]))
	require.Len(t, records.Writes, 1)
	assert.JSONEq(t, `true`, string(records.Writes[0].Data[model.FieldOriginReachable]))
	assert.Contains(t, records.Writes[0].Data, model.FieldOriginCheckedAt)
}

func TestTestZoneOriginStillAnswersWhenItCannotSaveTheResult(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(origin.Close)

	id := uuid.New()
	records := &actiontest.Records{WriteErr: crud.NewServiceError(http.StatusConflict, "version_conflict")}
	records.Store(model.Zones, id, map[string]any{"hostname": "shop.example.com", "originAddress": strings.TrimPrefix(origin.URL, "http://"), "originProtocol": "http"})
	res := actiontest.RunWith(netcheck.TestZoneOrigin(netcheck.Config{AllowPrivateTargets: true}),
		actiontest.Call(model.Zones, model.ActionTestOrigin, id, nil, actiontest.Caller()), records, &actiontest.Sink{})
	require.NoError(t, res.Err, "a failed save does not hide the answer")
	assert.JSONEq(t, `true`, string(res.Out["reachable"]))
}

func TestTestZoneOriginRefusesAZoneWithoutAnOrigin(t *testing.T) {
	id := uuid.New()
	records := &actiontest.Records{}
	records.Store(model.Zones, id, map[string]any{"hostname": "shop.example.com"})
	res := actiontest.RunWith(netcheck.TestZoneOrigin(netcheck.Config{}),
		actiontest.Call(model.Zones, model.ActionTestOrigin, id, nil, actiontest.Caller()), records, &actiontest.Sink{})
	se, ok := crud.AsServiceError(res.Err)
	require.True(t, ok)
	assert.EqualValues(t, http.StatusBadRequest, se.Status)
}
