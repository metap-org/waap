// Package analytics is Management's side of the Analytics plane boundary (ADR-0001): a client
// for its query API, and a read-only crud.RecordBackend that lets the GraphQL gateway serve
// waf.security_events from it exactly as it serves any other entity — so the portal's existing
// list/aggregate calls keep working with the data living in ClickHouse instead of Postgres.
//
// waf.security_events is a READ MODEL here, not a stored entity: no service reconciles a table
// for it and nothing in Management can write one. Events are written by control-plane straight
// into the Analytics plane. This definition only describes the shape (fields, labels, list view,
// filters) for the gateway's schema and the portal's metadata endpoint.
//
// Every call forwards the caller's own token; Analytics re-verifies it and scopes by its tenant.
package analytics

import "github.com/metap-org/metap/pkg/metadata"

// SecurityEvents is the read-model entity name.
const SecurityEvents = "waf.security_events"

// SecurityEventEntity: same fields and list view the Postgres-backed entity had, so the portal
// and the GraphQL schema see no change. zoneId/triggeredById stay plain strings (no reference
// field), as before: zones live in another service and a rule id points at one of two entities.
func SecurityEventEntity() metadata.EntityDefinition {
	name := metadata.NewField("triggeredByName", "Triggered By Name", metadata.KindString)
	return metadata.EntityDefinition{
		Name:  SecurityEvents,
		Label: "Security Event",
		// The registry requires a schema-qualified table name. It is nominal: nothing ever
		// reconciles this entity, so no such Postgres table exists — the data is in ClickHouse.
		TableName: "analytics.security_events",
		Fields: []metadata.EntityField{
			metadata.NewField("zoneId", "Zone", metadata.KindString).Require().Index(),
			metadata.EnumField("triggeredBy", "Triggered By", "ddosPolicy", "firewallRule").Require().Index(),
			metadata.NewField("triggeredById", "Triggered By Id", metadata.KindString).Require(),
			name,
			metadata.EnumField("action", "Action", "logged", "challenged", "blocked").Require().Index(),
			metadata.NewField("sourceIp", "Source IP", metadata.KindString).Require().Index(),
			metadata.NewField("requestPath", "Request Path", metadata.KindString).Require(),
			metadata.NewField("occurredAt", "Occurred At", metadata.KindDatetime).Require().Index().Sort(),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"zoneId", "triggeredByName", "action", "sourceIp", "requestPath", "occurredAt"},
			Filters:     []string{"zoneId", "triggeredBy", "action", "sourceIp"},
			DefaultSort: metadata.Ptr("-occurredAt"),
			MaxLimit:    50,
		}},
	}
}
