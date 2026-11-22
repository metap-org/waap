// Policy versions: a snapshot of one zone's rules after every change, and rollback to one.
//
// A version is the zone's DDoS policies, firewall rules and IP access lists (zone-scoped ones;
// see entities/policyversion.go) as they stand, with a content hash so a change that leaves the
// content as it was (an update that sets the same values) records nothing. Versions are written
// with an admin copy of the caller's context, because the roles that edit rules have no create
// right on waf.policy_versions on purpose; the rules themselves are always read and written as the
// caller, so what a caller can snapshot or roll back is bounded by what they may see and edit.
package zonesservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

// versionedEntities are the entities a zone's snapshot holds, in the order they are restored.
var versionedEntities = []string{model.DdosPolicies, model.FirewallRules, model.IPAccessLists}

// Where a version came from (waf.policy_versions.source).
const (
	versionSourceBaseline = "baseline"
	versionSourceChange   = "change"
	versionSourceRollback = "rollback"
)

// snapshotPage is the page size used to read a zone's rules in full.
const snapshotPage = 50

// snapshotItem is one record in a snapshot. Version is the record's own optimistic-lock version
// at read time; it is not stored in the snapshot.
type snapshotItem struct {
	ID   string          `json:"id"`
	Data crud.JsonObject `json:"data"`
}

type zoneSnapshot map[string][]snapshotItem

