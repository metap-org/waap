package events

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/kit/scope"
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
	plan, err := PlanList("db.security_events", scoped, ListQuery{
		Filters: map[string]string{"zoneId": "z1", "action": "blocked' OR 1=1 --"},
		Range:   TimeRange{Since: &since}, Limit: 1000,
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plan.SQL, "SELECT "+SelectColumns+" FROM db.security_events WHERE tenant_id = ? AND action = ? AND zone_id = ?"))
	require.Contains(t, plan.SQL, "ORDER BY occurred_at DESC, id DESC LIMIT 201")
	require.NotContains(t, plan.SQL, "OR 1=1", "values never reach the SQL text")
	require.Equal(t, []any{tenant, "blocked' OR 1=1 --", "z1", since.UnixMilli()}, plan.Args)
	require.Equal(t, MaxListLimit, plan.Limit)

	_, err = PlanList("t", scoped, ListQuery{Filters: map[string]string{"tenant_id": "x"}})
	require.ErrorAs(t, err, new(*InvalidQueryError), "unknown/internal columns are refused")
}

func TestCursorRoundTrip(t *testing.T) {
	e := Event{ID: uuid.New(), OccurredAt: now}
	plan, err := PlanList("t", scoped, ListQuery{Cursor: EncodeCursor(e), Ascending: true})
	require.NoError(t, err)
	require.Contains(t, plan.SQL, "(occurred_at, id) > (fromUnixTimestamp64Milli(?, 'UTC'), toUUID(?))")
	require.Contains(t, plan.SQL, "ORDER BY occurred_at ASC")
	require.Equal(t, []any{tenant, now.UnixMilli(), e.ID.String()}, plan.Args)

	_, err = PlanList("t", scoped, ListQuery{Cursor: "garbage!"})
	require.ErrorAs(t, err, new(*InvalidQueryError))
}

func TestPlanAggregate(t *testing.T) {
	plan, err := PlanAggregate("t", scoped, AggregateQuery{
		Bucket: "day", TimeField: "occurredAt", Since: "2026-11-01T00:00:00Z", Filters: map[string]string{"zoneId": "z1"},
	})
	require.NoError(t, err)
	require.Equal(t, "SELECT toDateTime(toStartOfDay(occurred_at), 'UTC') AS bucket, toFloat64(count()) AS m0 FROM t "+
		"WHERE tenant_id = ? AND zone_id = ? AND occurred_at >= fromUnixTimestamp64Milli(?, 'UTC') GROUP BY bucket ORDER BY bucket ASC LIMIT 100", plan.SQL)
	require.Equal(t, []string{"count"}, plan.Metrics)

	plan, err = PlanAggregate("t", scoped, AggregateQuery{GroupBy: "sourceIp", Metrics: []string{"count", "count:triggeredByName"}, Limit: 10})
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
		_, err := PlanAggregate("t", scoped, bad)
		require.ErrorAs(t, err, new(*InvalidQueryError), "%+v", bad)
	}
}

var scoped = scope.ForTenant(tenant.String())

func TestEveryReadIsBoundedByItsScope(t *testing.T) {
	// The tenant is always in the statement, first, whatever else is asked.
	stmt, err := PlanGet("t", scoped, uuid.MustParse("00000000-0000-0000-0000-000000000009"))
	require.NoError(t, err)
	require.Equal(t, "SELECT "+SelectColumns+" FROM t WHERE tenant_id = ? AND id = toUUID(?) LIMIT 1", stmt.SQL)
	require.Equal(t, []any{tenant, "00000000-0000-0000-0000-000000000009"}, stmt.Args)

	// The zero scope bounds nothing, so every read refuses it.
	_, err = PlanList("t", scope.Scope{}, ListQuery{})
	require.ErrorAs(t, err, new(*InvalidQueryError))
	_, err = PlanAggregate("t", scope.Scope{}, AggregateQuery{})
	require.ErrorAs(t, err, new(*InvalidQueryError))
	_, err = PlanGet("t", scope.Scope{}, uuid.New())
	require.ErrorAs(t, err, new(*InvalidQueryError))
}

func TestAZoneLimitAddsAConditionToEveryKindOfRead(t *testing.T) {
	limited := scoped.Limit("zone", "z1", "z2")

	list, err := PlanList("t", limited, ListQuery{Filters: map[string]string{"action": "blocked"}})
	require.NoError(t, err)
	require.Contains(t, list.SQL, "WHERE tenant_id = ? AND zone_id IN (?, ?) AND action = ?")
	require.Equal(t, []any{tenant, "z1", "z2", "blocked"}, list.Args)
	require.False(t, list.Empty)

	agg, err := PlanAggregate("t", limited, AggregateQuery{GroupBy: "action"})
	require.NoError(t, err)
	require.Contains(t, agg.SQL, "WHERE tenant_id = ? AND zone_id IN (?, ?)")

	get, err := PlanGet("t", limited, uuid.New())
	require.NoError(t, err)
	require.Contains(t, get.SQL, "tenant_id = ? AND zone_id IN (?, ?) AND id = toUUID(?)")
}

func TestAZoneLimitIsNotAnInjectionPoint(t *testing.T) {
	list, err := PlanList("t", scoped.Limit("zone", "z1' OR '1'='1"), ListQuery{})
	require.NoError(t, err)
	require.NotContains(t, list.SQL, "OR '1'='1")
	require.Contains(t, list.Args, "z1' OR '1'='1")
}

func TestAScopeWithNothingLeftToSeeNeverBecomesAQuery(t *testing.T) {
	none := scoped.Limit("zone")
	list, err := PlanList("t", none, ListQuery{})
	require.NoError(t, err)
	require.True(t, list.Empty, "an empty limit is closed: the answer is empty without asking the store")
	agg, err := PlanAggregate("t", scoped.Limit("zone", "z1").Limit("zone", "z2"), AggregateQuery{})
	require.NoError(t, err)
	require.True(t, agg.Empty)
	get, err := PlanGet("t", none, uuid.New())
	require.NoError(t, err)
	require.True(t, get.Empty)
}

func TestALimitOnADimensionTheStoreCannotFilterIsRefused(t *testing.T) {
	_, err := PlanList("t", scoped.Limit("account", "a1"), ListQuery{})
	require.ErrorAs(t, err, new(*InvalidQueryError), "a limit that cannot be applied is a hole, not something to skip")
}
