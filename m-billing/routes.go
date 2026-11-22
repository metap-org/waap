package billingservice

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/waap/model"
)

// internalSecretHeader carries BILLING_INTERNAL_SECRET on service-to-service calls.
const internalSecretHeader = "X-Internal-Secret"

// MountRoutes serves GET /internal/plans/resolve?tenantId=, the question other services ask. It is
// not a portal route: the caller is a service, not a person (a customer's token cannot read the
// operator's rows), so it is authenticated by a shared secret, compared in constant time. With no
// secret configured the route answers 503 rather than serve unauthenticated.
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	r.Get("/internal/plans/resolve", func(w http.ResponseWriter, req *http.Request) {
		expected := env.String("BILLING_INTERNAL_SECRET", "")
		if expected == "" {
			httpx.WriteError(w, http.StatusServiceUnavailable, "not_configured", "BILLING_INTERNAL_SECRET is not set.")
			return
		}
		if !httpx.SecretEqual(req.Header.Get(internalSecretHeader), expected) {
			httpx.WriteError(w, http.StatusUnauthorized, errcode.Unauthorized, "Invalid internal secret.")
			return
		}
		tenant, err := uuid.Parse(req.URL.Query().Get("tenantId"))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, errcode.ValidationFailed, "tenantId must be a uuid.")
			return
		}
		out, err := PlanSourceFromEnv(state.Crud).Resolve(req.Context(), tenant)
		if err != nil {
			httpserver.WriteCrudError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
	})
}

// RegisterActions registers the declared actions of this service's entities.
func RegisterActions(state *httpserver.AppState, reg *crud.ActionRegistry) error {
	if err := registerChangeActions(state, reg); err != nil {
		return err
	}
	return reg.Register(model.Subscriptions, model.ActionListTenants, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(listCustomerTenants(ctx, state, c.RC))
	})
}
