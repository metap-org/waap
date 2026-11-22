// Package model names what the services of this product share: entity names, the field names that
// tie entities together, and the actions entities declare. Each was a string literal retyped in
// every service, gateway and worker that touched it; a typo in one only showed up at run time.
// Declare a name here once and import it. Standard library only.
package model

// Entity names (EntityDefinition.Name).
const (
	Zones              = "waf.zones"
	Domains            = "waf.domains"
	Accounts           = "waf.accounts"
	Plans              = "waf.plans"
	Subscriptions      = "waf.subscriptions"
	DdosPolicies       = "waf.ddos_policies"
	FirewallRules      = "waf.firewall_rules"
	IPAccessLists      = "waf.ip_access_lists"
	PolicyVersions     = "waf.policy_versions"
	ScanJobs           = "waf.scan_jobs"
	ScanFindings       = "waf.scan_findings"
	AlertPolicies      = "waf.alert_policies"
	AlertNotifications = "waf.alert_notifications"
	Incidents          = "waf.incidents"
	FalsePositives     = "waf.false_positive_reports"
	SecurityEvents     = "waf.security_events" // a read model served from the analytics plane
)

// Scope types (metadata scopes: a role held only inside one).
const ScopeAccount = "account"

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
	ActionRollback        = "rollback"        // policy_versions
	ActionPlanUsage       = "planUsage"       // zones (entity scope): the tenant's plan and what it uses
	ActionListTenants     = "listTenants"     // subscriptions (entity scope): the customers a plan can be given to
	ActionRun             = "run"             // scan_jobs
	ActionTest            = "test"            // alert_policies
	ActionEvaluate        = "evaluate"        // alert_policies
	ActionCorrelate       = "correlate"       // incidents
)

// Error codes this product adds to metap's (errcode): returned to the portal, which switches on them.
const (
	ErrScanAlreadyInProgress     = "scan_already_in_progress"
	ErrScannerUnavailable        = "scanner_unavailable"
	ErrReferenceCheckUnavailable = "reference_check_unavailable"
	ErrPlanLimitReached          = "plan_limit_reached"
	ErrAccountInUse              = "account_in_use"
	ErrAccountClosed             = "account_closed"
)

// List page sizes: the MaxLimit of an entity's list views. Most entities cap at DefaultMaxListLimit;
// the ones a zone's detail page shows in full (rules, access lists, domains) allow more.
const (
	DefaultMaxListLimit = 50
	LargeMaxListLimit   = 100
)
