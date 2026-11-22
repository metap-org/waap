// Layer: APP · guard. Puts a new zone in its account; guards account close.

package guard

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// ---- account -----------------------------------------------------------------------------------

// accountGuard puts a new zone in its account, copies a zone's account onto the rules created for
// it, and refuses to close an account while a zone of it is running (this file).
type accountGuard struct{ passthrough }

func (g *accountGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	switch {
	case entity == model.Zones && hasHostname(data):
		accountID, err := g.resolveAccount(ctx, data, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		data = jx.Clone(data)
		data[model.FieldAccountID] = jx.String(accountID.String())
	case versions.IsVersioned(entity):
		data = g.withZoneAccount(ctx, versions.ZoneOfRule(data), data, rc)
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

func (g *accountGuard) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.Accounts && action == model.AccountClose {
		if err := g.refuseCloseWhileRunning(ctx, id, rc); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}

// DefaultAccountSlug names the account zones fall into when none is chosen.
const DefaultAccountSlug = "default"

// resolveAccount returns the account a new zone goes into: the one named in data (it must exist
// and be open), else the tenant's default account, created on first use.
func (b *accountGuard) resolveAccount(ctx context.Context, data crud.JsonObject, rc *permission.RequestContext) (uuid.UUID, error) {
	if raw, ok := jx.AsString(data[model.FieldAccountID]); ok && raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, crud.NewServiceErrorFields(422, errcode.ValidationFailed,
				map[string][]string{model.FieldAccountID: {"not a valid id"}})
		}
		got, err := b.inner.Get(ctx, model.Accounts, id, rc)
		if err != nil {
			return uuid.Nil, err
		}
		if status, _ := jx.AsString(got.Record.Data[model.FieldStatus]); status == model.AccountClosed {
			return uuid.Nil, crud.NewServiceErrorMsg(http.StatusConflict, model.ErrAccountClosed, "This account is closed and takes no new zones.")
		}
		return id, nil
	}
	existing, err := b.inner.List(ctx, model.Accounts, query.ListInput{Limit: jx.ExistenceLimit, Filters: [][2]string{{model.FieldSlug, DefaultAccountSlug}}}, rc)
	if err != nil {
		return uuid.Nil, err
	}
	if len(existing.Data) > 0 {
		return existing.Data[0].ID, nil
	}
	// A tenant's first zone creates its default account, which the caller may not be allowed to
	// create by hand; it is housekeeping, not a grant.
	created, err := b.inner.Create(ctx, model.Accounts, crud.JsonObject{
		model.FieldName: jx.String("Default"),
		model.FieldSlug: jx.String(DefaultAccountSlug),
	}, versions.AsAdmin(rc), nil)
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

// withZoneAccount returns data with the zone's account copied in (a tenant-wide record, or a zone
// that cannot be read, gets none and stays reachable by tenant-wide roles only).
func (b *accountGuard) withZoneAccount(ctx context.Context, zoneID uuid.UUID, data crud.JsonObject, rc *permission.RequestContext) crud.JsonObject {
	if zoneID == uuid.Nil {
		return data
	}
	zone, err := b.inner.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return data
	}
	raw, ok := zone.Record.Data[model.FieldAccountID]
	if !ok {
		return data
	}
	out := jx.Clone(data)
	out[model.FieldAccountID] = raw
	return out
}

// runningZoneStates are the zone states in which an account still has traffic going through it.
var runningZoneStates = []string{model.ZoneStatusActive, model.ZoneStatusPaused}

// refuseCloseWhileRunning is UC-02's exception: closing is refused while a zone of the account is
// running, and the answer names them.
func (b *accountGuard) refuseCloseWhileRunning(ctx context.Context, accountID uuid.UUID, rc *permission.RequestContext) error {
	const show = 5
	var names []string
	var total int64
	for _, state := range runningZoneStates {
		res, err := b.inner.List(ctx, model.Zones, query.ListInput{
			Limit: show, WithTotal: true,
			Filters: [][2]string{{model.FieldAccountID, accountID.String()}, {model.FieldStatus, state}},
		}, rc)
		if err != nil {
			return err
		}
		if res.Page.Total != nil {
			total += *res.Page.Total
		} else {
			total += int64(len(res.Data))
		}
		for _, z := range res.Data {
			if h, ok := jx.AsString(z.Data[model.FieldHostname]); ok && len(names) < show {
				names = append(names, h)
			}
		}
	}
	if total == 0 {
		return nil
	}
	more := ""
	if total > int64(len(names)) {
		more = fmt.Sprintf(" and %d more", total-int64(len(names)))
	}
	return crud.NewServiceErrorMsg(http.StatusConflict, model.ErrAccountInUse,
		fmt.Sprintf("This account still has running zones: %s%s. Pause and remove them, or move them first.", strings.Join(names, ", "), more))
}
