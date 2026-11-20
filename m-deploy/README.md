# m-deploy — chạy cụm management (Customer Portal backend)

Portal nghiệp vụ cho sản phẩm **WAAP** (Web Application & API Protection, kiểu Cloudflare) —
nơi khách hàng cấu hình chính sách bảo vệ (DDoS L7, WAF, vulnerability scan), xem attack
analytics, quản lý incident. Đây là **management plane** của `metap-demo-waf` — nơi giữ dữ liệu/config
nguồn (source of truth), xây nhanh trên nền `metap` (metadata-driven CRUD/workflow/permission).
Thư mục này giữ Dockerfile, docker-compose, `.env.example` và key JWKS cho các service `m-*`; code của
từng service nằm ở `../m-zones`, `../m-scanning`, `../m-alerting`, `../m-gateway`, frontend ở `../m-web`.

Không thuộc phạm vi cụm này: **`../c-config-distributor`** (control plane — lấy config từ đây,
tính toán, đẩy xuống edge) và **`../e-edge`** (edge plane — thực thi mitigation thật tại biên) — xem
`../docs/management/04-architecture-boundary.md` cho luồng dữ liệu đầy đủ giữa các plane.

## Cấu trúc: 3 service, không còn 1 binary (2026-09-01)

Các service `m-*` tách theo pillar, mỗi pillar 1
binary/deploy cycle riêng (chung 1 Go module ở gốc repo), cùng trỏ **1 database chung theo tenant** (`Router::pool_for`, không
tách theo data layer — xem lý do kỹ thuật ở doc-comment mỗi service's `main.rs`):

| Service | Sở hữu | Port HTTP |
|---|---|---|
| `zones-service` (`m-zones/`, `m-zones/cmd/zones-service`) | `waf.domains`, `waf.zones`, `waf.ddos_policies`, `waf.firewall_rules`, `waf.ip_access_lists` | 3000 |
| `scanning-service` (`m-scanning/`) | `waf.scan_jobs`, `waf.scan_findings` | 3010 |
| `alerting-service` (`m-alerting/`) | `waf.security_events` (read model trên Analytics plane), `waf.incidents`, `waf.alert_policies`, `waf.alert_notifications` | 3020 |
| `api-gateway` (`m-gateway/cmd/api-gateway`) | không sở hữu entity nào: cửa HTTP duy nhất của portal | 4000 |

`zoneId` ở `scanning-service`/`alerting-service` là **`String` thuần, không phải `Reference`**
(cùng workaround `waf.security_events.triggeredById` đã dùng cho case polymorphic) — đăng ký
`zone_entity()` ở 2 service đó chỉ để pass `validate_references()` sẽ đồng thời lộ CRUD
`waf.zones` qua route generic `/api/:entity*` của chính chúng, phá nguyên tắc "1 service sở hữu
Zone". Chi tiết đầy đủ ở từng service's `main.rs` doc comment.

**Chỉ còn REST (2026-11-21, roadmap 116 trong `../../metap-docs`)** — GraphQL và gRPC đã bị gỡ khỏi `metap` lẫn
waap. `m-gateway/` giờ là **API gateway** (`metap`'s `pkg/apigateway`, không chứa gì riêng của WAF): route
`/api/{entity}*` (CRUD, `?ids=`, aggregate, transition, action khai báo `POST /api/{entity}[/{id}]/actions/{action}`)
tới service đang phục vụ entity đó (học từ `/metadata/entities` của từng service), gộp `/metadata/entities` và
`/metadata/entity-access` của cả 3 service, tự resolve `?expand=` xuyên service (vd. `waf.scan_jobs.zoneId` →
`waf.zones`), và gửi các route còn lại theo tiền tố (`UPSTREAMS`: `/auth/`, `/me`, `/admin/`, `/platform/`, ... →
zones-service). Gateway **không xác thực ai và không giữ credential nào** — mỗi service tự verify token của người
gọi qua JWKS, nên không còn service account cho gateway. Gọi chéo giữa service (zones-service hỏi
scanning/alerting trước khi xoá Zone) đi qua REST (`metap`'s `recordclient`, `SCANNING_URL`/`ALERTING_URL`) và chuyển
tiếp token gốc của người gọi.

**Ngoài phạm vi 3 service này** (Customer Portal backend) — Admin Portal backend
(`Plan`/`Subscription`/billing, `docs/12-billing-plans.md`, chưa xây) là 1 trục tách khác hẳn
(platform-level, xuyên tenant, không phải per-tenant DB), đi theo pattern
`metap-control`/`metap-lowcode`'s platform-level service khi build, không phải pillar service
nào ở trên — xem `docs/09-access-control.md`'s mô hình 2-portal.

**`m-web/`'s dev server và nginx production đều chỉ proxy sang `api-gateway`** (`vite.config.ts`, `nginx.conf`) —
không còn middleware tự định tuyến theo tên entity hay tự gộp metadata ở phía FE; việc đó là của gateway.

Bắt đầu đọc nghiệp vụ từ `docs/01-product-vision.md`; `docs/05-metap-technical-mapping.md` là
bản ánh xạ kỹ thuật (entity/workflow/permission cụ thể trên `metap`) mà code dưới đây bám theo.

### Mục lục `docs/`

| Doc | Nội dung |
|---|---|
| `01-product-vision.md` | Scope v1 (4 trụ cột) + 3 mảng hạ tầng phát sinh (onboarding/access-control/billing) |
| `02-domain-model.md` | Domain model đầy đủ (business-level) — mọi entity, kể cả field/entity phát sinh từ `06-12` |
| `03-personas-workflows.md` | Persona, RBAC gợi ý, core workflow |
| `04-architecture-boundary.md` | Ranh giới 3 plane, luồng config/telemetry, SLA đồng bộ edge (10-30s) |
| `05-metap-technical-mapping.md` | Ánh xạ `EntityDefinition`/workflow/permission cụ thể — code bám theo đây |
| `06-onboarding-rules-lists.md` | Domain verification, rule config, whitelist/blacklist, domain/subdomain |
| `07-portal-features.md` | Sitemap + feature list toàn portal, ưu tiên P0/P1/P2 |
| `08-module-detail-specs.md` | Từng module xuống mức màn hình/field — 6 quyết định nghiệp vụ đã chốt |
| `09-access-control.md` | Mô hình 2-portal (Admin/Customer), gán quyền theo (user, domain) |
| `10-attack-visibility.md` | Dashboard WAF/DDoS, lịch sử/chi tiết tấn công, access log (quyết định scope) |
| `11-onboarding-dns-resolution.md` | Tra DNS/IP lúc onboard, phân biệt ownership verification vs DNS routing status |
| `12-billing-plans.md` | `Plan`/`Subscription`, quota theo gói, câu hỏi mở về thanh toán |
| `13-screen-api-map.md` | Từng màn hình ↔ API cụ thể — cái nào generic (metap tự sinh) vs cần code thêm |
| `14-cloudflare-gap-analysis.md` | So sánh feature với Cloudflare thật — cái nào cố tình loại, cái nào là gap thật |

## Trạng thái

Backend (Rust, scaffold từ `metap`'s `templates/metap-app`) đã chạy được — **cả 9 entity của
pillar 1-4 đã đăng ký, build sạch, test qua API thật** (2026-08-30):
- `waf.zones` — workflow `pending→active→paused→suspended`, guard `activate` chặn zone chưa đủ
  cả `hasConfig` **và** `verificationStatus = verified`.
- `waf.ddos_policies` — `zoneId` unique (1 policy/zone, enforce thật).
- `waf.firewall_rules` — `matchCondition` lưu JSON tự do.
- `waf.scan_jobs` — workflow lặp `idle↔queued↔running↔completed/failed` (test cả vòng lặp lại).
- `waf.scan_findings` — workflow remediation `open→confirmed→fixed`/`falsePositive`/`accepted`.
- `waf.security_events` — log, không workflow, `triggeredByName` denormalize.
- `waf.incidents` — workflow `open→acknowledged→mitigating→resolved` (test hết chuỗi).
- `waf.alert_policies` / `waf.alert_notifications` — CRUD.

**Tách thành 3 service** (2026-09-01, xem bảng ở trên) — `cargo build/test --workspace` sạch cho
cả 3, mỗi service tự verify đúng entity mình sở hữu (test `owns_exactly_its_own_N_entities`).
**Verify sống qua HTTP thật sau khi tách** (cùng ngày, cả 3 service chạy thật + tenant/token
thật): tạo `ScanJob` ở `scanning-service` với `zoneId` (String) trỏ 1 Zone có sẵn ở
`zones-service` — thành công, không lỗi ràng buộc; `GET`/`POST /api/waf.zones` ở
`scanning-service` **và** `alerting-service` đều `404 entity_not_found` — xác nhận route CRUD
`waf.zones` không tồn tại ở 2 service không sở hữu nó; `zones-service`'s `relatedDisplay` cho
`DdosPolicy.zoneId` (Reference nội bộ) vẫn tự resolve đúng, không bị ảnh hưởng.

**Frontend** (`web/`, 2026-08-30): dựng từ `@metap/platform-ui` (repo `../../platform-ui`, kế
thừa `packages/platform-react` cũ của `metap`) + `@metap/ui` (repo `../../design-system`) —
generic list/form/detail/workflow UI tự sinh từ metadata, y hệt cách `metap`'s `apps/crm-fe`
dùng. Đã test thật qua browser (Playwright) **trước khi tách**: login (local JWT), nav 9 entity,
list Zone, tạo/xem/sửa record, workflow diagram + nút transition tự sinh. **Routing qua 3
service đã verify sống (2026-09-01) bằng `curl` qua dev server thật** — xem ghi chú "Cấu trúc" ở
trên; chưa re-test qua browser thật sau khi tách (chỉ verify tầng HTTP, chưa click qua UI).

**(Lịch sử, trước roadmap 116 — trang này đã bị xoá, GraphQL đã bị gỡ) Zone overview** (`src/demo/ZoneOverviewPage.tsx`, `/zones/:zoneId/overview`, link từ trang chủ
"/") — màn đầu tiên gọi [GraphQL gateway](graphql-gateway/README.md) thật từ FE thay vì REST, gộp
DDoS policy + firewall rules + scan gần nhất + incident gần nhất của 1 zone trong 1 query. Dùng
`useGraphQLQuery` mới thêm vào `@metap/platform-ui` (cùng vai trò `useApiQuery` bên REST). Verify
sống bằng đúng token đăng nhập thật qua `curl` tới `/graphql` của dev server — phát hiện + sửa 1
bug thật lúc verify (gateway ban đầu dùng keypair riêng, không decode được token đăng nhập thật —
xem `m-gateway`'s mục Auth).

Còn thiếu — toàn bộ phần **Custom** (không phải CRUD generic) đã liệt kê ở
`docs/13-screen-api-map.md`: portal IA riêng theo `docs/07-portal-features.md` (hiện chỉ là danh
sách entity phẳng + Zone overview, chưa có tab Zone-centric đầy đủ/whitelist-blacklist view/
dashboard), DNS/verify domain thật, dashboard aggregate, job correlation
`SecurityEvent→Incident`, job gửi alert thật, `metap-cron` wiring cho `ScanJob.schedule`, access
control theo (user, domain) (`docs/09-access-control.md`), permission policy cho role khác
admin, `Plan`/`Subscription`
(`docs/12-billing-plans.md`, chưa build) — cùng Traefik/reverse-proxy thật cho production
(`web/`'s dev-server routing chỉ dùng được lúc `pnpm dev`), Admin Portal backend riêng.

## Chạy local (Go, cập nhật 2026-11-08)

Module Go là module gốc của repo (`../go.mod`, `github.com/metap-org/waap`) chung cho các service `m-*` và
`c-*`; lệnh `go` chạy từ gốc repo. Postgres/RabbitMQ dùng chung với `../../metap`.

```bash
# 0. Hạ tầng dùng chung (từ ../../metap)
(cd ../../metap && docker compose up -d postgres rabbitmq)
export DATABASE_URL=postgres://metap:metap@localhost:5433/metap

# 1. DB + key + tenant + user — chỉ 1 lần cho 1 DB mới (dev-tools nằm ở ../../metap)
cd ../../metap
go run ./cmd/db-migrate                                            # migrations nhúng sẵn trong binary
go run ./cmd/dev-tools gen-jwks-key ../waap/m-deploy/keys/jwks   # key Ed25519 của issuer (gitignored)
go run ./cmd/dev-tools provision-tenant <tenantId> schema admin@waap.local <password>   # user đăng nhập portal
cd ../waap/m-deploy
cp .env.example .env    # tuỳ chọn: SMTP/PUBLIC_APP_URL

# 2a. Cả stack bằng Docker: image production (Dockerfile Go dùng chung, chọn binary bằng SERVICE)
docker compose up -d --build                 # 3 service + api-gateway + web (nginx, :8081)

# 2b. Hoặc dev loop: `go run` từ source mount vào container + Vite dev server (:5173)
docker compose -f docker-compose.dev.yml up -d
docker compose -f docker-compose.dev.yml restart zones-service   # sau khi sửa code Go

# 2c. Hoặc chạy thẳng trên host (mỗi lệnh 1 terminal)
export RABBITMQ_URL=amqp://metap:metap@localhost:5672
JWKS=http://localhost:3000/.well-known/jwks.json
PORT=3000 JWKS_KEYS_DIR=m-deploy/keys/jwks go run ./m-zones/cmd/zones-service    # issuer, :3000
PORT=3010 JWKS_URL=$JWKS go run ./m-scanning/cmd/scanning-service           # :3010
PORT=3020 JWKS_URL=$JWKS go run ./m-alerting/cmd/alerting-service           # :3020
UPSTREAMS="zones=http://localhost:3000|/health,/auth/,/me,/admin/,/platform/,/tenant-users,/preferences,/dashboards,/oauth/,/.well-known/,/scim/,/public/;scanning=http://localhost:3010;alerting=http://localhost:3020" \
  go run ./m-gateway/cmd/api-gateway               # :4000
# 2c cần DNS mock riêng: (cd ../devtools/dns-mock && DNS_MOCK_SEED_FILE=seed.dev.json go run .)
# rồi chạy zones-service với DOH_RESOLVER_URL=http://localhost:8053/resolve
```

**Analytics plane**: cả 2 compose chạy kèm `clickhouse` (:9000/:8123) và `analytics-api` (:3030,
`../a-api`). Security event nằm ở đó; alerting-service đọc qua `ANALYTICS_URL` (cho correlate/evaluate)
và phục vụ chúng cho portal thành entity chỉ-đọc `waf.security_events` (`app.Options.ExternalEntities`). Chạy 2c
trên host thì đặt `ANALYTICS_URL=http://localhost:3030` cho alerting-service.

**DNS khi dev/staging**: cả 2 compose chạy kèm `dns-mock` (`../devtools/dns-mock`, xem README
của nó). zones-service tra mọi bản ghi xác minh domain (TXT) và định tuyến (CNAME) qua đó thay cho
`dns.google`. Muốn khai báo bản ghi thì vào UI ở `http://localhost:8053`, tra thật bằng
`dig @127.0.0.1 -p 15353` (cổng host; đổi bằng `DNS_MOCK_HOST_PORT`).

### Token: một issuer duy nhất (JWKS)

`zones-service` là **issuer duy nhất**: giữ key Ed25519 (`JWKS_KEYS_DIR`), ký mọi token (login,
refresh, OAuth2) và publish `/.well-known/jwks.json`. `scanning-service`, `alerting-service`
chỉ verify qua `JWKS_URL`, không giữ private key nào — `/auth/login` ở đó
trả `501 not_token_issuer`, nên mọi login (portal, control-plane) đều
trỏ về `:3000` (qua gateway thì `/auth/` cũng được route về đó). Gọi chéo giữa service (vd. `zones-service` hỏi `scanning-service` trước khi xoá
Zone) chuyển tiếp token gốc của người gọi, không tự ký token mới.

Xoay key, không cần restart (issuer đọc lại thư mục mỗi `JWKS_KEY_DIR_RELOAD_SECONDS`, mặc định
30s; verifier refresh mỗi `JWKS_REFRESH_SECONDS`, mặc định 60s, cache tối đa
`JWKS_CACHE_TTL_SECONDS`, mặc định 300s):

```bash
cd ../../metap && go run ./cmd/dev-tools gen-jwks-key ../waap/m-deploy/keys/jwks <kid-mới>   # 1. publish, chưa ký
# đợi > JWKS_CACHE_TTL_SECONDS để mọi verifier đã thấy key mới
echo <kid-mới> > ../waap/m-deploy/keys/jwks/signing-kid                                 # 2. bắt đầu ký bằng key mới
# đợi > TTL session dài nhất (auth.sessionTtlSeconds) để token ký bằng key cũ hết hạn
rm ../waap/m-deploy/keys/jwks/<kid-cũ>.key                                               # 3. retire
```

Chuyển từ cặp RSA dùng chung cũ sang JWKS mà không đăng xuất ai: giữ thêm
`AUTH_JWT_PUBLIC_KEY_PATH` trên mọi service trong 1 vòng TTL session — token RSA
cũ (không có `kid`) vẫn verify được — rồi bỏ biến đó đi.

Login portal bằng user tạo ở `provision-tenant`. Tài khoản đó là `admin` của tenant (deny-by-default
không áp lên admin); user khác trong tenant cần policy trước khi làm được gì.

Test: `go test ./...` (unit, không cần hạ tầng) và `go test -tags integration ./...` (cần Postgres
ở `localhost:5433`, mỗi test 1 DB riêng do `metap`'s `pkg/testkit` tạo).

### Frontend (`web/`) chạy trên host

```bash
cd m-web
pnpm install   # link:../../platform-ui + link:../../design-system — 2 repo sibling này
               # phải tồn tại ở đúng vị trí, xem package.json
pnpm dev       # http://localhost:5173, proxy /api /metadata /auth /me /admin /platform ... sang api-gateway (GATEWAY_URL, mặc định :4000)
```

## `SettingsPage` (2026-09-26 → 2026-11-21)

Từng chuyển sang GraphQL (`tenantConfig`/`setTenantConfig`/`resetTenantConfig`, 2026-09-26); từ roadmap 116 lại gọi
thẳng `GET /admin/config` và `PUT/DELETE /admin/config/{key}` (`m-web/src/pages/SettingsPage.tsx`).

## Docker

- `Dockerfile` (thư mục này) là recipe Go dùng chung cho cả 4 binary — `--build-arg
  SERVICE=m-zones/cmd/zones-service|m-scanning/cmd/scanning-service|m-alerting/cmd/alerting-service|m-gateway/cmd/api-gateway`; image runtime là
  `alpine:3` + 1 binary tĩnh (~75MB). `web/Dockerfile` build production frontend (nginx).
- **Mọi build dùng `context: ../../` (root `metap-org`)**: `go.mod` có `replace
  github.com/metap-org/metap => ../metap`, nên `metap/` phải nằm trong context (vẫn lấy code
  chưa commit ở `../../metap`, không phải 1 ref đã push); `web/` cần `design-system/`,
  `platform-ui/`, `metap-themes/`. `../../.dockerignore` giữ context đó không kéo theo
  `node_modules/`/`.git`/`target/`.
- Không tự chạy Postgres/RabbitMQ — 3 service trỏ vào instance dùng chung của `../../metap` qua
  `host.docker.internal` (cổng host 5433/5672).
- `web/nginx.conf` còn 1 gap ghi trong comment đầu file: `/metadata/entities` (không kèm tên
  entity) chỉ proxy về `zones-service`, không gộp được cả 3 service như dev server của Vite.
- Lịch sử: bản Rust (2026-09-01) có 5 Dockerfile riêng (`services/*/Dockerfile`, gateway generic
  của `metap`) và `cargo watch` cho dev — đã thay hết bằng bản Go ở trên ngày 2026-11-08.
