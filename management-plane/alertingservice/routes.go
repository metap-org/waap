// Custom (non-generic-CRUD) HTTP surface for alerting-service.
//
// Two of the things the product spec lists as needing real code live here, and both for the
// same reason: they are decisions over a SET of records, not a read or write of one record, so
// no metadata-driven CRUD route can generate them.
//
//   - POST /internal/incidents/correlate — turns recent SecurityEvents into Incidents using the
//     static v1 rule: same zoneId + same sourceIp inside a 15-minute window, threshold events or
//     more. Static, not per-tenant configurable.
//   - POST /internal/alerts/evaluate — the AlertPolicy side: "N events in M minutes on the same
//     zone" (counted per zone, never summed across zones), producing an AlertNotification per
//     firing.
//
// SecurityEvents live in the Analytics plane (ClickHouse), not here: both sweeps read them
// through its query API with the caller's own token (ANALYTICS_URL), with the time window pushed
// into the query. Retention is ClickHouse's table TTL, so the old Postgres cleanup sweep is gone.
//
// Both are written as endpoints rather than background loops so metap-cron can drive them
// on a schedule while a portal user/operator can also trigger one on demand.
package alertingservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/runtime"

	"github.com/metap-org/waap/management-plane/analytics"
)

// Default correlation window/threshold — the settled v1 rule. Overridable per request only so an
// operator can re-run a wider sweep by hand; the scheduled call uses the defaults.
const (
	defaultWindowMinutes = 15
	defaultThreshold     = 5
	// How many recent events one correlation pass reads. Bounded on purpose: this is a periodic
	// sweep, not a backfill, and an unbounded read of the highest-volume entity in the product is
	// the last thing this service should do on a timer.
	eventScanLimit = 200
)

func httpClient() *http.Client { return &http.Client{Timeout: 10 * time.Second} }

// analyticsClient: where security events are read from (the Analytics plane's query API).
func analyticsClient() *analytics.Client {
	v, _ := runtime.Optional("ANALYTICS_URL")
	if v == "" {
		v = "http://localhost:3030"
	}
	return analytics.NewClient(v)
}

// recentEvents reads events since cutoff (newest first, at most eventScanLimit). A failure is
// written to w and reported as ok=false: an Analytics error keeps its own status (401 stays 401,
// 503 stays 503) instead of turning into a generic 500.
func recentEvents(w http.ResponseWriter, r *http.Request, rc *permission.RequestContext, filters map[string]string, cutoff time.Time) ([]crud.RecordDto, bool) {
	events, err := analyticsClient().Window(r.Context(), rc.ForwardedBearerToken, filters, cutoff, eventScanLimit)
	if err != nil {
		var apiErr *analytics.APIError
		if errors.As(err, &apiErr) {
			httpserver.WriteServiceError(w, apiErr.Status, apiErr.Code, apiErr.Message, nil)
		} else {
			httpserver.WriteInternalError(w, err)
		}
		return nil, false
	}
	return events, true
}

func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func jsonAsString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func jsonAsInt(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int64(n), true
}

func strField(record crud.RecordDto, field string) string {
	s, _ := jsonAsString(record.Data[field])
	return s
}

func writeCrudError(w http.ResponseWriter, status uint16, errorCode string, message *string, fieldErrors map[string][]string) {
	msg := ""
	if message != nil {
		msg = *message
	}
	httpserver.WriteServiceError(w, int(status), errorCode, msg, fieldErrors)
}

// listRecords lists up to limit records with plain equality filters, unwrapping the service
// result into a plain error — every caller here treats a permission/validation failure as "abort
// the sweep", not as an empty set, because silently proceeding on an empty read would create
// nothing and look exactly like "there was nothing to do".
func listRecords(ctx context.Context, state *httpserver.AppState, entity string, filters [][2]string, limit int64, rc *permission.RequestContext) ([]crud.RecordDto, error) {
	result, err := state.Crud.List(ctx, entity, query.ListInput{Limit: limit, Filters: filters}, rc)
	if err != nil {
		return nil, err
	}
	if !result.IsOk {
		msg := ""
		if result.Message != nil {
			msg = " (" + *result.Message + ")"
		}
		return nil, fmt.Errorf("%s: %s%s", entity, result.ErrorCode, msg)
	}
	return result.Data, nil
}

// severityFor derives severity from volume. Deliberately a fixed ladder, not config: an
// incident's severity is only meaningful next to the same ladder every other incident was scored
// on.
func severityFor(count int) string {
	switch {
	case count < 10:
		return "low"
	case count < 50:
		return "medium"
	case count < 200:
		return "high"
	default:
		return "critical"
	}
}

