// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.scan_jobs — a recurring scan config (not a single run). status tracks the latest run, not
// the job's own lifecycle — it loops (idle/completed/failed -> queued -> running ->
// completed/failed), it doesn't terminate. TerminalStates is left empty on purpose: a recurring
// job genuinely has no terminal state.
//
// Actually running a scan (the DAST engine itself) is out of this entity's/portal's scope — this
// is a waf.scan_jobs/waf.scan_findings storage/config concern only. Something external reads
// status: queued records and drives them through start/complete/fail via the generic transition
// API.
//
// zoneId is a plain string, not Reference, since the pillar split: waf.zones is owned by
// zones-service, a separate binary from this one, and Reference's ValidateReferences requires
// the target EntityDefinition registered in the SAME MetadataRegistry — registering ZoneEntity
// here just to satisfy that would also expose full CRUD for waf.zones on this service's generic
// /api/:entity* route.
// It is declared RefersTo(waf.zones) (roadmap 116): still a plain string column, but the API
// gateway can expand it across services (`?expand=zoneId`).
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func ScanJobEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.ScanJobs,
		Label:     "Scan Job",
		TableName: reconciler.QualifiedTableNameIn(model.ScanJobs, "waf"),
		Fields: []metadata.EntityField{
			metadata.NewField(model.FieldZoneID, "Zone", metadata.KindString).Require().Index().RefersTo(model.Zones),
			metadata.EnumField(model.FieldScanType, "Scan Type", "quickScan", "fullScan", "apiScan").Require().Index(),
			metadata.NewField(model.FieldSchedule, "Schedule (cron)", metadata.KindString),
			metadata.EnumField(model.FieldStatus, "Status", "idle", "queued", "running", "completed", "failed").Index(),
			metadata.NewField(model.FieldLastRunAt, "Last Run At", metadata.KindDatetime).Sort(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{model.FieldZoneID, "scanType", "schedule", "status", "lastRunAt"},
			Filters:     []string{model.FieldZoneID, "status"},
			DefaultSort: metadata.Ptr("-createdAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: model.FieldStatus, InitialState: model.ScanJobIdle, TerminalStates: []string{},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition(model.ScanJobRun, model.ScanJobIdle, model.ScanJobQueued, "Run"),
				metadata.Transition(model.ScanJobRun, model.ScanJobCompleted, model.ScanJobQueued, "Run again"),
				metadata.Transition(model.ScanJobRun, model.ScanJobFailed, model.ScanJobQueued, "Retry"),
				metadata.Transition(model.ScanJobStart, model.ScanJobQueued, model.ScanJobRunning, "Start"),
				metadata.Transition(model.ScanJobComplete, model.ScanJobRunning, model.ScanJobCompleted, "Complete"),
				metadata.Transition(model.ScanJobFail, model.ScanJobRunning, model.ScanJobFailed, "Fail"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
		Actions: []metadata.EntityAction{
			{Name: model.ActionRun, Label: "Run scan now", Description: "Queues the job and hands it to the scanner.", Scope: metadata.ActionScopeRecord, Requires: "transition"},
		},
	}
}

func init() { metadata.SubmitEntity(ScanJobEntity) }
