// Plans and quotas (UC-70, UC-74). A tenant's plan is its `tier` (the platform admin sets it on the
// Tenants page); what each tier allows is this catalog. It is code, not data, on purpose: there is
// no payment gateway yet, so a plan is only ever assigned by a person, and a catalog change is a
// reviewed deploy rather than an edit anyone could make in a table.
//
// What is enforced here is what management can count: zones per tenant and firewall rules per
// zone. Requests per second cannot be, since management never sees traffic (docs/management/12).
// Unknown tiers, including metap's default "trial", get the free plan's limits.
//
// A limit stops a create; it never removes anything. A tenant that is over its limit (moved to a
// lower plan, or created before the limit) keeps what it has and cannot add more.
package zonesservice

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

// Unlimited is the limit value that means "no limit".
const Unlimited = 0

// Plan is one tier's allowance.
type Plan struct {
	Name            string `json:"name"`
	MaxZones        int    `json:"maxZones"`
	MaxRulesPerZone int    `json:"maxRulesPerZone"`
}

// PlanFree is the plan an unrecognised tier falls back to.
const PlanFree = "free"

var planCatalog = []Plan{
	{Name: PlanFree, MaxZones: 1, MaxRulesPerZone: 5},
	{Name: "starter", MaxZones: 5, MaxRulesPerZone: 25},
	{Name: "business", MaxZones: 25, MaxRulesPerZone: 100},
	{Name: "enterprise", MaxZones: Unlimited, MaxRulesPerZone: Unlimited},
}

// PlanCatalog is every plan, cheapest first, for the pricing table.
func PlanCatalog() []Plan { return append([]Plan(nil), planCatalog...) }

// PlanForTier resolves a tenant tier to its plan (case-insensitive).
func PlanForTier(tier string) Plan {
	tier = strings.ToLower(strings.TrimSpace(tier))
	for _, p := range planCatalog {
		if p.Name == tier {
			return p
		}
	}
	return planCatalog[0]
}

// TierLookup returns a tenant's tier.
type TierLookup func(ctx context.Context, tenantID uuid.UUID) (string, error)

// SharedPoolTier reads the tier from control.tenants.
func SharedPoolTier(pool *pgxpool.Pool) TierLookup {
	return func(ctx context.Context, tenantID uuid.UUID) (string, error) {
		var tier string
		err := pool.QueryRow(ctx, "SELECT tier FROM control.tenants WHERE id = $1", tenantID).Scan(&tier)
		return tier, err
	}
}

// WithPlanLimits turns quota enforcement on, resolving tiers with lookup. Without it a backend
// enforces nothing (the verify-only and test paths).
func (b *GuardedZonesBackend) WithPlanLimits(lookup TierLookup) *GuardedZonesBackend {
	b.tierOf = lookup
	return b
}

func (b *GuardedZonesBackend) planFor(ctx context.Context, rc *permission.RequestContext) (Plan, error) {
	id, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return Plan{}, err
	}
	tier, err := b.tierOf(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	return PlanForTier(tier), nil
}

// countRecords counts a tenant's records of entity matching filters (the caller's own view).
func countRecords(ctx context.Context, backend crud.RecordBackend, entity string, filters [][2]string, rc *permission.RequestContext) (int64, error) {
	res, err := backend.List(ctx, entity, query.ListInput{Limit: existenceCheckLimit, Filters: filters, WithTotal: true}, rc)
	if err != nil {
		return 0, err
	}
	if res.Page.Total == nil {
		return int64(len(res.Data)), nil
	}
	return *res.Page.Total, nil
}

// checkQuota refuses a create that would take the tenant past its plan. Zones are counted across
// the tenant, firewall rules within the zone they are created for.
func (b *GuardedZonesBackend) checkQuota(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext) error {
	if b.tierOf == nil || (entity != model.Zones && entity != model.FirewallRules) {
		return nil
	}
	plan, err := b.planFor(ctx, rc)
	if err != nil {
		return err
	}
	var limit int
	var filters [][2]string
	var what string
	switch entity {
	case model.Zones:
		limit, what = plan.MaxZones, "zones"
	case model.FirewallRules:
		zone := zoneOfRule(data)
		if zone == uuid.Nil {
			return nil // a tenant-wide rule belongs to no zone's quota
		}
		limit, what = plan.MaxRulesPerZone, "firewall rules per zone"
		filters = [][2]string{{model.FieldZoneID, zone.String()}}
	}
	if limit == Unlimited {
		return nil
	}
	used, err := countRecords(ctx, b.inner, entity, filters, rc)
	if err != nil {
		return err
	}
	if used >= int64(limit) {
		return crud.NewServiceErrorMsg(http.StatusConflict, model.ErrPlanLimitReached,
			fmt.Sprintf("The %s plan allows %d %s and this tenant already has %d. Ask your administrator to upgrade the plan.", plan.Name, limit, what, used))
	}
	return nil
}

// usageView is GET /api/waf.plan: the tenant's plan, what it allows and what is used.
type usageView struct {
	Tier  string `json:"tier"`
	Plan  Plan   `json:"plan"`
	Zones int64  `json:"zones"`
	Plans []Plan `json:"plans"`
}

func tenantUsage(ctx context.Context, backend crud.RecordBackend, lookup TierLookup, rc *permission.RequestContext) (usageView, error) {
	id, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return usageView{}, err
	}
	tier, err := lookup(ctx, id)
	if err != nil {
		return usageView{}, err
	}
	zones, err := countRecords(ctx, backend, model.Zones, nil, rc)
	if err != nil {
		return usageView{}, err
	}
	return usageView{Tier: tier, Plan: PlanForTier(tier), Zones: zones, Plans: PlanCatalog()}, nil
}
