// Layer: APP · declared-action logic. Alert policies over the event stream (actions evaluate, test).

package alerting

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/model"
)

// evaluateAlerts is POST /internal/alerts/evaluate — every enabled AlertPolicy, evaluated against
// the recent event stream, counted PER ZONE (never summed across zones).
func evaluateAlerts(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext) (map[string]any, error) {
	policies, err := listRecords(ctx, state, model.AlertPolicies, [][2]string{{model.FieldEnabled, "true"}}, recordScanLimit, rc)
	if err != nil {
		return nil, err
	}
	// One read covering the widest policy window; each policy then counts only its own window.
	widest := int64(defaultWindowMinutes)
	for _, policy := range policies {
		if m, ok := jsonAsInt(policy.Data[model.FieldWindowMinutes]); ok && m > widest {
			widest = m
		}
	}
	events, err := recentEvents(ctx, cfg, rc, nil, time.Now().UTC().Add(-time.Duration(widest)*time.Minute))
	if err != nil {
		return nil, err
	}

	fired := make([]map[string]any, 0)
	for _, policy := range policies {
		threshold, _ := jsonAsInt(policy.Data[model.FieldThresholdCount])
		windowMinutes, ok := jsonAsInt(policy.Data[model.FieldWindowMinutes])
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

		channels := policy.Data[model.FieldChannels]
		for zoneID, count := range perZone {
			if threshold == 0 || int64(count) < threshold {
				continue
			}
			payload := map[string]any{
				"policy":          strField(policy, model.FieldName),
				model.FieldZoneID: zoneID,
				"eventCount":      count,
				"windowMinutes":   windowMinutes,
			}
			if err := ensureCanRecord(ctx, state, rc); err != nil {
				return nil, err
			}
			delivered, detail := cfg.deliver(ctx, channels, payload)
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

// testAlertPolicy is the action waf.alert_policies.test — the "Send test alert" button. Runs
// the real delivery path and writes a real AlertNotification, because a test that takes a
// different path than production proves nothing about production.
func testAlertPolicy(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, policyID uuid.UUID) (map[string]any, error) {
	getResult, err := state.Crud.Get(ctx, model.AlertPolicies, policyID, rc)
	if err != nil {
		return nil, err
	}
	policy := getResult.Record

	channels := policy.Data[model.FieldChannels]
	payload := map[string]any{
		"test":    true,
		"policy":  strField(policy, model.FieldName),
		"message": "Test alert from the WAF portal",
	}
	if err := ensureCanRecord(ctx, state, rc); err != nil {
		return nil, err
	}
	delivered, detail := cfg.deliver(ctx, channels, payload)
	notificationID, err := recordNotification(ctx, state, rc, policy, channelName(channels), delivered)
	if err != nil {
		return nil, err
	}
	return map[string]any{"notificationId": notificationID, "delivered": delivered, "detail": detail}, nil
}
