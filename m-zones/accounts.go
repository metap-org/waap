// Accounts inside a tenant (UC-02). The entity is entities/account.go; this is what the service
// does around it: every new zone lands in an account (the chosen one, else the tenant's default),
// records under a zone carry its account so scoped roles reach them, and an account with a running
// zone cannot be closed.
package zonesservice

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
	"github.com/metap-org/waap/model"
)

// DefaultAccountSlug names the account zones fall into when none is chosen.
const DefaultAccountSlug = "default"

// resolveAccount returns the account a new zone goes into: the one named in data (it must exist
// and be open), else the tenant's default account, created on first use.
func (b *GuardedZonesBackend) resolveAccount(ctx context.Context, data crud.JsonObject, rc *permission.RequestContext) (uuid.UUID, error) {
	if raw, ok := jsonAsString(data[model.FieldAccountID]); ok && raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, crud.NewServiceErrorFields(422, errcode.ValidationFailed,
				map[string][]string{model.FieldAccountID: {"not a valid id"}})
		}
		got, err := b.inner.Get(ctx, model.Accounts, id, rc)
		if err != nil {
			return uuid.Nil, err
		}
		if status, _ := jsonAsString(got.Record.Data[model.FieldStatus]); status == model.AccountClosed {
			return uuid.Nil, crud.NewServiceErrorMsg(http.StatusConflict, model.ErrAccountClosed, "This account is closed and takes no new zones.")
		}
		return id, nil
	}
	existing, err := b.inner.List(ctx, model.Accounts, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldSlug, DefaultAccountSlug}}}, rc)
	if err != nil {
		return uuid.Nil, err
	}
	if len(existing.Data) > 0 {
		return existing.Data[0].ID, nil
	}
	// A tenant's first zone creates its default account, which the caller may not be allowed to
	// create by hand; it is housekeeping, not a grant.
	created, err := b.inner.Create(ctx, model.Accounts, crud.JsonObject{
		model.FieldName: jsonString("Default"),
		model.FieldSlug: jsonString(DefaultAccountSlug),
	}, asAdmin(rc), nil)
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

// withZoneAccount returns data with the zone's account copied in (a tenant-wide record, or a zone
// that cannot be read, gets none and stays reachable by tenant-wide roles only).
func (b *GuardedZonesBackend) withZoneAccount(ctx context.Context, zoneID uuid.UUID, data crud.JsonObject, rc *permission.RequestContext) crud.JsonObject {
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
	out := cloneJsonObject(data)
	out[model.FieldAccountID] = raw
	return out
}

// runningZoneStates are the zone states in which an account still has traffic going through it.
var runningZoneStates = []string{model.ZoneStatusActive, model.ZoneStatusPaused}

// refuseCloseWhileRunning is UC-02's exception: closing is refused while a zone of the account is
// running, and the answer names them.
func (b *GuardedZonesBackend) refuseCloseWhileRunning(ctx context.Context, accountID uuid.UUID, rc *permission.RequestContext) error {
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
			if h, ok := jsonAsString(z.Data[model.FieldHostname]); ok && len(names) < show {
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
