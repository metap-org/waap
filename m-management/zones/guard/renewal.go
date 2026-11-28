// Layer: APP · guard. Keeps the emergency channel from being used to renew a temporary rule (BR-6).

package guard

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// effectKey is what a rule does, whatever it is called: two records with one key are the same
// measure. "" for a record that cannot be compared.
func effectKey(entity string, data crud.JsonObject) string {
	var parts []json.RawMessage
	switch entity {
	case model.IPAccessLists:
		parts = []json.RawMessage{data[model.FieldType], data[model.FieldValue]}
	case model.FirewallRules:
		parts = []json.RawMessage{data[model.FieldAction], data[model.FieldMatchCondition]}
	default:
		return ""
	}
	// Decoding and encoding again sorts object keys, so the same condition written two ways matches.
	values := make([]any, len(parts))
	for i, part := range parts {
		if len(part) > 0 && json.Unmarshal(part, &values[i]) != nil {
			return ""
		}
	}
	key, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(key)
}

func hasTemporary(snap versions.Snapshot, entity, key string) bool {
	for _, item := range snap[entity] {
		if expires, _ := jx.AsString(item.Data[model.FieldExpiresAt]); expires != "" && effectKey(entity, item.Data) == key {
			return true
		}
	}
	return false
}

// renewsTemporaryRule says whether a new temporary rule repeats one the zone already has, or had in
// the last MaxEmergencyRuleLife: adding it again directly would extend a measure that has to go the
// normal way. Tenant-wide records keep no history, so only the ones that still exist are seen.
func renewsTemporaryRule(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, entity string, data crud.JsonObject, rc *permission.RequestContext) (bool, error) {
	key := effectKey(entity, data)
	if key == "" {
		return false, nil
	}
	admin := versions.AsAdmin(rc)
	current, _, err := versions.ReadZoneState(ctx, backend, zoneID, admin)
	if err != nil || hasTemporary(current, entity, key) {
		return err == nil, err
	}
	if zoneID == uuid.Nil {
		return false, nil
	}
	recent, err := versions.SnapshotsSince(ctx, backend, zoneID, time.Now().Add(-MaxEmergencyRuleLife), rc)
	if err != nil {
		return false, err
	}
	for _, snap := range recent {
		if hasTemporary(snap, entity, key) {
			return true, nil
		}
	}
	return false, nil
}
