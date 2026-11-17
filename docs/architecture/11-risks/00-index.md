# 11. Risks and Technical Debt

## 11.1 Rủi ro

| # | Rủi ro | Khả năng | Tác động | Giảm thiểu | Dấu hiệu cần hành động |
|---|---|---|---|---|---|
| R1 | Engine WAF + CRS vượt ngân sách độ trễ QS-1 | Trung bình | Cao | Spike so sánh trước khi chọn ([ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md)); giới hạn kiểm tra body theo entitlements; paranoia mặc định 1; engine sau interface `WafEngine` để thay | p99 > 2 ms trên traffic mẫu |
| R2 | Chặn nhầm làm mất niềm tin khách | Cao | Cao | Log-only 7 ngày khi onboarding; replay trước khi enforce; xử lý chặn nhầm tự phục vụ; tỉ lệ FP theo rule | Tỉ lệ FP của một rule > ngưỡng; ticket support về chặn nhầm tăng |
| R3 | Chi phí log vượt doanh thu | Trung bình | Cao | Entitlements, lấy mẫu access log, tách store theo chi phí | Chi phí log / doanh thu theo tenant > ngưỡng |
| R4 | Config hỏng ra toàn fleet | Thấp | Rất cao | Compiler kiểm tra, chữ ký, canary có gate, rollback tức thì | Lỗi nạp snapshot ở canary |
| R5 | Node trên hạ tầng đối tác bị chiếm | Thấp | Cao | Danh tính node, mTLS cert 24 h, thu hồi trong vài phút ([8.19](../08-cross-cutting/19-node-identity-and-mtls.md)), key cert chỉ trong RAM, keyless sau này | Hành vi bất thường từ một node |
| R6 | Phụ thuộc đối tác cho L3/L4 | Trung bình | Trung bình | Nhiều đối tác; hợp đồng có SLA | Đối tác không đáp ứng tấn công lớn |
| R7 | On-prem làm phức tạp mọi quyết định công nghệ | Trung bình | Trung bình | Ràng buộc "không dịch vụ độc quyền một cloud" từ đầu; chưa xây on-prem tới giai đoạn 3 | Khách lớn đòi on-prem sớm |
| R8 | Rate limit xấp xỉ không đạt kỳ vọng khách | Trung bình | Thấp | Ghi rõ trong tài liệu; chu kỳ gộp ngắn ở PoP lớn | Khiếu nại ngưỡng nhỏ không chính xác |
| R10 | Hai broker (RabbitMQ, Kafka) tăng vận hành | Trung bình | Thấp | Chỉ dùng RabbitMQ cho sự kiện điều khiển khối lượng nhỏ; gộp khi `metap` có adapter Kafka ([ADR-0022](../09-adr/0022-rabbitmq-for-control-events-kafka-for-telemetry.md)) | Sự cố vận hành broker lặp lại |
| R11 | Mẫu request đầy đủ lộ dữ liệu nhạy cảm | Thấp | Cao | Che trường bắt buộc tại edge, khách tắt được, retention ngắn, mã hoá at-rest ([ADR-0026](../09-adr/0026-replay-on-request-samples-with-shared-pipeline.md)) | Khách hoặc kiểm toán phản đối |
| R12 | Nếu chọn Rust cho edge: thiếu người viết Rust (org vừa bỏ Rust ở repo khác) | Trung bình | Cao | Tuyển hoặc đào tạo sớm; edge là codebase tách biệt ([ADR-0030](../09-adr/0030-language-per-workload.md)) | Tiến độ edge chậm hơn các plane khác |
| R13 | Nếu chọn libmodsecurity: lỗi bộ nhớ (C++) trên đường traffic | Trung bình | Cao | Pin version, vá trong 72 giờ, fuzz lớp tích hợp, bắt lỗi tại ranh giới; thay engine khi có lựa chọn an toàn hơn qua bộ test CRS | CVE mới của libmodsecurity |
| R14 | Nếu L7 không phải Go: ngữ nghĩa custom rule lệch giữa `cel-go` và evaluator IR | Thấp | Cao | Bộ vector test đối chiếu chạy ở cả hai phía trong CI ([ADR-0032](../09-adr/0032-cel-checked-at-control-ir-at-edge.md)) | Vector test lệch |
| R15 | Chặng L7 → WAF làm vượt ngân sách độ trễ | Thấp | Trung bình | PoP nhỏ đặt chung host qua Unix socket; verdict sớm; đo trong spike ([ADR-0033](../09-adr/0033-two-tier-edge-proxy-and-waf.md)) | p99 gọi WAF > 1,5 ms |
| R16 | Tự xây CDN tốn đầu tư băng thông và lưu trữ trước khi có doanh thu | Trung bình | Cao | Làm theo giai đoạn: cache cơ bản trong L7 ở MVP, tiered cache sau; chế độ đứng sau CDN của khách để bán sớm ([ADR-0035](../09-adr/0035-cdn-and-dns-strategy.md)) | Chi phí băng thông / doanh thu theo PoP |
| R17 | Bot detection chặn nhầm người dùng thật hoặc bị bot vượt qua (cuộc đua liên tục) | Cao | Cao | Log-only trước khi enforce; QS-21; dữ liệu bot phát hành qua đánh giá và log-only song song; phản hồi khách vào vòng học ([ADR-0038](../09-adr/0038-bot-management-layered-scoring.md)) | Tỉ lệ challenge tương tác tăng; khiếu nại của khách |
| R18 | Fingerprint và JS thu tín hiệu vướng quy định dữ liệu cá nhân | Trung bình | Trung bình | Lưu dạng hash, JS ngầm mặc định tắt, gộp chéo tenant chỉ thống kê | Yêu cầu từ pháp chế hoặc khách |
| R19 | Schema API sai làm chặn traffic hợp lệ | Trung bình | Cao | Log-only mặc định, hiển thị chênh lệch discovery và schema, enforce theo endpoint ([ADR-0039](../09-adr/0039-api-security-positive-model.md)) | Vi phạm schema tăng ngay sau khi enforce |
| R20 | Ước lượng tải ở 7.5 sai lệch lớn (phân bố traffic, năng lực node, kích thước cấu hình) | Trung bình | Cao | Đo lại sau spike và theo từng giai đoạn khách hàng; thiết kế không phụ thuộc tổng số zone nên chỉ cỡ hạ tầng phải đổi | Số đo thật lệch quá 2 lần so với giả định |
| R21 | Request đầu tiên của zone nguội chậm, hoặc nhiều zone nguội cùng lúc làm quá tải PoP store sau sự cố | Trung bình | Trung bình | Làm nóng trước theo danh sách zone nóng; PoP store dự phòng; giới hạn tốc độ dựng runtime; node lấy được từ regional relay | QS-25 vỡ |
| R22 | Tenant hoặc zone rất lớn dồn vào một partition Kafka, một shard ClickHouse, một cell | Trung bình | Trung bình | Khoá theo `(tenant, zone)` cho tenant lớn; cell riêng; cân bằng cell theo tải | Lag hoặc CPU lệch giữa partition/shard/cell |
| R23 | Dùng thử, Free có trần bị lạm dụng (phishing, phát file lớn, đăng ký hàng loạt) | Cao | Trung bình | 1 zone/account, xác minh domain và phương thức thanh toán, trần cứng ở edge, Trust and Safety ([8.18 §7](../08-cross-cutting/18-segments-and-plans.md#7-lạm-dụng)) | Chi phí của zone không trả phí vượt ngân sách; khiếu nại abuse tăng |
| R24 | Mô hình phân khúc và đơn giá ở 8.18 sai (tỉ lệ Free/Business, tải mỗi zone, giá băng thông) làm gói lỗ | Trung bình | Cao | Giá là entitlement đổi được; đo chi phí thật theo gói từ metering; xem lại hằng quý | Biên lợi nhuận một gói âm 2 tháng liền |
| R25 | Giá băng thông thuê cao hơn giả định (nhất là quốc tế) làm phần phân phối lỗ | Cao | Cao | Tính phí theo GB, đơn giá vượt có biên; ưu tiên PoP trong nước và peering tại IX; hybrid/on-prem cho khách lớn; đàm phán với nhà mạng | Chi phí băng thông thật trên mỗi GB lệch > 30% so với 8.18 |
| R26 | Sai số đo usage gây tranh chấp hoá đơn | Trung bình | Cao | `UsageCounter` không lấy mẫu, số thứ tự, đối soát 24 giờ, bộ đếm cục bộ 7 ngày (QS-35); khách xem usage theo zone trong kỳ | Khiếu nại hoá đơn; chênh lệch đối soát > 0,1% |
| R27 | Thêm runtime thứ ba (Elixir) làm khó tuyển người và vận hành | Trung bình | Trung bình | Giới hạn Elixir ở hai thành phần có hợp đồng rõ (realtime gateway, notifier); nếu không đủ người thì dự phòng Centrifugo + Go ([ADR-0044](../09-adr/0044-realtime-gateway-and-notifier-runtime.md)) | Thiếu người đọc được Elixir; sự cố gateway kéo dài |
| R28 | L7 và WAF hiểu cùng một request khác nhau (chuẩn hoá lệch), kẻ tấn công lách qua WAF | Trung bình | Cao | Một bên chuẩn hoá, gửi cả bản gốc và bản đã chuẩn hoá; bộ test evasion trong CI (IA2) | Payload bị chặn ở WAF test nhưng lọt qua edge thật |
| R29 | Thay đổi nền tảng ở `metap-go` (unique mặc định theo tenant, index mới) làm sản phẩm đang chạy dựng lại index hoặc đổi nghĩa unique khi nâng | Trung bình | Trung bình | Đã sửa ở `metap-go` (Phase 103) kèm e2e đường nâng cấp; field cần duy nhất toàn bảng khai báo `uniqueScope: global` ([5.8.14](../05-building-blocks/08-components/14-management-database.md)) | Lần reconcile đầu sau khi nâng chạy lâu; lỗi trùng unique mới |
| R9 | Đội nhỏ dàn trải 4 plane | Cao | Cao | Mua/dùng mã nguồn mở cho phần không khác biệt; làm đúng thứ tự giai đoạn | Trễ MVP |

## 11.2 Chênh lệch với code demo hiện tại

Code trong repo là demo, không phải điểm xuất phát bắt buộc. Những chỗ khác kiến trúc đích:

| Chủ đề | Demo | Kiến trúc đích |
|---|---|---|
| Plane | 3 plane; log nằm trong database của portal | 4 plane, Analytics riêng |
| Snapshot | Mỗi zone một key trong kho key-value, không chữ ký, không ack | Bundle bất biến có chữ ký, ack theo version, rollout theo đợt |
| Managed ruleset | Không có | CRS + system ruleset trên engine SecLang |
| Ngôn ngữ rule | JSON grammar riêng | Cú pháp có sẵn, AST, builder ↔ text |
| Rate limit | Đếm cục bộ từng node | Gộp định kỳ toàn fleet |
| Log | Chỉ security event, buffer trong RAM, ghi vào Postgres của portal | Access log + event, buffer đĩa, bus bền, store riêng |
| Vòng đời policy | Monitor mode, `configVersion` chỉ là số đếm | PolicyVersion bất biến, mô phỏng, duyệt, canary, rollback |
| Fleet | Một process, không danh tính node | Registry, mTLS, health, độ phủ version |
| Cert, Under Attack, SSO, entitlements, SIEM export | Không có | MVP |

Khuyến nghị: giữ Management (portal trên `metap`) làm nền và tiến hoá dần; viết lại Control và
Edge theo hợp đồng ở [05-contracts](../05-building-blocks/05-contracts.md); xây mới Analytics.

## 11.3 Quyết định còn mở

Đã chốt ngày 2026-11-06: biến thể cú pháp rule ([ADR-0014](../09-adr/0014-cel-as-rule-language.md),
CEL), công nghệ của Control, Analytics, Management ([05 · bản đồ công nghệ](../05-building-blocks/07-technology-map.md)),
database của Management ([ADR-0046](../09-adr/0046-management-database-tenancy-and-storage.md)),
Elixir + Phoenix cho realtime gateway và notifier ([ADR-0044](../09-adr/0044-realtime-gateway-and-notifier-runtime.md)).

| Chủ đề | Cần quyết | Ảnh hưởng |
|---|---|---|
| Tự bật Under Attack mode theo detection | Có cho phép, mặc định tắt hay bật | [ADR-0008](../09-adr/0008-under-attack-mode.md) |
| Giá và bảng entitlements | Bản nháp theo hồ sơ công ty phần mềm ở [8.18](../08-cross-cutting/18-segments-and-plans.md); business chốt giá, hạn mức, đơn giá vượt, có mở Free có trần hay không | [ADR-0043](../09-adr/0043-usage-based-pricing-and-metering.md), [ADR-0007](../09-adr/0007-log-by-subscription-entitlements.md), [ADR-0042](../09-adr/0042-plan-tiers-drive-placement-and-isolation.md) |
| Cấp phép và cập nhật on-prem | Cách phát hành, cách cập nhật ruleset cho khách offline | [ADR-0006](../09-adr/0006-saas-first-on-prem-capable.md) |
| Nhà cung cấp geo/ASN | MaxMind hay IPinfo, theo giá và điều khoản phân phối lại | [ADR-0025](../09-adr/0025-geo-asn-data-commercial-license.md) |
| Đối tác L3/L4 và hạ tầng PoP đầu tiên | Chọn đối tác, region đầu tiên | [ADR-0005](../09-adr/0005-edge-on-own-and-existing-infra.md) |
| Challenge tương tác | Tự xây (đề xuất, không gửi dữ liệu ra ngoài, chạy on-prem) hay mặc định dùng dịch vụ CAPTCHA bên thứ ba | [ADR-0038](../09-adr/0038-bot-management-layered-scoring.md) |
| Chính sách crawler AI mặc định | Allow, block, hay theo `robots.txt` cho zone mới | [8.16](../08-cross-cutting/16-bot-management.md#4-policy-của-khách) |
| **Hợp đồng Inspection API (L7 ↔ WAF) v1** | Mô hình stream, ai chuẩn hoá request, version lệch, chọn instance, retry, backpressure, mức tin dữ liệu (IA1–IA13) | [5.5](../05-building-blocks/05-contracts.md#inspection-api-l7--waf); nghiên cứu trong spike của [ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md) |
| **Cell và cách đặt zone** | Cell theo VIP (đề xuất), không cell, hash theo SNI, hay cell theo PoP; số cell ban đầu | [ADR-0041](../09-adr/0041-cells-and-zone-placement.md); phụ thuộc mô hình mạng và số dải IP có được |
| **Chiến lược CDN và DNS** | Tự xây theo giai đoạn + chế độ đứng sau CDN khách (đề xuất), hay đứng sau CDN, bán lại CDN đối tác, hoặc không CDN | [ADR-0035](../09-adr/0035-cdn-and-dns-strategy.md); kéo theo tiêu chí cache khi chọn nền L7 |
| **Nền tầng L7 và engine tầng WAF** | Hướng spike chốt 2026-11-17: Pingora làm L7 chính, NGINX đối chứng, Envoy hoặc NGINX dự phòng, tiêu chí "ít reload" ([ADR-0031 §6.1](../09-adr/0031-edge-l7-and-waf-engine-options.md#61-hướng-spike-chốt-2026-11-17)). Chọn sau spike 3 tuần: shortlist K2 (Pingora + Coraza), K3 (Go + Coraza), K1 (Envoy + Coraza); mốc K5 (NGINX, cho cache) và K4 (HAProxy + SPOE); đo thêm libmodsecurity ở tầng WAF; kèm HTTP/3 | [ADR-0031](../09-adr/0031-edge-l7-and-waf-engine-options.md); kéo theo ngôn ngữ L7, replay, `edge-verify`, [ADR-0032](../09-adr/0032-cel-checked-at-control-ir-at-edge.md) |
| Phân công zone cho node | MVP mọi node phục vụ mọi zone; khi nào chia shard | [5.8.6](../05-building-blocks/08-components/06-rollout-distribution-fleet.md) |
