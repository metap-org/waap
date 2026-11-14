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
	"github.com/metap-org/waap/model"
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
	{"verifyZoneDns", model.FieldZoneID, model.Zones, model.ActionVerifyDNS, func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"verifyDomainDns", model.FieldDomainID, model.Domains, model.ActionVerifyDNS, func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"testZoneOrigin", model.FieldZoneID, model.Zones, model.ActionTestOrigin, func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"syncZoneConfigState", model.FieldZoneID, model.Zones, model.ActionSyncConfigState, func(i Invokers) crud.ActionInvoker { return i.Zones }},
	{"runScanJob", "jobId", model.ScanJobs, "run", func(i Invokers) crud.ActionInvoker { return i.Scanning }},
	{"testAlertPolicy", "policyId", model.AlertPolicies, "test", func(i Invokers) crud.ActionInvoker { return i.Alerting }},
	{"evaluateAlerts", "", model.AlertPolicies, model.ActionEvaluate, func(i Invokers) crud.ActionInvoker { return i.Alerting }},
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
			Args: graphql.FieldConfigArgument{model.FieldZoneID: &graphql.ArgumentConfig{Type: graphql.String}},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				call := crud.ActionCall{Entity: model.Incidents, Name: model.ActionCorrelate, RC: gw.RequestContextFrom(p.Context), Input: crud.JsonObject{}}
				if zoneID, ok := p.Args[model.FieldZoneID].(string); ok && zoneID != "" {
					raw, err := json.Marshal(zoneID)
					if err != nil {
						return nil, err
					}
					call.Input[model.FieldZoneID] = raw
				}
				return gw.InvokeAction(p.Context, inv.Alerting, call)
			},
		}
	}
}
