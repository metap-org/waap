// waf-config-distributor — the whole of control-plane today.
//
// Three concurrent jobs in one process, all shut down together:
//
//  1. subscribe — outbox events from data-plane -> recompile the affected zone (fast path).
//  2. resync — full sweep on a timer -> guarantees convergence when an event is missed.
//  3. ingest — an HTTP endpoint the edge posts telemetry batches to, plus /health.
//
// One process rather than three because they share the same data-plane client (and therefore
// one service-account session) and the same Redis connection pool; splitting them would triple
// the login traffic and the connection count to serve the same work. If the ingest path ever
// needs to scale separately from config distribution, it is the obvious seam to cut.
//
// No UI, no CRUD, no database of its own — everything it knows comes from data-plane's API and
// everything it produces lands in Redis or back in data-plane.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/metap-org/metap/pkg/infra"
	"github.com/metap-org/metap/pkg/runtime"

	"github.com/metap-org/waap/control-plane/config"
	"github.com/metap-org/waap/control-plane/dataplane"
	"github.com/metap-org/waap/control-plane/distribute"
	"github.com/metap-org/waap/control-plane/ingest"
	"github.com/metap-org/waap/control-plane/resync"
	"github.com/metap-org/waap/control-plane/subscribe"
)

func main() {
	infra.InitTracing()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	// bg is used for all real data-plane/Redis work — never cancelled on shutdown, so an
	// in-flight compile or write finishes rather than being cut off (mirroring the Rust
	// original's plain "await a shutdown future" shape, which never threads a cancellation
	// token into the work itself). consumerCtx exists only to let the RabbitMQ consumer's own
	// reconnect loop stop trying once shutdown is requested.
	bg := context.Background()
	consumerCtx, cancelConsumer := context.WithCancel(bg)
	defer cancelConsumer()

	httpClient := runtime.DefaultHTTPClient()
	// Fails boot if it can't log in — unlike cron-scheduler, which degrades because some of its
	// job types need no credential. Here every single thing this process does needs data-plane,
	// so starting up without a session would just be a process that logs errors forever.
	token, err := runtime.StartServiceTokenSource(bg, httpClient, cfg.LoginURL, cfg.ServiceEmail, cfg.ServicePassword)
	if err != nil {
		slog.Error("logging into data-plane", "error", err)
		os.Exit(1)
	}

	dp := dataplane.New(httpClient, cfg.ZonesURL, cfg.AlertingURL, token)
	distributor, err := distribute.Connect(cfg.RedisURL)
	if err != nil {
		slog.Error("connecting to redis", "error", err)
		os.Exit(1)
	}
	if err := distributor.Ping(bg); err != nil {
		slog.Error("pinging redis", "error", err)
		os.Exit(1)
	}

	// Once at boot, before anything else starts: a cold edge otherwise has no config at all
	// until either the first change event or the first timer tick, and "no config" at the edge
	// means unprotected hostnames.
	if err := resync.RunOnce(bg, dp, distributor); err != nil {
		// Not fatal — the timer will retry, and refusing to boot would take the ingest endpoint
		// down with it for a problem that may well be a slow data-plane still starting up.
		slog.Error("initial resync failed, continuing", "error", err)
	}

	queue := make(chan ingest.EdgeEvent, 10_000)
	ingestState := &ingest.State{
		Queue: queue, Distributor: distributor,
		MaxRequestEvents: cfg.IngestMaxRequestEvents, IngestToken: cfg.IngestToken,
	}

	// One shutdown signal, three listeners.
	shutdown := make(chan struct{})
	go func() {
		runtime.WaitForShutdownSignal()
		close(shutdown)
		cancelConsumer()
	}()

	subscribeDone := make(chan struct{})
	go func() {
		defer close(subscribeDone)
		if err := subscribe.Run(consumerCtx, cfg.AmqpURL, cfg.Queue, dp, distributor); err != nil {
			slog.Error("subscribe loop exited with an error", "error", err)
		}
	}()
	resyncDone := make(chan struct{})
	go func() {
		defer close(resyncDone)
		resync.RunLoop(bg, dp, distributor, cfg.ResyncInterval, shutdown)
	}()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ingest.RunWriter(bg, dp, queue, cfg.IngestMaxBatch, shutdown)
	}()

	slog.Info("waf-config-distributor starting", "addr", cfg.Addr(), "zones_url", cfg.ZonesURL,
		"redis", cfg.RedisURL, "resync_seconds", cfg.ResyncInterval.Seconds())
	if err := runtime.Serve(cfg.Addr(), ingest.Router(ingestState)); err != nil && err != http.ErrServerClosed {
		slog.Error("waf-config-distributor exited with an error", "error", err)
	}

	// Serve returns on the same shutdown signal, so by here the other three are stopping too —
	// join them so an in-flight compile or write finishes rather than being cut off.
	<-subscribeDone
	<-resyncDone
	<-writerDone
}
