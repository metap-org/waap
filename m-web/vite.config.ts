import path from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";
import { defineConfig, searchForWorkspaceRoot } from "vite";
import react from "@vitejs/plugin-react";

// The Customer Portal backend is 3 separate services since the pillar split (2026-09-01, see
// ../README.md's service/port table) — no single origin serves every `waf.*` entity anymore.
// `targetForEntity` (below) picks the right one per request based on the entity name in the
// URL, the same "route by entity prefix" scheme `../README.md`/`../docs/61-*` describe for a
// real deployment's Traefik rules; here it's inlined into a hand-rolled dev-server middleware
// instead, since there's no Traefik in front of this dev server. **Not Vite's built-in
// `server.proxy`'s `router` option** — tried that first, found live it silently always resolved
// to the object's own static `target` regardless of what `router` returned (Vite 8's proxy
// internals didn't behave the way older Vite/`http-proxy` docs describe); a plain `fetch`-based
// forward here is simpler to get right than debugging that further.
// Env-overridable (default unchanged: plain `pnpm dev` on the host still hits `localhost`) so
// `../docker-compose.dev.yml`'s `web` service can point these at the pillar containers' own
// service DNS names (`http://zones-service:3000`, ...) instead — lets that container use normal
// bridge networking + port-mapped `ports:` rather than `network_mode: host`.
const ZONES = process.env.ZONES_URL ?? "http://localhost:3000";
const SCANNING = process.env.SCANNING_URL ?? "http://localhost:3010";
const ALERTING = process.env.ALERTING_URL ?? "http://localhost:3020";
const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:4000";

function targetForEntity(url: string): string {
  const entity = /waf\.[a-z_]+/.exec(url)?.[0];
  if (entity?.startsWith("waf.scan_")) return SCANNING;
  // Analytics read model: no service stores it, the GraphQL gateway serves its metadata (the
  // data itself lives in the Analytics plane, ../a-api).
  if (entity === "waf.security_events") return GATEWAY;
  if (entity === "waf.incidents" || entity?.startsWith("waf.alert_")) {
    return ALERTING;
  }
  return ZONES;
}

/** Forwards 1 request to `target + req.url` verbatim (method/headers/body), streams the
 *  response back unchanged. Buffers both bodies — every request/response in this app is a
 *  small JSON payload, no file upload/download route exists for `waf.*` entities, so streaming
 *  isn't needed. */
function forwardTo(target: string) {
  return async (
    req: IncomingMessage,
    res: ServerResponse,
    next: (err?: unknown) => void,
  ) => {
    try {
      const chunks: Buffer[] = [];
      for await (const chunk of req) chunks.push(chunk as Buffer);

      const headers = new Headers();
      for (const [key, value] of Object.entries(req.headers)) {
        if (value === undefined) continue;
        if (["host", "connection", "content-length"].includes(key)) continue;
        headers.set(key, Array.isArray(value) ? value.join(", ") : value);
      }

      const upstream = await fetch(`${target}${req.url}`, {
        method: req.method,
        headers,
        body: chunks.length > 0 ? Buffer.concat(chunks) : undefined,
      });

      res.statusCode = upstream.status;
      upstream.headers.forEach((value, key) => {
        if (
          ["content-encoding", "transfer-encoding", "content-length"].includes(
            key,
          )
        )
          return;
        res.setHeader(key, value);
      });
      res.end(Buffer.from(await upstream.arrayBuffer()));
    } catch (err) {
      next(err);
    }
  };
}

/** `GET /metadata/entities` (no entity in the URL — `useEntities()`, the nav/entity-list
 *  source) has no single owning service anymore; forwarding to just one would silently drop 6
 *  of 9 entities from the nav. Fan out to all 3 and merge `{ data: [...] }` — the only endpoint
 *  whose correct answer spans more than one service, everything else is a plain per-entity
 *  route.
 *
 *  Forwards `cookie` alongside `authorization` (2026-09-04, found live — `useEntities()` returned
 *  an empty list for every real caller once `@metap/platform-ui` moved to cookie sessions,
 *  2026-09-03: this only ever forwarded `authorization`, which no browser caller sends anymore,
 *  so all 3 upstream fetches landed unauthenticated, got back a `401` JSON error body with no
 *  `data` field, and silently merged into `{ data: [] }` — no error surfaced anywhere).
 *  `forwardTo` below (every other proxied route) never had this gap since it copies the whole
 *  incoming header set rather than selecting one field. No `X-CSRF-Token` forward needed here —
 *  a plain `GET` is exempt from the double-submit check REST/GraphQL both apply to
 *  state-changing requests (`metap_runtime::cookie_auth::requires_csrf_check`). */
