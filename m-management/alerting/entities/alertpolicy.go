// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.alert_policies — tenant-scoped (not per-Zone: 1 policy watches N zones, each zone counted
// separately — docs/08-module-detail-specs.md module 8 copy note: "N event trong M phut tren
// CUNG 1 zone", not summed across zones). See docs/02-domain-model.md.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func AlertPolicyEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.AlertPolicies,
		Label:     "Alert Policy",
		TableName: reconciler.QualifiedTableNameIn(model.AlertPolicies, "waf"),
		Fields: []metadata.EntityField{
			metadata.NewField(model.FieldName, "Name", metadata.KindString).Require().Sort(),
			metadata.NewField(model.FieldThresholdCount, "Threshold Count", metadata.KindNumber).Require(),
			metadata.NewField(model.FieldWindowMinutes, "Window (minutes)", metadata.KindNumber).Require(),
			metadata.NewField(model.FieldChannels, "Channels", metadata.KindJSON).Require(),
			metadata.NewField(model.FieldEnabled, "Enabled", metadata.KindBoolean).Index().WithWidget(metadata.WidgetSwitch),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"name", "thresholdCount", "windowMinutes", "enabled"},
			Filters:     []string{"enabled"},
			DefaultSort: metadata.Ptr("-createdAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionTest, Label: "Send test alert", Description: "Runs the real delivery path and records a notification.", Scope: metadata.ActionScopeRecord, Requires: "update"},
			{Name: model.ActionEvaluate, Label: "Evaluate alert policies", Description: "Evaluates every enabled policy against recent events.", Scope: metadata.ActionScopeEntity, Requires: "update"},
		},
	}
}

func init() { metadata.SubmitEntity(AlertPolicyEntity) }
