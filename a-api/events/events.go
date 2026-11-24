// Package events is the security-event model of the Analytics plane: what an event is, how one
// coming from the edge is validated, and how a structured read request becomes ClickHouse SQL.
//
// The query side is deliberately not "SQL in, rows out" (docs/architecture 5.8.9): a caller names
// fields from a fixed allowlist and gets equality filters, a time range, an optional group-by or
// time bucket, and a capped limit. Every statement is built here with `tenant_id = ?` first and
// every value bound as a parameter, so neither a field name nor a value from a request ever
// reaches the SQL text.
package events

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/kit/scope"
)

// Event is one security event as stored and as returned by the query API. JSON names match the
// Management plane's waf.security_events read model, so the portal sees the same shape it always
// did.
type Event struct {
	ID              uuid.UUID `json:"id"`
	ZoneID          string    `json:"zoneId"`
	TriggeredBy     string    `json:"triggeredBy"`
	TriggeredByID   string    `json:"triggeredById"`
	TriggeredByName string    `json:"triggeredByName"`
	Action          string    `json:"action"`
	SourceIP        string    `json:"sourceIp"`
	RequestPath     string    `json:"requestPath"`
	OccurredAt      time.Time `json:"occurredAt"`
	IngestedAt      time.Time `json:"ingestedAt"`
}

// Incoming is an event as the edge (through control-plane) reports it.
type Incoming struct {
	ZoneID          string  `json:"zoneId"`
	TriggeredBy     string  `json:"triggeredBy"`
	TriggeredByID   string  `json:"triggeredById"`
	TriggeredByName *string `json:"triggeredByName,omitempty"`
	Action          string  `json:"action"`
	SourceIP        string  `json:"sourceIp"`
	RequestPath     string  `json:"requestPath"`
	OccurredAt      string  `json:"occurredAt"`
}

var (
	triggeredByValues = map[string]bool{"ddosPolicy": true, "firewallRule": true}
	actionValues      = map[string]bool{"logged": true, "challenged": true, "blocked": true}
)

const (
	maxIDLen   = 128
	maxNameLen = 512
	maxPathLen = 4096
	// maxFutureSkew: an edge clock this far ahead is wrong, not early.
	maxFutureSkew = 5 * time.Minute
)

// Validate turns an Incoming into an Event, or says why it can't. now is injected for tests.
func (in Incoming) Validate(now time.Time) (Event, error) {
	var problems []string
	if in.ZoneID == "" || len(in.ZoneID) > maxIDLen {
		problems = append(problems, "zoneId is required (at most 128 characters)")
	}
	if !triggeredByValues[in.TriggeredBy] {
		problems = append(problems, "triggeredBy must be ddosPolicy or firewallRule")
	}
	if in.TriggeredByID == "" || len(in.TriggeredByID) > maxIDLen {
		problems = append(problems, "triggeredById is required (at most 128 characters)")
	}
	name := ""
	if in.TriggeredByName != nil {
		name = *in.TriggeredByName
		if len(name) > maxNameLen {
			problems = append(problems, "triggeredByName is longer than 512 characters")
		}
	}
	if !actionValues[in.Action] {
		problems = append(problems, "action must be logged, challenged or blocked")
	}
	ip, err := netip.ParseAddr(in.SourceIP)
	if err != nil {
		problems = append(problems, "sourceIp is not an IP address")
	}
	if len(in.RequestPath) > maxPathLen {
		problems = append(problems, "requestPath is longer than 4096 characters")
	}
	occurred, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
	if err != nil {
		problems = append(problems, "occurredAt must be an RFC 3339 timestamp")
	} else if occurred.After(now.Add(maxFutureSkew)) {
		problems = append(problems, "occurredAt is in the future")
	}
	if len(problems) > 0 {
		return Event{}, errors.New(strings.Join(problems, "; "))
	}
	return Event{
		ID: uuid.New(), ZoneID: in.ZoneID, TriggeredBy: in.TriggeredBy, TriggeredByID: in.TriggeredByID,
		TriggeredByName: name, Action: in.Action, SourceIP: ip.Unmap().String(), RequestPath: in.RequestPath,
		OccurredAt: occurred.UTC(), IngestedAt: now.UTC(),
	}, nil
}

// columns maps each queryable wire field to its column. occurredAt is the time axis, not a
// filter/group field; id and ingestedAt are never grouped or filtered on.
var columns = map[string]string{
	"zoneId":          "zone_id",
	"triggeredBy":     "triggered_by",
	"triggeredById":   "triggered_by_id",
	"triggeredByName": "triggered_by_name",
	"action":          "action",
	"sourceIp":        "source_ip",
	"requestPath":     "request_path",
}

// SelectColumns is the column list every row read scans into Event, in this order.
const SelectColumns = "id, zone_id, triggered_by, triggered_by_id, triggered_by_name, action, source_ip, request_path, occurred_at, ingested_at"

// InvalidQueryError is a request this package refuses to turn into SQL — the HTTP layer answers
// it with 400.
type InvalidQueryError struct{ Message string }

func (e *InvalidQueryError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &InvalidQueryError{Message: fmt.Sprintf(format, args...)}
}

// TimeRange bounds occurredAt; either end may be open.
type TimeRange struct {
	Since *time.Time
	Until *time.Time
}

// ParseTimeRange accepts RFC 3339 strings (empty = open).
func ParseTimeRange(since, until string) (TimeRange, error) {
	var r TimeRange
	for _, part := range []struct {
		raw  string
		name string
		dst  **time.Time
	}{{since, "since", &r.Since}, {until, "until", &r.Until}} {
		if part.raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, part.raw)
		if err != nil {
			return TimeRange{}, invalid("%s must be an RFC 3339 timestamp", part.name)
		}
		t = t.UTC()
		*part.dst = &t
	}
	return r, nil
}

// Statement is SQL plus its positional arguments.
type Statement struct {
	SQL  string
	Args []any
	// Empty: the scope matches nothing (a limit with no value left), so there is nothing to run and the
	// answer is empty.
	Empty bool
}

type where struct {
	parts []string
	args  []any
}

// scopeColumns are the dimensions of a scope.Scope this store can filter on, and the column of each.
// A scope limited on any other dimension is refused (scope.Plan), never ignored.
var scopeColumns = map[string]string{"zone": "zone_id"}

// newWhere starts a statement's conditions from the scope every read must carry: the tenant, then
// each limited dimension. empty is true when the scope matches nothing. The scope is mandatory: the
// zero Scope is refused, so no read can forget to bound itself.
func newWhere(s scope.Scope) (w *where, empty bool, err error) {
	plan, err := s.Plan("tenant_id", scopeColumns)
	if err != nil {
		return nil, false, invalid("%s", err.Error())
	}
	tenant, err := uuid.Parse(s.Tenant())
	if err != nil {
		return nil, false, invalid("the scope's tenant is not a uuid")
	}
	w = &where{parts: []string{"tenant_id = ?"}, args: []any{tenant}}
	for _, clause := range plan.Clauses[1:] {
		cond, args := clause.SQL("?")
		w.add(cond, args...)
	}
	return w, plan.None, nil
}

func (w *where) add(cond string, args ...any) {
	w.parts = append(w.parts, cond)
	w.args = append(w.args, args...)
}

func (w *where) filters(filters map[string]string) error {
	// Sorted for a deterministic statement (and testable SQL).
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, field := range keys {
		col, ok := columns[field]
		if !ok {
			return invalid("cannot filter on %q", field)
		}
		w.add(col+" = ?", filters[field])
	}
	return nil
}

func (w *where) sql() string { return strings.Join(w.parts, " AND ") }
