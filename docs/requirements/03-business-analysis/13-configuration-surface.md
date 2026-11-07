# 3.13 Bề mặt cấu hình: rule và setting trên portal

> **Trạng thái: bước 1, đã quyết một nhóm tính năng (2026-11-07, [§4](#4-đã-quyết)).** Trang này mở theo nhận xét của chủ dự án:
> một sản phẩm WAAP kiểu Cloudflare cho cấu hình hầu hết mọi thứ bằng rule trên portal: whitelist, blacklist,
> rate limit, JA4, cookie challenge, trang chặn, cảnh báo tấn công (ngưỡng, format, kênh). Ngoài ra admin cần
> cấu hình nâng cao cho từng lớp L7 và WAF. Tài liệu hiện có liệt kê các chức năng này rời rạc ở F3–F8, nhưng
> chưa có mô hình cấu hình chung và chưa có cấu hình nâng cao cho admin. Danh mục ở §2 là **bản nháp của
> Claude để thảo luận, chờ duyệt**; trang này chưa phân tích phương án.

## 1. Hiện trạng: đã có gì, thiếu gì

| Mục | Đã có ở | Còn thiếu |
|---|---|---|
| Custom rule | F5.1.1; **CustomRule** lưu text biểu thức kèm AST ([kiến trúc 5.6](../../architecture/05-building-blocks/06-data-model.md)) | Chưa định nghĩa biểu thức gồm những trường, hàm, toán tử nào; chưa rõ JA4, bot score, cookie, kết quả WAF có là trường dùng được trong rule không |
| Whitelist, blacklist | F3.2.1 (IP, CIDR, quốc gia, ASN) | Chưa có list dùng lại giữa nhiều rule và nhiều zone; chưa có list JA4, list hostname |
| Rate limit | F3.4.1, F3.4.2 | Khoá đếm theo JA4 hoặc theo biểu thức; đếm theo response (ví dụ chỉ đếm `401`); thời gian phạt; hành động khi vượt ngưỡng |
| Challenge | F3.8.1 (JS, tương tác); [kiến trúc 8.16](../../architecture/08-cross-cutting/16-bot-management.md) | Cookie challenge (kiểm cookie, không cần JS) và managed challenge chưa thành lựa chọn của khách; TTL clearance do ai đặt |
| Trang chặn | F3.8.2 (Mở rộng) | Khách sửa được gì (HTML, JSON cho API, mã trạng thái, ngôn ngữ); an toàn của template |
| Cảnh báo | F8.2 | Ngưỡng theo từng loại tấn công; format nội dung; payload webhook; định tuyến theo mức |
| Transform, redirect, header | Không có trong §1 | Toàn bộ nhóm |
| Cấu hình nâng cao từng lớp cho admin | Không có | Toàn bộ nhóm |
| Bot | F3.6, [3.9](09-bot-management.md) | Phần cấu hình chưa nằm chung mô hình rule với WAF và rate limit |
| Thứ tự áp dụng | Pipeline ở [kiến trúc 5.8.1](../../architecture/05-building-blocks/08-components/01-l7.md) | Portal chưa cho khách thấy thứ tự này |

## 2. Danh mục cấu hình theo lớp (bản nháp, chờ duyệt)

Đi theo thứ tự request đi qua edge. Cột "Admin NCC" là phần chỉ NCC chỉnh được, hoặc NCC chỉnh trần cho
khách; có mở một phần cho Enterprise hay không là OQ-159.

| Lớp | Khách cấu hình | Admin NCC cấu hình thêm |
|---|---|---|
| **Kết nối, TLS** | Phiên bản TLS tối thiểu, HTTP/3, HSTS, tự chuyển HTTPS | Bộ cipher, giới hạn kết nối theo node, timeout bắt tay và giữ kết nối |
| **Chuẩn hoá request** | Giới hạn kích thước header, body (trong trần của gói) | Quy tắc chuẩn hoá URI (decode kép, path traversal), số header tối đa, xử lý request sai giao thức |
| **Lọc sớm** | Allow, block, challenge theo IP, CIDR, quốc gia, ASN; list dùng lại | Reputation feed, list toàn fleet |
| **DDoS L7** | Độ nhạy, hành động, Under Attack | Baseline tối thiểu, ngưỡng tuyệt đối theo cell, cách ly zone (F3.3.4) |
| **Rate limit** | Khoá đếm (IP, JA4, header, cookie, API key, biểu thức), cửa sổ, ngưỡng, hành động, thời gian phạt, đếm theo response | Trần số rule, độ chính xác của bộ đếm phân tán |
| **Bot, challenge** | Chế độ bot, ngưỡng điểm theo path, bot đã xác minh được phép, loại challenge (cookie, JS, tương tác), TTL clearance, allow/block theo JA4 | Trọng số tín hiệu, mô hình điểm, list JA4 xấu toàn fleet |
| **WAF managed** | Bật ruleset, paranoia, ngưỡng anomaly, đổi hành động theo rule hoặc tag, exception | Phát hành ruleset, rule khẩn toàn fleet, giới hạn kiểm body, timeout regex, trọng số anomaly mặc định |
| **Custom rule** | Biểu thức trên các trường, hành động, thứ tự, log-only, ngày hết hạn | Trường và hàm nào được mở, trần độ phức tạp |
| **API security** | Schema, JWT ([3.12](12-api-security.md)) | |
| **Response** | Trang chặn, trang challenge, trang lỗi origin `5xx`, response JSON cho API, mã trạng thái | Template mặc định, giới hạn của template |
| **Transform** | Thêm, xoá header request và response, rewrite URL, redirect, gửi thông tin cho origin (IP thật, bot score) | |
| **Cache, origin** | Xem [3.10](10-cache-performance.md) | |
| **Cảnh báo** | Loại sự kiện, ngưỡng, cửa sổ, mức, kênh, định tuyến theo mức, template nội dung, giờ im lặng, gộp | Cảnh báo toàn fleet, kênh của NCC |
| **Log** | Đẩy log, chọn trường, lấy mẫu, che dữ liệu cá nhân | Thời gian lưu theo gói |

## 3. Kịch bản theo từng trục

### A. Mô hình rule chung

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-A1 | Một ngôn ngữ biểu thức cho mọi loại rule (custom, rate limit, bot, transform, cache) | Học một lần, dùng mọi nơi; nhưng phải thiết kế, viết tài liệu, giữ tương thích lâu dài |
| KB13-A2 | Mỗi tính năng có form riêng | Dễ cho khách nhỏ; nhưng mỗi tính năng một kiểu, khó kết hợp điều kiện |
| KB13-A3 | Khách nhỏ không viết biểu thức | Form builder sinh ra biểu thức; biểu thức sửa tay có quay lại form được không |
| KB13-A4 | Một trường như JA4 phải dùng được ở custom rule, rate limit và bot | Chỉ có được nếu các tính năng chung mô hình |
| KB13-A5 | Thêm trường mới sau này (ví dụ kết quả kiểm schema API) | Rule cũ không vỡ; edge cũ chưa biết trường mới |

### B. Thứ tự và tương tác giữa các phase

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-B1 | IP trong whitelist có bỏ qua WAF, rate limit, bot không | Whitelist bỏ qua những phase nào phải nói rõ; bỏ qua hết thì IP bị chiếm là thành lỗ hổng |
| KB13-B2 | Hành động "skip" một phase cho một path | Khách tắt nhầm cả WAF |
| KB13-B3 | Hai rule mâu thuẫn (một allow, một block) | Thứ tự ưu tiên, first match hay theo phase |
| KB13-B4 | Khách hỏi "vì sao request này bị chặn" | Cần trace: request đi qua phase nào, rule nào khớp |

### C. Phạm vi áp dụng

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-C1 | Rule cấp account áp cho mọi zone (chặn một quốc gia cho cả công ty) | Kế thừa F5.1.7; zone ghi đè được không |
| KB13-C2 | Rule chỉ cho một hostname hoặc một path | |
| KB13-C3 | Agency quản lý nhiều org muốn dùng chung một bộ rule | Template chia sẻ giữa các org |

### D. Giới hạn và an toàn của rule

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-D1 | Số rule theo gói | Trần, thông báo khi chạm trần |
| KB13-D2 | Regex gây backtracking, làm chậm edge | Từ chối khi biên dịch, hoặc giới hạn thời gian chạy |
| KB13-D3 | Rule đắt (kiểm body lớn trên mọi request) | Đo chi phí theo rule, cảnh báo khách |
| KB13-D4 | Rule của một khách làm chậm khách khác cùng node | Cô lập chi phí theo tenant |

### E. Kiểm thử trước khi áp

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-E1 | Log-only trước khi chặn | Đã có ở F5.1.6; áp cho mọi loại rule hay chỉ WAF |
| KB13-E2 | Thử rule với một request mẫu | Công cụ dán request, xem kết quả |
| KB13-E3 | Mô phỏng trên traffic lịch sử | F5.2.3, Mở rộng |
| KB13-E4 | Xem trước trang chặn trước khi phát hành | |

### F. Trang chặn và response tuỳ biến

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-F1 | Khách sửa HTML tự do | Chèn script chạy trên domain của khách; trang chặn bị dùng để lừa đảo |
| KB13-F2 | Trang tải ảnh, CSS từ nguồn ngoài | Nguồn ngoài sập hoặc theo dõi người dùng |
| KB13-F3 | API cần JSON chứ không cần HTML | Chọn theo path hoặc theo `Accept` |
| KB13-F4 | Đa ngôn ngữ (Việt, Anh) | Theo `Accept-Language` |
| KB13-F5 | Người dùng bị chặn cần báo cho khách | Request ID và thông tin liên hệ trên trang |
| KB13-F6 | Mã trạng thái: `403`, `429`, `503` | Khách muốn đổi mã; SEO và crawler hiểu sai |

### G. Cảnh báo tấn công

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-G1 | Khách đặt ngưỡng riêng cho từng loại (DDoS, WAF, bot, rate limit) | Hay chỉ chọn mức nhạy cho đơn giản |
| KB13-G2 | Ngưỡng tĩnh hay theo baseline | Baseline là F8.1.2, ở mức Nghiên cứu |
| KB13-G3 | Khách sửa nội dung cảnh báo bằng template có biến (zone, loại, số request, link) | Kênh có giới hạn độ dài khác nhau |
| KB13-G4 | Payload webhook phải ổn định | Có version, có chữ ký để khách xác minh |
| KB13-G5 | Kênh trong nước: Zalo, Telegram | F8.2.2 chưa có |
| KB13-G6 | Định tuyến: mức cao gọi điện, mức thấp gửi email; theo zone; theo giờ | |
| KB13-G7 | Bão cảnh báo khi bị tấn công kéo dài | Gộp, giới hạn tần suất |
| KB13-G8 | Kênh hỏng, cảnh báo không tới | Báo lại qua kênh khác |

### H. Cấu hình nâng cao cho admin

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-H1 | Admin NCC chỉnh tham số engine từng lớp (giới hạn kiểm body, timeout regex, chuẩn hoá URI) | Phạm vi áp: fleet, cell, tenant hay zone |
| KB13-H2 | Admin nâng trần cho một tenant (body lớn hơn gói) | Ghi lại như ngoại lệ hợp đồng, có hạn |
| KB13-H3 | Khách Enterprise muốn tự chỉnh một phần (giới hạn body, chuẩn hoá) | Mở cho khách đến đâu |
| KB13-H4 | Sửa sai tham số làm hỏng traffic cả fleet | Duyệt hai người (OQ-118), rollout từng bước, rollback |
| KB13-H5 | Công tắc khẩn: tắt một rule managed trên toàn fleet | Đã có ý ở 3.11, cần nằm trong cùng mô hình |

### I. Preset và template

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-I1 | Preset theo ngành | Gắn với OQ-124 (bot), OQ-144 (cache) |
| KB13-I2 | Thư viện rule mẫu (chặn `wp-login.php`, chặn quốc gia, giới hạn đăng nhập) | Ai duy trì; rule mẫu cũ đi |
| KB13-I3 | Khách nhỏ không biết bắt đầu từ đâu | Cấu hình mặc định an toàn ngay khi onboard |

### J. Cách áp và kênh cấu hình

| Mã | Kịch bản | Concern |
|---|---|---|
| KB13-J1 | Setting nào đi qua change request (BR-25), setting nào áp ngay (kênh cảnh báo, trang chặn) | Lớp áp dụng của [ADR-0037](../../architecture/09-adr/0037-config-apply-classes.md) |
| KB13-J2 | Mọi mục ở §2 có API, Terraform không | UI là nguồn sự thật (OQ-35 đã quyết) |
| KB13-J3 | Thay đổi một list dùng chung ảnh hưởng nhiều zone | Diff phải cho thấy zone nào bị ảnh hưởng |

## 4. Đã quyết

Chủ dự án đưa danh sách tính năng theo từng plane, Claude nêu các điểm cần chỉnh, chủ dự án đồng ý cả hai
(2026-11-07). Yêu cầu tương ứng ở [§2](../02-requirements.md); thiết kế ở các component 5.8.x.

| Nhóm | Quyết định | Yêu cầu |
|---|---|---|
| Trang lỗi tuỳ biến | Entity **CustomErrorPage** theo tenant hoặc zone, map với từng loại lỗi (403 WAF, 429, 502, 504, 503 bảo trì, trang challenge); path API trả JSON. HTML, CSS tĩnh: không JavaScript, không tài nguyên ngoài, ảnh nhúng có trần dung lượng; edge tự chèn request ID; xem trước được | FR-3-16 |
| Emergency override | Ở account cần duyệt, áp ngay vào fast lane, chỉ cho thay đổi siết chặt, có hạn, duyệt bù, cảnh báo critical tới Owner, Admin | FR-5-21, BR-27 |
| Trust và Skip | Hai hành động tách biệt: Trust bỏ qua bot và challenge, WAF vẫn kiểm, vẫn chịu DDoS L7 và rate limit; Skip bỏ qua đúng một chặng. Googlebot đi qua bot đã xác minh, không theo IP hay User-Agent | FR-3-15, BR-28 (đóng KB13-B1) |
| Xuất audit log | Đẩy liên tục sang SIEM, gồm thao tác của NCC, có chuỗi hash | FR-1-19 |
| Tiến độ rollout | Realtime trên portal, nêu lý do khi dừng. Policy của khách không tự rollback theo tỉ lệ chặn hay traffic (một rule block làm tăng 403 là đúng ý khách); bản phát hành của NCC thì có | FR-5-13, FR-6-04, BR-29 |
| Dry-run cho CI/CD | Endpoint validate không lưu, cùng checker và `edge-verify`, có quota | FR-5-20 |
| Origin failover | Active-passive, round-robin, theo trọng số; health check chủ động và thụ động; không gửi lại request không idempotent | FR-4-05 |
| Tarpit | Có, với trần số kết nối giữ trên mỗi node, vượt trần thì block; không dùng làm cách chính chống credential stuffing (cách chính là rate limit endpoint đăng nhập cộng challenge) | FR-3-17 |
| IP thật sau CDN | Khai header và dải IP CDN; chỉ tin header từ dải đó; đọc `X-Forwarded-For` từ phải sang; preset CDN phổ biến | FR-3-14, BR-31 |
| Xử lý một bấm | Link trong thông báo mở portal, đăng nhập, xác nhận; không bao giờ tự thực thi khi mở (trình quét link tự mở link) | FR-8-09, BR-30 |
| Live tail | Lọc theo rule ID, IP, path, hành động, gồm hit log-only; lọc ở Realtime gateway trên luồng live đã có; theo quyền xem dữ liệu request | FR-7-10 |
| Chống bão cảnh báo | Dedup theo alert, zone, nguồn trong cửa sổ 30 phút, gom vào thread hoặc digest; mức tăng hoặc nguồn mới phá dedup | FR-8-08 |
| Lọc khi export | Theo hành động, loại event, zone, trường, lấy mẫu log allow; bản lọc nâng cao dùng cùng ngôn ngữ biểu thức của rule thay vì regex | FR-7-11 |

**Nghiệp vụ lõi của pipeline (quyết 2026-11-07, đợt hai).** Chủ dự án nêu 6 điểm (thứ tự pipeline, xung đột
policy, bot, API, origin, kiểm thử rule); Claude đối chiếu với tài liệu, đề xuất A–G, chủ dự án đồng ý:

| Mục | Quyết định | Yêu cầu |
|---|---|---|
| A. Allowlist | Giữ BR-28: Trust không bỏ qua rate limit và WAF; cần bỏ qua nhiều chặng thì dùng Skip từng chặng, hiện rõ trên UI | BR-28 |
| B. Thứ tự pipeline | Rẻ trước đắt sau; DDoS L7 tách lên ngay sau access list, trước challenge; rate limit theo rule trước WAF | BR-32, FR-3-19 |
| C. Xung đột rule | Priority từ trên xuống, dừng ở hành động kết thúc đầu tiên; access list: cụ thể hơn thắng; block của NCC > allow của khách > block của khách > allow của NCC; support đặt log-only có hạn cho rule khẩn chặn nhầm một zone | BR-33, FR-12-07 (đóng KB13-B3) |
| D. Bot score | Bảng hành động mặc định theo điểm và nhóm path; managed challenge mặc định | BR-34 (đóng một phần OQ-155) |
| E. API | Schema chạy trước managed ruleset trong tầng WAF; endpoint enforce schema được giảm paranoia; rate limit theo API key, JWT lên MVP | BR-35, FR-3-06 |
| F. SSL passthrough | Mở rộng, Enterprise, theo hostname, UI ghi rõ "không được kiểm" | FR-3-18 |
| G. Log-only sang chặn | Không tự chuyển: báo cáo sau thời gian quan sát, người dùng bấm, qua change request | BR-36 |

Chi tiết Claude thêm khi ghi, **chờ duyệt**: DDoS L7 không skip được; emergency override mặc định hạn 24 giờ;
cửa sổ dedup mặc định 30 phút.

## 5. Câu hỏi cần chủ dự án quyết

| ID | Câu hỏi | Kịch bản | Ghi chú ban đầu |
|---|---|---|---|
| OQ-151 | **Đã quyết: một ngôn ngữ chung** ([3.14](14-config-tiers-attack-history-thresholds.md#24-các-câu-hỏi-cũ-đã-trả-lời)). Một ngôn ngữ biểu thức chung cho mọi loại rule, hay form riêng cho từng tính năng? | KB13-A1…KB13-A5 | Cloudflare dùng một ngôn ngữ chung, form builder sinh ra biểu thức. Custom rule đã dùng CEL ([ADR-0032](../../architecture/09-adr/0032-cel-checked-at-control-ir-at-edge.md)); câu hỏi là có mở rộng CEL cho mọi loại rule không |
| OQ-152 | Biểu thức ở MVP gồm những trường nào (IP, quốc gia, ASN, JA4, User-Agent, URI, header, cookie, body, bot score, kết quả WAF)? | KB13-A4 | |
| OQ-153 | Có list dùng lại cấp account (IP, ASN, JA4, hostname) không? | KB13-C1, KB13-J3 | |
| OQ-154 | Rate limit ở MVP có khoá đếm theo biểu thức, đếm theo response và thời gian phạt không? | §2 | |
| OQ-155 | Khách được chọn những loại challenge nào (cookie, JS, tương tác, managed)? TTL clearance do khách đặt hay cố định? | §2 | Gắn với 3.9 |
| OQ-156 | Trang chặn: khách sửa HTML tự do, hay chỉ điền template (logo, màu, nội dung, liên hệ)? Gói nào? | KB13-F1…KB13-F6 | **Đã quyết một phần** ở §4: tải HTML, CSS tĩnh, không JavaScript; còn mở gói nào có |
| OQ-157 | Có nhóm transform rule (header, rewrite, redirect) ở v1 không? | §2 | |
| OQ-158 | Cảnh báo: ngưỡng riêng từng loại hay chỉ mức nhạy? Có template nội dung không? Có kênh Zalo, Telegram không? | KB13-G1…KB13-G8 | **Đã quyết một phần** ở [3.14](14-config-tiers-attack-history-thresholds.md#23-cảnh-báo-và-ngưỡng): có ngưỡng ba mức, có template, ngưỡng ở tầng zone. Còn mở: Zalo, Telegram |
| OQ-159 | Cấu hình nâng cao từng lớp: những tham số nào, phạm vi áp ở đâu, chỉ admin NCC hay mở một phần cho Enterprise? | KB13-H1…KB13-H5 | Có đề xuất ở [3.14 §3.2](14-config-tiers-attack-history-thresholds.md#32-tham-số-nâng-cao-theo-lớp-các-sản-phẩm-waap-cho-cấu-hình-gì) và ba tầng ở [§2.1](14-config-tiers-attack-history-thresholds.md#21-ba-tầng-cấu-hình), chờ duyệt |
| OQ-160 | Setting nào đi qua change request, setting nào áp ngay? | KB13-J1 | |
| OQ-161 | Có công cụ giải thích "vì sao request này bị chặn" cho khách không? | KB13-B4 | |
| OQ-162 | Trần số rule và độ phức tạp theo gói? | KB13-D1…KB13-D4 | Gắn với 3.5 |

## 6. Phụ thuộc

F3, F5, F8 ở [§1](../01-functional-decomposition.md); [3.2](02-policy-change.md), [3.4](04-false-positive.md),
[3.9](09-bot-management.md), [3.10](10-cache-performance.md), [3.12](12-api-security.md); pipeline ở
[kiến trúc 5.8.1](../../architecture/05-building-blocks/08-components/01-l7.md).
