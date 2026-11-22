// Layer: APP · guard. The base every guard embeds.

package guard

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
)

// passthrough forwards every call to inner. A guard embeds it and overrides what it needs.
type passthrough struct{ inner crud.RecordBackend }

func (p passthrough) List(ctx context.Context, entity string, input query.ListInput, rc *permission.RequestContext) (crud.ListResult, error) {
	return p.inner.List(ctx, entity, input, rc)
}
func (p passthrough) Get(ctx context.Context, entity string, id uuid.UUID, rc *permission.RequestContext) (crud.GetResult, error) {
	return p.inner.Get(ctx, entity, id, rc)
}
func (p passthrough) GetMany(ctx context.Context, entity string, ids []uuid.UUID, rc *permission.RequestContext) ([]crud.GetManyResult, error) {
	return p.inner.GetMany(ctx, entity, ids, rc)
}
func (p passthrough) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	return p.inner.Create(ctx, entity, data, rc, reason)
}
func (p passthrough) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	return p.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}
func (p passthrough) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	return p.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}
func (p passthrough) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	return p.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
}
func (p passthrough) Aggregate(ctx context.Context, entity string, spec query.AggregateSpec, rc *permission.RequestContext) ([]json.RawMessage, error) {
	return p.inner.Aggregate(ctx, entity, spec, rc)
}
