# 08 — Chi tiết từng module portal

Đào sâu từng module ở `07-portal-features.md` xuống mức màn hình/field/luồng thao tác — vẫn là
phân tích (chưa wireframe, chưa code). Đi sâu vòi ra thêm **6 quyết định nghiệp vụ chưa chốt**
(đánh dấu ⚠️ tại chỗ phát sinh, gom lại ở mục cuối) — nên chốt trước khi build UI những chỗ đó,
tránh phải đổi lại giữa chừng.

---

## 1. Onboarding

**Màn hình**: modal/wizard "Add Zone" (2 bước) + panel verification thường trực trên Zone
Overview cho tới khi verified.

**Bước 1 — Basic info**
| Field | Input | Validate |
|---|---|---|
| `hostname` | text, placeholder `shop.example.com` | FQDN hoặc `*.`+FQDN; check trùng — debounce gọi API lúc gõ để báo "đã tồn tại" ngay, không đợi submit mới biết (dù backend `unique: true` đã chặn cứng ở tầng data) |
| `originAddress` | text, placeholder `10.0.0.5:8080` | format `host:port` |
| `protectionMode` | pre-chọn sẵn `monitor` (không để trống) | đúng nguyên tắc "test trước khi enforce" ở `03` |

Submit → `POST /api/waf.zones` → thành công → redirect Zone Detail, tự mở rộng panel verification.

**Panel verification**
- Chọn method: DNS TXT / HTTP File (đổi qua lại được trước khi verified, dùng lại cùng
  `verificationToken`).
- Hiện chính xác record cần tạo + nút copy.
- Nút "Verify now" (đồng bộ) — loading → thành công (badge "Verified") hoặc lỗi rõ nguyên nhân
  ("Chưa thấy TXT record — DNS có thể cần vài phút để propagate").
- Nếu có job nền (`metap-cron`) tự check định kỳ: hiện "Đang tự động kiểm tra mỗi N phút, lần
  cuối: ...".

**Checklist "sẵn sàng activate"** (thường trực trên Overview khi `status = pending`):
```
☐ Đã cấu hình ít nhất 1 DDoS Policy hoặc Firewall Rule
☐ Domain đã verify
```
Nút "Activate" disabled kèm tooltip nêu đúng điều kiện còn thiếu — không để khách bấm rồi mới
nhận lỗi từ guard.

**Case đặc biệt**: khách tạo Zone rồi bỏ dở (chưa cấu hình, chưa verify) — Zone list phải hiện rõ
trạng thái "chưa hoàn tất onboarding" (badge riêng hoặc filter "Cần hoàn tất setup"), không để
Zone đó chìm trong danh sách như 1 zone bình thường.

---

## 2. Zone management

**Zone List**: bảng group theo apex domain (`06` mục 1), cột Hostname/Status badge/Protection
Mode badge/Config Version/Updated At (relative time), menu ⋮ (View/Pause-Resume/Delete). Search
theo hostname, filter theo status/protectionMode, sort mặc định `-updatedAt` (khớp
`default_sort` đã set trong entity).

**Zone Detail — Overview**
- Header: hostname, status badge, toggle `protectionMode` — chuyển sang `enforce` phải qua modal
  xác nhận ("Traffic thật sẽ bắt đầu bị chặn theo rule hiện có").
- Info: `originAddress` (edit qua modal — **đổi `originAddress` trên zone đang active nên tăng
  `configVersion`** giống mọi thay đổi config khác, vì ảnh hưởng routing thật ở edge).
- Action buttons theo đúng transition hợp lệ của state hiện tại (không hiện nút không hợp lệ):
  `pending`→[Activate*]; `active`→[Pause][Suspend]; `paused`→[Resume][Suspend]; `suspended`→
  không có action (terminal).
- Checklist + verification panel (mục 1) hiện khi còn `pending`.
- "Danger zone": Delete Zone — chỉ Tenant Admin thấy nút (SOC bị ẩn hẳn theo permission `05`),
  confirm bằng gõ lại hostname.

---

## 3. DDoS Policy

Không phải list — **form 1-1** trong tab Zone (0..1 policy/zone). Chưa có → empty state +
"Create". Đã có → form pre-filled, nút Save (PATCH), không có nút "Create" nữa.

