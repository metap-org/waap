# 13 — Màn hình ↔ API: cái nào "phang metap là xong", cái nào phải tự code

Dừng code entity, quay lại phân tích thuần — nhưng lần này đi tới tận API cho từng màn hình
(`07`/`08` đã có màn hình + field, còn thiếu lớp API). Mục tiêu chính của tài liệu này: trả lời
thẳng câu hỏi "cái nào chỉ cần đăng ký `EntityDefinition` là `metap` tự sinh API xong, cái nào
vẫn phải tự viết code" — vì **không phải mọi màn hình đều tự động cả**.

## Tóm tắt trước — để khỏi ảo tưởng "phang metap là xong" mọi thứ

`metap` tự sinh generic REST cho **mọi entity đăng ký**: `GET/POST /api/{entity}`,
`GET/PATCH/DELETE /api/{entity}/{id}`, `POST /api/{entity}/{id}/transitions/{action}`. Cái này
đúng là "phang vào là chạy" — không cần viết 1 dòng handler nào, thấy đã test trực tiếp qua API
thật với `waf.zones`/`waf.ddos_policies`/`waf.firewall_rules`.

**Nhưng** khoảng 1/3 tính năng đã phân tích ở `06-12` **không phải CRUD trên 1 entity** — cần
code thêm thật sự, `metap` không tự làm giúp:

| Nhóm cần code thêm | Vì sao |
|---|---|
| DNS lookup, verify domain, test origin (mục 1, `11`) | Gọi ra ngoài (DNS resolver, HTTP client) — không phải đọc/ghi record |
| Dashboard/Analytics (mục 6) | Cần aggregate (SUM/COUNT/GROUP BY) — API generic chỉ trả list record thô, không tự tính tổng hợp |
| Gộp `SecurityEvent` → `Incident` (mục 7) | Thuật toán correlation — logic nghiệp vụ thật, không phải CRUD |
| Gửi Alert thật + test alert (mục 8) | Cần đọc điều kiện, quyết định lúc nào gửi, gọi email/webhook thật |
| Gán quyền theo (user, domain) (mục 9, `09`) | `PolicyCondition` không tự join bảng — cần bước resolve riêng |
| Quota theo gói + thanh toán thật (mục 12, `12`) | Đếm quan hệ (không tự làm được) + gọi cổng thanh toán ngoài |

Phần còn lại (đa số CRUD cấu hình: Zone/DdosPolicy/FirewallRule/ScanJob-config/ScanFinding-status/
Incident-status/AlertPolicy/Plan/Subscription) — đúng là chỉ cần đăng ký entity, generic API tự
có.

**Sửa lại 1 chỗ đã nói sai (2026-08-30)**: bản đầu liệt "chạy scan thật" vào danh sách trên — sai.
Việc *thực thi* quét lỗ hổng (DAST engine) không phải nghiệp vụ portal, giống hệt cách `edge-plane`
tách khỏi `management-plane` để thực thi WAF/DDoS thật (`04-architecture-boundary.md`) — portal chỉ cần
cấu hình `ScanJob` + hiện `ScanFinding`, đúng CRUD generic bình thường, xem mục 5 bên dưới. Ai
thực thi việc quét là câu hỏi kiến trúc riêng (thuộc `control-plane` hay 1 service quét độc lập,
chưa quyết), không chặn/không thuộc phân tích portal.

---

## 1. Onboarding

