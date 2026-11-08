// analytics-api — the Analytics plane's first service (ADR-0001): batch ingest of security events
// and the structured query API over them, backed by ClickHouse.
//
// v1 scope, on purpose: control-plane posts batches here directly (no Kafka/Vector/enricher yet);
// Management's GraphQL gateway and alerting-service read through the query API with the caller's
// own token. The rest of docs/architecture 5.8.8–5.8.9 (ingest gateway with mTLS, Kafka topics,
// enricher, archiver, detection, exporter) is later work.
//
// Env:
//
//	PORT                       HTTP port (default 3030)
//	CLICKHOUSE_URL             clickhouse://user:pass@host:9000 (default clickhouse://default:@localhost:9000)
//	ANALYTICS_DATABASE         database holding the tables (default waf_analytics)
//	ANALYTICS_RETENTION_DAYS   security event TTL (default 30, the v1 retention decision)
//	JWKS_URL                   token issuer's JWKS (required) — zones-service in this product
//	JWKS_CACHE_TTL_SECONDS     how long a fetched key set is trusted (default 300)
//	ANALYTICS_INGEST_TOKEN     shared secret control-plane presents on ingest; unset = not checked (dev)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/kit/httpx"

	"github.com/metap-org/waap/analytics-plane/api"
	"github.com/metap-org/waap/analytics-plane/auth"
	"github.com/metap-org/waap/analytics-plane/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("analytics-api exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	jwksURL, err := env.Require("JWKS_URL")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var st *store.ClickHouse
	// ClickHouse often comes up a few seconds after this process in a fresh compose stack.
	for attempt := 1; ; attempt++ {
		st, err = store.Open(ctx, env.String("CLICKHOUSE_URL", "clickhouse://default:@localhost:9000"), env.String("ANALYTICS_DATABASE", "waf_analytics"))
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return fmt.Errorf("ClickHouse never became reachable: %w", err)
		}
		slog.Warn("waiting for ClickHouse", "attempt", attempt, "error", err)
		time.Sleep(2 * time.Second)
	}
	defer st.Close()
	if err := st.Migrate(ctx, env.Int("ANALYTICS_RETENTION_DAYS", 30)); err != nil {
		return err
	}

	ingestToken := env.String("ANALYTICS_INGEST_TOKEN", "")
	if ingestToken == "" {
		slog.Warn("ANALYTICS_INGEST_TOKEN is not set: any valid platform token can write events for its own tenant")
	}
	srv := &api.Server{
		Store:       st,
		Verifier:    auth.NewJWKSVerifier(jwksURL, env.Seconds("JWKS_CACHE_TTL_SECONDS", 300*time.Second)),
		IngestToken: ingestToken,
	}
	addr := fmt.Sprintf(":%d", env.Int("PORT", 3030))
	slog.Info("analytics-api listening", "addr", addr)
	return httpx.Serve(addr, srv.Handler())
}
