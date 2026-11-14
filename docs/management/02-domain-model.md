# 02 — Domain Model (business level, chưa phải `EntityDefinition` code)

Mức field/kiểu dữ liệu ở đây là **business-level**, để review đúng-sai nghiệp vụ trước — chuyển
thành `EntityDefinition` (Rust, `metap-metadata`) là bước code sau, không lẫn vào đây.

## Sơ đồ quan hệ (tổng quan)

```
Tenant (= metap control.tenants, tái dùng, không tạo mới)
  ├─ Zone (site/domain được bảo vệ)
  │    ├─ DdosPolicy (0..1 áp dụng — 1 zone 1 policy DDoS L7 hiệu lực tại 1 thời điểm)
  │    ├─ FirewallRule (0..N — WAF/rate-limit/IP-geo, cùng 1 rule engine)
  │    ├─ ScanJob (0..N — lịch scan định kỳ hoặc chạy tay)
  │    │    └─ ScanFinding (0..N — kết quả 1 lần chạy ScanJob)
  │    ├─ SecurityEvent (0..N — log traffic bị match rule/policy, ghi từ edge-plane gửi lên)
  │    └─ Incident (0..N — gộp nhiều SecurityEvent liên quan thành 1 sự cố cần xử lý)
  ├─ AlertPolicy (không thuộc riêng 1 Zone — 1 alert có thể theo dõi nhiều zone)
  │    └─ AlertNotification (0..N — lần thực sự gửi cảnh báo, log lại đã gửi ai/khi nào/kênh gì)
  └─ Subscription (0..1 hiệu lực tại 1 thời điểm — xem `12-billing-plans.md`)
       └─ Plan (tham chiếu — Platform Admin định nghĩa, không thuộc riêng tenant nào)
```
`Plan`/`Subscription` phát sinh khi phân tích `12-billing-plans.md` — thêm vào đây để `02` giữ vai
trò là bản domain model đầy đủ, chi tiết field xem entity riêng bên dưới.

## Entity chi tiết

### `Zone`
Đại diện 1 site/domain khách hàng đăng ký bảo vệ — điểm neo mọi cấu hình khác.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `hostname` | String, unique, required | vd `shop.example.com` |
| `originAddress` | String, required | backend thật đứng sau, edge-plane forward traffic hợp lệ tới đây |
| `status` | Enum (`pending`/`active`/`paused`/`suspended`) | workflow — xem bên dưới |
| `protectionMode` | Enum (`monitor`/`enforce`) | `monitor` = chỉ log, không block — bắt buộc có để khách test rule mới không sợ block nhầm traffic thật (Cloudflare gọi là "Log only") |
| `configVersion` | Number | tăng mỗi lần bất kỳ policy/rule con nào đổi — edge-plane dùng để biết cần re-pull config (xem `04-architecture-boundary.md`) |
| `verificationToken`/`verificationMethod`/`verificationStatus` | String/Enum/Enum | chứng minh khách sở hữu domain trước khi activate — chi tiết ở `06-onboarding-rules-lists.md` mục 2 |
| `dnsRoutingStatus`/`lastDnsCheckAt` | Enum/DateTime | domain đã thực sự trỏ traffic về hệ thống chưa — thuần hiển thị, không chặn activate, xem `11-onboarding-dns-resolution.md` |
| `sizingTier` | Enum (`small`/`medium`/`large`) | chọn lúc onboard theo traffic dự kiến — gợi ý default cho `DdosPolicy.requestRateThreshold`, cộng dồn tính RPS tổng so với gói dịch vụ, xem `12-billing-plans.md` |

**Workflow `status`**: `pending → active` (guard: **cả 2 điều kiện** — đã có ít nhất 1
DdosPolicy/FirewallRule, **và** `verificationStatus = verified` — không cho activate 1 zone
trống config hoặc chưa chứng minh quyền sở hữu domain) `→ paused` (tạm ngưng bảo vệ, giữ config)
`→ active` (bật lại) `→ suspended` (terminal, do admin platform khoá, vd không thanh toán).

### `DdosPolicy`
Chính sách DDoS L7 cho 1 Zone — đúng trụ cột đầu tiên chủ dự án nêu.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `zoneId` | Reference → Zone, required | |
| `sensitivity` | Enum (`low`/`medium`/`high`/`aggressive`) | mức nhạy cảm phát hiện flood — cao hơn = dễ false-positive hơn |
| `requestRateThreshold` | Number | request/giây/IP (hoặc /session) trước khi coi là bất thường |
| `burstWindow` | Number (giây) | cửa sổ đo rate |
| `action` | Enum (`log`/`challenge`/`block`) | hành động khi vượt threshold |
| `enabled` | Boolean | tắt nhanh không cần xoá config |

