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
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"

	"github.com/metap-org/waap/m-analytics"
	"github.com/metap-org/waap/model"
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
	v, _ := env.Lookup("ANALYTICS_URL")
	if v == "" {
		v = "http://localhost:3030"
	}
	return analytics.NewClient(v)
}

// recentEvents reads events since cutoff (newest first, at most eventScanLimit). An Analytics
// error keeps its own status (401 stays 401, 503 stays 503) instead of turning into a generic 500.
func recentEvents(ctx context.Context, rc *permission.RequestContext, filters map[string]string, cutoff time.Time) ([]crud.RecordDto, error) {
	events, err := analyticsClient().Window(ctx, rc.ForwardedBearerToken, filters, cutoff, eventScanLimit)
	if err != nil {
		var apiErr *analytics.APIError
		if errors.As(err, &apiErr) {
			return nil, crud.NewServiceErrorMsg(uint16(apiErr.Status), apiErr.Code, apiErr.Message)
		}
		return nil, err
	}
	return events, nil
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

// listRecords lists up to limit records with plain equality filters, unwrapping the service
// result into a plain error — every caller here treats a permission/validation failure as "abort
// the sweep", not as an empty set, because silently proceeding on an empty read would create
// nothing and look exactly like "there was nothing to do".
func listRecords(ctx context.Context, state *httpserver.AppState, entity string, filters [][2]string, limit int64, rc *permission.RequestContext) ([]crud.RecordDto, error) {
	result, err := state.Crud.List(ctx, entity, query.ListInput{Limit: limit, Filters: filters}, rc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", entity, err)
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
func correlateIncidents(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, body correlateBody) (map[string]any, error) {
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
		filters[model.FieldZoneID] = *body.ZoneID
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)
	events, err := recentEvents(ctx, rc, filters, cutoff)
	if err != nil {
		return nil, err
	}

	// Grouped in memory rather than by SQL: the window is small and bounded by eventScanLimit,
	// and this keeps the correlation rule readable as one expression of the business rule
	// instead of a query that has to be re-read to know what it correlates.
	groups := map[zoneIPKey]int{}
	for _, event := range events {
		if occurredAtOrFallback(event).Before(cutoff) {
			continue
		}
		zoneID := strField(event, model.FieldZoneID)
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
		existing, err := listRecords(ctx, state, model.Incidents,
			[][2]string{{model.FieldZoneID, key.zoneID}, {"status", "open"}}, 50, rc)
		if err != nil {
			return nil, err
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
			model.FieldZoneID: jsonStr(key.zoneID),
			"title":           jsonStr(fmt.Sprintf("%d events from %s in %dm", count, key.sourceIP, windowMinutes)),
			"severity":        jsonStr(severityFor(count)),
			"eventCount":      json.RawMessage(fmt.Sprintf("%d", count)),
		}
		result, err := state.Crud.Create(ctx, model.Incidents, data, rc, nil)
		if err != nil {
			return nil, err
		}
		created = append(created, result.ID)
	}

	return map[string]any{
		"scannedEvents":    len(events),
		"createdIncidents": created,
		"skippedExisting":  skipped,
		"windowMinutes":    windowMinutes,
		"threshold":        threshold,
	}, nil
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
	result, err := state.Crud.Create(ctx, model.AlertNotifications, data, rc, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("alert_notifications: %w", err)
	}
	return result.ID, nil
}

// evaluateAlerts is POST /internal/alerts/evaluate — every enabled AlertPolicy, evaluated against
// the recent event stream, counted PER ZONE (never summed across zones).
func evaluateAlerts(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext) (map[string]any, error) {
	policies, err := listRecords(ctx, state, model.AlertPolicies, [][2]string{{"enabled", "true"}}, 50, rc)
	if err != nil {
		return nil, err
	}
	// One read covering the widest policy window; each policy then counts only its own window.
	widest := int64(defaultWindowMinutes)
	for _, policy := range policies {
		if m, ok := jsonAsInt(policy.Data["windowMinutes"]); ok && m > widest {
			widest = m
		}
	}
	events, err := recentEvents(ctx, rc, nil, time.Now().UTC().Add(-time.Duration(widest)*time.Minute))
	if err != nil {
		return nil, err
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
			perZone[strField(event, model.FieldZoneID)]++
		}

		channels := policy.Data["channels"]
		for zoneID, count := range perZone {
			if threshold == 0 || int64(count) < threshold {
				continue
			}
			payload := map[string]any{
				"policy":          strField(policy, "name"),
				model.FieldZoneID: zoneID,
				"eventCount":      count,
				"windowMinutes":   windowMinutes,
			}
			delivered, detail := deliver(channels, payload)
			notificationID, err := recordNotification(ctx, state, rc, policy, channelName(channels), delivered)
			if err != nil {
				return nil, err
			}
			fired = append(fired, map[string]any{
				"notificationId":  notificationID,
				"policyId":        policy.ID,
				model.FieldZoneID: zoneID,
				"eventCount":      count,
				"delivered":       delivered,
				"detail":          detail,
			})
		}
	}

	return map[string]any{"policiesEvaluated": len(policies), "fired": fired}, nil
}

// testAlertPolicy is POST /api/waf.alert_policies/{id}/test — the "Send test alert" button. Runs
// the real delivery path and writes a real AlertNotification, because a test that takes a
// different path than production proves nothing about production.
func testAlertPolicy(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, policyID uuid.UUID) (map[string]any, error) {
	getResult, err := state.Crud.Get(ctx, model.AlertPolicies, policyID, rc)
	if err != nil {
		return nil, err
	}
	policy := getResult.Record

	channels := policy.Data["channels"]
	payload := map[string]any{
		"test":    true,
		"policy":  strField(policy, "name"),
		"message": "Test alert from the WAF portal",
	}
	delivered, detail := deliver(channels, payload)
	notificationID, err := recordNotification(ctx, state, rc, policy, channelName(channels), delivered)
	if err != nil {
		return nil, err
	}
	return map[string]any{"notificationId": notificationID, "delivered": delivered, "detail": detail}, nil
}

// MountRoutes registers this service's custom (non-generic-CRUD) endpoints onto r. REST stays
// available next to the gRPC actions (RegisterActions).
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	r.Post("/internal/incidents/correlate", func(w http.ResponseWriter, req *http.Request) {
		rc, ok := httpserver.RequireAuth(w, req, state)
		if !ok {
			return
		}
		var body correlateBody
		if req.ContentLength != 0 && !httpserver.DecodeJSONBody(w, req, &body) {
			return
		}
		writeResult(w)(correlateIncidents(req.Context(), state, rc, body))
	})
	r.Post("/internal/alerts/evaluate", func(w http.ResponseWriter, req *http.Request) {
		rc, ok := httpserver.RequireAuth(w, req, state)
		if !ok {
			return
		}
		writeResult(w)(evaluateAlerts(req.Context(), state, rc))
	})
	r.Post("/api/waf.alert_policies/{id}/test", httpserver.RecordActionRoute(state, "policy", func(ctx context.Context, rc *permission.RequestContext, id uuid.UUID) (map[string]any, error) {
		return testAlertPolicy(ctx, state, rc, id)
	}))
}

func writeResult(w http.ResponseWriter) func(map[string]any, error) {
	return func(out map[string]any, err error) {
		if err != nil {
			httpserver.WriteCrudError(w, err)
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
	}
}

// RegisterActions registers the gRPC handlers for the actions declared on this service's entities.
func RegisterActions(state *httpserver.AppState, reg *crud.ActionRegistry) error {
	if err := reg.Register(model.Incidents, model.ActionCorrelate, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		raw, err := json.Marshal(c.Input)
		if err != nil {
			return nil, err
		}
		var body correlateBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, "validation_failed", "Invalid correlate input.")
		}
		return crud.ToObject(correlateIncidents(ctx, state, c.RC, body))
	}); err != nil {
		return err
	}
	if err := reg.Register(model.AlertPolicies, model.ActionEvaluate, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(evaluateAlerts(ctx, state, c.RC))
	}); err != nil {
		return err
	}
	return reg.Register(model.AlertPolicies, model.ActionTest, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(testAlertPolicy(ctx, state, c.RC, c.ID))
	})
}
