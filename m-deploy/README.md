# m-deploy — chạy cụm management (Customer Portal backend)

Portal nghiệp vụ cho sản phẩm **WAAP** (Web Application & API Protection, kiểu Cloudflare) —
nơi khách hàng cấu hình chính sách bảo vệ (DDoS L7, WAF, vulnerability scan), xem attack
analytics, quản lý incident. Đây là **management plane** của `metap-demo-waf` — nơi giữ dữ liệu/config
nguồn (source of truth), xây nhanh trên nền `metap` (metadata-driven CRUD/workflow/permission).
Thư mục này giữ Dockerfile, docker-compose, `.env.example` và key JWKS cho các service `m-*`; code của
service nằm ở `../m-management` (một process, gồm các domain `zones`, `scanning`, `alerting`, `billing`), frontend ở `../m-web`.

Không thuộc phạm vi cụm này: **`../c-config-distributor`** (control plane — lấy config từ đây,
tính toán, đẩy xuống edge) và **`../e-edge`** (edge plane — thực thi mitigation thật tại biên) — xem
`../docs/management/04-architecture-boundary.md` cho luồng dữ liệu đầy đủ giữa các plane.

## Cấu trúc: một service cho cả management plane (2026-11-22)

Management plane là **một process: `management-service`** (`../m-management/cmd/management-service`, cổng
`3000`), phục vụ mọi entity và mọi route; portal gọi thẳng nó, **không còn API gateway**. Mỗi domain là
một package riêng trong `../m-management`, cùng cắm vào metap qua các điểm như nhau
(`../docs/architecture/08-cross-cutting/21-metap-vs-app-map.md`):

| Domain (`m-management/...`) | Sở hữu |
|---|---|
| `zones/` | `waf.domains`, `waf.zones`, `waf.ddos_policies`, `waf.firewall_rules`, `waf.ip_access_lists`, `waf.accounts`, `waf.policy_versions` |
| `scanning/` | `waf.scan_jobs`, `waf.scan_findings` |
| `alerting/` | `waf.incidents`, `waf.alert_policies`, `waf.alert_notifications`, `waf.false_positive_reports` |
| `billing/` | `waf.plans`, `waf.subscriptions`, `waf.plan_change_requests` |
| `analytics/` | `waf.security_events`: read model trên Analytics plane (`app.Options.ExternalEntities`) |

Tách ra từng service (zones/scanning/alerting/billing + gateway) đã bỏ vì chưa cần: các domain gọi nhau
ngay trong process (zones hỏi billing giới hạn gói, kiểm tra tham chiếu scan job/incident khi xoá zone)
thay vì qua REST. Hệ quả cần biết:
- `?expand=` (resolve reference xuyên entity) **chỉ do gateway làm**; service đơn lẻ không hỗ trợ.
  Portal waap không dùng nó, nên không mất gì hôm nay; `platform-ui` có tuỳ chọn `expand` nhưng chưa ai truyền.
- Không còn `SCANNING_URL`, `ALERTING_URL`, `BILLING_URL`, `BILLING_INTERNAL_SECRET`. Còn `ANALYTICS_URL`,
  `OPERATOR_TENANT_ID`, `SCANNER_URL`, `DOH_RESOLVER_URL`, `EDGE_CNAME_TARGET`, `ALLOW_PRIVATE_TARGETS`.
- `management-service` là **token issuer duy nhất** và cũng là nơi mọi login đi vào.
- Hướng tách lại thành nhiều service (nếu cần) sẽ qua metap, chưa quyết.

**Chỉ còn REST** (roadmap 116 trong `../../metap-docs`): `POST /api/{entity}[/{id}]/actions/{action}` cho
action khai báo, CRUD/aggregate/transition ở `/api/{entity}*`.

**`m-web/`'s dev server và nginx production proxy thẳng sang `management-service`** (`vite.config.ts`,
`nginx.conf`, biến `API_URL`).

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

## Cấu hình: file YAML, env ghi đè (2026-11-22)

management-service đọc `config/management.yaml` (compose) hoặc `config/management.dev.yaml` (dev) qua
`CONFIG_FILE`. Mỗi khoá là tên biến nó thay (`smtp.host` là `SMTP_HOST`); biến môi trường đặt trong
container thắng file, biến rỗng coi như chưa đặt. Thứ tự: env, file, mặc định. Secret (mật khẩu DB và
broker, `SMTP_PASSWORD`, `SECRETS_ENCRYPTION_KEYS`, `VAULT_TOKEN`) không để trong file mà ở `.env` hoặc
env của container. Đổi cổng SMTP cho một môi trường: đặt `SMTP_PORT` trong `.env`, không sửa file.
Giới hạn hiện tại: `e-edge` và `a-api` chỉ được import `metap/pkg/kit/*` (stdlib-only) nên chưa đọc
được file YAML, vẫn chỉ đọc env.

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

