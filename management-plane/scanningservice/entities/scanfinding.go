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
			refField("scanJobId", "Scan Job", "waf.scan_jobs", "scanType", true, true),
			enumField("severity", "Severity", []string{"info", "low", "medium", "high", "critical"}, true, true),
			sortableField("category", "Category", metadata.KindString, true, true, true),
			field("endpoint", "Endpoint", metadata.KindString, true, false),
			field("description", "Description", metadata.KindString, false, false),
			enumField("remediationStatus", "Remediation Status", []string{"open", "confirmed", "falsePositive", "fixed", "accepted"}, false, true),
			sortableField("firstSeenAt", "First Seen At", metadata.KindDatetime, false, false, true),
			sortableField("lastSeenAt", "Last Seen At", metadata.KindDatetime, false, false, true),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"scanJobId", "severity", "category", "endpoint", "remediationStatus", "lastSeenAt"},
			Filters:     []string{"scanJobId", "severity", "remediationStatus"},
			DefaultSort: strPtr("-lastSeenAt"),
			MaxLimit:    50,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: "remediationStatus", InitialState: "open",
			TerminalStates: []string{"fixed", "falsePositive", "accepted"},
			Transitions: []metadata.WorkflowTransition{
				transitionNoGuard("confirm", "open", "confirmed", "Confirm"),
				transitionNoGuard("markFixed", "confirmed", "fixed", "Mark Fixed"),
				transitionNoGuard("markFalsePositive", "open", "falsePositive", "Mark False Positive"),
				transitionNoGuard("accept", "open", "accepted", "Accept Risk"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(ScanFindingEntity) }