async function mergeEntityList(
  req: IncomingMessage,
  res: ServerResponse,
  next: (err?: unknown) => void,
) {
  try {
    const headers: Record<string, string> = {};
    if (req.headers.authorization)
      headers.authorization = req.headers.authorization;
    if (req.headers.cookie) headers.cookie = req.headers.cookie;
    const responses = await Promise.all(
      [ZONES, SCANNING, ALERTING, GATEWAY].map(
        (base) =>
          fetch(`${base}/metadata/entities`, { headers }).then((r) =>
            r.json(),
          ) as Promise<{
            data?: unknown[];
          }>,
      ),
    );
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify({ data: responses.flatMap((r) => r.data ?? []) }));
  } catch (err) {
    next(err);
  }
}

/** `GET /metadata/entity-access` (the caller's per-entity read/create/update/delete map, which
 *  `@metap/platform-ui` uses to hide nav entries and buttons) — same shape of problem as
 *  `mergeEntityList`: each service only answers for its own entities, so fan out to the 3
 *  services and merge their `{ data: { <entity>: {...} } }` maps. A service that is down or
 *  answers with an error is skipped rather than failing the whole map: an entity missing from it
 *  reads as "unknown" in platform-ui, never as "denied". */
async function mergeEntityAccess(
  req: IncomingMessage,
  res: ServerResponse,
  next: (err?: unknown) => void,
) {
  try {
    const headers: Record<string, string> = {};
    if (req.headers.authorization)
      headers.authorization = req.headers.authorization;
    if (req.headers.cookie) headers.cookie = req.headers.cookie;
    const responses = await Promise.all(
      [ZONES, SCANNING, ALERTING].map((base) =>
        fetch(`${base}/metadata/entity-access`, { headers })
          .then(async (r) => ({
            status: r.status,
            body: r.ok ? await r.json() : null,
          }))
          .catch(() => ({ status: 502, body: null })),
      ),
    );
    // Every service refusing the session (401) is the caller's problem, not a partial outage:
    // pass it through so platform-ui's normal session handling sees it.
    if (responses.every((r) => r.status === 401)) {
      res.statusCode = 401;
      res.setHeader("Content-Type", "application/json");
      res.end(
        JSON.stringify({
          error: { code: "unauthorized", message: "Not signed in." },
        }),
      );
      return;
    }
    const data = Object.assign(
      {},
      ...responses.map(
        (r) =>
          (r.body as { data?: Record<string, unknown> } | null)?.data ?? {},
      ),
    );
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify({ data }));
  } catch (err) {
    next(err);
  }
}

/** Prefixes proxied to the backend that SPA routes share (`/admin/users`, `/admin/roles`,
 *  `/admin/policies`, `/admin/cron-jobs`, `/auth/oidc/callback`). */
const PROXIED_PREFIXES = [
  "/admin",
  "/auth",
  "/users",
  "/preferences",
  "/metadata",
  "/health",
];

/** A browser opening (or reloading) a page under one of PROXIED_PREFIXES, as opposed to the app's
 *  own API calls: GET with `Accept: text/html`, which `fetch` never sends by default. Those get the
 *  SPA, so a hard refresh of `/admin/users` no longer lands on the backend's 404. Kept on the
 *  backend: the browser legs of single sign-on, which redirect to or come back from the IdP —
 *  `/auth/{oidc,oauth2}/<tenant>/login`, and `/auth/sso/{start,callback}` (roadmap 115 M7).
 *  `nginx.conf` applies the same rule in production. */
