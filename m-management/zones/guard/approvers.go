// Layer: APP · guard. Four-eyes may only be switched on when two people could approve (OQ-34).

package guard

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// ApproverCounter says how many people could approve a change to an account's policy. accountID is
// uuid.Nil for an account that does not exist yet.
type ApproverCounter interface {
	CountApprovers(ctx context.Context, accountID uuid.UUID, rc *permission.RequestContext) (int, error)
}

// minFourEyesApprovers: with one, that person's own changes could never be approved.
const minFourEyesApprovers = 2

// fourEyesGuard refuses to switch an account to four-eyes without enough approvers. It judges the
// moment of switching on; people leaving later is not watched.
type fourEyesGuard struct {
	passthrough
	approvers ApproverCounter
}

func (g *fourEyesGuard) require(ctx context.Context, accountID uuid.UUID, rc *permission.RequestContext) error {
	count, err := g.approvers.CountApprovers(ctx, accountID, rc)
	if err != nil || count >= minFourEyesApprovers {
		return err
	}
	return crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed, map[string][]string{
		model.FieldChangeControl: {"Two people who may change the rules are needed before two-person approval can be switched on."},
	})
}

func asksFourEyes(data crud.JsonObject) bool {
	mode, _ := jx.AsString(data[model.FieldChangeControl])
	return mode == model.ChangeControlFourEyes
}

func (g *fourEyesGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if g.approvers != nil && entity == model.Accounts && asksFourEyes(data) {
		if err := g.require(ctx, uuid.Nil, rc); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

func (g *fourEyesGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if g.approvers != nil && entity == model.Accounts && asksFourEyes(data) {
		got, err := g.inner.Get(ctx, entity, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		// An edit of an account that already has it, a rename say, is not switching it on.
		if !asksFourEyes(got.Record.Data) {
			if err := g.require(ctx, id, rc); err != nil {
				return crud.RecordDto{}, err
			}
		}
	}
	return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}
