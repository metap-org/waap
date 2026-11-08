package events

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	tenant = uuid.MustParse("2e87cf98-46a7-473f-bebc-20307e17d3b3")
	now    = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
)

func validIncoming() Incoming {
	name := "Block bad bots"
	return Incoming{
		ZoneID: "zone-1", TriggeredBy: "firewallRule", TriggeredByID: "rule-1", TriggeredByName: &name,
		Action: "blocked", SourceIP: "::ffff:203.0.113.7", RequestPath: "/login", OccurredAt: "2026-11-08T11:59:00.123Z",
	}
}

func TestValidate(t *testing.T) {
	e, err := validIncoming().Validate(now)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.7", e.SourceIP, "IPv4-mapped addresses are stored as plain IPv4")
	require.Equal(t, 123*time.Millisecond, time.Duration(e.OccurredAt.Nanosecond()))
	require.NotEqual(t, uuid.Nil, e.ID)
	require.Equal(t, now, e.IngestedAt)

	bad := validIncoming()
	bad.Action = "allowed"
	bad.SourceIP = "not-an-ip"
	bad.OccurredAt = "2026-11-08T13:00:00Z"
	bad.ZoneID = ""
	_, err = bad.Validate(now)
	require.Error(t, err)
	for _, want := range []string{"zoneId", "action", "sourceIp", "future"} {
		require.Contains(t, err.Error(), want)
	}
}

func TestPlanListIsTenantScopedAndParameterized(t *testing.T) {
	since := now.Add(-time.Hour)
	plan, err := PlanList("db.security_events", tenant, ListQuery{
		Filters: map[string]string{"zoneId": "z1", "action": "blocked' OR 1=1 --"},
		Range:   TimeRange{Since: &since}, Limit: 1000,
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plan.SQL, "SELECT "+SelectColumns+" FROM db.security_events WHERE tenant_id = ? AND action = ? AND zone_id = ?"))
	require.Contains(t, plan.SQL, "ORDER BY occurred_at DESC, id DESC LIMIT 201")
	require.NotContains(t, plan.SQL, "OR 1=1", "values never reach the SQL text")
	require.Equal(t, []any{tenant, "blocked' OR 1=1 --", "z1", since.UnixMilli()}, plan.Args)
	require.Equal(t, MaxListLimit, plan.Limit)

	_, err = PlanList("t", tenant, ListQuery{Filters: map[string]string{"tenant_id": "x"}})
	require.ErrorAs(t, err, new(*InvalidQueryError), "unknown/internal columns are refused")
}

func TestCursorRoundTrip(t *testing.T) {
	e := Event{ID: uuid.New(), OccurredAt: now}
	plan, err := PlanList("t", tenant, ListQuery{Cursor: EncodeCursor(e), Ascending: true})
	require.NoError(t, err)
	require.Contains(t, plan.SQL, "(occurred_at, id) > (fromUnixTimestamp64Milli(?, 'UTC'), toUUID(?))")
	require.Contains(t, plan.SQL, "ORDER BY occurred_at ASC")
	require.Equal(t, []any{tenant, now.UnixMilli(), e.ID.String()}, plan.Args)

	_, err = PlanList("t", tenant, ListQuery{Cursor: "garbage!"})
	require.ErrorAs(t, err, new(*InvalidQueryError))
}

func TestPlanAggregate(t *testing.T) {
	plan, err := PlanAggregate("t", tenant, AggregateQuery{
		Bucket: "day", TimeField: "occurredAt", Since: "2026-11-01T00:00:00Z", Filters: map[string]string{"zoneId": "z1"},
	})
	require.NoError(t, err)
	require.Equal(t, "SELECT toDateTime(toStartOfDay(occurred_at), 'UTC') AS bucket, toFloat64(count()) AS m0 FROM t "+
		"WHERE tenant_id = ? AND zone_id = ? AND occurred_at >= fromUnixTimestamp64Milli(?, 'UTC') GROUP BY bucket ORDER BY bucket ASC LIMIT 100", plan.SQL)
	require.Equal(t, []string{"count"}, plan.Metrics)

	plan, err = PlanAggregate("t", tenant, AggregateQuery{GroupBy: "sourceIp", Metrics: []string{"count", "count:triggeredByName"}, Limit: 10})
	require.NoError(t, err)
	require.Contains(t, plan.SQL, "source_ip AS grp, toFloat64(count()) AS m0, toFloat64(countIf(triggered_by_name != '')) AS m1")
	require.Contains(t, plan.SQL, "GROUP BY grp ORDER BY m0 DESC LIMIT 10")
	require.Equal(t, []string{"count", "count_triggeredByName"}, plan.Metrics)

	for _, bad := range []AggregateQuery{
		{Bucket: "minute"},
		{GroupBy: "occurredAt"},
		{Metrics: []string{"sum:requestPath"}},
		{TimeField: "ingestedAt"},
		{Since: "yesterday"},
		{Filters: map[string]string{"id": "x"}},
	} {
		_, err := PlanAggregate("t", tenant, bad)
		require.ErrorAs(t, err, new(*InvalidQueryError), "%+v", bad)
	}
}
