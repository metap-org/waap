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
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:   []string{"customerTenantId", "planId"},
			MaxLimit: model.LargeMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionListTenants, Label: "List customers", Description: "The tenants a plan can be given to.", Scope: metadata.ActionScopeEntity},
		},
	}
}

func init() {
	metadata.SubmitEntity(PlanEntity)
	metadata.SubmitEntity(SubscriptionEntity)
}
