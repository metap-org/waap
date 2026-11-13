# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

`metap-demo-waf` is a demo **WAAP** (Web Application & API Protection, Cloudflare-style) product.

**Rust removed from all 3 planes, Go is now the sole implementation (2026-11-01).** Each plane's
Rust workspace (`management-plane/{services,graphql-gateway}`, `control-plane/waf-config-distributor`,
`edge-plane/waf-edge`, plus each plane's own `Cargo.toml`/`.lock`/`.cargo/`) was deleted outright
once its `go/` replacement (same per-plane module boundary: `management-plane/go`, `control-plane/go`,
`edge-plane/go` — `edge-plane/go` still carries **zero** `metap-go` dependency, matching the
Rust original's own hard rule) reached full build/test/vet-clean parity and was verified live
against real Postgres/Redis/HTTP. `edge-plane/go`'s one disclosed behavioral divergence: Go's
`net.IP` has no explicit V4/V6 discriminant independent of bit pattern, so an IPv4-mapped IPv6
client address matches a plain IPv4 CIDR rule there, where the Rust original's own test pinned
the opposite as a documented gap. `management-plane/go`'s `graphqlgateway` package is a **bespoke**
forwarding gateway (`github.com/graphql-go/graphql`, no codegen) rather than a port of
`metap-graphql`'s generic dynamic-schema-from-metadata engine — that engine was never built in
`metap-go` at all (see `../metap/CLAUDE.md`), so a direct port wasn't possible; confirmed with
the project owner before building the replacement. Every `cargo`/`Cargo.toml`/`crates/`-shaped
reference anywhere below in this file (and in each plane's own `README.md`) is now **historical**
— describing the Rust-era layout this file's own "don't rewrite history" convention kept rather
than deleted, not a path/command that still works in this repo — read it as "this is what the
Rust version used to do/be called," the same way `../metap/CLAUDE.md` treats its own
`crates/`→`go/` references after its Rust removal.

**Update 2026-11-07:** "zero `metap-go` dependency" for `edge-plane/go` above is history too —
it now imports `metap`'s `pkg/kit/*` and nothing else from `metap` (depguard-enforced, see
Working conventions). `metap`'s Go module moved to that repo's root as
`github.com/metap-org/metap` (was `github.com/metap-org/metap-go`, rooted at `../metap/go`),
packages under `pkg/` — each plane's `go.mod` has `replace github.com/metap-org/metap =>
../../metap`. **Each plane's Go module also moved up out of its `go/` subdirectory the same day**:
`management-plane/go.mod`, `control-plane/go.mod`, `edge-plane/go.mod`, module paths
`github.com/metap-org/waap/{management-plane,control-plane,edge-plane}` (were
`github.com/metap-org/metap-demo-waf-*-go`; the repo itself is now `metap-org/waap`). Read every
`<plane>/go/...` path above or below as `<plane>/...`. `management-plane/go.mod` carries an `ignore`
directive for `./web` and `./docs`, so `go ./...` never walks `web/node_modules`.

**Update 2026-11-08: `data-plane/` was renamed `management-plane/`** to match the architecture's own
four planes (`docs/architecture/09-adr/0001-four-planes.md`: Management, Control, Edge,
Analytics) — in industry terms *data plane* means the traffic path, i.e. `edge-plane` here, so the
old name actively misled. Module path `github.com/metap-org/waap/management-plane`;
`control-plane`'s client package `dataplane` is now `management` (type `management.Client`). Every
`data-plane` mention in this file was rewritten to the new name, historical paragraphs included
(the plane is the same thing, renamed); other repos' historical docs (`../metap-docs` roadmap
entries etc.) still say `data-plane` / `metap-demo-waf/data-plane` and are left as written.

**All three planes have code now** (2026-09-04) — `management-plane/` (3 pillar services + GraphQL
gateway config + Customer Portal frontend), `control-plane/` (`waf-config-distributor`), and
`edge-plane/` (`waf-edge`, the mitigation engine). See the plane table below. **All 3 Rust
workspaces build/clippy/test clean and `management-plane/web` passes `tsc`/`oxlint`/`prettier`/`vite
build`, and `management-plane`'s own 3 e2e tests now pass against a real Postgres too** (verified
2026-09-04, same day, two separate passes — see Working conventions for exactly what each covered).
**The remaining gap this paragraph used to name — no live-infra proof for `control-plane`/
`edge-plane`, nothing showing a portal rule change actually reaching the edge — was closed
2026-09-27** (`../metap-docs/docs/roadmap/98-control-edge-live-e2e-proof.md`): a real Zone +
FirewallRule created through the portal's own GraphQL path was compiled, published to Redis, and
enforced by a real `waf-edge` process against a real HTTP request end to end, telemetry included.
The product/architecture
spec lives in `management-plane/docs/` and remains the source of truth
for anything not yet built. Read
`management-plane/docs/01-product-vision.md` through `04-architecture-boundary.md` in order before
proposing or writing any code — they contain the settled decisions (scope, domain model, personas,
plane boundaries) that later work must stay consistent with. Docs are written in Vietnamese.

This repo is built on top of a sibling repo, `../metap` (a metadata-driven platform core: Rust/axum/
sqlx/PostgreSQL/RabbitMQ, outbox pattern, generic CRUD/workflow/permission engine). `metap-demo-waf`
is one product built on that platform, not a fork of it — `metap`'s own *code/build* conventions
(its Cargo workspace commands, crate layering, `CLAUDE.md`) live in that separate sibling repo and
don't apply here directly. That's different from the *operational* conventions in
`../CLAUDE.md` (root of `metap-org`) — respond in Vietnamese, never commit without being asked,
check `target/` size before a build session — which do apply here, same as every repo in this
directory, since this file doesn't override any of them. `metap`'s primitives
(`metap-metadata`, `metap-workflow`, `metap-cron`, `metap-permission`, `metap-cache`, `metap-grpc`,
`metap-reconciler`, `metap-storage`) are the ones `management-plane/` is meant to reuse rather than
reinvent.

## Repo structure: 4 planes, 4 deploy cycles

The repo is deliberately split into four top-level plane directories (ADR-0001: Management,
Control, Edge, Analytics), each a separate codebase/deploy cycle with a hard boundary — do not
blur these when implementing:

| Directory | Role | Status |
|---|---|---|
| `management-plane/` | Business portal (source of truth): Zone, DDoS policy, firewall rule, vulnerability scan, incident, alert — built on `metap`, full CRUD/workflow/permission UI | 3 services (`zones`/`scanning`/`alerting`) + `waf-graphql-gateway` (own binary, `management-plane/graphql-gateway/`, not just config — 7 custom mutations, wraps `metap`'s generic gateway library via `build_with_extensions`) + Customer Portal frontend (10-module IA, zone-centric). Custom non-CRUD endpoints live in each service's `src/routes.rs`. **Both compose files must build/run `waf-graphql-gateway`, not `metap/crates/metap-graphql-gateway`'s generic binary** — found live, 2026-09-04: they'd been pointed at the generic one, silently dropping every custom field (`docs/roadmap/76-waf-portal-live-bugfixes.md`) |
| `control-plane/` | Headless worker: pulls config changes from `management-plane` (RabbitMQ outbox subscribe), compiles them into an edge-ready rule-set, writes to Redis/DragonflyDB. No UI, not CRUD | `waf-config-distributor` — 3 jobs in one process: subscribe (fast path), periodic full resync (**the convergence guarantee**), telemetry ingest. The Redis contract is `waf-config-distributor/src/ruleset.rs` |
| `analytics-plane/` | Telemetry: security events (later access logs, usage) — ingest, storage, query. **Not** built on `metap` (ADR-0009): only `pkg/kit/*`, depguard-enforced like `edge-plane` | `analytics-api` (2026-11-08, first cut): batch ingest from `control-plane` (`POST /v1/security-events/ingest`, ingest secret + platform token, tenant from the token), ClickHouse MergeTree with TTL retention (30 days), structured query API (allowlisted equality filters, keyset cursor, metap-shaped aggregate). Verifies platform tokens itself against zones-service's JWKS. Management reads it two ways, both with the caller's own token: the gateway serves `waf.security_events` as a read-only read model (`management-plane/analytics`, no mutations, metadata served by the gateway itself), and alerting-service's correlate/evaluate sweeps read a time window. Not yet: Kafka/Vector/enricher/archiver/detection/exporter, zone/role-level read filtering |
| `edge-plane/` | High-performance, low-latency mitigation engine: evaluates rules against real traffic, blocks/challenges/logs. Deliberately **not** built on `metap`/metadata-driven approach | `waf-edge` — hyper 1.x (now Go `net/http`), **no `metap` platform dependency** — only `metap`'s stdlib-only `pkg/kit/*` since 2026-11-07, depguard-enforced. `ArcSwap` rule snapshot (a request never touches Redis), DDoS budget → priority-ordered rules → block/challenge/log → proxy to origin |

**`devtools/` is not a plane** (2026-11-08): dev/staging stand-ins for the outside world, each its
own Go module with no `metap` dependency and never deployed to production. `devtools/dns-mock`
plays every external DNS (customer DNS providers, public resolvers): real DNS on :5353 (host :15353 under compose), DoH on
:8053, records/faults set via API or its UI. Both `management-plane` compose files run it, and point
zones-service's `DOH_RESOLVER_URL` at it. Anything that needs "real" DNS in dev goes through it
rather than a real domain.

Key rule, stated repeatedly in the docs: **`edge-plane` never talks to `management-plane` directly.** It
only reads config that `control-plane` has already computed into Redis/DragonflyDB.

Data flow:
```
management-plane (Zone/DdosPolicy/FirewallRule change via portal)
  → metap outbox (same transaction as the DB write)
  → outbox-publisher → RabbitMQ
  → control-plane worker subscribes, compiles a per-Zone rule-set, writes to Redis/DragonflyDB
  → edge-plane reads Redis directly (low latency, many edge nodes share one key)
```
`Zone.configVersion` increments on every change to a zone's policies/rules; `control-plane` and
`edge-plane` both compare it to detect stale cache instead of guessing from timestamps.

**Update 2026-11-08: telemetry now lands in `analytics-plane`, not `management-plane`.** The
edge → `control-plane` leg below is unchanged; `control-plane` now batches into analytics-api
instead of one `createWafSecurityEvents` per event. `waf.security_events` is no longer a stored
entity anywhere in Management (its Postgres table was empty when this moved — checked — and is
left in place, unused). The paragraph below is the history.

Telemetry direction (`SecurityEvent`, edge → up) **was decided in Phase 72 (2026-09-04): option 2**
— the edge batches to `control-plane`, which writes into `management-plane` through the ordinary CRUD
route. That is the option `04-architecture-boundary.md` already leaned toward, and it keeps the
"edge never talks to `management-plane` directly" rule intact. It was decided in-session rather than by
the project owner, so it is flagged in `../metap-docs/docs/roadmap/72-control-edge-planes.md` and
in that PR, and is cheap to reverse (the edge knows exactly one ingest URL).

## Domain model (business-level, not yet `EntityDefinition` code)

`management-plane/docs/02-domain-model.md` has full field-level detail. Summary of the entity graph:

```
Tenant (reused from metap control.tenants)
  └─ Zone (protected site/domain; status: pending→active→paused→(active); terminal: suspended)
       ├─ DdosPolicy (0..1 active at a time)
       ├─ FirewallRule (0..N — shared rule engine for WAF custom rules/rate-limit/IP-geo firewall;
       │    ordered by priority, first match wins)
       ├─ ScanJob (0..N, schedule via metap-cron cron expression)
       │    └─ ScanFinding (0..N; remediationStatus workflow: open→confirmed→fixed / falsePositive / accepted)
       ├─ SecurityEvent (0..N — high volume, written by edge-plane)
       └─ Incident (0..N — correlates SecurityEvents; status: open→acknowledged→mitigating→resolved,
            a metap-workflow EntityWorkflow)
AlertPolicy (Tenant-scoped, watches N zones)
  └─ AlertNotification (delivery log, sent/failed)
```

**All 9 entities across the 3 `management-plane` services are on dedicated tables now** (2026-09-07,
`../metap-docs/docs/roadmap/79-lowcode-dynamic-table-per-entity-and-waf-migration.md`),
not just `SecurityEvent`. Each service's `main.rs` reconciles its own entities at boot
(`metap_reconciler::reconcile`, hand-ordered: `zones-service` — `waf.zones` before
`waf.ddos_policies`/`waf.firewall_rules`; `scanning-service` — `waf.scan_jobs` before
`waf.scan_findings`; `alerting-service` — `waf.alert_policies` before
`waf.alert_notifications`, `waf.security_events`/`waf.incidents` independent of everything). All
9 converge to `ops_applied: 0` on a second boot, confirmed live.

**This migration found and fixed 4 real bugs in `metap` core** (`crates/metap-reconciler`,
`crates/metap-crud`), all pre-existing and unrelated to this repo's own code, all newly exposed
because `waf.ddos_policies.zoneId`/`waf.zones.hostname` are the first `unique: true` fields
anywhere in this codebase's history to go through table-per-entity:

1. **Non-convergent reconcile.** `compile.rs` used to emit *two* redundant unique constructs for
   the same field (an `IndexSpec` unique index **and** a `UniqueSpec` table constraint), and
   separately `diff.rs`'s orphan-index cleanup didn't know a constraint's own backing index isn't
   an independently-droppable object — together these meant `ops_applied` never reached 0 for any
   `unique: true` real-column field.
2. **No soft-delete awareness.** The resulting blanket `UNIQUE` constraint had no notion of
   `deleted = false` — a soft-deleted row permanently occupied its unique value, so
   deleting-then-recreating a `DdosPolicy` for the same zone (a real portal action) was rejected
   with `unique_violation` against its own soft-deleted predecessor. Reported live by the user
   mid-session, from the actual portal.
3. **`searchable` silently dropped `unique`.** `compile()`'s `searchable` branch `continue`d
   unconditionally, skipping the unique-handling logic entirely for any field that was *also*
   `unique: true` — `waf.zones.hostname` (searchable + unique) lost its uniqueness enforcement
   completely the moment `waf.zones` moved to table-per-entity, even though it was enforced on the
   old shared `records` table. Found by auditing every `unique: true` field across
   `metap-demo-waf`/`metap-demo-jira`/`metap-demo-crm` after fixing #1/#2 (only WAF has any —
   `waf.ddos_policies.zoneId` and `waf.zones.hostname`; jira/crm's code-authored entities have
   none).
4. **`unique_violation`'s field-name extraction only worked for the shared `records` table.**
   `metap-crud`'s error mapper guessed the violated constraint's name from a single hardcoded
   prefix (`uniq_records_<entity>_`) — a dedicated table's constraint is named
   `uniq_<table>_<field>` instead (no `records_`), so every unique-violation on *any*
   table-per-entity entity (not just WAF's) silently fell back to a bare `409
   {"code":"unique_violation"}` with no field or table named at all. This is what the user saw
   directly in the browser ("lỗi k rõ ràng") and flagged as its own issue.

All 4 fixed in `metap` core (`crates/metap-reconciler/src/{compile,diff}.rs`,
`crates/metap-crud/src/crud_service/{helpers,create,update}.rs`), never worked around here.
Regression-tested there (`compile.rs`/`reconcile_postgres.rs`/`crud_service_postgres.rs` unit +
e2e tests) and verified live: `zones-service` converges to `ops_applied: 0` for every entity
across repeated boots, `waf.zones.hostname`'s uniqueness is enforced again, and a duplicate
`waf.ddos_policies` create now returns `field_errors: {"zoneId": [...]}` instead of a bare code.

**A 5th bug hit live on the portal the same day, after these 4 fixes had already landed**
(2026-09-07, `../metap-docs/docs/roadmap/80-composite-unique-constraints-and-partial-unique-index-fix.md`):
recreating a `DdosPolicy` for a zone whose prior policy had been soft-deleted still failed with
`unique_violation`, because `waf.ddos_policies` on the dev DB was still sitting on the *pre-fix*
blanket `UNIQUE` constraint from before bug #2 above was fixed — bug #2's fix changes what
`compile()` builds for *new* reconciles, it doesn't retroactively convert an index that already
exists. Immediate unblock: deleted the 1 colliding soft-deleted row (user-confirmed). Root-cause
fix: converted `waf.ddos_policies` to the partial unique index bug #2 already describes. **This
surfaced a 6th, still-unresolved gap**: after the `compile()` fix, the old blanket index did not
get replaced by the new partial one through a normal `reconcile()` call — `ops_applied` stayed
nonzero indefinitely on repeated boots, even though the same mechanism converges correctly for a
brand-new entity (e2e-tested). Worked around once with a manual non-concurrent `DROP INDEX IF
EXISTS` (reconcile then converged normally on the next boot); suspected `CREATE INDEX
CONCURRENTLY` leaving state that lets a later `IF NOT EXISTS` silently no-op, but **not
root-caused** — treat as a known gap in the executor-level transition logic, not proof the
mechanism is broken for fresh entities.
**Root-caused and fixed 2026-11-06** (`../metap-docs/docs/roadmap/103-tenant-scoped-indexes-system-fields-audit-partitions.md`,
`metap-go` executor): `DROP INDEX CONCURRENTLY IF EXISTS` was emitted **without a schema**, so for any
table in a product schema not on the `search_path` (`waf`, `jira`, `entities`) it silently dropped
nothing, and the `CREATE ... IF NOT EXISTS` that follows a same-name rebuild then silently no-op'd too
— exactly the "suspected" mechanism above. The drop is now schema-qualified, with an e2e test that
failed on the old code. The same pass made `unique` tenant-scoped by default (`uniqueScope`), so
`waf.zones.hostname` and `waf.domains.apexDomain` now declare `uniqueScope: global` explicitly to keep
their platform-wide uniqueness.

**Composite/multi-field unique constraints were also built this same pass** (`EntityDefinition
.unique_constraints`, `metap-metadata`/`metap-reconciler`/`metap-crud`/`metap-lowcode`), prompted
by the user asking what a multi-field case would need (illustrated with a blacklist/whitelist
`(type, value)` example, not a request for a new real entity). **No entity in this repo uses it**
— all `unique: true` fields here (`waf.zones.hostname`, `waf.ddos_policies.zoneId`) stay
single-field; re-confirmed by grepping `unique: Some(true)` across all 9 entities after this pass,
same 2 fields as before, nothing else needs it yet.

**Real data-loss incident during this same migration, worth recording so it isn't repeated**: the
first pass wrongly assumed all 9 entities had 0 rows in `records` (based on "no seed script
exists" + `docs/05-metap-technical-mapping.md`'s stale note, never a direct query) — in fact 2
tenants (`2e87cf98-46a7-473f-bebc-20307e17d3b3`, `9de4259e-dd15-44e2-a0ff-70d323ad0ae9`) had 25
real rows across all 9 entities from live portal testing. Flipping `table_name` + reconciling
empty dedicated tables without copying that data made it invisible through the API (login still
worked — auth is unrelated to entity data — but every list came back empty), reported live by the
user as "login vẫn được nhưng dữ liệu mất". Recovered same session via
`metap_reconciler::migrate_generic_to_dedicated` per (tenant, entity) — row counts now match
`records` exactly for 8 of 9 entities; `waf.ddos_policies` needed a `deleted=false`-only copy
(bug #2 above hadn't been fixed yet at that point in the session) to avoid colliding with its own
soft-deleted history. **Lesson**: any future table-per-entity migration on a table that's had real
traffic must query `records` directly for row counts per (tenant, entity) first — doc claims and
"no seed script" are not a substitute.

**Resolved 2026-09-09** (`../metap-docs/docs/roadmap/82-record-referenced-ux-and-metadata-control-schema-split.md`):
deleting a `Zone` that still has a live `DdosPolicy`/`FirewallRule` stays blocked with
`record_referenced` (confirmed correct, not a bug, since Phase 81) — the user chose to keep the
block rather than cascade-soft-delete, and improve the error's UX instead:
`metap-crud::find_referencing_records` now reports every blocking record (not just the first),
and `../platform-ui`'s `ReferencedByErrorMessage.tsx` renders each as a link straight to the
blocking record, generically for every entity/app that uses the generic delete flow, not a
WAF-specific fix.

**2 more incidents found live 2026-09-12, debugging a "policy created but portal doesn't seem to
reflect it" report** (`waf.ddos_policies`, zone `fb2137d0-d012-4a01-9032-8ad3bed24de1`) — both are
the same *class* of bug as the 6th gap above (a tracking/ledger table says "done", but the actual
database object silently isn't there anymore), just in 2 different places:

**7th: the `pg_trgm` extension itself had silently vanished from the shared dev database**,
despite `_sqlx_migrations` recording migration 16 (`CREATE EXTENSION IF NOT EXISTS pg_trgm;`) as
applied. `zones-service` failed to boot entirely (`operator class "gin_trgm_ops" does not exist
for access method "gin"`, reconcile can't build the trigram index a `searchable` field needs).
Exactly why the extension disappeared while the ledger still says "applied" is unknown — not
root-caused, same as the 6th gap's own "not root-caused" note. Immediate fix: re-ran `CREATE
EXTENSION IF NOT EXISTS pg_trgm;` by hand (idempotent, took effect this time), restarted
`zones-service` — reconciled clean, `ops_applied=1/0/0` across its 3 entities.

**8th, still open: `waf.ddos_policies.zoneId`'s sync trigger had also silently vanished**, and
this one *did* let real duplicate data through. `zoneId` is a `Reference` field — per
`compile.rs`'s doc comment, a `Reference` field with `ref_entity` set gets `ColumnOrigin::Generated`
exactly like an explicit `storage: column` field does, meaning `compile()` intends it to get the
same `BEFORE INSERT OR UPDATE` sync trigger (`executor::build_sync_trigger_sql`) keeping the real
column in sync with `data ->> 'zoneId'` — this is not a design gap, `metap-crud`'s `create()`/
`update()` were never supposed to write that column directly, the trigger was. But querying the
live DB found **both the trigger and its backing function gone** (`pg_trigger`/`pg_proc` empty for
`waf_ddos_policies`), while `reconciler_backfill_progress` still has 4 rows marked
`completed = true` for this exact `(table, zoneId)` backfill, dated 2026-09-07/2026-09-08 (from
the original table-per-entity migration and the later `waf` schema-rename pass) — so the trigger
genuinely existed once and was later lost by some means not yet identified. Consequence: every
`createWafDdosPolicies` since then wrote a correct `data->>'zoneId'` (so the portal read the right
value back — this is why it looked like "created, portal doesn't show it" rather than "creation
failed") but left the real `"zoneId"` column `NULL`, and Postgres's own `NULL <> NULL` semantics
mean `uniq_waf_ddos_policies_zoneId ... WHERE deleted = false` **enforces nothing** against a NULL
column — 11 duplicate policies for the same zone got created back to back with zero rejection.
**Why `reconcile()` never re-detects or re-heals this on its own, every boot, `ops_applied=0`
regardless**: `diff.rs`'s convergence check for an *already-existing* `Generated` column only
re-asserts the sync trigger when `introspect()` reports `backfilled: false` — and `introspect()`
derives that flag *purely* from `reconciler_backfill_progress.completed`, never by actually
checking `pg_trigger`/`pg_proc` for the trigger's real presence. Once a row is marked
`completed = true`, nothing in the reconcile loop will ever re-verify the trigger is still there,
so a trigger dropped outside the reconciler's own lifecycle (however that happened here — not
root-caused) becomes a **silent, permanent, self-inflicted-looking gap**: the entity that already
converged once never gets re-checked again. This is the exact same failure shape as the 6th gap
above (blanket→partial unique index never retried) and the 7th (`pg_trgm`) — a real convergence
promise `metap-reconciler` makes ("level-triggered, always resumes from actual state") that
currently only holds for the specific properties `introspect()` actually re-derives from
`pg_catalog` each run, not for anything it instead trusts a bookkeeping table's flag for.

**Immediate unblock — applied, 2026-09-12**: deleted the 10 duplicate NULL-`zoneId` rows (kept the
newest of the original batch), flipped the nil-tenant `reconciler_backfill_progress` row for
`backfill:waf.waf_ddos_policies:zoneId` back to `completed = false`, restarted `zones-service` —
`reconcile()` re-asserted the trigger through the normal mechanism (`ops_applied=2`, confirmed
`trg_sync_waf_ddos_policies_zoneId` + its function both present again). This surfaced the 9th
finding below (the accompanying backfill silently touched 0 rows) — worked around by hand-backfilling
the 1 real surviving row directly, and soft-deleting a duplicate this verification pass itself
created while confirming the fix. Re-verified live end to end afterward: a repeat
`createWafDdosPolicies` for the same zone now correctly returns `409 unique_violation` with
`fieldErrors: {"zoneId": [...]}`, not a silent duplicate.

**Root-cause fix, 3 parts** (docs-first per project owner's request — this section is the plan,
not a changelog of what's done; **update 2026-09-17: items 1 and 3 are now done** —
`../metap-docs/docs/roadmap/84-reconciler-sync-trigger-ledger-drift-fix.md` and
`../metap-docs/docs/roadmap/89-backfill-tenant-scoping-fix.md` respectively. Item 2 remains a
flagged, undecided architecture question):
1. **Close the specific gap**: make `introspect()`'s `ColumnOrigin::Generated` classification (or
   a new check `diff()` runs alongside it) also verify the sync trigger/function actually exist in
   `pg_catalog` (`pg_trigger`/`pg_proc`), not just trust `reconciler_backfill_progress.completed`.
   A `Generated` column whose trigger is missing should read back as `backfilled: false` (or a new,
   more precisely-named state) regardless of what the ledger says, so `diff()`'s existing
   `push_sync_and_backfill` re-assertion path fires on the very next reconcile — no new DDL op type
   needed, just a more honest `actual` read.
2. **Close the general pattern — decided 2026-09-27, no new code**: the 6th gap, the `pg_trgm`
   incident, and this one are 3 separate instances of "a ledger says done,
   `introspect()`/migration-runner never re-verifies the real object". Re-examined rather than
   just picking one of the 2 options this bullet used to pose:
   - **Full re-derivation from `pg_catalog`** (no trusted ledger at all) was rejected as
     disproportionate — the broader audit this same finding triggered (see the paragraph above)
     already checked every other convergence signal `introspect.rs`/`diff.rs` reads (indexes/FKs/
     unique constraints) and found all of them *already* re-derive fresh from `pg_catalog` every
     call, no ledger trust involved. The sync-trigger case (item 1, done) was the one real
     instance in this crate, and it's now fixed structurally — `introspect()` calls
     `sync_trigger_exists` against `pg_trigger`/`pg_proc` on every reconcile, unconditionally, so
     this exact drift self-heals the next time `reconcile()` runs for that entity, ledger or no
     ledger.
   - **A periodic spot-check in `reconciler-orchestrator`'s poll loop** (this bullet's other
     original suggestion) turns out not to fit the actual risk: that orchestrator only ever ticks
     over *published low-code entities* (`metap_lowcode::get_published`) pulled from
     `reconciler_entity_deployments` — a **code-authored** entity (every one of the 3 real
     incidents here — `waf.ddos_policies`, `waf.zones`, all 3 WAF services' entities) is never in
     that queue at all unless an operator manually runs `dev-tools enqueue-reconcile` for it.
     Spot-checking the orchestrator's own queue would silently check nothing for the entities that
     have actually broken so far — building a periodic check that *would* help means a new,
     separate always-on watchdog covering every code-authored entity across every service, which
     is a materially bigger feature (new scheduling infrastructure, not "add a check to an
     existing loop") than this bullet originally implied.
   - **Net decision**: don't build that new watchdog right now. `introspect()`'s existing
     self-healing-on-next-reconcile behavior (item 1) is judged sufficient given zero repeat
     incidents since Phase 84 shipped it, and a code-authored entity already gets a fresh
     `reconcile()` on every service restart (a real, if infrequent, re-verification point) —
     `dev-tools enqueue-reconcile` remains the manual escape hatch if a drift is *suspected*
     between restarts. The residual risk this leaves (an externally-dropped trigger/extension
     going unnoticed between restarts) reads as an **operational monitoring gap** — alerting on an
     unplanned `DROP TRIGGER`/`DROP EXTENSION` against this database — rather than something
     `metap-reconciler` itself should be built to detect on its own schedule; flagged here rather
     than silently assumed someone else owns it.
3. **Close the 9th finding below**: `backfill::run_batched_update` needs to accept "backfill every
   tenant's rows in this table", not just one `tenant_id`, for any table reconciled with a
   sentinel/non-owning tenant id (`PLATFORM_TENANT_ID` at a `Schema`-strategy service's own boot,
   same pattern `zones-service`/`scanning-service`/`alerting-service` all use) — either drop the
   `tenant_id` filter entirely for a caller that already knows the table is shared (a new
   `BackfillColumn` variant or parameter), or have `executor.rs` resolve the *real* tenant id set
   to iterate (e.g. `SELECT DISTINCT tenant_id FROM {table}`) before calling
   `run_batched_update` once per real tenant, instead of once with whatever sentinel `reconcile()`
   itself was called with. Whichever direction, `mark_completed` firing after a query that matched
   zero rows for the *wrong reason* (not "already done", but "was never going to find anything")
   needs to stop looking identical to genuine completion — the 8th and 9th together mean a
   `Generated` column on a shared table can pass every convergence check available today
   (`ops_applied=0`, `backfilled=true`) while never actually holding correct data outside whichever
   single tenant id someone happened to reconcile it with by hand.

   **Done 2026-09-17** (`../metap-docs/docs/roadmap/89-backfill-tenant-scoping-fix.md`) — took the
   first direction (drop the filter entirely), not the per-tenant loop: `metap-reconciler` gained
   `BackfillScope::{SingleTenant,AllTenants}`, threaded through new `reconcile_with_scope`/
   `execute_with_scope` (the existing `reconcile()`/`execute()` keep their old signatures,
   defaulting to `SingleTenant`, since `../metap-demo-jira`/`../metap-demo-crm` call `reconcile()`
   directly and a signature break there can't be fixed in the same session). All 3 services below
   now call `reconcile_with_scope(..., BackfillScope::AllTenants)`. Same pass found this wasn't a
   WAF peculiarity: `metap-app::MetapApp::with_entities` (the shared builder `templates/metap-app`/
   `../metap-lowcode` also use) always reconciles with the same sentinel for every entity it
   registers, so it was fixed there too — closes the gap for every service built on that builder,
   not just these 3.

**9th, found while manually unblocking the 8th**: flipping the backfill-progress row and
restarting `zones-service` *did* restore the trigger (confirmed: `trg_sync_waf_ddos_policies_zoneId`
+ its function both back in `pg_catalog`), but the accompanying `BackfillColumn` op reported
success while backfilling **zero** real rows — `146a49e1-...` (the surviving real policy from the
8th's cleanup) still had `"zoneId" IS NULL` immediately after. Root cause:
`backfill::run_batched_update`'s batch query scopes every row it touches by `t.tenant_id = $2`,
and `zones-service`'s own boot-time `reconcile()` call always passes `metap::control::
PLATFORM_TENANT_ID` (`Uuid::nil()`) — a sentinel with zero real rows in a **shared, `Schema`-strategy**
table like `waf.waf_ddos_policies` (many real tenants' zones' policies all live in this one table,
distinguished by their own `tenant_id` column per row — this is not a `DedicatedDb` tenant's own
exclusively-owned table). The backfill batch's `WHERE t.tenant_id = $2` query matches nothing,
the loop exits immediately (`ids.is_empty()`), and `mark_completed` still fires — a real historical
row belonging to any actual tenant can never be reached by a boot-time reconcile's backfill, only
by a reconcile explicitly invoked with that tenant's own id. `run_batched_update`'s own doc comment
already states the assumption this violates: "every dedicated table belongs to exactly one
`DedicatedDb` tenant" — true for `../metap-demo-jira`'s per-tenant dedicated databases, **false**
for every `Schema`-strategy shared table in this repo (all 9 WAF entities). Immediate unblock used
here: hand-run the single-row `UPDATE ... SET "zoneId" = (data->>'zoneId')::uuid WHERE id = ...`
directly (bypassing the batch backfill's broken tenant scoping) for the one surviving real row.
**Fixed at the root 2026-09-17 — see the fix plan below, item 3.**

**Broader audit of `metap-reconciler` for the same failure class, done same day**: checked
`introspect.rs`'s other reads (indexes/FKs/unique constraints — all already re-derived fresh from
`pg_catalog` every call, no ledger trust there), `diff.rs` (compares only against that fresh data),
`watchdog.rs` (no independent cached state, re-derives everything via the next `reconcile()`), and
`orchestrator.rs`'s `reconciler_entity_deployments` (written immediately after a real `reconcile()`
call succeeds in the same call chain — not read back later as a substitute for checking reality, so
no drift window). Found one more instance of the same shape as the 9th finding above, lower risk:
`migrate.rs::copy_generic_records`'s `mark_completed` also fires unconditionally regardless of how
many rows the one-shot copy actually moved — but unlike `run_batched_update`, every current caller
of this specific function always passes a real tenant_id for a real one-shot migration, never the
`PLATFORM_TENANT_ID` sentinel that triggered the 9th finding, so this is a latent defense-in-depth
gap, not a live bug today. No other instances found — the `sync_trigger_exists` fix already shipped
(8th) and the backfill tenant-scoping gap (9th, still open) are the only 2 real instances of this
failure class in this crate.

`entities.waf_*` (all 9 tables) also moved into their own `waf` schema the same pass, out of the
schema shared with `../metap-demo-crm` — `table_name` is `qualified_table_name_in(name, "waf")`
now, not the old shared-`entities` default; see the phase doc and `../metap/CLAUDE.md`'s
`metap-control`/`metap-reconciler` bullets for the full mechanism and the `compile()` bug it
exposed (fixed at the root, not worked around here).

**`zones-service`'s create/update/delete guards were completely unreachable dead code for a while,
found and mostly fixed 2026-09-27** (`../metap-docs/docs/roadmap/99-zones-service-guard-reachability-fix.md`):
`zone_domain_guard`/`firewall_rule_match_condition_guard`/`ip_access_list_value_guard`/
`zone_delete_guard` were axum middleware gated on REST paths (`/api/waf.zones`, ...) that `metap`
core removed entirely 2026-09-21 — and this service never mounted GraphQL of its own either, so
gRPC (called by `waf-graphql-gateway` on the real Customer Portal's behalf) has been the only
reachable mutation path for a while, never touching any of the 4. The clearest live consequence:
the real Onboarding page's "Create Zone" button was hard-failing with `validation_failed: domainId
required` this whole time. Fixed by widening `metap-grpc`'s `GrpcRecordService`/
`OptionalServeConfig` to take `Arc<dyn RecordBackend>` instead of a hardcoded `Arc<CrudService>`
(a `metap` core change, `RecordBackend` being the same decoration seam `metap-graphql` already
used) and wrapping `zones-service`'s own backend in a new `GuardedZonesBackend` that ports 3 of the
4 guards' logic into `create`/`update`. **`zone_delete_guard`'s cross-service reference check was
closed the same day too** (`../metap-docs/docs/roadmap/100-zone-delete-cross-service-check.md`,
project owner: "port hết sang graphql", not left as a flagged gap) — instead of a service-account
login (which would only ever see one tenant's rows in this `Schema`-strategy shared table, wrong
for every other tenant), `GuardedZonesBackend` mints a fresh token for the *same* tenant/user as
the incoming request (the 3 services already share one signing key) and dials `scanning-service`'s/
`alerting-service`'s gRPC lazily, caching only a successful connection (`CrossServiceLink`) — so
`zones-service`'s own boot has zero dependency on either sibling being up, preserving "3
independently-deployable services" exactly. Verified live end to end through the real gateway: an
unreferenced zone deletes cleanly, one with a live `ScanJob` pointing at it is blocked with the
same `409 record_referenced` shape the old REST guard used, and deleting that `ScanJob` first makes
the zone deletable again.

**These 3 questions are resolved (2026-11-05), each already settled in code/docs before this
file's own "Notable open questions" list below was last touched — found while auditing it for
staleness, not re-decided here:**
- `FirewallRule.matchCondition` is its own JSON grammar, not `metap-permission`'s
  `PolicyCondition` — `zonesservice/entities/firewallrule.go`'s own doc comment states why (no
  `uri.*`/`header.*`/`body.*` namespace there, missing `Contains`/`Regex`/`CidrMatch`), and
  `management-plane/docs/02-domain-model.md`'s 2026-08-30 "Chốt" section recorded the same decision.
- `Incident` correlation is a static v1 rule (same `zoneId` + same `sourceIp` inside a 15-minute
  window, threshold events or more), not per-tenant configurable — see
  `alertingservice/routes.go`'s own doc comment and the same domain-model section.
- `SecurityEvent` retention: 30 days flat (no Plan-tier variation yet — `Plan`/billing has no
  entity or code anywhere in this product, so there's nothing to vary by), enforced by a new
  scheduled sweep rather than left undone: `POST /internal/security-events/cleanup`
  (`alertingservice/retention.go`), the one piece of the domain-model doc's retention decision
  that was decided but never actually built (in either the Rust original or this Go port) until
  now. Still **no real cold-storage archive** — the doc's own "xoá, không archive ở v1" call
  stands; this just stops the table growing forever rather than adding the deferred S3 path.
  **Superseded 2026-11-08**: events moved to `analytics-plane`, where the same 30 days is the
  ClickHouse table's TTL; the sweep, its route and the gateway's `cleanupSecurityEvents` mutation
  were removed.

**Separate finding from the same pass, closed the same day**: all 3 `management-plane` services' own
`tests/http_server.rs` (`zones-service`, `scanning-service`, `alerting-service` — the canonical
smoke test each copied from `metap-http`'s own template, byte-identical across all 3) still called
REST `/api/{entity}` directly and failed with a `404` where they expected `401`/`201` — the same
REST-removal breakage as everything else in this section. Ported to `/graphql` (`createTestTasks`/
`testTasks`), mirroring `metap-http`'s own already-migrated `http_server.rs` — each still stands up
a real axum server + a real RS256 JWT, just mounts `metap-graphql-http::router()` as `extra_routes`
instead of relying on the now-deleted REST route. All 3 pass against real Postgres.

## v1 scope

Four pillars only — treat anything else as out of scope unless the user says otherwise:
1. **DDoS L7 Protection** (policy per Zone: threshold/sensitivity/action)
2. **WAF / Firewall Rules** (one shared match-condition→action engine for WAF custom rules, rate
   limiting, and IP/geo firewall — deliberately not three separate features)
3. **Vulnerability Scanning** (scheduled/manual scan jobs → findings → remediation tracking)
4. **Analytics + Alerting + Incident management** (required to make the other three pillars provable)

Explicitly out of v1 (documented so it isn't forgotten, not "never"): Bot management, API schema
validation/discovery, managed WAF ruleset (OWASP CRS-style), TLS/certificate management, Page
Shield, Attack Surface Management, L3/L4 DDoS.

## Personas / RBAC

Reuses `metap`'s existing RBAC matrix + ABAC condition builder — no new permission engine, just new
action sets for these entities. Roles: Platform Admin (all tenants), Tenant Admin (full control
within tenant), Security Analyst/SOC (handles Incidents/SecurityEvents, edits rules, can't delete
Zone), Developer (owns ScanFinding remediation only, no DdosPolicy/FirewallRule access), Viewer
(read-only Analytics/Incident).

## Working conventions

- **Merge PR: luôn dùng squash merge** (chủ dự án, 2026-11-13), không dùng merge commit hay rebase merge, để `master` mỗi PR chỉ còn 1 commit. Chỉ merge khi người dùng nói "merge" và CI xanh hết. Một nhánh làm việc đã squash vào `master` thì dựng lại từ `master`; nếu không push ghi đè được thì gộp nhánh cũ bằng `merge -s ours` (PR sẽ hiện nhiều commit nhưng squash gộp lại).
- Three independent Cargo workspaces, one per plane — never one workspace spanning them, since a
  shared workspace would mean a portal change rebuilds and redeploys the edge. `cargo
  build/test/clippy --workspace` from `management-plane/`, `control-plane/`, or `edge-plane/`
  separately; `pnpm dev`/`tsc -b`/`oxlint` from `management-plane/web`.
- `edge-plane/` may import **only `github.com/metap-org/metap/pkg/kit/*`** from `metap` —
  standard-library-only utilities (`kit/env`, `kit/shutdown`, `kit/httpx`) with no
  metadata/CRUD/permission/DB knowledge, enforced by depguard (`edge-plane/.golangci.yml`, run
  in CI). This replaced the earlier "never gain a `metap` dependency" rule on 2026-11-07 (project
  owner's call: non-management-plane Go code was re-writing env/shutdown/serve helpers from
  scratch). The rule's purpose is unchanged — the edge is not metadata-driven, and a portal
  change never forces an edge redeploy (kit is a pinned dependency, not shared runtime state).
  If something there needs any *other* `metap` package, that is still the signal the work
  belongs in `control-plane` instead. Missing a general-purpose helper? Add it to `metap`'s
  `pkg/kit` (stdlib-only, that repo's own depguard rule) rather than re-implementing it here.
- **control-plane seams** (`control-plane/ports`): `ConfigSource` (management-plane reads),
  `EventSink` (telemetry writes), `RuleSink` (Redis rule-sets) — `sync`/`resync`/`subscribe`/
  `ingest` take these, and `ports/portsmock` (mockery) makes the pipeline unit-testable without a
  live management-plane or Redis. **The control-plane → edge-plane Redis format is pinned by golden
  files in `contracts/ruleset/`** (producer test in `control-plane/ruleset`, consumer test in
  `edge-plane/ruleset`); see that directory's README before changing either `ruleset`
  package.
- **Integration tests** (anything needing real Postgres) carry `//go:build integration` and use
  `metap`'s `pkg/testkit` (a fresh migrated database per test; fails, never skips, when Postgres
  is unreachable): `go test -tags integration ./...` from `management-plane`, with
  `docker compose up -d postgres` running in `../metap` (testkit defaults to its
  `localhost:5433`; override with `METAP_TEST_DATABASE_URL`). Plain `go test ./...` is unit-only.
- **Phases 70/71 (2026-09-03) and 72 (2026-09-04) landed unverified on purpose, then got a
  dedicated build/test/verify pass the same day (2026-09-04).** `cargo build`/`clippy --all-targets
  -- -D warnings`/`test --workspace` are clean across all 3 Rust workspaces (`management-plane`,
  `control-plane`, `edge-plane`); `management-plane/web` passes `tsc -b`/`oxlint`/`prettier --check`/a
  real `vite build`. That pass found and fixed real bugs — 48 TypeScript errors (every `toast()`
  call site used a shape the design system doesn't have), a clippy error, 6 dead-code warnings —
  and added 80 unit tests for the previously-untested pure logic (`aggregate` SQL planning,
  `compile.rs`'s zone/rule compilation, `evaluate.rs`'s mitigation decision, the rate limiter, the
  clearance cookie).
- **A second pass the same day (2026-09-04) ran the 3 `management-plane` services' own e2e tests against
  a real Postgres for the first time.** Docker Hub image pulls are blocked in this environment by
  an org network policy (403 at the proxy's CONNECT layer, confirmed via `/__agentproxy/status` —
  not retried/bypassed, per that proxy's own instructions), so a native (apt-installed, non-Docker)
  Postgres/RabbitMQ stood in for `docker compose up -d postgres rabbitmq`, matched to that file's
  own `metap`/`metap` credential convention. `zones-service`/`scanning-service`/`alerting-service`'s
  `http_server.rs` `#[ignore]` e2e test (`cargo test -p <service> --test http_server -- --ignored`)
  found and fixed a real bug in all 3: the test minted a JWT and called `POST /api/test.tasks`
  without ever seeding a `user_roles` row for that user, so `PermissionService::check_action`'s
  deny-by-default entity-level check (no matching policy → forbidden) correctly rejected it —
  `201` expected, `403` actual. `metap-http`'s own canonical `http_server.rs` (the template this
  was copied from) seeds `INSERT INTO user_roles (..., 'admin')` before minting its token; this
  repo's copy had dropped that line. Fixed identically in all 3 (seed + matching teardown); all 3
  now pass against live Postgres. `metap` core's own `cargo test --workspace -- --ignored` (run
  the same session, same native Postgres/RabbitMQ) is green across dozens of test files — strong
  evidence the platform primitives Phase 70-72 build on are sound beyond unit-test level. **The gap
  this paragraph used to name — no live-infra e2e proof for `control-plane`/`edge-plane`, no
  end-to-end proof a portal rule change reaches the edge — was closed 2026-09-27**
  (`../metap-docs/docs/roadmap/98-control-edge-live-e2e-proof.md`), see that phase doc and the
  top-of-file summary above for what it found and fixed along the way (2 real, previously-unnoticed
  build/runtime breaks in this exact pipeline, not just the missing proof itself).
  Full detail on all 3 passes (what shipped unverified, what the first verify pass then found, and
  what this live-Postgres pass found) is in the "Xác minh" / "Đã verify" sections of
  `../metap-docs/docs/roadmap/70-aggregate-api.md`, `71-waf-admin-portal.md`, and
  `72-control-edge-planes.md`.
- Docs are the spec. If an implementation choice isn't settled in `management-plane/docs/`, don't invent
  one silently — it's likely one of the explicitly-flagged open questions above.
- **Current state (2026-11-08): single JWKS issuer.** `zones-service` is the only service holding a
  signing key (`JWKS_KEYS_DIR` → `metap`'s `jwks.LoadKeyDir`: `<kid>.key` files + `signing-kid`);
  it signs every token (EdDSA, `kid` header) and publishes `/.well-known/jwks.json`.
  `scanning-service`, `alerting-service` and `waf-graphql-gateway` verify via `JWKS_URL` and hold
  no private key — their `/auth/login` answers `501 not_token_issuer` (checked before the
  password), so every login (portal, gateway service accounts, control-plane) goes to `:3000`.
  The gateway therefore takes `UPSTREAM_N_REST_URL` explicitly (it used to derive each upstream's
  REST base from its login URL). Cross-service calls forward the caller's own token
  (`RequestContext.ForwardedBearerToken`, now set by `metap`'s HTTP and gRPC auth) —
  `GuardedZonesBackend` no longer mints a token, and refuses a zone delete (503) when there is no
  caller token to forward. Rotation is live, no restart (issuer re-reads the key dir; verifiers
  refetch on an unknown kid and on a timer) — verified end to end on the compose stack: publish,
  promote, retire, with an old-key token rejected by all 3 services after retirement. RSA
  (`AUTH_JWT_PUBLIC_KEY_PATH`) is only a cutover fallback for kid-less tokens; the compose files no
  longer set it. Keys live in `management-plane/keys/` (gitignored, and excluded from the image build
  by `management-plane/Dockerfile.dockerignore`) and are mounted at runtime. Recipe and rotation steps:
  `management-plane/README.md` "Token: một issuer duy nhất".
- **(History) Auth moved from a shared static RSA keypair to `metap-jwks` (Ed25519, 2026-09-04).** The 3
  `management-plane` services + `graphql-gateway` used to share 1 `dev-jwt-private.pem` file (copied
  into every container) for both mint and verify. They now default to a `metap-jwks` trust root
  instead — `zones-service` publishes `/.well-known/jwks.json`, every service (including
  `zones-service` itself) verifies via `JWKS_URL`, and `graphql-gateway` verifies the same way
  (no private key file needed there at all now). This was additive/opt-in work in `metap` core
  (`metap-http::AppState.token_verifier`/`token_signer`, `metap-jwks::{TokenVerifier,
  TokenSigner}`, `dev-tools gen-jwks-key`) — `metap-demo-crm`/`metap-demo-jira`/`metap-lowcode`
  are unaffected, verified by building them unchanged. **Scope deliberately stopped short of a
  single-issuer topology**: all 3 services still hold the same private key locally (mint
  themselves), matching the old RSA topology's blast radius — only rotation got safer (`JwksKeyStore`'s
  3-step add/promote/retire), not the "1 process holds the only private key" property JWKS
  otherwise enables. See `management-plane/README.md`'s GraphQL gateway section for the auth-flow
  detail and the 3 services' own `.env.example` for the exact env vars
  (`JWKS_PRIVATE_KEY_PATH`/`JWKS_KID_PATH`/`JWKS_URL`). RSA (`AUTH_JWT_PUBLIC_KEY_PATH`/
  `AUTH_JWT_PRIVATE_KEY_PATH`, `dev-tools gen-keys`) still works as an explicit fallback if the
  JWKS env vars are unset.
