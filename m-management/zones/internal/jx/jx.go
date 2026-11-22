// Layer: APP · helper. Reading and building the JSON values a record carries; no rules, no I/O.

// Package jx is the few helpers every layer of the zones domain needs to read a field of a
// crud.JsonObject (a record's data is raw JSON, field by field) or to build one.
package jx

import (
	"context"
	"encoding/json"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
)

// ExistenceLimit asks a list for one row: "is there any", not "how many".
const ExistenceLimit = 1

// String is s as a JSON string value.
func String(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// AsString reads raw as a JSON string; ok is false when it is absent or not a string.
func AsString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// Bool is b as a JSON boolean value.
func Bool(b bool) json.RawMessage {
	out, _ := json.Marshal(b)
	return out
}

// AsBool reads raw as a JSON boolean; ok is false when it is absent or not a boolean.
func AsBool(raw json.RawMessage) (value, ok bool) {
	if len(raw) == 0 {
		return false, false
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

// AsFloat reads raw as a JSON number; ok is false when it is absent or not a number.
func AsFloat(raw json.RawMessage) (float64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return f, true
}

// Clone copies a record's data so a guard can change it without touching the caller's map.
func Clone(data crud.JsonObject) crud.JsonObject {
	out := make(crud.JsonObject, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}

// Count counts a tenant's records of entity matching filters (the caller's own view).
func Count(ctx context.Context, backend crud.RecordBackend, entity string, filters [][2]string, rc *permission.RequestContext) (int64, error) {
	res, err := backend.List(ctx, entity, query.ListInput{Limit: ExistenceLimit, Filters: filters, WithTotal: true}, rc)
	if err != nil {
		return 0, err
	}
	if res.Page.Total == nil {
		return int64(len(res.Data)), nil
	}
	return *res.Page.Total, nil
}