| Field | Input | Ghi chú |
|---|---|---|
| `enabled` | toggle, đặt đầu form | tắt thì các field dưới disable nhưng giữ giá trị |
| `sensitivity` | segmented control 4 mức | kèm mô tả ngắn dưới mỗi mức (thấp = ít false-positive, aggressive = dễ chặn nhầm) |
| `requestRateThreshold` | number, đơn vị "request/giây/IP" | validate > 0 |
| `burstWindow` | number, đơn vị "giây" | validate > 0 |
| `action` | dropdown log/challenge/block | |

Lưu lần đầu → app-level tự set `Zone.hasConfig = true` (side-effect đã thiết kế ở `05`) — FE cần
refetch/optimistic-update lại checklist ở Overview sau khi save.

---

## 4. Firewall Rules (WAF / rate-limit / whitelist-blacklist)

Module lớn nhất. 4 sub-view trong 1 tab, cùng entity `FirewallRule`, khác form/filter.

**4.1 Rule List** — filter chip: Tất cả / WAF / Rate Limit / Whitelist-Blacklist. Cột: Priority
(kéo-thả), Name, Type badge, Action badge (allow=xanh lá, block=đỏ, challenge=cam, `log`=xám
"Đang test"), Enabled toggle inline. Nút "+ New rule" → chọn loại trước, vào đúng form tương ứng.

**4.2 Form WAF custom rule**: match-condition builder dạng UI, không bắt gõ JSON:
- Mỗi dòng: Field (dropdown `uri.path`/`header.<tên>`/`body.<jsonpath>`/`sourceIp`/`method`) —
  Operator (tuỳ field: string field có Contains/StartsWith/Regex/Eq/Neq, IP field có
  CidrMatch/Eq/In) — Value.
- Nút "+ AND" / "+ OR group" build `All`/`Any` lồng nhau.
- Preview JSON thu gọn (minh bạch, cho ai muốn xem raw) + toggle "Advanced/JSON mode" cho
  power-user.

**4.3 Form Rate Limit**: form riêng, ẩn hẳn field không liên quan — `threshold`, `window`,
(optional) path scope → app tự build `matchCondition` từ path scope thay vì bắt khách tự viết.

**4.4 Whitelist/Blacklist** (view riêng, đúng `06` mục 5c):
- List lọc `ruleType in [ipFirewall, geoFirewall]`, ẩn field rate-limit.
- Form IP: input IP/CIDR (+ toggle "Thêm nhiều" → textarea dán nhiều dòng → gộp 1 rule
  `matchCondition: {attribute: sourceIp, op: In, value: [...]}`), action.
- Form Geo: multi-select quốc gia (searchable, có cờ), action.
- **Banner cảnh báo rõ ràng**: "Allow sẽ bỏ qua mọi Firewall Rule khác **và cả DDoS Policy** cho
  IP/quốc gia này" — hành vi bypass toàn bộ (`06` mục 5a) đủ quan trọng để không chỉ nằm trong
  docs mà phải hiện ngay trong UI lúc khách tạo whitelist, tránh khách tưởng chỉ bypass WAF.

**✅ Chốt #1 — `priority` không cần unique.** Trong nhóm whitelist/blacklist, thứ tự thật sự
quyết định bởi **action** (allow luôn trước block/challenge), `priority` chỉ tie-break trong cùng
1 nhóm action — xem `06` mục 5a (đã cập nhật). Không cần ép unique, không cần UI cảnh báo trùng số.