**(Lịch sử) Tách thành 3 service** (2026-09-01; đã gộp lại 2026-11-22) — `cargo build/test --workspace` sạch cho
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

## Migration DB (2026-11-28): chạy tay, theo file SQL, rollback được

Hướng dẫn đầy đủ (lệnh, thứ tự deploy và rollback, review file, xử lý lỗi):
`../../metap-docs/docs/migrations.md`. Tóm tắt, chạy từ gốc repo `waap`:

```bash
M="go run ./m-management/cmd/management-service migrate"
$M status                 # còn thiếu gì (chỉ đọc)
$M generate <tên>         # sau khi đổi entity: sinh SQL up/down, review rồi commit
$M up --dry-run           # xem SQL sẽ chạy
$M up                     # bảng nền + bảng entity
$M down                   # chỉ in SQL rollback
$M down --yes             # rollback 1 migration (mất dữ liệu của cột/bảng bị xoá)
(cd a-api && go run ./cmd/analytics-api migrate status)   # ClickHouse
```

**DB đã có từ trước ngày 2026-11-28** (bảng do service tự tạo lúc boot), làm một lần trước khi chạy bản mới:
`db-migrate up` (ở `../metap`) → `$M baseline` → `$M baseline --yes` → `$M status`; ClickHouse:
`analytics-api migrate up`.

## Chạy local (Go, cập nhật 2026-11-08)

Module Go là module gốc của repo (`../go.mod`, `github.com/metap-org/waap`) chung cho các service `m-*` và
`c-*`; lệnh `go` chạy từ gốc repo. Postgres/RabbitMQ dùng chung với `../../metap`.

```bash
# 0. Hạ tầng dùng chung (từ ../../metap)
(cd ../../metap && docker compose up -d postgres rabbitmq)
export DATABASE_URL=postgres://metap:metap@localhost:5433/metap

# 1. DB + key + tenant + user — chỉ 1 lần cho 1 DB mới (dev-tools nằm ở ../../metap)
cd ../../metap
go run ./cmd/db-migrate                                            # bảng nền (migrations nhúng sẵn trong binary)
(cd ../waap && go run ./m-management/cmd/management-service migrate up)   # bảng entity
go run ./cmd/dev-tools gen-jwks-key ../waap/m-deploy/keys/jwks   # key Ed25519 của issuer (gitignored)
go run ./cmd/dev-tools provision-tenant <tenantId> schema admin@waap.local <password>   # user đăng nhập portal
cd ../waap/m-deploy
cp .env.example .env    # tuỳ chọn: SMTP/PUBLIC_APP_URL

# 2a. Cả stack bằng Docker: image production (Dockerfile Go dùng chung, chọn binary bằng SERVICE)
docker compose build
docker compose run --rm management-service migrate up      # lần đầu và mỗi lần có migration mới
docker compose up -d clickhouse management-service
docker compose run --rm analytics-api migrate up
docker compose up -d                                       # management-service + analytics-api + web (nginx, :8081)

# 2b. Hoặc dev loop: `go run` từ source mount vào container + Vite dev server (:5173)
#     Container nào mount mã nguồn đều chạy bằng user của bạn, không phải root, nên file chúng ghi ra
#     (node_modules, design-system/dist) vẫn sửa/xoá được trên máy. Mặc định uid:gid là 1000:1000; nếu
#     `id -u`, `id -g` khác thì thêm HOST_UID, HOST_GID vào .env. Cache Go và kho pnpm nằm trong volume,
#     service `volume-owner` (chạy root, chỉ mount volume) giao chúng cho user đó trước khi service khác chạy.
#     `web` dùng chung node_modules với máy nhưng khác kho pnpm: nếu chạy `pnpm install` trên máy,
#     pnpm sẽ cài lại, và lần `up` sau container lại cài lại.
docker compose -f docker-compose.dev.yml up -d
docker compose -f docker-compose.dev.yml restart management-service   # sau khi sửa code Go
# Service KHÔNG tự migrate: thiếu bảng/cột thì nó không khởi động (xem "Migration DB" bên dưới).
# Dev loop:
docker compose -f docker-compose.dev.yml run --rm management-service go run ./m-management/cmd/management-service migrate up
docker compose -f docker-compose.dev.yml run --rm --no-deps analytics-api go run ./cmd/analytics-api migrate up   # cần clickhouse đang chạy

# 2c. Hoặc chạy thẳng trên host (mỗi lệnh 1 terminal)
export RABBITMQ_URL=amqp://metap:metap@localhost:5672
JWKS=http://localhost:3000/.well-known/jwks.json
PORT=3000 JWKS_KEYS_DIR=m-deploy/keys/jwks ANALYTICS_URL=http://localhost:3030 \
  go run ./m-management/cmd/management-service     # :3000, issuer + mọi entity
# Không đặt DOH_RESOLVER_URL thì tra DNS qua dns.google (xem "DNS và email khi dev" bên dưới)
```

