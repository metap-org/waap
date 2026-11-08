// Package store keeps security events in ClickHouse (ADR-0020).
//
// One MergeTree table, partitioned by day and ordered by (tenant_id, zone_id, occurred_at, id):
// every read is tenant-scoped and most are zone-scoped and time-bounded, so that order lets
// ClickHouse skip almost everything it doesn't need. Retention is the table's own TTL — expired
// parts are dropped by the server, replacing the batched DELETE sweep the Postgres version
// needed. The schema is applied at boot (idempotent), and the TTL is re-asserted each boot so a
// changed ANALYTICS_RETENTION_DAYS takes effect without a manual ALTER.
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/metap-org/waap/analytics-plane/events"
)

// ErrNotFound: no event with that id in the caller's tenant.
var ErrNotFound = errors.New("event not found")

// Store is what the HTTP layer needs; ClickHouse implements it, tests fake it.
type Store interface {
	Insert(ctx context.Context, tenantID uuid.UUID, evs []events.Event) error
	List(ctx context.Context, tenantID uuid.UUID, q events.ListQuery) (Page, error)
	Get(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (events.Event, error)
	Aggregate(ctx context.Context, tenantID uuid.UUID, q events.AggregateQuery) ([]map[string]any, error)
	Ping(ctx context.Context) error
}

type Page struct {
	Records    []events.Event `json:"records"`
	NextCursor *string        `json:"nextCursor"`
	HasMore    bool           `json:"hasMore"`
}

type ClickHouse struct {
	conn  driver.Conn
	table string
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Open connects to dsn (clickhouse://user:pass@host:9000) and uses database for the table.
func Open(ctx context.Context, dsn, database string) (*ClickHouse, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name %q", database)
	}
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing CLICKHOUSE_URL: %w", err)
	}
	opts.Settings = clickhouse.Settings{"max_execution_time": 30}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}
	return &ClickHouse{conn: conn, table: database + ".security_events"}, nil
}

func (c *ClickHouse) Close() error { return c.conn.Close() }

func (c *ClickHouse) Ping(ctx context.Context) error { return c.conn.Ping(ctx) }

// Migrate creates the database and table if missing and sets the retention TTL.
func (c *ClickHouse) Migrate(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		return fmt.Errorf("retention must be at least 1 day, got %d", retentionDays)
	}
	db := c.table[:len(c.table)-len(".security_events")]
	ttl := fmt.Sprintf("toDateTime(occurred_at) + INTERVAL %d DAY", retentionDays)
	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS " + db,
		`CREATE TABLE IF NOT EXISTS ` + c.table + ` (
			tenant_id         UUID,
			id                UUID,
			zone_id           String,
			triggered_by      LowCardinality(String),
			triggered_by_id   String,
			triggered_by_name String,
			action            LowCardinality(String),
			source_ip         String,
			request_path      String,
			occurred_at       DateTime64(3, 'UTC'),
			ingested_at       DateTime64(3, 'UTC')
		) ENGINE = MergeTree
		PARTITION BY toYYYYMMDD(occurred_at)
		ORDER BY (tenant_id, zone_id, occurred_at, id)
		TTL ` + ttl,
		"ALTER TABLE " + c.table + " MODIFY TTL " + ttl,
	}
	for _, s := range stmts {
		if err := c.conn.Exec(ctx, s); err != nil {
			return fmt.Errorf("migrating %s: %w", c.table, err)
		}
	}
	return nil
}

// readCtx caps every read the query API runs (the cost guard of docs 5.8.9): a runaway query is
// stopped by the server rather than left to tie up a connection.
func readCtx(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"max_execution_time": 10,
		"max_memory_usage":   1 << 30,
	}))
}

func (c *ClickHouse) Insert(ctx context.Context, tenantID uuid.UUID, evs []events.Event) error {
	if len(evs) == 0 {
		return nil
	}
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO "+c.table+
		" (tenant_id, id, zone_id, triggered_by, triggered_by_id, triggered_by_name, action, source_ip, request_path, occurred_at, ingested_at)")
	if err != nil {
		return err
	}
	for _, e := range evs {
		if err := batch.Append(tenantID, e.ID, e.ZoneID, e.TriggeredBy, e.TriggeredByID, e.TriggeredByName,
			e.Action, e.SourceIP, e.RequestPath, e.OccurredAt, e.IngestedAt); err != nil {
			_ = batch.Abort()
			return err
		}
	}
	return batch.Send()
}

func scanEvent(rows driver.Rows) (events.Event, error) {
	var e events.Event
	err := rows.Scan(&e.ID, &e.ZoneID, &e.TriggeredBy, &e.TriggeredByID, &e.TriggeredByName,
		&e.Action, &e.SourceIP, &e.RequestPath, &e.OccurredAt, &e.IngestedAt)
	e.OccurredAt, e.IngestedAt = e.OccurredAt.UTC(), e.IngestedAt.UTC()
	return e, err
}

func (c *ClickHouse) List(ctx context.Context, tenantID uuid.UUID, q events.ListQuery) (Page, error) {
	plan, err := events.PlanList(c.table, tenantID, q)
	if err != nil {
		return Page{}, err
	}
	rows, err := c.conn.Query(readCtx(ctx), plan.SQL, plan.Args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Records: []events.Event{}}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return Page{}, err
		}
		page.Records = append(page.Records, e)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Records) > plan.Limit {
		page.Records = page.Records[:plan.Limit]
		page.HasMore = true
		cursor := events.EncodeCursor(page.Records[plan.Limit-1])
		page.NextCursor = &cursor
	}
	return page, nil
}

func (c *ClickHouse) Get(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (events.Event, error) {
	stmt := events.PlanGet(c.table, tenantID, id)
	rows, err := c.conn.Query(readCtx(ctx), stmt.SQL, stmt.Args...)
	if err != nil {
		return events.Event{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return events.Event{}, err
		}
		return events.Event{}, ErrNotFound
	}
	return scanEvent(rows)
}

// Aggregate returns rows shaped like metap's aggregate output: {"bucket"?, "group"?, <metric>...},
// bucket as an RFC 3339 string, every metric a float.
func (c *ClickHouse) Aggregate(ctx context.Context, tenantID uuid.UUID, q events.AggregateQuery) ([]map[string]any, error) {
	plan, err := events.PlanAggregate(c.table, tenantID, q)
	if err != nil {
		return nil, err
	}
	rows, err := c.conn.Query(readCtx(ctx), plan.SQL, plan.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var bucket time.Time
		var group string
		metrics := make([]float64, len(plan.Metrics))
		var dest []any
		if plan.HasBucket {
			dest = append(dest, &bucket)
		}
		if plan.HasGroup {
			dest = append(dest, &group)
		}
		for i := range metrics {
			dest = append(dest, &metrics[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		if plan.HasBucket {
			row["bucket"] = bucket.UTC().Format(time.RFC3339)
		}
		if plan.HasGroup {
			row["group"] = group
		}
		for i, key := range plan.Metrics {
			row[key] = metrics[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
