// Layer: APP · policy versions. Which of a zone's recorded states may be returned to, and what it held lately.

package versions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// history calls visit on the zone's versions, newest first, until it returns false.
func history(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext, visit func(crud.RecordDto) bool) error {
	sortExpr := "-" + model.FieldVersion
	for offset := int64(0); ; offset += SnapshotPage {
		page, err := backend.List(ctx, model.PolicyVersions, query.ListInput{
			Limit: SnapshotPage, Offset: offset, Sort: &sortExpr,
			Filters: [][2]string{{model.FieldZoneID, zoneID.String()}},
		}, AsAdmin(rc))
		if err != nil {
			return err
		}
		for _, row := range page.Data {
			if !visit(row) {
				return nil
			}
		}
		if int64(len(page.Data)) < SnapshotPage {
			return nil
		}
	}
}

// WasApproved says whether target holds a state of the zone that went through approval (BR-40): the
// content a change request published, or the content one was published on top of, which its
// approver saw as the "before" of the diff. Matching is by content, so a rollback to such a state
// counts too.
func WasApproved(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, target crud.RecordDto, rc *permission.RequestContext) (bool, error) {
	want, _ := jx.AsString(target.Data[model.FieldContentHash])
	found := false
	// previousIsBase: the row visited before this one (the next newer version) was published by a
	// change request, so this one is what it was published on.
	previousIsBase := false
	err := history(ctx, backend, zoneID, rc, func(row crud.RecordDto) bool {
		request, _ := jx.AsString(row.Data[model.FieldChangeRequestID])
		hash, _ := jx.AsString(row.Data[model.FieldContentHash])
		if (request != "" || previousIsBase) && hash != "" && hash == want {
			found = true
		}
		previousIsBase = request != ""
		return !found
	})
	return found, err
}

// SnapshotsSince is the zone's state in each version recorded after since.
func SnapshotsSince(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, since time.Time, rc *permission.RequestContext) ([]Snapshot, error) {
	var out []Snapshot
	var unreadable error
	err := history(ctx, backend, zoneID, rc, func(row crud.RecordDto) bool {
		if row.CreatedAt.Before(since) {
			return false
		}
		var snap Snapshot
		if unreadable = json.Unmarshal(row.Data[model.FieldSnapshot], &snap); unreadable != nil {
			return false
		}
		out = append(out, snap)
		return true
	})
	if err == nil {
		err = unreadable
	}
	return out, err
}
