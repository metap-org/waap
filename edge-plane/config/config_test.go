package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUsesDefaultsWhenNothingIsSet(t *testing.T) {
	cfg := Load()
	assert.Equal(t, "0.0.0.0:8080", cfg.ListenAddr)
	assert.Equal(t, 10*time.Second, cfg.RefreshInterval)
	assert.Equal(t, 2*time.Second, cfg.TelemetryFlushInterval)
	assert.Equal(t, 30*time.Minute, cfg.ClearanceTTL)
	assert.Nil(t, cfg.IngestToken)
	assert.Nil(t, cfg.ClientIPHeader)
}

func TestLoadReadsEveryOverride(t *testing.T) {
	t.Setenv("REFRESH_INTERVAL_SECONDS", "3")
	t.Setenv("TELEMETRY_FLUSH_MS", "250")
	t.Setenv("TELEMETRY_BUFFER", "42")
	t.Setenv("CLIENT_IP_HEADER", "X-Real-IP")
	t.Setenv("CLEARANCE_SECRET", "shared")
	cfg := Load()
	assert.Equal(t, 3*time.Second, cfg.RefreshInterval)
	assert.Equal(t, 250*time.Millisecond, cfg.TelemetryFlushInterval)
	assert.Equal(t, 42, cfg.TelemetryBuffer)
	require.NotNil(t, cfg.ClientIPHeader)
	assert.Equal(t, "X-Real-IP", *cfg.ClientIPHeader)
	assert.Equal(t, "shared", cfg.ClearanceSecret)
}

func TestEmptyOptionalHeaderStaysUnset(t *testing.T) {
	t.Setenv("CLIENT_IP_HEADER", "")
	assert.Nil(t, Load().ClientIPHeader)
}

// The default clearance secret is an HMAC key: it must be high-entropy and differ per process.
func TestDefaultClearanceSecretIsRandomAndLong(t *testing.T) {
	a, b := randomSecret(), randomSecret()
	assert.NotEqual(t, a, b)
	assert.Len(t, a, len("edge-")+64)
}
