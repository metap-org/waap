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
)

func IncidentEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.incidents",
		Label:     "Incident",
		TableName: reconciler.QualifiedTableNameIn("waf.incidents", "waf"),
		Fields: []metadata.EntityField{
			field("zoneId", "Zone", metadata.KindString, true, true, false),
			field("title", "Title", metadata.KindString, true, false, true),
			enumField("severity", "Severity", []string{"low", "medium", "high", "critical"}, true, true),
			enumField("status", "Status", []string{"open", "acknowledged", "mitigating", "resolved"}, false, true),
			field("eventCount", "Event Count", metadata.KindNumber, false, false, true),
			field("assignedTo", "Assigned To", metadata.KindString, false, true, false),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"title", "zoneId", "severity", "status", "assignedTo"},
			Filters:     []string{"zoneId", "severity", "status", "assignedTo"},
			DefaultSort: strPtr("-createdAt"),
			MaxLimit:    50,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField:     "status",
			InitialState:   "open",
			TerminalStates: []string{"resolved"},
			Transitions: []metadata.WorkflowTransition{
				transitionNoGuard("acknowledge", "open", "acknowledged", "Acknowledge"),
				transitionNoGuard("startMitigating", "acknowledged", "mitigating", "Start Mitigating"),
				transitionNoGuard("resolve", "mitigating", "resolved", "Resolve"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(IncidentEntity) }

// assignedTo is a metap user id (see the entity's own doc comment for why it can't be a
// Reference) — this tells @metap/platform-ui's generic list/detail views to resolve it to an
// email via GET /users instead of showing the raw id.
func incidentFieldDisplayHints() []metadata.FieldDisplayHint {
	return []metadata.FieldDisplayHint{
		{Field: "assignedTo", ResolveVia: strPtr("users")},
	}
}

func init() { metadata.SubmitFieldDisplayHints("waf.incidents", incidentFieldDisplayHints) }