### `FirewallRule`
Rule engine dùng chung cho WAF custom rule / rate-limit / IP-geo firewall (gộp theo quyết định ở
`01-product-vision.md`) — 1 zone có N rule, có thứ tự ưu tiên.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `zoneId` | Reference → Zone, required | |
| `name` | String, required | |
| `priority` | Number | rule số nhỏ hơn evaluate trước, dừng ở rule đầu tiên match (giống Cloudflare) |
| `matchCondition` | JSON (structured, không phải chuỗi tự do) | vd `{"field": "uri.path", "op": "contains", "value": "/admin"}`, hỗ trợ AND/OR lồng nhau — tương tự `PolicyCondition` đã có trong `metap-permission`, nên tái dùng cấu trúc đó thay vì phát minh 1 grammar mới |
| `ruleType` | Enum (`waf`/`rateLimit`/`ipFirewall`/`geoFirewall`) | phân loại cho UI nhóm hiển thị, logic evaluate vẫn chung 1 engine |
| `rateLimitThreshold` | Number, optional | chỉ có nghĩa khi `ruleType = rateLimit` |
| `rateLimitWindow` | Number (giây), optional | |
| `action` | Enum (`allow`/`block`/`challenge`/`log`) | |
| `enabled` | Boolean | |

### `ScanJob`
1 cấu hình scan (không phải 1 lần chạy) — trụ cột thứ hai chủ dự án nêu.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `zoneId` | Reference → Zone, required | |
| `scanType` | Enum (`quickScan`/`fullScan`/`apiScan`) | phạm vi/độ sâu quét |
| `schedule` | String (cron expression), optional | trống = chỉ chạy tay; tái dùng thẳng `metap-cron` |
| `status` | Enum (`idle`/`queued`/`running`/`completed`/`failed`) | trạng thái lần chạy gần nhất, không phải trạng thái "job" nói chung |
| `lastRunAt` | DateTime, optional | |

### `ScanFinding`
1 lỗ hổng tìm được trong 1 lần chạy `ScanJob` — mối quan hệ N:1 với `ScanJob` qua field bên dưới,
không phải qua `zoneId` trực tiếp (1 finding luôn thuộc đúng 1 lần chạy scan).

| Field | Kiểu | Ghi chú |
|---|---|---|
| `scanJobId` | Reference → ScanJob, required | |
| `severity` | Enum (`info`/`low`/`medium`/`high`/`critical`) | |
| `category` | String | vd "SQL Injection", "XSS", "Outdated TLS" — nhóm loại lỗ hổng, không cố định enum vì danh mục sẽ tăng dần |
| `endpoint` | String | URL/route cụ thể phát hiện ra |
| `description` | String (long text) | |
| `remediationStatus` | Enum (`open`/`confirmed`/`falsePositive`/`fixed`/`accepted`) | workflow riêng cho finding — SOC/dev team xử lý độc lập với vòng đời `ScanJob` |
| `firstSeenAt` / `lastSeenAt` | DateTime | phân biệt lỗ hổng mới với lỗ hổng lặp lại qua nhiều lần scan |

### `SecurityEvent`
Log 1 request bị 1 rule/policy match (block/challenge/log) — dữ liệu này **do edge-plane gửi
ngược lên**, không phải portal tự sinh ra (xem `04-architecture-boundary.md`). Khối lượng lớn nhất
trong toàn hệ thống — cân nhắc từ đầu là ứng viên cho **table-per-entity** (`metap-reconciler`)
thay vì bảng `records` chung, và có thể cần retention/archival policy riêng (không nằm trong
`metap` hiện tại, ghi nhận như 1 gap cần xử lý khi có volume thật).

| Field | Kiểu | Ghi chú |
|---|---|---|
| `zoneId` | Reference → Zone, required | |
| `triggeredBy` | Enum (`ddosPolicy`/`firewallRule`) + `triggeredById` (id của policy/rule cụ thể) | |
| `action` | Enum (`logged`/`challenged`/`blocked`) | hành động thực tế edge-plane đã làm |
| `sourceIp` | String | |
| `requestPath` | String | |
| `occurredAt` | DateTime | thời điểm thật ở edge, không phải thời điểm portal nhận được (2 cái có thể lệch nhau) |

