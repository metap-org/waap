// Layer: APP · guard. Plan limits: refuses a create past the tenant's plan.

package guard

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

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

// quotaLocks serialises count-then-create for one quota scope (the tenant's zones, or one zone's
// rules): without it N concurrent creates all read the same count and overshoot the limit by N-1.
// It is in-process, which is what the management service is (one process); with replicas this needs
// a database-side guard (advisory lock in the creating transaction) instead.
var quotaLocks keyedMutex

type keyedMutex struct{ m sync.Map }

// lock takes the mutex for key and returns its release.
func (k *keyedMutex) lock(key string) func() {
	v, _ := k.m.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// quotaScope names what a create counts against, or "" when the entity has no quota.
func quotaScope(entity string, data crud.JsonObject, rc *permission.RequestContext) string {
	switch entity {
	case model.Zones:
		return rc.TenantID + "|" + entity
	case model.FirewallRules:
		if zone := versions.ZoneOfRule(data); zone != uuid.Nil {
			return rc.TenantID + "|" + entity + "|" + zone.String()
		}
	}
	return ""
}

func (g *quotaGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if g.plans != nil {
		if scope := quotaScope(entity, data, rc); scope != "" {
			defer quotaLocks.lock(scope)()
		}
	}
	if err := g.checkQuota(ctx, entity, data, rc); err != nil {
		return crud.RecordDto{}, err
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

// Update closes the one way around the per-zone rule limit: moving a rule to another zone is not a
// create, but it adds a rule to that zone.
func (g *quotaGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.FirewallRules {
		if to := versions.ZoneOfRule(data); to != uuid.Nil {
			current, err := g.inner.Get(ctx, entity, id, rc)
			if err != nil {
				return crud.RecordDto{}, err
			}
			if versions.ZoneOfRule(current.Record.Data) != to {
				if g.plans != nil {
					defer quotaLocks.lock(quotaScope(entity, data, rc))()
				}
				if err := g.checkQuota(ctx, entity, data, rc); err != nil {
					return crud.RecordDto{}, err
				}
			}
		}
	}
	return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
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
		// Fail open on purpose (pinned by TestPlanQuotasFollowWhatTheBillingDomainSays): a billing
		// hiccup must not block onboarding. Logged at error level so it is seen and alerted on.
		slog.Error("plan limit not checked: plans could not be resolved", "tenant", tenant, "error", err)
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
