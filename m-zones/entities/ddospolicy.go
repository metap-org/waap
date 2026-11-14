// waf.ddos_policies — DDoS L7 policy for a Zone. A zone can have multiple policies at once,
// scoped by pathPrefix/httpMethod so different endpoints can carry different thresholds.
// priority breaks ties the same way FirewallRule.priority does — first match (by scope, in
// priority order) wins. An absent pathPrefix/httpMethod (or httpMethod "any") matches
// everything, preserving the "one policy protects the whole zone" default shape.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func DdosPolicyEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.ddos_policies",
		Label:     "DDoS Policy",
		TableName: reconciler.QualifiedTableNameIn("waf.ddos_policies", "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField("zoneId", "Zone", "waf.zones", "hostname").Require().Index(),
			metadata.EnumField("sensitivity", "Sensitivity", "low", "medium", "high", "aggressive").Require(),
			metadata.NewField("requestRateThreshold", "Request Rate Threshold", metadata.KindNumber).Require().Sort(),
			metadata.NewField("burstWindow", "Burst Window (s)", metadata.KindNumber).Require(),
			metadata.EnumField("action", "Action", "log", "challenge", "block").Require(),
			metadata.NewField("enabled", "Enabled", metadata.KindBoolean).Index(),
			metadata.NewField("priority", "Priority", metadata.KindNumber).Require().Sort(),
			// Absent/empty = applies to every path — same "empty means unscoped" convention as
			// FirewallRule.matchCondition's nil case.
			metadata.NewField("pathPrefix", "Path Prefix", metadata.KindString),
			metadata.EnumField("httpMethod", "HTTP Method", "any", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"zoneId", "sensitivity", "action", "pathPrefix", "httpMethod", "priority", "enabled"},
			Filters:     []string{"zoneId", "enabled"},
			DefaultSort: metadata.Ptr("priority"),
			MaxLimit:    50,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(DdosPolicyEntity) }
