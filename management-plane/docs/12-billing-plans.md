# 12 — Gói giá, Subscription, giới hạn theo gói

Mảng nghiệp vụ hoàn toàn mới, chưa xuất hiện ở `01-09` — `01-product-vision.md` chưa từng nhắc
billing/subscription (không phải "ngoài scope", chỉ là chưa bàn tới). Phân tích ở mức business,
chưa phải `EntityDefinition` — giống cách `02-domain-model.md` đã làm.

## Entity mới

### `Plan`
Gói dịch vụ — Platform Admin định nghĩa, khách chọn lúc đăng ký/nâng cấp.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `name` | String | "Free", "Starter", "Business", "Enterprise" |
| `price` | Money | |
| `billingCycle` | Enum(`monthly`, `annual`) | |
| `maxZones` | Number | giới hạn số website (domain/subdomain) được bảo vệ cùng lúc |
| `maxFirewallRulesPerZone` | Number | |
| `rpsLimit` | Number | tổng request/giây được bảo vệ, tính trên toàn bộ zone của tenant |
| `scanFrequency` | Enum(`manualOnly`, `daily`, `hourly`) | gói thấp chỉ chạy tay, gói cao có lịch tự động |
| `accessLogRetentionDays` | Number | 0 = không có access log chi tiết (xem `10` mục Access Log) |
| `featureFlags` | Json (mảng string) | tính năng nào được bật theo gói — vd `["geoFirewall", "alertWebhook"]`, gói thấp thiếu vài cái |
| `enabled` | Boolean | ẩn gói cũ không bán nữa mà không xoá (còn khách đang dùng) |

### `Subscription`
1 tenant có đúng 1 subscription hiệu lực tại 1 thời điểm — cùng pattern "0..1 áp dụng" như
`DdosPolicy` (`unique` trên field tham chiếu tenant).

| Field | Kiểu | Ghi chú |
|---|---|---|
| `tenantId` | Reference, unique | |
| `planId` | Reference → `Plan` | |
| `status` | Enum(`trialing`, `active`, `pastDue`, `cancelled`) | workflow, giống pattern `Zone` |
| `currentPeriodStart` / `currentPeriodEnd` | Datetime | |

Workflow: `trialing → active → pastDue → active` (thanh toán lại) hoặc `→ cancelled` (terminal).

### `Zone.sizingTier` (field thêm vào entity đã có, không phải entity mới)
Chọn lúc onboard, dựa trên traffic dự kiến của website đó — Enum(`small`, `medium`, `large`).
Dùng để: (a) gợi ý default cho `DdosPolicy.requestRateThreshold` (site nhỏ threshold thấp hơn site
lớn), (b) cộng dồn vào tính RPS tổng của tenant so với `Plan.rpsLimit`.

## Giới hạn (quota) — enforce ở đâu, bằng cách nào

**Số lượng (zone/rule)** — enforce được ngay ở `management-plane`, app-level, **cùng 1 dạng vấn đề với
guard `hasConfig` đã gặp ở `Zone`**: `PolicyCondition` không tự đếm được số bản ghi liên quan
(đã xác nhận khi nghiên cứu `metap-permission`). Trước khi cho tạo `Zone`/`FirewallRule` mới,
phải tự đếm (`count(zones hiện có của tenant) < Plan.maxZones`) ở tầng app rồi mới cho `CrudService`
tạo tiếp — không phải declarative guard, là 1 bước check thêm trước khi gọi create.

→ **Đây là use-case thứ 2 cần đúng khả năng "đếm quan hệ"** mà `05-metap-technical-mapping.md`
từng liệt kê là MR khả thi nhưng "chưa gấp" (mục MR backlog, dòng 2b). Giờ có ít nhất 2 chỗ trong
chính sản phẩm này cần (Zone.hasConfig + quota theo gói) — đáng cân nhắc nâng độ ưu tiên MR đó lên
khi thực sự build tới phần billing, thay vì viết 2 đoạn code đếm thủ công riêng lẻ ở 2 chỗ.

**RPS** — **không thể** enforce ở `management-plane` (đó là traffic thật lúc runtime, `management-plane` chỉ
lưu cấu hình, không thấy traffic). `management-plane` chỉ là nơi lưu `Plan.rpsLimit` — việc đo/chặn
traffic vượt ngưỡng là việc của `edge-plane` (đọc `rpsLimit` qua `control-plane` giống cách đọc
`DdosPolicy`, tự throttle/block khi vượt) — **not this repo's phần yet**, chỉ ghi nhận ở đây để
`control-plane`/`edge-plane` biết cần đọc thêm giá trị này từ đâu khi tới lượt build.

## Portal — module "Gói & Thanh toán"

- Trang tổng quan: gói hiện tại, usage vs limit dạng progress bar ("3/5 website đã dùng", "RPS:
  120/500") — đổi màu cảnh báo khi gần chạm giới hạn (vd >90%).
- Bảng so sánh gói (pricing table) — hiện lúc đăng ký mới hoặc muốn nâng/hạ cấp.
- CTA "Nâng cấp" xuất hiện đúng lúc bị chặn vì quota (vd bấm "Add Zone" mà đã đủ `maxZones` →
  modal nâng cấp thay vì lỗi khô khan).
- Lịch sử hoá đơn — **chỉ làm nếu có tích hợp cổng thanh toán thật** (xem câu hỏi mở bên dưới).

## Câu hỏi mở — thuộc quyết định kinh doanh, không tự chọn thay

- **Cổng thanh toán nào** (Stripe/VNPay/chuyển khoản thủ công/chưa cần thu tiền thật, chỉ demo
  model)? Ảnh hưởng lớn tới việc có code phần thu tiền thật hay chỉ code
  `Plan`/`Subscription` như dữ liệu cấu hình.
- **Có gói Free không**, hay bắt buộc trial rồi trả phí?
- **RPS tính real-time hay theo tổng traffic tích luỹ** (vd giới hạn theo tháng thay vì tức thời)?
- Việc này **có cần cho v1 (demo) không**, hay để sau khi 4 trụ cột chính (`01-product-vision.md`)
  chạy được đã? Đề xuất: làm sau — quota/billing chỉ có ý nghĩa khi đã có Zone/Rule/ScanJob thật
  để mà giới hạn, nên hợp lý nhất là phase cuối cùng, không chặn thứ tự build hiện tại ở `05`.
