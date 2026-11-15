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
	"github.com/metap-org/waap/model"
)

func ZoneEntity() metadata.EntityDefinition {
	hostname := metadata.NewField(model.FieldHostname, "Hostname", metadata.KindString).Require().Index().Sort()
	hostname.Unique = metadata.Ptr(true)
	// A hostname is served by exactly one zone across every tenant, not once per tenant.
	globalScope := metadata.UniqueScopeGlobal
	hostname.UniqueScope = &globalScope
	hostname.Searchable = metadata.Ptr(true)

	activateGuard := permission.AllCondition(
		permission.AttributeCondition("hasConfig", permission.OpEq, permission.LiteralValue(json.RawMessage("true"))),
		permission.AttributeCondition(model.FieldVerificationStatus, permission.OpEq, permission.LiteralValue(json.RawMessage(`"verified"`))),
	)

	return metadata.EntityDefinition{
		Name:      model.Zones,
		Label:     "Zone",
		TableName: reconciler.QualifiedTableNameIn(model.Zones, "waf"),
		Fields: []metadata.EntityField{
			hostname,
			metadata.NewField(model.FieldOriginAddress, "Origin Address", metadata.KindString).Require(),
			metadata.EnumField(model.FieldStatus, "Status", "pending", "active", "paused", "suspended").Index(),
			metadata.EnumField(model.FieldProtectionMode, "Protection Mode", "monitor", "enforce").Require().Index().WithWidget(metadata.WidgetRadio),
			metadata.NewField(model.FieldConfigVersion, "Config Version", metadata.KindNumber).Sort(),
			metadata.NewField(model.FieldHasConfig, "Has Config", metadata.KindBoolean),
			metadata.RefField(model.FieldDomainID, "Domain", model.Domains, "apexDomain").Require().Index(),
			metadata.EnumField(model.FieldVerificationStatus, "Verification Status", model.VerificationUnverified, model.VerificationVerified).Index(),
			metadata.EnumField(model.FieldDNSRoutingStatus, "DNS Routing Status", model.DNSNotRouted, model.DNSRouted, "unknown"),
			metadata.NewField(model.FieldLastDNSCheckAt, "Last DNS Check At", metadata.KindDatetime).Sort(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:  []string{"hostname", "status", "protectionMode", "verificationStatus", "dnsRoutingStatus", "hasConfig", "configVersion"},
			Filters: []string{"hostname", "status", "protectionMode"},
			// hostname (which zone) and status (is it even protecting anything) are the 2
			// columns nobody should be able to hide from this list.
			RequiredFields: []string{"hostname", "status"},
			DefaultSort:    metadata.Ptr("-createdAt"),
			MaxLimit:       model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: model.FieldStatus, InitialState: model.ZoneStatusPending, TerminalStates: []string{model.ZoneStatusSuspended},
			Transitions: []metadata.WorkflowTransition{
				{Action: model.ZoneActivate, From: model.ZoneStatusPending, To: model.ZoneStatusActive, Label: "Activate", Guard: &activateGuard},
				{Action: model.ZonePause, From: model.ZoneStatusActive, To: model.ZoneStatusPaused, Label: "Pause"},
				{Action: model.ZoneResume, From: model.ZoneStatusPaused, To: model.ZoneStatusActive, Label: "Resume"},
				{Action: model.ZoneSuspend, From: model.ZoneStatusActive, To: model.ZoneStatusSuspended, Label: "Suspend"},
				{Action: model.ZoneSuspend, From: model.ZoneStatusPaused, To: model.ZoneStatusSuspended, Label: "Suspend"},
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionVerifyDNS, Label: "Verify DNS routing", Description: "Checks whether the zone hostname points at the edge.", Scope: metadata.ActionScopeRecord},
			{Name: model.ActionTestOrigin, Label: "Test origin", Description: "One-shot connectivity check against the customer's origin.", Scope: metadata.ActionScopeRecord},
			{Name: model.ActionSyncConfigState, Label: "Sync config state", Description: "Recomputes hasConfig from the zone's policies and rules.", Scope: metadata.ActionScopeRecord},
		},
	}
}

func ZoneRelatedViews() []metadata.RelatedView {
	limit5 := uint32(5)
	limit10 := uint32(10)
	return []metadata.RelatedView{
		{Name: "ddosPolicy", Label: "DDoS Policy", Entity: model.DdosPolicies, FilterField: model.FieldZoneID,
			Fields: []string{"sensitivity", "action", "enabled"}, Limit: &limit5},
		{Name: "firewallRules", Label: "Firewall Rules", Entity: model.FirewallRules, FilterField: model.FieldZoneID,
			Fields: []string{"name", "ruleType", "action"}, Limit: &limit10},
		{Name: "scanJobs", Label: "Recent Scan Jobs", Entity: model.ScanJobs, FilterField: model.FieldZoneID,
			Fields: []string{"scanType", "status"}, Limit: &limit5},
		{Name: "incidents", Label: "Recent Incidents", Entity: model.Incidents, FilterField: model.FieldZoneID,
			Fields: []string{"title", "severity", "status"}, Limit: &limit5},
	}
}

// ZoneFieldDisplayHints mirrors primitives.tsx's TONES zone-status mapping, so waf.zones' status
// reads the same colour on the generic escape-hatch screen as it does on the product's own
// StatusBadge.
func ZoneFieldDisplayHints() []metadata.FieldDisplayHint {
	return []metadata.FieldDisplayHint{
		{Field: "status", EnumTones: map[string]string{
			"active": "default", "pending": "secondary", "paused": "outline", "suspended": "destructive",
		}},
		{Field: "protectionMode", EnumTones: map[string]string{"monitor": "outline", "enforce": "default"}},
		{Field: "verificationStatus", EnumTones: map[string]string{"verified": "default", "unverified": "outline"}},
		{Field: "dnsRoutingStatus", EnumTones: map[string]string{"routed": "default", "notRouted": "outline"}},
	}
}

func init() {
	metadata.SubmitEntity(ZoneEntity)
	metadata.SubmitRelatedViews(model.Zones, ZoneRelatedViews)
	metadata.SubmitFieldDisplayHints(model.Zones, ZoneFieldDisplayHints)
}
