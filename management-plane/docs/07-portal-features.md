# 07 — Portal Feature Breakdown (BA view)

`03-personas-workflows.md` mô tả *luồng nghiệp vụ* (persona làm gì, theo thứ tự nào).
`02-domain-model.md` mô tả *dữ liệu*. Tài liệu này là lớp còn thiếu ở giữa: **portal cần màn
hình/tính năng cụ thể nào** để 2 tài liệu kia thực sự dùng được — sitemap + feature list, chưa
phải wireframe/UI design. Ưu tiên đánh theo v1 scope đã chốt ở `01-product-vision.md`:
**P0** = phải có để 4 trụ cột v1 chạy được, **P1** = nên có nhưng portal vẫn dùng được nếu thiếu,
**P2** = ghi nhận, không làm ở v1 (đa số rơi vào đúng danh sách "ngoài scope v1" của `01`).

## Sitemap (navigation chính)

```
Portal (sau khi login)
├─ Dashboard (tổng quan cross-zone)
├─ Zones                          → list, mỗi Zone có sub-nav riêng:
│   └─ [Zone detail]
│       ├─ Overview
│       ├─ DDoS Policy
│       ├─ Firewall Rules  (bao gồm view con "Whitelist/Blacklist")
│       ├─ Vulnerability Scans
│       └─ Security Events (scoped riêng zone này)
├─ Incidents                      (= "Lịch sử tấn công", cross-zone, xem `10-attack-visibility.md`)
├─ Analytics                      (dashboard WAF/DDoS, cross-zone — xem `10`)
├─ Alerting                       (Alert Policy + Notification log, cross-zone)
├─ Team & Permissions             (xem `09-access-control.md`)
├─ Billing & Plans                (xem `12-billing-plans.md` — có thể để phase sau, không chặn v1)
└─ Settings (tenant)
```
Zone là đơn vị điều hướng trung tâm — đa số CRUD (DdosPolicy/FirewallRule/ScanJob) nằm *trong*
1 Zone, không phải list phẳng toàn tenant (dù vẫn cần lọc theo Zone ở các view cross-zone).
`Onboarding` (mục 1) còn cần thêm bước phân giải DNS/IP — xem `11-onboarding-dns-resolution.md`.

## 1. Onboarding

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Wizard "Add Zone" | Tenant Admin | Nhập `hostname` + `originAddress` → validate format (FQDN/wildcard) → tạo Zone (`status: pending`) | P0 |
| Hướng dẫn verify domain | Tenant Admin | Hiện TXT record hoặc file cần tạo (tuỳ `verificationMethod` chọn), nút copy-to-clipboard | P0 (phụ thuộc quyết định ở `06` mục 2) |
| Nút "Verify now" | Tenant Admin | Trigger check đồng bộ, cập nhật `verificationStatus` ngay thay vì chờ job định kỳ | P0 |
| Empty state tenant mới | Tenant Admin | Chưa có Zone nào → CTA "Add your first zone", không phải bảng trống trơn | P1 |
| Checklist "sẵn sàng activate" | Tenant Admin | Hiện rõ 2 điều kiện activate (đã có DdosPolicy/FirewallRule? đã verify?) dạng checklist, không phải lỗi khi bấm activate mới biết thiếu gì | P0 |

## 2. Zone management

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Zone list | Tenant Admin, SOC, Viewer | Bảng: hostname, status badge, protectionMode, configVersion, updatedAt — search + filter theo status | P0 |
| Group theo apex domain | mọi role | Gộp hiển thị `shop./api./www.example.com` dưới 1 nhóm `example.com` (UX-only, xem `06` mục 1) | P1 |
| Zone detail — Overview | mọi role | originAddress, status, protectionMode toggle, nút pause/resume/suspend (theo quyền), verification panel | P0 |
| protectionMode toggle | Tenant Admin, SOC | Chuyển monitor ↔ enforce, có modal cảnh báo khi chuyển sang enforce lần đầu ("traffic thật sẽ bị block") | P0 |
| Sync status với edge | mọi role | Hiện `configVersion` hiện tại + (nếu control-plane expose được) đã đồng bộ xuống edge chưa — **phụ thuộc control-plane, có thể chưa làm được ở giai đoạn chỉ có management-plane** | P1 |
| Xoá Zone | Tenant Admin | Có, kèm confirm modal — SOC **không** có quyền này (đã chốt ở `05` permission) | P1 |

