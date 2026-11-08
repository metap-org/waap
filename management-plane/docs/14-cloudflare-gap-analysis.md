# 14 — So sánh với Cloudflare: đã đủ chưa

Trả lời thẳng: **chưa, và không nên đủ** — `01-product-vision.md` đã chốt scope v1 chỉ 4 trụ cột
+ hạ tầng tối thiểu, không phải clone Cloudflare. Nhưng rà lại theo đúng feature set Cloudflare
thật để tách 3 loại rõ ràng: **loại có chủ đích** (đã biết, đã ghi trong docs) vs **gap thật**
(chưa từng nhắc tới, cần quyết định) vs **khác biệt hoá** (sản phẩm này có mà Cloudflare không).

## Đã loại có chủ đích (biết rồi, không phải gap)

| Cloudflare feature | Ghi ở đâu |
|---|---|
| DNS hosting/quản lý record | `01` — reverse-proxy per-hostname, không hosting DNS |
| SSL/TLS certificate, mTLS | `01` — ngoài phạm vi |
| DDoS L3/L4 (network-layer) | `01` — cần hạ tầng mạng riêng |
| Bot Management | `01` — v2+ |
| API Schema validation/discovery | `01` — v2+ |
| Managed WAF ruleset (OWASP CRS) | `01` — v2+ |
| Page Shield | `01` — v2+ |
| Attack Surface Management | `01` — v2+ |

## Gap thật — chưa từng nhắc tới ở đâu trong `01-13`

| Cloudflare feature | Vì sao đáng chú ý | Đề xuất |
|---|---|---|
| **Custom block/challenge page** — khách bị chặn thấy trang gì? | Hiện `FirewallRule.action = block` chỉ nói "chặn", chưa định nghĩa response trả về cho visitor (mã lỗi? trang HTML nào? có thể tuỳ chỉnh không?) | **Nên thêm vào v1** — thiếu cái này thì "block" chưa hoàn chỉnh về UX, dù nhỏ. Field gợi ý: `Zone.blockPageTemplate` hoặc dùng 1 trang mặc định chung trước, tuỳ chỉnh để v2 |
| **Origin health monitoring liên tục** (uptime check định kỳ) | Doc `11` chỉ có "Test Origin Connection" — 1 lần lúc onboard, không phải theo dõi liên tục. Cloudflare tự động failover/cảnh báo khi origin down | **Cân nhắc v1 nhẹ**: 1 cron job định kỳ ping `originAddress`, tạo `Alert`/cảnh báo nếu down — không cần failover (không có multi-origin) |
| **Load Balancing** (nhiều origin, tự chuyển khi 1 cái down) | `Zone.originAddress` hiện chỉ 1 địa chỉ, không có concept "nhiều origin, failover" | **Để v2+** — origin health monitoring (trên) đã đủ giá trị ở v1, load balancing thật cần nhiều thiết kế hơn (health check + traffic steering ở edge-plane) |
| **SSO/Enterprise identity** (SAML/OIDC cho khách doanh nghiệp login) | `09-access-control.md` chỉ có JWT/local login, chưa nhắc SSO | **Để v2+** — hợp lý vì cần khách hàng doanh nghiệp thật mới có giá trị (đúng tinh thần trigger-based) |
| **Redirect Rules / Transform Rules** (rewrite header, redirect theo URL) | Không phải tính năng bảo mật — Cloudflare có vì gộp chung CDN, sản phẩm này định vị thuần bảo mật | **Cố tình không làm** — không phải gap, là khác định vị sản phẩm (xem mục dưới) |
| **CDN/Caching/Performance** (cache static content, minify, image optimize) | Cloudflare gộp CDN+WAAP chung 1 sản phẩm, đây thì không | **Cố tình không làm** — nên ghi rõ vào `01` để không ai nhầm là thiếu, mà là chủ đích: sản phẩm này định vị **thuần bảo mật**, không phải CDN |
| **Sensitive Data Detection** (WAF quét response tìm PII leak) | Tính năng nâng cao, cần content scanning | **Để v2+**, độ ưu tiên thấp, ít nhắc tới ở brief gốc |

## Sản phẩm này có mà Cloudflare không có (khác biệt hoá, đã ghi ở `01`)

**Vulnerability Scanning** (DAST-kiểu, giống Detectify/Qualys hơn Cloudflare gốc) — đã là điểm
khác biệt hoá chủ đích, không phải bắt chước thiếu Cloudflare.

## Khuyến nghị cụ thể

Chỉ 2 gap đáng đưa vào v1 (nhỏ, tăng độ hoàn chỉnh rõ rệt, không phá vỡ tinh thần tối giản):
1. **Custom/default block page** — ít nhất 1 trang mặc định khi bị block, không cần tuỳ chỉnh v1.
2. **Origin health check định kỳ** — 1 cron job ping `originAddress`, cảnh báo khi down (tái dùng
   hẳn cơ chế `Alert`/`metap-cron` đã thiết kế ở `08`/`12`, không phải xây mới).

Còn lại (Load Balancing, SSO, Redirect Rules, CDN, Sensitive Data Detection) — nên **ghi rõ vào
`01-product-vision.md`'s "Ngoài scope v1"** để lần sau không ai hỏi lại "sao thiếu cái này", thay
vì để im lặng như hiện tại.
