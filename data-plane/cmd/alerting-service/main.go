// Boot sequence for alerting-service — 1 of 3 WAF Customer Portal backend services, ported from
// Rust's data-plane/services/alerting-service.
//
// Owns waf.security_events + waf.incidents + waf.alert_policies + waf.alert_notifications — kept
// together because Incident correlation logic calls straight into CrudService in-process;
// splitting SecurityEvent/Incident across services would turn that into a cross-service call for
// no real benefit. SecurityEvent.zoneId/Incident.zoneId/Incident.assignedTo are plain strings,
// not Reference — see entities/securityevent.go and entities/incident.go's own doc comments for
// why. All 3 services point at the SAME tenant database (Router.PoolFor) — split by
// compute/deploy/ownership, not by data layer, so Reference fields within this service's own
// entities (AlertNotification.alertPolicyId) keep a real Postgres FK.
//
// Same disclosed divergences as zones-service/scanning-service: no JWKS trust-root dual-dispatch,
// no RouteGroups toggle — see zones-service's own doc comment.
package main

import (
	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/data-plane/alertingservice"
	"github.com/metap-org/waap/data-plane/alertingservice/entities"
)

func main() {
	app.Main(app.Options{
		Name: "alerting-service",
		Entities: []metadata.EntityDefinition{
			entities.AlertPolicyEntity(),
			entities.AlertNotificationEntity(),
			entities.SecurityEventEntity(),
			entities.IncidentEntity(),
		},
		GRPCPort: 3021,
		Routes:   alertingservice.MountRoutes,
	})
}
