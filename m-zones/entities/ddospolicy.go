// waf.ddos_policies — DDoS L7 policy for a Zone. A zone can have multiple policies at once,
// scoped by pathPrefix/httpMethod so different endpoints can carry different thresholds.
// priority breaks ties the same way FirewallRule.priority does — first match (by scope, in
// priority order) wins. An absent pathPrefix/httpMethod (or httpMethod "any") matches
// everything, preserving the "one policy protects the whole zone" default shape.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func DdosPolicyEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.DdosPolicies,
		Label:     "DDoS Policy",
		TableName: reconciler.QualifiedTableNameIn(model.DdosPolicies, "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField(model.FieldZoneID, "Zone", model.Zones, "hostname").Require().Index(),
			metadata.EnumField(model.FieldSensitivity, "Sensitivity", "low", "medium", "high", "aggressive").Require().WithWidget(metadata.WidgetRadio),
			metadata.NewField(model.FieldRequestRateThreshold, "Request Rate Threshold", metadata.KindNumber).Require().Sort(),
			metadata.NewField(model.FieldBurstWindow, "Burst Window (s)", metadata.KindNumber).Require(),
			metadata.EnumField(model.FieldAction, "Action", "log", "challenge", "block").Require(),
			metadata.NewField(model.FieldEnabled, "Enabled", metadata.KindBoolean).Index().WithWidget(metadata.WidgetSwitch),
			metadata.NewField(model.FieldPriority, "Priority", metadata.KindNumber).Require().Sort(),
			// Absent/empty = applies to every path — same "empty means unscoped" convention as
			// FirewallRule.matchCondition's nil case.
			metadata.NewField(model.FieldPathPrefix, "Path Prefix", metadata.KindString),
			metadata.EnumField(model.FieldHTTPMethod, "HTTP Method", "any", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{model.FieldZoneID, "sensitivity", "action", "pathPrefix", "httpMethod", "priority", "enabled"},
			Filters:     []string{model.FieldZoneID, "enabled"},
			DefaultSort: metadata.Ptr("priority"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true, Parent: model.FieldZoneID},
	}
}

func init() { metadata.SubmitEntity(DdosPolicyEntity) }
