// waf-graphql-gateway — WAF's own GraphQL BFF.
//
// A bespoke, forwarding gateway (confirmed with the project owner, since metap-go never got a
// GraphQL layer at all — metap-graphql/metap-graphql-http/metap-graphql-gateway were never
// ported). The schema is generated from the 3 WAF services' own entity definitions (imported
// below for their init() SubmitEntity side effect) in the Rust metap-graphql wire shape the
// portal and control-plane expect (graphqlgateway.BuildSchema), forwarding to each upstream's gRPC RecordBackend
// (grpcapi.GrpcBackend, the same client the 3 Go WAF services already use for their own
// cross-service calls); the 8 custom business-action mutations forward to each service's REST
// endpoint instead, since those have no gRPC equivalent.
//
// Auth: verifies every caller's token against the token issuer's JWKS (JWKS_URL — zones-service
// in this product), and forwards that same token upstream; it never signs anything. Legacy RS256
// tokens also verify while AUTH_JWT_PUBLIC_KEY_PATH is set (cutover).
package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/grpcapi"
	"github.com/metap-org/metap/pkg/jwks"
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/runtime"

	_ "github.com/metap-org/waap/data-plane/alertingservice/entities"
	_ "github.com/metap-org/waap/data-plane/scanningservice/entities"
	_ "github.com/metap-org/waap/data-plane/zonesservice/entities"

	"github.com/metap-org/waap/data-plane/graphqlgateway"
)

type upstreamConfig struct {
	name            string
	grpcAddr        string
	loginURL        string
	restURL         string
	serviceEmail    string
	servicePassword string
}

func loadUpstream(n int) (upstreamConfig, bool) {
	name, ok := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_NAME", n))
	if !ok {
		return upstreamConfig{}, false
	}
	grpcAddr, _ := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_GRPC_ADDR", n))
	loginURL, _ := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_LOGIN_URL", n))
	email, _ := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_SERVICE_EMAIL", n))
	password, _ := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_SERVICE_PASSWORD", n))
	// UPSTREAM_N_REST_URL: the upstream's own REST base for custom mutations. Defaults to the
	// login URL's host, which is only right while every upstream issues its own tokens — with a
	// single JWKS issuer every LOGIN_URL points at the issuer, so set REST_URL explicitly.
	restURL, ok := runtime.Optional(fmt.Sprintf("UPSTREAM_%d_REST_URL", n))
	if !ok {
		restURL = restBaseFromLoginURL(loginURL)
	}
	return upstreamConfig{name: name, grpcAddr: stripScheme(grpcAddr), loginURL: loginURL, restURL: strings.TrimSuffix(restURL, "/"), serviceEmail: email, servicePassword: password}, true
}

// stripScheme: UPSTREAM_N_GRPC_ADDR is written as a URL (http://localhost:3001) in
// .env.example, matching the Rust client's Endpoint::connect — grpc.NewClient wants a bare
// host:port target instead.
func stripScheme(addr string) string {
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	return addr
}

// restBaseFromLoginURL derives http://localhost:3000 from http://localhost:3000/auth/login —
// the one REST base URL each upstream's custom-mutation forwarding needs, without adding a 4th
// per-upstream env var that would just repeat UPSTREAM_N_LOGIN_URL's own host.
func restBaseFromLoginURL(loginURL string) string {
	return strings.TrimSuffix(loginURL, "/auth/login")
}

// buildVerifier: JWKS_URL (the issuer's /.well-known/jwks.json) verifies EdDSA tokens;
// AUTH_JWT_PUBLIC_KEY_PATH additionally (or, without JWKS_URL, only) verifies legacy RS256 ones.
func buildVerifier(ctx context.Context) (*jwks.TokenVerifier, error) {
	var publicKey *rsa.PublicKey
	if path, ok := runtime.Optional("AUTH_JWT_PUBLIC_KEY_PATH"); ok {
		key, err := readRSAPublicKey(path)
		if err != nil {
			return nil, fmt.Errorf("reading AUTH_JWT_PUBLIC_KEY_PATH: %w", err)
		}
		publicKey = key
	}
	jwksURL, ok := runtime.Optional("JWKS_URL")
	if !ok {
		if publicKey == nil {
			return nil, errors.New("set JWKS_URL (or AUTH_JWT_PUBLIC_KEY_PATH for static RSA)")
		}
		return &jwks.TokenVerifier{Kind: jwks.VerifierStatic, PublicKey: publicKey, Leeway: 20 * time.Second}, nil
	}
	client := jwks.NewJwksClient(jwksURL, time.Duration(runtime.EnvOr("JWKS_CACHE_TTL_SECONDS", 300))*time.Second)
	client.SpawnBackgroundRefresh(ctx, time.Duration(runtime.EnvOr("JWKS_REFRESH_SECONDS", 60))*time.Second)
	return &jwks.TokenVerifier{Kind: jwks.VerifierAuto, Keys: client, PublicKey: publicKey, Leeway: 20 * time.Second}, nil
}

func readRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, jwt.ErrKeyMustBePEMEncoded
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}
	return jwt.ParseRSAPublicKeyFromPEM(data)
}

func main() {
	host, _ := runtime.Optional("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := runtime.EnvOr("PORT", 4000)
	ctx := context.Background()
	verifier, err := buildVerifier(ctx)
	if err != nil {
		slog.Error("configuring token verification", "error", err)
		os.Exit(1)
	}

	httpClient := runtime.DefaultHTTPClient()

	var upstreams []upstreamConfig
	for n := 1; ; n++ {
		u, ok := loadUpstream(n)
		if !ok {
			break
		}
		upstreams = append(upstreams, u)
	}
	byName := make(map[string]upstreamConfig, len(upstreams))
	backendByName := make(map[string]*grpcapi.GrpcBackend, len(upstreams))
	for _, u := range upstreams {
		byName[u.name] = u
		token, err := runtime.StartServiceTokenSource(ctx, httpClient, u.loginURL, u.serviceEmail, u.servicePassword)
		if err != nil {
			slog.Error("logging into upstream", "upstream", u.name, "error", err)
			os.Exit(1)
		}
		backend, err := grpcapi.ConnectGrpcBackend(u.grpcAddr, token)
		if err != nil {
			slog.Error("connecting to upstream gRPC", "upstream", u.name, "addr", u.grpcAddr, "error", err)
			os.Exit(1)
		}
		backendByName[u.name] = backend
		slog.Info("connected to upstream", "upstream", u.name, "grpc_addr", u.grpcAddr)
	}

	zonesBackend, ok := backendByName["zones"]
	if !ok {
		slog.Error("no upstream named 'zones' configured — set UPSTREAM_N_NAME=zones (see .env.example)")
		os.Exit(1)
	}
	scanningBackend, ok := backendByName["scanning"]
	if !ok {
		slog.Error("no upstream named 'scanning' configured — set UPSTREAM_N_NAME=scanning (see .env.example)")
		os.Exit(1)
	}
	alertingBackend, ok := backendByName["alerting"]
	if !ok {
		slog.Error("no upstream named 'alerting' configured — set UPSTREAM_N_NAME=alerting (see .env.example)")
		os.Exit(1)
	}

	// Which upstream owns each entity. The entity shapes themselves come from the metadata
	// registry, so a field added to an entity shows up here without touching this file.
	owners := map[string]crud.RecordBackend{
		"waf.domains":             zonesBackend,
		"waf.zones":               zonesBackend,
		"waf.ddos_policies":       zonesBackend,
		"waf.firewall_rules":      zonesBackend,
		"waf.ip_access_lists":     zonesBackend,
		"waf.scan_jobs":           scanningBackend,
		"waf.scan_findings":       scanningBackend,
		"waf.alert_policies":      alertingBackend,
		"waf.alert_notifications": alertingBackend,
		"waf.security_events":     alertingBackend,
		"waf.incidents":           alertingBackend,
	}
	registry := metadata.NewMetadataRegistry()
	if err := registry.RegisterAllSubmitted(); err != nil {
		slog.Error("registering WAF entity metadata", "error", err)
		os.Exit(1)
	}

	rest := graphqlgateway.RestURLs{
		Zones:    byName["zones"].restURL,
		Scanning: byName["scanning"].restURL,
		Alerting: byName["alerting"].restURL,
	}

	schema, err := graphqlgateway.BuildSchema(registry, owners, rest)
	if err != nil {
		slog.Error("building GraphQL schema", "error", err)
		os.Exit(1)
	}

	defaults := graphqlgateway.DefaultLimits()
	limits := graphqlgateway.Limits{
		MaxDepth:      runtime.EnvOr("GRAPHQL_MAX_DEPTH", defaults.MaxDepth),
		MaxComplexity: runtime.EnvOr("GRAPHQL_MAX_COMPLEXITY", defaults.MaxComplexity),
	}
	server := graphqlgateway.NewServer(schema, verifier, limits)

	corsOrigins, _ := runtime.Optional("CORS_ORIGINS")
	var origins []string
	if corsOrigins != "" {
		origins = strings.Split(corsOrigins, ",")
	}
	handler := runtime.CORS(runtime.CORSOptions{
		Origins: origins,
		Methods: []string{"GET", "POST", "OPTIONS"},
		Headers: []string{"Authorization", "Content-Type", "X-CSRF-Token"},
	})(server.Router())

	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("waf-graphql-gateway listening", "addr", addr, "entities", len(owners))
	if err := runtime.Serve(addr, handler); err != nil && err != http.ErrServerClosed {
		slog.Error("waf-graphql-gateway exited with an error", "error", err)
		os.Exit(1)
	}
}
