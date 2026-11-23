// Layer: APP · wiring. Reads configuration and plugs the app's pieces into metap's app.Options; no business rules.

package billing

import (
	"context"

	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the declared actions of this service's entities.
func RegisterActions(cfg Config) func(*httpserver.AppState, *crud.ActionRegistry) error {
	return func(state *httpserver.AppState, reg *crud.ActionRegistry) error {
		// Reads and writes through the service's own backend, as these actions always did.
		deps := state.ActionDeps()
		deps.Records = state.Crud
		for _, x := range []action.Action{
			RequestPlan(cfg, state),
			MyPlanRequests(cfg, state),
			DecidePlan(cfg, state),
			ListCustomerTenants(cfg, state),
		} {
			if err := x.Register(reg, deps); err != nil {
				return err
			}
		}
		return nil
	}
}

// ListCustomerTenants is the action waf.subscriptions.listTenants: the tenants a plan can be given to.
func ListCustomerTenants(cfg Config, state *httpserver.AppState) action.Action {
	return action.Entity(model.Subscriptions, model.ActionListTenants,
		func(ctx context.Context, a *action.Ctx, _ action.None) (map[string]any, error) {
			return listCustomerTenants(ctx, cfg, state, a.RC)
		})
}
