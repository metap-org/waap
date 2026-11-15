// waf.incidents — groups related SecurityEvents into 1 SOC-manageable case. See
// docs/02-domain-model.md's Incident section. assignedTo is a plain string (a user id/email), not
// Reference — metap's users live in the platform-level control schema, not as a registered
// EntityDefinition in this app's own MetadataRegistry, so ValidateReferences would reject a
// Reference pointing at it.
//
// zoneId is a plain string for the same class of reason, since the pillar split: waf.zones now
// lives in a separate binary (zones-service), and registering a zone entity here just to pass
// ValidateReferences would leak full CRUD for waf.zones onto this service's route.
//
// The correlation logic that actually creates an Incident from N SecurityEvents is out of this
// entity's/portal's scope — real business logic, not CRUD. This entity is just where the result
// lands.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func IncidentEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.Incidents,
		Label:     "Incident",
		TableName: reconciler.QualifiedTableNameIn(model.Incidents, "waf"),
		Fields: []metadata.EntityField{
			metadata.NewField(model.FieldZoneID, "Zone", metadata.KindString).Require().Index(),
			metadata.NewField(model.FieldTitle, "Title", metadata.KindString).Require().Sort(),
			metadata.EnumField(model.FieldSeverity, "Severity", "low", "medium", "high", "critical").Require().Index(),
			metadata.EnumField(model.FieldStatus, "Status", "open", "acknowledged", "mitigating", "resolved").Index(),
			metadata.NewField(model.FieldEventCount, "Event Count", metadata.KindNumber).Sort(),
			metadata.NewField(model.FieldAssignedTo, "Assigned To", metadata.KindString).Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"title", model.FieldZoneID, "severity", "status", "assignedTo"},
			Filters:     []string{model.FieldZoneID, "severity", "status", "assignedTo"},
			DefaultSort: metadata.Ptr("-createdAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField:     model.FieldStatus,
			InitialState:   model.IncidentOpen,
			TerminalStates: []string{model.IncidentResolved},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition(model.IncidentAcknowledge, model.IncidentOpen, model.IncidentAcknowledged, "Acknowledge"),
				metadata.Transition(model.IncidentStartMitigating, model.IncidentAcknowledged, model.IncidentMitigating, "Start Mitigating"),
				metadata.Transition(model.IncidentResolve, model.IncidentMitigating, model.IncidentResolved, "Resolve"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionCorrelate, Label: "Correlate incidents", Description: "Opens incidents for recent events grouped by zone and source IP.", Scope: metadata.ActionScopeEntity},
		},
	}
}

func init() { metadata.SubmitEntity(IncidentEntity) }

// assignedTo is a metap user id (see the entity's own doc comment for why it can't be a
// Reference) — this tells @metap/platform-ui's generic list/detail views to resolve it to an
// email via GET /users instead of showing the raw id.
func incidentFieldDisplayHints() []metadata.FieldDisplayHint {
	return []metadata.FieldDisplayHint{
		{Field: "assignedTo", ResolveVia: metadata.Ptr("users")},
	}
}

func init() { metadata.SubmitFieldDisplayHints(model.Incidents, incidentFieldDisplayHints) }
