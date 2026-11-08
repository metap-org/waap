// Package telemetry reports what happened, without slowing down what happens next.
//
// Every non-allow decision produces a SecurityEvent. Two rules shape this package:
//
//  1. Request handling never waits on telemetry. Events go onto a bounded channel with a
//     non-blocking send; if the channel is full they are dropped and counted. An edge that
//     stalls requests because a reporting endpoint is slow has failed at its only job.
//  2. The edge posts to control-plane, never to management-plane. N edge nodes writing straight into
//     the portal's database would put real traffic volume onto it. The batching worker on the
//     other side is what turns this into sane write rates.
//
// Dropping events under pressure is a deliberate trade, not an oversight: security events are
// EVIDENCE, and losing some evidence during a flood is strictly better than letting the flood
// through because the edge was busy reporting it.
package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

type SecurityEvent struct {
	ZoneID          string `json:"zoneId"`
	TriggeredBy     string `json:"triggeredBy"`
	TriggeredByID   string `json:"triggeredById"`
	TriggeredByName string `json:"triggeredByName"`
	Action          string `json:"action"`
	SourceIP        string `json:"sourceIp"`
	RequestPath     string `json:"requestPath"`
	OccurredAt      string `json:"occurredAt"`
}

type batch struct {
	Events []SecurityEvent `json:"events"`
}

type Telemetry struct {
	queue   chan SecurityEvent
	dropped atomic.Uint64
	sent    atomic.Uint64
}

func New(capacity int) *Telemetry {
	return &Telemetry{queue: make(chan SecurityEvent, capacity)}
}

// Record is non-blocking by construction — called from the request path.
func (t *Telemetry) Record(event SecurityEvent) {
	select {
	case t.queue <- event:
	default:
		t.dropped.Add(1)
	}
}

func (t *Telemetry) Stats() (sent, dropped uint64) {
	return t.sent.Load(), t.dropped.Load()
}

func (t *Telemetry) markSent(count uint64) {
	t.sent.Add(count)
}

// RunShipper drains the queue into control-plane's ingest endpoint, batching by size or by
// time — whichever comes first, so a quiet zone still reports promptly and a busy one still
// batches.
func RunShipper(t *Telemetry, ingestURL string, ingestToken *string, maxBatch int, flushInterval time.Duration, shutdown <-chan struct{}) {
	client := &http.Client{Timeout: 10 * time.Second}
	pending := make([]SecurityEvent, 0, maxBatch)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-shutdown:
			// Best-effort final flush: a clean shutdown shouldn't throw away evidence that is
			// already in hand.
			if len(pending) > 0 {
				ship(client, ingestURL, ingestToken, pending, t)
			}
			slog.Info("telemetry shipper shutting down")
			return
		case event, ok := <-t.queue:
			if !ok {
				return
			}
			pending = append(pending, event)
			if len(pending) >= maxBatch {
				ship(client, ingestURL, ingestToken, pending, t)
				pending = pending[:0]
			}
		case <-ticker.C:
			if len(pending) > 0 {
				ship(client, ingestURL, ingestToken, pending, t)
				pending = pending[:0]
			}
		}
	}
}

// ship: one POST. Failures are logged and the batch is dropped rather than retried: a retry
// queue here would grow without bound under exactly the conditions that caused the failure, and
// the package doc already owns the "telemetry is lossy under pressure" trade.
func ship(client *http.Client, url string, token *string, events []SecurityEvent, t *Telemetry) {
	body, err := json.Marshal(batch{Events: events})
	if err != nil {
		slog.Warn("failed to encode telemetry batch", "error", err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("failed to build telemetry request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token != nil {
		req.Header.Set("X-WAF-Ingest-Token", *token)
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("failed to ship telemetry batch", "error", err, "count", len(events))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.markSent(uint64(len(events)))
		return
	}
	slog.Warn("ingest rejected batch", "status", resp.StatusCode, "count", len(events))
}

func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
