// waf.zones — the anchor entity every other config entity (DdosPolicy, FirewallRule) hangs off
// via a zoneId reference.
//
// hasConfig is a technical field, not a business one: PolicyCondition (the grammar
// WorkflowTransition.Guard is written in) can only compare one attribute against a literal — it
// has no count/aggregate operator over a zone's related DdosPolicy/FirewallRule records. So the
// "can't activate an empty zone" guard can't read those related records directly; instead the
// app layer (routes.SyncConfigState) flips hasConfig to true whenever a DdosPolicy or
// FirewallRule is created for this zone (and back to false if the last one is deleted), and the
// activate guard below just checks that flag.
//
// verificationStatus gates activate alongside hasConfig — proves the customer controls the
// hostname before edge-plane ever routes traffic for it. verificationStatus stays here as a
// technical mirror field, kept in sync by routes.VerifyDomainDNS's cascade — see domain.go's own
// doc comment. domainId is set automatically by guardedbackend.resolveDomainID at create time
// from the hostname's apex — never chosen by hand.
package entities

import (
	"encoding/json"

	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
)

func ZoneEntity() metadata.EntityDefinition {
	hostname := sortableField("hostname", "Hostname", metadata.KindString, true, true, true)
	hostname.Unique = boolPtr(true)
	// A hostname is served by exactly one zone across every tenant, not once per tenant.
	globalScope := metadata.UniqueScopeGlobal
	hostname.UniqueScope = &globalScope
	hostname.Searchable = boolPtr(true)

	activateGuard := permission.AllCondition(
		permission.AttributeCondition("hasConfig", permission.OpEq, permission.LiteralValue(json.RawMessage("true"))),
		permission.AttributeCondition("verificationStatus", permission.OpEq, permission.LiteralValue(json.RawMessage(`"verified"`))),
	)

	return metadata.EntityDefinition{
		Name:      "waf.zones",
		Label:     "Zone",
		TableName: reconciler.QualifiedTableNameIn("waf.zones", "waf"),
		Fields: []metadata.EntityField{
			hostname,
			field("originAddress", "Origin Address", metadata.KindString, true, false),
			enumField("status", "Status", []string{"pending", "active", "paused", "suspended"}, false, true),
			enumField("protectionMode", "Protection Mode", []string{"monitor", "enforce"}, true, true),
			sortableField("configVersion", "Config Version", metadata.KindNumber, false, false, true),
			field("hasConfig", "Has Config", metadata.KindBoolean, false, false),
			refField("domainId", "Domain", "waf.domains", "apexDomain", true, true),
			enumField("verificationStatus", "Verification Status", []string{"unverified", "verified"}, false, true),
			enumField("dnsRoutingStatus", "DNS Routing Status", []string{"notRouted", "routed", "unknown"}, false, false),
			sortableField("lastDnsCheckAt", "Last DNS Check At", metadata.KindDatetime, false, false, true),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:  []string{"hostname", "status", "protectionMode", "configVersion"},
			Filters: []string{"hostname", "status", "protectionMode"},
			// hostname (which zone) and status (is it even protecting anything) are the 2
			// columns nobody should be able to hide from this list.
			RequiredFields: []string{"hostname", "status"},
			DefaultSort:    strPtr("-createdAt"),
			MaxLimit:       50,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: "status", InitialState: "pending", TerminalStates: []string{"suspended"},
			Transitions: []metadata.WorkflowTransition{
				{Action: "activate", From: "pending", To: "active", Label: "Activate", Guard: &activateGuard},
				{Action: "pause", From: "active", To: "paused", Label: "Pause"},
				{Action: "resume", From: "paused", To: "active", Label: "Resume"},
				{Action: "suspend", From: "active", To: "suspended", Label: "Suspend"},
				{Action: "suspend", From: "paused", To: "suspended", Label: "Suspend"},
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func ZoneRelatedViews() []metadata.RelatedView {
	limit5 := uint32(5)
	limit10 := uint32(10)
	return []metadata.RelatedView{
		{Name: "ddosPolicy", Label: "DDoS Policy", Entity: "waf.ddos_policies", FilterField: "zoneId",
			Fields: []string{"sensitivity", "action", "enabled"}, Limit: &limit5},
		{Name: "firewallRules", Label: "Firewall Rules", Entity: "waf.firewall_rules", FilterField: "zoneId",
			Fields: []string{"name", "ruleType", "action"}, Limit: &limit10},
		{Name: "scanJobs", Label: "Recent Scan Jobs", Entity: "waf.scan_jobs", FilterField: "zoneId",
			Fields: []string{"scanType", "status"}, Limit: &limit5},
		{Name: "incidents", Label: "Recent Incidents", Entity: "waf.incidents", FilterField: "zoneId",
			Fields: []string{"title", "severity", "status"}, Limit: &limit5},
	}
}

// ZoneFieldDisplayHints mirrors primitives.tsx's TONES zone-status mapping, so waf.zones' status
// reads the same colour on the generic escape-hatch screen as it does on the product's own
// StatusBadge.
func ZoneFieldDisplayHints() []metadata.FieldDisplayHint {
	return []metadata.FieldDisplayHint{{
		Field: "status",
		EnumTones: map[string]string{
			"active": "default", "pending": "secondary", "paused": "outline", "suspended": "destructive",
		},
	}}
}

func init() {
	metadata.SubmitEntity(ZoneEntity)
	metadata.SubmitRelatedViews("waf.zones", ZoneRelatedViews)
	metadata.SubmitFieldDisplayHints("waf.zones", ZoneFieldDisplayHints)
}
