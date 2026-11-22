// Layer: APP · guard. Refuses to delete a zone that is still referenced.

package guard

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// ---- references --------------------------------------------------------------------------------

// referenceGuard refuses to delete a zone that scan jobs or incidents still point at. Both are
// entities of this same service, read through the chain's inner backend as the caller, so the check
// runs with the caller's own permissions: a caller who may not read them cannot delete the zone.
type referenceGuard struct{ passthrough }

func (g *referenceGuard) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.Zones {
		blocking, err := g.zoneReferencedBy(ctx, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		if blocking != "" {
			return crud.RecordDto{}, crud.NewServiceErrorMsg(http.StatusConflict, errcode.RecordReferenced,
				fmt.Sprintf("This zone is still referenced by %q and cannot be deleted.", blocking))
		}
	}
	return g.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
}

// zoneReferencedBy returns the blocking entity name, or "" when the zone is clear to delete.
func (g *referenceGuard) zoneReferencedBy(ctx context.Context, zoneID uuid.UUID, rc *permission.RequestContext) (string, error) {
	// waf.security_events is deliberately not checked: it is an append-only log in the Analytics
	// plane, and a deleted zone's history ageing out by TTL is correct, not an orphan.
	for _, entity := range []string{model.ScanJobs, model.Incidents} {
		result, err := g.inner.List(ctx, entity, query.ListInput{Limit: jx.ExistenceLimit, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}, rc)
		if err != nil {
			return "", err
		}
		if len(result.Data) > 0 {
			return entity, nil
		}
	}
	return "", nil
}
