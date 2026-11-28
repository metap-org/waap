// Layer: APP · wiring. Registers the declared actions with metap and mounts the few non-action routes; each action only calls the logic in the files named below.

// What the zones domain plugs into metap, in one place (see ../cmd/management-service/main.go):
//
//   - RegisterActions → app.Options.Actions: every declared action of this service's entities, each
//     a typed action (metap's pkg/action) over a business function (netcheck, presets, policy,
//     versions).
//   - MountRoutes → app.Options.Routes: only what is not an entity action (health).
package zones

import (
	"context"

	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/changes"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/m-management/zones/netcheck"
	"github.com/metap-org/waap/m-management/zones/policy"
	"github.com/metap-org/waap/m-management/zones/presets"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// RegisterActions registers the handlers for the actions declared on this service's
// entities (see entities: Actions).
func RegisterActions(cfg Config, plans billing.PlanLookup) func(*web.State, *crud.ActionRegistry) error {
	return func(state *web.State, reg *crud.ActionRegistry) error {
		return registerActions(cfg, plans, state, reg)
	}
}

func registerActions(cfg Config, plans billing.PlanLookup, state *web.State, reg *crud.ActionRegistry) error {
	deps := state.ActionDeps()

	// Most actions read and write through the service's own backend, not the guard chain: what they
	// write (a check's result, a mirror field, a restore that records its own single version) is not
	// an edit a person makes. applyPreset is the exception and goes through the guards (below).
	direct := deps
	direct.Records = state.Crud

	for _, x := range []action.Action{
		netcheck.VerifyZone(cfg),
		netcheck.TestZoneOrigin(cfg),
		netcheck.VerifyDomain(cfg),
		SyncConfigState(),
		ResolveHostname(cfg),
		ProbeOrigin(cfg),
		ValidatePolicy(),
		ExportData(),
		PlanUsage(plans),
		RollbackVersion(),
	} {
		if err := x.Register(reg, direct); err != nil {
			return err
		}
	}
	for _, x := range ChangeActions(state, plans) {
		if err := x.Register(reg, deps); err != nil {
			return err
		}
	}
	return ApplyPreset(state.Crud, plans, state.Router).Register(reg, deps)
}

// ChangeActions are the actions of a change request's life (package changes): proposing one on a
// zone, then revise, submit, approve, reject and cancel on it. The operations go through the guarded
// backend; the change request rows through the service's own, since clients may not write them.
func ChangeActions(state *web.State, plans billing.PlanLookup) []action.Action {
	s := changes.Service{
		Records: state.Crud, Policy: Guarded(state.Crud, plans, state.Router),
		Permissions: state.Permissions, Locks: state.Router,
	}
	return []action.Action{
		action.Custom(model.Zones, model.ActionCreateChange, s.Propose),
		action.Custom(model.ChangeRequests, model.ActionReviseChange, s.Revise),
		action.Custom(model.ChangeRequests, model.ActionSubmitChange, s.Submit),
		action.Custom(model.ChangeRequests, model.ActionApproveChange, s.Approve),
		action.Custom(model.ChangeRequests, model.ActionRejectChange, s.Reject),
		action.Custom(model.ChangeRequests, model.ActionCancelChange, s.Cancel),
	}
}

// SyncConfigState is the action waf.zones.syncConfigState: recompute hasConfig from the zone's records.
func SyncConfigState() action.Action {
	return action.Custom(model.Zones, model.ActionSyncConfigState,
		func(ctx context.Context, a *action.Ctx) (any, error) {
			// The zone must exist and be readable: the sweep below lists by zone id and would say
			// "no config" about a zone that is not there.
			if err := a.LoadRecord(ctx, nil); err != nil {
				return nil, err
			}
			return policy.SyncConfigState(ctx, a.Records, a.RC, a.ID)
		})
}

type resolveHostnameIn struct {
	Hostname string `json:"hostname"`
}

// ResolveHostname is the action waf.zones.resolveHostname: what a hostname resolves to now.
func ResolveHostname(cfg Config) action.Action {
	return action.Entity(model.Zones, model.ActionResolveHostname,
		func(ctx context.Context, _ *action.Ctx, in resolveHostnameIn) (netcheck.HostnameView, error) {
			return cfg.ResolveHostname(ctx, in.Hostname)
		})
}

type probeOriginIn struct {
	OriginAddress    string `json:"originAddress"`
	OriginProtocol   string `json:"originProtocol"`
	OriginPort       int    `json:"originPort"`
	OriginHostHeader string `json:"originHostHeader"`
	// Hostname is the host header to send when no originHostHeader is given.
	Hostname string `json:"hostname"`
}

// ProbeOrigin is the action waf.zones.probeOrigin: can an origin be reached, before a zone exists for it.
func ProbeOrigin(cfg Config) action.Action {
	return action.Entity(model.Zones, model.ActionProbeOrigin,
		func(ctx context.Context, _ *action.Ctx, in probeOriginIn) (map[string]any, error) {
			spec := netcheck.OriginSpec{Address: in.OriginAddress, Protocol: in.OriginProtocol, Port: in.OriginPort, HostHeader: in.OriginHostHeader}
			if spec.HostHeader == "" {
				spec.HostHeader = in.Hostname
			}
			return cfg.ProbeOrigin(ctx, spec), nil
		})
}

// ValidatePolicy is the action waf.zones.validatePolicy: the zone's rules, policies and lists checked
// for mistakes. Changes nothing.
func ValidatePolicy() action.Action {
	return action.Custom(model.Zones, model.ActionValidatePolicy,
		func(ctx context.Context, a *action.Ctx) (any, error) {
			if err := a.LoadRecord(ctx, nil); err != nil { // the zone exists and the caller may read it
				return nil, err
			}
			return policy.Validate(ctx, a.Records, a.ID, a.RC)
		})
}

// ExportData is the action waf.zones.exportData: everything this tenant has configured, as one document.
func ExportData() action.Action {
	return action.Entity(model.Zones, model.ActionExportData,
		func(ctx context.Context, a *action.Ctx, _ action.None) (policy.ExportDocument, error) {
			return policy.Export(ctx, a.Records, a.RC)
		})
}

// PlanUsage is the action waf.zones.planUsage: the tenant's plan, its limits and how much is used.
func PlanUsage(plans billing.PlanLookup) action.Action {
	return action.Entity(model.Zones, model.ActionPlanUsage,
		func(ctx context.Context, a *action.Ctx, _ action.None) (policy.UsageView, error) {
			return policy.Usage(ctx, a.Records, plans, a.RC)
		})
}

// RollbackVersion is the action waf.policy_versions.rollback: make the zone's rules, DDoS policies and
// IP lists equal to a version's snapshot. The whole restore is one step, so a failure part-way leaves
// the zone as it was instead of half restored, and the rollback's own version is recorded with it.
func RollbackVersion() action.Action {
	return action.Custom(model.PolicyVersions, model.ActionRollback,
		func(ctx context.Context, a *action.Ctx) (any, error) {
			// RollbackToVersion reads the version as the caller itself, so there is no LoadRecord here.
			var result versions.RollbackResult
			err := a.Step(ctx, "rollback", func(ctx context.Context) error {
				var err error
				result, err = versions.RollbackToVersion(ctx, a.Records, a.ID, a.RC)
				return err
			})
			return result, err
		})
}

type applyPresetIn struct {
	Preset string `json:"preset"`
}

// ApplyPreset is the action waf.zones.applyPreset: the preset's protections go through the guarded
// backend (validation, plan limits, the zone's account, versions), as a person's own edits do, and
// all of them in one step: either the zone gets the whole preset and its single policy version, or
// it gets nothing.
func ApplyPreset(base crud.RecordBackend, plans billing.PlanLookup, locks guard.Locker) action.Action {
	return action.Custom(model.Zones, model.ActionApplyPreset,
		func(ctx context.Context, a *action.Ctx) (any, error) {
			var in applyPresetIn
			if err := a.Decode(&in); err != nil {
				return nil, err
			}
			// presets.Apply reads the zone through the guarded backend as the caller, so there is no
			// LoadRecord here: a zone that is missing or unreadable is its error.
			guarded := Guarded(base, plans, locks)
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
