// Layer: APP · wiring. Registers the declared actions with metap; each handler calls the logic in correlate.go, evaluate.go.

// The alerting domain's operations beyond generic CRUD.
//
// Two of the things the product spec lists as needing real code live here, and both for the
// same reason: they are decisions over a SET of records, not a read or write of one record, so
// no metadata-driven CRUD route can generate them.
//
//   - the action waf.incidents.correlate — turns recent SecurityEvents into Incidents using the
//     static v1 rule: same zoneId + same sourceIp inside a 15-minute window, threshold events or
//     more. Static, not per-tenant configurable.
//   - the action waf.alert_policies.evaluate — the AlertPolicy side: "N events in M minutes on the same
//     zone" (counted per zone, never summed across zones), producing an AlertNotification per
//     firing.
//
// SecurityEvents live in the Analytics plane (ClickHouse), not here: both sweeps read them
// through its query API with a tenant-wide read token signed for the run (ANALYTICS_URL), with the time window pushed
// into the query. Retention is ClickHouse's table TTL, so the old Postgres cleanup sweep is gone.
//
// Both are written as actions rather than background loops so metap-cron can drive them
// on a schedule while a portal user/operator can also trigger one on demand.
package alerting

import (
	"context"

	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the handlers for the actions declared on this service's entities.
func RegisterActions(cfg Config) func(*web.State, *crud.ActionRegistry) error {
	return func(state *web.State, reg *crud.ActionRegistry) error {
		return registerActions(cfg, state, reg)
	}
}

func registerActions(cfg Config, state *web.State, reg *crud.ActionRegistry) error {
	// Reads and writes through the service's own backend, as these actions always did.
	deps := state.ActionDeps()
	deps.Records = state.Crud
	for _, x := range []action.Action{
		CorrelateIncidents(cfg, state),
		EvaluateAlerts(cfg, state),
		TestAlertPolicy(cfg, state),
	} {
		if err := x.Register(reg, deps); err != nil {
			return err
		}
	}
	return nil
}

// CorrelateIncidents is the action waf.incidents.correlate: recent events into incidents.
func CorrelateIncidents(cfg Config, state *web.State) action.Action {
	return action.Entity(model.Incidents, model.ActionCorrelate,
		func(ctx context.Context, a *action.Ctx, in correlateBody) (correlation, error) {
			return correlateIncidents(ctx, a, cfg, state, in)
		})
}

// EvaluateAlerts is the action waf.alert_policies.evaluate: every enabled policy against the recent
// events, counted per zone. Each firing delivers a notification (a call out that cannot be undone)
// and records it, so it is not one step.
func EvaluateAlerts(cfg Config, state *web.State) action.Action {
	return action.Entity(model.AlertPolicies, model.ActionEvaluate,
		func(ctx context.Context, a *action.Ctx, _ action.None) (map[string]any, error) {
			return evaluateAlerts(ctx, a, cfg, state)
		})
}

// TestAlertPolicy is the action waf.alert_policies.test: the "Send test alert" button.
func TestAlertPolicy(cfg Config, state *web.State) action.Action {
	return action.Custom(model.AlertPolicies, model.ActionTest,
		func(ctx context.Context, a *action.Ctx) (any, error) {
			// testAlertPolicy reads the policy as the caller itself, so there is no LoadRecord here.
			return testAlertPolicy(ctx, a, cfg, state)
		})
}
