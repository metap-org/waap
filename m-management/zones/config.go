// Layer: APP · wiring. Reads configuration and plugs the app's pieces into metap's app.Options; no business rules.

package zones

import (
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/m-management/zones/netcheck"
)

// Config is everything the zones domain reads from its environment: what the outbound checks need.
// It is read once, in cmd/, and passed down; nothing below main calls env.*
// (docs/architecture/08-cross-cutting/20-code-structure-standard.md, 2.10).
type Config = netcheck.Config

// ConfigFromEnv reads the environment.
func ConfigFromEnv() Config {
	return Config{
		DoHURL:              env.String("DOH_RESOLVER_URL", "https://dns.google/resolve"),
		EdgeCNAMETarget:     env.String("EDGE_CNAME_TARGET", "edge.waf.local"),
		AllowPrivateTargets: env.String("ALLOW_PRIVATE_TARGETS", "") == "true",
	}
}

// Guarded is the backend every write goes through: validation, plan limits, accounts, versions and
// the zone-delete check (package guard). The one place it is built. plans may be nil (no limits).
func Guarded(inner crud.RecordBackend, plans billing.PlanLookup) *guard.Backend {
	return guard.New(inner).WithPlanLimits(plans)
}
