import path from "node:path";
import type { IncomingMessage } from "node:http";
import { defineConfig, searchForWorkspaceRoot } from "vite";
import react from "@vitejs/plugin-react";

// Every backend call goes to the API gateway (`../m-gateway`, metap's pkg/apigateway, roadmap
// 116): it routes /api/{entity} to the service that serves the entity, merges the product-wide
// metadata, and sends the other routes by prefix. Env-overridable so `../m-deploy/
// docker-compose.dev.yml`'s `web` service can point it at the container (`http://api-gateway:4000`).
const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:4000";

/** Prefixes proxied to the backend. Some are shared with SPA routes (`/admin/users`,
 *  `/admin/roles`, `/auth/oidc/callback`, ...), see isSpaNavigation. */
const PROXIED_PREFIXES = [
  "/api",
  "/admin",
  "/auth",
  "/me",
  "/platform",
  "/tenant-users",
  "/preferences",
  "/dashboards",
  "/metadata",
  "/oauth",
  "/scim",
  "/public",
  "/.well-known",
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
      name: "waf-spa-navigation",
      configureServer(server) {
        server.middlewares.use((req, _res, next) => {
          if (isSpaNavigation(req)) req.url = "/index.html";
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
    proxy: Object.fromEntries(
      PROXIED_PREFIXES.map((prefix) => [prefix, GATEWAY]),
    ),
  },
});
