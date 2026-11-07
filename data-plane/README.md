# data-plane

Portal nghiệp vụ cho sản phẩm **WAAP** (Web Application & API Protection, kiểu Cloudflare) —
nơi khách hàng cấu hình chính sách bảo vệ (DDoS L7, WAF, vulnerability scan), xem attack
analytics, quản lý incident. Đây là **data-plane** của `metap-demo-waf` — nơi giữ dữ liệu/config
nguồn (source of truth), xây nhanh trên nền `metap` (metadata-driven CRUD/workflow/permission).

Không thuộc phạm vi thư mục này: **`../control-plane`** (lấy config từ đây, tính toán, đẩy
xuống edge) và **`../edge-plane`** (thực thi mitigation thật tại biên) — xem
`docs/04-architecture-boundary.md` cho luồng dữ liệu đầy đủ giữa 3 phần.

## Cấu trúc: 3 service, không còn 1 binary (2026-09-01)

`data-plane/` là **Cargo workspace**, không phải 1 package — tách theo pillar, mỗi pillar 1
binary/deploy cycle riêng, cùng trỏ **1 database chung theo tenant** (`Router::pool_for`, không
tách theo data layer — xem lý do kỹ thuật ở doc-comment mỗi service's `main.rs`):

| Service | Sở hữu | Port (HTTP / gRPC opt-in) |
|---|---|---|
| `zones-service` (`cmd/zones-service`, logic ở `zonesservice/`) | `waf.domains`, `waf.zones`, `waf.ddos_policies`, `waf.firewall_rules`, `waf.ip_access_lists` | 3000 / 3001 |
| `scanning-service` (`cmd/scanning-service`, `scanningservice/`) | `waf.scan_jobs`, `waf.scan_findings` | 3010 / 3011 |
| `alerting-service` (`cmd/alerting-service`, `alertingservice/`) | `waf.security_events`, `waf.incidents`, `waf.alert_policies`, `waf.alert_notifications` | 3020 / 3021 |

`zoneId` ở `scanning-service`/`alerting-service` là **`String` thuần, không phải `Reference`**
(cùng workaround `waf.security_events.triggeredById` đã dùng cho case polymorphic) — đăng ký
`zone_entity()` ở 2 service đó chỉ để pass `validate_references()` sẽ đồng thời lộ CRUD
`waf.zones` qua route generic `/api/:entity*` của chính chúng, phá nguyên tắc "1 service sở hữu
Zone". Chi tiết đầy đủ ở từng service's `main.rs` doc comment.

**[GraphQL gateway](graphql-gateway/README.md)** (cho FE gộp query cross-service) — đã dựng, chạy
thật, tái dùng nguyên trạng `metap/crates/metap-graphql-gateway` (không code mới, chỉ cấu hình ở
`graphql-gateway/.env`) — 1 instance riêng cho WAF (không gộp chung upstream với
`metap-demo-crm`/`metap-demo-jira`), 3 `UPSTREAM_<N>_*` trỏ vào 3 service trên, port 4000. **Dùng
được cho cả mutation** (2026-09-02, đã verify sống: mutation qua `/graphql` phản ánh đúng
`updated_by` của user thật gọi, và bị 403 đúng khi user không có quyền) — token của caller được
forward nguyên vẹn xuống upstream khi hợp lệ (`RequestContext::forwarded_bearer_token`), enforce
đúng permission/audit trail của người gọi thật, không phải danh nghĩa service account; điều này
chạy được vì gateway verify qua cùng 1 trust root với 3 service (2026-09-04: chuyển từ share 1
file RSA private key sang `metap-jwks` — Ed25519, `zones-service` publish `/.well-known/
jwks.json`, mọi nơi (kể cả `zones-service` tự nó) verify qua `JWKS_URL`; gateway giờ chỉ cần
`JWKS_URL`, không cần giữ bản PEM riêng nữa. Cả 3 service vẫn cùng giữ 1 private key để mint —
chưa chuyển sang mô hình 1-issuer-duy-nhất, đó là bước tiếp theo nếu cần giảm blast radius thật
sự; xem `../CLAUDE.md`'s mục JWKS/rotation cho chi tiết. RSA vẫn còn làm fallback nếu
`JWKS_PRIVATE_KEY_PATH`/`JWKS_KID_PATH` chưa được set). Khi không có token forward (vd
lúc gateway tự fetch schema lúc boot), gateway tự login bằng service-account riêng
(`UPSTREAM_<N>_SERVICE_EMAIL`/`SERVICE_PASSWORD`, xem `graphql-gateway/README.md`'s Auth section) —
không còn JWT tĩnh mint tay phải refresh thủ công nữa.

**Ngoài phạm vi 3 service này** (Customer Portal backend) — Admin Portal backend
(`Plan`/`Subscription`/billing, `docs/12-billing-plans.md`, chưa xây) là 1 trục tách khác hẳn
(platform-level, xuyên tenant, không phải per-tenant DB), đi theo pattern
`metap-control`/`metap-lowcode`'s platform-level service khi build, không phải pillar service
nào ở trên — xem `docs/09-access-control.md`'s mô hình 2-portal.

**`web/`'s dev server route đúng cả 3 service** (`vite.config.ts`, verify qua `curl` thật) —
middleware tự forward theo tiền tố tên entity (không dùng `server.proxy`'s `router` option của
Vite — tìm ra Vite 8 không tôn trọng option đó, xem `docs/61-*` cho chi tiết), cộng 1 middleware
gộp `GET /metadata/entities` từ cả 3 service (không có entity trong path — không thuộc về đúng
1 service nào). Production thật (không phải Vite dev server) sẽ cần Traefik hoặc reverse-proxy
tương đương làm việc này, chưa dựng.

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

**Zone overview** (`src/demo/ZoneOverviewPage.tsx`, `/zones/:zoneId/overview`, link từ trang chủ
"/") — màn đầu tiên gọi [GraphQL gateway](graphql-gateway/README.md) thật từ FE thay vì REST, gộp
DDoS policy + firewall rules + scan gần nhất + incident gần nhất của 1 zone trong 1 query. Dùng
`useGraphQLQuery` mới thêm vào `@metap/platform-ui` (cùng vai trò `useApiQuery` bên REST). Verify
sống bằng đúng token đăng nhập thật qua `curl` tới `/graphql` của dev server — phát hiện + sửa 1
bug thật lúc verify (gateway ban đầu dùng keypair riêng, không decode được token đăng nhập thật —
xem `graphql-gateway/README.md`'s mục Auth).

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

Module Go nằm thẳng ở `data-plane/` (`go.mod`, module `github.com/metap-org/waap/data-plane`) —
không còn `data-plane/go/`, không còn `services/*` Rust. Postgres/RabbitMQ dùng chung với
`../../metap`.

```bash
# 0. Hạ tầng dùng chung (từ ../../metap)
(cd ../../metap && docker compose up -d postgres rabbitmq)
export DATABASE_URL=postgres://metap:metap@localhost:5433/metap

# 1. DB + key + tenant + user — chỉ 1 lần cho 1 DB mới (dev-tools nằm ở ../../metap)
cd ../../metap
go run ./cmd/db-migrate                                            # migrations nhúng sẵn trong binary
go run ./cmd/dev-tools gen-keys ../waap/data-plane/keys            # RSA keypair (gitignored)
go run ./cmd/dev-tools provision-tenant <tenantId> schema admin@waap.local <password>   # user đăng nhập portal
go run ./cmd/dev-tools create-user <tenantId> gateway@waap.local <password>             # service account cho gateway
go run ./cmd/dev-tools seed-admin <tenantId> <userId-vừa-tạo>
cd ../waap/data-plane
cp .env.example .env    # điền WAF_SERVICE_EMAIL/WAF_SERVICE_PASSWORD = service account ở trên

# 2a. Cả stack bằng Docker: image production (Dockerfile Go dùng chung, chọn binary bằng SERVICE)
docker compose up -d --build                 # 3 service + graphql-gateway + web (nginx, :8081)

# 2b. Hoặc dev loop: `go run` từ source mount vào container + Vite dev server (:5173)
docker compose -f docker-compose.dev.yml up -d
docker compose -f docker-compose.dev.yml restart zones-service   # sau khi sửa code Go

# 2c. Hoặc chạy thẳng trên host (mỗi lệnh 1 terminal)
export RABBITMQ_URL=amqp://metap:metap@localhost:5672 GRPC_ENABLED=true \
  AUTH_JWT_PRIVATE_KEY_PATH=keys/dev-jwt-private.pem AUTH_JWT_PUBLIC_KEY_PATH=keys/dev-jwt-public.pem
PORT=3000 go run ./cmd/zones-service        # HTTP :3000, gRPC :3001
PORT=3010 go run ./cmd/scanning-service     # HTTP :3010, gRPC :3011
PORT=3020 go run ./cmd/alerting-service     # HTTP :3020, gRPC :3021
UPSTREAM_1_NAME=zones UPSTREAM_1_GRPC_ADDR=localhost:3001 UPSTREAM_1_LOGIN_URL=http://localhost:3000/auth/login \
UPSTREAM_2_NAME=scanning UPSTREAM_2_GRPC_ADDR=localhost:3011 UPSTREAM_2_LOGIN_URL=http://localhost:3010/auth/login \
UPSTREAM_3_NAME=alerting UPSTREAM_3_GRPC_ADDR=localhost:3021 UPSTREAM_3_LOGIN_URL=http://localhost:3020/auth/login \
UPSTREAM_1_SERVICE_EMAIL=... UPSTREAM_1_SERVICE_PASSWORD=...  # (lặp lại cho 2, 3)
  go run ./cmd/waf-graphql-gateway         # :4000
```

Login portal bằng user tạo ở `provision-tenant`. Tài khoản đó là `admin` của tenant (deny-by-default
không áp lên admin); user khác trong tenant cần policy trước khi làm được gì.

Test: `go test ./...` (unit, không cần hạ tầng) và `go test -tags integration ./...` (cần Postgres
ở `localhost:5433`, mỗi test 1 DB riêng do `metap`'s `pkg/testkit` tạo).

### Frontend (`web/`) chạy trên host

```bash
cd web
pnpm install   # link:../../../platform-ui + link:../../../design-system — 2 repo sibling này
               # phải tồn tại ở đúng vị trí, xem package.json
pnpm dev       # http://localhost:5173, proxy /graphql /metadata /auth /admin /health sang backend
```

## `SettingsPage` moved off `/admin/config*` onto GraphQL (2026-09-26)

`metap` core migrated its remaining pre-Phase-90 REST backlog to GraphQL
(`../../metap-docs/docs/roadmap/95-platform-graphql-fields.md`) — `web/src/pages/SettingsPage.tsx`
(the tenant config editor) now calls `tenantConfig`/`setTenantConfig`/`resetTenantConfig`
(`metap-graphql-http::platform_fields`) via `@metap/platform-ui`'s `graphqlFetch`, instead of
`GET/PUT/DELETE /admin/config*`. This page never called `/platform/config*`, so there was nothing
else to migrate here. `/graphql` was already proxied in `vite.config.ts` (this portal's generated
UI has used GraphQL since Phase 93). `tsc -b`/`oxlint`/`prettier --check` clean; not yet verified
through a real browser.

## Docker

- `Dockerfile` (thư mục này) là recipe Go dùng chung cho cả 4 binary — `--build-arg
  SERVICE=zones-service|scanning-service|alerting-service|waf-graphql-gateway`; image runtime là
  `alpine:3` + 1 binary tĩnh (~75MB). `web/Dockerfile` build production frontend (nginx).
- **Mọi build dùng `context: ../../` (root `metap-org`)**: `go.mod` có `replace
  github.com/metap-org/metap => ../../metap`, nên `metap/` phải nằm trong context (vẫn lấy code
  chưa commit ở `../../metap`, không phải 1 ref đã push); `web/` cần `design-system/`,
  `platform-ui/`, `metap-themes/`. `../../.dockerignore` giữ context đó không kéo theo
  `node_modules/`/`.git`/`target/`.
- Không tự chạy Postgres/RabbitMQ — 3 service trỏ vào instance dùng chung của `../../metap` qua
  `host.docker.internal` (cổng host 5433/5672).
- `web/nginx.conf` còn 1 gap ghi trong comment đầu file: `/metadata/entities` (không kèm tên
  entity) chỉ proxy về `zones-service`, không gộp được cả 3 service như dev server của Vite.
- Lịch sử: bản Rust (2026-09-01) có 5 Dockerfile riêng (`services/*/Dockerfile`, gateway generic
  của `metap`) và `cargo watch` cho dev — đã thay hết bằng bản Go ở trên ngày 2026-11-08.
