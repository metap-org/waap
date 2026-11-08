// waf-edge — the mitigation engine. The part of this product that actually blocks a request.
//
// # Why this is not built on metap
//
// Everything else in this repo is: the portal is metadata-driven CRUD, and that is the right
// tool for config that changes shape as the business learns. This is the opposite problem —
// fixed shape, enormous volume, latency measured per request — so it is a plain net/http server
// with a short dependency list and no framework between it and the socket.
//
// # What it is allowed to know
//
// Only what control-plane compiled into Redis (package ruleset). It never reads management-plane's
// database, never calls its API, and never learns an entity name. Telemetry goes back up through
// control-plane for the same reason.
//
// # Request path
//
//	request -> Host -> zone snapshot (in-memory, no I/O)
//	        -> clearance cookie? -> skip challenge
//	        -> evaluate: DDoS budget, then rules by priority (first match wins)
//	        -> block / challenge / log
//	        -> allowed: proxy to origin
//	        -> non-allow: queue a SecurityEvent (never blocking)
//
// # Scope
//
// Reverse proxy adequate for a demo, not a CDN: HTTP only (TLS termination is expected in
// front), buffered request bodies, no origin health checking or failover, no GeoIP database of
// its own. Each of those is called out where it lives rather than implied to work.
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/metap-org/waap/edge-plane/cache"
	"github.com/metap-org/waap/edge-plane/clearance"
	"github.com/metap-org/waap/edge-plane/config"
	"github.com/metap-org/waap/edge-plane/evaluate"
	"github.com/metap-org/waap/edge-plane/pages"
	"github.com/metap-org/waap/edge-plane/proxy"
	"github.com/metap-org/waap/edge-plane/ratelimit"
	"github.com/metap-org/waap/edge-plane/ruleset"
	"github.com/metap-org/waap/edge-plane/telemetry"

	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/metap/pkg/kit/shutdown"
)

type edge struct {
	config    config.Config
	cache     *cache.RuleSetCache
	limiter   *ratelimit.RateLimiter
	telemetry *telemetry.Telemetry
	client    *http.Client
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg := config.Load()

	ruleCache, err := cache.New(cfg.RedisURL)
	if err != nil {
		slog.Error("building rule-set cache", "error", err)
		os.Exit(1)
	}
	// Fatal on purpose: a node that starts with no rule-sets answers every request with
	// "unknown host", which to every zone it should be serving is indistinguishable from a
	// total outage.
	bg := context.Background()
	if err := ruleCache.LoadInitial(bg); err != nil {
		slog.Error("loading initial rule-sets", "error", err)
		os.Exit(1)
	}

	limiter := ratelimit.New()
	tel := telemetry.New(cfg.TelemetryBuffer)

	// Background loops stop only after the server has drained, so events queued by the last
	// in-flight requests still reach the shipper's final flush.
	stopLoops := make(chan struct{})

	go ruleCache.RunRefreshLoop(bg, cfg.RefreshInterval, stopLoops)
	go telemetry.RunShipper(tel, cfg.IngestURL, cfg.IngestToken, cfg.TelemetryMaxBatch, cfg.TelemetryFlushInterval, stopLoops)
	// Rate-limit counters are pruned on a timer rather than during Check so the request path
	// never pays for cleanup.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopLoops:
				return
			case <-ticker.C:
				limiter.Prune()
			}
		}
	}()

	e := &edge{config: cfg, cache: ruleCache, limiter: limiter, telemetry: tel, client: proxy.BuildClient()}

	ctx, stop := shutdown.Context(bg)
	defer stop()
	server := &http.Server{Addr: cfg.ListenAddr, Handler: http.HandlerFunc(e.handle)}
	slog.Info("waf-edge starting", "listen_addr", cfg.ListenAddr, "zones", ruleCache.ZoneCount())
	// Returns immediately if the listener fails (e.g. port in use) instead of idling until a
	// signal arrives, as the hand-rolled loop this replaced did.
	serveErr := httpx.ServeContext(ctx, server, 10*time.Second)
	close(stopLoops)
	if serveErr != nil {
		slog.Error("waf-edge exited with an error", "error", serveErr)
		os.Exit(1)
	}
	slog.Info("shut down cleanly")
}

