// Layer: APP · config. Reads the environment once, in cmd/; the sweep limits.

package alerting

import (
	"time"

	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/waap/m-management/analytics"
)

// Default correlation window/threshold — the settled v1 rule. Overridable per request only so an
// operator can re-run a wider sweep by hand; the scheduled call uses the defaults.
const (
	defaultWindowMinutes = 15
	defaultThreshold     = 5
	// The window a caller may ask for, in minutes.
	minWindowMinutes = 1
	maxWindowMinutes = 24 * 60
	// How many recent events one correlation pass reads. Bounded on purpose: this is a periodic
	// sweep, not a backfill, and an unbounded read of the highest-volume entity in the product is
	// the last thing this service should do on a timer.
	eventScanLimit = 200
)

// deliveryTimeout bounds one alert delivery (webhook or similar) so a slow endpoint cannot stall a sweep.
const deliveryTimeout = 10 * time.Second

// recordScanLimit is how many incidents or alert policies one sweep reads.
const recordScanLimit = 50

// Config is what alerting-service reads from its environment, once, in cmd/.
type Config struct {
	// AnalyticsURL is the Analytics plane's query API, where security events are read from.
	AnalyticsURL string
	// AllowPrivateTargets lets a webhook reach private addresses and use http (dev only, see notify.go).
	AllowPrivateTargets bool
}

// LoadConfig reads the settings: the environment, then the config file (CONFIG_FILE), then the default.
func LoadConfig() Config {
	return Config{
		AnalyticsURL:        env.String("ANALYTICS_URL", "http://localhost:3030"),
		AllowPrivateTargets: env.String("ALLOW_PRIVATE_TARGETS", "") == "true",
	}
}

// analytics is the client of the Analytics plane.
func (c Config) analytics() *analytics.Client { return analytics.NewClient(c.AnalyticsURL) }
