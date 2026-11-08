// dns-mock — programmable DNS server for dev and staging (see package dnsmock). It belongs to no
// plane: it stands in for the outside world's DNS (a customer's DNS provider, public resolvers)
// so every plane can be exercised against "real" DNS without a real domain. Never deploy it as a
// production resolver.
//
// Env:
//
//	DNS_MOCK_DNS_ADDR    UDP+TCP listen address for classic DNS (default :5353)
//	DNS_MOCK_HTTP_ADDR   DoH (/dns-query, /resolve), admin API (/api/*) and UI (/) (default :8053)
//	DNS_MOCK_STATE_FILE  where records/faults are persisted; loaded at boot (default: in-memory)
//	DNS_MOCK_SEED_FILE   loaded only when there is no state file yet (same JSON shape)
//	DNS_MOCK_UPSTREAM    real resolver (host:port) for names the mock doesn't own; unset = NXDOMAIN
//	DNS_MOCK_ADMIN_TOKEN when set, /api/* requires "Authorization: Bearer <token>" — set it on
//	                     any shared (staging) instance
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/miekg/dns"

	"github.com/metap-org/waap/devtools/dns-mock/dnsmock"
)

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func main() {
	var forward dnsmock.Forwarder
	upstream := os.Getenv("DNS_MOCK_UPSTREAM")
	if upstream != "" {
		forward = dnsmock.Upstream(upstream)
	}
	store, err := dnsmock.NewStore(os.Getenv("DNS_MOCK_STATE_FILE"), os.Getenv("DNS_MOCK_SEED_FILE"), forward)
	if err != nil {
		slog.Error("loading dns-mock state", "error", err)
		os.Exit(1)
	}
	token := os.Getenv("DNS_MOCK_ADMIN_TOKEN")
	if token == "" {
		slog.Warn("DNS_MOCK_ADMIN_TOKEN is not set: anyone who can reach the admin API can change records")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dnsAddr := envOr("DNS_MOCK_DNS_ADDR", ":5353")
	handler := dnsmock.DNSHandler(store)
	servers := []*dns.Server{
		{Addr: dnsAddr, Net: "udp", Handler: handler},
		{Addr: dnsAddr, Net: "tcp", Handler: handler},
	}
	httpAddr := envOr("DNS_MOCK_HTTP_ADDR", ":8053")
	httpServer := &http.Server{Addr: httpAddr, Handler: dnsmock.HTTPHandler(store, token), ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, len(servers)+1)
	for _, srv := range servers {
		go func() { errCh <- srv.ListenAndServe() }()
	}
	go func() {
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info("dns-mock listening", "dns", dnsAddr, "http", httpAddr, "upstream", upstream, "records", len(store.Records()))

	select {
	case err := <-errCh:
		slog.Error("dns-mock listener failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	for _, srv := range servers {
		_ = srv.Shutdown()
	}
}
