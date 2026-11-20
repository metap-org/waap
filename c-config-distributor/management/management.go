// Package management is the only place this worker talks to management-plane.
//
// Over management-plane's own REST API (/api/{entity}, metap's recordclient), not its database.
// That is a deliberate boundary choice: a direct Postgres read would couple this process to
// metap's dedicated-table layout and would bypass permission and validation entirely. Going
// through the API means this worker sees exactly what a portal user with its service account's
// role sees, and management-plane stays free to change how it stores things.
//
// Authentication is httpx.ServiceTokenSource — a real user this process logs in as, refreshed
// in the background. metap already learned the alternative the hard way: a hand-minted static
// JWT expired inside a running deployment and crashed the caller at boot.
//
// zonesURL is zones-service itself (every entity read here is one of its own) or the API gateway
// in front of it. Telemetry does not go through here: it is written to the Analytics plane
// (package analytics).
package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/recordclient"
	"github.com/metap-org/waap/model"
)

// tenantWide is the zoneId filter value of a rule or list that applies to every zone.
var tenantWide = ""

// Record is a record as reshaped from the REST response — the fixed envelope's id/status plus
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

// recordFrom keeps id/status and the fields this package reads.
func recordFrom(dto crud.RecordDto, fields []string) Record {
	data := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		if v, ok := dto.Data[field]; ok {
			data[field] = v
		}
	}
	return Record{ID: dto.ID.String(), Status: dto.Status, Data: data}
}

var (
	zoneFields         = []string{model.FieldHostname, model.FieldOriginAddress, model.FieldProtectionMode, model.FieldConfigVersion}
	ddosFields         = []string{model.FieldEnabled, model.FieldSensitivity, model.FieldAction, model.FieldRequestRateThreshold, model.FieldBurstWindow, model.FieldPathPrefix, model.FieldHTTPMethod, model.FieldPriority}
	ruleFields         = []string{model.FieldEnabled, model.FieldRuleType, model.FieldMatchCondition, model.FieldRateLimitThreshold, model.FieldRateLimitWindow, model.FieldName, model.FieldAction, model.FieldPriority}
	ipAccessListFields = []string{model.FieldEnabled, model.FieldType, model.FieldValue}
)

type Client struct {
	records *recordclient.Client
}

// New is a Client for the service at zonesURL; token signs every call. httpClient is unused
// (recordclient keeps its own, with a timeout).
func New(_ *http.Client, zonesURL string, token *httpx.ServiceTokenSource) *Client {
	return &Client{records: recordclient.New(zonesURL, token)}
}

func (d *Client) list(ctx context.Context, entity string, fields []string, zoneID *string, limit int64) ([]Record, error) {
	input := query.ListInput{Limit: limit}
	if zoneID != nil {
		input.Filters = [][2]string{{model.FieldZoneID, *zoneID}}
	}
	res, err := d.records.List(ctx, entity, input, nil)
	if se, ok := crud.AsServiceError(err); ok && se.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", entity, err)
	}
	records := make([]Record, 0, len(res.Data))
	for _, r := range res.Data {
		records = append(records, recordFrom(r, fields))
	}
	return records, nil
}

func (d *Client) get(ctx context.Context, entity string, fields []string, id string) (*Record, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, nil
	}
	res, err := d.records.Get(ctx, entity, parsed, nil)
	if se, ok := crud.AsServiceError(err); ok && se.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s %s: %w", entity, id, err)
	}
	record := recordFrom(res.Record, fields)
	return &record, nil
}

func (d *Client) Zone(ctx context.Context, zoneID string) (*Record, error) {
	return d.get(ctx, model.Zones, zoneFields, zoneID)
}

// AllZones fetches every zone, however many pages that takes.
//
// Known limitation carried over unchanged from the Rust original: only the first page is
// fetched — a tenant with more than 50 zones would need this to actually follow nextCursor,
// which the original client doesn't either.
func (d *Client) AllZones(ctx context.Context) ([]Record, error) {
	return d.list(ctx, model.Zones, zoneFields, nil, 50)
}

// DdosPoliciesFor returns every DDoS policy on this zone — a zone can carry more than one,
// scoped by path/method.
func (d *Client) DdosPoliciesFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, model.DdosPolicies, ddosFields, &zoneID, 50)
}

func (d *Client) RulesFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, model.FirewallRules, ruleFields, &zoneID, 50)
}

// TenantWideRulesFor returns tenant-wide ("global") FirewallRules — zoneId unset applies to
// every zone. zoneId: "" (empty value) is metap-query's own "field is unset" convention.
func (d *Client) TenantWideRulesFor(ctx context.Context) ([]Record, error) {
	return d.list(ctx, model.FirewallRules, ruleFields, &tenantWide, 50)
}

func (d *Client) IPAccessListsFor(ctx context.Context, zoneID string) ([]Record, error) {
	return d.list(ctx, model.IPAccessLists, ipAccessListFields, &zoneID, 100)
}

// TenantWideIPAccessListsFor returns tenant-wide ("global") IpAccessList entries — same
// zoneId: "" (empty) convention as TenantWideRulesFor.
func (d *Client) TenantWideIPAccessListsFor(ctx context.Context) ([]Record, error) {
	return d.list(ctx, model.IPAccessLists, ipAccessListFields, &tenantWide, 100)
}
