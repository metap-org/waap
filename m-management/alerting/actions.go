// Layer: APP · wiring. Registers the declared actions with metap; each handler calls the logic in correlate.go, evaluate.go.

// Custom (non-generic-CRUD) HTTP surface for alerting-service.
//
// Two of the things the product spec lists as needing real code live here, and both for the
// same reason: they are decisions over a SET of records, not a read or write of one record, so
// no metadata-driven CRUD route can generate them.
//
//   - POST /internal/incidents/correlate — turns recent SecurityEvents into Incidents using the
//     static v1 rule: same zoneId + same sourceIp inside a 15-minute window, threshold events or
//     more. Static, not per-tenant configurable.
//   - POST /internal/alerts/evaluate — the AlertPolicy side: "N events in M minutes on the same
//     zone" (counted per zone, never summed across zones), producing an AlertNotification per
//     firing.
//
// SecurityEvents live in the Analytics plane (ClickHouse), not here: both sweeps read them
// through its query API with the caller's own token (ANALYTICS_URL), with the time window pushed
// into the query. Retention is ClickHouse's table TTL, so the old Postgres cleanup sweep is gone.
//
// Both are written as endpoints rather than background loops so metap-cron can drive them
// on a schedule while a portal user/operator can also trigger one on demand.
package alerting

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the handlers for the actions declared on this service's entities.
func RegisterActions(cfg Config) func(*httpserver.AppState, *crud.ActionRegistry) error {
	return func(state *httpserver.AppState, reg *crud.ActionRegistry) error {
		return registerActions(cfg, state, reg)
	}
}

func registerActions(cfg Config, state *httpserver.AppState, reg *crud.ActionRegistry) error {
	if err := reg.Register(model.Incidents, model.ActionCorrelate, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		raw, err := json.Marshal(c.Input)
		if err != nil {
			return nil, err
		}
		var body correlateBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Invalid correlate input.")
		}
		return crud.ToObject(correlateIncidents(ctx, cfg, state, c.RC, body))
	}); err != nil {
		return err
	}
	if err := reg.Register(model.AlertPolicies, model.ActionEvaluate, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(evaluateAlerts(ctx, cfg, state, c.RC))
	}); err != nil {
		return err
	}
	return reg.Register(model.AlertPolicies, model.ActionTest, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(testAlertPolicy(ctx, state, c.RC, c.ID))
	})
}
