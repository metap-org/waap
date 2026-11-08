// Package management is the only place this worker talks to management-plane.
//
// Over management-plane's own GraphQL API, not its database. That is a deliberate boundary choice: a
// direct Postgres read would couple this process to metap's dedicated-table/JSONB layout and
// would bypass permission and validation entirely. Going through the API means this worker sees
// exactly what a portal user with its service account's role sees, and management-plane stays free to
// change how it stores things.
//
// Authentication is runtime.ServiceTokenSource — a real user this process logs in as, refreshed
// in the background. metap already learned the alternative the hard way: a hand-minted static
// JWT expired inside a running deployment and crashed the caller at boot.
//
// zonesURL must point at the GraphQL gateway, not at zones-service's own port — no service mounts
// a GraphQL router itself (each only exposes gRPC for the gateway to aggregate). Telemetry no
// longer goes through here at all: it is written to the Analytics plane (package analytics). loginURL (auth only) is unaffected — it still points at zones-service's
// own /auth/login, since the gateway has no login endpoint of its own and only decode-verifies
// the bearer token this process already minted.
package management

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/metap-org/metap/pkg/runtime"
)

// Record is a record as reshaped from the GraphQL response — the fixed envelope's id/status plus
// whichever data fields this package actually reads.
type Record struct {
	ID     string
	Status *string
	Data   map[string]json.RawMessage
}

func (r Record) Str(field string) (string, bool) {
	raw, ok := r.Data[field]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func (r Record) Int64(field string) (int64, bool) {
	raw, ok := r.Data[field]
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int64(n), true
}

func (r Record) Bool(field string) (bool, bool) {
	raw, ok := r.Data[field]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

// recordFromJSON builds a Record from one GraphQL result object: id/status pulled off the
// envelope, everything in fields copied into Data under its own name (already flat — none of
// the fields this package selects are Reference-typed, so no nested-object unwrapping is
// needed).
func recordFromJSON(raw map[string]json.RawMessage, fields []string) Record {
	var id string
	if idRaw, ok := raw["id"]; ok {
		_ = json.Unmarshal(idRaw, &id)
	}
	var status *string
	if statusRaw, ok := raw["status"]; ok && string(statusRaw) != "null" {
		var s string
		if json.Unmarshal(statusRaw, &s) == nil {
			status = &s
		}
	}
	data := make(map[string]json.RawMessage)
	for _, field := range fields {
		if v, ok := raw[field]; ok {
			data[field] = v
		}
	}
	return Record{ID: id, Status: status, Data: data}
}

var (
	zoneFields         = []string{"hostname", "originAddress", "protectionMode", "configVersion"}
	ddosFields         = []string{"enabled", "sensitivity", "action", "requestRateThreshold", "burstWindow", "pathPrefix", "httpMethod", "priority"}
	ruleFields         = []string{"enabled", "ruleType", "matchCondition", "rateLimitThreshold", "rateLimitWindow", "name", "action", "priority"}
	ipAccessListFields = []string{"enabled", "type", "value"}
)

type Client struct {
	http     *http.Client
	zonesURL string
	token    *runtime.ServiceTokenSource
}

func New(httpClient *http.Client, zonesURL string, token *runtime.ServiceTokenSource) *Client {
	return &Client{http: httpClient, zonesURL: zonesURL, token: token}
}

type graphqlRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables"`
}

type graphqlError struct {
	Message    string `json:"message"`
	Extensions struct {
		Status *int64 `json:"status"`
	} `json:"extensions"`
}

type graphqlResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphqlError             `json:"errors"`
}

