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
	locks Locker
}

// Locker runs fn while holding a lock named key for tenant, so two callers with the same key run one
// after the other. metap's *tenancy.Router is the production Locker: a database advisory lock on
// the transaction fn's reads and writes join, so it holds across replicas and takes no second
// connection while it waits.
type Locker interface {
	WithKeyLock(ctx context.Context, tenantID uuid.UUID, key string, fn func(ctx context.Context) error) error
}

// processLocks is the Locker a guard has until it is given a database one (tests, and anything that
// runs with no router): a mutex per key, in this process only. It does not serialise across
// replicas, and a caller that waits on it while holding a connection can starve a pool, which is
// why the service passes the router instead.
type processLocks struct{ m sync.Map }

func (p *processLocks) WithKeyLock(ctx context.Context, _ uuid.UUID, key string, fn func(ctx context.Context) error) error {
	v, _ := p.m.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn(ctx)
}

// quotaLocks is what a guard locks with unless it is given another Locker.
var quotaLocks Locker = &processLocks{}

// locked runs fn holding the lock of the quota scope the create counts against: without it N
// concurrent creates all read the same count and overshoot the limit by N-1. It is a plain call
// when nothing is enforced or the entity has no quota.
func (g *quotaGuard) locked(ctx context.Context, scope string, rc *permission.RequestContext, fn func(ctx context.Context) error) error {
	if g.plans == nil || scope == "" {
		return fn(ctx)
	}
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return err
	}
	locks := g.locks
	if locks == nil {
		locks = quotaLocks
	}
	return locks.WithKeyLock(ctx, tenant, scope, fn)
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
	var created crud.RecordDto
	err := g.locked(ctx, quotaScope(entity, data, rc), rc, func(ctx context.Context) error {
		if err := g.checkQuota(ctx, entity, data, rc); err != nil {
			return err
		}
		var err error
		created, err = g.inner.Create(ctx, entity, data, rc, reason)
		return err
	})
	return created, err
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
				var updated crud.RecordDto
				err := g.locked(ctx, quotaScope(entity, data, rc), rc, func(ctx context.Context) error {
					if err := g.checkQuota(ctx, entity, data, rc); err != nil {
						return err
					}
					var err error
					updated, err = g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
					return err
				})
				return updated, err
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
