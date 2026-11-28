package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func from(peer string, headers ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://zone.test/", nil)
	r.RemoteAddr = peer + ":4000"
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

func TestWithNothingConfiguredTheHeadersAreIgnored(t *testing.T) {
	r, err := New("", nil)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.9", r.Of(from("203.0.113.9", "X-Forwarded-For", "10.0.0.1", "X-Real-IP", "10.0.0.2")).String())
}

// What a caller puts in front of the list must never become their address.
func TestForwardedForIsReadFromTheRightPastTrustedProxies(t *testing.T) {
	r, err := New("x-forwarded-for", []string{"10.0.0.0/8"})
	require.NoError(t, err)
	got := r.Of(from("10.0.0.5", "X-Forwarded-For", "1.2.3.4, 198.51.100.7, 10.0.0.9"))
	assert.Equal(t, "198.51.100.7", got.String(), "the forged first entry is not reached")
	assert.Equal(t, "203.0.113.9", r.Of(from("203.0.113.9", "X-Forwarded-For", "1.2.3.4")).String(), "an untrusted peer is the client")
}

func TestASingleValueHeaderCountsOnlyFromATrustedProxy(t *testing.T) {
	r, err := New("X-Real-IP", []string{"10.0.0.1"})
	require.NoError(t, err)
	assert.Equal(t, "198.51.100.7", r.Of(from("10.0.0.1", "X-Real-IP", "198.51.100.7")).String())
	assert.Equal(t, "203.0.113.9", r.Of(from("203.0.113.9", "X-Real-IP", "1.2.3.4")).String())
	assert.Equal(t, "10.0.0.1", r.Of(from("10.0.0.1", "X-Real-IP", "1.2.3.4", "X-Real-IP", "198.51.100.7")).String(), "sent twice")
	assert.Equal(t, "10.0.0.1", r.Of(from("10.0.0.1", "X-Real-IP", "1.2.3.4, 5.6.7.8")).String(), "not one address")
}

func TestAHeaderWithoutATrustedProxyIsRefused(t *testing.T) {
	_, err := New("X-Forwarded-For", nil)
	assert.ErrorContains(t, err, "TRUSTED_PROXIES")
	_, err = New("", []string{"not-an-address"})
	assert.Error(t, err)
}