// clientIP: the client's IP.
//
// The socket peer by default. config.ClientIPHeader overrides it only when explicitly
// configured, because trusting a client-supplied header by default would let anyone spoof their
// source address past every IP rule and every rate limit in the product.
func (e *edge) clientIP(r *http.Request) net.IP {
	peerIP := func() net.IP {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return net.ParseIP(r.RemoteAddr)
		}
		return net.ParseIP(host)
	}
	if e.config.ClientIPHeader == nil {
		return peerIP()
	}
	value := r.Header.Get(*e.config.ClientIPHeader)
	if value == "" {
		return peerIP()
	}
	// X-Forwarded-For is a list; the first entry is the original client.
	first, _, _ := strings.Cut(value, ",")
	if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
		return ip
	}
	return peerIP()
}

func (e *edge) handle(w http.ResponseWriter, r *http.Request) {
	// Operational endpoints, answered before any zone lookup so they work even with no config
	// loaded at all.
	if r.URL.Path == "/__edge/health" {
		sent, dropped := e.telemetry.Stats()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(healthJSON(e.cache.ZoneCount(), e.cache.Epoch(), e.limiter.TrackedKeys(), sent, dropped)))
		return
	}

	// Strip the port: a zone is keyed by hostname, and example.com:8080 is the same zone.
	host, _, found := strings.Cut(r.Host, ":")
	if !found {
		host = r.Host
	}

	zone, ok := e.cache.ZoneFor(host)
	if !ok {
		pages.UnknownHost(w)
		return
	}

	ip := e.clientIP(r)
	ipText := ip.String()
	country := ""
	if e.config.GeoCountryHeader != nil {
		country = r.Header.Get(*e.config.GeoCountryHeader)
	}

	ctx := evaluate.RequestContext{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
		ClientIP: ip, ClientIPText: ipText, UserAgent: r.Header.Get("User-Agent"),
		Headers: r.Header, Country: country,
	}

	decision, matched := evaluate.Evaluate(zone, &ctx, e.limiter)
	if !matched {
		e.passThrough(w, r, zone, ipText, host)
		return
	}

	action := decision.EffectiveAction(*zone)
	ray := pages.RayID()

	// Reported before the response is built, so an event exists even for a request the origin
	// never sees. Record never blocks — a full buffer drops the event rather than the request.
	e.telemetry.Record(telemetry.SecurityEvent{
		ZoneID: zone.ZoneID, TriggeredBy: decision.TriggeredBy, TriggeredByID: decision.TriggeredByID,
		TriggeredByName: decision.TriggeredByName,
		// The REAL verdict, not the monitor-mode downgrade — seeing what enforcing would have
		// done is the entire point of monitor mode.
		Action: decision.Action.EventName(), SourceIP: ipText, RequestPath: r.URL.Path,
		OccurredAt: telemetry.NowRFC3339(),
	})

	switch action {
	case ruleset.ActionBlock:
		pages.Blocked(w, ray)
	case ruleset.ActionChallenge:
		cookieHeader := r.Header.Get("Cookie")
		// A client that already passed a challenge for this zone and IP is let through rather
		// than looped forever on the interstitial.
		if clearance.Verify(cookieHeader, e.config.ClearanceCookie, e.config.ClearanceSecret, zone.ZoneID, ipText) {
			e.passThrough(w, r, zone, ipText, host)
			return
		}
		cookie := clearance.Issue(e.config.ClearanceCookie, e.config.ClearanceSecret, zone.ZoneID, ipText, e.config.ClearanceTTL)
		pages.Challenge(w, cookie, ray)
	default:
		// Log and Allow both pass the request on: the event above is the whole effect of a log
		// rule, and this is also the branch every decision takes in monitor mode.
		e.passThrough(w, r, zone, ipText, host)
	}
}

func (e *edge) passThrough(w http.ResponseWriter, r *http.Request, zone *ruleset.CompiledZone, clientIP, host string) {
	if err := proxy.Forward(e.client, w, r, zone.OriginAddress, clientIP, host); err != nil {
		slog.Warn("origin request failed", "hostname", zone.Hostname, "origin", zone.OriginAddress, "error", err)
		pages.OriginUnreachable(w, pages.RayID())
	}
}

func healthJSON(zones int, epoch uint64, rateLimitKeys int, sent, dropped uint64) string {
	return `{"zones":` + strconv.Itoa(zones) + `,"epoch":` + strconv.FormatUint(epoch, 10) +
		`,"rateLimitKeys":` + strconv.Itoa(rateLimitKeys) + `,"eventsSent":` + strconv.FormatUint(sent, 10) +
		`,"eventsDropped":` + strconv.FormatUint(dropped, 10) + `}`
}
