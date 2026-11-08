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
  // data itself lives in the Analytics plane, ../../analytics-plane).
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
    if (req.headers.authorization) headers.authorization = req.headers.authorization;
    if (req.headers.cookie) headers.cookie = req.headers.cookie;
    const responses = await Promise.all(
      [ZONES, SCANNING, ALERTING, GATEWAY].map(
        (base) =>
          fetch(`${base}/metadata/entities`, { headers }).then((r) => r.json()) as Promise<{
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

export default defineConfig({
  plugins: [
    react(),
    {
      name: "waf-multi-service-routing",
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          const url = req.url ?? "";
          if (url === "/metadata/entities" || url === "/metadata/entities/") {
            return void mergeEntityList(req, res, next);
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
    // are real symlinks (`link:../../../design-system`, `link:../../../platform-ui`) to sibling
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
    // `platform-ui`/`design-system` live outside this workspace root — Vite's default `fs.allow`
    // would 403 every request for their files through `/@fs/...`.
    fs: {
      allow: [
        searchForWorkspaceRoot(process.cwd()),
        path.resolve(import.meta.dirname, "../../../platform-ui"),
        path.resolve(import.meta.dirname, "../../../design-system"),
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
