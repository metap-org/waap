// waf.scan_findings — 1 vulnerability found by a ScanJob run. Dedupe across runs of the same
// ScanJob uses (scanJobId, category, endpoint) as the identity key (there's no separate ScanRun
// entity — only lastSeenAt distinguishes "still present" from "new") — that dedupe logic lives
// wherever the scan engine writes results back (out of portal/management-plane scope), not here; this
// entity just needs firstSeenAt/lastSeenAt to support it.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
)

func ScanFindingEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      "waf.scan_findings",
		Label:     "Scan Finding",
		TableName: reconciler.QualifiedTableNameIn("waf.scan_findings", "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField("scanJobId", "Scan Job", "waf.scan_jobs", "scanType").Require().Index(),
			metadata.EnumField("severity", "Severity", "info", "low", "medium", "high", "critical").Require().Index(),
			metadata.NewField("category", "Category", metadata.KindString).Require().Index().Sort(),
			metadata.NewField("endpoint", "Endpoint", metadata.KindString).Require(),
			metadata.NewField("description", "Description", metadata.KindString),
			metadata.EnumField("remediationStatus", "Remediation Status", "open", "confirmed", "falsePositive", "fixed", "accepted").Index(),
			metadata.NewField("firstSeenAt", "First Seen At", metadata.KindDatetime).Sort(),
			metadata.NewField("lastSeenAt", "Last Seen At", metadata.KindDatetime).Sort(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"scanJobId", "severity", "category", "endpoint", "remediationStatus", "lastSeenAt"},
			Filters:     []string{"scanJobId", "severity", "remediationStatus"},
			DefaultSort: metadata.Ptr("-lastSeenAt"),
			MaxLimit:    50,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: "remediationStatus", InitialState: "open",
			TerminalStates: []string{"fixed", "falsePositive", "accepted"},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition("confirm", "open", "confirmed", "Confirm"),
				metadata.Transition("markFixed", "confirmed", "fixed", "Mark Fixed"),
				metadata.Transition("markFalsePositive", "open", "falsePositive", "Mark False Positive"),
				metadata.Transition("accept", "open", "accepted", "Accept Risk"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(ScanFindingEntity) }
