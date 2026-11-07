// waf.alert_notifications — audit log of 1 actual alert send, separate from AlertPolicy (config)
// since it's history, not configuration. See docs/02-domain-model.md. No workflow — append-only,
// written once when a send is attempted.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func AlertNotificationEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.alert_notifications",
		Label:     "Alert Notification",
		TableName: reconciler.QualifiedTableNameIn("waf.alert_notifications", "waf"),
		Fields: []metadata.EntityField{
			refField("alertPolicyId", "Alert Policy", "waf.alert_policies", "name", true, true),
			field("triggeredAt", "Triggered At", metadata.KindDatetime, true, true, true),
			enumField("channel", "Channel", []string{"email", "webhook"}, true, false),
			enumField("deliveryStatus", "Delivery Status", []string{"sent", "failed"}, true, true),
		},
		ListViews: []metadata.EntityListView{{
			Name:        "default",
			Label:       "Default",
			Fields:      []string{"alertPolicyId", "triggeredAt", "channel", "deliveryStatus"},
			Filters:     []string{"alertPolicyId", "deliveryStatus"},
			DefaultSort: strPtr("-triggeredAt"),
			MaxLimit:    50,
		}},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(AlertNotificationEntity) }
