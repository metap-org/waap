# edge-plane

`waf-edge` — the mitigation engine. Takes real traffic, evaluates each request against its zone's
rules, and blocks / challenges / logs / passes it through to the customer's origin.

This is the part of the product that actually protects a site. Everything else configures it.

## Deliberately not built on `metap`

Nothing from `metap`'s platform (metadata, CRUD, permission, DB access) appears anywhere in this
tree, and that is a design decision rather than an omission (`../management-plane/docs/04-architecture-boundary.md`): a metadata-driven CRUD platform is
the right tool for config that changes shape as the business learns, and the wrong tool for a
fixed-shape, high-volume, latency-sensitive request path. It also means an edge node has no way to
reach the portal's database or schema even by accident.

The one exception, since 2026-11-07: `metap`'s `pkg/kit/*` — standard-library-only utilities
(env parsing, signal handling, HTTP serve/client) with no platform knowledge. `go/.golangci.yml`
(depguard, run in CI) rejects any other `github.com/metap-org/metap/...` import.

What it *is*: a plain hyper 1.x server, an `ArcSwap` snapshot of compiled rules, and a short
dependency list.

## How it gets its configuration

It reads Redis keys that `../control-plane` writes, and nothing else. It never calls
`../management-plane`.

- The snapshot lives in memory; **a request never touches Redis**.
- Request bodies are buffered up to `MAX_REQUEST_BODY_BYTES` (default 10 MiB); a larger body is answered `413` without reaching the origin. The server applies read-header/read/idle timeouts and recovers a handler panic as a logged 500.
- A refresh ticker (`REFRESH_INTERVAL_SECONDS`, default 10s — this value *is* the 10-30s
  config-propagation SLA in the architecture doc) checks each zone's `configVersion` and
  re-fetches only what changed.
- If Redis is down, a payload fails to parse, or the schema is newer than this build, **the last
  good snapshot keeps serving**. A config-distribution problem degrades to staleness, never to an
  outage and never to an unprotected zone.

## Request path

```
request → Host → zone snapshot (in-memory, no I/O)
        → clearance cookie? → skip challenge
        → evaluate: DDoS budget first, then rules by priority — first match wins
        → block / challenge / log
        → allowed: proxy to origin
        → non-allow: queue a SecurityEvent (never blocking)
```

Monitor mode is applied at the very end: the decision still records what *would* have happened,
and the request is passed through. That is what makes monitor mode useful rather than just "off".

## Telemetry goes up through `control-plane`

`POST` batches to `control-plane`'s ingest endpoint, never straight into `management-plane`. This
resolves the open question in `04-architecture-boundary.md` in favour of its option 2 — the one
that doc already leaned toward, because it keeps the "the edge never talks to `management-plane`" rule
intact and keeps N edge nodes' worth of small writes off the portal's database.

Events are queued with `try_send` and **dropped when the buffer is full**. Losing some evidence
during a flood is strictly better than letting the flood through because the edge was busy
reporting it.

## Limits, stated up front

Each of these is a real gap, not a rough edge that will quietly work out:

- **HTTP only** — TLS termination belongs in front of this node.
- **Buffered request bodies**, not streamed. Fine for a mitigation demo, wrong for large uploads.
- **No origin health checking or failover.** One origin per zone, and if it is down the visitor
  gets a 502 page.
- **No GeoIP database.** Country rules never match unless something upstream resolves the country
  into `GEO_COUNTRY_HEADER`.
- **Rate limiting is per-node**, so a fleet of N nodes gives an effective budget of roughly N× the
  configured one. Exactness would cost a Redis round trip per request.
- **The challenge is demo-grade** — it proves the client runs JS and honours cookies, nothing
  more. Bot management proper is v2 (`../management-plane/docs/01-product-vision.md`).
- **`CLIENT_IP_HEADER` is unset by default.** Trusting a client-supplied IP header by default
  would let anyone spoof past every IP rule and rate limit in the product.

## Running it

```bash
cp .env.example .env
go build -o waf-edge ./cmd/waf-edge && ./waf-edge   # from edge-plane/
curl -H 'Host: shop.example.com' http://localhost:8080/
curl http://localhost:8080/__edge/health
```

Needs `../control-plane` to have published at least one zone into Redis first — a node with no
rule-sets answers every request with `421 Misdirected Request`, which is the correct answer for a
hostname it has no configuration for.

## Status

**Written 2026-09-04, never compiled or run.** The project owner asked for code without
build/test in that session. Treat it as a draft to verify — see
`../../metap-docs/docs/roadmap/72-control-edge-planes.md` for the specific spots most likely to be
wrong.