## 3. DDoS L7 Policy

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Form DDoS Policy (trong tab Zone) | Tenant Admin, SOC | 0..1 policy/zone → create-or-edit cùng 1 form (không phải list), sensitivity dropdown, threshold/burstWindow input, action dropdown, enabled toggle | P0 |
| Giải thích sensitivity | mọi role | Tooltip/help text mô tả trade-off (cao = dễ false-positive) ngay cạnh dropdown — tránh khách chọn `aggressive` mù quáng | P1 |

## 4. Firewall Rules (WAF / rate-limit / IP-geo hợp nhất)

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Rule list (trong tab Zone) | Tenant Admin, SOC | Bảng: name, ruleType badge, priority, action, enabled — sort theo priority | P0 |
| Reorder priority (kéo-thả) | Tenant Admin, SOC | Kéo thả đổi thứ tự evaluate, save = bulk update `priority` | P1 (P0 nếu không có thì vẫn sửa `priority` bằng tay qua form được, chỉ kém tiện) |
| Form WAF custom rule | Tenant Admin, SOC | Match-condition builder dạng UI (field/operator/value theo hàng, nhóm AND/OR) — không bắt khách viết JSON tay | P0 |
| — chế độ "Advanced/JSON" | Tenant Admin, SOC | Toggle sang textarea JSON thô cho power-user/debug | P1 |
| Form Rate Limit | Tenant Admin, SOC | Form riêng đơn giản: threshold + window + (optional) path scope — ẩn hẳn field `matchCondition` phức tạp | P0 |
| **View Whitelist/Blacklist riêng** | Tenant Admin, SOC | List-view lọc sẵn `ruleType in [ipFirewall, geoFirewall]`, ẩn field không liên quan (`rateLimitThreshold`...) — xem `06` mục 5c | P0 |
| — Form IP allow/block | Tenant Admin, SOC | Chỉ hỏi IP/CIDR + action, tự build `matchCondition` | P0 |
| — Bulk add IP | Tenant Admin, SOC | Dán nhiều IP/CIDR 1 lúc → 1 rule gộp (`In` operator) | P1 |
| — Form Geo allow/block | Tenant Admin, SOC | Country multi-select + action | P0 |
| Đánh dấu rule đang "testing" | Tenant Admin, SOC | Badge trực quan cho rule có `action = log` (đang test, chưa enforce thật) — tận dụng cơ chế sẵn có ở `06` mục 4 | P1 |
| Link rule → SecurityEvent đã match | SOC | Từ 1 rule, xem nhanh các event gần nhất match rule này (filter `triggeredById`) — đúng workflow #2 ở `03` | P1 |
| Rule template/managed ruleset | — | **P2 — ngoài scope v1** theo `01-product-vision.md` | P2 |

## 5. Vulnerability Scanning

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Scan Job list (trong tab Zone) | Tenant Admin, Developer | scanType, schedule (hiển thị human-readable, vd "Mỗi thứ 2, 2h sáng" thay vì raw cron string), status, lastRunAt | P0 |
| Form tạo Scan Job | Tenant Admin, Developer | scanType select + schedule picker (cron builder UI, không bắt gõ cron string tay) hoặc để trống = chỉ chạy tay | P0 |
| Nút "Run now" | Tenant Admin, Developer | Trigger chạy ngay ngoài lịch | P0 |
| Scan Finding list | Developer, SOC | Theo ScanJob hoặc rollup theo Zone — filter/sort theo severity, `remediationStatus` | P0 |
| Finding detail + transition | Developer | description, endpoint, nút chuyển `remediationStatus` (confirm/fixed/falsePositive/accepted) | P0 |
| "My findings" (assigned zones) | Developer | Lọc riêng finding thuộc Zone mình phụ trách — cần cơ chế gán Developer↔Zone (xem mục 8) | P1 |
| Badge severity nổi bật | mọi role | `critical`/`high` có màu cảnh báo rõ trong list, không chỉ text | P1 |

## 6. Security Events & Analytics

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Security Event table (per-zone) | SOC | zoneId (ngầm định, đang ở trong Zone), triggeredBy, action, sourceIp, requestPath, occurredAt — filter theo action/rule | P0 |
| Dashboard tổng quan (cross-zone) | Tenant Admin, Viewer | Traffic theo thời gian (chart), top blocked IP, top endpoint bị tấn công, top rule match nhiều nhất | P0 |
| Dashboard per-zone | mọi role | Cùng loại chart nhưng scoped 1 zone (nằm trong tab Zone) | P1 |
| Export/report | Tenant Admin | Xuất CSV/PDF báo cáo — **P2**, không thấy nhắc trong docs 01-04, không tự thêm scope nếu bạn không cần | P2 |

