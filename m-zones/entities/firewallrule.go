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
	"github.com/metap-org/waap/model"
)

func FirewallRuleEntity() metadata.EntityDefinition {
	// zoneId is nullable since the tenant-wide ("global") scope feature: zoneId == nil means this
	// rule applies to every zone in the tenant, not just one.
	zoneID := metadata.RefField(model.FieldZoneID, "Zone", model.Zones, "hostname").Index()

	return metadata.EntityDefinition{
		Name:      model.FirewallRules,
		Label:     "Firewall Rule",
		TableName: reconciler.QualifiedTableNameIn(model.FirewallRules, "waf"),
		Fields: []metadata.EntityField{
			zoneID,
			metadata.NewField(model.FieldName, "Name", metadata.KindString).Require().Sort(),
			metadata.NewField(model.FieldPriority, "Priority", metadata.KindNumber).Require().Sort(),
			metadata.NewField(model.FieldMatchCondition, "Match Condition", metadata.KindJSON).Require(),
			// ipFirewall/geoFirewall were removed once waf.ip_access_lists took over IP/CIDR
			// whitelist and blacklist. Geo (country-code) access rules stay expressible as an
			// ordinary ruleType "waf" rule with a matchCondition field of "country".
			metadata.EnumField(model.FieldRuleType, "Rule Type", "waf", "rateLimit").Require().Index(),
			metadata.NewField(model.FieldRateLimitThreshold, "Rate Limit Threshold", metadata.KindNumber),
			metadata.NewField(model.FieldRateLimitWindow, "Rate Limit Window (s)", metadata.KindNumber),
			metadata.EnumField(model.FieldAction, "Action", "allow", "block", "challenge", "log").Require(),
			metadata.NewField(model.FieldEnabled, "Enabled", metadata.KindBoolean).Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{model.FieldZoneID, "name", "priority", "ruleType", "action", "enabled"},
			Filters:     []string{model.FieldZoneID, "ruleType", "enabled"},
			DefaultSort: metadata.Ptr("priority"),
			MaxLimit:    100,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(FirewallRuleEntity) }
