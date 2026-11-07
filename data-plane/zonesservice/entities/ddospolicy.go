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
			refField("zoneId", "Zone", "waf.zones", "hostname", true, true),
			enumField("sensitivity", "Sensitivity", []string{"low", "medium", "high", "aggressive"}, true, false),
			sortableField("requestRateThreshold", "Request Rate Threshold", metadata.KindNumber, true, false, true),
			field("burstWindow", "Burst Window (s)", metadata.KindNumber, true, false),
			enumField("action", "Action", []string{"log", "challenge", "block"}, true, false),
			field("enabled", "Enabled", metadata.KindBoolean, false, true),
			sortableField("priority", "Priority", metadata.KindNumber, true, false, true),
			// Absent/empty = applies to every path — same "empty means unscoped" convention as
			// FirewallRule.matchCondition's nil case.
			field("pathPrefix", "Path Prefix", metadata.KindString, false, false),
			enumField("httpMethod", "HTTP Method", []string{"any", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}, false, false),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"zoneId", "sensitivity", "action", "pathPrefix", "httpMethod", "priority", "enabled"},
			Filters:     []string{"zoneId", "enabled"},
			DefaultSort: strPtr("priority"),
			MaxLimit:    50,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(DdosPolicyEntity) }