**Lưu ý kỹ thuật**: `SecurityEvent` volume lớn — list/dashboard này cần pagination + có thể cần
pre-aggregation ở backend trước (đã ghi nhận ở `05-metap-technical-mapping.md` mục cuối), portal
chỉ tiêu thụ API, không tự tính toán aggregate ở FE.

## 7. Incident Management

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Incident list | SOC, Tenant Admin, Viewer | Bảng hoặc board theo `status` (open/acknowledged/mitigating/resolved), filter severity/zone | P0 |
| Incident detail | SOC | title, severity, `eventCount`, timeline `SecurityEvent` liên quan, `assignedTo` | P0 |
| Nút chuyển trạng thái | SOC | acknowledge → mitigating → resolved (transition buttons, đúng workflow) | P0 |
| "Assign to me" | SOC | Gán nhanh `assignedTo` = user hiện tại | P1 |
| Quick-create rule từ Incident | SOC | Từ context Incident, mở form tạo `FirewallRule`/sửa `DdosPolicy` ngay (đúng workflow #4 ở `03`: "tạo/sửa rule ngay từ context Incident") — không bắt SOC rời trang đi tìm zone | P1 |

## 8. Alerting

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Alert Policy list/form | Tenant Admin | name, threshold (count + window), channels (email/webhook multi-select), enabled | P0 |
| Alert Notification log | Tenant Admin, SOC | Audit: đã gửi ai, khi nào, kênh gì, `deliveryStatus` — đọc-only | P1 |
| Nút "Send test alert" | Tenant Admin | Gửi thử 1 notification để xác nhận channel hoạt động (vd webhook URL đúng) | P1 |

## 9. Team & Permissions

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| User list + invite | Tenant Admin | Mời user qua email, gán role (Tenant Admin/SOC/Developer/Viewer) | P0 |
| Đổi role user | Tenant Admin | | P0 |
| **Gán Developer ↔ Zone cụ thể** | Tenant Admin | `03-personas-workflows.md` nói Developer "xem ScanFinding của zone mình phụ trách" — **cần cơ chế gán phạm vi zone cho Developer, chưa có trong domain model hiện tại** (permission hiện chỉ theo role, chưa theo role+zone). Gap cần quyết định: thêm bảng gán user↔zone, hay field `assignedDeveloperIds` trên Zone? | P1, nhưng **cần chốt thiết kế trước khi build tab Developer** |
| Xoá/vô hiệu hoá user | Tenant Admin | | P1 |

## 10. Tenant Settings

| Tính năng | Vai trò | Mô tả | Ưu tiên |
|---|---|---|---|
| Thông tin tenant (tên, ...) | Tenant Admin | | P1 |
| Audit log (ai đổi gì) | Tenant Admin, SOC | `metap`'s `workflow_events`/outbox có thể generic hoá thành audit trail — cần kiểm tra khi build có API generic lộ ra được không | P1 |
| API key (nếu cần tích hợp ngoài) | Tenant Admin | **Không thấy trong docs 01-04** — không tự thêm scope, hỏi lại nếu cần | P2 (chưa xác nhận có cần) |

## Cross-cutting (áp dụng mọi module)

- Toast/notification cho mọi action CRUD (tạo/sửa/xoá/transition thành công hay lỗi field-level).
- Loading/empty/error state nhất quán mọi list-view (nhiều màn hình ở trên dùng chung 1 danh sách
  generic list/detail — hợp với cách `metap`'s `platform-react` được mô tả là "reusable FE
  primitives", nên phần lớn các list/form ở trên có thể dùng chung 1 bộ component thay vì code
  riêng từng cái).
- Quyền theo role ẩn/hiện đúng nút bấm (không chỉ chặn ở API — SOC không thấy nút "Delete Zone"
  thay vì thấy nút rồi bị 403).

## Ngoài scope portal v1 (nhắc lại, khớp `01-product-vision.md`)

Không làm: Bot management UI, API schema validation UI, managed ruleset toggle, TLS/certificate
UI, Page Shield, Attack Surface Management UI, billing.
