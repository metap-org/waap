// Layer: APP · entity declaration. metap reads it to build the table, the REST routes, the permission checks and the workflow.

// waf.policy_versions — an immutable snapshot of one zone's policy: its DDoS policies, firewall
// rules and IP access lists as they stood right after a change. Rows are written by zones-service
// itself (every change to one of those three entities, and every rollback), never through the
// portal: only the admin role may create one over REST, so a role that may edit rules cannot forge
// history. Rolling back is an action on a version, and itself records a new version, so the
// history only ever grows.
//
// Tenant-wide rules (zoneId empty) apply to every zone but belong to none, so they are not in a
// zone's snapshot and a change to one records nothing.
package entities

import (
	"github.com/metap-org/metap/pkg/metadata"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/reconciler"
	"github.com/metap-org/waap/model"
)

func PolicyVersionEntity() metadata.EntityDefinition {
	return metadata.EntityDefinition{
		Name:      model.PolicyVersions,
		Label:     "Policy Version",
		TableName: reconciler.QualifiedTableNameIn(model.PolicyVersions, "waf"),
		Fields: []metadata.EntityField{
			metadata.RefField(model.FieldZoneID, "Zone", model.Zones, "hostname").Require().Index(),
			// Copied from the zone when the record is created, so a role held inside an account can
			// reach it (a scope is read off the record itself). Empty on a tenant-wide record.
			metadata.NewField(model.FieldAccountID, "Account", metadata.KindString).Index(),
			metadata.NewField(model.FieldVersion, "Version", metadata.KindNumber).Require().Sort(),
			metadata.EnumField(model.FieldSource, "Source", "baseline", "change", "rollback").Require(),
			metadata.NewField(model.FieldSummary, "Summary", metadata.KindString),
			// Set only on a rollback: the version whose content was restored.
			metadata.NewField(model.FieldRestoredFrom, "Restored from", metadata.KindNumber),
			metadata.NewField(model.FieldContentHash, "Content hash", metadata.KindString),
			metadata.NewField(model.FieldSnapshot, "Snapshot", metadata.KindJSON).Require(),
		},
		ListViews: []metadata.EntityListView{{
			Name: "default", Label: "Default",
			// The snapshot is large; it is read from a single version, not listed.
			Fields:      []string{model.FieldZoneID, "number", "source", "summary", "restoredFrom"},
			Filters:     []string{model.FieldZoneID, "source"},
			DefaultSort: metadata.Ptr("-number"),
			MaxLimit:    model.DefaultMaxListLimit,
		}},
		Actions: []metadata.EntityAction{
			{Name: model.ActionRollback, Label: "Roll back to this version", Description: "Restores the zone's rules, DDoS policies and IP lists to this version.", Scope: metadata.ActionScopeRecord, Requires: "read"},
		},
		// Two edits at once must not both become "version 5": the writer that loses retries with the
		// next number (versions.RecordVersion).
		UniqueConstraints: []metadata.EntityUniqueConstraint{{Fields: []string{model.FieldZoneID, model.FieldVersion}}},
		Scopes:            []permission.ScopeBinding{metadata.Scope(model.ScopeAccount, model.FieldAccountID)},
		Audit:             &metadata.EntityAuditConfig{Enabled: true, Parent: model.FieldZoneID},
	}
}

func init() { metadata.SubmitEntity(PolicyVersionEntity) }
