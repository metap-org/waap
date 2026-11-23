// Layer: APP · wiring. Reads configuration and plugs the app's pieces into metap's app.Options; no business rules.

// management-service is the whole Management plane in one process: the Customer Portal's backend.
// It serves every management entity (zones, scanning, alerting, billing) and the portal talks to it
// directly; there is no gateway in front. Each domain lives in its own package and plugs into
// metap through the same points (docs/architecture/08-cross-cutting/21-metap-vs-app-map.md):
//
//   - zones:    zones, domains, DDoS policies, firewall rules, IP lists, accounts, policy versions
//   - scanning: scan jobs and findings
//   - alerting: alert policies, notifications, incidents, false positives
//   - billing:  plans, subscriptions, plan change requests
//
// This process is also the token issuer (JWKS_KEYS_DIR): it signs every token and publishes
// /.well-known/jwks.json, which analytics-api verifies against. Telemetry (waf.security_events) is a
// read model over the Analytics plane, the one thing here that lives outside this database.
package main

import (
	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/config"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"

	"github.com/metap-org/waap/m-management/alerting"
	"github.com/metap-org/waap/m-management/analytics"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/scanning"
	"github.com/metap-org/waap/m-management/zones"

	// Declared entities register themselves in init(); app.Main reconciles them, referenced first.
	_ "github.com/metap-org/waap/m-management/alerting/entities"
	_ "github.com/metap-org/waap/m-management/billing/entities"
	_ "github.com/metap-org/waap/m-management/scanning/entities"
	_ "github.com/metap-org/waap/m-management/zones/entities"
)

func main() {
	config.MustLoadFileFromEnv() // before any setting is read
	zonesCfg, billingCfg := zones.LoadConfig(), billing.LoadConfig()
	scanningCfg, alertingCfg := scanning.LoadConfig(), alerting.LoadConfig()

	app.Main(app.Options{
		Name: "management-service",
		// Every write to the REST record routes goes through the zones guard chain (it steps aside
		// for entities it has no rule for). Plan limits are asked of the billing domain, in-process.
		WrapBackend: func(state *httpserver.AppState) (crud.RecordBackend, error) {
			return zones.Guarded(state.Crud, billingCfg.PlanSource(state.Crud)), nil
		},
		Actions: func(state *httpserver.AppState, reg *crud.ActionRegistry) error {
			for _, register := range []func(*httpserver.AppState, *crud.ActionRegistry) error{
				zones.RegisterActions(zonesCfg, billingCfg.PlanSource(state.Crud)),
				scanning.RegisterActions(scanningCfg),
				alerting.RegisterActions(alertingCfg),
				billing.RegisterActions(billingCfg),
			} {
				if err := register(state, reg); err != nil {
					return err
				}
			}
			return nil
		},
		// Only the scanner's callback is a route of its own; everything else is an entity or an action.
		Routes: scanning.MountRoutes,
		// waf.security_events: a read model over the Analytics plane (ClickHouse), served here with
		// the caller's own token so the portal lists and aggregates it like any entity.
		ExternalEntities: []app.ExternalEntity{{
			Definition: analytics.SecurityEventEntity(),
			Backend: func(*httpserver.AppState) (crud.RecordBackend, error) {
				return &analytics.Backend{Client: analytics.NewClient(alertingCfg.AnalyticsURL)}, nil
			},
		}},
	})
}
