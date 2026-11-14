// waf.firewall_rules — shared rule engine backing WAF custom rules, rate limiting, and IP/geo
// firewall in one entity (ruleType just groups them for UI; matchCondition + action is the same
// evaluate loop for all of them).
//
// matchCondition is deliberately its own JSON grammar, not metap-permission's PolicyCondition —
// no uri.*/header.*/body.* namespace there, and it's missing Contains/Regex/CidrMatch operators.
// Stored as opaque JSON here; the grammar itself and its validation belong to edge-plane/
// control-plane, not this entity definition (see matchcondition.go in this service for the
// save-time validator copy).
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func FirewallRuleEntity() metadata.EntityDefinition {
	// zoneId is nullable since the tenant-wide ("global") scope feature: zoneId == nil means this
	// rule applies to every zone in the tenant, not just one.
	zoneID := metadata.RefField("zoneId", "Zone", "waf.zones", "hostname").Index()

	return metadata.EntityDefinition{
		Name:      "waf.firewall_rules",
		Label:     "Firewall Rule",
		TableName: reconciler.QualifiedTableNameIn("waf.firewall_rules", "waf"),
		Fields: []metadata.EntityField{
			zoneID,
			metadata.NewField("name", "Name", metadata.KindString).Require().Sort(),
			metadata.NewField("priority", "Priority", metadata.KindNumber).Require().Sort(),
			metadata.NewField("matchCondition", "Match Condition", metadata.KindJSON).Require(),
			// ipFirewall/geoFirewall were removed once waf.ip_access_lists took over IP/CIDR
			// whitelist and blacklist. Geo (country-code) access rules stay expressible as an
			// ordinary ruleType "waf" rule with a matchCondition field of "country".
			metadata.EnumField("ruleType", "Rule Type", "waf", "rateLimit").Require().Index(),
			metadata.NewField("rateLimitThreshold", "Rate Limit Threshold", metadata.KindNumber),
			metadata.NewField("rateLimitWindow", "Rate Limit Window (s)", metadata.KindNumber),
			metadata.EnumField("action", "Action", "allow", "block", "challenge", "log").Require(),
			metadata.NewField("enabled", "Enabled", metadata.KindBoolean).Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"zoneId", "name", "priority", "ruleType", "action", "enabled"},
			Filters:     []string{"zoneId", "ruleType", "enabled"},
			DefaultSort: metadata.Ptr("priority"),
			MaxLimit:    100,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(FirewallRuleEntity) }