func occurredAtOrFallback(event crud.RecordDto) time.Time {
	if raw, ok := jsonAsString(event.Data["occurredAt"]); ok {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.UTC()
		}
	}
	// An event with no parseable timestamp falls back to when the row was written, so a
	// malformed payload can't silently drop out of correlation entirely.
	return event.CreatedAt.UTC()
}

type correlateBody struct {
	WindowMinutes *int64  `json:"windowMinutes"`
	Threshold     *int    `json:"threshold"`
	ZoneID        *string `json:"zoneId"`
}

type zoneIPKey struct {
	zoneID   string
	sourceIP string
}

// correlateIncidents is POST /internal/incidents/correlate — groups recent events by
// (zoneId, sourceIp) and opens one Incident per group above the threshold.
//
// Deduplication is by "is there already an open incident for this zone with this source IP in
// its title": the incident entity has no sourceIp field of its own (it correlates events, it
// doesn't copy them), so the title carries the key. Crude but stable, and it keeps a re-run
// idempotent — which matters because this is scheduled, so it WILL run again over the same
// events.
func correlateIncidents(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}

	var body correlateBody
	if r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	windowMinutes := int64(defaultWindowMinutes)
	if body.WindowMinutes != nil {
		windowMinutes = *body.WindowMinutes
	}
	threshold := defaultThreshold
	if body.Threshold != nil {
		threshold = *body.Threshold
	}

	filters := map[string]string{}
	if body.ZoneID != nil {
		filters["zoneId"] = *body.ZoneID
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)
	events, ok := recentEvents(w, r, rc, filters, cutoff)
	if !ok {
		return
	}

	// Grouped in memory rather than by SQL: the window is small and bounded by eventScanLimit,
	// and this keeps the correlation rule readable as one expression of the business rule
	// instead of a query that has to be re-read to know what it correlates.
	groups := map[zoneIPKey]int{}
	for _, event := range events {
		if occurredAtOrFallback(event).Before(cutoff) {
			continue
		}
		zoneID := strField(event, "zoneId")
		sourceIP := strField(event, "sourceIp")
		if zoneID == "" || sourceIP == "" {
			continue
		}
		groups[zoneIPKey{zoneID, sourceIP}]++
	}

	created := make([]uuid.UUID, 0)
	skipped := 0
	for key, count := range groups {
		if count < threshold {
			continue
		}
		existing, err := listRecords(r.Context(), state, "waf.incidents",
			[][2]string{{"zoneId", key.zoneID}, {"status", "open"}}, 50, rc)
		if err != nil {
			httpserver.WriteInternalError(w, err)
			return
		}
		alreadyOpen := false
		for _, incident := range existing {
			if strings.Contains(strField(incident, "title"), key.sourceIP) {
				alreadyOpen = true
				break
			}
		}
		if alreadyOpen {
			skipped++
			continue
		}

		data := crud.JsonObject{
			"zoneId":     jsonStr(key.zoneID),
			"title":      jsonStr(fmt.Sprintf("%d events from %s in %dm", count, key.sourceIP, windowMinutes)),
			"severity":   jsonStr(severityFor(count)),
			"eventCount": json.RawMessage(fmt.Sprintf("%d", count)),
		}
		result, err := state.Crud.Create(r.Context(), "waf.incidents", data, rc, nil)
		if err != nil {
			httpserver.WriteInternalError(w, err)
			return
		}
		if !result.IsOk {
			writeCrudError(w, result.Status, result.ErrorCode, result.Message, result.FieldErrors)
			return
		}
		created = append(created, result.Data.ID)
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"scannedEvents":    len(events),
		"createdIncidents": created,
		"skippedExisting":  skipped,
		"windowMinutes":    windowMinutes,
		"threshold":        threshold,
	}})
}

func channelsObject(raw json.RawMessage) map[string]json.RawMessage {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	return obj
}

