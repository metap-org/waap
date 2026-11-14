# 01 — Product Vision & Scope

## WAAP là gì

WAAP (Web Application and API Protection) là lớp bảo vệ đặt trước ứng dụng/API của khách hàng,
gộp chung nhiều năng lực từng tách rời (WAF, DDoS mitigation, Bot management, API security) vào
một sản phẩm. Cloudflare/Akamai/AWS WAF+Shield là các sản phẩm tham chiếu.

## Toàn cảnh tính năng (nếu làm đủ như Cloudflare) — để cân nhắc scope, không phải cam kết xây hết

| Nhóm | Tính năng | Ghi chú |
|---|---|---|
| **DDoS Protection** | L3/L4 (network-layer flood) | Off-path/BGP-level — ngoài phạm vi 1 portal ứng dụng, cần hạ tầng mạng riêng, **không làm** |
| | **L7 (HTTP flood)** | Threshold theo request rate/pattern, sensitivity level, action (log/challenge/block) — **trong scope v1**, đúng cái chủ dự án nêu |
| **WAF** | Managed ruleset (OWASP Core Rule Set-kiểu) | Cần duy trì bộ rule signature — công sức lớn, **v2+** |
| | Custom rule builder (match condition + action) | Match trên URI/method/header/cookie/body, action allow/block/challenge/log — **trong scope v1** (nền tảng cho cả WAF lẫn DDoS L7 dùng chung 1 rule engine) |
| | Rate limiting rule | Threshold theo IP/session/API key trên 1 route — **trong scope v1**, chung họ với DDoS L7 |
| **API Protection** | API schema validation (OpenAPI-based) | **v2+** |
| | API discovery (tự phát hiện endpoint) | **v2+** |
| **Bot Management** | Bot score, JS challenge, managed challenge | Cần model/heuristic riêng — **v2+** |
| **Vulnerability Scanning** | Scheduled scan (DAST-kiểu) trên target, tìm lỗ hổng | **Trong scope v1**, đúng cái chủ dự án nêu — khác biệt so với Cloudflare gốc (CF không có sẵn, gần giống Detectify/Qualys hơn) — là điểm khác biệt hoá của sản phẩm này |
| | Attack Surface Management (subdomain discovery) | **v2+**, tự nhiên nối tiếp vulnerability scan |
| **SSL/TLS** | Certificate issuance/management | Cloudflare tự làm CA proxy — hạ tầng nặng, **ngoài phạm vi**, giả định khách tự quản TLS hoặc dùng edge-plane có sẵn |
| **Firewall Rules** | IP allow/block list, geo-block, ASN block | **Trong scope v1** — đơn giản, giá trị cao, dùng chung rule engine với WAF |
| **Analytics & Logging** | Traffic analytics, security event log, top attacked endpoint | **Trong scope v1** — cần để chứng minh giá trị sản phẩm ("chặn được gì") |
| **Alerting** | Notification policy (email/webhook) khi có spike/incident | **Trong scope v1**, tái dùng `metap-cron`'s workflow automation |
| **Incident Management** | Timeline, acknowledge/resolve, SOC workflow | **Trong scope v1** — tận dụng `metap-workflow` (state machine) có sẵn |
| **Page Shield** | Giám sát script phía client (supply-chain attack) | **v2+**, engine hoàn toàn khác (browser-side telemetry) |

## Quyết định scope v1

**4 trụ cột chính, đúng cái chủ dự án đã nêu + phần tối thiểu để nó thành sản phẩm dùng được**:

1. **DDoS L7 Protection** — cấu hình policy (threshold/sensitivity/action) theo Zone.
2. **WAF / Firewall Rules** — 1 rule engine dùng chung cho WAF custom rule, rate limit, IP/geo
   firewall (gộp làm 1 vì bản chất đều là "match condition → action" trên request, tách 3 UI khác
   nhau sẽ trùng lặp logic không cần thiết ở portal).
3. **Vulnerability Scanning** — scan job theo lịch, tìm finding, track trạng thái xử lý.
4. **Analytics + Alerting + Incident** — lớp "nhìn thấy kết quả" bắt buộc phải có để 3 trụ cột
   trên có giá trị thật (không ai cấu hình DDoS policy mà không xem được nó đã chặn gì).

**Ngoài scope v1** (ghi lại để không quên, không phải sẽ không bao giờ làm): Bot management, API
schema validation/discovery, managed WAF ruleset (OWASP-kiểu), TLS/certificate, Page Shield,
Attack Surface Management, Load Balancing (nhiều origin/failover), SSO/Enterprise identity,
Sensitive Data Detection. Mỗi cái đáng thành 1 phase riêng khi có trigger (khách hàng thật cần)
— đúng tinh thần trigger-based evolution `metap` đang theo.

**Cố tình không làm, khác định vị sản phẩm chứ không phải thiếu** (so sánh đầy đủ ở
`14-cloudflare-gap-analysis.md`): Redirect/Transform Rules, CDN/Caching/Performance
(minify/image optimization). Cloudflare gộp chung CDN+WAAP; sản phẩm này định vị **thuần bảo
mật**, không đi theo hướng CDN.

**2 gap nhỏ đưa thêm vào v1** (phát hiện lúc so với Cloudflare, đủ nhỏ để không phá tinh thần tối
giản): trang mặc định hiện ra khi 1 request bị block, và 1 job kiểm tra `originAddress` còn sống
định kỳ (cảnh báo khi origin down) — chi tiết ở `14-cloudflare-gap-analysis.md`.

**Mảng phát sinh thêm khi phân tích sâu hơn (2026-08-30)** — không phải trụ cột mới, là hạ tầng
bắt buộc để 4 trụ cột trên vận hành đúng như 1 sản phẩm thật (không phải demo trống trơn):
- **Onboarding/domain verification** (`06`, `11`) — xác minh sở hữu domain trước khi bảo vệ, tra
  DNS/IP lúc onboard.
- **Access control theo (user, domain)** cho cả Admin Portal lẫn Customer Portal (`09`) — không
  chỉ RBAC theo role như bản `03` gốc.
- **Billing/Plan/Subscription** (`12`), có tích hợp cổng thanh toán thật (provider chưa chọn) —
  quota theo gói (số zone/rule/RPS).

3 mảng này **trong scope build**, không phải "ngoài scope v1" — chỉ khác 4 trụ cột ở chỗ chúng là
điều kiện để sản phẩm dùng được thật (giống lý do "Analytics + Alerting" đã ở trong scope), không
phải tính năng bảo vệ trực tiếp.

## Vì sao xây nhanh được bằng `metap`

- **Zone / DDoS Policy / Firewall Rule / Scan Job / Scan Finding / Alert / Incident** đều là CRUD
  + workflow (state machine) + permission theo tenant — đúng bài toán `metap` giải sẵn (portal
  list/form/detail generic, RBAC/ABAC matrix, transition có guard).
- **Incident** đặc biệt hợp với `metap-workflow`: `open → acknowledged → mitigating → resolved`
  là 1 `EntityWorkflow` chuẩn, không cần code riêng.
- **Alerting theo lịch/theo threshold** dùng thẳng `metap-cron`'s workflow automation
  (`TargetType::Steps`/`WaitEvent`) thay vì viết 1 hệ thống notification riêng.
- Phần **không** hợp với `metap` (và cố tình để ngoài management-plane này): control-plane (tính toán +
  đẩy config) và edge-plane (thực thi mitigation) — cả hai đều là hệ thống real-time, hiệu năng
  cao, không phải CRUD — xem `docs/04-architecture-boundary.md`.
