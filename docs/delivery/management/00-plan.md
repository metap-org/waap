# Kế hoạch thực thi Management plane

> **Trạng thái (2026-11-21, S2 cập nhật 2026-11-28): đã làm một phần lớn S0 đến S3; xem [mục 7](#7-trạng-thái-thực-tế-2026-11-21).**
> Bản đề xuất ban đầu (2026-11-18) ghi "chưa có dòng mã nào"; điều đó không còn đúng từ khi các lát cắt được làm theo `metap` Go (roadmap 106 đến 117 ở `metap-docs`).
> [Use case](01-use-cases.md) và [thiết kế database](02-database.md) vẫn là đặc tả; mục 7 là đối chiếu use case với mã.

## 1. Cách làm

**Cắt dọc, không cắt ngang.** Mỗi lát cắt (S0, S1, ...) đi từ database tới màn hình và API, và **dùng được ngay**
với một nhóm người dùng thật. Không làm "toàn bộ database trước, toàn bộ API sau".

Ba nguyên tắc:

1. **Chỉ làm tính năng mức MVP** của bản đồ phân rã (cột "Mức"). Mở rộng (MR) và Nâng cao (NC) để sau; thiết kế DB chỉ chừa chỗ, không tạo bảng.
2. **Phần của plane khác thì dựng giả (stub) sau một hợp đồng cố định.** Management không chờ Control, Edge, Analytics; mỗi chỗ gọi sang plane khác đi qua một cổng
   (gRPC action hoặc sự kiện outbox, [5.5](../../architecture/05-building-blocks/05-contracts.md)) và có bản giả trả kết quả đặt trước. Thay bản giả bằng bản thật không đổi mã Management.
3. **Mỗi thay đổi dữ liệu có audit và outbox ngay từ lát cắt đầu.** Đây là mặc định của `metap` (audit sau commit, outbox cùng transaction), không phải việc làm thêm; chỉ cần khai báo đúng.

## 2. Các lát cắt

Thứ tự theo phụ thuộc: cái sau cần cái trước.

| Lát cắt | Giá trị trao cho người dùng | Tính năng (mã trong [bản đồ](../../requirements/01-functional-decomposition.md)) | Cần stub của |
|---|---|---|---|
| **S0 Nền** | Tạo được org, mời người, đăng nhập, phân quyền, có audit | F1.1.1–2, F1.2.1–2, F1.2.4, F1.2.6, F1.2.8, F1.3.1, F1.3.4, F1.4.1–3, F1.5.1–2 | — |
| **S1 Zone và onboarding** | Thêm site, xác minh, khai origin, kích hoạt ở chế độ log-only | F2.1.1, F2.1.3, F2.1.8–10, F2.2.1–2, F2.2.4–5, F2.3.4, F2.4.1–3, F2.5.1, F2.6.1, F2.7.1–2, F2.7.4–7 | Control: xác minh domain, cert, kiểm origin, tra DNS |
| **S2 Policy và thay đổi** | Soạn rule, gửi, duyệt, phát hành, xem version, rollback, bật Under Attack | F5.1.1–6, F5.2.1–2, F5.3.1, F5.3.4–6, F5.4.1–4, F5.5.1–2 | Control: biên dịch thử, trạng thái rollout |
| **S3 Vận hành bảo mật** | Xem sự kiện chặn, báo và xử lý chặn nhầm, cảnh báo, case, mitigation | F7.3.3, F7.5.1, F7.5.3, F8.2.1–7, F8.4.1–5, F8.4.7 | Analytics: truy vấn sự kiện, gửi thông báo |
| **S4 Gói và thanh toán** | Chọn gói, thanh toán, nâng và hạ gói, hạn mức | F10.1–F10.6 (MVP) | Cổng thanh toán, Analytics: usage |
| **S5 Rời dịch vụ và dữ liệu** | Xuất, xoá org, khôi phục, chuyển zone | F1.1.4–6, F2.1.12, F2.4.3 | — |
| **S6 NCC và SSO** | NCC quản trị, support, SSO | F1.3.2–3, F1.3.5–6, F12.3.2, F12.4.1–3 | IdP |

S0 và S1 là **đường găng**: mọi thứ sau cần org, người, zone. S2 là giá trị cốt lõi của sản phẩm. S3–S6 có thể đổi thứ tự theo
khách đầu tiên cần gì (đề xuất: S3 trước S4, vì bán gói chưa cần khi chưa có khách trả tiền).

## 3. Cổng giữa Management và plane khác (stub)

Cố định trước khi viết S1/S2, để hai bên làm song song. Chi tiết trường ở [5.5](../../architecture/05-building-blocks/05-contracts.md) (tên field còn tạm).

| Cổng | Hướng | Dùng ở | Bản giả trả về |
|---|---|---|---|
| `VerifyDomain(zone)` | M → C | UC-21 | đạt sau 3 giây; có công tắc ép "không đạt" để thử lỗi |
| `CheckOrigin(origin)` | M → C | UC-23 | đạt/không đạt theo địa chỉ thử (ví dụ `*.fail.test` không đạt) |
| `LookupDns(domain)` | M → C | UC-20 | trả DNS mẫu (A, NS, CAA) |
| `CompilePolicy(draft)` | M → C | UC-42 | biên dịch được nếu qua kiểm cú pháp ở Management |
| `PolicyPublished` (outbox) | M → C | UC-46 | Control giả đánh dấu "đã áp" sau vài giây |
| `RolloutProgress` (sự kiện) | C → M | UC-47 | tiến độ giả 0 → 100% |
| `QueryEvents(zone, filter)` | M → A | UC-60 | dữ liệu mẫu cố định |
| `Notify(channel, msg)` | M → A | UC-64 | ghi vào log |

## 4. "Xong" cho một lát cắt

- Mọi use case của lát cắt chạy được từ API; màn hình portal cho các use case chính.
- Mỗi use case có test tích hợp trên PostgreSQL thật (`go test -tags integration`) theo luồng chính và ít nhất một ngoại lệ.
- Entity khai báo bằng DSL của `metap` (`metadata.NewField...`), đăng ký trong `init()`; reconcile hội tụ (`ops_applied=0` ở lần khởi động thứ hai).
- Phân quyền kiểm theo vai trò (đúng người thấy đúng nút), có test cho trường hợp bị từ chối.
- Mọi thao tác ghi sinh audit; thao tác ảnh hưởng traffic sinh outbox.
- Tài liệu: mục tương ứng ở [use case](01-use-cases.md) và [database](02-database.md) khớp với mã (cập nhật cùng PR).

## 5. Rủi ro và việc chưa quyết

| Rủi ro | Xử lý |
|---|---|
| Hợp đồng với Control còn tạm | Cổng ở mục 3 là điều cần chốt đầu tiên cùng người làm Control; sai thì sửa ở một chỗ |
| Hạn mức theo gói cần trước khi có billing (đếm zone, số rule) | S0 có bảng `plan_limits` cố định theo `plan_tier`; S4 thay bằng entitlements thật |
| Lựa chọn L7 (Pingora/NGINX) chưa chốt | **Không chặn Management**: Management chỉ tạo cấu hình có version; L7 đọc qua Control |
| Thiết kế DB cũ ([5.8.14](../../architecture/05-building-blocks/08-components/14-management-database.md)) viết trước bản đồ phân rã | [02-database.md](02-database.md) đối chiếu và liệt kê chỗ khác ở mục 6 của nó |
| Phạm vi MVP của F7/F8 (chủ dự án tạm hoãn 3.8) | S3 chỉ làm phần liên quan trực tiếp tới xử lý tấn công và chặn nhầm; báo cáo và dashboard để sau |

## 6. Cần chủ dự án quyết trước khi bắt đầu mã

1. Duyệt thứ tự lát cắt (đặc biệt S3 trước S4).
2. Khách đầu tiên (dùng thử) cần S1/S2 đến mức nào để cho dùng, vì đó là mốc đầu tiên "có giá trị thật".
3. Có làm portal (màn hình) song song với API ngay từ S0, hay API trước, portal sau mỗi lát cắt một nhịp.

## 7. Trạng thái thực tế (2026-11-21)

Đối chiếu từng use case với mã hiện có. **Có** = chạy được từ API và có màn hình (nếu use case cần); **Một phần** = làm được lõi, thiếu chi tiết nêu ở cột ghi chú; **Chưa** = chưa có. Cột "Nguồn" cho biết đã kiểm trực tiếp trong mã hay chỉ dựa vào roadmap (`metap-docs`): mục nào chỉ dựa vào roadmap thì chưa được kiểm lại bằng tay.

### S0 Nền: tenant và danh tính

| UC | Trạng thái | Ghi chú | Nguồn |
|---|---|---|---|
| 01 Đăng ký org | Có | `/auth/signup` + xác nhận email + trang `/signup`; tắt mặc định (`signup.enabled`); chưa có ô CAPTCHA ở giao diện | mã |
| 02 Account (tạo, đổi tên, đóng) | Có | entity `waf.accounts` (tạo, đổi tên, đóng); mỗi zone thuộc một account (mặc định `default`, tạo cùng zone đầu tiên); đóng bị từ chối khi còn zone active hoặc paused và liệt kê zone; account đóng không nhận zone mới. Không cần đổi `metap`: phạm vi `account` dùng role theo phạm vi sẵn có (roadmap 115). Zone cũ chưa có account chỉ thấy được bằng role toàn tenant | mã |
| 03 Đăng nhập, MFA | Có | mật khẩu, TOTP, mã khôi phục, SSO OIDC; rate limit theo IP cho các route đăng nhập | mã |
| 04 Quản lý phiên | Có | trang Bảo mật tài khoản | roadmap 115 |
| 05, 06 Mời, nhận lời mời | Có | email qua mail-mock ở dev | mã |
| 07 Gán vai trò | Một phần | vai trò và phân quyền xong (roadmap 114); gán theo phạm vi account có màn hình (Account, Thành viên) và có hiệu lực trên zone, rule, DDoS policy, IP list và version của account; chưa có phạm vi nhóm zone và zone, chưa có sự cố hay finding theo account | roadmap 115, mã |
| 08 Gỡ thành viên | Có | thu hồi vai trò khi gỡ; thu hồi phiên và API key chưa kiểm lại | mã (một phần) |
| 09 Quyền xem dữ liệu request | Chưa | chưa có quyền riêng | mã |
| 10, 11, 12 API key, service account | Có | tab Workspace; trần hạn key theo setting | roadmap 115 |
| 13 Audit log | Có | trang Workspace và tab Audit của zone | mã |

### S1 Zone và onboarding

| UC | Trạng thái | Ghi chú | Nguồn |
|---|---|---|---|
| 20 Thêm zone | Có | hostname được chuẩn hoá và kiểm (bỏ scheme, đường dẫn, cổng; từ chối IP, tên một nhãn, ký tự lạ; chấp nhận `*.domain`); gõ xong hiện tên đang trỏ về đâu, gợi ý origin, cảnh báo nếu tra không ra; tự tạo domain theo apex; chưa kiểm danh sách đen domain | mã |
| 21 Xác minh domain | Có | chọn TXT `_waf-verify` hoặc file `/.well-known/waf-verify/<token>` (host phải là domain hoặc tên dưới nó; chỉ kết nối tới địa chỉ công khai) | mã |
| 22 Hướng dẫn trỏ DNS | Một phần | bản ghi CNAME cần thêm hiện ngay ở bước kết nối kèm nút sao chép, kiểm tra định tuyến chỉ để biết, không chặn; chưa có hướng dẫn riêng cho A/AAAA hay theo nhà cung cấp DNS | mã |
| 23 Hostname | Một phần | wildcard `*.domain` được chấp nhận (một zone cho mọi subdomain); mỗi zone vẫn một hostname, chưa có nhiều hostname trong một zone | mã |
| 24 Origin | Có | địa chỉ, giao thức, cổng, Host header (cũng là tên TLS); kiểm tra origin ngay ở form trước khi tạo zone và lưu lần kiểm tra gần nhất; chỉ kết nối tới địa chỉ công khai (tắt được cho dev) | mã |
| 25 TLS, nguồn IP client | Chưa | | mã |
| 26 Preset | Có | ba preset (Cơ bản, API, Nghiêm ngặt) áp dụng ở bước bảo vệ: một DDoS policy và vài rule, ghi thành một version, áp lại không thêm trùng; chưa có preset do vận hành định nghĩa | mã |
| 27 Kích hoạt zone | Có | cổng: domain đã xác minh và zone có cấu hình bảo vệ | mã |
| 28 Tiến độ onboarding | Một phần | tiến độ đọc từ chính zone (đã xác minh, có bảo vệ, origin kiểm lần cuối, định tuyến), mở lại từ danh sách zone ("Tiếp tục thiết lập") ở bước chưa xong; chưa có nhắc ngày 1, 3, 7 và huỷ claim sau 14 ngày | mã |
| 29 Bật chặn theo bậc | Một phần | chỉ `monitor` đến `enforce` cho cả zone, chưa theo nhóm rule | mã |
| 30, 31 Khoá origin, hoàn tất onboarding | Chưa | | mã |
| 32 Tạm dừng, mở lại | Có | workflow `active`, `paused` | mã |
| 33 Gỡ zone | Một phần | xoá có chặn khi còn scan job hay incident; chưa có trạng thái `removed` giải phóng domain | mã |
| 34 Tag, nhóm zone | Một phần | tag tự do trên zone (hiện ở danh sách, sửa ở trang tổng quan); chưa có nhóm zone để phân quyền | mã |

### S2 Policy và thay đổi

**Cập nhật 2026-11-28:** change request, duyệt và tự xác nhận đã có ở **backend và portal** (portal chưa kiểm trên trình duyệt; [chi tiết](03-change-requests.md)). Kiểm soát thay đổi bật theo account và **mặc định tắt**: khi tắt, rule và DDoS policy vẫn được **ghi thẳng** và phát hành ngay (outbox rồi resync). BR-39 (mọi thay đổi ảnh hưởng traffic qua change request) vì vậy mới đúng với account đã bật.

| UC | Trạng thái | Ghi chú | Nguồn |
|---|---|---|---|
| 40 Soạn policy | Một phần | CRUD rule, DDoS policy, danh sách IP; bản nháp là change request chứa danh sách thao tác, không đụng bản đang chạy; trên portal nháp được tạo từ chính form sửa khi account yêu cầu duyệt, chưa sửa được nội dung một thao tác trong nháp | mã |
| 41 DDoS, managed ruleset | Một phần | DDoS policy có; managed ruleset nằm ngoài v1 | mã |
| 42 Kiểm nháp | Một phần | kiểm nháp bằng cách áp thử rồi rollback: đúng kiểm tra, hạn mức, quyền và ràng buộc như lúc phát hành, kèm kết quả kiểm policy và diff; nút Kiểm tra policy ở tab Phiên bản vẫn kiểm bản đang chạy; chưa ước lượng chi phí, chưa dò rule không bao giờ khớp; chưa gọi biên dịch của Control | mã |
| 43 đến 45 Change request, duyệt, tự xác nhận | Một phần | `waf.change_requests` với action submit, approve, reject, revise, cancel; account chọn `off`, `review` (tự xác nhận) hoặc `fourEyes` (người khác duyệt). Có tab Thay đổi của zone, trang Thay đổi policy và cài đặt ở trang Account; bật 4-eyes cần đủ hai người duyệt được; [những chỗ chưa chặn](03-change-requests.md#chưa-chặn) | mã |
| 46 Phát hành | Một phần | duyệt là phát hành, trong một transaction; từ chối nếu policy đã đổi từ lúc gửi; version ghi change request và người duyệt. Chưa có sự kiện `PolicyPublished` riêng (control-plane vẫn đi theo sự kiện của record) | mã |
| 47 Theo dõi phát hành | Chưa | edge không báo tiến độ ngược lại | mã |
| 48 Xem, so sánh version | Một phần | tab Phiên bản của zone: mỗi thay đổi ghi một snapshot bất biến (`waf.policy_versions`), xem khác biệt với bản trước; version từ change request có người duyệt (màn hình chưa hiện); rule áp dụng cả tenant không nằm trong version của zone nào | mã |
| 49 Rollback | Có | action `rollback` trên một version: khôi phục (tạo lại, sửa, xoá) rồi ghi một version mới; không nguyên tử, lỗi giữa chừng thì chạy lại cùng version để hoàn tất | mã |
| 50 Under Attack | Chưa | là hành vi của edge (thách thức mọi request); phía management chưa có gì để làm khi edge chưa đọc cờ | mã |
| 51 Rule tạm có hạn | Có | firewall rule có `expiresAt` (chọn 1 giờ đến 7 ngày khi tạo hoặc sửa); control-plane ngừng biên dịch rule hết hạn ở lần resync kế (tối đa 1 phút); kiểm tra policy báo rule đã hết hạn | mã |
| 52 Đổi thông tin không ảnh hưởng traffic | Có | | mã |

### S3 Vận hành bảo mật

| UC | Trạng thái | Ghi chú | Nguồn |
|---|---|---|---|
| 60 Xem sự kiện chặn | Có | tab Events của zone và trang Analytics, qua analytics-api (ClickHouse) | mã |
| 61 Báo chặn nhầm | Một phần | entity `waf.false_positive_reports`, SOC ghi báo cáo trong portal; chưa có nhận từ trang chặn (edge chưa hiện request ID và form báo) | mã |
| 62 Xử lý chặn nhầm | Một phần | trang Chặn nhầm: bỏ qua, đóng, hoặc cho IP đi qua (whitelist) kèm cảnh báo CGNAT/dải riêng; ngoại lệ chưa qua change request (UC-43 chưa có) | mã |
| 63 Alert policy | Có | ngưỡng, cửa sổ, kênh; action đánh giá | mã |
| 64 Kênh và gửi thử | Có | action `test` | mã |
| 65 Alert thành case | Có | action `correlate` (luật tĩnh: cùng zone + cùng IP trong 15 phút) | mã |
| 66 Xử lý case | Có | workflow incident | mã |
| 67 Mitigation từ case | Một phần | chặn IP nguồn có thời hạn (1 giờ đến 7 ngày) từ incident; hết hạn tự gỡ ở lần resync kế (tối đa 1 phút); chưa có Under Attack và challenge | mã |
| 68 Đóng case | Một phần | đóng được; mitigation còn lại giữ vĩnh viễn hoặc gỡ ngay từ trang incident; chưa tự nhắc khi đóng | mã |

### S4 đến S6

| Lát | Trạng thái |
|---|---|
| S4 Gói và thanh toán (UC-70 đến 75) | Một phần. Có: service riêng `billing-service` (`m-billing`) sở hữu `waf.plans` và `waf.subscriptions`, nằm trong tenant vận hành (`OPERATOR_TENANT_ID`), admin của tenant đó định nghĩa gói (số zone, số rule mỗi zone, gói mặc định) và gán subscription cho từng tenant khách; trang Gói cho khách xem mức dùng và cảnh báo từ 80% (UC-70, 74); `zones-service` hỏi billing-service qua REST (`/internal/plans/resolve`, bí mật dùng chung) rồi chặn tạo zone và rule vượt hạn mức (`plan_limit_reached`; billing không trả lời thì cho qua), không xoá gì của khách đang vượt; tenant vận hành không bị giới hạn; chưa đặt tenant vận hành thì không giới hạn gì. Không dùng `tier` của metap. Khách (admin) xin đổi gói, admin vận hành duyệt hoặc từ chối kèm phản hồi (UC-71, 73): nâng gói có hiệu lực ngay, hạ gói có hiệu lực sau một kỳ 30 ngày kể từ lúc duyệt (subscription ghi gói chờ và ngày áp dụng); hạ gói không xoá gì của khách, chỉ chặn thêm. Chưa có: thanh toán, hoá đơn, trần chi tiêu, RPS, kỳ thanh toán thật (UC-72, 74 phần trần chi tiêu, 75) |
| S5 UC-80 Xuất dữ liệu | Có | Cài đặt, Xuất dữ liệu: một file JSON gồm account, domain, zone, DDoS policy, firewall rule, IP list mà người xuất được phép thấy, đọc theo trang, báo nếu quá lớn bị cắt. Chưa có cấu hình của billing và phần phân tích |
| S5 UC-81 Xoá org | Có (không xoá dữ liệu vật lý) | platform admin lên lịch và huỷ xoá (trang Tenants); Owner tự xoá ở Cài đặt bằng cách gõ tên workspace: workspace bị khoá ngay, mọi admin nhận email kèm liên kết huỷ dùng một lần (trang `/cancel-deletion`, công khai vì workspace đã khoá nên không đăng nhập được), job chuyển `deleted` sau 30 ngày; chưa xoá dữ liệu vật lý |
| S5 UC-82 Chuyển zone sang org khác | Chưa |
| S6 UC-90 SSO | Có (OIDC và SAML 2.0): domain email phải được chứng minh sở hữu bằng bản ghi TXT mới định tuyến đăng nhập hay tạo thành viên, bắt buộc SSO (kể cả khi nhận lời mời), chỉ tin email IdP đã xác minh (OIDC), ánh xạ nhóm IdP sang role (không bao giờ admin); chưa có đăng xuất một lần (SLO) và assertion mã hoá |
| S6 UC-91 Khôi phục Owner | Có: yêu cầu, chờ, báo admin, hoàn tất (trang Tenants, hộp thoại Truy cập) |
| S6 UC-92 NCC hỗ trợ | Một phần: tenant cấp quyền, platform admin mở phiên hỗ trợ; chưa duyệt từng phiên hay thông báo từng phiên |

### Cập nhật 2026-11-28

- **Che path ở edge** ([BR-44](../../requirements/02-requirements.md)): path trong security event được che trước khi rời edge (`e-edge/telemetry/maskpath.go`).
- **Quyết định sản phẩm** ([§4](../../requirements/04-product-direction/00-index.md)): bảo vệ ứng dụng AI để backlog; SOC (AI cộng người, người ra quyết định) được đổi toàn bộ policy, nên change request và duyệt (UC-43 đến 45) là phần cần có; IP nguồn được dùng cho bảo vệ chung, có điều kiện, chưa có code.

### Ba chỗ lệch cần quyết (cập nhật 2026-11-22)

1. **S2.** Version, rollback (UC-48, 49) và change request, duyệt (UC-43 đến 46, backend, 2026-11-28) đã làm. Rule cho cả tenant, màn hình, cài đặt zone, giới hạn rollback, kênh khẩn và điều kiện bật 4-eyes xong ngày 2026-11-29. Còn: [vài chỗ chưa chặn](03-change-requests.md#chưa-chặn), theo dõi phát hành (UC-47, cần edge). Kiểm soát thay đổi mặc định tắt.
2. **Account (UC-02)**: xong ở waap, không đổi `metap`.
3. **S4**: chỉ làm gói, subscription và hạn mức (chủ dự án chọn bỏ thanh toán, hoá đơn). Làm tiếp khi có khách trả tiền và chốt cổng thanh toán.
