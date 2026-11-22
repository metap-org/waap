// Layer: APP · declared-action logic. The hasConfig sync.

// Package policy is what the zones domain says about a zone's rules as a whole: whether the zone
// has any (SyncConfigState), whether they are fit to publish (Validate), what a tenant has used
// of its plan (Usage) and the export of a tenant's data (Export).
package policy

import (
	"context"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// SyncConfigState is the action waf.zones.syncConfigState: it recomputes hasConfig from the zone's
// actual DdosPolicy/FirewallRule records and writes it back. Idempotent, so calling it at any time
// (or twice) is harmless. It exists because PolicyCondition (the grammar a workflow guard is
// written in) has no count operator over related records, which is why zone.go documents hasConfig
// as a technical field "the app layer flips"; this is that app layer.
func SyncConfigState(ctx context.Context, backend crud.RecordBackend, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {

	anyForZone := func(entity string) (bool, error) {
		input := query.ListInput{Limit: jx.ExistenceLimit, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}
		result, err := backend.List(ctx, entity, input, rc)
		if err != nil {
			return false, err
		}
		return len(result.Data) > 0, nil
	}

	// A caller who cannot list rules cannot be told a zone has none either — surfacing "false"
	// here would silently flip hasConfig off on a permission error.
	hasDdos, err := anyForZone(model.DdosPolicies)
	if err != nil {
		return nil, err
	}
	hasFirewall, err := anyForZone(model.FirewallRules)
	if err != nil {
		return nil, err
	}
	hasConfig := hasDdos || hasFirewall

	got, err := backend.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	zone := got.Record

	currentHasConfig, _ := jx.AsBool(zone.Data[model.FieldHasConfig])
	if currentHasConfig == hasConfig {
		// Already correct — skip the write so this doesn't bump version/updatedAt on every call
		// and turn an idempotent sync into a source of version conflicts for the portal.
		return map[string]any{"hasConfig": hasConfig, "changed": false}, nil
	}

	patch := crud.JsonObject{model.FieldHasConfig: jx.Bool(hasConfig)}
	_, err = backend.Update(ctx, model.Zones, zoneID, zone.Version, patch, rc, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{"hasConfig": hasConfig, "changed": true}, nil
}
