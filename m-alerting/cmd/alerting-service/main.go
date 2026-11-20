// Boot sequence for alerting-service — 1 of 3 WAF Customer Portal backend services, ported from
// Rust's management-plane/services/alerting-service.
//
// Owns waf.incidents + waf.alert_policies + waf.alert_notifications. SecurityEvents moved to the
// Analytics plane (2026-11-08): incident correlation and alert evaluation read them through its
// query API (ANALYTICS_URL, see alertingservice/routes.go) instead of in-process CrudService, and
// this service serves them to the portal as the read-only entity waf.security_events.
// Incident.zoneId/Incident.assignedTo are plain strings, not Reference — see
// entities/incident.go's own doc comment for why. All 3 services point at the SAME tenant database (Router.PoolFor) — split by
// compute/deploy/ownership, not by data layer, so Reference fields within this service's own
// entities (AlertNotification.alertPolicyId) keep a real Postgres FK.
//
// Same disclosed divergences as zones-service/scanning-service: no JWKS trust-root dual-dispatch,
// no RouteGroups toggle — see zones-service's own doc comment.
package main

import (
	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"

	analytics "github.com/metap-org/waap/m-analytics"

	"github.com/metap-org/waap/m-alerting"
	// Declared entities register themselves in init(); app.Main reconciles them, referenced first.
	_ "github.com/metap-org/waap/m-alerting/entities"
)

func main() {
	app.Main(app.Options{
		Name:    "alerting-service",
		Routes:  alertingservice.MountRoutes,
		Actions: alertingservice.RegisterActions,
		// waf.security_events: a read model over the Analytics plane (ClickHouse), served here
		// with the caller's own token so the portal lists and aggregates it like any entity.
		ExternalEntities: []app.ExternalEntity{{
			Definition: analytics.SecurityEventEntity(),
			Backend: func(*httpserver.AppState) (crud.RecordBackend, error) {
				return &analytics.Backend{Client: alertingservice.AnalyticsClient()}, nil
			},
		}},
	})
}
