// Package config loads process configuration, read once at boot.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/metap-org/metap/pkg/runtime"
)

type Config struct {
	// ZonesURL is where the wafZones/wafDdosPolicies/wafFirewallRules/wafIpAccessLists GraphQL
	// fields live — the GATEWAY, not zones-service's own port. Neither zones-service nor
	// alerting-service mounts /graphql itself — only the gateway aggregates all 3 management-plane
	// services' entities into one reachable schema.
	ZonesURL string
	// AnalyticsURL is the Analytics plane's analytics-api — edge telemetry is written there in
	// batches. AnalyticsIngestToken is its ingest secret (ANALYTICS_INGEST_TOKEN on both sides).
	AnalyticsURL         string
	AnalyticsIngestToken string
	// LoginURL: a real user this process logs in as, exactly like cron-scheduler does. Not a
	// hand-minted static JWT: that pattern already caused a live outage in metap when the
	// token's TTL expired in a running deployment.
	LoginURL        string
	ServiceEmail    string
	ServicePassword string

	RedisURL string
	AmqpURL  string
	// Queue is the durable queue name. Fixed (not per-instance) so several replicas of this
	// worker share the queue and compete for messages instead of each compiling the same change.
	Queue string

	// ResyncInterval: full reconcile interval. Events can be missed — a message lands in the
	// DLQ, a replica dies mid-handle, someone edits the database directly — so the incremental
	// path is an optimisation and this sweep is what actually guarantees convergence.
	ResyncInterval time.Duration
	// Port/Host: HTTP port for /health and the telemetry ingest endpoint.
	Port int
	Host string
	// IngestMaxBatch: ingest batching, flush when either bound is hit.
	IngestMaxBatch int
	// IngestMaxRequestEvents: reject an ingest request larger than this many events outright —
	// the edge is trusted, but an unbounded body on the highest-volume path in the system is how
	// one bad node takes the worker down.
	IngestMaxRequestEvents int
	// IngestToken: shared secret the edge presents on /ingest/events. nil in dev; when set, a
	// request without it is rejected.
	IngestToken *string
}

func envOrString(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

func envOrUint(name string, def int) int {
	if v, ok := os.LookupEnv(name); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func Load() (Config, error) {
	serviceEmail, err := runtime.RequireEnv("CONTROL_SERVICE_EMAIL")
	if err != nil {
		return Config{}, err
	}
	servicePassword, err := runtime.RequireEnv("CONTROL_SERVICE_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	var ingestToken *string
	if v, ok := runtime.Optional("INGEST_TOKEN"); ok {
		ingestToken = &v
	}
	resyncSeconds := envOrUint("RESYNC_INTERVAL_SECONDS", 60)

	return Config{
		ZonesURL:               envOrString("ZONES_URL", "http://localhost:4000"),
		AnalyticsURL:           envOrString("ANALYTICS_URL", "http://localhost:3030"),
		AnalyticsIngestToken:   envOrString("ANALYTICS_INGEST_TOKEN", ""),
		LoginURL:               envOrString("CONTROL_LOGIN_URL", "http://localhost:3000/auth/login"),
		ServiceEmail:           serviceEmail,
		ServicePassword:        servicePassword,
		RedisURL:               envOrString("REDIS_URL", "redis://localhost:6379"),
		AmqpURL:                envOrString("AMQP_URL", "amqp://guest:guest@localhost:5672/%2f"),
		Queue:                  envOrString("CONFIG_DISTRIBUTOR_QUEUE", "waf.config-distributor"),
		ResyncInterval:         time.Duration(resyncSeconds) * time.Second,
		Port:                   envOrUint("PORT", 4100),
		Host:                   envOrString("HOST", "0.0.0.0"),
		IngestMaxBatch:         envOrUint("INGEST_MAX_BATCH", 200),
		IngestMaxRequestEvents: envOrUint("INGEST_MAX_REQUEST_EVENTS", 1000),
		IngestToken:            ingestToken,
	}, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
