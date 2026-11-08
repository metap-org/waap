# metap-demo-waf

Sản phẩm demo **WAAP** (Web Application & API Protection, kiểu Cloudflare) — build portal nhanh
trên nền `metap` (`../metap`), phần thực thi bảo vệ thật tách riêng. Repo chia theo 4 plane của
[ADR-0001](docs/architecture/09-adr/0001-four-planes.md) (thư mục `management-plane/` trước 2026-11-08
tên là `data-plane/`):

| Thư mục | Vai trò | Trạng thái |
|---|---|---|
| [`management-plane/`](management-plane/) | Nơi giữ dữ liệu/config nguồn — portal nghiệp vụ (Zone, DDoS policy, firewall rule, vulnerability scan, incident, alert), build trên `metap` | Backend: cả 9 entity pillar 1-4 chạy được, test qua API thật. Frontend (`web/`, `@metap/platform-ui`): generic list/form/detail/workflow UI chạy được, test qua browser thật — còn thiếu portal IA riêng theo `docs/07` + logic custom (DNS verify, dashboard, correlation, alert gửi thật) |
| `control-plane/` | Lấy config từ `management-plane`, tính toán (compile) thành rule-set, đẩy (distribute) xuống `edge-plane` — worker nền, không UI | Chưa bắt đầu |
| `edge-plane/` | Thực thi mitigation thật tại biên (nhận traffic, evaluate rule, chặn/challenge) — hệ thống riêng, hiệu năng cao | Chưa bắt đầu |
| [`analytics-plane/`](analytics-plane/) | Log và số liệu traffic: nhận security event từ control-plane, lưu ClickHouse, query API cho portal và alerting — không dùng `metap` | Bản đầu (2026-11-08): `analytics-api`, xem README của nó |
| [`devtools/`](devtools/) | Không phải plane: thứ giả lập thế giới bên ngoài cho dev/staging (`dns-mock`) | Dùng được |

Xem `management-plane/docs/04-architecture-boundary.md` cho luồng dữ liệu giữa các plane (config
đi xuống, telemetry đi lên) và lý do tách theo hướng này.

Bắt đầu đọc từ `management-plane/docs/01-product-vision.md`.
