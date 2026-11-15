// waf.ip_access_lists — whitelist/blacklist by IP or CIDR. A separate entity from
// FirewallRule for a clearer, purpose-built API/UI (no matchCondition JSON to author). Geo
// (country-code) access rules stay on FirewallRule — this entity is IP/CIDR only.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func IPAccessListEntity() metadata.EntityDefinition {
	// zoneId is nullable for the same tenant-wide ("global") reason as FirewallRule.zoneId —
	// nil applies to every zone in the tenant.
	zoneID := metadata.RefField(model.FieldZoneID, "Zone", model.Zones, "hostname").Index()
	typ := metadata.EnumField(model.FieldType, "Type", "whitelist", "blacklist").Require().Index()

	return metadata.EntityDefinition{
		Name:      model.IPAccessLists,
		Label:     "IP Access List",
		TableName: reconciler.QualifiedTableNameIn(model.IPAccessLists, "waf"),
		Fields: []metadata.EntityField{
			zoneID,
			typ,
			// One IP or CIDR per row, validated at save time by guardedbackend's
			// validateIPAccessList. Bulk-add in the portal creates N rows client-side rather
			// than this field taking a list.
			metadata.NewField(model.FieldValue, "IP / CIDR", metadata.KindString).Require(),
			metadata.NewField(model.FieldEnabled, "Enabled", metadata.KindBoolean).Index().WithWidget(metadata.WidgetSwitch),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:   []string{model.FieldZoneID, "type", "value", "enabled"},
			Filters:  []string{model.FieldZoneID, "type", "enabled"},
			MaxLimit: model.LargeMaxListLimit,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true, Parent: model.FieldZoneID},
	}
}

func init() { metadata.SubmitEntity(IPAccessListEntity) }
