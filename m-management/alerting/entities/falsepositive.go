// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.false_positive_reports — "this request was blocked and should not have been" (UC-61, UC-62).
// A report names the zone, the blocked request (its id and path, and the client address the edge saw)
// and what the reporter says; the SOC then dismisses it or resolves it, usually by letting the
// address through (exceptionEntryId points at that IP list entry) and replies in resolutionNote.
//
// Reports are entered in the portal (by the SOC from a forwarded request id). Intake from the block
// page itself needs the edge to show a request id and a report form, which it does not yet.
//
// zoneId is a plain string for the same reason as Incident.zoneId: zones live in another service.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func FalsePositiveEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.FalsePositives,
		Label:     "False positive report",
		TableName: reconciler.QualifiedTableNameIn(model.FalsePositives, "waf"),
		Fields: []metadata.EntityField{
			metadata.NewField(model.FieldZoneID, "Zone", metadata.KindString).Require().Index().RefersTo(model.Zones),
			metadata.NewField(model.FieldRequestID, "Request ID", metadata.KindString).Index(),
			metadata.NewField(model.FieldSourceIP, "Client IP", metadata.KindString).Index(),
			metadata.NewField(model.FieldRequestPath, "Request path", metadata.KindString),
			metadata.NewField(model.FieldDescription, "What happened", metadata.KindString).Require(),
			metadata.NewField(model.FieldReporterContact, "Reporter contact", metadata.KindString),
			metadata.EnumField(model.FieldStatus, "Status", "open", "dismissed", "resolved").Index(),
			metadata.NewField(model.FieldResolutionNote, "Reply", metadata.KindString),
			// The IP list entry that lets this client through, when the SOC created one.
			metadata.NewField(model.FieldExceptionEntryID, "Exception entry", metadata.KindString),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{model.FieldZoneID, "sourceIp", "requestPath", "description", "status"},
			Filters:     []string{model.FieldZoneID, "status"},
			DefaultSort: metadata.Ptr("-createdAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField:     model.FieldStatus,
			InitialState:   model.FalsePositiveOpen,
			TerminalStates: []string{model.FalsePositiveDismissed, model.FalsePositiveResolved},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition(model.FalsePositiveDismiss, model.FalsePositiveOpen, model.FalsePositiveDismissed, "Dismiss"),
				metadata.Transition(model.FalsePositiveResolve, model.FalsePositiveOpen, model.FalsePositiveResolved, "Resolve"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(FalsePositiveEntity) }
