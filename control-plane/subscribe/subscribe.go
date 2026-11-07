// Package subscribe is the incremental path: RabbitMQ outbox events -> recompile the affected
// zone.
//
// Subscribes with metap-go's eventbus.RunResilientConsumer rather than a hand-rolled loop — that
// wrapper already handles reconnect-with-backoff, and metap learned the cost of the naive
// version (every consumer bailing on disconnect and waiting for a process manager to restart it)
// well enough to centralise it.
//
// Routing key waf.*.record.* catches create/update/delete on every WAF entity; the handler
// filters to the three that actually shape edge behaviour. Subscribing narrowly per entity would
// mean three queues and three consumers for no gain.
package subscribe

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/metap-org/waap/control-plane/ports"

	"github.com/metap-org/metap/pkg/infra/eventbus"

	"github.com/metap-org/waap/control-plane/sync"
)

// watched lists entities whose changes can alter what the edge should do. waf.scan_*,
// waf.incidents, waf.alert_* all flow through the same exchange and are deliberately ignored
// here — none of them affects request handling.
var watched = map[string]bool{
	"waf.zones":           true,
	"waf.ddos_policies":   true,
	"waf.firewall_rules":  true,
	"waf.ip_access_lists": true,
}

func Run(ctx context.Context, amqpURL, queue string, dp ports.ConfigSource, distributor ports.RuleSink) error {
	return eventbus.RunResilientConsumer(ctx, queue, "waf.*.record.*", nil,
		func() (eventbus.EventBus, error) { return eventbus.ConnectRabbitEventBus(amqpURL) },
		func(ctx context.Context, event *eventbus.ConsumedEvent) {
			handle(ctx, dp, distributor, event)
		},
	)
}

// handle processes one message. It deliberately does not reuse the consumer loop's own ctx for
// the data-plane/Redis calls below (ctx exists only to let RunResilientConsumer's reconnect loop
// stop on shutdown) — cancelling it must stop new reconnect attempts, not abort an in-flight
// compile that is already running, which is what the Rust original's plain "await a shutdown
// future" shape (as opposed to a cancellation token threaded into the call itself) achieves.
func handle(ctx context.Context, dp ports.ConfigSource, distributor ports.RuleSink, event *eventbus.ConsumedEvent) {
	// <entity>.record.<action> — the entity name is everything before ".record.".
	entity, _, found := strings.Cut(event.RoutingKey, ".record.")
	if !found || !watched[entity] {
		_ = event.Ack()
		return
	}

	var envelope struct {
		TenantID string `json:"tenantId"`
	}
	_ = json.Unmarshal(event.Payload, &envelope)

	zoneID, ok := sync.ZoneIDFromEvent(entity, event.Payload)
	if !ok {
		// A rule/policy event with no zoneId (a delete, which carries no data) can't be
		// resolved to a zone from the event alone. Ack it and let the periodic resync pick the
		// change up — better than requeueing forever on a message that will never carry more.
		slog.Debug("event carries no resolvable zone, leaving it to the next resync", "routing_key", event.RoutingKey)
		_ = event.Ack()
		return
	}

	hostname, hasHostname := sync.HostnameFromEvent(event.Payload)
	var knownHostname *string
	if hasHostname {
		knownHostname = &hostname
	}

	if err := sync.Zone(context.Background(), dp, distributor, zoneID, envelope.TenantID, knownHostname); err != nil {
		// Requeue: the usual cause is data-plane or Redis being briefly unreachable, which
		// clears on its own. The periodic resync is the backstop if it doesn't, so this never
		// needs to be the only thing that can recover.
		slog.Warn("failed to sync zone, requeueing", "error", err, "zone_id", zoneID)
		_ = event.Nack(true)
		return
	}
	_ = event.Ack()
}
