// waf-graphql-gateway — WAF's own GraphQL BFF.
//
// A bespoke, forwarding gateway (confirmed with the project owner, since metap-go never got a
// GraphQL layer at all — metap-graphql/metap-graphql-http/metap-graphql-gateway were never
// ported): every entity field is built from a hardcoded list of the 11 entities the 3 WAF
// services own (graphqlgateway.BuildSchema), forwarding to each upstream's gRPC RecordBackend
// (grpcapi.GrpcBackend, the same client the 3 Go WAF services already use for their own
// cross-service calls); the 8 custom business-action mutations forward to each service's REST
// endpoint instead, since those have no gRPC equivalent.
//
// Same disclosed divergence as the 3 WAF Go services: no JWKS trust-root dual-dispatch — this
// gateway decode-verifies against the SAME static RSA keypair file the 3 services use
// (AUTH_JWT_PUBLIC_KEY_PATH), not a separate gateway-specific key and not metap-jwks.
package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/metap-org/metap/pkg/grpcapi"
	"github.com/metap-org/metap/pkg/runtime"

	"github.com/metap-org/waap/data-plane/graphqlgateway"
)

type upstreamConfig struct {
	name            string
	grpcAddr        string
	loginURL        string
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
	return upstreamConfig{name: name, grpcAddr: stripScheme(grpcAddr), loginURL: loginURL, serviceEmail: email, servicePassword: password}, true
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
	publicKeyPath, ok := runtime.Optional("AUTH_JWT_PUBLIC_KEY_PATH")
	if !ok {
		slog.Error("AUTH_JWT_PUBLIC_KEY_PATH is required")
		os.Exit(1)
	}
	publicKey, err := readRSAPublicKey(publicKeyPath)
	if err != nil {
		slog.Error("reading AUTH_JWT_PUBLIC_KEY_PATH", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
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

	// Order is cosmetic (a map in BuildSchema either way) — order is load-bearing nowhere here
	// since each entity's name is globally unique and the backend map is built from it.
	entities := []graphqlgateway.EntityBackend{
		{Name: "waf.domains", Backend: zonesBackend},
		{Name: "waf.zones", Backend: zonesBackend, HasWorkflow: true},
		{Name: "waf.ddos_policies", Backend: zonesBackend},
		{Name: "waf.firewall_rules", Backend: zonesBackend},
		{Name: "waf.ip_access_lists", Backend: zonesBackend},
		{Name: "waf.scan_jobs", Backend: scanningBackend, HasWorkflow: true},
		{Name: "waf.scan_findings", Backend: scanningBackend, HasWorkflow: true},
		{Name: "waf.alert_policies", Backend: alertingBackend},
		{Name: "waf.alert_notifications", Backend: alertingBackend},
		{Name: "waf.security_events", Backend: alertingBackend},
		{Name: "waf.incidents", Backend: alertingBackend, HasWorkflow: true},
	}

	rest := graphqlgateway.RestURLs{
		Zones:    restBaseFromLoginURL(byName["zones"].loginURL),
		Scanning: restBaseFromLoginURL(byName["scanning"].loginURL),
		Alerting: restBaseFromLoginURL(byName["alerting"].loginURL),
	}

	schema, err := graphqlgateway.BuildSchema(entities, rest)
	if err != nil {
		slog.Error("building GraphQL schema", "error", err)
		os.Exit(1)
	}

	server := graphqlgateway.NewServer(schema, publicKey)

	corsOrigins, _ := runtime.Optional("CORS_ORIGINS")
	var origins []string
	if corsOrigins != "" {
		origins = strings.Split(corsOrigins, ",")
	}
	handler := runtime.CORS(runtime.CORSOptions{
		Origins: origins,
		Methods: []string{"GET", "POST", "OPTIONS"},
		Headers: []string{"Authorization", "Content-Type"},
	})(server.Router())

	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("waf-graphql-gateway listening", "addr", addr, "entities", len(entities))
	if err := runtime.Serve(addr, handler); err != nil && err != http.ErrServerClosed {
		slog.Error("waf-graphql-gateway exited with an error", "error", err)
		os.Exit(1)
	}
}
