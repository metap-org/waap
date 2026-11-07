// Package config is edge node configuration, read once at boot from the environment.
//
// Hand-rolled rather than reusing any shared helper: edge-plane deliberately does not depend on
// metap (or on control-plane/go) at all. Ten lines of os.Getenv is a much smaller cost than any
// dependency that would drag in.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/metap-org/metap/pkg/kit/env"
)

type Config struct {
	ListenAddr string
	// RedisURL: where compiled rule-sets are read from. The only thing this process reads
	// config from — never data-plane.
	RedisURL string
	// RefreshInterval: how often the snapshot is refreshed. This value IS the
	// config-propagation SLA (10-30s in the architecture doc).
	RefreshInterval time.Duration
	// IngestURL: control-plane's ingest endpoint. Telemetry goes here, never to data-plane.
	IngestURL              string
	IngestToken            *string
	TelemetryBuffer        int
	TelemetryMaxBatch      int
	TelemetryFlushInterval time.Duration
	// ClientIPHeader: header carrying the client's real IP when this node sits behind another
	// proxy or an L4 load balancer. Unset by default, and that is the safe default: trusting a
	// client-supplied header would let anyone spoof their source IP past every IP rule and every
	// rate limit. Only set this when something in front is guaranteed to overwrite it.
	ClientIPHeader *string
	// GeoCountryHeader: header carrying an already-resolved ISO-3166 country code. There is no
	// GeoIP database in this build; country rules simply never match unless something upstream
	// provides this.
	GeoCountryHeader *string
	ClearanceCookie  string
	ClearanceTTL     time.Duration
	// ClearanceSecret: secret the clearance cookie is keyed with. A per-node random default
	// means a restart invalidates outstanding clearances (a re-challenge, not an outage); set it
	// explicitly to share clearances across a fleet.
	ClearanceSecret string
}

func Load() Config {
	return Config{
		ListenAddr:             env.String("LISTEN_ADDR", "0.0.0.0:8080"),
		RedisURL:               env.String("REDIS_URL", "redis://localhost:6379"),
		RefreshInterval:        env.Seconds("REFRESH_INTERVAL_SECONDS", 10*time.Second),
		IngestURL:              env.String("INGEST_URL", "http://localhost:4100/ingest/events"),
		IngestToken:            env.Ptr("INGEST_TOKEN"),
		TelemetryBuffer:        env.Int("TELEMETRY_BUFFER", 10_000),
		TelemetryMaxBatch:      env.Int("TELEMETRY_MAX_BATCH", 100),
		TelemetryFlushInterval: env.Millis("TELEMETRY_FLUSH_MS", 2_000*time.Millisecond),
		ClientIPHeader:         env.Ptr("CLIENT_IP_HEADER"),
		GeoCountryHeader:       env.Ptr("GEO_COUNTRY_HEADER"),
		ClearanceCookie:        env.String("CLEARANCE_COOKIE", "waf_clearance"),
		ClearanceTTL:           env.Seconds("CLEARANCE_TTL_SECONDS", 1_800*time.Second),
		ClearanceSecret:        env.String("CLEARANCE_SECRET", randomSecret()),
	}
}

// randomSecret: node-local clearance-cookie signing key when none is configured. It must be
// unguessable — it is the HMAC key behind every clearance cookie — so it comes from crypto/rand
// (a timestamp-derived value, as this used to be, is brute-forceable offline from the node's
// approximate start time). Two nodes, or two runs, never accept each other's cookies; set
// CLEARANCE_SECRET to share one across nodes.
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return "edge-" + hex.EncodeToString(b)
}
