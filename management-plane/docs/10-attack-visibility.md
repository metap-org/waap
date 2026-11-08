# 10 — Dashboard chặn lọc tấn công, lịch sử/chi tiết tấn công, access log

Mở rộng module 6 (`08-module-detail-specs.md`) — vẫn dùng `SecurityEvent`/`Incident` đã có, thêm
1 quyết định scope thật quan trọng ở cuối (access log).

## Dashboard chặn lọc (tổng quan)

Stat tile đầu trang: tổng request, số bị block/challenge/log, % bị chặn — theo khoảng thời gian
chọn được (24h/7 ngày/30 ngày/tuỳ chọn), lọc theo zone (hoặc "tất cả zone" ở dashboard cross-zone).
Chart traffic theo thời gian, stack theo `action` (logged/challenged/blocked) — nhìn 1 phát biết
lúc nào traffic tăng bất thường.

## Dashboard riêng WAF

Top `FirewallRule` bị match nhiều nhất (bảng + bar chart), breakdown theo `ruleType`
(waf/rateLimit/ipFirewall/geoFirewall — bao nhiêu % traffic bị chặn bởi loại nào), top endpoint
bị nhắm tới nhiều nhất.

## Dashboard riêng DDoS

Chart traffic thật (request/giây) so với `DdosPolicy.requestRateThreshold` (vẽ đường ngưỡng để
so sánh trực quan — vượt ngưỡng là thấy ngay, không phải đọc số). Đánh dấu mốc thời điểm
sensitivity/threshold từng đổi (nếu policy có sửa) để biết thay đổi cấu hình có làm giảm attack
được ăn qua không.

## Lịch sử tấn công

= chính là **Incident list** (đã spec ở module 7) — không phải 1 concept mới, chỉ là cách gọi
khác trong UI (menu "Lịch sử tấn công" trỏ vào đúng list Incident, filter mặc định theo thời gian
gần nhất).

## Chi tiết tấn công

= **Incident detail** (module 7), bổ sung thêm cho đúng nghĩa "chi tiết":
- Mini chart lưu lượng trong đúng khung giờ Incident diễn ra (không phải toàn bộ zone, chỉ đúng
  cửa sổ thời gian của Incident đó).
- Breakdown theo `sourceIp` / quốc gia (top N IP/country gây ra nhiều event nhất trong Incident
  này).
- Breakdown theo rule/policy nào bị match nhiều nhất trong đúng Incident này.
- Danh sách endpoint bị nhắm tới trong Incident.

Tất cả đều tính từ tập `SecurityEvent` đã join sẵn theo Incident (qua correlation đã có ở
`05-metap-technical-mapping.md`), không cần dữ liệu mới.

## Access Log — ⚠️ đây là tính năng MỚI, khác hẳn `SecurityEvent`, cần quyết định scope

**Access Log** (mọi request đi qua, kể cả request bình thường không bị chặn gì) **khác hoàn toàn**
`SecurityEvent` (chỉ log request **bị match** rule/policy — đã chốt rõ ở `02-domain-model.md`).
Nếu làm access log thật, khối lượng dữ liệu tăng lên rất nhiều lần — thay vì chỉ log request bị
chặn (số ít), phải log **tất cả** request (số nhiều, có thể gấp hàng trăm-hàng nghìn lần).

**2 hướng, cần chọn 1 trước khi build**:
- **(a) Log toàn bộ traffic vào cùng cơ chế `SecurityEvent`** (thêm `action = allowed` cho request
  bình thường) — đơn giản về mặt model (dùng lại entity có sẵn), nhưng volume tăng vọt, chắc chắn
  cần table-per-entity + retention/archival ngay từ đầu (không để sau như hiện đang tính cho
  `SecurityEvent`), có thể cần hạ tầng khác Postgres hoàn toàn ở quy mô traffic thật (log
  aggregator kiểu ClickHouse/Elasticsearch).
- **(b) Access log KHÔNG lưu ở `management-plane` (Postgres)** — coi là luồng log riêng do edge-plane
  xuất ra (kiểu Cloudflare Logpush: đẩy thẳng ra S3/log sink ngoài), `management-plane`/portal chỉ hiện
  **thống kê tổng hợp** (request/giây, mã trạng thái, top endpoint) chứ không cho xem từng dòng
  log chi tiết vô hạn thời gian. Rẻ hơn nhiều, đúng tinh thần tối giản v1.

**Khuyến nghị**: (b) cho v1 — access log chi tiết từng request thường là tính năng **trả phí theo
gói** ở sản phẩm thật (Cloudflare cũng giới hạn Logpush theo plan) — hợp lý để nối sang
`11-billing-plans.md` (gán access log detail là 1 feature-flag theo gói, không phải có sẵn cho
mọi khách). Quyết định cuối vẫn cần chủ dự án chốt, không tự ý mở rộng scope volume lớn thế này
nếu không thật sự cần cho demo.
