// Boot sequence for scanning-service — 1 of 3 WAF Customer Portal backend services, ported from
// Rust's management-plane/services/scanning-service.
//
// Owns waf.scan_jobs + waf.scan_findings. ScanJob.zoneId (pointing at zones-service's waf.zones)
// is a plain string, not Reference — see entities/scanjob.go's own doc comment for why. All 3
// services point at the SAME tenant database (Router.PoolFor) — split by compute/deploy/
// ownership, not by data layer, so Reference fields within this service's own entities
// (ScanFinding.scanJobId) keep a real Postgres FK.
//
// Same disclosed divergences as zones-service: no JWKS trust-root dual-dispatch, no RouteGroups
// toggle — see that binary's own doc comment.
package main

import (
	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/metadata"

	"github.com/metap-org/waap/management-plane/scanningservice"
	"github.com/metap-org/waap/management-plane/scanningservice/entities"
)

func main() {
	app.Main(app.Options{
		Name:     "scanning-service",
		Entities: []metadata.EntityDefinition{entities.ScanJobEntity(), entities.ScanFindingEntity()},
		GRPCPort: 3011,
		Routes:   scanningservice.MountRoutes,
	})
}
