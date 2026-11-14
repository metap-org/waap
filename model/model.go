// Package model names what the services of this product share: entity names, the field names that
// tie entities together, and the actions entities declare. Each was a string literal retyped in
// every service, gateway and worker that touched it; a typo in one only showed up at run time.
// Declare a name here once and import it. Standard library only.
package model

// Entity names (EntityDefinition.Name).
const (
	Zones              = "waf.zones"
	Domains            = "waf.domains"
	DdosPolicies       = "waf.ddos_policies"
	FirewallRules      = "waf.firewall_rules"
	IPAccessLists      = "waf.ip_access_lists"
	ScanJobs           = "waf.scan_jobs"
	ScanFindings       = "waf.scan_findings"
	AlertPolicies      = "waf.alert_policies"
	AlertNotifications = "waf.alert_notifications"
	Incidents          = "waf.incidents"
	SecurityEvents     = "waf.security_events" // a read model served from the analytics plane
)

// Field names shared across entities.
const (
	FieldZoneID   = "zoneId"
	FieldDomainID = "domainId"
)

// Actions declared on entities (EntityDefinition.Actions).
const (
	ActionVerifyDNS       = "verifyDns"       // zones, domains
	ActionTestOrigin      = "testOrigin"      // zones
	ActionSyncConfigState = "syncConfigState" // zones
	ActionRun             = "run"             // scan_jobs
	ActionTest            = "test"            // alert_policies
	ActionEvaluate        = "evaluate"        // alert_policies
	ActionCorrelate       = "correlate"       // incidents
)