| Màn hình/hành động | API | Loại |
|---|---|---|
| Submit "Add Zone" | `POST /api/waf.zones` | **Generic** |
| Check hostname trùng lúc gõ | dựa vào lỗi `unique_violation` khi submit, hoặc `GET /api/waf.zones?filter=hostname:X` | **Generic** |
| Tra DNS hiện tại (gợi ý originAddress) | `POST /api/waf.zones/{id}/dns-lookup` (hoặc endpoint độc lập trước khi Zone tồn tại) | **Custom** — DNS resolver, không liên quan CRUD |
| Nút "Verify now" | `POST /api/waf.zones/{id}/verify` — check TXT/HTTP file rồi tự cập nhật `verificationStatus` | **Custom** (bước check), nhưng ghi kết quả xuống có thể tái dùng `PATCH` nội bộ |
| Panel "Trạng thái DNS routing" | `POST /api/waf.zones/{id}/dns-routing-check` | **Custom** |
| Test Origin Connection | `POST /api/waf.zones/{id}/test-origin` | **Custom** |
| Checklist sẵn sàng activate | đọc từ `GET /api/waf.zones/{id}` (field `hasConfig`/`verificationStatus`), tính ở FE | **Generic** |
| Nút Activate | `POST /api/waf.zones/{id}/transitions/activate` | **Generic** (đã test thật) |

## 2. Zone management

| Màn hình/hành động | API | Loại |
|---|---|---|
| Zone list | `GET /api/waf.zones?sort=-updatedAt&filter=...` | **Generic** |
| Zone detail | `GET /api/waf.zones/{id}` | **Generic** |
| Sửa `originAddress`, `protectionMode` | `PATCH /api/waf.zones/{id}` | **Generic** |
| Pause/Resume/Suspend | `POST /api/waf.zones/{id}/transitions/{action}` | **Generic** |
| Xoá Zone | `DELETE /api/waf.zones/{id}` | **Generic** |
| "Đã sync xuống edge chưa" | so `configVersion` — cần hỏi `control-plane`, `management-plane` không tự biết | **Custom + cross-plane**, có khi không làm được ở giai đoạn chỉ có management-plane (đã note P1 ở `07`) |

## 3. DDoS Policy

| Màn hình/hành động | API | Loại |
|---|---|---|
| Xem/tạo/sửa (0..1 theo zone) | `GET /api/waf.ddos_policies?filter=zoneId:X` rồi `POST` (chưa có) hoặc `PATCH` (đã có) | **Generic** — logic "create-or-edit" chỉ là if/else ở FE, không phải API riêng |

## 4. Firewall Rules

