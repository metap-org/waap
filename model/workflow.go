package model

// Workflow states and transition actions. A state is the value of the entity's StateField; an
// action is what CrudService.Transition takes. Two different things share some words ("run" is both
// the scan job's workflow transition and its declared action) — the entity-prefixed names keep them apart.

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

// waf.false_positive_reports (StateField "status"): a report is open until someone decides.
const (
	FalsePositiveOpen      = "open"
	FalsePositiveDismissed = "dismissed"
	FalsePositiveResolved  = "resolved"

	FalsePositiveDismiss = "dismiss"
	FalsePositiveResolve = "resolve"
)

// waf.change_requests status. Only the change request's own actions move it (no workflow: every
// write to the entity from a client is refused).
const (
	ChangeDraft         = "draft"
	ChangePendingReview = "pendingReview"
	ChangePublished     = "published"
	ChangeRejected      = "rejected"
	ChangeCancelled     = "cancelled"
)

// waf.change_requests decision.
const (
	DecisionApproved      = "approved"
	DecisionSelfConfirmed = "selfConfirmed"
	DecisionRejected      = "rejected"
)

// waf.accounts changeControl: how a change that affects traffic reaches the account's zones. Empty
// means off.
const (
	ChangeControlOff      = "off"
	ChangeControlReview   = "review"   // through a change request; the author may confirm their own
	ChangeControlFourEyes = "fourEyes" // through a change request that someone else approves
)

// waf.accounts (StateField "status"): closing is final.
const (
	AccountActive = "active"
	AccountClosed = "closed"

	AccountClose = "close"
)

// waf.domains verificationStatus / verificationMethod, and waf.zones dnsRoutingStatus.
const (
	VerificationUnverified     = "unverified"
	VerificationVerified       = "verified"
	VerificationMethodDNSTxt   = "dnsTxt"
	VerificationMethodHTTPFile = "httpFile"

	DNSRouted    = "routed"
	DNSNotRouted = "notRouted"
)
