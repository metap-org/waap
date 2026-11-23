// Layer: APP · wiring. Registers the declared actions with metap and mounts the few non-action routes; each handler only calls the logic in the files named below.

// What the zones domain plugs into metap, in one place (see ../cmd/management-service/main.go):
//
//   - RegisterActions → app.Options.Actions: every declared action of this service's entities, each
//     a thin handler over a business function (netcheck, presets, policy, versions).
//   - MountRoutes → app.Options.Routes: only what is not an entity action (health).
package zones

import (
	"context"

	"github.com/metap-org/metap/pkg/action"
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
	} {
		if err := reg.Register(a.entity, a.name, a.run); err != nil {
			return err
		}
	}
	return registerTypedActions(cfg, plans, state, reg)
}

// registerTypedActions registers the actions written with pkg/action: typed input and output, a
// context that carries the caller, and writes, events and audit that commit together.
func registerTypedActions(cfg Config, plans billing.PlanLookup, state *httpserver.AppState, reg *crud.ActionRegistry) error {
	deps := state.ActionDeps()

	// Ownership verification reads and writes through the service's own backend, like the other
	// checks above: its cascade writes a mirror field, not an edit a person makes.
	direct := deps
	direct.Records = state.Crud
	if err := netcheck.VerifyDomain(cfg).Register(reg, direct); err != nil {
		return err
	}
	return ApplyPreset(state.Crud, plans).Register(reg, deps)
}

type applyPresetIn struct {
	Preset string `json:"preset"`
}

// ApplyPreset is the action waf.zones.applyPreset: the preset's protections go through the guarded
// backend (validation, plan limits, the zone's account, versions), as a person's own edits do, and
// all of them in one step: either the zone gets the whole preset and its single policy version, or
// it gets nothing.
func ApplyPreset(base crud.RecordBackend, plans billing.PlanLookup) action.Action {
	return action.Record(model.Zones, model.ActionApplyPreset,
		func(ctx context.Context, a *action.Ctx, _ action.None, in applyPresetIn) (presets.Result, error) {
			guarded := Guarded(base, plans)
			var out presets.Result
			err := a.Step(ctx, "apply", func(ctx context.Context) error {
				var err error
				out, err = presets.Apply(ctx, guarded, a.ID, in.Preset, a.RC)
				return err
			})
			if err != nil {
				return presets.Result{}, err
			}
			if len(out.Added) > 0 {
				a.Audit("zone.preset_applied", a.ID, out)
			}
			return out, nil
		})
}
