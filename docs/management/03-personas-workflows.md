# 03 — User Personas & Core Workflows

## Persona

| Role | Mô tả | Quyền chính (RBAC gợi ý) |
|---|---|---|
| **Platform Admin** | Đội ngũ vận hành `metap-demo-waf` như 1 SaaS | ~~Toàn quyền mọi tenant~~ **Sửa (xem `09-access-control.md`)**: chỉ xem được tổ chức (tenant) + domain (Zone) cụ thể mà tài khoản admin đó **được gán quyền** — không mặc định thấy hết mọi khách hàng, kể cả Platform Admin |
| **Tenant Admin** | Chủ tài khoản khách hàng | Toàn quyền trong tenant mình: quản lý Zone, mọi rule/policy, mời user |
| **Security Analyst (SOC)** | Vận hành hàng ngày | Xem/xử lý Incident, xem SecurityEvent/Analytics, sửa rule (không xoá Zone) — **chỉ các Zone được gán quyền**, xem `09-access-control.md` |
| **Developer** | Team dev của khách | Xem ScanFinding của zone mình phụ trách, cập nhật `remediationStatus`, không đụng DdosPolicy/FirewallRule — **"zone mình phụ trách" = zone được gán quyền**, xem `09-access-control.md` |
| **Viewer** | Stakeholder chỉ xem | Read-only Analytics/Incident, cũng theo Zone được gán quyền |

Đúng khớp mô hình RBAC matrix + ABAC condition builder đã có sẵn ở `metap` (`docs/roadmap/48-...`)
— không cần thiết kế permission engine mới, chỉ cần định nghĩa action set cho các entity mới này.

**Nguyên tắc gán quyền chung** (áp dụng cả Admin Portal lẫn Customer Portal, xem
`09-access-control.md` để biết chi tiết): lúc tạo user **phải** gán quyền (role + phạm vi
domain/tổ chức được xem); không gán gì thì mặc định chỉ đọc (read-only), không suy luận ngầm
thành toàn quyền.

## Core Workflows

### 1. Onboard 1 Zone mới
Bản đầy đủ (bao gồm bước xác minh sở hữu domain, đã phát hiện là thiếu ở bản rút gọn dưới đây) ở
`06-onboarding-rules-lists.md` mục 3. Tóm tắt:
1. Tenant Admin tạo `Zone` (hostname + originAddress) → status `pending`, `verificationStatus =
   unverified`.
2. Song song: verify domain (DNS TXT/HTTP file) **và** cấu hình ít nhất 1 `DdosPolicy`/
   `FirewallRule` (cả 2 đều bắt buộc, guard chặn activate khi thiếu 1 trong 2 — đã build + test).
3. Đặt `protectionMode = monitor` trước — chạy thử, xem `SecurityEvent`/Analytics để chắc rule
   không chặn nhầm traffic thật.
4. Chuyển `protectionMode = enforce` khi yên tâm → transition `status: pending → active`.
5. Edge-plane nhận config (qua cơ chế ở `04-architecture-boundary.md`) và bắt đầu bảo vệ thật.

### 2. Viết 1 Firewall Rule mới
1. Security Analyst tạo `FirewallRule` (matchCondition + action), đặt `priority` phù hợp thứ tự
   evaluate với rule khác.
2. Rule mới nghiễm nhiên tăng `Zone.configVersion` (side-effect tự động, không phải bước tay).
3. Xem `SecurityEvent` lọc theo `triggeredById = rule này` để xác nhận rule hoạt động đúng ý.

### 3. Chạy Vulnerability Scan & xử lý Finding
1. Tenant Admin/Developer tạo `ScanJob` cho 1 Zone — chạy tay hoặc đặt `schedule` (cron).
2. `ScanJob` chuyển `queued → running → completed`, sinh ra N `ScanFinding`.
3. Developer review từng `ScanFinding`, chuyển `remediationStatus`:
   `open → confirmed → fixed` (đã sửa xong, lần scan sau không thấy lại) hoặc
   `open → falsePositive`/`accepted` (không sửa, có lý do).
4. Lần `ScanJob` chạy tiếp theo tự đối chiếu `firstSeenAt`/`lastSeenAt` — finding cũ còn tồn tại
   thì cập nhật `lastSeenAt`, không tạo trùng.

### 4. Phản ứng khi có tấn công (Incident response)
1. Edge-plane phát hiện traffic bất thường → gửi `SecurityEvent` lên portal (số lượng lớn, near
   real-time).
2. Hệ thống correlation (logic chưa chốt, xem `02-domain-model.md`) gộp thành 1 `Incident`,
   trạng thái `open`.
3. `AlertPolicy` khớp điều kiện → sinh `AlertNotification` (email/webhook) báo SOC.
4. SOC Analyst mở Incident, `acknowledged` → điều tra, có thể tạo/sửa `FirewallRule`/`DdosPolicy`
   ngay từ context của Incident (rule mới áp dụng gần như ngay — xem độ trễ ở
   `04-architecture-boundary.md`) → `mitigating` → xác nhận traffic bất thường đã hết →
   `resolved`.

### 5. Xem Analytics
Tenant Admin/Viewer xem dashboard tổng hợp: traffic theo Zone, top rule bị match nhiều nhất, top
endpoint bị tấn công, xu hướng theo thời gian. Đây là màn hình đọc-nhiều dữ liệu `SecurityEvent`
khối lượng lớn — cân nhắc kỹ ở tầng data (table-per-entity/pre-aggregation) khi vào chi tiết kỹ
thuật, không thuộc phạm vi tài liệu nghiệp vụ này.
