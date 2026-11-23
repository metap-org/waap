package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func channels(t *testing.T, url string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"webhook": url})
	require.NoError(t, err)
	return raw
}

// A tenant-typed webhook must not reach our own network, and the answer must not say what was
// found there.
func TestDeliverRefusesPrivateTargetsWithoutLeakingTransportErrors(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	ok, detail := Config{}.deliver(context.Background(), channels(t, srv.URL), map[string]any{})
	assert.False(t, ok)
	assert.Equal(t, "webhook refused: the address is not public", detail)
	assert.Zero(t, hits.Load(), "the loopback receiver must never be called")

	for _, target := range []string{"https://127.0.0.1:1/", "https://169.254.169.254/latest/meta-data/", "https://10.0.0.5:6379/"} {
		ok, detail := Config{}.deliver(context.Background(), channels(t, target), map[string]any{})
		assert.False(t, ok, target)
		assert.Equal(t, "webhook refused: the address is not public", detail, target)
	}
}

func TestDeliverRequiresHTTPSUnlessPrivateTargetsAreAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(srv.Close)

	ok, detail := Config{}.deliver(context.Background(), channels(t, srv.URL), map[string]any{})
	assert.False(t, ok)
	assert.Equal(t, "webhook refused: must be an https URL", detail)

	ok, detail = Config{AllowPrivateTargets: true}.deliver(context.Background(), channels(t, srv.URL), map[string]any{})
	assert.True(t, ok)
	assert.Equal(t, "webhook 204", detail)

	ok, _ = Config{}.deliver(context.Background(), channels(t, "ftp://example.com/x"), map[string]any{})
	assert.False(t, ok)
	ok, _ = Config{}.deliver(context.Background(), channels(t, "https://user:pw@example.com/"), map[string]any{})
	assert.False(t, ok)
}

// A redirect is not followed: an allowed host answering 302 to an internal address gets nowhere.
func TestDeliverDoesNotFollowRedirects(t *testing.T) {
	var inner atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { inner.Add(1) }))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	ok, detail := Config{AllowPrivateTargets: true}.deliver(context.Background(), channels(t, redirector.URL), map[string]any{})
	assert.False(t, ok)
	assert.Equal(t, "webhook 302", detail)
	assert.Zero(t, inner.Load())
}
