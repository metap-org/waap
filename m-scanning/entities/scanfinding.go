// waf.scan_findings — 1 vulnerability found by a ScanJob run. Dedupe across runs of the same
// ScanJob uses (scanJobId, category, endpoint) as the identity key (there's no separate ScanRun
// entity — only lastSeenAt distinguishes "still present" from "new") — that dedupe logic lives
// wherever the scan engine writes results back (out of portal/management-plane scope), not here; this
// entity just needs firstSeenAt/lastSeenAt to support it.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func ScanFindingEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.ScanFindings,
		Label:     "Scan Finding",
		TableName: reconciler.QualifiedTableNameIn(model.ScanFindings, "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField(model.FieldScanJobID, "Scan Job", model.ScanJobs, "scanType").Require().Index(),
			metadata.EnumField(model.FieldSeverity, "Severity", "info", "low", "medium", "high", "critical").Require().Index(),
			metadata.NewField(model.FieldCategory, "Category", metadata.KindString).Require().Index().Sort(),
			metadata.NewField(model.FieldEndpoint, "Endpoint", metadata.KindString).Require(),
			metadata.NewField(model.FieldDescription, "Description", metadata.KindString),
			metadata.EnumField(model.FieldRemediationStatus, "Remediation Status", "open", "confirmed", "falsePositive", "fixed", "accepted").Index(),
			metadata.NewField(model.FieldFirstSeenAt, "First Seen At", metadata.KindDatetime).Sort(),
			metadata.NewField(model.FieldLastSeenAt, "Last Seen At", metadata.KindDatetime).Sort(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			Fields:      []string{"scanJobId", "severity", "category", "endpoint", "remediationStatus", "lastSeenAt"},
			Filters:     []string{"scanJobId", "severity", "remediationStatus"},
			DefaultSort: metadata.Ptr("-lastSeenAt"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Workflow: &metadata.EntityWorkflow{
			StateField: model.FieldRemediationStatus, InitialState: model.FindingOpen,
			TerminalStates: []string{model.FindingFixed, model.FindingFalsePositive, model.FindingAccepted},
			Transitions: []metadata.WorkflowTransition{
				metadata.Transition(model.FindingConfirm, model.FindingOpen, model.FindingConfirmed, "Confirm"),
				metadata.Transition(model.FindingMarkFixed, model.FindingConfirmed, model.FindingFixed, "Mark Fixed"),
				metadata.Transition(model.FindingMarkFalsePositive, model.FindingOpen, model.FindingFalsePositive, "Mark False Positive"),
				metadata.Transition(model.FindingAccept, model.FindingOpen, model.FindingAccepted, "Accept Risk"),
			},
		},
		Audit: &metadata.EntityAuditConfig{Enabled: true},
	}
}

func init() { metadata.SubmitEntity(ScanFindingEntity) }
