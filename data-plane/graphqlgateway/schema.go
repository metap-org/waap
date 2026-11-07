// Package graphqlgateway is WAF's own GraphQL BFF — a bespoke, forwarding gateway, not a port
// of metap-graphql's generic dynamic-schema-from-metadata engine (that engine was never ported
// to metap-go at all; see this repo's own notes on why the port stopped there). Every entity
// field here is built from a small, hardcoded list of the 11 entities the 3 WAF services own
// (survivable because that list almost never changes, unlike a true low-code platform's), and
// every field just forwards to the matching upstream's crud.RecordBackend — in this case a real
// gRPC call (grpcapi.GrpcBackend), the same client already used by the 3 Go WAF services'
// cross-service calls.
//
// Naming convention (matches the Rust gateway's metap-graphql::naming so a frontend query
// string is portable either way): "waf.ddos_policies" -> get field "wafDdosPolicies", list
// field "wafDdosPoliciesList", mutations "createWafDdosPolicies"/"updateWafDdosPolicies"/
// "deleteWafDdosPolicies", plus "transitionWafDdosPolicies" only for an entity with a workflow.
package graphqlgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/graphql-go/graphql"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
)

// EntityBackend is one entity this gateway serves, plus which upstream backend answers for it.
type EntityBackend struct {
	Name        string // "waf.zones"
	Backend     crud.RecordBackend
	HasWorkflow bool
}

type contextKey int

const requestContextKey contextKey = 0

func withRequestContext(ctx context.Context, rc *permission.RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey, rc)
}

func requestContextFrom(ctx context.Context) *permission.RequestContext {
	rc, _ := ctx.Value(requestContextKey).(*permission.RequestContext)
	if rc == nil {
		// Unreachable in practice — the HTTP handler always sets this before calling
		// graphql.Do — but a resolver must never nil-deref on a programming slip.
		return &permission.RequestContext{}
	}
	return rc
}

var recordType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Record",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
		"entity":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"status":    &graphql.Field{Type: graphql.String},
		"data":      &graphql.Field{Type: jsonScalar},
		"version":   &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"createdAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"updatedAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
})

var recordListType = graphql.NewObject(graphql.ObjectConfig{
	Name: "RecordList",
	Fields: graphql.Fields{
		"records":    &graphql.Field{Type: graphql.NewList(recordType)},
		"nextCursor": &graphql.Field{Type: graphql.String},
	},
})

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// fieldBase turns "waf.ddos_policies" into "wafDdosPolicies".
func fieldBase(entityName string) string {
	prefix, local, found := strings.Cut(entityName, ".")
	if !found {
		local = prefix
		prefix = ""
	}
	segments := strings.Split(local, "_")
	for i, s := range segments {
		segments[i] = capitalize(s)
	}
	return prefix + strings.Join(segments, "")
}

func recordToMap(dto crud.RecordDto) map[string]interface{} {
	data, _ := jsonObjectToMap(dto.Data)
	return map[string]interface{}{
		"id": dto.ID.String(), "entity": dto.Entity, "status": dto.Status,
		"data": data, "version": dto.Version,
		"createdAt": dto.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		"updatedAt": dto.UpdatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func jsonObjectToMap(obj crud.JsonObject) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(obj))
	for k, raw := range obj {
		// False positive below: jsonObjectToMap converts a crud.JsonObject (map[string]json.RawMessage)
		// into the untyped map[string]interface{} shape this bespoke, no-codegen GraphQL gateway's
		// resolvers need for all 11 entities across 3 upstream services — the field set is
		// metadata-driven at runtime, never known at compile time, so there is no single concrete
		// struct to deserialize into here either (same reasoning as custom.go's forwardCustomMutation).
		// The rule's match spans from this declaration through the Unmarshal call below, and
		// Semgrep anchors a multi-line match's suppression to its FIRST line, not the call site.
		var v interface{} // nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func mapToJsonObject(m map[string]interface{}) (crud.JsonObject, error) {
	out := make(crud.JsonObject, len(m))
	for k, v := range m {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = raw
	}
	return out, nil
}

func parseID(args map[string]interface{}, name string) (uuid.UUID, error) {
	s, _ := args[name].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("%s is not a valid id: %w", name, err)
	}
	return id, nil
}

func int32Arg(args map[string]interface{}, name string) int32 {
	switch v := args[name].(type) {
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	case float64:
		return int32(v)
	default:
		return 0
	}
}

func resultError(status uint16, code string, message *string, fieldErrors map[string][]string) error {
	return newServiceError(status, code, message, fieldErrors)
}

func getField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: recordType,
		Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			id, err := parseID(p.Args, "id")
			if err != nil {
				return nil, err
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.Get(p.Context, eb.Name, id, rc)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			return recordToMap(result.Data.Record), nil
		},
	}
}

