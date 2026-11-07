// Package ingest handles telemetry going UP: edge-plane -> here -> data-plane.
//
// # The architecture decision this implements
//
// The design doc left this explicitly unresolved, with two candidates: (1) the edge calls
// metap-grpc's generic RecordService.Create on data-plane directly, or (2) the edge batches to
// control-plane, which writes onward. This implements (2), which is also the option that doc
// leaned toward, for the reason it gave: option (1) breaks the rule stated everywhere else in
// the design — edge-plane never talks to data-plane directly — and it would put N edge nodes x
// real traffic worth of small writes straight onto the portal's database.
//
// # What this is not
//
// It is not a bypass of CrudService. Every event still goes through data-plane's ordinary
// createWafSecurityEvents mutation, so validation, permission and the outbox all apply exactly
// as they do for a portal write. What batching buys is fewer, larger requests — not a shortcut
// past the platform's rules.
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
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ingestHandler(state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if state.IngestToken != nil {
			presented := r.Header.Get("x-waf-ingest-token")
			// Length-independent comparison isn't worth reaching for here (this is a coarse
			// deployment boundary, not a per-user credential), but an absent token must never
			// pass.
			// Constant-time: a plain != leaks how many leading bytes matched via timing.
			if presented == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(*state.IngestToken)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid ingest token"})
				return
			}
		}

		var body ingestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed body"})
			return
		}
		if len(body.Events) > state.MaxRequestEvents {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
				"error": "too many events in one request", "max": state.MaxRequestEvents,
			})
			return
		}

		accepted, dropped := 0, 0
		for _, event := range body.Events {
			// A non-blocking send, not a blocking one: blocking this request until the writer
			// drains would turn a slow data-plane into back-pressure on request handling at the
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

		writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"accepted": accepted, "dropped": dropped}})
	}
}

func healthHandler(state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redisOK := state.Distributor.Ping(r.Context()) == nil
		status := http.StatusOK
		if !redisOK {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]any{"data": map[string]any{"self": "ok", "redis": redisOK}})
	}
}

func Router(state *State) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", healthHandler(state))
	r.Post("/ingest/events", ingestHandler(state))
	return r
}

// RunWriter drains the buffer into data-plane, one record per event.
//
// The batching here is of REQUESTS IN FLIGHT, not of a bulk endpoint: metap's generic CRUD has
// no batch-create route, and inventing one for this would be a much larger change to core than
// this feature warrants. So the win is that the edge makes one call per batch instead of one per
// request, and this worker absorbs the fan-out where a slow write costs nobody latency.
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

		written := 0
		for _, event := range batch {
			payload := map[string]any{
				"zoneId": event.ZoneID, "triggeredBy": event.TriggeredBy, "triggeredById": event.TriggeredByID,
				"triggeredByName": event.TriggeredByName, "action": event.Action, "sourceIp": event.SourceIP,
				"requestPath": event.RequestPath, "occurredAt": event.OccurredAt,
			}
			if err := dp.CreateSecurityEvent(ctx, payload); err != nil {
				// Logged and dropped, not retried: a retry loop here would stall the whole
				// buffer behind one bad event, and this package's own doc comment already owns
				// the "telemetry is lossy under failure" trade.
				slog.Warn("failed to write security event", "error", err)
				continue
			}
			written++
		}
		slog.Debug("ingest batch written", "batch", len(batch), "written", written)
	}
}
