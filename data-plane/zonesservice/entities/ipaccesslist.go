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
	zoneID := refField("zoneId", "Zone", "waf.zones", "hostname", false, true)
	typ := enumField("type", "Type", []string{"whitelist", "blacklist"}, true, true)

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
			field("value", "IP / CIDR", metadata.KindString, true, false),
			field("enabled", "Enabled", metadata.KindBoolean, false, true),
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
