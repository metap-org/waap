# 3.14 Cấu hình nhiều tầng, lịch sử tấn công, ngưỡng cảnh báo

> **Trạng thái: chủ dự án đã quyết hướng (2026-11-07); các đề xuất của Claude được chốt làm quyết định làm việc, "có vấn đề gì tính sau" (2026-11-07). Phần chi tiết và luồng chưa viết.** Trang này ghi lại
> các tính năng sẽ có để không bị quên. Theo yêu cầu của chủ dự án, **luồng vẽ và phân tích cụ thể làm sau**
> ([§5](#5-luồng-cần-vẽ-chưa-viết)). Phần "đề xuất của Claude" ở [§3](#3-đề-xuất-của-claude-đã-chốt-làm-quyết-định-làm-việc-đổi-sau-được) chưa phải quyết định.

## 1. Phạm vi

Ba nhóm nghiệp vụ lộ ra khi chủ dự án xem prototype portal Management Plane: cấu hình có nhiều tầng (system,
global, website), lịch sử tấn công chia theo loại, và cảnh báo có ngưỡng. Liên quan: F5, F7, F8, F11 ở
[§1](../01-functional-decomposition.md); [3.13](13-configuration-surface.md) (bề mặt cấu hình);
[3.8](08-monitoring-investigation.md) (giám sát, được mở lại một phần).

## 2. Đã quyết (chủ dự án, 2026-11-07)

### 2.1 Ba tầng cấu hình

| Tầng | Là gì | Phạm vi áp dụng |
|---|---|---|
| **System** | Bộ rule managed (CRS) mặc định của NCC | Mặc định cho mọi website. Mỗi lần cập nhật phát ra toàn bộ website, có chiến lược canary (bàn sau) |
| **Global** | Cấu hình áp cho mọi website, cộng với các **tập rule custom của NCC** | Chọn tập rule để dùng cho từng domain, hoặc cho cluster, node |
| **Website** | Rule của riêng một website | Chỉ áp cho website đó; số rule theo gói subscription |

- **Tầng trên ghi đè tầng dưới.** Global thắng website.
- **Whitelist luôn được ưu tiên** trong các cặp whitelist, blacklist. Ranh giới của ưu tiên này: [OQ-163](#4-đã-chốt-theo-đề-xuất-của-claude).
- Tầng nào ai được sửa: chủ dự án giao Claude phân tích ([§3.1](#31-ai-sửa-tầng-nào)); đã chốt theo đề xuất.

### 2.2 Lịch sử tấn công

- Có **hai loại đợt tấn công**: **DDoS L7** và **vuln-scan** (rà quét lỗ hổng, về bản chất là WAF chặn rồi gửi event).
  Mỗi cuộc tấn công thuộc một trong hai loại là **một bản ghi** (không phải từng request).
- Cả hai đều có **ngưỡng cấu hình**; chạm ngưỡng mới lên cảnh báo.
- Có **một màn riêng cho event WAF**: xem chi tiết từng event và rule vi phạm; nếu là chặn nhầm thì tuning ngay từ
  event (nối với F7.5).
- **Lịch sử giữ theo gói.** Khách xem được nếu gói cho phép. Admin, NOC và người vận hành nào được phân quyền
  cũng xem được. Phần gói cần phân tích thêm ([§3.4](#34-lịch-sử-theo-gói)).
- DDoS và WAF được tách (đợt DDoS và đợt vuln-scan là hai loại; event WAF là màn riêng).

### 2.3 Cảnh báo và ngưỡng

- Mọi loại tấn công có **ngưỡng mặc định**, chia **ba mức: low, medium, high**.
- **Template nội dung thông báo** có chắc chắn, khách chỉnh được, có template mẫu mặc định.
- **Ngưỡng đặt ở tầng zone.**
- Định tuyến thông báo: chủ dự án giao Claude tự quyết ([§3.5](#35-định-tuyến-thông-báo-claude-quyết-theo-uỷ-quyền)).

### 2.4 Các câu hỏi cũ đã trả lời

| Câu | Kết luận |
|---|---|
| OQ-151 | **Một ngôn ngữ biểu thức chung** cho mọi loại rule (CEL, [ADR-0032](../../architecture/09-adr/0032-cel-checked-at-control-ir-at-edge.md)) |
| OQ-118 | Thao tác nào cần người duyệt thuộc **nghiệp vụ phân quyền**, có tính năng rồi mới tính. Không chặn các việc trên |
| OQ-158 | Một phần: ngưỡng riêng từng loại có mặc định (low, medium, high), template có và chỉnh được, ngưỡng ở tầng zone. Còn mở: kênh Zalo, Telegram |

## 3. Đề xuất của Claude (đã chốt làm quyết định làm việc, đổi sau được)

### 3.1 Ai sửa tầng nào

| Tầng | Ai sửa | Ai đọc | Duyệt |
|---|---|---|---|
| System (CRS, ruleset NCC) | Threat research của NCC | NCC; khách chỉ thấy phiên bản đang chạy và ghi chú phát hành | Rollout theo wave, log-only trước khi chặn ([FR-11-01](../02-requirements.md#f6-phân-phối-cấu-hình-và-f11-threat-intelligence)) |
| Global (cấu hình chung, tập rule custom của NCC, gán cho domain, cluster, node) | Admin NCC, SOC của NCC có quyền | NCC; khách thấy tập rule nào đang áp cho website của mình, không thấy nội dung rule | Hai người cho thao tác ảnh hưởng nhiều website (chờ phần phân quyền) |
| Website | Khách (theo vai trò); support NCC khi khách cấp quyền | Khách | Theo loại tài khoản: self-service tự phát hành, doanh nghiệp cần duyệt ([BR-26](../02-requirements.md#4-quy-tắc-nghiệp-vụ)) |

Gợi ý hệ quả: rule khách có thể bị rule global đè nhưng khách phải **nhìn thấy** vì sao (rule nào, tầng nào),
nếu không sẽ không gỡ lỗi được. Điều này nối với công cụ giải thích của [OQ-161](13-configuration-surface.md#5-câu-hỏi-cần-chủ-dự-án-quyết).

### 3.2 Tham số nâng cao theo lớp: các sản phẩm WAAP cho cấu hình gì

Tổng hợp theo hiểu biết của Claude về tài liệu công khai của Cloudflare, Akamai, AWS WAF, Imperva, F5, Fastly
(Signal Sciences) và Azure WAF; **chưa tra lại từng nhà**. Cột cuối là đề xuất: **K** = khách chỉnh, **E** =
Enterprise chỉnh, **A** = chỉ admin NCC.

| Lớp | Tham số hay gặp | Ai |
|---|---|---|
| Kết nối, TLS | Phiên bản TLS tối thiểu, bộ cipher, HTTP/2, HTTP/3, HSTS, mTLS từ client, timeout bắt tay và giữ kết nối | K: TLS tối thiểu, HSTS; E: mTLS; A: cipher, timeout |
| Giới hạn request | Kích thước header, URL, body, số tham số, độ sâu JSON, method cho phép, content-type cho phép | K: trong trần của gói; A: trần |
| Chuẩn hoá | Giải mã URL nhiều lần, chuẩn hoá unicode, xử lý request mơ hồ (smuggling), HTTP version | A (E đọc) |
| DDoS L7 | Độ nhạy, ngưỡng tuyệt đối theo req/s, đo theo IP, ASN, quốc gia, cửa sổ học baseline, thời gian giữ biện pháp, đường dẫn loại trừ | K: độ nhạy, hành động, loại trừ; E: ngưỡng tuyệt đối; A: baseline |
| Rate limit | Khoá đếm, cửa sổ, ngưỡng, hành động, thời gian phạt, đếm theo mã response | K |
| Bot | Độ nhạy, danh sách JA4 cho phép, chặn, loại bot đã xác minh, loại challenge, TTL clearance, loại trừ file tĩnh | K; A: trọng số tín hiệu |
| WAF managed | Chọn ruleset, ghim phiên bản hay tự cập nhật, paranoia level, ngưỡng anomaly vào và ra, đổi hành động theo rule hoặc nhóm, tắt rule, exception theo phạm vi | K: paranoia, ngưỡng, exception; E: ghim phiên bản; A: phát hành |
| Kiểm body | Giới hạn kích thước kiểm, hành vi khi vượt (kiểm một phần, bỏ qua, chặn), bật tắt parser JSON, XML, multipart, timeout regex | K: hành vi khi vượt; A: timeout |
| API | Chế độ schema, hành động với endpoint lạ, JWT (issuer, audience, lệch giờ), GraphQL (độ sâu, độ phức tạp, introspection) | K |
| Response | Trang lỗi, mã trạng thái, header thêm bớt, CORS | K |
| Origin | Timeout kết nối và đọc, retry, health check (chu kỳ, ngưỡng), xác minh TLS tới origin, SNI, Host header, kích thước pool | K: phần lớn; A: trần |
| Cache | TTL, bỏ qua, xử lý query string, stale-if-error, tiered cache | K |
| Log | Lấy mẫu access log, trường được ghi, che dữ liệu cá nhân, retention | K trong trần gói; A: trần |

Nguyên tắc đề xuất: tham số **làm giảm an toàn** (nới giới hạn, tắt kiểm) chỉ chỉnh được trong trần của gói
và có audit; tham số **ảnh hưởng nhiều khách** (chuẩn hoá, cipher, timeout engine) chỉ admin NCC.

### 3.3 Hành động DDoS L7 cho khách

| Hành động | Khi nào dùng | Ghi chú |
|---|---|---|
| Chỉ ghi log | Giai đoạn quan sát, ngưỡng mới | Không áp dụng ở tuần đầu: [BR-24](../02-requirements.md#4-quy-tắc-nghiệp-vụ) chặn DDoS từ request đầu tiên |
| Challenge (managed) | **Mặc định** | Người thật qua được, công cụ flood thì không |
| Giới hạn tốc độ nguồn | Nguồn có tỉ lệ cao nhưng chưa chắc xấu (đối tác, proxy) | Throttle thay vì chặn |
| Chặn có thời hạn | Nguồn đã rõ ràng | Thời hạn 10 phút, tăng dần nếu quay lại; tự hết hạn |
| Chặn theo ASN, quốc gia tạm thời | Flood từ một nhà mạng | Qua kênh khẩn, có hạn ([BR-27](../02-requirements.md#4-quy-tắc-nghiệp-vụ)) |
| Under Attack mode | Toàn zone đang bị flood | Challenge mọi request chưa có clearance; tự bật có hẹn giờ tắt chưa chốt ([ADR-0008](../../architecture/09-adr/0008-under-attack-mode.md)) |
| Tarpit | Nguồn dò đăng nhập | Có trần theo node (đã quyết) |
| Chỉ phục vụ từ cache | Origin quá tải do flood | Chỉ khi zone có cache; cảnh báo khách |

Khách chọn: độ nhạy (low, medium, high), hành động chính, thời gian giữ biện pháp, danh sách loại trừ (Trust).
Ngưỡng tuyệt đối cho Enterprise.

### 3.4 Lịch sử theo gói

Hai kho có chi phí rất khác nhau, nên giữ khác nhau:

| Kho | Kích thước | Free | Pro | Business | Enterprise |
|---|---|---|---|---|---|
| **Đợt tấn công** (DDoS L7, vuln-scan; một bản ghi mỗi đợt) | Nhỏ | 30 ngày | 90 ngày | 12 tháng | 24 tháng, tuỳ hợp đồng |
| **Event WAF** (từng request) | Lớn | 3 ngày | 14 ngày | 30 ngày | 90 ngày, lâu hơn qua archive |

Các con số là điểm khởi đầu để thảo luận, không dựa trên dữ liệu chi phí thật. Nguyên tắc: bản ghi đợt tấn công
rẻ, nên giữ lâu hơn nhiều so với event; security event không bao giờ lấy mẫu ([8.18](../../architecture/08-cross-cutting/18-segments-and-plans.md)).
Quyền xem: khách theo vai trò, nhân viên NCC theo quyền được cấp (NOC, admin, SOC), xem dữ liệu request của
khách cần lý do và để lại audit ([BR-20](../02-requirements.md#4-quy-tắc-nghiệp-vụ)).

**Đợt vuln-scan** cần định nghĩa gom event: đề xuất cùng nguồn (IP hoặc ASN) trong cửa sổ 10 phút, vượt
ngưỡng số event hoặc số rule khác nhau thì mở đợt; im lặng 30 phút thì đóng.

### 3.5 Định tuyến thông báo (Claude quyết theo uỷ quyền)

Chủ dự án giao "m tự quyết", nên ghi như quyết định, đổi lại dễ:

| Mức | Kênh mặc định | Nhịp |
|---|---|---|
| High | Chat và email tới Owner, người trực; PagerDuty hoặc SMS nếu khách bật | Tức thì; không ack trong 15 phút thì leo thang người kế tiếp. Giờ im lặng **không** chặn mức high |
| Medium | Chat và email tới nhóm bảo mật của zone | Tức thì, gộp theo [FR-8-08](../02-requirements.md#f8-phát-hiện-cảnh-báo-sự-cố) |
| Low | Hiện trên portal; email tóm tắt hằng ngày | Digest |

Người nhận theo vai trò và theo zone (ghi đè được ở cấp zone); mọi kênh của cảnh báo high lỗi thì gửi email tới
liên hệ bảo mật của account (đã có ở 5.8.11).

## 4. Đã chốt theo đề xuất của Claude

Chủ dự án: "làm theo đề xuất của bro trước, có vấn đề gì tính sau; phần cần chốt mày cũng đề xuất luôn". Ba câu dưới đây chốt theo đề xuất, ghi là quyết định làm việc và có thể đảo.

| ID | Câu hỏi | Quyết định làm việc |
|---|---|---|
| OQ-163 | "Whitelist luôn ưu tiên" có thắng cả **rule khẩn của NCC** (chặn 0-day, CVE) không? Và nó chỉ thắng blacklist, hay cũng bỏ qua WAF, rate limit, DDoS? | **Chốt:** whitelist thắng blacklist cùng tầng; whitelist của website **không** thắng tầng trên (global, system) và không bỏ qua WAF, rate limit, DDoS (giữ [BR-28](../02-requirements.md#4-quy-tắc-nghiệp-vụ)); **rule khẩn của NCC không bị whitelist nào đè** (whitelist một IP bị chiếm sẽ mở lỗ hổng 0-day), chỉ support đặt log-only có hạn cho một zone ([FR-12-07](../02-requirements.md#f12-vận-hành-ncc)); whitelist của **global** thắng blacklist của website và CRS, có audit. Bảng đủ 7 trường hợp có trong prototype portal, màn "Ba tầng cấu hình" |
| OQ-164 | Có tầng **account (tổ chức)** cho khách không (một bộ rule dùng cho nhiều website của cùng khách, như agency 50 site)? | Ba tầng anh nêu đều của NCC hoặc của một website. Tài liệu cũ có kế thừa org → nhóm zone → zone (FR-5-18, Mở rộng). **Chốt:** MVP **không có tầng account**; tầng website chỉ là tập rule của một website. Khách nhiều site dùng **mẫu policy** (PolicyTemplate, sao chép hoặc liên kết vào từng website) ở mức Mở rộng; mẫu không thêm tầng ưu tiên mới, rule trong mẫu vẫn tính vào trần theo gói của từng website |
| OQ-165 | Tập rule custom của NCC gán theo **node, cluster** được: khách có thấy và chọn không, hay chỉ NCC gán? | Gán theo node liên quan hạ tầng (cell, PoP); **Chốt:** chỉ NCC gán; khách chỉ thấy tập nào đang áp cho website của mình, không thấy nội dung rule |

## 5. Luồng cần vẽ (chưa viết)

Theo yêu cầu của chủ dự án, các luồng sau cần vẽ và phân tích cụ thể, **chưa làm**:

1. Hợp nhất ba tầng khi biên dịch: thứ tự, ghi đè, whitelist, xung đột, kết quả cho khách thấy.
2. Phát ruleset mới ở tầng system tới toàn bộ website (canary bàn riêng, nối [8.15](../../architecture/08-cross-cutting/15-config-propagation-and-apply.md)).
3. Tạo tập rule global và gán cho domain, cluster, node.
4. Áp giới hạn số rule theo gói ở tầng website.
5. Vòng đời một đợt tấn công: mở, leo thang, giảm thiểu, đóng; cả hai loại.
6. Từ ngưỡng đến thông báo: đánh giá, mức low, medium, high, định tuyến, template.
7. Từ event WAF đến tuning chặn nhầm.

## 6. Phụ thuộc

F5, F7, F8, F11 ở [§1](../01-functional-decomposition.md); [3.13](13-configuration-surface.md), [3.9](09-bot-management.md);
[kiến trúc 8.15](../../architecture/08-cross-cutting/15-config-propagation-and-apply.md) (lớp phát cấu hình),
[5.8.9](../../architecture/05-building-blocks/08-components/09-query-detection-export-replay.md) (detection),
[5.8.10](../../architecture/05-building-blocks/08-components/10-alerting-rules-and-algorithms.md) (cảnh báo).
