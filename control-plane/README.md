# control-plane

`waf-config-distributor` — the worker between the portal and the edge. Pulls config changes from
`../data-plane`, compiles them into an edge-ready rule-set, and writes them to Redis/DragonflyDB
for `../edge-plane` to read. No UI, no CRUD, no database of its own.

Full data flow and the reasoning behind the plane split:
`../data-plane/docs/04-architecture-boundary.md`.

## Three jobs, one process

| Job | What it does |
|---|---|
| **subscribe** | RabbitMQ (`waf.*.record.*` from `metap`'s outbox) → recompile the affected zone. The fast path. |
| **resync** | Full sweep every `RESYNC_INTERVAL_SECONDS`. This is what actually guarantees convergence — an event can always be missed (a DLQ'd message, a replica dying mid-handle, someone editing the database directly), so the incremental path is an optimisation, never the guarantee. |
| **ingest** | `POST /ingest/events` — the edge posts telemetry batches here; this worker writes them into `data-plane` through its normal CRUD API. Plus `GET /health`. |

They share one process because they share one `data-plane` session and one Redis pool. If ingest
ever needs to scale separately from config distribution, that is the seam to cut.

## The Redis contract

`waf-config-distributor/src/ruleset.rs` **is** the contract — the only thing `edge-plane` is
allowed to know. Keys:

| Key | Contents |
|---|---|
| `waf:zone:{hostname}` | The zone's full compiled rule-set (JSON) |
| `waf:zone-version:{hostname}` | Its `configVersion` — lets the edge check for changes cheaply |
| `waf:zones` | SET of every published hostname |
| `waf:ruleset-epoch` | Bumped after each completed full resync |

Written with the plain `redis` crate rather than `metap-cache`'s `RedisCache`, deliberately: this
is configuration, not a cache (if it expires, a zone silently stops being protected), and the key
layout is a published contract for a process that does not depend on `metap` at all. See
`src/distribute.rs` for the full reasoning.

## What compilation decides, so the edge doesn't have to

- Zones that must not be served at all (`pending`, `suspended`) are never published.
- Disabled rules and disabled DDoS policies are dropped, not carried with a flag.
- Rules are sorted by priority once — "first match wins" is a plain iteration at the edge.
- The portal's authoring form for `matchCondition` is translated into the fixed compiled grammar.
  Whether that authoring grammar reuses `PolicyCondition` is **still an open question**
  (`../data-plane/docs/02-domain-model.md`); `src/compile.rs`'s `parse_match` is the single place
  that changes when it is answered. The edge contract does not move.
- A rule whose condition cannot be represented is **dropped and logged loudly**, never published
  with a weakened condition — a rule that matches more than its author wrote is worse than one
  that is missing.

## Running it

```bash
cp .env.example .env      # then set CONTROL_SERVICE_EMAIL / CONTROL_SERVICE_PASSWORD
go run ./cmd/waf-config-distributor   # from control-plane/
```

Needs `../data-plane`'s services running (at least `zones-service` and `alerting-service`), plus
Redis and RabbitMQ. The service account is a real user in the tenant whose config this
distributes — create it with `metap`'s `dev-tools create-user`, and give it read access to
`waf.zones`/`waf.ddos_policies`/`waf.firewall_rules` and create on `waf.security_events`.

## Status

**Written 2026-09-04, never compiled or run.** The project owner asked for code without
build/test in that session. Treat it as a draft to verify — see
`../../metap-docs/docs/roadmap/72-control-edge-planes.md` for the specific spots most likely to
be wrong.