// readZoneState lists everything a zone's snapshot holds, as rc may see it, plus each record's
// lock version so a restore can update it.
func readZoneState(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) (zoneSnapshot, map[string]int32, error) {
	snap := zoneSnapshot{}
	versions := map[string]int32{}
	for _, entity := range versionedEntities {
		items := []snapshotItem{}
		for offset := int64(0); ; offset += snapshotPage {
			page, err := backend.List(ctx, entity, query.ListInput{
				Limit: snapshotPage, Offset: offset,
				Filters: [][2]string{{model.FieldZoneID, zoneID.String()}},
			}, rc)
			if err != nil {
				return nil, nil, err
			}
			for _, r := range page.Data {
				items = append(items, snapshotItem{ID: r.ID.String(), Data: r.Data})
				versions[r.ID.String()] = r.Version
			}
			if int64(len(page.Data)) < snapshotPage {
				break
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		snap[entity] = items
	}
	return snap, versions, nil
}

// hashSnapshot is stable: encoding/json writes map keys in sorted order and items are sorted by id.
func hashSnapshot(snap zoneSnapshot) (string, error) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// asAdmin is rc with the admin role, for writing the history rows only.
func asAdmin(rc *permission.RequestContext) *permission.RequestContext {
	admin := *rc
	admin.Roles = []string{permission.RoleAdmin}
	admin.HasRoles = true
	admin.ScopedRoles = nil
	return &admin
}

// latestVersion returns the zone's newest version row, or nil when it has none.
func latestVersion(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) (*crud.RecordDto, error) {
	sortExpr := "-" + model.FieldVersion
	page, err := backend.List(ctx, model.PolicyVersions, query.ListInput{
		Limit: existenceCheckLimit, Sort: &sortExpr,
		Filters: [][2]string{{model.FieldZoneID, zoneID.String()}},
	}, asAdmin(rc))
	if err != nil || len(page.Data) == 0 {
		return nil, err
	}
	return &page.Data[0], nil
}

type versionNote struct {
	source       string
	summary      string
	restoredFrom int
	// force records the version even when the content equals the latest one (a rollback to the
	// state already in place is still an event worth keeping).
	force bool
}

// recordVersion snapshots the zone as rc sees it and stores it as the next version. It returns the
// version number, or 0 when nothing was recorded because the content has not changed.
func recordVersion(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext, note versionNote) (int, error) {
	snap, _, err := readZoneState(ctx, backend, zoneID, rc)
	if err != nil {
		return 0, err
	}
	hash, err := hashSnapshot(snap)
	if err != nil {
		return 0, err
	}
	latest, err := latestVersion(ctx, backend, zoneID, rc)
	if err != nil {
		return 0, err
	}
	next := 1
	if latest != nil {
		if h, _ := jsonAsString(latest.Data[model.FieldContentHash]); h == hash && !note.force {
			return 0, nil
		}
		n, _ := jsonAsFloat(latest.Data[model.FieldVersion])
		next = int(n) + 1
	}
	rawSnap, err := json.Marshal(snap)
	if err != nil {
		return 0, err
	}
	data := crud.JsonObject{
		model.FieldZoneID:      jsonString(zoneID.String()),
		model.FieldVersion:     json.RawMessage(fmt.Sprint(next)),
		model.FieldSource:      jsonString(note.source),
		model.FieldSummary:     jsonString(note.summary),
		model.FieldContentHash: jsonString(hash),
		model.FieldSnapshot:    rawSnap,
	}
	if zone, err := backend.Get(ctx, model.Zones, zoneID, rc); err == nil {
		if account, ok := zone.Record.Data[model.FieldAccountID]; ok {
			data[model.FieldAccountID] = account
		}
	}
	if note.restoredFrom > 0 {
		data[model.FieldRestoredFrom] = json.RawMessage(fmt.Sprint(note.restoredFrom))
	}
	if _, err := backend.Create(ctx, model.PolicyVersions, data, asAdmin(rc), nil); err != nil {
		return 0, err
	}
	return next, nil
}

func jsonAsFloat(raw json.RawMessage) (float64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return f, true
}

// zoneOfRule is the zone a rule record belongs to, or uuid.Nil for a tenant-wide one.
func zoneOfRule(data crud.JsonObject) uuid.UUID {
	s, ok := jsonAsString(data[model.FieldZoneID])
	if !ok {
		return uuid.Nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func isVersioned(entity string) bool {
	for _, e := range versionedEntities {
		if e == entity {
			return true
		}
	}
	return false
}

// ensureBaseline records the zone as it is before its first tracked change, so version 1 is what
// the customer had and the change that follows is version 2. Best effort, like recordChange.
func ensureBaseline(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) {
	if zoneID == uuid.Nil || versionsOff(ctx) {
		return
	}
	latest, err := latestVersion(ctx, backend, zoneID, rc)
	if err != nil || latest != nil {
		if err != nil {
			slog.Warn("policy version baseline check failed", "zone", zoneID, "error", err)
		}
		return
	}
	if _, err := recordVersion(ctx, backend, zoneID, rc, versionNote{source: versionSourceBaseline, summary: "State before the first tracked change"}); err != nil {
		slog.Warn("policy version baseline failed", "zone", zoneID, "error", err)
	}
}

// recordChange snapshots the zone after a rule change. A failure here is logged, not returned: the
// change itself already happened and refusing it now would only hide that.
func recordChange(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext, verb, entity string) {
	if zoneID == uuid.Nil || versionsOff(ctx) {
		return
	}
	if _, err := recordVersion(ctx, backend, zoneID, rc, versionNote{source: versionSourceChange, summary: verb + " " + entityLabel(entity)}); err != nil {
		slog.Warn("policy version not recorded", "zone", zoneID, "error", err)
	}
}

func entityLabel(entity string) string {
	switch entity {
	case model.DdosPolicies:
		return "DDoS policy"
	case model.FirewallRules:
		return "firewall rule"
	case model.IPAccessLists:
		return "IP access list entry"
	}
	return entity
}

// rollbackResult is what the rollback action returns.
type rollbackResult struct {
	RestoredVersion int `json:"restoredVersion"`
	NewVersion      int `json:"newVersion"`
	Created         int `json:"created"`
	Updated         int `json:"updated"`
	Deleted         int `json:"deleted"`
}

// rollbackToVersion makes the zone's rules, DDoS policies and IP lists equal to version versionID's
// snapshot. Records that still exist are updated in place, ones deleted since are created again
// (with a new id), ones added since are deleted. It writes through backend as rc, so it needs the
// same rights as making those edits by hand; it is not atomic, so a failure part-way leaves a
// partial restore that the next rollback (to the same version) completes. Then it records a new
// version so the history shows the rollback.
func rollbackToVersion(ctx context.Context, backend crud.RecordBackend, versionID uuid.UUID, rc *permission.RequestContext) (rollbackResult, error) {
	var out rollbackResult
	got, err := backend.Get(ctx, model.PolicyVersions, versionID, rc)
	if err != nil {
		return out, err
	}
	zoneID := zoneOfRule(got.Record.Data)
	if zoneID == uuid.Nil {
		return out, crud.NewServiceErrorMsg(400, errcode.ValidationFailed, "Version has no zone.")
	}
	var target zoneSnapshot
	if err := json.Unmarshal(got.Record.Data[model.FieldSnapshot], &target); err != nil {
		return out, crud.NewServiceErrorMsg(500, errcode.ValidationFailed, "Version snapshot is unreadable.")
	}
	n, _ := jsonAsFloat(got.Record.Data[model.FieldVersion])
	out.RestoredVersion = int(n)

	current, lockVersions, err := readZoneState(ctx, backend, zoneID, rc)
	if err != nil {
		return out, err
	}
	for _, entity := range versionedEntities {
		wanted := map[string]snapshotItem{}
		for _, it := range target[entity] {
			wanted[it.ID] = it
		}
		have := map[string]snapshotItem{}
		for _, it := range current[entity] {
			have[it.ID] = it
		}
		for _, it := range current[entity] {
			if _, keep := wanted[it.ID]; keep {
				continue
			}
			id, _ := uuid.Parse(it.ID)
			if _, err := backend.Delete(ctx, entity, id, lockVersions[it.ID], rc, nil); err != nil {
				return out, err
			}
			out.Deleted++
		}
		for _, it := range target[entity] {
			existing, ok := have[it.ID]
			if !ok {
				if _, err := backend.Create(ctx, entity, it.Data, rc, nil); err != nil {
					return out, err
				}
				out.Created++
				continue
			}
			if sameJSON(existing.Data, it.Data) {
				continue
			}
			id, _ := uuid.Parse(it.ID)
			if _, err := backend.Update(ctx, entity, id, lockVersions[it.ID], it.Data, rc, nil); err != nil {
				return out, err
			}
			out.Updated++
		}
	}
	if err := syncHasConfig(ctx, backend, zoneID, rc); err != nil {
		return out, err
	}
	next, err := recordVersion(ctx, backend, zoneID, rc, versionNote{
		source: versionSourceRollback, restoredFrom: out.RestoredVersion, force: true,
		summary: fmt.Sprintf("Rolled back to version %d", out.RestoredVersion),
	})
	if err != nil {
		return out, err
	}
	out.NewVersion = next
	return out, nil
}

func sameJSON(a, b crud.JsonObject) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	return string(ra) == string(rb)
}

// syncHasConfig is syncConfigState over a backend: Zone.hasConfig mirrors whether the zone has any
// DDoS policy or firewall rule, which a restore can flip either way.
func syncHasConfig(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) error {
	has := false
	for _, entity := range []string{model.DdosPolicies, model.FirewallRules} {
		page, err := backend.List(ctx, entity, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}, rc)
		if err != nil {
			return err
		}
		has = has || len(page.Data) > 0
	}
	zone, err := backend.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return err
	}
	if current, _ := jsonAsBool(zone.Record.Data[model.FieldHasConfig]); current == has {
		return nil
	}
	_, err = backend.Update(ctx, model.Zones, zoneID, zone.Record.Version, crud.JsonObject{model.FieldHasConfig: jsonBool(has)}, rc, nil)
	return err
}