// graphql POSTs one GraphQL query/mutation, returns the parsed data object on success. A nil
// map with no error means the request succeeded but every error present was a 404-shaped
// not-found (the only error shape a caller here treats as a normal "gone" answer, matching
// REST's old 404 -> Ok(None) behavior) — any other error bails.
func (d *Client) graphql(ctx context.Context, base, query string, variables any) (map[string]json.RawMessage, error) {
	url := base + "/graphql"
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.token.Current())
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s returned %d", url, resp.StatusCode)
	}
	var parsed graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", url, err)
	}
	if len(parsed.Errors) > 0 {
		allNotFound := true
		for _, e := range parsed.Errors {
			if e.Extensions.Status == nil || *e.Extensions.Status != 404 {
				allNotFound = false
				break
			}
		}
		if allNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("POST %s returned GraphQL errors: %+v", url, parsed.Errors)
	}
	return parsed.Data, nil
}

func (d *Client) list(ctx context.Context, base, listField string, fields []string, filter any, limit int64) ([]Record, error) {
	selection := strings.Join(fields, " ")
	query := fmt.Sprintf(
		"query($filter: Json, $limit: Int) { result: %s(filter: $filter, limit: $limit) { records { id status %s } } }",
		listField, selection,
	)
	data, err := d.graphql(ctx, base, query, map[string]any{"filter": filter, "limit": limit})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	resultRaw, ok := data["result"]
	if !ok {
		return nil, nil
	}
	var result struct {
		Records []map[string]json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return nil, fmt.Errorf("parsing %s result: %w", listField, err)
	}
	records := make([]Record, 0, len(result.Records))
	for _, r := range result.Records {
		records = append(records, recordFromJSON(r, fields))
	}
	return records, nil
}

func (d *Client) get(ctx context.Context, base, getField string, fields []string, id string) (*Record, error) {
	selection := strings.Join(fields, " ")
	query := fmt.Sprintf("query($id: ID!) { result: %s(id: $id) { id status %s } }", getField, selection)
	data, err := d.graphql(ctx, base, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if data == nil {
		// A deleted zone is the normal way this happens — the delete event and this fetch race,
		// and "gone" is a valid answer the caller turns into an unpublish.
		return nil, nil
	}
	resultRaw, ok := data["result"]
	if !ok || string(resultRaw) == "null" {
		return nil, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resultRaw, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s result: %w", getField, err)
	}
	record := recordFromJSON(raw, fields)
	return &record, nil
}

func (d *Client) Zone(ctx context.Context, zoneID string) (*Record, error) {
	return d.get(ctx, d.zonesURL, "wafZones", zoneFields, zoneID)
}

// AllZones fetches every zone, however many pages that takes.
//
// Known limitation carried over unchanged from the Rust original: only the first page is
// fetched — a tenant with more than 50 zones would need this to actually follow nextCursor,
// which the original client doesn't either.
func (d *Client) AllZones(ctx context.Context) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafZonesList", zoneFields, map[string]any{}, 50)
}

// DdosPoliciesFor returns every DDoS policy on this zone — a zone can carry more than one,
// scoped by path/method.
func (d *Client) DdosPoliciesFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafDdosPoliciesList", ddosFields, map[string]any{"zoneId": zoneID}, 50)
}

func (d *Client) RulesFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafFirewallRulesList", ruleFields, map[string]any{"zoneId": zoneID}, 50)
}

// TenantWideRulesFor returns tenant-wide ("global") FirewallRules — zoneId unset applies to
// every zone. zoneId: "" (empty value) is metap-query's own "field is unset" convention.
func (d *Client) TenantWideRulesFor(ctx context.Context) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafFirewallRulesList", ruleFields, map[string]any{"zoneId": ""}, 50)
}

func (d *Client) IPAccessListsFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafIpAccessListsList", ipAccessListFields, map[string]any{"zoneId": zoneID}, 100)
}

// TenantWideIPAccessListsFor returns tenant-wide ("global") IpAccessList entries — same
// zoneId: "" (empty) convention as TenantWideRulesFor.
func (d *Client) TenantWideIPAccessListsFor(ctx context.Context) ([]Record, error) {
	return d.list(ctx, d.zonesURL, "wafIpAccessListsList", ipAccessListFields, map[string]any{"zoneId": ""}, 100)
}
