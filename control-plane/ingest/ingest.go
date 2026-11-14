// Package ingest handles telemetry going UP: edge-plane -> here -> analytics-plane.
//
// Since 2026-11-08 events go to the Analytics plane (ADR-0001), in real batches, instead of one
// createWafSecurityEvents GraphQL mutation per event into Management's Postgres. The edge-facing
// side (this endpoint, its token, its back-pressure rule) is unchanged.
//
// # The architecture decision this implements
//
// The design doc left this explicitly unresolved, with two candidates: (1) the edge calls
// metap-grpc's generic RecordService.Create on management-plane directly, or (2) the edge batches to
// control-plane, which writes onward. This implements (2), which is also the option that doc
// leaned toward, for the reason it gave: option (1) breaks the rule stated everywhere else in
// the design — edge-plane never talks to management-plane directly — and it would put N edge nodes x
// real traffic worth of small writes straight onto the portal's database.
//
// # What this is not
//
// It is not a bypass of validation: analytics-api validates every event (enums, IP, timestamp,
// lengths) and files it under the service account's own tenant; an invalid event is rejected
// there and counted here, never silently stored.
//
// # Known limitation
//
// The buffer is in memory and unacknowledged: if this process dies with events queued, those
// events are lost. That is an accepted trade for v1 (telemetry, not billing data), and the
// honest fix — a durable queue between edge and portal — is a real piece of work, not a tweak.
// Flagged rather than hidden.
package ingest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/waap/control-plane/ports"

	"github.com/go-chi/chi/v5"
)

// EdgeEvent is one event as the edge reports it. Field names match waf.security_events' own
// metadata so this layer stays a pass-through rather than a second schema to keep in sync.
type EdgeEvent struct {
	ZoneID          string  `json:"zoneId"`
	TriggeredBy     string  `json:"triggeredBy"`
	TriggeredByID   string  `json:"triggeredById"`
	TriggeredByName *string `json:"triggeredByName,omitempty"`
	Action          string  `json:"action"`
	SourceIP        string  `json:"sourceIp"`
	RequestPath     string  `json:"requestPath"`
	OccurredAt      string  `json:"occurredAt"`
}

type ingestBody struct {
	Events []EdgeEvent `json:"events"`
}

type State struct {
	Queue            chan EdgeEvent
	Distributor      ports.RuleSink
	MaxRequestEvents int
	IngestToken      *string
}

// maxIngestBodyBytes bounds one ingest request. MaxRequestEvents is only checked after the body
// has been decoded, so without this an oversized body is fully read into memory first.
const maxIngestBodyBytes int64 = 16 << 20

func ingestHandler(state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// SecretEqual compares in constant time and never matches an empty token.
		if state.IngestToken != nil && !httpx.SecretEqual(r.Header.Get("x-waf-ingest-token"), *state.IngestToken) {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid ingest token")
			return
		}

		var body ingestBody
		if err := httpx.DecodeJSON(w, r, &body, maxIngestBodyBytes); err != nil {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "ingest body is too large")
				return
			}
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request_body", "malformed body")
			return
		}
		if len(body.Events) > state.MaxRequestEvents {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_many_events",
				"at most "+strconv.Itoa(state.MaxRequestEvents)+" events per request")
			return
		}

		accepted, dropped := 0, 0
		for _, event := range body.Events {
			// A non-blocking send, not a blocking one: blocking this request until the writer
			// drains would turn a slow management-plane into back-pressure on request handling at the
			// edge, which is the one place in this system that must never wait on the portal.
			select {
			case state.Queue <- event:
				accepted++
			default:
				dropped++
			}
		}
		if dropped > 0 {
			slog.Warn("ingest buffer full, dropped events", "dropped", dropped)
		}

		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"accepted": accepted, "dropped": dropped}})
	}
}

func healthHandler(state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redisOK := state.Distributor.Ping(r.Context()) == nil
		status := http.StatusOK
		if !redisOK {
			status = http.StatusServiceUnavailable
		}
		httpx.WriteJSON(w, status, map[string]any{"data": map[string]any{"self": "ok", "redis": redisOK}})
	}
}

func Router(state *State) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.Recover)
	r.Get("/health", healthHandler(state))
	r.Post("/ingest/events", ingestHandler(state))
	return r
}

// RunWriter drains the buffer into the Analytics plane, one ingest request per batch.
func RunWriter(ctx context.Context, dp ports.EventSink, queue <-chan EdgeEvent, maxBatch int, shutdown <-chan struct{}) {
	for {
		var batch []EdgeEvent
		select {
		case <-shutdown:
			slog.Info("ingest writer shutting down")
			return
		case event, ok := <-queue:
			if !ok {
				return
			}
			batch = append(batch, event)
		}
		// Opportunistically take whatever else is already queued, up to the cap — this is what
		// turns a burst into one drain pass instead of one pass per event.
	drainMore:
		for len(batch) < maxBatch {
			select {
			case event, ok := <-queue:
				if !ok {
					break drainMore
				}
				batch = append(batch, event)
			default:
				break drainMore
			}
		}

		payloads := make([]map[string]any, 0, len(batch))
		for _, event := range batch {
			payloads = append(payloads, map[string]any{
				"zoneId": event.ZoneID, "triggeredBy": event.TriggeredBy, "triggeredById": event.TriggeredByID,
				"triggeredByName": event.TriggeredByName, "action": event.Action, "sourceIp": event.SourceIP,
				"requestPath": event.RequestPath, "occurredAt": event.OccurredAt,
			})
		}
		rejected, err := dp.WriteSecurityEvents(ctx, payloads)
		if err != nil {
			// Logged and dropped, not retried: a retry loop here would stall the whole buffer
			// behind one bad batch, and this package's own doc comment already owns the
			// "telemetry is lossy under failure" trade.
			slog.Warn("failed to write security events", "error", err, "batch", len(batch))
			continue
		}
		if rejected > 0 {
			slog.Warn("analytics rejected invalid security events", "rejected", rejected, "batch", len(batch))
		}
		written := len(batch) - rejected
		slog.Debug("ingest batch written", "batch", len(batch), "written", written)
	}
}