func listField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: recordListType,
		Args: graphql.FieldConfigArgument{
			"filter": &graphql.ArgumentConfig{Type: jsonScalar},
			"limit":  &graphql.ArgumentConfig{Type: graphql.Int},
			"sort":   &graphql.ArgumentConfig{Type: graphql.String},
			"cursor": &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			input := query.ListInput{Limit: 50}
			if _, ok := p.Args["limit"]; ok {
				input.Limit = int64(int32Arg(p.Args, "limit"))
			}
			if sortVal, ok := p.Args["sort"].(string); ok {
				input.Sort = &sortVal
			}
			if cursorVal, ok := p.Args["cursor"].(string); ok {
				input.Cursor = &cursorVal
			}
			if filter, ok := p.Args["filter"].(map[string]interface{}); ok {
				for k, v := range filter {
					input.Filters = append(input.Filters, [2]string{k, fmt.Sprintf("%v", v)})
				}
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.List(p.Context, eb.Name, input, rc)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			records := make([]map[string]interface{}, 0, len(result.Data))
			for _, r := range result.Data {
				records = append(records, recordToMap(r))
			}
			var nextCursor interface{}
			if result.Page != nil && result.Page.NextCursor != nil {
				nextCursor = *result.Page.NextCursor
			}
			return map[string]interface{}{"records": records, "nextCursor": nextCursor}, nil
		},
	}
}

func createField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewNonNull(recordType),
		Args: graphql.FieldConfigArgument{
			"data": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			dataMap, _ := p.Args["data"].(map[string]interface{})
			data, err := mapToJsonObject(dataMap)
			if err != nil {
				return nil, err
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.Create(p.Context, eb.Name, data, rc, nil)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			return recordToMap(result.Data), nil
		},
	}
}

func updateField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewNonNull(recordType),
		Args: graphql.FieldConfigArgument{
			"id":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			"version": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			"data":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			id, err := parseID(p.Args, "id")
			if err != nil {
				return nil, err
			}
			dataMap, _ := p.Args["data"].(map[string]interface{})
			data, err := mapToJsonObject(dataMap)
			if err != nil {
				return nil, err
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.Update(p.Context, eb.Name, id, int32Arg(p.Args, "version"), data, rc, nil)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			return recordToMap(result.Data), nil
		},
	}
}

func deleteField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewNonNull(recordType),
		Args: graphql.FieldConfigArgument{
			"id":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			"version": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			id, err := parseID(p.Args, "id")
			if err != nil {
				return nil, err
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.Delete(p.Context, eb.Name, id, int32Arg(p.Args, "version"), rc, nil)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			return recordToMap(result.Data), nil
		},
	}
}

func transitionField(eb EntityBackend) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewNonNull(recordType),
		Args: graphql.FieldConfigArgument{
			"id":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			"action":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"version": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			"data":    &graphql.ArgumentConfig{Type: jsonScalar},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			id, err := parseID(p.Args, "id")
			if err != nil {
				return nil, err
			}
			action, _ := p.Args["action"].(string)
			var data crud.JsonObject
			if dataMap, ok := p.Args["data"].(map[string]interface{}); ok {
				data, err = mapToJsonObject(dataMap)
				if err != nil {
					return nil, err
				}
			}
			rc := requestContextFrom(p.Context)
			result, err := eb.Backend.Transition(p.Context, eb.Name, id, action, int32Arg(p.Args, "version"), data, rc, nil)
			if err != nil {
				return nil, err
			}
			if !result.IsOk {
				return nil, resultError(result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			}
			return recordToMap(result.Data), nil
		},
	}
}

// BuildSchema assembles the full schema: generic CRUD fields for every entity in entities, plus
// the 8 hand-written custom business-action mutations (addCustomFields).
func BuildSchema(entities []EntityBackend, rest RestURLs) (graphql.Schema, error) {
	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}

	for _, eb := range entities {
		base := fieldBase(eb.Name)
		pascal := capitalize(base)
		queryFields[base] = getField(eb)
		queryFields[base+"List"] = listField(eb)
		mutationFields["create"+pascal] = createField(eb)
		mutationFields["update"+pascal] = updateField(eb)
		mutationFields["delete"+pascal] = deleteField(eb)
		if eb.HasWorkflow {
			mutationFields["transition"+pascal] = transitionField(eb)
		}
	}

	addCustomFields(mutationFields, rest)

	queryType := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: queryFields})
	mutationType := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: mutationFields})
	return graphql.NewSchema(graphql.SchemaConfig{Query: queryType, Mutation: mutationType})
}
