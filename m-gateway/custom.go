// Package wafgateway holds what is WAF-specific about the Customer Portal GraphQL gateway; the
// generic gateway (schema from metadata, auth, limits, platform fields) is metap's pkg/graphqlgateway.
//
// 8 custom, non-CRUD mutations — real WAF business actions (DNS verification, scan dispatch,
// incident correlation, ...) that no generic entity field can synthesize. Every resolver here is
// a thin proxy: forward the caller's own bearer token to the exact REST endpoint that already
// implements the real logic (each WAF service's own src/routes.go), and hand back its JSON
// response verbatim as the JSON scalar. No business logic is duplicated here — mirrors the Rust
// gateway's add_custom_fields exactly. (A 9th, cleanupSecurityEvents, existed until 2026-11-08:
// security events moved to the Analytics plane, whose ClickHouse TTL replaced that sweep.)
package wafgateway

import (
	"fmt"

	"github.com/graphql-go/graphql"

	gw "github.com/metap-org/metap/pkg/graphqlgateway"
)

// RestURLs are the 3 upstreams' REST base URLs (e.g. http://localhost:3000) — used only by the
// custom mutations below; every generic CRUD field goes over gRPC instead.
type RestURLs struct {
	Zones, Scanning, Alerting string
}

// AddCustomFields adds the WAF business-action mutations; pass it to graphqlgateway.WithExtensions.
func AddCustomFields(rest RestURLs) func(query, mutation graphql.Fields) {
	return func(_, mutation graphql.Fields) {
		mutation["verifyZoneDns"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				zoneID, err := gw.IDArg(p, "zoneId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.zones/%s/verify-dns", rest.Zones, zoneID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["verifyDomainDns"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"domainId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				domainID, err := gw.IDArg(p, "domainId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.domains/%s/verify-dns", rest.Zones, domainID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["testZoneOrigin"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				zoneID, err := gw.IDArg(p, "zoneId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.zones/%s/test-origin", rest.Zones, zoneID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["syncZoneConfigState"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				zoneID, err := gw.IDArg(p, "zoneId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.zones/%s/sync-config-state", rest.Zones, zoneID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["runScanJob"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"jobId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				jobID, err := gw.IDArg(p, "jobId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.scan_jobs/%s/run", rest.Scanning, jobID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["testAlertPolicy"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"policyId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				policyID, err := gw.IDArg(p, "policyId")
				if err != nil {
					return nil, err
				}
				url := fmt.Sprintf("%s/api/waf.alert_policies/%s/test", rest.Alerting, policyID)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}

		mutation["correlateIncidents"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.String}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				url := fmt.Sprintf("%s/internal/incidents/correlate", rest.Alerting)
				body := map[string]interface{}{}
				if zoneID, ok := p.Args["zoneId"].(string); ok && zoneID != "" {
					body["zoneId"] = zoneID
				}
				return gw.ForwardPost(p.Context, url, body)
			},
		}

		mutation["evaluateAlerts"] = &graphql.Field{
			Type: gw.JSONScalar,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				url := fmt.Sprintf("%s/internal/alerts/evaluate", rest.Alerting)
				return gw.ForwardPost(p.Context, url, map[string]interface{}{})
			},
		}
	}
}