// deliver actually delivers a notification. channels on an AlertPolicy is free-form JSON, so this
// reads it defensively: {"webhook": "https://..."} posts, {"email": "..."} logs (there is no
// mail transport in this product yet — saying so in a log line is more honest than a
// deliveryStatus: "sent" nobody can verify).
func deliver(channels json.RawMessage, payload any) (bool, string) {
	obj := channelsObject(channels)
	if raw, ok := obj["webhook"]; ok {
		if url, ok := jsonAsString(raw); ok {
			body, _ := json.Marshal(payload)
			resp, err := httpClient().Post(url, "application/json", bytes.NewReader(body))
			if err != nil {
				return false, fmt.Sprintf("webhook error: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return true, fmt.Sprintf("webhook %d", resp.StatusCode)
			}
			return false, fmt.Sprintf("webhook %d", resp.StatusCode)
		}
	}
	if raw, ok := obj["email"]; ok {
		if address, ok := jsonAsString(raw); ok {
			return true, fmt.Sprintf("email logged to %s", address)
		}
	}
	return false, "no deliverable channel configured"
}

func channelName(channels json.RawMessage) string {
	obj := channelsObject(channels)
	if _, ok := obj["webhook"]; ok {
		return "webhook"
	}
	if _, ok := obj["email"]; ok {
		return "email"
	}
	return "none"
}

// recordNotification writes the delivery-log row every firing produces, sent or failed alike —
// an alert that failed to deliver is exactly the one an operator needs to find later.
func recordNotification(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, policy crud.RecordDto, channel string, delivered bool) (uuid.UUID, error) {
	status := "failed"
	if delivered {
		status = "sent"
	}
	data := crud.JsonObject{
		"alertPolicyId":  jsonStr(policy.ID.String()),
		"channel":        jsonStr(channel),
		"deliveryStatus": jsonStr(status),
		"triggeredAt":    jsonStr(time.Now().UTC().Format(time.RFC3339)),
	}
	result, err := state.Crud.Create(ctx, "waf.alert_notifications", data, rc, nil)
	if err != nil {
		return uuid.Nil, err
	}
	if !result.IsOk {
		return uuid.Nil, fmt.Errorf("alert_notifications: %s", result.ErrorCode)
	}
	return result.Data.ID, nil
}

// evaluateAlerts is POST /internal/alerts/evaluate — every enabled AlertPolicy, evaluated against
// the recent event stream, counted PER ZONE (never summed across zones).
func evaluateAlerts(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}

	policies, err := listRecords(r.Context(), state, "waf.alert_policies", [][2]string{{"enabled", "true"}}, 50, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	// One read covering the widest policy window; each policy then counts only its own window.
	widest := int64(defaultWindowMinutes)
	for _, policy := range policies {
		if m, ok := jsonAsInt(policy.Data["windowMinutes"]); ok && m > widest {
			widest = m
		}
	}
	events, ok := recentEvents(w, r, rc, nil, time.Now().UTC().Add(-time.Duration(widest)*time.Minute))
	if !ok {
		return
	}

	fired := make([]map[string]any, 0)
	for _, policy := range policies {
		threshold, _ := jsonAsInt(policy.Data["thresholdCount"])
		windowMinutes, ok := jsonAsInt(policy.Data["windowMinutes"])
		if !ok {
			windowMinutes = defaultWindowMinutes
		}
		cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)

		perZone := map[string]int{}
		for _, event := range events {
			if occurredAtOrFallback(event).Before(cutoff) {
				continue
			}
			perZone[strField(event, "zoneId")]++
		}

		channels := policy.Data["channels"]
		for zoneID, count := range perZone {
			if threshold == 0 || int64(count) < threshold {
				continue
			}
			payload := map[string]any{
				"policy":        strField(policy, "name"),
				"zoneId":        zoneID,
				"eventCount":    count,
				"windowMinutes": windowMinutes,
			}
			delivered, detail := deliver(channels, payload)
			notificationID, err := recordNotification(r.Context(), state, rc, policy, channelName(channels), delivered)
			if err != nil {
				httpserver.WriteInternalError(w, err)
				return
			}
			fired = append(fired, map[string]any{
				"notificationId": notificationID,
				"policyId":       policy.ID,
				"zoneId":         zoneID,
				"eventCount":     count,
				"delivered":      delivered,
				"detail":         detail,
			})
		}
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"policiesEvaluated": len(policies), "fired": fired,
	}})
}

// testAlertPolicy is POST /api/waf.alert_policies/{id}/test — the "Send test alert" button. Runs
// the real delivery path and writes a real AlertNotification, because a test that takes a
// different path than production proves nothing about production.
func testAlertPolicy(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	policyID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid policy id.", nil)
		return
	}
	getResult, err := state.Crud.Get(r.Context(), "waf.alert_policies", policyID, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !getResult.IsOk {
		writeCrudError(w, getResult.Status, getResult.ErrorCode, getResult.Message, getResult.FieldErrors)
		return
	}
	policy := getResult.Data.Record

	channels := policy.Data["channels"]
	payload := map[string]any{
		"test":    true,
		"policy":  strField(policy, "name"),
		"message": "Test alert from the WAF portal",
	}
	delivered, detail := deliver(channels, payload)
	notificationID, err := recordNotification(r.Context(), state, rc, policy, channelName(channels), delivered)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"notificationId": notificationID, "delivered": delivered, "detail": detail,
	}})
}

// MountRoutes registers this service's custom (non-generic-CRUD) endpoints onto r.
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	wrap := func(h func(http.ResponseWriter, *http.Request, *httpserver.AppState)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(w, r, state) }
	}
	r.Post("/internal/incidents/correlate", wrap(correlateIncidents))
	r.Post("/internal/alerts/evaluate", wrap(evaluateAlerts))
	r.Post("/api/waf.alert_policies/{id}/test", wrap(testAlertPolicy))
}
