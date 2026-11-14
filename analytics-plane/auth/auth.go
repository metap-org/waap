// Package auth verifies the platform's access tokens without depending on metap (ADR-0009:
// Analytics does not use metap). Tokens are EdDSA JWTs signed by the single issuer
// (zones-service), whose public keys are published as a JWKS. This package fetches that set,
// caches it, and refetches when a token names a kid it hasn't seen (key rotation) — at most once
// per minRefetch, so a flood of bogus kids can't turn into a flood of JWKS requests.
//
// Claims it relies on: iss "metap", aud "metap-api", exp, sub (user id) and tenantId — the same
// contract metap's own issuer mints (pkg/peripherals). The tenant a request may read or write is
// always the token's, never a request parameter.
package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/kit/httpx"
)

const (
	Issuer   = "metap"
	Audience = "metap-api"

	minRefetch = 10 * time.Second
	leeway     = 20 * time.Second
)

// Identity is who a verified token says the caller is.
type Identity struct {
	TenantID uuid.UUID
	Subject  string
	Token    string // the raw bearer token, for forwarding
}

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}

// JWKSVerifier verifies tokens against a remote JWKS.
type JWKSVerifier struct {
	url    string
	ttl    time.Duration
	client *http.Client

	mu          sync.Mutex
	keys        map[string]ed25519.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

func NewJWKSVerifier(url string, ttl time.Duration) *JWKSVerifier {
	return &JWKSVerifier{url: url, ttl: ttl, client: &http.Client{Timeout: 5 * time.Second}}
}

func (v *JWKSVerifier) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching JWKS: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decoding JWKS: %w", err)
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(raw)
	}
	v.keys, v.fetchedAt = keys, time.Now()
	return nil
}

func (v *JWKSVerifier) keyFor(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	stale := time.Since(v.fetchedAt) > v.ttl
	key, known := v.keys[kid]
	if (stale || !known) && time.Since(v.lastAttempt) >= minRefetch {
		v.lastAttempt = time.Now()
		if err := v.fetch(ctx); err != nil && v.keys == nil {
			return nil, err
		}
		key, known = v.keys[kid]
	}
	if !known {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	return key, nil
}

type claims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenantId"`
}

// Verify checks signature, issuer, audience and expiry, and returns the caller's identity.
func (v *JWKSVerifier) Verify(ctx context.Context, token string) (Identity, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no kid")
		}
		return v.keyFor(ctx, kid)
	},
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(leeway),
	)
	if err != nil {
		return Identity{}, err
	}
	tenant, err := uuid.Parse(c.TenantID)
	if err != nil {
		return Identity{}, errors.New("token has no valid tenantId")
	}
	return Identity{TenantID: tenant, Subject: c.Subject, Token: token}, nil
}

// Verifier is what the HTTP layer needs.
type Verifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}

type ctxKey struct{}

// FromContext returns the identity Middleware attached.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// WithIdentity is for tests and for Middleware.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// Middleware requires "Authorization: Bearer <token>" and attaches the verified identity.
func Middleware(v Verifier, onError func(w http.ResponseWriter, status int, code, msg string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := httpx.BearerToken(header)
			if !ok {
				onError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
				return
			}
			id, err := v.Verify(r.Context(), token)
			if err != nil {
				onError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
