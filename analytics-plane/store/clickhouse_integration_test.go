//go:build integration

package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/waap/analytics-plane/events"
)

// Runs against a real ClickHouse: CLICKHOUSE_URL=clickhouse://default:@127.0.0.1:9000 (e.g.
// `docker compose up -d clickhouse` in management-plane). Fails, never skips, when unset. Each
// test gets its own database, dropped afterwards.
func openTest(t *testing.T) *ClickHouse {
	t.Helper()
	dsn := os.Getenv("CLICKHOUSE_URL")
	if dsn == "" {
		t.Fatal("CLICKHOUSE_URL is not set (e.g. clickhouse://default:@127.0.0.1:9000)")
	}
	ctx := context.Background()
	db := "it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	st, err := Open(ctx, dsn, db)
	require.NoError(t, err)
	require.NoError(t, st.Migrate(ctx, 30))
	require.NoError(t, st.Migrate(ctx, 7), "migrate is idempotent and re-asserts the TTL")
	t.Cleanup(func() {
		_ = st.conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db)
		_ = st.Close()
	})
	return st
}

func ev(zone, action, ip string, at time.Time) events.Event {
	return events.Event{ID: uuid.New(), ZoneID: zone, TriggeredBy: "firewallRule", TriggeredByID: "r1",
		TriggeredByName: "Rule 1", Action: action, SourceIP: ip, RequestPath: "/p", OccurredAt: at, IngestedAt: at}
}

func TestClickHouseRoundTrip(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	base := time.Now().UTC().Truncate(time.Hour).Add(-48 * time.Hour)

	var all []events.Event
	for i := 0; i < 7; i++ {
		action := "blocked"
		if i%3 == 0 {
			action = "logged"
		}
		all = append(all, ev("z1", action, "203.0.113.1", base.Add(time.Duration(i)*time.Hour+123*time.Millisecond)))
	}
	all = append(all, ev("z2", "challenged", "198.51.100.9", base.Add(26*time.Hour)))
	require.NoError(t, st.Insert(ctx, tenantA, all))
	require.NoError(t, st.Insert(ctx, tenantB, []events.Event{ev("z1", "blocked", "192.0.2.1", base)}))

	// Pagination, newest first, keyset cursor — 8 events of tenant A in pages of 3.
	var seen []events.Event
	q := events.ListQuery{Limit: 3}
	for pages := 0; ; pages++ {
		require.Less(t, pages, 5)
		page, err := st.List(ctx, tenantA, q)
		require.NoError(t, err)
		seen = append(seen, page.Records...)
		if !page.HasMore {
			break
		}
		q.Cursor = *page.NextCursor
	}
	require.Len(t, seen, 8, "tenant B's event never shows up")
	for i := 1; i < len(seen); i++ {
		require.False(t, seen[i].OccurredAt.After(seen[i-1].OccurredAt), "newest first")
	}
	require.Equal(t, 123*time.Millisecond, time.Duration(seen[len(seen)-1].OccurredAt.Nanosecond()), "millisecond precision survives")

	since := base.Add(2 * time.Hour)
	page, err := st.List(ctx, tenantA, events.ListQuery{Filters: map[string]string{"zoneId": "z1", "action": "blocked"}, Range: events.TimeRange{Since: &since}})
	require.NoError(t, err)
	require.Len(t, page.Records, 3) // hours 2, 4, 5

	got, err := st.Get(ctx, tenantA, all[0].ID)
	require.NoError(t, err)
	require.Equal(t, all[0].ID, got.ID)
	_, err = st.Get(ctx, tenantB, all[0].ID)
	require.ErrorIs(t, err, ErrNotFound, "another tenant's id is not found, not forbidden")

	rows, err := st.Aggregate(ctx, tenantA, events.AggregateQuery{GroupBy: "action"})
	require.NoError(t, err)
	require.Equal(t, []map[string]any{
		{"group": "blocked", "count": 4.0},
		{"group": "logged", "count": 3.0},
		{"group": "challenged", "count": 1.0},
	}, rows)

	rows, err = st.Aggregate(ctx, tenantA, events.AggregateQuery{Bucket: "day", TimeField: "occurredAt", Filters: map[string]string{"zoneId": "z1"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, base.Truncate(24*time.Hour).Format(time.RFC3339), rows[0]["bucket"])
	require.Equal(t, 7.0, rows[0]["count"])

	rows, err = st.Aggregate(ctx, tenantA, events.AggregateQuery{Bucket: "week", Metrics: []string{"count", "count:triggeredByName"}})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Contains(t, rows[0], "count_triggeredByName")
}

func TestTTLIsSet(t *testing.T) {
	st := openTest(t)
	var ddl string
	row := st.conn.QueryRow(context.Background(), "SHOW CREATE TABLE "+st.table)
	require.NoError(t, row.Scan(&ddl))
	require.Contains(t, ddl, "TTL toDateTime(occurred_at) + toIntervalDay(7)")
}
