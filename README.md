# metap-demo-waf

Sản phẩm demo **WAAP** (Web Application & API Protection, kiểu Cloudflare) — build portal nhanh
trên nền `metap` (`../metap`), phần thực thi bảo vệ thật tách riêng. Repo chia theo 4 plane của
[ADR-0001](docs/architecture/09-adr/0001-four-planes.md). Từ 2026-11-14 plane không còn là thư mục:
mỗi service là một thư mục phẳng ở gốc repo, tên mang tiền tố plane — `m-` management, `c-` control,
`e-` edge, `a-` analytics (thư mục `management-plane/` trước 2026-11-08 tên là `data-plane/`):

| Thư mục | Plane | Vai trò |
|---|---|---|
| `m-management/` | Management | Một service backend portal (domain `zones`, `scanning`, `alerting`, `billing`, `analytics`; `zones/rules/` là logic thuần) — build trên `metap`, portal gọi thẳng, không gateway |
| `m-web/` | Management | Customer Portal (`@metap/platform-ui`) |
| `m-deploy/` | Management | Dockerfile, docker-compose, `.env.example`, key JWKS cho các `m-*` service |
| `c-config-distributor/` | Control | Lấy config từ management, compile thành rule-set, đẩy xuống Redis cho edge; nhận telemetry từ edge — worker nền, không UI |
| `e-edge/` | Edge | Thực thi mitigation thật tại biên (evaluate rule, chặn/challenge) — module Go riêng, chỉ import `metap/pkg/kit` |
| `a-api/` | Analytics | Nhận security event từ control, lưu ClickHouse, query API cho portal và alerting — module Go riêng, chỉ import `metap/pkg/kit` |

Các service `m-*` và `c-*` chung một Go module ở gốc repo (`go build ./...`, `go test ./...` từ gốc);
`e-edge` và `a-api` là module riêng.

Xem `docs/management/04-architecture-boundary.md` cho luồng dữ liệu giữa các plane (config
đi xuống, telemetry đi lên) và lý do tách theo hướng này.

Bắt đầu đọc từ `docs/management/01-product-vision.md`.
