// waf.security_events — log of 1 request matched by a DdosPolicy/FirewallRule, written by
// edge-plane (near real-time, high volume — see docs/02-domain-model.md's SecurityEvent
// section). No workflow — pure append-only log, never transitions state.
//
// triggeredById is a plain string, not Reference: a Reference field's RefEntity is one fixed
// target, but this needs to point at either waf.ddos_policies or waf.firewall_rules depending on
// triggeredBy — no polymorphic reference support in metap. triggeredByName denormalizes the
// rule/policy's name at write time — this entity is the highest-volume in the whole system, an
// N+1 lookup per list render doesn't scale.
//
// zoneId is also a plain string, not Reference, since the pillar split: waf.zones is owned by
// zones-service, a separate binary from this one, and registering a zone entity here just to
// satisfy ValidateReferences would leak full CRUD for waf.zones onto this service's route.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func SecurityEventEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.security_events",
		Label:     "Security Event",
		TableName: reconciler.QualifiedTableNameIn("waf.security_events", "waf"),
		Fields: []metadata.EntityField{
			field("zoneId", "Zone", metadata.KindString, true, true, false),
			enumField("triggeredBy", "Triggered By", []string{"ddosPolicy", "firewallRule"}, true, true),
			field("triggeredById", "Triggered By Id", metadata.KindString, true, false, false),
			field("triggeredByName", "Triggered By Name", metadata.KindString, false, false, false),
			enumField("action", "Action", []string{"logged", "challenged", "blocked"}, true, true),
			field("sourceIp", "Source IP", metadata.KindString, true, true, false),
			field("requestPath", "Request Path", metadata.KindString, true, false, false),
			field("occurredAt", "Occurred At", metadata.KindDatetime, true, true, true),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"zoneId", "triggeredByName", "action", "sourceIp", "requestPath", "occurredAt"},
			Filters:     []string{"zoneId", "triggeredBy", "action", "sourceIp"},
			DefaultSort: strPtr("-occurredAt"),
			MaxLimit:    50,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(SecurityEventEntity) }
