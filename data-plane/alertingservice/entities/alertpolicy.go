// waf.alert_policies — tenant-scoped (not per-Zone: 1 policy watches N zones, each zone counted
// separately — docs/08-module-detail-specs.md module 8 copy note: "N event trong M phut tren
// CUNG 1 zone", not summed across zones). See docs/02-domain-model.md.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func AlertPolicyEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.alert_policies",
		Label:     "Alert Policy",
		TableName: reconciler.QualifiedTableNameIn("waf.alert_policies", "waf"),
		Fields: []metadata.EntityField{
			field("name", "Name", metadata.KindString, true, false, true),
			field("thresholdCount", "Threshold Count", metadata.KindNumber, true, false, false),
			field("windowMinutes", "Window (minutes)", metadata.KindNumber, true, false, false),
			field("channels", "Channels", metadata.KindJSON, true, false, false),
			field("enabled", "Enabled", metadata.KindBoolean, false, true, false),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"name", "thresholdCount", "windowMinutes", "enabled"},
			Filters:     []string{"enabled"},
			DefaultSort: strPtr("-createdAt"),
			MaxLimit:    50,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(AlertPolicyEntity) }
