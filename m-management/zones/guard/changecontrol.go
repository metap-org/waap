// Layer: APP · guard. Makes an account's change control hold, and keeps change requests out of reach of clients.

package guard

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// ---- change control ----------------------------------------------------------------------------

// MaxEmergencyRuleLife is how long a rule created without a change request may live (BR-6).
const MaxEmergencyRuleLife = 72 * time.Hour

type publishingKey struct{}

// Publishing marks the context a change request applies its operations under, so they pass change
// control. Only server code can set it: nothing a client sends reaches a context key.
func Publishing(ctx context.Context) context.Context {
	return context.WithValue(ctx, publishingKey{}, true)
}

func isPublishing(ctx context.Context) bool { v, _ := ctx.Value(publishingKey{}).(bool); return v }

// ChangeControl is how a change reaches the zone: its account's setting, off when the zone has no
// account or does not exist (the write is then refused for its own reason). The zone and account are
// read as admin: the answer must not depend on what the caller may see.
func ChangeControl(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) (string, error) {
	if zoneID == uuid.Nil {
		return model.ChangeControlOff, nil
	}
	admin := versions.AsAdmin(rc)
	zone, err := backend.Get(ctx, model.Zones, zoneID, admin)
	if err != nil {
		return offWhenMissing(err)
	}
	raw, _ := jx.AsString(zone.Record.Data[model.FieldAccountID])
	accountID, err := uuid.Parse(raw)
	if err != nil {
		return model.ChangeControlOff, nil
	}
	account, err := backend.Get(ctx, model.Accounts, accountID, admin)
	if err != nil {
		return offWhenMissing(err)
	}
	if mode, _ := jx.AsString(account.Record.Data[model.FieldChangeControl]); mode != "" {
		return mode, nil
	}
	return model.ChangeControlOff, nil
}

// offWhenMissing reads "not there" as off; any other failure stops the write.
func offWhenMissing(err error) (string, error) {
	if se, ok := crud.AsServiceError(err); ok && se.Status == http.StatusNotFound {
		return model.ChangeControlOff, nil
	}
	return "", err
}

// changeControlGuard refuses (1) every client write to a change request, and (2) a direct write to the
// policy of a zone whose account has change control on, except the emergency channel: creating a
// temporary rule that only restricts, and removing a temporary one.
//
// Not covered, on purpose for now: tenant-wide rules (they belong to no zone, so to no account),
// zone settings such as protectionMode and origin, and rollback (it restores content that was live).
type changeControlGuard struct{ passthrough }

func (g *changeControlGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.ChangeRequests {
		return crud.RecordDto{}, errChangeRequestReadOnly()
	}
	if versions.IsVersioned(entity) && !isPublishing(ctx) {
		if err := g.require(ctx, versions.ZoneOfRule(data), rc, isEmergencyRule(entity, data, time.Now())); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

func (g *changeControlGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.ChangeRequests {
		return crud.RecordDto{}, errChangeRequestReadOnly()
	}
	if versions.IsVersioned(entity) && !isPublishing(ctx) {
		got, err := g.inner.Get(ctx, entity, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		// Both ends: the zone the rule is in, and the one the update would move it to. Extending a
		// temporary rule is not an emergency (BR-6), so no update is.
		from := versions.ZoneOfRule(got.Record.Data)
		if err := g.require(ctx, from, rc, false); err != nil {
			return crud.RecordDto{}, err
		}
		if _, moves := data[model.FieldZoneID]; moves {
			if to := versions.ZoneOfRule(data); to != from {
				if err := g.require(ctx, to, rc, false); err != nil {
					return crud.RecordDto{}, err
				}
			}
		}
	}
	return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}

func (g *changeControlGuard) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.ChangeRequests {
		return crud.RecordDto{}, errChangeRequestReadOnly()
	}
	return g.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}

func (g *changeControlGuard) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.ChangeRequests {
		return crud.RecordDto{}, errChangeRequestReadOnly()
	}
	if versions.IsVersioned(entity) && !isPublishing(ctx) {
		got, err := g.inner.Get(ctx, entity, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		expires, _ := jx.AsString(got.Record.Data[model.FieldExpiresAt])
		if err := g.require(ctx, versions.ZoneOfRule(got.Record.Data), rc, expires != ""); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
}

func (g *changeControlGuard) require(ctx context.Context, zoneID uuid.UUID, rc *permission.RequestContext, emergency bool) error {
	if emergency {
		return nil
	}
	mode, err := ChangeControl(ctx, g.inner, zoneID, rc)
	if err != nil || mode == model.ChangeControlOff {
		return err
	}
	return crud.NewServiceErrorMsg(http.StatusConflict, model.ErrChangeRequestRequired,
		"This zone's account requires a change request for this change. Only a temporary rule that blocks, challenges or logs (at most 72 hours) can be added directly.")
}

// isEmergencyRule: a new rule that expires within MaxEmergencyRuleLife and cannot let more through.
func isEmergencyRule(entity string, data crud.JsonObject, now time.Time) bool {
	raw, _ := jx.AsString(data[model.FieldExpiresAt])
	expires, err := time.Parse(time.RFC3339, raw)
	if err != nil || !expires.After(now) || expires.After(now.Add(MaxEmergencyRuleLife)) {
		return false
	}
	switch entity {
	case model.FirewallRules:
		action, _ := jx.AsString(data[model.FieldAction])
		return action == "block" || action == "challenge" || action == "log"
	case model.IPAccessLists:
		typ, _ := jx.AsString(data[model.FieldType])
		return typ == "blacklist"
	}
	return false
}

func errChangeRequestReadOnly() error {
	return crud.NewServiceErrorMsg(http.StatusForbidden, errcode.Forbidden, "A change request is written only by its actions.")
}
