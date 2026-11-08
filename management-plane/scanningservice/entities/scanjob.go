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
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func ScanJobEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.scan_jobs",
		Label:     "Scan Job",
		TableName: reconciler.QualifiedTableNameIn("waf.scan_jobs", "waf"),
		Fields: []metadata.EntityField{
			field("zoneId", "Zone", metadata.KindString, true, true),
			enumField("scanType", "Scan Type", []string{"quickScan", "fullScan", "apiScan"}, true, true),
			field("schedule", "Schedule (cron)", metadata.KindString, false, false),
			enumField("status", "Status", []string{"idle", "queued", "running", "completed", "failed"}, false, true),
			sortableField("lastRunAt", "Last Run At", metadata.KindDatetime, false, false, true),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"zoneId", "scanType", "schedule", "status", "lastRunAt"},
			Filters:     []string{"zoneId", "status"},
			DefaultSort: strPtr("-createdAt"),
			MaxLimit:    50,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: "status", InitialState: "idle", TerminalStates: []string{},
			Transitions: []metadata.WorkflowTransition{
				transitionNoGuard("run", "idle", "queued", "Run"),
				transitionNoGuard("run", "completed", "queued", "Run again"),
				transitionNoGuard("run", "failed", "queued", "Retry"),
				transitionNoGuard("start", "queued", "running", "Start"),
				transitionNoGuard("complete", "running", "completed", "Complete"),
				transitionNoGuard("fail", "running", "failed", "Fail"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(ScanJobEntity) }
