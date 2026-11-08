package events

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultListLimit = 30
	MaxListLimit     = 200
	DefaultGroups    = 100
	MaxGroups        = 500
	maxMetrics       = 10
)

// Times go in as epoch milliseconds through fromUnixTimestamp64Milli, so the driver's own
// time.Time formatting (and any precision it might drop) never matters.
func millis(t time.Time) int64 { return t.UnixMilli() }

const tsParam = "fromUnixTimestamp64Milli(?, 'UTC')"

// ListQuery is a filtered, keyset-paginated read of raw events.
type ListQuery struct {
	Filters   map[string]string
	Range     TimeRange
	Ascending bool // default newest first
	Limit     int
	Cursor    string
}

// ListPlan carries the statement plus the limit the caller must apply to compute hasMore (the
// statement fetches one extra row).
type ListPlan struct {
	Statement
	Limit int
}

func clamp(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

// EncodeCursor/decodeCursor: an opaque keyset position (occurred_at, id) of the last row served.
func EncodeCursor(e Event) string {
	raw := strconv.FormatInt(e.OccurredAt.UnixMilli(), 10) + "|" + e.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(c string) (int64, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, uuid.Nil, invalid("malformed cursor")
	}
	ms, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return 0, uuid.Nil, invalid("malformed cursor")
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return 0, uuid.Nil, invalid("malformed cursor")
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return 0, uuid.Nil, invalid("malformed cursor")
	}
	return n, u, nil
}

func PlanList(table string, tenantID uuid.UUID, q ListQuery) (ListPlan, error) {
	w := newWhere(tenantID)
	if err := w.filters(q.Filters); err != nil {
		return ListPlan{}, err
	}
	w.timeRangeMillis(q.Range)
	dir, cmp := "DESC", "<"
	if q.Ascending {
		dir, cmp = "ASC", ">"
	}
	if q.Cursor != "" {
		ms, id, err := decodeCursor(q.Cursor)
		if err != nil {
			return ListPlan{}, err
		}
		w.add("(occurred_at, id) "+cmp+" ("+tsParam+", toUUID(?))", ms, id.String())
	}
	limit := clamp(q.Limit, DefaultListLimit, MaxListLimit)
	sql := "SELECT " + SelectColumns + " FROM " + table + " WHERE " + w.sql() +
		" ORDER BY occurred_at " + dir + ", id " + dir + " LIMIT " + strconv.Itoa(limit+1)
	return ListPlan{Statement: Statement{SQL: sql, Args: w.args}, Limit: limit}, nil
}

func PlanGet(table string, tenantID uuid.UUID, id uuid.UUID) Statement {
	return Statement{
		SQL:  "SELECT " + SelectColumns + " FROM " + table + " WHERE tenant_id = ? AND id = toUUID(?) LIMIT 1",
		Args: []any{tenantID, id.String()},
	}
}

func (w *where) timeRangeMillis(r TimeRange) {
	if r.Since != nil {
		w.add("occurred_at >= "+tsParam, millis(*r.Since))
	}
	if r.Until != nil {
		w.add("occurred_at < "+tsParam, millis(*r.Until))
	}
}

// AggregateQuery mirrors metap's AggregateSpec wire shape (metrics / groupBy / bucket /
// timeField / filters / since / until / limit) so the portal's existing aggregate calls work
// unchanged. The only metrics an event has are counts — it has no numeric field to sum.
type AggregateQuery struct {
	Metrics   []string          `json:"metrics,omitempty"`
	GroupBy   string            `json:"groupBy,omitempty"`
	Bucket    string            `json:"bucket,omitempty"`
	TimeField string            `json:"timeField,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
	Since     string            `json:"since,omitempty"`
	Until     string            `json:"until,omitempty"`
	Limit     int               `json:"limit,omitempty"`
}

// AggregatePlan says which result keys the statement's columns map to, in order.
type AggregatePlan struct {
	Statement
	HasBucket bool
	HasGroup  bool
	Metrics   []string // result keys of the metric columns
}

var bucketExpr = map[string]string{
	"hour":  "toStartOfHour(occurred_at)",
	"day":   "toStartOfDay(occurred_at)",
	"week":  "toMonday(occurred_at)",
	"month": "toStartOfMonth(occurred_at)",
}

func PlanAggregate(table string, tenantID uuid.UUID, q AggregateQuery) (AggregatePlan, error) {
	if q.TimeField != "" && q.TimeField != "occurredAt" {
		return AggregatePlan{}, invalid("timeField must be occurredAt")
	}
	metrics := q.Metrics
	if len(metrics) == 0 {
		metrics = []string{"count"}
	}
	if len(metrics) > maxMetrics {
		return AggregatePlan{}, invalid("at most %d metrics", maxMetrics)
	}

	var selects, groups []string
	plan := AggregatePlan{}
	if q.Bucket != "" {
		expr, ok := bucketExpr[q.Bucket]
		if !ok {
			return AggregatePlan{}, invalid("unknown bucket %q (hour, day, week, month)", q.Bucket)
		}
		expr = "toDateTime(" + expr + ", 'UTC')"
		selects = append(selects, expr+" AS bucket")
		groups = append(groups, "bucket")
		plan.HasBucket = true
	}
	if q.GroupBy != "" {
		col, ok := columns[q.GroupBy]
		if !ok {
			return AggregatePlan{}, invalid("cannot group by %q", q.GroupBy)
		}
		selects = append(selects, col+" AS grp")
		groups = append(groups, "grp")
		plan.HasGroup = true
	}
	for i, m := range metrics {
		fn, field, hasField := strings.Cut(strings.TrimSpace(m), ":")
		if fn != "count" {
			return AggregatePlan{}, invalid("unsupported metric %q: security events have no numeric field, only count and count:<field>", m)
		}
		alias := "m" + strconv.Itoa(i)
		if hasField {
			col, ok := columns[strings.TrimSpace(field)]
			if !ok {
				return AggregatePlan{}, invalid("unknown field %q", field)
			}
			// count:<field> = rows where the field is set, as in metap.
			selects = append(selects, "toFloat64(countIf("+col+" != '')) AS "+alias)
			plan.Metrics = append(plan.Metrics, "count_"+strings.TrimSpace(field))
		} else {
			selects = append(selects, "toFloat64(count()) AS "+alias)
			plan.Metrics = append(plan.Metrics, "count")
		}
	}

	w := newWhere(tenantID)
	if err := w.filters(q.Filters); err != nil {
		return AggregatePlan{}, err
	}
	r, err := ParseTimeRange(q.Since, q.Until)
	if err != nil {
		return AggregatePlan{}, err
	}
	w.timeRangeMillis(r)

	sql := "SELECT " + strings.Join(selects, ", ") + " FROM " + table + " WHERE " + w.sql()
	if len(groups) > 0 {
		sql += " GROUP BY " + strings.Join(groups, ", ")
	}
	switch {
	case plan.HasBucket:
		sql += " ORDER BY bucket ASC"
	case plan.HasGroup:
		sql += " ORDER BY m0 DESC"
	}
	sql += " LIMIT " + strconv.Itoa(clamp(q.Limit, DefaultGroups, MaxGroups))
	plan.Statement = Statement{SQL: sql, Args: w.args}
	return plan, nil
}
