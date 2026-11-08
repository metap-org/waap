// Boot sequence for zones-service — 1 of 3 WAF Customer Portal backend services (zones-service/
// scanning-service/alerting-service), ported from Rust's management-plane/services/zones-service.
//
// Owns waf.domains + waf.zones + waf.ddos_policies + waf.firewall_rules + waf.ip_access_lists —
// kept together because Zone's workflow guard "activate" depends on the technical field
// hasConfig, updated by app logic whenever a DdosPolicy/FirewallRule is created/deleted for that
// zone; keeping that hook in-process (not a cross-service call) matters most at exactly this
// sensitive a spot (a workflow guard). All 3 services point at the SAME tenant database
// (Router.PoolFor) — split by compute/deploy/ownership, not by data layer, so Reference fields
// within this service's own entities keep a real Postgres FK.
//
// Divergence from the Rust original: no JWKS trust-root dual-dispatch (this Go port's
// httpserver.AppState only supports the static RSA keypair path — a metap limitation, not
// introduced here; all 3 WAF Go services share one RSA keypair instead, the same topology this
// app used before it adopted JWKS). Optional route groups (OAuth2, cron, dashboards,
// attachments) are app.Options.Modules; left nil, every service serves all of them, as before.
package main

import (
	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/management-plane/zonesservice"
	"github.com/metap-org/waap/management-plane/zonesservice/entities"
)

func main() {
	app.Main(app.Options{
		Name: "zones-service",
		// Reference targets first: domains <- zones <- ddos_policies/firewall_rules/ip_access_lists.
		Entities: []metadata.EntityDefinition{
			entities.DomainEntity(),
			entities.ZoneEntity(),
			entities.DdosPolicyEntity(),
			entities.FirewallRuleEntity(),
			entities.IPAccessListEntity(),
		},
		GRPCPort: 3001,
		// The gRPC API (what waf-graphql-gateway calls) goes through the zone/firewall/IP-list
		// guards and the cross-service zone-delete check.
		WrapBackend: func(state *httpserver.AppState) (crud.RecordBackend, error) {
			return zonesservice.NewGuardedZonesBackend(state.Crud,
				env.String("SCANNING_GRPC_ADDR", "localhost:3011"),
				env.String("ALERTING_GRPC_ADDR", "localhost:3021"),
			), nil
		},
		Routes: zonesservice.MountRoutes,
	})
}
