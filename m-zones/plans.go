// Plan limits (UC-70, UC-74), enforced here and decided by billing-service: before a zone or a
// firewall rule is created this service asks billing-service what the tenant's plan allows
// (m-billing/plans.go has the rules for which plan that is).
//
// What is enforced is what management can count: zones per tenant and firewall rules per zone.
// Requests per second cannot be, since management never sees traffic (docs/management/12).
//
// A limit stops a create; it never removes anything. A tenant over its limit (moved to a smaller
// plan) keeps what it has and cannot add more. If billing-service cannot be reached the create is
// allowed and a warning logged: a billing outage must not stop customers onboarding, and the cost
// is a limit briefly not applied.
package zonesservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

// Unlimited is the limit value that means "no limit".
const Unlimited = 0

// Plan is one plan's allowance, as billing-service reports it.
type Plan struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name"`
	MaxZones        int    `json:"maxZones"`
	MaxRulesPerZone int    `json:"maxRulesPerZone"`
}

// Resolution is billing-service's answer for one tenant.
type Resolution struct {
	Enforced bool   `json:"enforced"`
	Operator bool   `json:"operator"`
	Plan     Plan   `json:"plan"`
	Plans    []Plan `json:"plans"`
}

// PlanClient asks billing-service about a tenant's plan.
type PlanClient struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

// PlanClientFromEnv reads BILLING_URL and BILLING_INTERNAL_SECRET. Without a URL there is no
// billing service and no limits (nil).
func PlanClientFromEnv() *PlanClient {
	base := env.String("BILLING_URL", "")
	if base == "" {
		return nil
	}
	return &PlanClient{URL: base, Secret: env.String("BILLING_INTERNAL_SECRET", ""), HTTP: &http.Client{Timeout: probeTimeout}}
}

// Resolve asks what the tenant may use.
func (c *PlanClient) Resolve(ctx context.Context, tenant uuid.UUID) (Resolution, error) {
	var out Resolution
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/internal/plans/resolve?tenantId="+url.QueryEscape(tenant.String()), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("X-Internal-Secret", c.Secret)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("billing-service answered %d", resp.StatusCode)
	}
	var body struct {
		Data Resolution `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return out, err
	}
	return body.Data, nil
}

// WithPlanLimits turns quota enforcement on. Without it (nil client) a backend enforces nothing.
func (b *GuardedZonesBackend) WithPlanLimits(c *PlanClient) *GuardedZonesBackend {
	b.plans = c
	return b
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
	if b.plans == nil || (entity != model.Zones && entity != model.FirewallRules) {
		return nil
	}
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return err
	}
	res, err := b.plans.Resolve(ctx, tenant)
	if err != nil {
		slog.Warn("plan limit not checked: billing-service unreachable", "tenant", tenant, "error", err)
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
			fmt.Sprintf("The %s plan allows %d %s and this tenant already has %d. Ask your service provider to upgrade the plan.", plan.Name, limit, what, used))
	}
	return nil
}

// usageView is what the Plan page shows.
type usageView struct {
	Enforced bool   `json:"enforced"`
	Operator bool   `json:"operator"`
	Plan     Plan   `json:"plan"`
	Zones    int64  `json:"zones"`
	Plans    []Plan `json:"plans"`
}

// tenantUsage is the plan billing-service resolves for the caller's tenant plus its zone count.
// With no billing service, or when it cannot be reached, the plan part is empty (nothing enforced)
// and the page says so.
func tenantUsage(ctx context.Context, backend crud.RecordBackend, plans *PlanClient, rc *permission.RequestContext) (usageView, error) {
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return usageView{}, err
	}
	zones, err := countRecords(ctx, backend, model.Zones, nil, rc)
	if err != nil {
		return usageView{}, err
	}
	view := usageView{Zones: zones, Plans: []Plan{}}
	if plans == nil {
		return view, nil
	}
	res, err := plans.Resolve(ctx, tenant)
	if err != nil {
		slog.Warn("plan usage without a plan: billing-service unreachable", "tenant", tenant, "error", err)
		return view, nil
	}
	view.Enforced, view.Operator, view.Plan = res.Enforced, res.Operator, res.Plan
	if res.Plans != nil {
		view.Plans = res.Plans
	}
	return view, nil
}
