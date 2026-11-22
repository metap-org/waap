// Package billingservice is the WAAP service's plans and subscriptions (UC-70, UC-74): the
// operator defines plans, gives each customer tenant a subscription, and every other service asks
// "what may this tenant use?" here, over REST, instead of reading plan rows itself.
//
// The rows (entities/plan.go) live in the operator's tenant, named by OPERATOR_TENANT_ID. A
// customer's plan is its subscription's plan; else the operator's default plan; else FallbackPlan.
// The operator's own tenant is never limited, and with no operator configured nothing is.
package billingservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

const existenceCheckLimit = 1

// Unlimited is the limit value that means "no limit".
const Unlimited = 0

// Plan is one plan's allowance.
type Plan struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name"`
	MaxZones        int    `json:"maxZones"`
	MaxRulesPerZone int    `json:"maxRulesPerZone"`
}

// FallbackPlan applies to a customer with no subscription when the operator has defined no default
// plan: small, so that forgetting to configure plans never means unlimited.
var FallbackPlan = Plan{Name: "free", MaxZones: 1, MaxRulesPerZone: 5}

// PlanSource resolves plans from the operator's tenant.
type PlanSource struct {
	// Operator is the tenant that owns the plan rows; the zero value means no plans are enforced.
	Operator uuid.UUID
	Backend  crud.RecordBackend
}

// PlanSourceFromEnv reads OPERATOR_TENANT_ID.
func PlanSourceFromEnv(backend crud.RecordBackend) *PlanSource {
	raw, _ := env.Lookup("OPERATOR_TENANT_ID")
	id, err := uuid.Parse(raw)
	if err != nil {
		return &PlanSource{Backend: backend}
	}
	return &PlanSource{Operator: id, Backend: backend}
}

// operatorRC reads the operator's rows as the operator: the customer asking is a different tenant.
func (p *PlanSource) operatorRC() *permission.RequestContext {
	return &permission.RequestContext{TenantID: p.Operator.String(), Roles: []string{permission.RoleAdmin}, HasRoles: true}
}

func (p *PlanSource) enforced() bool { return p != nil && p.Operator != uuid.Nil }

func planFromRecord(r crud.RecordDto) Plan {
	name, _ := jsonAsString(r.Data[model.FieldName])
	zones, _ := jsonAsFloat(r.Data[model.FieldMaxZones])
	rules, _ := jsonAsFloat(r.Data[model.FieldMaxRulesPerZone])
	return Plan{ID: r.ID.String(), Name: name, MaxZones: int(zones), MaxRulesPerZone: int(rules)}
}

// Enabled lists the plans a customer can compare, cheapest first by zone allowance (unlimited last).
func (p *PlanSource) Enabled(ctx context.Context) ([]Plan, error) {
	records, err := p.enabledRecords(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Plan, 0, len(records))
	for _, r := range records {
		out = append(out, planFromRecord(r))
	}
	return sortPlans(out), nil
}

// For resolves the plan of one tenant. The bool is false when nothing is enforced for it (the
// operator itself, or no operator configured).
func (p *PlanSource) For(ctx context.Context, tenant uuid.UUID) (Plan, bool, error) {
	if !p.enforced() || tenant == p.Operator {
		return Plan{}, false, nil
	}
	rc := p.operatorRC()
	subs, err := p.Backend.List(ctx, model.Subscriptions, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldCustomerTenantID, tenant.String()}}}, rc)
	if err != nil {
		return Plan{}, false, err
	}
	if len(subs.Data) > 0 {
		if raw, ok := jsonAsString(subs.Data[0].Data[model.FieldPlanID]); ok {
			if id, err := uuid.Parse(raw); err == nil {
				if got, err := p.Backend.Get(ctx, model.Plans, id, rc); err == nil {
					return planFromRecord(got.Record), true, nil
				}
				// A subscription to a plan that is gone falls through to the default.
			}
		}
	}
	records, err := p.enabledRecords(ctx)
	if err != nil {
		return Plan{}, false, err
	}
	for _, r := range records {
		if def, _ := jsonAsBool(r.Data[model.FieldIsDefault]); def {
			return planFromRecord(r), true, nil
		}
	}
	return FallbackPlan, true, nil
}

// enabledRecords reads the operator's plans and keeps the enabled ones. Filtered here, not in the
// query: there are a handful of plans, and the choice is a business rule worth reading in one place.
func (p *PlanSource) enabledRecords(ctx context.Context) ([]crud.RecordDto, error) {
	res, err := p.Backend.List(ctx, model.Plans, query.ListInput{Limit: 50}, p.operatorRC())
	if err != nil {
		return nil, err
	}
	out := res.Data[:0:0]
	for _, r := range res.Data {
		if on, _ := jsonAsBool(r.Data[model.FieldEnabled]); on {
			out = append(out, r)
		}
	}
	return out, nil
}

func sortPlans(plans []Plan) []Plan {
	rank := func(p Plan) int {
		if p.MaxZones == Unlimited {
			return int(^uint(0) >> 1)
		}
		return p.MaxZones
	}
	for i := 1; i < len(plans); i++ {
		for j := i; j > 0 && rank(plans[j]) < rank(plans[j-1]); j-- {
			plans[j], plans[j-1] = plans[j-1], plans[j]
		}
	}
	return plans
}

// Resolution is what a service learns about one tenant's plan.
type Resolution struct {
	// Enforced is false when no limit applies to the tenant (the operator itself, or none set up).
	Enforced bool `json:"enforced"`
	// Operator is true for the operator's own tenant.
	Operator bool `json:"operator"`
	Plan     Plan `json:"plan"`
	// Plans are the enabled plans, smallest first, for a comparison table.
	Plans []Plan `json:"plans"`
}

// Resolve answers for one tenant.
func (p *PlanSource) Resolve(ctx context.Context, tenant uuid.UUID) (Resolution, error) {
	out := Resolution{Plans: []Plan{}}
	if !p.enforced() {
		return out, nil
	}
	out.Operator = tenant == p.Operator
	plan, enforced, err := p.For(ctx, tenant)
	if err != nil {
		return Resolution{}, err
	}
	out.Plan, out.Enforced = plan, enforced
	if out.Plans, err = p.Enabled(ctx); err != nil {
		return Resolution{}, err
	}
	return out, nil
}

// customerTenant is one tenant a plan can be given to.
type customerTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listCustomerTenants lists the platform's active tenants for the operator's admin to choose from.
// It reads metap's tenant registry, read-only, and answers only the operator's own admin.
func listCustomerTenants(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext) (map[string]any, error) {
	src := PlanSourceFromEnv(state.Crud)
	if !src.enforced() || rc.TenantID != src.Operator.String() || !rc.IsAdmin() {
		return nil, crud.NewServiceErrorMsg(http.StatusForbidden, "forbidden", "Only the service provider's admin can list customers.")
	}
	rows, err := state.Router.SharedPool().Query(ctx,
		"SELECT id::text, name FROM control.tenants WHERE status = 'active' AND id <> $1 ORDER BY name LIMIT 500", src.Operator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []customerTenant{}
	for rows.Next() {
		var t customerTenant
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return map[string]any{"tenants": out}, rows.Err()
}

func jsonAsString(raw []byte) (string, bool) {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

func jsonAsFloat(raw []byte) (float64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return f, true
}

func jsonAsBool(raw []byte) (bool, bool) {
	var b bool
	if len(raw) == 0 || json.Unmarshal(raw, &b) != nil {
		return false, false
	}
	return b, true
}
