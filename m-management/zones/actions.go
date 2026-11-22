// Layer: APP · wiring. Registers the declared actions with metap and mounts the few non-action routes; each handler only calls the logic in the files named below.

// What the zones domain plugs into metap, in one place (see ../cmd/management-service/main.go):
//
//   - RegisterActions → app.Options.Actions: every declared action of this service's entities, each
//     a thin handler over a business function (netcheck, presets, policy, versions).
//   - MountRoutes → app.Options.Routes: only what is not an entity action (health).
package zones

import (
	"context"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/netcheck"
	"github.com/metap-org/waap/m-management/zones/policy"
	"github.com/metap-org/waap/m-management/zones/presets"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the handlers for the actions declared on this service's
// entities (see entities: Actions).
func RegisterActions(cfg Config, plans billing.PlanLookup) func(*httpserver.AppState, *crud.ActionRegistry) error {
	return func(state *httpserver.AppState, reg *crud.ActionRegistry) error {
		return registerActions(cfg, plans, state, reg)
	}
}

func registerActions(cfg Config, plans billing.PlanLookup, state *httpserver.AppState, reg *crud.ActionRegistry) error {
	for _, a := range []struct {
		entity, name string
		run          crud.ActionHandler
	}{
		{model.Zones, model.ActionVerifyDNS, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(netcheck.VerifyDNS(ctx, cfg, state, c.RC, c.ID))
		}},
		{model.Zones, model.ActionTestOrigin, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(netcheck.TestOrigin(ctx, cfg, state, c.RC, c.ID))
		}},
		{model.Zones, model.ActionSyncConfigState, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(policy.SyncConfigState(ctx, state.Crud, c.RC, c.ID))
		}},
		{model.Zones, model.ActionResolveHostname, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(cfg.ResolveHostname(ctx, c.Input))
		}},
		{model.Zones, model.ActionProbeOrigin, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			spec := netcheck.OriginSpecOf(crud.JsonObject{
				model.FieldOriginAddress: c.Input[model.FieldOriginAddress], model.FieldOriginProtocol: c.Input[model.FieldOriginProtocol],
				model.FieldOriginPort: c.Input[model.FieldOriginPort], model.FieldOriginHostHeader: c.Input[model.FieldOriginHostHeader],
			})
			if spec.HostHeader == "" {
				spec.HostHeader, _ = jx.AsString(c.Input[model.FieldHostname])
			}
			return crud.ToObject(cfg.ProbeOrigin(ctx, spec), nil)
		}},
		{model.Zones, model.ActionApplyPreset, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			key, _ := jx.AsString(c.Input["preset"])
			// Through the guarded backend, as a person's own edits are: validation, plan limits,
			// the zone's account and policy versions all apply.
			guarded := Guarded(state.Crud, plans)
			return crud.ToObject(presets.Apply(ctx, guarded, c.ID, key, c.RC))
		}},
		{model.Zones, model.ActionValidatePolicy, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(policy.Validate(ctx, state.Crud, c.ID, c.RC))
		}},
		{model.Zones, model.ActionExportData, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(policy.Export(ctx, state.Crud, c.RC))
		}},
		{model.Zones, model.ActionPlanUsage, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(policy.Usage(ctx, state.Crud, plans, c.RC))
		}},
		{model.PolicyVersions, model.ActionRollback, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			// state.Crud, not the guarded wrapper: the rollback records its own single version.
			result, err := versions.RollbackToVersion(ctx, state.Crud, c.ID, c.RC)
			if err != nil {
				return nil, err
			}
			return crud.ToObject(result, nil)
		}},
		{model.Domains, model.ActionVerifyDNS, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			var body netcheck.VerifyDNSBody
			body.Method, _ = jx.AsString(c.Input["method"])
			body.Host, _ = jx.AsString(c.Input["host"])
			if raw, ok := c.Input["expectedToken"]; ok {
				if tok, ok := jx.AsString(raw); ok {
					body.ExpectedToken = &tok
				}
			}
			return crud.ToObject(netcheck.VerifyDomainDNS(ctx, cfg, state, c.RC, c.ID, body))
		}},
	} {
		if err := reg.Register(a.entity, a.name, a.run); err != nil {
			return err
		}
	}
	return nil
}
