package model

// Workflow states and transition actions. A state is the value of the entity's StateField; an
// action is what CrudService.Transition takes. Two different things share some words ("run" is both
// the scan job's workflow transition and its gRPC action) — the entity-prefixed names keep them apart.

// waf.zones (StateField "status").
const (
	ZoneStatusPending   = "pending"
	ZoneStatusActive    = "active"
	ZoneStatusPaused    = "paused"
	ZoneStatusSuspended = "suspended"

	ZoneActivate = "activate"
	ZonePause    = "pause"
	ZoneResume   = "resume"
	ZoneSuspend  = "suspend"
)

// waf.scan_jobs (StateField "status"). Not terminal: a job can run again.
const (
	ScanJobIdle      = "idle"
	ScanJobQueued    = "queued"
	ScanJobRunning   = "running"
	ScanJobCompleted = "completed"
	ScanJobFailed    = "failed"

	ScanJobRun      = "run"
	ScanJobStart    = "start"
	ScanJobComplete = "complete"
	ScanJobFail     = "fail"
)

// waf.scan_findings (StateField "remediationStatus").
const (
	FindingOpen          = "open"
	FindingConfirmed     = "confirmed"
	FindingFixed         = "fixed"
	FindingFalsePositive = "falsePositive"
	FindingAccepted      = "accepted"

	FindingConfirm           = "confirm"
	FindingMarkFixed         = "markFixed"
	FindingMarkFalsePositive = "markFalsePositive"
	FindingAccept            = "accept"
)

// waf.incidents (StateField "status").
const (
	IncidentOpen         = "open"
	IncidentAcknowledged = "acknowledged"
	IncidentMitigating   = "mitigating"
	IncidentResolved     = "resolved"

	IncidentAcknowledge     = "acknowledge"
	IncidentStartMitigating = "startMitigating"
	IncidentResolve         = "resolve"
)

// waf.domains verificationStatus / verificationMethod, and waf.zones dnsRoutingStatus.
const (
	VerificationUnverified   = "unverified"
	VerificationVerified     = "verified"
	VerificationMethodDNSTxt = "dnsTxt"

	DNSRouted    = "routed"
	DNSNotRouted = "notRouted"
)
