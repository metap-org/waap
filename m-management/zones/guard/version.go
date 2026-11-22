// Layer: APP · guard. Records a policy version around every rule change.

package guard

import (
	"context"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/versions"
)

// ---- versions ----------------------------------------------------------------------------------

// versionGuard records a policy version of the zone around every change to its rules (package versions):
// a baseline before the first change, a version after each.
type versionGuard struct{ passthrough }

func (g *versionGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if !versions.IsVersioned(entity) {
		return g.inner.Create(ctx, entity, data, rc, reason)
	}
	zoneID := versions.ZoneOfRule(data)
	versions.EnsureBaseline(ctx, g.inner, zoneID, rc)
	created, err := g.inner.Create(ctx, entity, data, rc, reason)
	if err == nil {
		versions.RecordChange(ctx, g.inner, zoneID, rc, "Created", entity)
	}
	return created, err
}

func (g *versionGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if !versions.IsVersioned(entity) {
		return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
	}
	zoneID := g.zoneOf(ctx, entity, id, rc)
	versions.EnsureBaseline(ctx, g.inner, zoneID, rc)
	updated, err := g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
	if err == nil {
		versions.RecordChange(ctx, g.inner, zoneID, rc, "Updated", entity)
	}
	return updated, err
}

func (g *versionGuard) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if !versions.IsVersioned(entity) {
		return g.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
	}
	zoneID := g.zoneOf(ctx, entity, id, rc)
	versions.EnsureBaseline(ctx, g.inner, zoneID, rc)
	deleted, err := g.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
	if err == nil {
		versions.RecordChange(ctx, g.inner, zoneID, rc, "Deleted", entity)
	}
	return deleted, err
}

// zoneOf is the zone a rule record belongs to (uuid.Nil when tenant-wide or unreadable).
func (g *versionGuard) zoneOf(ctx context.Context, entity string, id uuid.UUID, rc *permission.RequestContext) uuid.UUID {
	got, err := g.inner.Get(ctx, entity, id, rc)
	if err != nil {
		return uuid.Nil
	}
	return versions.ZoneOfRule(got.Record.Data)
}
