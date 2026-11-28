// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.plans and waf.subscriptions — what each customer of the WAAP service may use (UC-70, 74).
//
// Owned by billing-service, a service of its own (zones-service asks it, over REST, what a customer
// may use). These two entities live in the OPERATOR's tenant, the one that runs the service
// (OPERATOR_TENANT_ID), and are edited by that tenant's admin. A customer is another tenant: its
// subscription is a row here naming the customer's tenant id, and billing-service reads the limits
// from the operator's rows when that customer creates a zone or rule. A customer's own admin sees
// its plan on the Plan page; it can write plan rows in its own tenant (an admin may do anything
// there) but nothing reads them, so they change nothing.
//
// This is waap's own domain (docs/management/12-billing-plans.md), independent of metap's tenant
// `tier` label. A limit of 0 means unlimited.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func PlanEntity() metadata.EntityDefinition {
	name := metadata.NewField(model.FieldName, "Name", metadata.KindString).Require().Sort()
	name.Unique = metadata.Ptr(true)
	return metadata.EntityDefinition{
		Name:      model.Plans,
		Label:     "Plan",
		TableName: reconciler.QualifiedTableNameIn(model.Plans, "waf"),
		Fields: []metadata.EntityField{
			name,
			metadata.NewField(model.FieldMaxZones, "Max zones (0 = unlimited)", metadata.KindNumber).Require(),
			metadata.NewField(model.FieldMaxRulesPerZone, "Max firewall rules per zone (0 = unlimited)", metadata.KindNumber).Require(),
			// The plan a customer without a subscription gets.
			metadata.NewField(model.FieldIsDefault, "Default plan", metadata.KindBoolean).Index(),
			metadata.NewField(model.FieldEnabled, "Enabled", metadata.KindBoolean).Index().WithWidget(metadata.WidgetSwitch),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"name", "maxZones", "maxRulesPerZone", "isDefault", "enabled"},
			DefaultSort: metadata.Ptr("name"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func SubscriptionEntity() metadata.EntityDefinition {
	// A plain string, not a Reference: the customer is another tenant, not a record of this one.
	// One subscription per customer.
	customer := metadata.NewField(model.FieldCustomerTenantID, "Customer tenant", metadata.KindString).Require().Index()
	customer.Unique = metadata.Ptr(true)
	return metadata.EntityDefinition{
		Name:      model.Subscriptions,
		Label:     "Subscription",
		TableName: reconciler.QualifiedTableNameIn(model.Subscriptions, "waf"),
		Fields: []metadata.EntityField{
			customer,
			metadata.RefField(model.FieldPlanID, "Plan", model.Plans, "name").Require().Index(),
			// A downgrade takes effect later (UC-73): the plan the customer moves to, and from when.
			// Plain strings: they are resolved against the plans at the time, and a plan removed
			// meanwhile just means the move does not happen.
			metadata.NewField(model.FieldPendingPlanID, "Pending plan", metadata.KindString),
			metadata.NewField(model.FieldPendingFrom, "Pending from", metadata.KindDatetime),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:   []string{"customerTenantId", "planId"},
			Filters:  []string{model.FieldCustomerTenantID},
			MaxLimit: model.LargeMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionListTenants, Label: "List customers", Description: "The tenants a plan can be given to.", Scope: metadata.ActionScopeEntity, Roles: []string{permission.RoleAdmin}},
		},
	}
}

// PlanChangeRequestEntity is a customer asking for another plan (UC-71, UC-73). The customer cannot
// edit the subscription itself, so it asks; the operator's admin approves (an upgrade applies at
// once, a downgrade from the end of the current period) or rejects with a reply.
func PlanChangeRequestEntity() metadata.EntityDefinition {
	// One waiting request per customer, enforced by the database: openRequestKey is the customer's
	// tenant while the request waits and empty once decided, and empty values never collide.
	openKey := metadata.NewField(model.FieldOpenRequestKey, "Open request key", metadata.KindString)
	openKey.Unique = metadata.Ptr(true)
	return metadata.EntityDefinition{
		Name:      model.PlanChanges,
		Label:     "Plan change request",
		TableName: reconciler.QualifiedTableNameIn(model.PlanChanges, "waf"),
		Fields: []metadata.EntityField{
			metadata.NewField(model.FieldCustomerTenantID, "Customer tenant", metadata.KindString).Require().Index(),
			// The customer's name when it asked, so the operator's list reads without a lookup.
			metadata.NewField(model.FieldCustomerName, "Customer", metadata.KindString),
			metadata.RefField(model.FieldRequestedPlanID, "Requested plan", model.Plans, "name").Require().Index(),
			metadata.EnumField(model.FieldStatus, "Status", "pending", "approved", "rejected").Index(),
			metadata.NewField(model.FieldNote, "Customer note", metadata.KindString),
			metadata.NewField(model.FieldDecisionNote, "Reply", metadata.KindString),
			openKey,
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"customerName", "requestedPlanId", "status", "note"},
			Filters:     []string{"status", model.FieldCustomerTenantID},
			DefaultSort: metadata.Ptr("-createdAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionRequestPlan, Label: "Request a plan", Description: "Ask the service provider for another plan.", Scope: metadata.ActionScopeEntity, Roles: []string{permission.RoleAdmin}},
			{Name: model.ActionMyPlanRequests, Label: "My requests", Description: "The plan change requests of the caller's own workspace.", Scope: metadata.ActionScopeEntity, Roles: []string{permission.RoleAdmin}},
			{Name: model.ActionDecidePlan, Label: "Decide", Description: "Approve or reject a plan change request.", Scope: metadata.ActionScopeRecord, Roles: []string{permission.RoleAdmin}},
		},
	}
}

func init() {
	metadata.SubmitEntity(PlanEntity)
	metadata.SubmitEntity(PlanChangeRequestEntity)
	metadata.SubmitEntity(SubscriptionEntity)
}