**✅ Chốt #2 — `hasConfig` KHÔNG tính theo `enabled`.** Chỉ cần *tồn tại* ít nhất 1
DdosPolicy/FirewallRule là đủ để activate, không cần đang bật. Lý do: bật/tắt 1 rule là thao tác
runtime có SLA đồng bộ xuống edge riêng (10-30s, xem `04-architecture-boundary.md`), không nên
trộn vào điều kiện activate của Zone (2 khái niệm khác tầng: "đã cấu hình" vs "đang thực thi tại
edge lúc này").

---

## 5. Vulnerability Scanning

**Scan Job list** (trong tab Zone): scanType, `schedule` hiển thị human-readable (không phải raw
cron string — vd "Mỗi Thứ 2, 2:00 sáng"), status, lastRunAt, nút "Run now" (chạy ngay ngoài
lịch, không phụ thuộc `schedule`).

**Form tạo Scan Job**: `scanType` (radio 3 lựa chọn kèm mô tả độ sâu quét) — `schedule`: toggle
"Chạy theo lịch"; bật thì hiện cron-builder đơn giản (tần suất Daily/Weekly/Monthly/Custom → sinh
cron string ở app, không bắt khách gõ tay) kèm live preview "Chạy vào ...".

**Scan Finding list**: theo Zone (rollup mọi `ScanJob` của zone đó), cột severity (badge màu,
`critical`/`high` nổi bật), category, endpoint, `remediationStatus`, `firstSeenAt`/`lastSeenAt`.
Finding detail (drawer): description, nút chuyển trạng thái đúng theo state hiện tại
(`open`→[Confirm][False Positive][Accept]; `confirmed`→[Mark Fixed]).

**✅ Chốt #3 — key dedupe = `(scanJobId, category, endpoint)`.** 2 finding cùng bộ 3 này ở 2 lần
quét khác nhau coi là 1 lỗi, chỉ update `lastSeenAt` (không tạo bản ghi mới). Không có entity
`ScanRun` riêng (mỗi lần quét chỉ cập nhật `ScanJob.lastRunAt`) nên đây là cách duy nhất hợp lý
để nhận biết "lần nào" — logic này nằm ở tầng app xử lý kết quả scan (bên thực thi DAST engine,
không phải portal — xem `13-screen-api-map.md`), không phải ở `EntityDefinition`.

**Developer "My Findings"**: lọc theo Zone Developer phụ trách — phụ thuộc gap #6 ở mục Team.

---

## 6. Security Events & Analytics

**Security Event table** (trong tab Zone): occurredAt, triggeredBy + tên rule/policy cụ thể,
action badge, sourceIp, requestPath. Filter theo date range/action/loại trigger/sourceIp.

**✅ Chốt #4 — hướng (b): thêm field denormalize `triggeredByName: String` trên `SecurityEvent`.**
Ghi kèm ngay lúc event được tạo (edge-plane/control-plane biết tên rule lúc match, ghi kèm luôn),
không lookup lại lúc đọc — bắt buộc phải vậy vì `SecurityEvent` là entity volume lớn nhất hệ
thống, N+1 lookup theo hướng (a) không chịu nổi ở quy mô thật. Thêm field này vào `05`'s field
list cho `waf.security_events` khi build tới đó.

**Dashboard** (cross-zone + per-zone riêng trong tab Zone): traffic theo thời gian (area chart
stack theo action), top blocked IP, top endpoint bị tấn công, top rule match nhiều nhất, tỉ lệ
DDoS/WAF. Cần API aggregate riêng ở backend (không tính client-side trên raw event — đã ghi nhận
ở `05`).

---

## 7. Incident Management

**Incident list**: bảng (khuyến nghị bảng cho v1, kanban theo status để sau — P2) — title, zone,
severity, `eventCount`, `assignedTo`, status, updatedAt. Filter status/severity/zone/"của tôi".

**Incident detail**: header (title/severity/status badge) — timeline `SecurityEvent` liên quan
(scroll, phân trang nếu dài) — `assignedTo` picker — nút chuyển state đúng transition hợp lệ
(`open`→[Acknowledge]; `acknowledged`→[Bắt đầu Mitigate]; `mitigating`→[Resolve]).

**Quick-action panel** (đúng workflow #4 ở `03`: "tạo/sửa rule ngay từ context Incident"): nút
"Tạo Firewall Rule chặn IP này" — mở form 4.4 (whitelist/blacklist form) **pre-fill sẵn `sourceIp`
lấy từ event trong incident** — giảm thao tác thay vì bắt SOC tự gõ lại IP đã thấy trong timeline.

**Ghi nhận giới hạn model hiện tại** (không phải gap cần sửa gấp, chỉ nêu rõ): 1 `Incident` chỉ
thuộc 1 `zoneId` — nếu cùng 1 IP tấn công nhiều zone cùng lúc, mỗi zone ra 1 incident riêng, không
gộp cross-zone. Chấp nhận được ở v1 (đúng tinh thần tối giản), note lại nếu sau này cần incident
cross-zone.

---

## 8. Alerting

**Alert Policy list/form**: name, `thresholdCount` + `windowMinutes` — **copy UI nên viết rõ**
"Cảnh báo khi ≥ N event trong M phút, **trên cùng 1 zone**" (không phải tổng dồn nhiều zone —
đúng ý câu ví dụ gốc ở `02`: "trên 1 zone bất kỳ" nghĩa là mỗi zone tự tính riêng, không cộng dồn
— dễ hiểu lầm nếu chỉ hiện số N mà không chú thích). Channels: checkbox Email/Webhook — chọn
Webhook thì hiện thêm input URL ngay dưới.

**Alert Notification log**: bảng read-only — audit đã gửi ai/khi nào/kênh gì/`deliveryStatus`.

**Nút "Send test alert"**: gửi thử ngay, không cần đợi threshold thật xảy ra — xác nhận channel
(đặc biệt webhook URL) hoạt động trước khi tin tưởng.

---

## 9. Team & Permissions

**User list**: email, role, trạng thái (đã invite/active), danh sách domain được gán quyền xem.
**Invite**: email + role select + chọn domain được gán (bắt buộc nếu không phải Tenant Admin —
Tenant Admin mặc định thấy hết zone trong tổ chức mình). **Edit**: đổi role, đổi domain assignment.

**✅ Chốt #5 — không còn là gap riêng của Developer.** Đây là nguyên tắc chung "gán quyền theo
(user, domain)" áp dụng cả Admin Portal lẫn Customer Portal — xem thiết kế đầy đủ ở
`09-access-control.md` (kèm 1 điểm kỹ thuật cần xác nhận khi build module này: bảng gán quyền
cho Admin Portal phải đặt ở tầng platform, không nằm trong DB riêng từng tenant).

---

## 10. Tenant Settings

Thông tin tenant (tên) — form đơn giản. Audit log (ai đổi gì) — khả năng tận dụng
`workflow_events`/outbox có sẵn của `metap` thay vì tự xây audit trail riêng, **cần kiểm tra lúc
build có API generic nào lộ ra được không** (chưa xác nhận). API key cho tích hợp ngoài — không
thấy trong `01-04`, giữ P2 chờ xác nhận có cần không.

---

## Tổng hợp 6 quyết định — tất cả đã chốt (2026-08-30)

| # | Câu hỏi | Trạng thái |
|---|---|---|
| 1 | `FirewallRule.priority` unique hay cho trùng? | ✅ Không cần unique — allow luôn trước block trong nhóm whitelist/blacklist, `priority` chỉ tie-break cùng nhóm action (`06` mục 5a) |
| 2 | `hasConfig` tính cả rule đang tắt (`enabled:false`) không? | ✅ Không — chỉ cần tồn tại record, bật/tắt là chuyện runtime riêng có SLA 10-30s (`04`) |
| 3 | Dedupe `ScanFinding` giữa các lần chạy dùng key `(scanJobId, category, endpoint)` — đúng không? | ✅ Đúng, chốt làm mặc định |
| 4 | `SecurityEvent` có nên thêm field denormalize `triggeredByName`? | ✅ Có — bắt buộc vì volume lớn, N+1 lookup không chịu nổi |
| 5 | Developer↔Zone assignment: JWT context claim hay entity riêng? | ✅ Mở rộng thành nguyên tắc chung 2-portal, xem `09-access-control.md` — còn 1 điểm kỹ thuật (bảng gán quyền Admin Portal đặt ở tầng platform) cần xác nhận lúc build module Team |
| 6 | Wildcard hostname có cần hỗ trợ không? | ✅ Có — hỗ trợ cả wildcard lẫn khai từng subdomain riêng, khách tự chọn |

Đã build + test qua API thật (`waf.zones`): mục 2 (guard `hasConfig`) + guard verification cùng
`All()` — xem `05-metap-technical-mapping.md`.
