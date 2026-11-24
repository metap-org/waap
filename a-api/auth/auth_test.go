package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type issuer struct {
	keys    map[string]ed25519.PrivateKey
	fetches atomic.Int32
	srv     *httptest.Server
}

func newIssuer(t *testing.T, kids ...string) *issuer {
	is := &issuer{keys: map[string]ed25519.PrivateKey{}}
	for _, kid := range kids {
		_, priv, _ := ed25519.GenerateKey(rand.Reader)
		is.keys[kid] = priv
	}
	is.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		is.fetches.Add(1)
		var keys []map[string]string
		for kid, priv := range is.keys {
			keys = append(keys, map[string]string{"kty": "OKP", "crv": "Ed25519", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(is.srv.Close)
	return is
}

func (is *issuer) sign(t *testing.T, kid string, mutate func(jwt.MapClaims)) string {
	c := jwt.MapClaims{
		"iss": Issuer, "aud": Audience, "sub": "user-1",
		"tenantId": "2e87cf98-46a7-473f-bebc-20307e17d3b3",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	if mutate != nil {
		mutate(c)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(is.keys[kid])
	require.NoError(t, err)
	return s
}

func TestVerify(t *testing.T) {
	is := newIssuer(t, "k1")
	v := NewJWKSVerifier(is.srv.URL, time.Hour)
	ctx := context.Background()

	id, err := v.Verify(ctx, is.sign(t, "k1", nil))
	require.NoError(t, err)
	require.Equal(t, uuid.MustParse("2e87cf98-46a7-473f-bebc-20307e17d3b3"), id.TenantID)
	require.Equal(t, "user-1", id.Subject)

	for name, mutate := range map[string]func(jwt.MapClaims){
		"expired":     func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"wrong aud":   func(c jwt.MapClaims) { c["aud"] = "someone-else" },
		"wrong iss":   func(c jwt.MapClaims) { c["iss"] = "evil" },
		"no tenant":   func(c jwt.MapClaims) { delete(c, "tenantId") },
		"no exp":      func(c jwt.MapClaims) { delete(c, "exp") },
		"tenant junk": func(c jwt.MapClaims) { c["tenantId"] = "not-a-uuid" },
	} {
		_, err := v.Verify(ctx, is.sign(t, "k1", mutate))
		require.Error(t, err, name)
	}

	// A key the verifier has never seen (another issuer's) is rejected.
	other := newIssuer(t, "k1")
	_, err = v.Verify(ctx, other.sign(t, "k1", nil))
	require.Error(t, err)

	// HS256 with the public key as secret (alg confusion) is refused outright.
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": Issuer, "aud": Audience, "exp": time.Now().Add(time.Hour).Unix(), "tenantId": uuid.NewString()})
	hs.Header["kid"] = "k1"
	s, _ := hs.SignedString([]byte(is.keys["k1"].Public().(ed25519.PublicKey)))
	_, err = v.Verify(ctx, s)
	require.Error(t, err)
}

func TestRotationRefetchesOnUnknownKid(t *testing.T) {
	is := newIssuer(t, "k1")
	v := NewJWKSVerifier(is.srv.URL, time.Hour)
	ctx := context.Background()
	_, err := v.Verify(ctx, is.sign(t, "k1", nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, is.fetches.Load())

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	is.keys["k2"] = priv
	v.lastAttempt = time.Time{} // past the refetch rate limit
	_, err = v.Verify(ctx, is.sign(t, "k2", nil))
	require.NoError(t, err, "a newly published kid is picked up")
	require.EqualValues(t, 2, is.fetches.Load())

	// Unknown kids don't hammer the issuer: within minRefetch there is no second fetch.
	for i := 0; i < 5; i++ {
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
		tok.Header["kid"] = "bogus"
		s, _ := tok.SignedString(priv)
		_, err := v.Verify(ctx, s)
		require.Error(t, err)
	}
	require.EqualValues(t, 2, is.fetches.Load())
}

func TestAReadTokenCarriesWhatTheCallerMaySee(t *testing.T) {
	is := newIssuer(t, "k1")
	v := NewJWKSVerifier(is.srv.URL, time.Minute)
	ctx := context.Background()
	read := func(mutate func(jwt.MapClaims)) string {
		return is.sign(t, "k1", func(c jwt.MapClaims) {
			c["aud"] = ReadAudience
			mutate(c)
		})
	}

	id, err := v.Verify(ctx, read(func(c jwt.MapClaims) { c["zoneAccess"] = ZoneAccessAll }))
	require.NoError(t, err)
	sc, ok := id.Scope()
	require.True(t, ok)
	require.True(t, sc.Allows("zone", "any"), "all: no zone limit")

	id, err = v.Verify(ctx, read(func(c jwt.MapClaims) { c["zoneAccess"] = ZoneAccessLimited; c["zones"] = []string{"z1", "z2"} }))
	require.NoError(t, err)
	sc, _ = id.Scope()
	require.True(t, sc.Allows("zone", "z1"))
	require.False(t, sc.Allows("zone", "z3"))

	id, err = v.Verify(ctx, read(func(c jwt.MapClaims) { c["zoneAccess"] = ZoneAccessLimited }))
	require.NoError(t, err)
	sc, _ = id.Scope()
	require.False(t, sc.Allows("zone", "z1"), "limited to no zone sees none, it does not fall open")

	_, err = v.Verify(ctx, read(func(c jwt.MapClaims) {}))
	require.Error(t, err, "a read token that says nothing about zones is refused, not read as 'all'")

	_, err = v.Verify(ctx, read(func(c jwt.MapClaims) { c["zoneAccess"] = "everything" }))
	require.Error(t, err)
}

func TestAnOrdinaryTokenIsNotAReader(t *testing.T) {
	is := newIssuer(t, "k1")
	v := NewJWKSVerifier(is.srv.URL, time.Minute)
	id, err := v.Verify(context.Background(), is.sign(t, "k1", func(c jwt.MapClaims) {
		c["zoneAccess"], c["zones"] = ZoneAccessAll, []string{"z1"} // claims it has no business carrying
	}))
	require.NoError(t, err, "still good for what it is for: identifying the control plane's ingest")
	_, ok := id.Scope()
	require.False(t, ok)
}

func TestATokenForAnotherServiceIsRefused(t *testing.T) {
	is := newIssuer(t, "k1")
	v := NewJWKSVerifier(is.srv.URL, time.Minute)
	_, err := v.Verify(context.Background(), is.sign(t, "k1", func(c jwt.MapClaims) { c["aud"] = "somebody-else" }))
	require.Error(t, err)
}

func TestTheAudienceAndClaimNamesAreThoseManagementSigns(t *testing.T) {
	require.Equal(t, "waap-analytics", ReadAudience)
	require.Equal(t, "all", ZoneAccessAll)
	require.Equal(t, "limited", ZoneAccessLimited)
}