| Màn hình/hành động | API | Loại |
|---|---|---|
| List (lọc theo `ruleType`) | `GET /api/waf.firewall_rules?filter=zoneId:X,ruleType:Y&sort=priority` | **Generic** |
| Tạo/sửa WAF/RateLimit/IP/Geo rule | `POST`/`PATCH /api/waf.firewall_rules` (`matchCondition` build ở FE, gửi JSON) | **Generic** |
| Bật/tắt `enabled` | `PATCH` | **Generic** |
| Bulk add IP (dán nhiều dòng) | vẫn 1 `POST` (gộp thành 1 rule `In` array) | **Generic** |
| Reorder kéo-thả priority | N lần `PATCH` (không atomic) | **Generic nhưng không atomic** — nếu cần atomic phải tự thêm 1 endpoint bulk (chưa rõ `metap-cron`'s `BulkQueryAction` có dùng được ngoài cron không, cần kiểm tra lúc build) |

## 5. Vulnerability Scanning

| Màn hình/hành động | API | Loại |
|---|---|---|
| ScanJob list/create/edit | CRUD `waf.scan_jobs` | **Generic** |
| Nút "Run now" | `POST /api/waf.scan_jobs/{id}/transitions/run` | **Generic** — portal chỉ cần đổi state sang `queued`, hết việc của portal |
| ScanFinding list/detail | CRUD `waf.scan_findings` | **Generic** |
| Chuyển `remediationStatus` | `POST /api/waf.scan_findings/{id}/transitions/{action}` | **Generic** |

**Ngoài phạm vi portal** (không phải màn hình/API của management-plane, ghi chú lại để không quên):
việc thực thi quét thật (DAST engine đọc job `queued`, chạy công cụ quét, ghi `ScanFinding` qua
`CrudService`/gRPC — bao gồm cả logic dedupe theo `scanJobId+category+endpoint`, doc `08` quyết
định #3) thuộc về 1 service/plane thực thi riêng, cùng loại câu hỏi kiến trúc như `edge-plane`
thực thi WAF — không phải phân tích nghiệp vụ portal.

## 6. Security Events & Analytics

| Màn hình/hành động | API | Loại |
|---|---|---|
| Security Event list (theo zone) | `GET /api/waf.security_events?filter=zoneId:X` | **Generic** (ghi thì edge-plane gọi gRPC `RecordService.Create`, cũng generic — đã xác nhận ở `05`) |
| Dashboard (traffic chart, top rule, top IP...) | cần 1 endpoint tổng hợp riêng, vd `GET /api/analytics/waf.security_events/summary?zoneId=X&range=7d` | **Custom** — generic list chỉ trả record thô, không SUM/GROUP BY được |
| Access Log (mọi request, không chỉ bị chặn) | chưa quyết định có làm không (`10`) | **Custom, có thể không nằm ở management-plane** |

## 7. Incident Management

| Màn hình/hành động | API | Loại |
|---|---|---|
| Incident list/detail | CRUD `waf.incidents` | **Generic** |
| acknowledge/mitigate/resolve | `POST /api/waf.incidents/{id}/transitions/{action}` | **Generic** |
| Gán `assignedTo` | `PATCH` | **Generic** |
| Tự động **tạo** Incident từ nhiều `SecurityEvent` | job (`metap-cron` `OnRecordEvent` trigger, đúng cơ chế có sẵn) + **thuật toán gộp tự viết** | **Custom** — cơ chế trigger có sẵn, nhưng "gộp thế nào là 1 incident" là logic nghiệp vụ 100% tự viết |
| Quick-create rule từ Incident (pre-fill IP) | `POST /api/waf.firewall_rules` với data pre-fill sẵn ở FE | **Generic** (chỉ là UX, không phải API riêng) |

## 8. Alerting

| Màn hình/hành động | API | Loại |
|---|---|---|
| AlertPolicy CRUD | CRUD `waf.alert_policies` | **Generic** |
| AlertNotification log (đọc) | `GET /api/waf.alert_notifications?filter=alertPolicyId:X` | **Generic** |
| Gửi alert thật khi đủ điều kiện | `metap-cron` job `target_type: Email/Webhook` | **Nửa generic** — cơ chế gửi có sẵn (không code SMTP/HTTP client tay), nhưng phải tự cấu hình đúng trigger + đọc `AlertPolicy.thresholdCount/windowMinutes` để quyết định khi nào bắn |
| Nút "Send test alert" | endpoint riêng bắn 1 lần bỏ qua điều kiện | **Custom** |

## 9. Team & Permissions

| Màn hình/hành động | API | Loại |
|---|---|---|
| User list/invite/đổi role | `metap` có sẵn route admin (`POST /admin/users` xác nhận có trong nghiên cứu trước) | **Có sẵn ở metap** (không phải entity CRUD tự đăng ký, nhưng cũng không phải tự viết) |
| Gán domain theo user (`09`) | entity mới + logic resolve riêng (JWT context hoặc bảng gán quyền platform-level) | **Custom** — đã note rõ ở `09`, không phải CRUD đơn giản |

## 10-12. Tenant Settings / Billing

| Màn hình/hành động | API | Loại |
|---|---|---|
| Plan/Subscription CRUD | CRUD (entity mới, admin quản lý Plan, tenant có Subscription) | **Generic** |
| Usage vs limit (progress bar) | cần đếm số zone/rule hiện có so với `Plan` | **Custom** — đúng vấn đề đếm quan hệ đã nhắc ở `12` |
| Chặn tạo Zone khi vượt quota | check trước khi gọi `POST /api/waf.zones` | **Custom** (1 bước validate thêm trước khi cho generic API chạy) |
| Thanh toán thật | tích hợp cổng thanh toán (chưa chọn provider) | **Custom, ngoài `metap` hoàn toàn** |
| Audit log | có thể tận dụng `workflow_events`/outbox nếu có API lộ ra | **Chưa xác nhận** — kiểm tra lúc build |
