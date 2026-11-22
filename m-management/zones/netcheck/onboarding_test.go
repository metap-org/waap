package netcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOriginTargetBuildsTheURLFromWhatWasTyped(t *testing.T) {
	cases := []struct {
		spec       OriginSpec
		url, host  string
		wantFailed bool
	}{
		{OriginSpec{Address: "203.0.113.10"}, "https://203.0.113.10/", "203.0.113.10", false},
		{OriginSpec{Address: "origin.example.com", Protocol: "http"}, "http://origin.example.com/", "origin.example.com", false},
		{OriginSpec{Address: "http://origin.example.com/health?x=1"}, "http://origin.example.com/", "origin.example.com", false},
		{OriginSpec{Address: "origin.example.com:8443"}, "https://origin.example.com:8443/", "origin.example.com", false},
		{OriginSpec{Address: "origin.example.com:8443", Port: 9000, Protocol: "http"}, "http://origin.example.com:9000/", "origin.example.com", false},
		{OriginSpec{Address: "2001:db8::1", Port: 8080}, "https://[2001:db8::1]:8080/", "2001:db8::1", false},
		{OriginSpec{Address: "ftp://origin.example.com"}, "", "", true},
		{OriginSpec{Address: ""}, "", "", true},
		{OriginSpec{Address: "origin.example.com", Port: 70000}, "", "", true},
	}
	for _, c := range cases {
		url, host, err := originTarget(c.spec)
		if c.wantFailed {
			assert.Error(t, err, "%+v", c.spec)
			continue
		}
		require.NoError(t, err, "%+v", c.spec)
		assert.Equal(t, c.url, url, "%+v", c.spec)
		assert.Equal(t, c.host, host, "%+v", c.spec)
	}
}

// An origin on a private address is not contacted unless private targets are allowed; with them
// allowed, a reachable origin is reported with its status and the Host header is the one asked for.
func TestProbeOriginRefusesPrivateAddressesByDefault(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	spec := OriginSpec{Address: addr, Protocol: "http", HostHeader: "shop.example.com"}

	res := Config{}.ProbeOrigin(context.Background(), spec)
	assert.Equal(t, false, res["reachable"])
	assert.Contains(t, res["error"], "not a public one")
	assert.Empty(t, gotHost, "nothing was sent")

	res = Config{AllowPrivateTargets: true}.ProbeOrigin(context.Background(), spec)
	assert.Equal(t, true, res["reachable"], "%v", res)
	assert.Equal(t, http.StatusTeapot, res["status"], "an error status still means it answered")
	assert.Equal(t, "shop.example.com", gotHost)
}

func TestVerificationFileMustContainTheTokenAndStayUnderTheDomain(t *testing.T) {
	cfg := Config{AllowPrivateTargets: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/good"):
			_, _ = w.Write([]byte("good\n"))
		case strings.HasSuffix(r.URL.Path, "/other"):
			_, _ = w.Write([]byte("something else"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := verificationURL
	verificationURL = func(_, token string) string { return srv.URL + "/.well-known/waf-verify/" + token }
	t.Cleanup(func() { verificationURL = old })

	ok, _, err := cfg.fetchVerificationFile(context.Background(), "example.com", "www.example.com", "good")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, detail, _ := cfg.fetchVerificationFile(context.Background(), "example.com", "example.com", "other")
	assert.False(t, ok)
	assert.Contains(t, detail, "does not contain")
	ok, detail, _ = cfg.fetchVerificationFile(context.Background(), "example.com", "", "missing")
	assert.False(t, ok)
	assert.Contains(t, detail, "404")

	// A host outside the domain is refused before anything is fetched.
	_, _, err = cfg.fetchVerificationFile(context.Background(), "example.com", "evil.test", "good")
	assert.Error(t, err)
	_, _, err = cfg.fetchVerificationFile(context.Background(), "example.com", "notexample.com", "good")
	assert.Error(t, err, "a name that merely ends in the domain's letters is not under it")
}

func TestResolveHostnameRejectsInvalidAndNormalises(t *testing.T) {
	// Nothing answers at the resolver: the name resolves to nothing.
	cfg := Config{DoHURL: "http://127.0.0.1:1/resolve", EdgeCNAMETarget: "edge.waf.local"}
	_, err := cfg.ResolveHostname(context.Background(), crud.JsonObject{"hostname": json.RawMessage(`"203.0.113.5"`)})
	assert.Error(t, err)
	view, err := cfg.ResolveHostname(context.Background(), crud.JsonObject{"hostname": json.RawMessage(`"HTTPS://Shop.Example.com/"`)})
	require.NoError(t, err)
	assert.Equal(t, "shop.example.com", view.Hostname)
	assert.Equal(t, []string{"unresolved"}, view.Warnings)
	assert.Equal(t, "edge.waf.local", view.EdgeTarget)
}
