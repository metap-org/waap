// Layer: APP · declared-action logic. Turns recent events into incidents (action correlate).

package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/model"
)

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
	if raw, ok := jsonAsString(event.Data[model.FieldOccurredAt]); ok {
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
func correlateIncidents(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, body correlateBody) (map[string]any, error) {
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
	events, err := recentEvents(ctx, cfg, rc, filters, cutoff)
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
		sourceIP := strField(event, model.FieldSourceIP)
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
			[][2]string{{model.FieldZoneID, key.zoneID}, {model.FieldStatus, model.IncidentOpen}}, recordScanLimit, rc)
		if err != nil {
			return nil, err
		}
		alreadyOpen := false
		for _, incident := range existing {
			if strings.Contains(strField(incident, model.FieldTitle), key.sourceIP) {
				alreadyOpen = true
				break
			}
		}
		if alreadyOpen {
			skipped++
			continue
		}

		data := crud.JsonObject{
			model.FieldZoneID:     jsonStr(key.zoneID),
			model.FieldTitle:      jsonStr(fmt.Sprintf("%d events from %s in %dm", count, key.sourceIP, windowMinutes)),
			model.FieldSeverity:   jsonStr(severityFor(count)),
			model.FieldEventCount: json.RawMessage(fmt.Sprintf("%d", count)),
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
