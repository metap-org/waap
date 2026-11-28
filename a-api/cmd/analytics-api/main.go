// analytics-api — the Analytics plane's first service (ADR-0001): batch ingest of security events
// and the structured query API over them, backed by ClickHouse.
//
// v1 scope, on purpose: control-plane posts batches here directly (no Kafka/Vector/enricher yet);
// management-service reads through the query API with a short-lived read token it signs
// for the caller. The rest of docs/architecture 5.8.8–5.8.9 (ingest gateway with mTLS, Kafka topics,
// enricher, archiver, detection, exporter) is later work.
//
// The service does not create or change its tables: run `analytics-api migrate up` first (and
// `analytics-api migrate retention` after changing ANALYTICS_RETENTION_DAYS); see migrate.go.
//
// Env:
//
//	PORT                       HTTP port (default 3030)
//	CLICKHOUSE_URL             clickhouse://user:pass@host:9000 (default clickhouse://default:@localhost:9000)
//	ANALYTICS_DATABASE         database holding the tables (default waf_analytics)
//	ANALYTICS_RETENTION_DAYS   security event TTL (default 30, the v1 retention decision)
//	JWKS_URL                   token issuer's JWKS (required) — management-service in this product
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

	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/waap/a-api/api"
	"github.com/metap-org/waap/a-api/auth"
	"github.com/metap-org/waap/a-api/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("analytics-api exited", standard.LogError, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var st *store.ClickHouse
	var err error
	// ClickHouse often comes up a few seconds after this process in a fresh compose stack.
	for attempt := 1; ; attempt++ {
		st, err = store.Open(ctx, env.String("CLICKHOUSE_URL", "clickhouse://default:@localhost:9000"), env.String("ANALYTICS_DATABASE", "waf_analytics"))
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return fmt.Errorf("ClickHouse never became reachable: %w", err)
		}
		slog.Warn("waiting for ClickHouse", standard.LogAttempt, attempt, standard.LogError, err)
		time.Sleep(2 * time.Second)
	}
	defer st.Close()
	retention := env.Int("ANALYTICS_RETENTION_DAYS", 30)
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return migrateCommand(ctx, st, retention, os.Args[2:])
	}
	// Only a check: the schema is changed by `analytics-api migrate`, never by a booting service.
	if err := st.CheckSchema(ctx, retention); err != nil {
		return err
	}

	jwksURL, err := env.Require("JWKS_URL")
	if err != nil {
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
