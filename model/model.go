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
	PlanChanges        = "waf.plan_change_requests"
	DdosPolicies       = "waf.ddos_policies"
	FirewallRules      = "waf.firewall_rules"
	IPAccessLists      = "waf.ip_access_lists"
	PolicyVersions     = "waf.policy_versions"
	ChangeRequests     = "waf.change_requests"
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
	ActionVerifyDNS          = "verifyDns"                 // zones, domains
	ActionTestOrigin         = "testOrigin"                // zones
	ActionSyncConfigState    = "syncConfigState"           // zones
	ActionRollback           = "rollback"                  // policy_versions
	ActionCreateChange       = "createChangeRequest"       // zones: draft a change to the zone's policy
	ActionCreateTenantChange = "createTenantChangeRequest" // change_requests (entity scope): draft a change to the tenant-wide records
	ActionReviseChange       = "revise"                    // change_requests: the author replaces the draft's operations
	ActionSubmitChange       = "submit"                    // change_requests: the author asks for the diff to be approved
	ActionApproveChange      = "approve"                   // change_requests: approve and publish (or confirm one's own)
	ActionRejectChange       = "reject"                    // change_requests
	ActionCancelChange       = "cancel"                    // change_requests
	ActionExportData         = "exportData"                // zones (entity scope): the tenant's configuration as one document
	ActionResolveHostname    = "resolveHostname"           // zones (entity scope): what a hostname resolves to now
	ActionProbeOrigin        = "probeOrigin"               // zones (entity scope): can this origin be reached, before the zone exists
	ActionApplyPreset        = "applyPreset"               // zones: give the zone a starting set of protections
	ActionValidatePolicy     = "validatePolicy"            // zones: check a zone's rules before they matter
	ActionPlanUsage          = "planUsage"                 // zones (entity scope): the tenant's plan and what it uses
	ActionRequestPlan        = "requestPlanChange"         // plan_change_requests (entity scope): a customer asks for another plan
	ActionMyPlanRequests     = "myPlanRequests"            // plan_change_requests (entity scope): the caller's own requests
	ActionDecidePlan         = "decide"                    // plan_change_requests (record scope): approve or reject
	ActionListTenants        = "listTenants"               // subscriptions (entity scope): the customers a plan can be given to
	ActionRun                = "run"                       // scan_jobs
	ActionTest               = "test"                      // alert_policies
	ActionEvaluate           = "evaluate"                  // alert_policies
	ActionCorrelate          = "correlate"                 // incidents
)

// Error codes this product adds to metap's (errcode): returned to the portal, which switches on them.
const (
	ErrScanAlreadyInProgress     = "scan_already_in_progress"
	ErrScannerUnavailable        = "scanner_unavailable"
	ErrReferenceCheckUnavailable = "reference_check_unavailable"
	ErrPlanLimitReached          = "plan_limit_reached"
	ErrTooManyFindings           = "too_many_findings"
	ErrScanNotRunnable           = "scan_not_running"
	ErrAccountInUse              = "account_in_use"
	ErrAccountClosed             = "account_closed"
	ErrChangeRequestRequired     = "change_request_required"
	ErrChangeRequestStale        = "change_request_stale"
	ErrChangeRequestState        = "change_request_state"
	ErrChangeRequestNotReady     = "change_request_not_ready"
	ErrSelfApproval              = "self_approval_refused"
)

// List page sizes: the MaxLimit of an entity's list views. Most entities cap at DefaultMaxListLimit;
// the ones a zone's detail page shows in full (rules, access lists, domains) allow more.
const (
	DefaultMaxListLimit = 50
	LargeMaxListLimit   = 100
)
