// waf.ip_access_lists — whitelist/blacklist by IP or CIDR. A separate entity from
// FirewallRule for a clearer, purpose-built API/UI (no matchCondition JSON to author). Geo
// (country-code) access rules stay on FirewallRule — this entity is IP/CIDR only.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func IPAccessListEntity() metadata.EntityDefinition {
	// zoneId is nullable for the same tenant-wide ("global") reason as FirewallRule.zoneId —
	// nil applies to every zone in the tenant.
	zoneID := metadata.RefField("zoneId", "Zone", "waf.zones", "hostname").Index()
	typ := metadata.EnumField("type", "Type", "whitelist", "blacklist").Require().Index()

	return metadata.EntityDefinition{
		Name:      "waf.ip_access_lists",
		Label:     "IP Access List",
		TableName: reconciler.QualifiedTableNameIn("waf.ip_access_lists", "waf"),
		Fields: []metadata.EntityField{
			zoneID,
			typ,
			// One IP or CIDR per row, validated at save time by guardedbackend's
			// validateIPAccessList. Bulk-add in the portal creates N rows client-side rather
			// than this field taking a list.
			metadata.NewField("value", "IP / CIDR", metadata.KindString).Require(),
			metadata.NewField("enabled", "Enabled", metadata.KindBoolean).Index(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:   []string{"zoneId", "type", "value", "enabled"},
			Filters:  []string{"zoneId", "type", "enabled"},
			MaxLimit: 100,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(IPAccessListEntity) }
