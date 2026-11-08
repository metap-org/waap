# 04 — Architecture Boundary: 3 Plane của `metap-demo-waf`

Ghi lại đúng quyết định đã thống nhất, để không lẫn lộn phạm vi khi vào chi tiết kỹ thuật. Repo
`metap-demo-waf` chia 3 thư mục top-level theo đúng 3 vai trò dưới đây — mỗi thư mục 1 codebase/
deploy cycle riêng.

## Ranh giới

- **`management-plane/` (thư mục này) = nơi giữ dữ liệu/config nguồn (source of truth).** Portal cấu
  hình nghiệp vụ (Zone, DdosPolicy, FirewallRule, ScanJob/Finding, Incident, Alert) — build trên
  `metap`, hưởng sẵn CRUD/permission/workflow generic, RBAC matrix, low-code entity nếu cần mở
  rộng nhanh. Đây là nơi duy nhất khách hàng/SOC tương tác trực tiếp qua UI.
- **`control-plane/` = lấy config từ `management-plane`, tính toán, đẩy xuống `edge-plane`.** Không có
  UI, không phải CRUD — là 1 (hoặc vài) worker/service chạy nền, subscribe thay đổi từ
  `management-plane`, biên dịch (compile) thành rule-set mà `edge-plane` hiểu được, rồi phân phối
  (distribute) xuống. Vai trò tương đương "Quicksilver" của Cloudflare hay control-plane API kiểu
  xDS (Envoy) — chỉ khác là pull-from-cache thay vì push-stream (xem chi tiết bên dưới).
- **`edge-plane/` = hệ thống riêng, tự code, thực thi mitigation thật.** Nhận traffic, evaluate
  rule, chặn/challenge request — bài toán hiệu năng cao, latency thấp, không phải CRUD, không hợp
  với metadata-driven approach của `metap`. Chỉ đọc config đã được `control-plane` tính toán sẵn,
  không bao giờ tự đọc thẳng `management-plane`.

## Luồng dữ liệu

**Config đi xuống (`management-plane` → `control-plane` → `edge-plane`):**

```
Zone/DdosPolicy/FirewallRule đổi (qua portal, management-plane)
  → metap outbox (transaction cùng lúc ghi DB — không mất event kể cả RabbitMQ down)
  → outbox-publisher → RabbitMQ
  → control-plane: 1 worker (kiểu notification-worker của metap, tên gợi ý
    "waf-config-distributor") subscribe event, build rule-set đã tính toán sẵn cho từng Zone,
    ghi vào Redis/DragonflyDB (metap-cache đã có RedisCache, không cần code cache mới)
  → edge-plane đọc thẳng Redis (latency thấp, nhiều edge node đọc cùng 1 key không tải lên
    management-plane)
```

`Zone.configVersion` (định nghĩa ở `management-plane`) tăng mỗi lần có thay đổi — `control-plane` và
`edge-plane` đều so `configVersion` để biết cache có đang stale không, không cần đoán qua
timestamp.

**SLA đồng bộ config xuống edge: 10-30 giây** — từ lúc khách bấm bật/tắt 1 rule/policy (hoặc đổi
`protectionMode`) ở `management-plane` tới lúc `edge-plane` thực sự áp dụng thay đổi đó. Vì có độ trễ
này (kiến trúc pull-based/cache, không phải push tức thời), các guard/validation ở `management-plane`
(vd `Zone.activate`) **không nên** coi việc bật/tắt 1 rule là 1 thao tác "đồng bộ, có kết quả tức
thời" — đó là quyết định đã áp dụng khi thiết kế guard `hasConfig` (chỉ check *có tồn tại*
policy/rule, không check *đang bật*, xem `08-module-detail-specs.md` quyết định #2).

**Telemetry đi lên (`edge-plane` → `control-plane`/`management-plane`):** `SecurityEvent` (số lượng
lớn, near real-time) — cách ghi vào `management-plane` chưa chốt kỹ thuật, 2 hướng khả dĩ để bàn sau:
1. `edge-plane` gọi thẳng `metap-grpc`'s generic `RecordService.Create` trên `management-plane` (đã có
   sẵn, generic mọi entity) — đơn giản, nhưng N edge node × traffic lớn → nhiều lời gọi gRPC nhỏ
   lẻ tới `management-plane`.
2. `edge-plane` tự batch/buffer, gửi qua `control-plane` trước (aggregate/dedupe ở đó), rồi
   `control-plane` mới ghi xuống `management-plane` theo batch lớn hơn — đúng vai trò trung gian
   `control-plane` đã có sẵn cho chiều xuống, tận dụng luôn cho chiều lên thay vì thêm 1 đường
   ghi trực tiếp `edge-plane` → `management-plane` khác.

**Đã chốt hướng 2 (2026-09-04, Phase 72 — `../../../metap-docs/docs/roadmap/72-control-edge-planes.md`).**
Trước đó mục này ghi "chưa chốt hướng nào"; giữ nguyên phần phân tích ở trên vì lý do chọn không
đổi: hướng 2 giữ đúng nguyên tắc "`edge-plane` không bao giờ nói chuyện trực tiếp với `management-plane`"
đã chốt ở trên, và không đẩy N edge node × traffic thật thành write nhỏ lẻ vào DB của portal.

Cách cài đặt giữ nguyên nguyên tắc "mọi write đi qua `CrudService`" mà mục này lo: `control-plane`
ghi `SecurityEvent` bằng **đúng route CRUD thường** (`POST /api/waf.security_events`), nên
validation/permission/outbox vẫn áp dụng đủ — batching chỉ làm ít request hơn, không phải đường
vòng qua platform. Đánh đổi đã biết, ghi rõ ở `control-plane/waf-config-distributor/src/ingest.rs`:
buffer nằm trong bộ nhớ và không ack, nên process chết là mất số event đang xếp hàng. Chấp nhận cho
v1 (telemetry, không phải dữ liệu tính tiền); muốn chắc thì cần 1 queue bền giữa edge và portal —
việc riêng, không phải chỉnh nhỏ.

**Lưu ý cách quyết định này ra đời**: chốt trong phiên code (chủ dự án đã nói trước "không cần hỏi
quyết định"), không phải qua một vòng bàn riêng như các quyết định khác trong tài liệu này. Đảo lại
rẻ — `edge-plane` chỉ biết đúng 1 URL ingest.

## Vì sao tách 3 plane

- `management-plane` cần đổi nhanh theo nghiệp vụ (thêm field, đổi workflow, thêm loại rule) — đúng thế
  mạnh `metap` (metadata-driven, low-code).
- `edge-plane` cần ổn định, hiệu năng cao, không đổi theo từng lần khách chỉnh 1 field UI — không
  nên chung 1 codebase/deploy cycle với `management-plane`.
- `control-plane` đứng giữa để 2 phía không cần biết nội bộ của nhau: `edge-plane` không cần biết
  schema Postgres/JSONB của `metap`, `management-plane` không cần biết edge-plane có bao nhiêu node/đặt
  ở đâu. Redis/DragonflyDB (`control-plane` ghi, `edge-plane` đọc) tận dụng cái đã có sẵn trong
  `metap` (`metap-cache`) thay vì xây thêm hạ tầng cache mới.
