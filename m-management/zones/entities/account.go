// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.accounts — a unit inside a tenant (an org) that groups zones: a business unit, a project, a
// customer of an agency (UC-02). A role can be held only inside one account (metap's scoped roles,
// scope type "account"), so a team sees and edits only the zones of its own account.
//
// Every tenant has at least the "default" account, created with its first zone, so a tenant that
// never uses accounts never sees them. An account is closed, not deleted, and only once none of its
// zones is running (guardedbackend); a closed account takes no new zones.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func AccountEntity() metadata.EntityDefinition {
	// Unique per tenant (the default scope), among accounts that still exist.
	slug := metadata.NewField(model.FieldSlug, "Slug", metadata.KindString).Require().Index()
	slug.Unique = metadata.Ptr(true)
	name := metadata.NewField(model.FieldName, "Name", metadata.KindString).Require().Sort()
	name.Searchable = metadata.Ptr(true)

	return metadata.EntityDefinition{
		Name:      model.Accounts,
		Label:     "Account",
		TableName: reconciler.QualifiedTableNameIn(model.Accounts, "waf"),
		Fields: []metadata.EntityField{
			name,
			slug,
			metadata.EnumField(model.FieldStatus, "Status", model.AccountActive, model.AccountClosed).Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:         []string{"name", "slug", "status"},
			Filters:        []string{"status", model.FieldSlug},
			RequiredFields: []string{"name"},
			DefaultSort:    metadata.Ptr("name"),
			MaxLimit:       model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: model.FieldStatus, InitialState: model.AccountActive, TerminalStates: []string{model.AccountClosed},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition(model.AccountClose, model.AccountActive, model.AccountClosed, "Close"),
			},
		},
		Scopes: []permission.ScopeBinding{metadata.ScopeSelf(model.ScopeAccount)},
		Audit:  &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(AccountEntity) }