### `Incident`
Gộp nhiều `SecurityEvent` liên quan (cùng nguồn tấn công/cùng zone/cùng khung giờ) thành 1 sự cố
SOC cần xử lý — tránh SOC phải nhìn hàng nghìn event rời rạc.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `zoneId` | Reference → Zone, required | |
| `title` | String | vd "DDoS L7 spike từ 14:00-14:15" |
| `severity` | Enum (`low`/`medium`/`high`/`critical`) | |
| `status` | Enum (`open`/`acknowledged`/`mitigating`/`resolved`) | workflow chuẩn, dùng `metap-workflow` |
| `eventCount` | Number | số `SecurityEvent` gộp vào, tính lúc tạo incident (không phải live-count) |
| `assignedTo` | Reference → User, optional | |

**Cách gộp `SecurityEvent` thành `Incident`** là logic nghiệp vụ cần bàn riêng (rule-based
correlation theo zone+khung giờ+nguồn, hay cần 1 job phân tích) — chưa chốt ở tài liệu này, để
bàn khi vào chi tiết kỹ thuật.

### `AlertPolicy` / `AlertNotification`
| Entity | Field chính | Ghi chú |
|---|---|---|
| `AlertPolicy` | `name`, `thresholdCount` + `windowMinutes` (vd "≥ N event trong M phút, trên **cùng 1 zone**"), `channels` (email/webhook), `enabled` | thuộc Tenant, theo dõi nhiều zone — mỗi zone tự tính riêng, không cộng dồn |
| `AlertNotification` | `alertPolicyId`, `triggeredAt`, `channel`, `deliveryStatus` (`sent`/`failed`) | log audit đã gửi — tách khỏi `AlertPolicy` vì là lịch sử phát sinh, không phải cấu hình |

### `Plan` / `Subscription`
Phân tích đầy đủ ở `12-billing-plans.md` — tóm tắt field chính:

| Entity | Field chính | Ghi chú |
|---|---|---|
| `Plan` | `name`, `price`, `billingCycle`, `maxZones`, `maxFirewallRulesPerZone`, `rpsLimit`, `scanFrequency`, `accessLogRetentionDays`, `featureFlags`, `enabled` | Platform Admin định nghĩa, không thuộc riêng tenant |
| `Subscription` | `tenantId` (unique — 0..1 hiệu lực), `planId`, `status` (`trialing`/`active`/`pastDue`/`cancelled`), `currentPeriodStart`/`currentPeriodEnd` | workflow giống pattern `Zone`/`DdosPolicy` |

Có tích hợp cổng thanh toán thật (provider chưa chọn) — xem `12-billing-plans.md` phần câu hỏi
mở.

## Điểm từng để ngỏ — đã chốt

- ✅ `matchCondition` của `FirewallRule` — **không** tái dùng `PolicyCondition`, tự định nghĩa
  grammar riêng (namespace `uri.*`/`header.*`/`body.*`, thêm operator `Contains`/`Regex`/
  `CidrMatch`) — xem `05-metap-technical-mapping.md` mục "matchCondition".

## Chốt tiếp (hoàn thiện đợt BA này, 2026-08-30) — đề xuất mặc định, có thể override

- **`Incident` correlation: rule tĩnh cho v1**, không configurable per-tenant. Đề xuất cụ thể:
  gộp các `SecurityEvent` cùng `zoneId` + cùng `sourceIp` (hoặc cùng dải CIDR /24) trong cửa sổ
  15 phút thành 1 `Incident`. Threshold configurable per-tenant để v2+ — đúng tinh thần tối giản
  đã theo xuyên suốt (`01-product-vision.md`), và `AlertPolicy.thresholdCount/windowMinutes` đã
  cho tenant tự chỉnh *khi nào báo động*, không cần thêm 1 lớp cấu hình *cách gộp incident* nữa ở
  v1.
- **`SecurityEvent` retention: 30 ngày mặc định**, gắn theo `Plan` (tier cao hơn giữ lâu hơn —
  cùng cơ chế với `accessLogRetentionDays` đã thiết kế ở `12-billing-plans.md`, dùng chung 1 kiểu
  field thay vì 2 khái niệm retention riêng biệt). **Không build cơ chế archive S3 thật ở v1** —
  hết hạn thì xoá (dùng `metap-cron` `Schedule` job dọn định kỳ), việc chuyển sang cold storage
  (`metap-storage`) để khi có volume thật cần giữ lâu hơn Postgres chịu được (đúng ghi chú gốc ở
  đây, giờ chỉ cụ thể hoá thêm thời điểm cần quay lại: khi bắt đầu thấy chi phí Postgres tăng rõ
  rệt vì volume, không phải mốc thời gian cố định).
