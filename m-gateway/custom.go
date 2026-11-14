// Package wafgateway holds what is WAF-specific about the Customer Portal GraphQL gateway; the
// generic gateway (schema from metadata, auth, limits, platform fields) is metap's pkg/graphqlgateway.
//
// Every WAF business action (DNS verification, scan dispatch, incident correlation, ...) is declared
// as an action on its entity (EntityDefinition.Actions) and served by the owning service over gRPC
// (ActionService). The generic gateway already exposes each one as "<action><TypeName>"
// (verifyDnsWafZones, ...); the mutations below keep the names and argument shapes the Customer
// Portal was built against and forward to the same gRPC action with the caller's own token, so
// they hold no logic. (A 9th, cleanupSecurityEvents, existed until 2026-11-08: security events
// moved to the Analytics plane, whose ClickHouse TTL replaced that sweep.)
package wafgateway

import (
	"encoding/json"

	"github.com/graphql-go/graphql"

	"github.com/metap-org/metap/pkg/crud"
	gw "github.com/metap-org/metap/pkg/graphqlgateway"
)

// RestURLs are the upstreams' REST base URLs (e.g. http://localhost:3000). Only the platform
// admin fields use them now; every entity action goes over gRPC.
type RestURLs struct {
	Zones, Scanning, Alerting string
}

// Invokers are the gRPC action clients of the three services.
type Invokers struct {
	Zones, Scanning, Alerting crud.ActionInvoker
}

type legacyMutation struct {
	name   string // mutation name the portal uses
	arg    string // its id argument, "" for an entity-scoped action
	entity string
	action string
	owner  func(Invokers) crud.ActionInvoker
}

var legacyMutations = []legacyMutation{
	{"verifyZoneDns", "zoneId", "waf.zones", "verifyDns", func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"verifyDomainDns", "domainId", "waf.domains", "verifyDns", func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"testZoneOrigin", "zoneId", "waf.zones", "testOrigin", func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"syncZoneConfigState", "zoneId", "waf.zones", "syncConfigState", func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"runScanJob", "jobId", "waf.scan_jobs", "run", func(i Invokers) crud.ActionInvoker { return i.Scanning }},
	{"testAlertPolicy", "policyId", "waf.alert_policies", "test", func(i Invokers) crud.ActionInvoker { return i.Alerting }},
	{"evaluateAlerts", "", "waf.alert_policies", "evaluate", func(i Invokers) crud.ActionInvoker { return i.Alerting }},
}

// AddCustomFields adds the portal-compatible mutations; pass it to graphqlgateway.WithExtensions.
func AddCustomFields(inv Invokers) func(query, mutation graphql.Fields) {
	return func(_, mutation graphql.Fields) {
		for _, m := range legacyMutations {
			m := m
			args := graphql.FieldConfigArgument{}
			if m.arg != "" {
				args[m.arg] = &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}
			}
			mutation[m.name] = &graphql.Field{
				Type: gw.JSONScalar,
				Args: args,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					call := crud.ActionCall{Entity: m.entity, Name: m.action, RC: gw.RequestContextFrom(p.Context)}
					if m.arg != "" {
						id, err := gw.IDArg(p, m.arg)
						if err != nil {
							return nil, err
						}
						call.ID, err = gw.ParseUUID(id)
						if err != nil {
							return nil, err
						}
					}
					return gw.InvokeAction(p.Context, m.owner(inv), call)
				},
			}
		}

		mutation["correlateIncidents"] = &graphql.Field{
			Type: gw.JSONScalar,
			Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.String}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				call := crud.ActionCall{Entity: "waf.incidents", Name: "correlate", RC: gw.RequestContextFrom(p.Context), Input: crud.JsonObject{}}
				if zoneID, ok := p.Args["zoneId"].(string); ok && zoneID != "" {
					raw, err := json.Marshal(zoneID)
					if err != nil {
						return nil, err
					}
					call.Input["zoneId"] = raw
				}
				return gw.InvokeAction(p.Context, inv.Alerting, call)
			},
		}
	}
}
