//go:build integration

package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/kit/scope"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/waap/a-api/events"
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
	_, err = st.Up(ctx, 30)
	require.NoError(t, err)
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
		page, err := st.List(ctx, scope.ForTenant(tenantA.String()), q)
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
	page, err := st.List(ctx, scope.ForTenant(tenantA.String()), events.ListQuery{Filters: map[string]string{"zoneId": "z1", "action": "blocked"}, Range: events.TimeRange{Since: &since}})
	require.NoError(t, err)
	require.Len(t, page.Records, 3) // hours 2, 4, 5

	got, err := st.Get(ctx, scope.ForTenant(tenantA.String()), all[0].ID)
	require.NoError(t, err)
	require.Equal(t, all[0].ID, got.ID)
	_, err = st.Get(ctx, scope.ForTenant(tenantB.String()), all[0].ID)
	require.ErrorIs(t, err, ErrNotFound, "another tenant's id is not found, not forbidden")

	rows, err := st.Aggregate(ctx, scope.ForTenant(tenantA.String()), events.AggregateQuery{GroupBy: "action"})
	require.NoError(t, err)
	require.Equal(t, []map[string]any{
		{"group": "blocked", "count": 4.0},
		{"group": "logged", "count": 3.0},
		{"group": "challenged", "count": 1.0},
	}, rows)

	rows, err = st.Aggregate(ctx, scope.ForTenant(tenantA.String()), events.AggregateQuery{Bucket: "day", TimeField: "occurredAt", Filters: map[string]string{"zoneId": "z1"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, base.Truncate(24*time.Hour).Format(time.RFC3339), rows[0]["bucket"])
	require.Equal(t, 7.0, rows[0]["count"])

	rows, err = st.Aggregate(ctx, scope.ForTenant(tenantA.String()), events.AggregateQuery{Bucket: "week", Metrics: []string{"count", "count:triggeredByName"}})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Contains(t, rows[0], "count_triggeredByName")
}

// A scope limited to some zones sees those zones only, on every kind of read: the list, one event by
// id, and the aggregate; a scope limited to nothing sees nothing.
func TestAScopeLimitedToAZoneSeesOnlyThatZone(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	tenant := uuid.New()
	base := time.Now().UTC().Truncate(time.Hour).Add(-3 * time.Hour)
	z1a, z1b, z2, z3 := ev("z1", "blocked", "203.0.113.1", base), ev("z1", "logged", "203.0.113.2", base.Add(time.Minute)),
		ev("z2", "blocked", "198.51.100.9", base.Add(2*time.Minute)), ev("z3", "challenged", "192.0.2.7", base.Add(3*time.Minute))
	require.NoError(t, st.Insert(ctx, tenant, []events.Event{z1a, z1b, z2, z3}))
	whole := scope.ForTenant(tenant.String())

	page, err := st.List(ctx, whole.Limit("zone", "z1"), events.ListQuery{})
	require.NoError(t, err)
	require.Len(t, page.Records, 2, "only zone z1")
	for _, e := range page.Records {
		require.Equal(t, "z1", e.ZoneID)
	}

	page, err = st.List(ctx, whole.Limit("zone", "z1", "z3"), events.ListQuery{})
	require.NoError(t, err)
	require.Len(t, page.Records, 3, "z1 and z3, not z2")

	_, err = st.Get(ctx, whole.Limit("zone", "z1"), z1a.ID)
	require.NoError(t, err)
	_, err = st.Get(ctx, whole.Limit("zone", "z1"), z2.ID)
	require.ErrorIs(t, err, ErrNotFound, "an event of another zone is not found with this scope")

	rows, err := st.Aggregate(ctx, whole.Limit("zone", "z2"), events.AggregateQuery{GroupBy: "action"})
	require.NoError(t, err)
	require.Equal(t, []map[string]any{{"group": "blocked", "count": 1.0}}, rows)

	// A limit and a filter on the same column narrow together; they never widen each other.
	page, err = st.List(ctx, whole.Limit("zone", "z1"), events.ListQuery{Filters: map[string]string{"zoneId": "z2"}})
	require.NoError(t, err)
	require.Empty(t, page.Records, "asking for z2 inside a z1 scope finds nothing")

	// Nothing left to see: empty answers, no query.
	nothing := whole.Limit("zone")
	page, err = st.List(ctx, nothing, events.ListQuery{})
	require.NoError(t, err)
	require.Empty(t, page.Records)
	_, err = st.Get(ctx, nothing, z1a.ID)
	require.ErrorIs(t, err, ErrNotFound)
	rows, err = st.Aggregate(ctx, nothing, events.AggregateQuery{})
	require.NoError(t, err)
	require.Empty(t, rows)

	// The zero scope bounds nothing and is refused.
	_, err = st.List(ctx, scope.Scope{}, events.ListQuery{})
	require.Error(t, err)
}

func TestTTLIsSet(t *testing.T) {
	st := openTest(t)
	require.NoError(t, st.SetRetention(context.Background(), 7))
	var ddl string
	row := st.conn.QueryRow(context.Background(), "SHOW CREATE TABLE "+st.table)
	require.NoError(t, row.Scan(&ddl))
	require.Contains(t, ddl, "TTL toDateTime(occurred_at) + toIntervalDay(7)")
}

// The boot check only reads; the schema moves with up, retention and down.
func TestClickHouseSchemaIsChangedOnlyByMigrating(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	require.NoError(t, st.CheckSchema(ctx, 30))
	again, err := st.Up(ctx, 30)
	require.NoError(t, err)
	require.Empty(t, again)

	require.ErrorIs(t, st.CheckSchema(ctx, 7), ErrSchemaNotReady, "another retention is not applied by booting")
	days, err := st.RetentionDays(ctx)
	require.NoError(t, err)
	require.Equal(t, 30, days)
	require.NoError(t, st.SetRetention(ctx, 7))
	require.NoError(t, st.CheckSchema(ctx, 7))

	targets, err := st.LastApplied(ctx, 7, 1)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.NoError(t, st.Down(ctx, targets))
	require.ErrorIs(t, st.CheckSchema(ctx, 7), ErrSchemaNotReady)
	days, err = st.RetentionDays(ctx)
	require.NoError(t, err)
	require.Zero(t, days, "the table is gone and checking did not bring it back")
	applied, err := st.Up(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, []int{1}, applied)
	require.NoError(t, st.CheckSchema(ctx, 7))
}
