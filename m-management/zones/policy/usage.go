// Layer: APP · declared-action logic. What a tenant has used of its plan.

package policy

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// UsageView is what the Plan page shows.
type UsageView struct {
	Enforced bool           `json:"enforced"`
	Operator bool           `json:"operator"`
	Plan     billing.Plan   `json:"plan"`
	Zones    int64          `json:"zones"`
	Plans    []billing.Plan `json:"plans"`
}

// Usage is the plan the billing domain resolves for the caller's tenant plus its zone count.
// With no plan lookup, or when it fails, the plan part is empty (nothing enforced) and the page
// says so.
func Usage(ctx context.Context, backend crud.RecordBackend, plans billing.PlanLookup, rc *permission.RequestContext) (UsageView, error) {
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return UsageView{}, err
	}
	zones, err := jx.Count(ctx, backend, model.Zones, nil, rc)
	if err != nil {
		return UsageView{}, err
	}
	view := UsageView{Zones: zones, Plans: []billing.Plan{}}
	if plans == nil {
		return view, nil
	}
	res, err := plans.Resolve(ctx, tenant)
	if err != nil {
		slog.Warn("plan usage without a plan: plans could not be resolved", "tenant", tenant, "error", err)
		return view, nil
	}
	view.Enforced, view.Operator, view.Plan = res.Enforced, res.Operator, res.Plan
	if res.Plans != nil {
		view.Plans = res.Plans
	}
	return view, nil
}