**Analytics plane**: cả 2 compose chạy kèm `clickhouse` (:9000/:8123) và `analytics-api` (:3030,
`../a-api`). Security event nằm ở đó; management-service đọc qua `ANALYTICS_URL` (cho correlate/evaluate)
và phục vụ chúng cho portal thành entity chỉ-đọc `waf.security_events` (`app.Options.ExternalEntities`). Chạy 2c
trên host thì đặt `ANALYTICS_URL=http://localhost:3030` cho management-service.

**DNS và email khi dev** (2026-11-27): dùng `mockhub` (`../../../mockhub`), chạy trên host, thay cho
`dns-mock` và `mail-mock` đã gỡ. `docker-compose.dev.yml` đã trỏ sẵn vào nó (`config/management.dev.yaml`):

```bash
cd ../../../mockhub
MOCKHUB__DNS__PORT=15353 MOCKHUB__DNS__SEED_FILE=config/seed.example.json cargo run --release -p mockhub-server
# DNS :15353 (udp+tcp), SMTP :2525, REST + DoH :8053. Seed có sẵn *.edge.waf.local (EDGE_CNAME_TARGET).
```

Xác minh một domain: lấy `verificationToken` của domain trên portal, rồi

```bash
curl -XPOST localhost:8053/v1/dns/records -H 'content-type: application/json' \
  -d '{"name":"_waf-verify.shop.test","type":"TXT","value":"<token>","replace":true}'
```

Đọc thư mời / đặt lại mật khẩu: `curl 'localhost:8053/v1/mail/messages/latest?to=<email>'` (UI mail của
mockhub chưa nối dữ liệu thật). Không chạy mockhub thì tra DNS thất bại và không gửi được thư.

### Token: một issuer duy nhất (JWKS)

`management-service` là **issuer duy nhất**: giữ key Ed25519 (`JWKS_KEYS_DIR`), ký mọi token (login,
refresh, OAuth2) và publish `/.well-known/jwks.json`. Chỉ `analytics-api` verify bằng `JWKS_URL`
(không giữ private key); mọi login (portal, control-plane) đều vào `:3000`.

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
pnpm dev       # http://localhost:5173, proxy /api /metadata /auth /me /admin /platform ... sang management-service (API_URL, mặc định :3000)
```

## `SettingsPage` (2026-09-26 → 2026-11-21)

Từng chuyển sang GraphQL (`tenantConfig`/`setTenantConfig`/`resetTenantConfig`, 2026-09-26); từ roadmap 116 lại gọi
thẳng `GET /admin/config` và `PUT/DELETE /admin/config/{key}` (`m-web/src/pages/SettingsPage.tsx`).

## Docker

- `Dockerfile` (thư mục này) là recipe Go dùng chung cho mọi binary — `--build-arg
  SERVICE=m-management/cmd/management-service` (hoặc `c-config-distributor/cmd/waf-config-distributor`); image runtime là
  `alpine:3` + 1 binary tĩnh (~75MB). `web/Dockerfile` build production frontend (nginx).
- **Mọi build dùng `context: ../../` (root `metap-org`)**: `go.mod` có `replace
  github.com/metap-org/metap => ../metap`, nên `metap/` phải nằm trong context (vẫn lấy code
  chưa commit ở `../../metap`, không phải 1 ref đã push); `web/` cần `design-system/`,
  `platform-ui/`, `metap-themes/`. `../../.dockerignore` giữ context đó không kéo theo
  `node_modules/`/`.git`/`target/`.
- Không tự chạy Postgres/RabbitMQ — management-service trỏ vào instance dùng chung của `../../metap` qua
  `host.docker.internal` (cổng host 5433/5672).
- `web/nginx.conf` còn 1 gap ghi trong comment đầu file: `/metadata/entities` nay đi thẳng về `management-service`, nên không còn gap gộp metadata.
- Lịch sử: bản Rust (2026-09-01) có 5 Dockerfile riêng (`services/*/Dockerfile`, gateway generic
  của `metap`) và `cargo watch` cho dev — đã thay hết bằng bản Go ở trên ngày 2026-11-08.