function isSpaNavigation(req: IncomingMessage): boolean {
  if (req.method !== "GET" && req.method !== "HEAD") return false;
  if (!(req.headers.accept ?? "").includes("text/html")) return false;
  const path = (req.url ?? "").split("?")[0] ?? "";
  if (/^\/auth\/(oidc|oauth2)\/[^/]+\/login\/?$/.test(path)) return false;
  if (/^\/auth\/sso\/(start|callback)\/?$/.test(path)) return false;
  return PROXIED_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`),
  );
}

export default defineConfig({
  plugins: [
    react(),
    {
      name: "waf-multi-service-routing",
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          // Runs before Vite's own proxy: a page load is served the SPA's index.html.
          if (isSpaNavigation(req)) {
            req.url = "/index.html";
            return next();
          }
          const url = req.url ?? "";
          if (url === "/metadata/entities" || url === "/metadata/entities/") {
            return void mergeEntityList(req, res, next);
          }
          if (url === "/metadata/entity-access") {
            return void mergeEntityAccess(req, res, next);
          }
          if (
            url.startsWith("/api/") ||
            url.startsWith("/metadata/entities/")
          ) {
            return void forwardTo(targetForEntity(url))(req, res, next);
          }
          // The services' own non-CRUD endpoints (`services/*/src/routes.rs`). These have no
          // entity name in the path, so `targetForEntity` can't place them — each is owned by
          // exactly one service and routed by prefix instead.
          if (url.startsWith("/internal/")) {
            const target = url.startsWith("/internal/scan-jobs/")
              ? SCANNING
              : url.startsWith("/internal/incidents/") ||
                  url.startsWith("/internal/alerts/")
                ? ALERTING
                : ZONES;
            return void forwardTo(target)(req, res, next);
          }
          next();
        });
      },
    },
  ],
  resolve: {
    // Same reason as `apps/crm-fe/vite.config.ts` in `metap`: `@metap/ui`/`@metap/platform-ui`
    // are real symlinks (`link:../../design-system`, `link:../../platform-ui`) to sibling
    // repos with their own independent `pnpm install`, no shared workspace root to hoist a common
    // copy against. `@metap/ui` ships pre-bundled (`tsup`, `react`/`react-dom` externalized) so it
    // only needs those 2 deduped; `@metap/platform-ui` has no build step (`main: "./src/index.ts"`,
    // consumed as raw source), so every one of ITS OWN dependencies that also appears in this
    // app's own `package.json` needs deduping too, or Vite/Rollup resolves 2 separate physical
    // copies (one from this app's `node_modules`, one from `platform-ui`'s own) — fine for a
    // stateless function, but breaks anything that relies on module-level identity (a React
    // Context, a singleton instance): `react`/`react-dom` (the classic "Invalid hook call" case),
    // `react-router-dom` (`useNavigate()`'s `<Router>` context), `@tanstack/react-query`
    // (`QueryClientProvider`'s context) and `react-i18next` all fit that shape and are direct
    // deps of both `package.json`s (checked, 2026-09-01) — none of `platform-ui`'s OTHER deps
    // (`zustand`/`zundo`/`dayjs`/`i18next`/`@tanstack/react-virtual`) have a second copy in this
    // app's own `node_modules` to conflict with, so they don't need listing here. Never surfaced
    // under `vite dev` (its esbuild pre-bundling apparently didn't hit this the same way) —
    // found live via the Docker/nginx production `vite build` (2026-09-01): first
    // `react-router-dom`'s duplicate broke `useNavigate()`, then `@tanstack/react-query`'s
    // duplicate broke `QueryClientProvider` the same way once that one was fixed.
    dedupe: [
      "react",
      "react-dom",
      "react-router-dom",
      "@tanstack/react-query",
      "react-i18next",
    ],
  },
  server: {
    // Inside the dev compose container, file events from the bind mount don't reach Vite's
    // watcher, so an edit (to this app or to ../../platform-ui) kept serving the old module
    // until the container restarted. Polling is opt-in (VITE_USE_POLLING, set by
    // m-deploy/docker-compose.dev.yml) so plain `pnpm dev` on the host keeps native events.
    watch:
      process.env.VITE_USE_POLLING === "true"
        ? { usePolling: true, interval: 300 }
        : undefined,
    // `platform-ui`/`design-system` live outside this workspace root — Vite's default `fs.allow`
    // would 403 every request for their files through `/@fs/...`.
    fs: {
      allow: [
        searchForWorkspaceRoot(process.cwd()),
        path.resolve(import.meta.dirname, "../../platform-ui"),
        path.resolve(import.meta.dirname, "../../design-system"),
      ],
    },
    proxy: {
      // Not entity-specific — a fixed action-name enum (`/metadata/actions`) or single-service
      // OpenAPI doc (`/metadata/openapi.json`, only used for the manual `pnpm generate:types`
      // step, not by the running app) — any one service's answer is representative enough for
      // dev; zones-service same as before the split. `/api`/`/metadata/entities` are handled by
      // the plugin middleware above instead, not here.
      "/metadata": ZONES,
      "/health": ZONES,
      "/preferences": ZONES,
      "/auth": ZONES,
      "/admin": ZONES,
      // Tenant user list (`@metap/platform-ui`'s `useTenantUsers`/`useCurrentUserEmail`) — was
      // simply missing here until this app got its first consumer of it (the shell's "logged in
      // as" display, `Incident.assignedTo`'s display-hint resolution, both 2026-09-02); every
      // `@metap/platform-ui` consumer's backend exposes this route generically, same as the
      // others above.
      "/users": ZONES,
      // The WAF `graphql-gateway` instance (`../graphql-gateway/README.md`) — 1 fixed address,
      // no entity-based routing needed since the gateway itself already resolves that. This is
      // the portal's one FE->BE data transport now (2026-09-04, `src/api/waf.ts`'s doc comment) —
      // every screen using `useRecords`/`useRecord`/`useAggregate`/the mutations/custom actions in
      // that file goes through here, not just the 1 screen (`ZoneOverviewPage`, since deleted)
      // that first proved this route out.
      "/graphql": GATEWAY,
    },
  },
});
