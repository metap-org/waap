// Layer: APP · guard. Plan limits: refuses a create past the tenant's plan.

package guard

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// ---- plan limits -------------------------------------------------------------------------------

// quotaGuard refuses a create that would take the tenant past its plan (checkQuota, below).
// With no plan client nothing is enforced.
type quotaGuard struct {
	passthrough
	plans billing.PlanLookup
}

func (g *quotaGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if err := g.checkQuota(ctx, entity, data, rc); err != nil {
		return crud.RecordDto{}, err
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

// checkQuota refuses a create that would take the tenant past its plan. Zones are counted across
// the tenant, firewall rules within the zone they are created for.
func (b *quotaGuard) checkQuota(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext) error {
	if b.plans == nil || (entity != model.Zones && entity != model.FirewallRules) {
		return nil
	}
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return err
	}
	res, err := b.plans.Resolve(ctx, tenant)
	if err != nil {
		slog.Warn("plan limit not checked: plans could not be resolved", "tenant", tenant, "error", err)
		return nil
	}
	if !res.Enforced {
		return nil
	}
	plan := res.Plan
	var limit int
	var filters [][2]string
	var what string
	switch entity {
	case model.Zones:
		limit, what = plan.MaxZones, "zones"
	case model.FirewallRules:
		zone := versions.ZoneOfRule(data)
		if zone == uuid.Nil {
			return nil // a tenant-wide rule belongs to no zone's quota
		}
		limit, what = plan.MaxRulesPerZone, "firewall rules per zone"
		filters = [][2]string{{model.FieldZoneID, zone.String()}}
	}
	if limit == billing.Unlimited {
		return nil
	}
	used, err := jx.Count(ctx, b.inner, entity, filters, rc)
	if err != nil {
		return err
	}
	if used >= int64(limit) {
		return crud.NewServiceErrorMsg(http.StatusConflict, model.ErrPlanLimitReached,
			fmt.Sprintf("The %s plan allows %d %s and this tenant already has %d. Ask your service provider to upgrade the plan.", plan.Name, limit, what, used))
	}
	return nil
}
