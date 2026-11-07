# First frontend Dockerfile anywhere in `metap-org` — no existing pattern to follow (only Rust
# service Dockerfiles exist elsewhere; see ../../README.md's Docker section). Build context MUST
# be the metap-org ROOT (`../../../docker-compose.yml` builds this with `context: ../../`), not
# this directory or even `data-plane/` — this app's `package.json` depends on
# `@metap/platform-ui`/`@metap/ui` via `link:../../../platform-ui`/`link:../../../design-system`,
# plus `@metap/theme-enterprise` via `link:../../../metap-themes/packages/enterprise`
# (real pnpm symlinks to sibling repos, not published packages), so the sibling repos have to be
# visible in the same build context at the same relative nesting they have on disk
# (`design-system/`, `platform-ui/`, `metap-themes/`, `waap/data-plane/web/`, all under
# 1 root) for `pnpm install` to resolve them. The root `../../../.dockerignore`
# (metap-org/.dockerignore, new alongside this file) keeps that wide context from also shipping
# every sibling repo's `target/`/`node_modules/`/`.git` — read that file's own comment before
# touching either.
#
# `@metap/theme-enterprise` is source-consumed (`main`/`exports` point straight at `src/`, no
# build step, no `dependencies` of its own) and pnpm resolves the `link:` symlink lazily — the
# earlier `pnpm install --frozen-lockfile` for `web` succeeds even before this directory is
# copied in, but `web`'s `pnpm build` (which actually imports `@metap/theme-enterprise/theme.css`)
# fails to resolve it if the copy below is missing (found live building this Dockerfile for the
# first time, 2026-09-11 — this dependency was added 2026-09-03 but nothing had rebuilt the image
# since).
#
# `@metap/ui` (design-system) is NOT source-consumed like `@metap/platform-ui` — its
# `package.json`'s `main`/`exports` point at `dist/`, so it needs its own `pnpm build`
# (tsup + tokens + css) before anything importing it can resolve. `@metap/platform-ui` has no
# build step of its own (`main: "./src/index.ts"`) — installed, not built.
#
# Reimplements `vite.config.ts`'s dev-only entity-routing middleware as nginx `location` rules
# (`nginx.conf`, same directory) — see that file's own top comment, including a known gap
# (`/metadata/entities` merge across 3 services) it does NOT solve.

# ---- build -----------------------------------------------------------------
FROM node:22-slim AS build
# Pinned to match the host's actual pnpm (11.24.0, confirmed working against these lockfiles) —
# NOT derived from the lockfile's own `lockfileVersion: '9.0'` header, which is a lockfile FORMAT
# version pnpm has kept stable since pnpm 8/9 through current 11.x, unrelated to the pnpm package's
# own semver. `pnpm@9` here previously resolved to an old pnpm 9.x that fails `pnpm build` in
# design-system with "packages field missing or empty" against its `pnpm-workspace.yaml` (which
# intentionally has no `packages:` field — see that file's own `allowBuilds` comment) — pnpm 11.x
# tolerates that file shape fine, matching the host.
RUN npm install -g pnpm@11
WORKDIR /workspace

# package.json + lockfile first (better layer caching) — install each package independently since
# these are 3 separate pnpm projects linked via `link:`, not 1 pnpm workspace with a shared root.
# design-system/pnpm-workspace.yaml (declares onlyBuiltDependencies: [esbuild]) MUST be copied
# before `pnpm install` runs, same stage as its package.json/lockfile — pnpm reads it during
# install to decide which postinstall scripts to allow; copying it later (with the rest of the
# source) is too late and re-triggers [ERR_PNPM_IGNORED_BUILDS] (found live, 2026-09-01).
COPY design-system/package.json design-system/pnpm-lock.yaml design-system/pnpm-workspace.yaml design-system/
COPY platform-ui/package.json platform-ui/pnpm-lock.yaml platform-ui/
COPY waap/data-plane/web/package.json waap/data-plane/web/pnpm-lock.yaml waap/data-plane/web/

RUN cd design-system && pnpm install --frozen-lockfile
RUN cd platform-ui && pnpm install --frozen-lockfile
RUN cd waap/data-plane/web && pnpm install --frozen-lockfile

COPY design-system/ design-system/
COPY platform-ui/ platform-ui/
COPY metap-themes/packages/enterprise/ metap-themes/packages/enterprise/
COPY waap/data-plane/web/ waap/data-plane/web/

RUN cd design-system && pnpm build
RUN cd waap/data-plane/web && pnpm build

# ---- runtime -----------------------------------------------------------------
FROM nginx:1.27-alpine AS runtime
COPY --from=build /workspace/waap/data-plane/web/dist /usr/share/nginx/html
COPY waap/data-plane/web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
