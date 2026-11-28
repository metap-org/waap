// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.change_requests: a proposed change to one zone's policy (UC-40, 43 to 46). It holds the
// operations to make, the diff and check of the last time they were tried, who wrote it and who
// decided. Publishing it is the only way its operations reach the zone's records.
//
// Nobody writes these rows over REST, the admin role included (guard/changecontrol.go): the status,
// the diff and the approver are only ever what the actions below computed (package changes).
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func ChangeRequestEntity() metadata.EntityDefinition {
	action := func(name, label, description string) metadata.EntityAction {
		// Requires "read": each handler decides who may act (the author, or someone who may make the
		// writes the change contains).
		return metadata.EntityAction{Name: name, Label: label, Description: description, Scope: metadata.ActionScopeRecord, Requires: "read"}
	}
	return metadata.EntityDefinition{
		Name:      model.ChangeRequests,
		Label:     "Change Request",
		TableName: reconciler.QualifiedTableNameIn(model.ChangeRequests, "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField(model.FieldZoneID, "Zone", model.Zones, "hostname").Require().Index(),
			// Copied from the zone, so a role held inside an account reaches it.
			metadata.NewField(model.FieldAccountID, "Account", metadata.KindString).Index(),
			metadata.NewField(model.FieldTitle, "Title", metadata.KindString).Require().Sort(),
			metadata.NewField(model.FieldDescription, "Description", metadata.KindString),
			metadata.EnumField(model.FieldStatus, "Status",
				model.ChangeDraft, model.ChangePendingReview, model.ChangePublished, model.ChangeRejected, model.ChangeCancelled).Require().Index(),
			metadata.NewField(model.FieldOperations, "Operations", metadata.KindJSON).Require(),
			metadata.NewField(model.FieldDiff, "Diff", metadata.KindJSON),
			metadata.NewField(model.FieldCheckResult, "Check result", metadata.KindJSON),
			// The zone's policy the diff was made against; publishing refuses if it has moved.
			metadata.NewField(model.FieldBaseHash, "Base content hash", metadata.KindString),
			metadata.NewField(model.FieldAuthorID, "Author", metadata.KindString).Require().Index(),
			metadata.NewField(model.FieldSubmittedAt, "Submitted at", metadata.KindDatetime).Sort(),
			metadata.NewField(model.FieldDecidedBy, "Decided by", metadata.KindString),
			metadata.NewField(model.FieldDecidedAt, "Decided at", metadata.KindDatetime),
			metadata.EnumField(model.FieldDecision, "Decision", model.DecisionApproved, model.DecisionSelfConfirmed, model.DecisionRejected),
			metadata.NewField(model.FieldDecisionComment, "Comment", metadata.KindString),
			metadata.NewField(model.FieldPublishedVersion, "Published version", metadata.KindNumber),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:   []string{model.FieldZoneID, "title", "status", "authorId", "submittedAt", "decidedBy"},
			Filters:  []string{model.FieldZoneID, "status", "authorId"},
			MaxLimit: model.DefaultMaxListLimit,
		}},
		Actions: []metadata.EntityAction{
			action(model.ActionReviseChange, "Revise", "The author replaces the operations; the change goes back to draft."),
			action(model.ActionSubmitChange, "Submit", "The author asks for the diff to be approved."),
			action(model.ActionApproveChange, "Approve and publish", "Publishes the approved diff. Confirming your own needs an account without four-eyes."),
			action(model.ActionRejectChange, "Reject", "Sends the change back to its author with a comment."),
			action(model.ActionCancelChange, "Cancel", "Closes the change without publishing it."),
		},
		Scopes: []permission.ScopeBinding{metadata.Scope(model.ScopeAccount, model.FieldAccountID)},
		Audit:  &metadata.EntityAuditConfig{Enabled: true, Parent: model.FieldZoneID},
	}
}

func init() { metadata.SubmitEntity(ChangeRequestEntity) }
