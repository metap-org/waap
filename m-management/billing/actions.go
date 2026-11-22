// Layer: APP · wiring. Reads configuration and plugs the app's pieces into metap's app.Options; no business rules.

package billing

import (
	"context"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the declared actions of this service's entities.
func RegisterActions(cfg Config) func(*httpserver.AppState, *crud.ActionRegistry) error {
	return func(state *httpserver.AppState, reg *crud.ActionRegistry) error {
		if err := registerChangeActions(cfg, state, reg); err != nil {
			return err
		}
		return reg.Register(model.Subscriptions, model.ActionListTenants, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(listCustomerTenants(ctx, cfg, state, c.RC))
		})
	}
}
