# 2. Requirements

> **Trạng thái: bản nháp chờ review (2026-11-06).** Mọi dòng ở đây là **đề xuất**. Các điểm cần quyết
> nằm ở [§9](#9-câu-hỏi-mở-cho-review). Khi chốt xong, requirements là căn cứ để đối chiếu lại module
> ([5.8.12](../architecture/05-building-blocks/08-components/12-management.md)) và database
> ([5.8.14](../architecture/05-building-blocks/08-components/14-management-database.md)).

Requirements viết từ [bản đồ phân rã chức năng](01-functional-decomposition.md): mỗi requirement trỏ về
một hay nhiều chức năng lá `F…`. Requirement chất lượng không viết lại ở đây mà trỏ về kịch bản đo được
`QS-…` ở [10.3](../architecture/10-quality/00-index.md#103-kịch-bản).

## 1. Quy ước

| Mã | Loại | Ví dụ |
|---|---|---|
| `FR-<nhóm>-<số>` | Yêu cầu chức năng; nhóm trùng nhóm F (`FR-5-03` thuộc F5) | FR-5-03 |
| `BR-<số>` | Quy tắc nghiệp vụ: điều luôn đúng, nhiều chức năng cùng phải tuân theo | BR-4 |
| `DT-<số>` | Yêu cầu dữ liệu: đối tượng dữ liệu, chủ sở hữu, retention, độ nhạy | DT-3 |
| `NFR-<số>` | Yêu cầu phi chức năng chưa có trong 10.3 | NFR-2 |
| `IR-<số>` | Yêu cầu giao tiếp với hệ thống ngoài | IR-1 |
| `OQ-<số>` | Câu hỏi mở cần quyết khi review | OQ-1 |

- **Mức**: MVP / MR (Mở rộng) / NC (Nâng cao), như [1.2](../architecture/01-introduction/00-index.md#12-tổng-quan-yêu-cầu). Một requirement
  MVP là bắt buộc để ký hợp đồng doanh nghiệp đầu tiên.
- **"Phải"** là bắt buộc; **"nên"** là mong muốn, có thể bỏ khi thiếu thời gian mà không đổi kiến trúc.
- Giới hạn theo gói (số rule, retention, kênh…) lấy từ bảng entitlements ở
  [8.18 §4](../architecture/08-cross-cutting/18-segments-and-plans.md#bảng-tính-năng-và-giới-hạn-entitlements), không
  lặp lại ở đây.

## 2. Tác nhân

| Tác nhân | Loại | Dùng chức năng |
|---|---|---|
| Chủ tài khoản (owner) | Người, phía khách | F1, F10 |
| Quản trị bảo mật | Người, phía khách | F2, F5, F8, F9 |
| SOC analyst | Người, phía khách | F5.5, F7, F8 |
| DevOps / platform | Người hoặc máy (API key, Terraform), phía khách | F2, F4, F5.6 |
| Developer | Người, phía khách | F5.1.4 trong phạm vi được giao, F7 chỉ đọc |
| Viewer, Billing | Người, phía khách | F7 chỉ đọc; F10 |
| Kiểm toán | Người, phía khách | F1.5, F9 |
| NOC / SRE, Support, Threat research, Sales | Người, phía NCC | F6, F11, F12; F10.1 |
| End-user | Người hoặc bot đi qua edge | F3, F4 |
| IdP, cổng thanh toán, nhà cung cấp hoá đơn điện tử, CA ACME, SIEM, kênh thông báo | Hệ thống ngoài | [§7](#7-giao-tiếp-với-hệ-thống-ngoài) |

## 3. Yêu cầu chức năng

### F1 Tenant và danh tính

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-1-01 | Người dùng phải tự đăng ký org bằng email đã xác minh; NCC phải tạo được org thay cho khách Enterprise | F1.1.1 | MVP | Org mới có một owner và một account mặc định |
| FR-1-02 | Owner phải tạo, đổi tên, đóng được account; đóng account khi còn zone hoạt động thì bị từ chối | F1.1.2 | MVP | Lỗi nêu số zone còn hoạt động |
| FR-1-03 | Owner phải yêu cầu xoá org; dữ liệu bị xoá sau thời hạn giữ ([OQ-6](#9-câu-hỏi-mở-cho-review)) | F1.1.4 | MVP | Trong thời hạn giữ có thể huỷ yêu cầu; sau đó không đăng nhập được |
| FR-1-04 | Owner, admin phải mời thành viên qua email; lời mời hết hạn sau 7 ngày, thu hồi được | F1.2.1 | MVP | Lời mời đã hết hạn hoặc thu hồi không dùng được |
| FR-1-05 | Vai trò phải gán được ở bốn cấp: org, account, nhóm zone, zone; quyền là hợp của mọi binding | F1.2.2 | MVP | User có vai trò ở một zone không thấy zone khác |
| FR-1-06 | Vai trò mặc định phải gồm owner, security admin, SOC analyst, developer, viewer, billing với quyền như [5.8.12](../architecture/05-building-blocks/08-components/12-management.md#quyền-mặc-định) | F1.2.2 | MVP | Ma trận quyền kiểm bằng test tự động |
| FR-1-07 | Admin nên tạo được vai trò tuỳ chỉnh từ danh mục quyền | F1.2.3 | MR | |
| FR-1-08 | Gỡ thành viên phải thu hồi phiên và API key cá nhân của họ ([BR-15](#4-quy-tắc-nghiệp-vụ)) | F1.2.4 | MVP | Request kế tiếp bằng phiên cũ bị từ chối |
| FR-1-09 | Hệ thống nên đồng bộ user, nhóm qua SCIM 2.0 và gán vai trò theo nhóm IdP | F1.2.5 | MR | User bị khoá ở IdP mất quyền ≤ 5 phút |
| FR-1-10 | Hệ thống phải hỗ trợ đăng nhập mật khẩu + MFA (TOTP, WebAuthn) và SSO SAML 2.0 / OIDC | F1.3.1, F1.3.2 | MVP | |
| FR-1-11 | Org phải bắt buộc được SSO cho domain email đã xác minh, và bắt buộc MFA | F1.3.3 | MVP | User thuộc domain đó không đăng nhập bằng mật khẩu được, trừ tài khoản khẩn của owner |
| FR-1-12 | User phải xem và đăng xuất được các phiên của mình; admin đăng xuất được phiên của thành viên | F1.3.4 | MVP | |
| FR-1-13 | Hệ thống phải cấp API key có phạm vi quyền, phạm vi zone, ngày hết hạn; secret chỉ hiện một lần | F1.4.1 | MVP | [BR-14](#4-quy-tắc-nghiệp-vụ) |
| FR-1-14 | API key phải xoay được (hai key song song trong thời gian chuyển) và hiện lần dùng cuối | F1.4.2 | MVP | Lần dùng cuối trễ ≤ 5 phút so với thực tế |
| FR-1-15 | Hệ thống phải có service account không gắn người, sở hữu API key cho CI/CD | F1.4.3 | MVP | Gỡ người tạo không làm mất service account |
| FR-1-16 | Mọi thay đổi qua UI, API, Terraform, SCIM, kênh khẩn và thao tác của NCC phải ghi audit: người/máy, đối tượng, trước/sau, thời điểm, IP, kênh | F1.5.1 | MVP | Không có API ghi nào mà không sinh audit (test tự động trên toàn bộ API) |
| FR-1-17 | Kiểm toán phải lọc audit theo người, đối tượng, loại hành động, thời gian, và xuất CSV/JSON | F1.5.2, F1.5.3 | MVP | Xuất 1 triệu dòng không làm chậm portal của tenant khác |
| FR-1-19 | Audit log phải đẩy liên tục sang SIEM của khách qua cùng exporter với security event, gồm thao tác của nhân viên NCC trên zone khách, và có chuỗi hash để kiểm toàn vẹn (PCI DSS 10) | F1.5.3, F1.5.4 | MVP | Sửa hoặc xoá một dòng audit làm kiểm chuỗi hash báo lỗi |
| FR-1-18 | Một tài khoản người dùng phải thuộc được nhiều org; người dùng chuyển org trong portal; SSO bắt buộc và chính sách MFA áp theo từng org; API key luôn thuộc một org ([3.6](03-business-analysis/06-members-access.md#3-đã-quyết)) | F1.2.6 | MVP | Gỡ khỏi org A không ảnh hưởng quyền ở org B; SCIM của org A chỉ đổi membership trong org A |

### F2 Zone và onboarding

> **Chờ onboarding.** FR-2-17…FR-2-30, BR-22…BR-24 và phần sửa của FR-2-02, FR-2-06, BR-2 đến từ bản nháp
> onboarding chưa duyệt; sẽ viết lại sau khi phân tích ở [§3.1](03-business-analysis/01-onboarding.md) có trả lời.

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-2-01 | Người dùng phải thêm zone theo domain gốc; domain đã có ở tenant khác bị từ chối ([BR-1](#4-quy-tắc-nghiệp-vụ)) | F2.1.1 | MVP | Lỗi không tiết lộ tenant đang giữ domain |
| FR-2-02 | Hệ thống phải xác minh quyền sở hữu qua bản ghi CNAME uỷ quyền `_acme-challenge` (mặc định) hoặc TXT, kiểm từ ≥ 3 resolver ở các mạng khác nhau, và kiểm lại mỗi ngày khi zone đang hoạt động | F2.1.2 | MVP | Bản ghi đúng được phát hiện ≤ 2 phút sau khi resolver công cộng thấy; mất bản ghi 7 ngày liên tục thì cảnh báo, 14 ngày thì tạm dừng ([OQ-3](#9-câu-hỏi-mở-cho-review)); claim chưa xác minh hết hạn ở ngày 14 ([BR-22](#4-quy-tắc-nghiệp-vụ)) |
| FR-2-03 | Portal phải hiện hướng dẫn trỏ DNS theo chế độ (CNAME, nameserver) và trạng thái đã trỏ hay chưa | F2.1.3 | MVP | Trạng thái cập nhật ≤ 5 phút sau khi DNS đổi |
| FR-2-04 | Người dùng phải thêm, xoá hostname trong zone, kể cả wildcard; hostname là duy nhất toàn nền tảng ([BR-17](#4-quy-tắc-nghiệp-vụ)) | F2.2.1 | MVP | |
| FR-2-05 | Người dùng phải khai báo origin (địa chỉ, cổng, giao thức, SNI, header Host, timeout, retry) | F2.2.2, F2.2.4 | MVP | |
| FR-2-06 | Hệ thống phải kiểm origin **từ edge** ở ≥ 2 PoP trước khi kích hoạt và cho xem kết quả (kết nối, TLS, mã trả, thời gian theo chặng) | F2.2.3 | MVP | Thông điệp phân biệt được firewall chưa mở cho edge, lỗi TLS, vòng chuyển hướng; kích hoạt khi kiểm lỗi cần xác nhận rõ ràng của người dùng |
| FR-2-07 | Zone phải cấu hình được chế độ đứng sau CDN của khách: danh sách IP tin cậy, header chứa IP client | F2.2.5 | MVP | |
| FR-2-08 | Hệ thống phải tự cấp và gia hạn cert cho hostname đã xác minh; gia hạn lỗi thì cảnh báo trước hạn ≥ 14 ngày | F2.3.1, F2.3.2 | MVP | Không hostname nào phục vụ cert hết hạn khi DNS vẫn trỏ đúng |
| FR-2-09 | Khách nên tải được cert riêng; khoá riêng chỉ lưu ở secret store | F2.3.3 | MR | Khoá không bao giờ có trong database hay log |
| FR-2-10 | Zone phải cấu hình được TLS tối thiểu, HSTS, chế độ TLS tới origin | F2.3.4 | MVP | |
| FR-2-11 | Kích hoạt zone phải bật managed ruleset ở log-only trong 7 ngày đầu ([OQ-4](#9-câu-hỏi-mở-cho-review)) | F2.4.1 | MVP | Ngày thứ 7 có thông báo kèm tóm tắt các rule đã khớp |
| FR-2-12 | Người dùng phải tạm dừng (chuyển tiếp không bảo vệ) và mở lại zone; tạm dừng là thao tác khẩn, không cần duyệt | F2.4.2 | MVP | Có hiệu lực ≤ 30 s (QS-4) |
| FR-2-13 | Người dùng phải gỡ zone; domain và hostname được giải phóng ([BR-16](#4-quy-tắc-nghiệp-vụ)) | F2.4.3 | MVP | |
| FR-2-14 | Owner nên chuyển zone giữa các account cùng org mà không gián đoạn traffic | F2.4.4 | MR | |
| FR-2-15 | Người dùng phải gắn tag và nhóm zone; nhóm dùng được cho phân quyền và lọc | F2.5.1 | MVP | |
| FR-2-16 | Thao tác hàng loạt trên nhóm zone nên chạy nền, có tiến độ, dừng được | F2.5.2 | MR | QS-27 |
| FR-2-17 | Khi thêm domain, hệ thống phải tra DNS hiện tại (NS, A/AAAA/CNAME, CAA, TTL), hiện kết quả và gợi ý origin ([requirements §3.1](03-business-analysis/01-onboarding.md) OB2) | F2.1.5 | MVP | Kết quả ≤ 10 s; IDN lưu dạng punycode; không nhận public suffix làm domain gốc |
| FR-2-18 | Hệ thống phải phát hiện CDN hoặc WAF đang đứng trước domain, và CAA chặn CA của NCC, rồi báo trước bước xác minh | F2.1.5, F2.3.6 | MVP | Thông báo CAA nêu đúng bản ghi cần thêm |
| FR-2-19 | Với xác minh bằng CNAME uỷ quyền, cert đầu tiên phải được cấp trước khi khách đổi DNS | F2.1.2, F2.3.1 | MVP | Không có khoảng lỗi cert lúc đổi DNS (OG2) |
| FR-2-20 | Người onboard phải gửi được hướng dẫn tạo bản ghi DNS cho người giữ DNS qua email; người nhận không cần tài khoản | F2.1.2 | MVP | Email chứa đúng bản ghi, không chứa dữ liệu khác của tenant |
| FR-2-21 | Portal phải hướng dẫn theo phương thức kết nối: CNAME, ALIAS cho apex, A/AAAA tĩnh cho apex ([OQ-16](03-business-analysis/01-onboarding.md#a9-câu-hỏi-mở)), đứng sau CDN của khách; đổi nameserver ở mức MR | F2.1.3 | MVP | |
| FR-2-22 | Khách phải chọn preset theo loại ứng dụng; preset bật chặn DDoS L7 và lọc sớm từ request đầu, phần còn lại log-only; có lựa chọn "đang bị tấn công, chặn ngay" ([BR-24](#4-quy-tắc-nghiệp-vụ)) | F2.6.1 | MVP | Đổi preset được bất kỳ lúc nào trong giai đoạn log-only |
| FR-2-23 | Portal phải sinh lệnh `curl --resolve` và dòng `hosts` để kiểm thử qua edge trước khi đổi DNS, và hiện event realtime của zone trong lúc kiểm thử | F2.6.2 | MVP | Request kiểm thử có trong tra cứu theo request ID |
| FR-2-24 | Hệ thống phải hiện theo từng hostname: đã trỏ, chưa trỏ hay trỏ một phần về edge, và thời điểm edge thấy traffic thật đầu tiên | F2.1.7, F2.7.2 | MVP | Cập nhật ≤ 5 phút sau khi DNS đổi |
| FR-2-25 | Hệ thống phải lưu bản ghi DNS cũ và hiện kế hoạch quay lui (bản ghi cũ, tạm dừng zone) trước khi khách đổi DNS | F2.7.1 | MVP | |
| FR-2-26 | Trong log-only, hệ thống phải gửi báo cáo hằng ngày: request sẽ bị chặn theo rule, rule nghi chặn nhầm, đề xuất exception, mức sẵn sàng bật chặn | F2.7.3 | MVP | Có trên portal và email; exception tạo được một bấm |
| FR-2-27 | Bật chặn lần đầu phải làm được theo bậc nhóm rule, mỗi bậc một change request | F2.7.4 | MVP | Rollback một bấm từng bậc |
| FR-2-28 | Hệ thống phải hướng dẫn khoá origin (dải IP của edge có version, header bí mật, mTLS) và ghi nhận đã khoá hay bỏ qua; nên kiểm origin còn lộ từ điểm không thuộc edge | F2.7.5, F2.2.6 | MVP / MR | Bỏ qua khoá origin có audit |
| FR-2-29 | Danh sách zone phải hiện tiến độ onboarding theo bước; hệ thống nhắc ở ngày 1, 3, 7 tới đúng người của bước đang dừng | F2.7.6 | MVP | Zone bỏ dở lọc được |
| FR-2-30 | Hệ thống nên nhập hàng loạt hostname và origin qua CSV, API | F2.2.7 | MR | 1.000 hostname ≤ 5 phút, báo lỗi theo dòng |

### F3 Bảo vệ runtime và F4 Hiệu năng

Phần lớn là hành vi của edge; requirement đo được đã có ở 10.3. Chỉ ghi phần chức năng.

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-3-01 | Edge phải kết thúc TLS theo SNI cho HTTP/1.1, HTTP/2, HTTP/3, và định danh zone, hostname của mọi request | F3.1.1, F3.1.2 | MVP | QS-1 |
| FR-3-02 | Mọi request phải có request ID trả về client trong header và trang chặn | F3.1.3 | MVP | Request ID tra được ở F7.3.1 (QS-11) |
| FR-3-03 | Edge phải áp access list IP/CIDR/quốc gia/ASN trước mọi bước kiểm tốn kém | F3.2.1 | MVP | QS-17 |
| FR-3-04 | Edge phải phát hiện và giảm thiểu DDoS L7 tự động theo độ nhạy, hành động khách chọn | F3.3.1, F3.3.2 | MVP | QS-17, QS-29 |
| FR-3-05 | Under Attack mode phải challenge mọi request chưa có clearance của zone | F3.3.3 | MVP | QS-5 |
| FR-3-06 | Edge phải áp rate limit với bộ đếm toàn fleet theo khoá khách chọn: IP, path, header, cookie, API key, JWT claim (CGNAT của mạng di động làm khoá IP chặn nhầm nhiều) | F3.4.1, F3.4.2 | MVP | Sai lệch ngưỡng toàn fleet ≤ 10% ([OQ-8](#9-câu-hỏi-mở-cho-review)) |
| FR-3-07 | Edge phải kiểm managed ruleset, custom rule theo thứ tự ưu tiên, exception có phạm vi, trên body trong giới hạn của gói | F3.5.1–F3.5.4 | MVP | QS-1, QS-8 |
| FR-3-08 | Edge phải chặn bot xấu rõ ràng, không chặn bot đã xác minh; tính điểm bot ở gói cao | F3.6.1, F3.6.2 | MVP / MR | QS-20, QS-21 |
| FR-3-09 | Edge nên kiểm request theo schema OpenAPI, JWT, mTLS client của endpoint | F3.7.1, F3.7.2 | MR | QS-22, QS-23 |
| FR-3-10 | Edge phải thực hiện block, challenge (JS, tương tác, managed), log, allow, trust, skip và phát security event cho mọi quyết định khác allow; rule xung đột giải theo [BR-33](#4-quy-tắc-nghiệp-vụ) | F3.8.1, F3.8.3 | MVP | QS-10, QS-12 |
| FR-3-11 | Edge phải thực thi trần cứng của gói có trần | F3.8.4 | MVP | QS-36 |
| FR-3-12 | Zone phải chọn được fail-open hoặc fail-closed khi tầng WAF không trả lời kịp | F3.9.1 | MVP | QS-18 |
| FR-3-13 | Edge phải xác thực với origin bằng mTLS hoặc header bí mật do khách cấu hình | F3.9.2 | MVP | |
| FR-3-14 | Khi zone đứng sau CDN của khách, edge chỉ lấy IP client từ header đã khai nếu kết nối đến từ dải IP CDN đã khai; `X-Forwarded-For` đọc từ phải sang, lấy IP đầu tiên không thuộc dải tin cậy ([BR-31](#4-quy-tắc-nghiệp-vụ)) | F3.1.4 | MVP | Request từ ngoài dải CDN mang `True-Client-IP` giả không đổi được IP mà rate limit dùng |
| FR-3-15 | Access list và custom rule phải có hai hành động tách biệt Trust và Skip theo [BR-28](#4-quy-tắc-nghiệp-vụ); UI và API gọi đúng tên, nói rõ chặng nào bị bỏ qua | F3.2.4 | MVP | IP được Trust vẫn bị rate limit và DDoS L7 chặn khi flood |
| FR-3-16 | Khách nên tải trang lỗi tuỳ biến (HTML, CSS tĩnh) theo tenant hoặc zone và map với từng loại lỗi; edge tự chèn request ID; path API trả JSON | F3.8.2 | MR | Trang có JavaScript hoặc tài nguyên ngoài bị từ chối khi lưu; xem trước được trước khi phát hành |
| FR-3-17 | Edge nên có hành động tarpit có trần theo node | F3.8.5 | MR | Vượt trần, request tarpit mới chuyển thành block, không tăng số kết nối giữ |
| FR-3-18 | Edge nên có SSL passthrough theo SNI cho hostname Enterprise chọn: không giải mã, chỉ định tuyến và chống DDoS L3/L4; UI ghi rõ hostname đó không được kiểm | F3.1.5 | MR | Hostname passthrough không có event WAF, bot, cache nào |
| FR-3-19 | Edge phải chạy pipeline theo thứ tự [BR-32](#4-quy-tắc-nghiệp-vụ) | F3 | MVP | Request bị chặn ở một bước không tốn CPU của các bước sau |
| FR-4-01 | Edge phải cache theo Cache-Control và cache rule của khách, phục vụ bản cũ khi origin lỗi | F4.1.1, F4.1.2, F3.9.3 | MVP | |
| FR-4-02 | Người dùng phải purge toàn zone, theo URL, theo prefix trong hạn mức gói | F4.2.1 | MVP | QS-15 |
| FR-4-03 | DNS điều hướng phải trả IP PoP khoẻ, gần client | F4.3.1 | MVP | QS-16 |
| FR-4-04 | Khách nên dùng được DNS authoritative với bản ghi proxied, DNSSEC | F4.3.2 | MR | |
| FR-4-05 | Khách gói Business, Enterprise nên cấu hình nhóm origin active-passive, round-robin hoặc theo trọng số, health check chủ động và thụ động; origin chính lỗi liên tiếp trong cửa sổ cấu hình thì edge chuyển sang origin dự phòng và báo khách | F4.4.1 | MR | Request không idempotent (POST, PATCH...) không bao giờ được gửi lại sang origin khác |

### F5 Vòng đời policy

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-5-01 | Người dùng phải soạn custom rule bằng builder và bằng biểu thức text; hai cách chuyển qua lại không mất thông tin | F5.1.1 | MVP | Mọi rule tạo bằng builder hiển thị được dạng text và ngược lại (trong tập builder hỗ trợ) |
| FR-5-02 | Người dùng phải cấu hình managed ruleset: profile, paranoia, ngưỡng anomaly, tắt từng rule trong giới hạn gói | F5.1.2 | MVP | |
| FR-5-03 | Người dùng phải soạn rate limit rule, access list, cấu hình DDoS L7, bot, API security của zone | F5.1.3, F5.1.5 | MVP | |
| FR-5-04 | Người dùng phải tạo exception có phạm vi (zone bắt buộc; rule, trường, đường dẫn tuỳ chọn) và có thể đặt ngày hết hạn | F5.1.4 | MVP | [BR-21](#4-quy-tắc-nghiệp-vụ) |
| FR-5-05 | Mọi rule (custom, managed, rate limit, bot, schema API) và cả zone phải đặt được log-only; chuyển sang chặn theo [BR-36](#4-quy-tắc-nghiệp-vụ) | F5.1.6 | MVP | Rule log-only sinh event với hành động `log`, không ảnh hưởng request |
| FR-5-06 | Mọi chỉnh sửa phải nằm trong bản nháp của zone cho tới khi phát hành; nhiều người sửa cùng bản nháp không ghi đè nhau | F5.1 | MVP | Xung đột version trả lỗi, không mất thay đổi |
| FR-5-07 | Hệ thống phải kiểm cú pháp, kiểu, giới hạn gói khi soạn, và biên dịch thử bằng cùng checker với Control trước khi gửi duyệt | F5.2.1, F5.2.2 | MVP | Bản nháp qua kiểm tra không bao giờ bị Control từ chối vì cú pháp |
| FR-5-08 | Người dùng nên mô phỏng thay đổi trên traffic 7 ngày gần nhất và thấy số request đổi kết quả | F5.2.3 | MR | Kết quả ≤ 5 phút cho zone 10 triệu request/ngày |
| FR-5-09 | Gửi duyệt phải tạo change request chứa diff, kết quả kiểm tra, mô phỏng nếu có | F5.3.1 | MVP | |
| FR-5-10 | Account bật 4-eyes thì change request phải có người duyệt khác người tạo ([BR-5](#4-quy-tắc-nghiệp-vụ)); account self-service phát hành thẳng, không qua duyệt ([BR-26](#4-quy-tắc-nghiệp-vụ)) | F5.3.2 | MR | Cùng một workflow: mỗi người chỉ thấy nút mình bấm được (phát hành, gửi duyệt, duyệt); người gửi có vai trò duyệt vẫn không tự duyệt được |
| FR-5-11 | Account nên đặt được khung giờ cho phép phát hành; ngoài khung chỉ kênh khẩn chạy | F5.3.3 | MR | |
| FR-5-12 | Phát hành phải tạo PolicyVersion bất biến và gửi sang Control trong cùng giao dịch với lần ghi ([BR-4](#4-quy-tắc-nghiệp-vụ)) | F5.3.4 | MVP | Không có version nào "đã phát hành" mà Control chưa nhận |
| FR-5-13 | Portal phải hiện trạng thái phát hành realtime: đã biên dịch, % node đã áp, lỗi; khi dừng hoặc rollback phải nêu lý do theo region | F5.3.5 | MVP | QS-4 |
| FR-5-14 | Người dùng phải xem lịch sử version, so sánh hai version bất kỳ, và rollback một bấm | F5.4.1–F5.4.3 | MVP | QS-7 |
| FR-5-15 | SOC phải bật Under Attack mode và rule tạm có hạn mà không cần duyệt; rule tạm tự hết hạn ([BR-6](#4-quy-tắc-nghiệp-vụ)) | F5.5.1, F5.5.2 | MVP | QS-5 |
| FR-5-16 | Mọi thao tác F5.1–F5.4 phải làm được qua public API với cùng quyền như UI | F5.6.1 | MVP | UI chỉ dùng public API |
| FR-5-17 | Hệ thống nên có Terraform provider và đồng bộ policy từ Git; **UI là nguồn sự thật**: khi xung đột, bản trên UI thắng, Terraform và GitOps báo drift chứ không ghi đè ([3.2](03-business-analysis/02-policy-change.md#3-đã-quyết)) | F5.6.2, F5.6.3 | MR | `terraform plan` lần hai sau `apply` không có thay đổi; sửa trên UI sau đó làm `plan` báo drift |
| FR-5-18 | Hệ thống nên kế thừa policy org → nhóm zone → zone; zone thấy được rule nào đến từ cấp nào | F5.1.7 | MR | |
| FR-5-19 | Người dùng nên duyệt endpoint API từ discovery và tải OpenAPI làm schema | F5.1.8 | MR | |
| FR-5-20 | Public API phải có endpoint kiểm tra không lưu (validate, dry-run): chạy cùng checker và `edge-verify` với Control, trả lỗi theo rule, không tạo PolicyVersion hay change request; có quota theo tenant | F5.2.4 | MVP | Gọi validate nhiều lần không sinh dòng nào trong `policy_versions` |
| FR-5-21 | Account cần duyệt nên có emergency override theo [BR-27](#4-quy-tắc-nghiệp-vụ) | F5.5.3 | MR | Override sinh cảnh báo critical tới mọi Owner và Admin của account trong ≤ 1 phút |
| FR-5-22 | Cấu hình có ba tầng: system, global, website ([3.14](03-business-analysis/14-config-tiers-attack-history-thresholds.md)); tầng trên ghi đè tầng dưới ([BR-37](#4-quy-tắc-nghiệp-vụ)) | F5.7.1–F5.7.4 | MVP (system, website); MR (global) | Khách xem được rule nào áp lên website của mình và từ tầng nào |
| FR-5-23 | Số rule và tập rule tầng website bị giới hạn theo gói | F5.7.3 | MVP | Vượt trần thì từ chối khi lưu, nêu rõ trần của gói |
| FR-5-24 | NCC phải tạo được tập rule custom ở tầng global và gán cho domain, cluster hoặc node; phát qua đường thường có canary | F5.7.2 | MR | Tập rule gán cho một domain chỉ ảnh hưởng domain đó |

### F6 Phân phối cấu hình và F11 Threat intelligence

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-6-01 | Control phải biên dịch mỗi thay đổi thành bundle của zone, từ chối bundle vượt ngân sách và báo lý do về Management | F6.1.1, F6.1.2 | MVP | |
| FR-6-02 | Mọi bundle và manifest phải được ký; edge từ chối bản không có chữ ký hợp lệ | F6.1.3 | MVP | |
| FR-6-03 | Cấu hình khẩn (Under Attack, rule khẩn, purge) phải đi đường nhanh tách khỏi đường thường | F6.2.2 | MVP | QS-5, QS-15 |
| FR-6-04 | Thay đổi phải đi canary theo cell, có gate theo chỉ số, tự rollback khi vượt ngưỡng; gate theo tỉ lệ chặn và traffic chỉ tự rollback bản phát hành của NCC ([BR-29](#4-quy-tắc-nghiệp-vụ)) | F6.3.1 | MVP | QS-6 |
| FR-6-05 | Node phải có danh tính, cert ngắn hạn tự gia hạn, thu hồi được | F6.4.1, F6.4.2 | MVP | QS-39, QS-40 |
| FR-6-06 | NOC phải biết mỗi node chạy bản edge, ruleset, manifest nào | F6.4.3 | MVP | |
| FR-6-07 | Zone phải được đặt vào pool cell theo gói và chuyển khi đổi gói | F6.5.1 | MVP | QS-34 |
| FR-11-01 | Threat research phải phát hành ruleset có version, rollout theo wave, log-only trước khi chặn | F11.1.1, F11.1.2 | MVP | |
| FR-11-02 | Threat research phải phát hành rule khẩn toàn fleet | F11.2.1 | MVP | QS-5 |
| FR-11-03 | Hệ thống nên nhập feed IP reputation và gợi ý virtual patch cho zone bị ảnh hưởng bởi CVE | F11.3.1, F11.4.1 | MR | |

### F7 Quan sát và điều tra

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-7-01 | Analytics phải nhận đủ security event, access log lấy mẫu theo gói, và đối soát số bản ghi | F7.1.1, F7.1.2, F7.1.4 | MVP | QS-10 |
| FR-7-02 | Portal phải hiện luồng security event trực tiếp của zone đang xem | F7.2.1 | MVP | QS-37, QS-38 |
| FR-7-03 | Người dùng phải tra một request ID và thấy rule, bước pipeline, policy version, người duyệt | F7.3.1 | MVP | QS-11, QS-12 |
| FR-7-04 | Người dùng phải lọc, nhóm event theo trường và thời gian trong retention của gói | F7.3.2 | MVP | Truy vấn 24 giờ của một zone ≤ 3 s p95 ([OQ-9](#9-câu-hỏi-mở-cho-review)) |
| FR-7-05 | Người dùng phải lưu truy vấn và chia sẻ trong account | F7.3.3 | MVP | |
| FR-7-06 | Portal phải có dashboard traffic, bảo mật, cache, origin, hiệu quả rule | F7.4.1, F7.4.2 | MVP | |
| FR-7-07 | Người dùng phải đánh dấu event là chặn nhầm, thấy tỉ lệ chặn nhầm theo rule, và tạo nháp exception từ event | F7.5.1–F7.5.3 | MVP | Exception nháp điền sẵn rule, đường dẫn, trường |
| FR-7-08 | Người dùng phải cấu hình đích export SIEM; Analytics đẩy liên tục, thử lại, cảnh báo khi đích lỗi | F7.6.1, F7.6.2 | MVP | Đích lỗi 24 giờ không làm mất event trong retention |
| FR-7-09 | Mọi truy vấn dữ liệu traffic phải đi qua Query API áp quyền theo tenant | F7.3 | MVP | QS-9 |
| FR-7-10 | Portal phải có live tail lọc theo rule ID, IP, path, hành động, gồm hit log-only; lọc trước khi đẩy về trình duyệt; giới hạn tốc độ và thời lượng phiên; theo quyền xem dữ liệu request | F7.2.3 | MVP | Phiên live tail chỉ nhận event khớp bộ lọc |
| FR-7-11 | Đích export phải có bộ lọc theo hành động, loại event, zone, trường, và lấy mẫu log allow | F7.6.3 | MVP | Đích lọc "chỉ block, challenge" không nhận bản ghi allow nào |
| FR-7-12 | Mỗi đợt tấn công DDoS L7 hoặc vuln-scan phải là **một bản ghi**, mở khi chạm ngưỡng và đóng khi hết ([BR-38](#4-quy-tắc-nghiệp-vụ)) | F7.7.1 | MVP | Một đợt flood 30 phút sinh một bản ghi, không phải hàng nghìn |
| FR-7-13 | Lịch sử đợt tấn công và event WAF giữ theo gói; khách xem nếu gói cho phép; nhân viên NCC xem theo quyền được cấp, có audit | F7.7.2, F7.7.3 | MVP | Gói không cho xem thì ẩn màn, không trả dữ liệu |
| FR-7-14 | Portal phải có màn event WAF riêng: danh sách, chi tiết, rule vi phạm, và tuning chặn nhầm từ event | F7.8.1, F7.8.2 | MVP | Từ một event bấm tạo exception nháp điền sẵn rule, đường dẫn |

### F8 Phát hiện, cảnh báo, sự cố

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-8-01 | Người dùng phải tạo alert policy từ template, chọn phạm vi zone, ngưỡng, mức | F8.2.1 | MVP | |
| FR-8-02 | Người dùng phải cấu hình kênh theo gói và gửi thử | F8.2.2, F8.2.4 | MVP | |
| FR-8-03 | Alert policy phải có giờ im lặng, chu kỳ lặp, gộp cảnh báo | F8.2.3 | MVP | Một sự cố kéo dài 1 giờ sinh ≤ 1 thông báo mỗi chu kỳ lặp |
| FR-8-04 | Hệ thống phải ghi kết quả gửi của từng thông báo, thử lại khi kênh lỗi | F8.3.1 | MVP | |
| FR-8-05 | SOC nên mở case tay hoặc từ đề xuất của Detection, gắn event, truy vấn, mitigation | F8.4.1, F8.4.2 | MR | |
| FR-8-06 | Case nên có workflow mở → tiếp nhận → giảm thiểu → đóng, người phụ trách, SLA, và bắt buộc tóm tắt khi đóng | F8.4.3, F8.4.4 | MR | [BR-19](#4-quy-tắc-nghiệp-vụ) |
| FR-8-07 | Chấp nhận đề xuất mitigation từ case phải tạo rule tạm có hạn qua kênh khẩn | F8.1.3, F5.5.2 | MR | |
| FR-8-08 | Notifier phải dedup theo alert, zone, nguồn: cùng khoá trong cửa sổ gộp (mặc định 30 phút) chỉ gửi tin đầu, các lần sau gom vào thread hoặc digest; mức tăng hoặc nguồn mới phá dedup | F8.3.2 | MVP | Alert W3 bắn 1.000 lần trong 30 phút sinh 1 tin và 1 digest trên Slack |
| FR-8-09 | Thông báo nên kèm link xử lý một bấm tới mitigation đề xuất theo [BR-30](#4-quy-tắc-nghiệp-vụ) | F8.3.3 | MR | Mở link bằng trình quét link (không đăng nhập) không thay đổi gì |
| FR-8-10 | Mỗi loại tấn công (DDoS L7, vuln-scan và các loại khác) phải có ngưỡng mặc định ở ba mức low, medium, high; khách chỉnh ở tầng zone; chạm ngưỡng mới lên cảnh báo | F8.2.5 | MVP | Zone không chỉnh gì vẫn nhận cảnh báo theo mặc định |
| FR-8-11 | Nội dung thông báo dùng template có biến, có mẫu mặc định, khách tuỳ chỉnh được | F8.2.6 | MVP | Xem trước được trước khi lưu |

### F9 Báo cáo và tuân thủ

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-9-01 | Người dùng nên định nghĩa báo cáo, lịch gửi, người nhận; xuất PDF/CSV | F9.1.1, F9.1.2 | MR | |
| FR-9-02 | Hệ thống nên có báo cáo tuân thủ: thay đổi policy và người duyệt, phân quyền, retention | F9.2.1 | MR | Đủ làm bằng chứng cho PCI DSS 6.4.2 |
| FR-9-03 | Log traffic phải bị xoá khi hết retention của gói | F9.3.1 | MVP | |
| FR-9-04 | Audit log phải giữ tối thiểu 13 tháng truy vấn được, sau đó archive, không xoá ([BR-12](#4-quy-tắc-nghiệp-vụ)) | F9.3.2 | MVP | |
| FR-9-05 | Hệ thống phải xoá hoặc ẩn danh dữ liệu cá nhân của một người theo yêu cầu ([OQ-6](#9-câu-hỏi-mở-cho-review)) | F9.3.3 | MVP | Tuân thủ Nghị định 13/2023/NĐ-CP |
| FR-9-06 | Account nên chọn region lưu log; tenant Enterprise chọn region lưu cấu hình | F9.4.1, F9.4.2 | NC | |

### F10 Gói và billing

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-10-01 | NCC phải định nghĩa gói, giá (VND, USD), hạn mức, đơn giá vượt, entitlements; thay đổi gói tạo version mới, subscription cũ giữ version cũ tới khi gia hạn | F10.1.1 | MVP | Hoá đơn của kỳ luôn tính theo version gói của kỳ đó |
| FR-10-02 | Sales phải tạo gói riêng cho một hợp đồng Enterprise | F10.1.2 | MVP | Gói riêng không hiện cho tenant khác |
| FR-10-03 | Khách phải dùng thử 14 ngày tính năng Business trên một zone có trần ([BR-8](#4-quy-tắc-nghiệp-vụ)) | F10.2.1 | MVP | |
| FR-10-04 | Owner, billing phải mua, nâng, hạ gói theo zone; nâng có hiệu lực ngay, hạ có hiệu lực cuối kỳ ([OQ-10](#9-câu-hỏi-mở-cho-review)) | F10.2.2 | MVP | QS-14, QS-34 |
| FR-10-05 | Quá hạn thanh toán phải chuyển zone sang chuyển tiếp tối giản 30 ngày rồi tạm dừng ([BR-9](#4-quy-tắc-nghiệp-vụ)) | F10.2.3 | MVP | Có thông báo ở mỗi bước |
| FR-10-06 | Management phải tính entitlements của zone từ gói, add-on, hợp đồng và phát sang Control khi đổi | F10.3.1, F10.3.2 | MVP | QS-14 |
| FR-10-07 | Management phải từ chối tạo đối tượng vượt giới hạn gói (số rule, đích export, kênh…) | F10.3.3 | MVP | Lỗi nêu giới hạn và gói cần nâng |
| FR-10-08 | Edge phải đếm băng thông sạch và request theo zone; Analytics gộp và đối soát; Management nhận usage theo ngày | F10.4.1–F10.4.3 | MVP | QS-35 |
| FR-10-09 | Owner, billing phải nhận cảnh báo ở 80% và 100% hạn mức | F10.5.1 | MVP | Mỗi mốc gửi một lần mỗi kỳ |
| FR-10-10 | Gói có trần phải phát lệnh áp trần khi vượt | F10.5.2 | MVP | QS-36 |
| FR-10-11 | Hệ thống phải lập hoá đơn cuối kỳ gồm phí nền, phí vượt, add-on, thuế; xuất hoá đơn điện tử | F10.6.1, F10.6.3 | MVP | Tổng phí vượt khớp usage đã đối soát |
| FR-10-12 | Hệ thống phải thu tiền qua cổng thanh toán; chỉ lưu token của cổng | F10.6.2 | MVP | Không có số thẻ trong hệ thống (ngoài phạm vi PCI DSS SAQ D) |
| FR-10-13 | NCC nên điều chỉnh, hoàn tiền, ghi có | F10.6.4 | MR | Mọi điều chỉnh có audit và lý do |
| FR-10-14 | Khách phải đặt được trần chi tiêu; cách xử lý phần dùng vượt quota (trừ sau bao nhiêu, trừ vào đâu) chờ phân tích ([3.5](03-business-analysis/05-plans-billing.md#3-đã-quyết)) | F10.5.3 | MVP | |

### F12 Vận hành NCC

| ID | Yêu cầu | F | Mức | Tiêu chí chấp nhận |
|---|---|---|---|---|
| FR-12-01 | NOC phải rút một PoP khỏi DNS và đóng băng phát hành toàn fleet | F12.2.1, F12.2.2 | MVP | QS-16 |
| FR-12-02 | Support phải tra request ID của khách mà không có quyền đọc dữ liệu khác của tenant | F12.3.1 | MVP | |
| FR-12-03 | Support chỉ được vào tenant khi khách cấp quyền, có hạn, có audit hiện cho khách ([BR-20](#4-quy-tắc-nghiệp-vụ)) | F12.3.2 | MVP | |
| FR-12-04 | NCC phải tạm khoá, mở khoá org (vi phạm, gian lận) kèm lý do | F12.4.1 | MVP | Zone của org bị khoá chuyển theo [BR-9](#4-quy-tắc-nghiệp-vụ), không cắt đột ngột |
| FR-12-05 | NCC phải quản lý danh mục gói, đối tác hạ tầng, PoP | F12.4.2 | MVP | |
| FR-12-06 | NCC phải bật được mitigation (Under Attack mode, rule tạm) trên zone của khách từ portal vận hành của NCC; khách tự bật trên portal của khách; mọi lần NCC bật hiện cho khách và có audit ([3.3](03-business-analysis/03-attack-response.md#3-đã-quyết)) | F12.4.3 | MVP | |
| FR-12-07 | Support NCC phải đặt được log-only có hạn cho một rule khẩn của NCC trên đúng một zone bị chặn nhầm, có audit; khách không tự tắt được rule khẩn ([BR-33](#4-quy-tắc-nghiệp-vụ)) | F12.4.4 | MVP | Hết hạn thì rule khẩn tự chặn lại trên zone đó |

## 4. Quy tắc nghiệp vụ

| ID | Quy tắc | Áp cho |
|---|---|---|
| BR-1 | Một domain gốc thuộc tối đa một zone chưa gỡ trên toàn nền tảng. Subdomain của domain đã có zone ở tenant khác bị từ chối ([OQ-2](#9-câu-hỏi-mở-cho-review)) | F2.1.1 |
| BR-2 | Zone chỉ kích hoạt khi qua cổng OG1: mọi hostname đã chọn đã xác minh, có cert, có origin đã kiểm (hoặc khách xác nhận bỏ qua), đã chọn preset ([requirements §3.1](03-business-analysis/01-onboarding.md)) | F2.4.1 |
| BR-3 | Một zone thuộc đúng một account; entitlements tính theo zone | F2, F10 |
| BR-4 | PolicyVersion không sửa được. Rollback tạo version mới có nội dung của version cũ | F5.3.4, F5.4.3 |
| BR-5 | Khi bật 4-eyes: người duyệt khác người tạo và khác mọi người đã sửa bản nháp đó; người duyệt phải có quyền duyệt trên mọi zone trong change request | F5.3.2 |
| BR-6 | Kênh khẩn gồm: Under Attack mode, tạm dừng zone, rule tạm, rule khẩn của NCC. Không cần duyệt, luôn có audit. Rule tạm hết hạn tối đa sau 72 giờ, gia hạn phải đi đường thường ([OQ-5](#9-câu-hỏi-mở-cho-review)) | F5.5 |
| BR-7 | Traffic bị edge chặn hoặc challenge không tính vào usage tính tiền | F10.4 |
| BR-8 | Mỗi org dùng thử một lần; định danh dùng thử gồm org, domain email của owner, phương thức thanh toán, domain zone | F10.2.1 |
| BR-9 | Không cắt traffic đột ngột: hết dùng thử, quá hạn, bị khoá đều qua chuyển tiếp tối giản 30 ngày rồi mới tạm dừng | F10.2.3, F12.4.1 |
| BR-10 | Gói trả phí không bị cắt traffic vì vượt hạn mức, chỉ tính phí vượt. Gói có trần bị áp trần | F10.5 |
| BR-11 | Giới hạn gói kiểm khi tạo và khi phát hành. Hạ gói khi đang vượt giới hạn mới thì bị từ chối tới khi giảm bớt ([OQ-10](#9-câu-hỏi-mở-cho-review)) | F10.3.3 |
| BR-12 | Audit không xoá, không sửa. Dữ liệu cá nhân trong audit xử lý theo [OQ-6](#9-câu-hỏi-mở-cho-review) | F1.5, F9.3.2 |
| BR-13 | Org luôn có ít nhất một owner; không gỡ hay hạ quyền được owner cuối cùng | F1.2.4 |
| BR-14 | Secret của API key chỉ hiện lúc tạo; hệ thống chỉ lưu hash và tiền tố để nhận diện | F1.4.1 |
| BR-15 | Gỡ thành viên hoặc khoá user ở IdP thu hồi phiên và key cá nhân trong ≤ 1 phút | F1.2.4, F1.2.5 |
| BR-16 | Gỡ zone giải phóng domain và hostname ngay; dữ liệu của zone giữ theo retention rồi xoá | F2.4.3 |
| BR-17 | Một hostname thuộc tối đa một zone chưa gỡ trên toàn nền tảng | F2.2.1 |
| BR-18 | Thay đổi ảnh hưởng traffic chỉ tới edge qua PolicyVersion hoặc kênh khẩn, không có đường thứ ba | F5, F6 |
| BR-19 | Case chỉ đóng khi có tóm tắt và mọi mitigation tạm của case đã hết hạn hoặc được chuyển thành rule thường | F8.4 |
| BR-20 | Support của NCC chỉ vào tenant khi khách cấp quyền, tối đa 72 giờ, khách thấy và thu hồi được | F12.3.2 |
| BR-21 | Exception luôn có phạm vi zone; exception tắt toàn bộ managed ruleset cho cả zone phải đi qua duyệt kể cả khi 4-eyes tắt | F5.1.4 |
| BR-22 | Claim domain chưa xác minh hết hạn sau 14 ngày; domain được giải phóng | F2.1.8 |
| BR-23 | Không cấp cert cho hostname chưa xác minh sở hữu | F2.3.1 |
| BR-24 | DDoS L7 và lọc sớm chặn từ request đầu tiên, không theo log-only của onboarding. Bật chặn phần còn lại lần đầu cần ≥ 7 ngày và ≥ 10.000 request log-only ([OQ-17](03-business-analysis/01-onboarding.md#a9-câu-hỏi-mở)), trừ khi khách chọn "đang bị tấn công" (có audit) | F2.6.1, F2.7.4 |
| BR-25 | Chặn nhầm được xử lý bằng tuning rule (tắt rule, đổi ngưỡng hoặc paranoia, log-only, sửa custom rule) hoặc whitelist chọn theo tình huống (IP, path, tham số / header / cookie, client đã xác định); cả hai đi qua change request như mọi thay đổi policy ([3.4](03-business-analysis/04-false-positive.md#3-đã-quyết)) | F5.1, F7.5 |
| BR-26 | Khách self-service tự làm chủ: thay đổi phát hành không cần ai duyệt. Khách doanh nghiệp (account bật chế độ cần duyệt) và admin của NCC có phân chia quyền theo từng bước, người duyệt khác người gửi ([3.2](03-business-analysis/02-policy-change.md#3-đã-quyết), [3.11](03-business-analysis/11-provider-operations.md#3-đã-quyết)) | F5.3, F12.4 |
| BR-27 | Emergency override chỉ cho thay đổi **siết chặt** (block IP, ASN, quốc gia; siết rate limit; bật Under Attack), không cho nới lỏng (allowlist, Trust, Skip, tắt rule, tắt WAF). Chỉ role được giao mới dùng; bắt buộc có hạn (mặc định 24 giờ, tự gỡ khi hết hạn); người thứ hai phải duyệt bù trong hạn, nếu không thì tự rollback; mỗi lần dùng gửi cảnh báo critical tới mọi Owner, Admin ([3.13](03-business-analysis/13-configuration-surface.md#4-đã-quyết)) | F5.5.3 |
| BR-28 | **Trust** bỏ qua bot và challenge, WAF vẫn kiểm, vẫn chịu DDoS L7 và rate limit. **Skip** bỏ qua đúng một chặng đã chỉ định trong phạm vi đã chỉ định; DDoS L7 không skip được. Cả hai vẫn ghi log. Bỏ qua bot cho crawler dùng bot đã xác minh, không dùng IP hay User-Agent | F3.2.4 |
| BR-29 | Policy của khách chỉ tự rollback khi lỗi biên dịch hoặc áp dụng, node lỗi, độ trễ tăng; thay đổi tỉ lệ chặn, traffic chỉ cảnh báo và gợi ý rollback. Bản phát hành của NCC (ruleset, engine, bundle chung) tự rollback theo cả gate traffic | F6.3.1 |
| BR-30 | Link trong thông báo không bao giờ tự thực thi hành động: token chỉ trỏ tới đề xuất, có hạn ngắn, không thay cho đăng nhập; hành động chạy sau khi người dùng đăng nhập và xác nhận, qua kênh khẩn | F8.3.3 |
| BR-31 | Header IP client thật chỉ được tin khi kết nối đến từ dải IP CDN khách đã khai; nếu không, edge dùng IP TCP | F3.1.4 |
| BR-32 | Thứ tự pipeline, rẻ trước đắt sau: L3/L4 → TLS → chuẩn hoá giao thức → access list (IP, CIDR, ASN, quốc gia, reputation) → **DDoS L7** → tính điểm bot → Under Attack, challenge → kiểm API rẻ (endpoint, xác thực, tham số) → rate limit theo rule → policy bot, custom rule phần rẻ → **WAF: schema body trước, rồi managed ruleset và custom rule cần body** → cache → origin. Trust và Skip không bỏ qua được DDoS L7 ([BR-28](#4-quy-tắc-nghiệp-vụ)) | F3 |
| BR-33 | Xung đột rule: (1) trong một chặng, rule chạy theo priority từ trên xuống, gặp hành động kết thúc (block, challenge, allow, trust) đầu tiên thì dừng chặng đó; `log` không dừng; (2) trong access list, mục cụ thể hơn thắng: IP > CIDR > ASN > quốc gia; (3) giữa NCC và khách: block của NCC (lớp rule khẩn) > allow của khách > block của khách > allow của NCC. Khách không tự tắt được rule khẩn; support đặt log-only có hạn cho đúng một zone (FR-12-07) | F3.5, F3.8, F12.4.4 |
| BR-34 | Hành động mặc định theo bot score (1–99, thấp là bot) và nhóm path: path thường: dưới 10 challenge, 10 đến dưới 50 log, từ 50 allow; path nhạy cảm (login, checkout): dưới 10 block, 10 đến dưới 50 challenge tương tác, từ 50 allow; path API: không challenge, dùng rate limit theo consumer. Mặc định dùng managed challenge (edge tự chọn JS hay tương tác theo điểm). Khách sửa được mọi mốc | F3.6.2 |
| BR-35 | Endpoint API có schema: kiểm schema chạy trước managed ruleset trong tầng WAF, request sai schema bị loại không quét signature; endpoint đang enforce schema được giảm paranoia CRS để giảm chặn nhầm | F3.7.1 |
| BR-36 | Log-only không bao giờ tự chuyển sang chặn: sau thời gian quan sát (mặc định 7 ngày, BR-24) hệ thống gửi báo cáo số request sẽ bị chặn và rule nghi chặn nhầm; người dùng bấm chuyển, bước chuyển đi qua change request | F5.1.6 |
| BR-37 | Giữa các tầng cấu hình: tầng trên ghi đè tầng dưới (system trên global trên website). Whitelist ưu tiên trong các cặp whitelist, blacklist cùng tầng; whitelist của website không thắng tầng trên và không bỏ qua WAF, rate limit, DDoS ([BR-28](#4-quy-tắc-nghiệp-vụ)); rule khẩn của NCC không bị whitelist nào đè ([BR-33](#4-quy-tắc-nghiệp-vụ)); whitelist của global thắng blacklist của website và CRS ([OQ-163](03-business-analysis/14-config-tiers-attack-history-thresholds.md#4-đã-chốt-theo-đề-xuất-của-claude)) | F5.7.4 |
| BR-38 | Một đợt tấn công là một bản ghi, thuộc đúng một trong hai loại (DDoS L7, vuln-scan); chỉ mở đợt và gửi cảnh báo khi vượt ngưỡng đã cấu hình | F7.7.1 |

## 5. Yêu cầu dữ liệu

Bảng này là đầu vào trực tiếp cho thiết kế database. "Chủ" là plane giữ bản gốc; plane khác chỉ giữ bản sao
hoặc đọc qua API.

| ID | Nhóm dữ liệu | Đối tượng chính | Chủ | Quy mô thiết kế | Retention | Độ nhạy |
|---|---|---|---|---|---|---|
| DT-1 | Tenant, danh tính | Org, Account, Member, Invitation, Role, RoleBinding, Group, SsoConnection, SsoDomain, ServiceAccount, ApiKey, MfaFactor, Session | M | 400k org, 1,5 triệu member | Tới khi xoá org + thời hạn giữ | Dữ liệu cá nhân (email, tên, IP đăng nhập); secret SSO ở secret store |
| DT-2 | Registry toàn cục | DomainClaim, HostnameClaim, ApiKey prefix, SsoDomain claim, TrialClaim, nơi đặt tenant | M (dùng chung mọi tenant) | ≈ 5 triệu dòng | Theo vòng đời đối tượng gốc | Thấp |
| DT-3 | Cấu hình zone | Zone, Hostname, Origin, OriginPool, ZoneSettings, ZoneGroup, ZoneTag, CacheRule, DnsRecord, CustomErrorPage, TrustedProxyRange, DomainVerification, OriginCheck, DnsSnapshot, OnboardingProgress; ZonePreset là danh mục của NCC | M | 1 triệu zone, 3 triệu hostname | Tới khi gỡ zone | Thấp; secret tới origin ở secret store |
| DT-4 | Policy nháp | PolicyDraft, CustomRule, RateLimitRule, AccessList, ManagedRulesetConfig, Exception, DdosSettings, BotPolicy, ApiEndpoint, ApiSchema, ApiAuthConfig, PolicyTemplate, PolicyBinding | M | Tới 1.000 rule mỗi zone, trung bình ≈ 10 | Tới khi gỡ zone | Trung bình (lộ cách phòng thủ của khách) |
| DT-5 | Policy đã phát hành | PolicyVersion (tài liệu bất biến + `content_hash`) | M | ≈ 50 triệu mỗi năm | Giữ đủ để rollback và kiểm toán ([OQ-7](#9-câu-hỏi-mở-cho-review)) | Trung bình |
| DT-6 | Thay đổi và duyệt | ChangeRequest, Approval, ChangeWindow, TemporaryMitigation, BulkOperation, GitSource | M | ≈ thay đổi cấu hình 1,7/s | 13 tháng truy vấn được | Trung bình |
| DT-7 | Audit | AuditEntry | M | Lớn nhất của Management, ghi thêm | Không xoá; archive sau 13 tháng | Có dữ liệu cá nhân |
| DT-8 | Gói, billing | Plan, PlanVersion, Price, AddOn, Subscription, Entitlements, UsageDaily, QuotaNotice, Invoice, InvoiceLine, Payment, PaymentMethod (token), CreditNote | M | 1 triệu subscription, 1 triệu dòng usage mỗi ngày | Hoá đơn, chứng từ ≥ 10 năm (Luật Kế toán) | Tài chính; không lưu số thẻ |
| DT-9 | Vận hành bảo mật của khách | SavedSearch, AlertPolicy, AlertThreshold, NotificationTemplate, NotificationChannel, ExportDestination, FalsePositiveLabel, Case, CaseLink, ReportDefinition, ReportRun | M | Nhỏ so với DT-3 | Theo vòng đời; ReportRun 13 tháng | Credential kênh, đích export ở secret store |
| DT-10 | Dữ liệu NCC | Partner, Pop, SupportAccessGrant, trạng thái khoá org | M (`PLATFORM_TENANT_ID`) | Nhỏ | Theo vòng đời | Thấp |
| DT-11 | Phân phối, fleet | RuleSet, RuleSetAssignment, ZoneBundle, Manifest, CompileResult, Rollout, RolloutStatus, Node, NodeCert, NodeDenyList, ZonePlacement, RulesetRelease, EmergencyRule, EmergencyRuleOverride, ReputationFeed, PopState, FleetFreeze | C | 8 GB bundle mỗi PoP, ≈ 700 node | Bundle: N version gần nhất mỗi zone | Khoá ký ở KMS |
| DT-12 | Telemetry | SecurityEvent, AttackEpisode, AccessLog, RequestSample, UsageCounter, AlertState, NotificationDelivery, ReplayJob, bảng tổng hợp | A | ≈ 1,15 triệu bản ghi/s, 12 TB/ngày | Theo gói: 24 giờ đến 365 ngày | Có IP và header của end-user |

Yêu cầu chung cho dữ liệu:

| ID | Yêu cầu |
|---|---|
| DT-13 | Mọi bản ghi của khách mang tenant; không truy vấn nào của khách đọc được bản ghi tenant khác (QS-9) |
| DT-14 | Mọi entity ghi được bởi khách có optimistic locking; ghi đè im lặng là lỗi |
| DT-15 | Xoá do khách thực hiện là xoá mềm; xoá cứng chỉ khi hết retention hoặc xoá org |
| DT-16 | Không có secret (khoá riêng cert, credential origin, token kênh, secret SSO, secret API key) trong database, log, audit hay bản sao lưu; chỉ tham chiếu tới secret store |
| DT-17 | Thay đổi dữ liệu cần tới Control hay Analytics phát sự kiện trong cùng giao dịch với lần ghi |

## 6. Yêu cầu phi chức năng

Kịch bản đo được của toàn hệ thống ở [10.3](../architecture/10-quality/00-index.md#103-kịch-bản) (QS-1…QS-41) là phần
chính. Dưới đây chỉ là những điểm 10.3 chưa có, chủ yếu cho Management.

| ID | Thuộc tính | Yêu cầu | Đề xuất |
|---|---|---|---|
| NFR-1 | Khôi phục | Mất database Management | RPO ≤ 1 phút, RTO ≤ 1 giờ; traffic không bị ảnh hưởng (edge chạy bằng snapshot) |
| NFR-2 | Khôi phục | Khôi phục về một thời điểm | Point-in-time recovery trong 14 ngày; diễn tập mỗi quý |
| NFR-3 | Hiệu năng | API ghi của Management | p95 ≤ 500 ms (không tính biên dịch, mô phỏng) |
| NFR-4 | Hiệu năng | Mở một trang portal bất kỳ | p95 ≤ 2 s trên đường truyền 10 Mbps |
| NFR-5 | Bảo vệ API | Public API theo API key và theo tenant | Có rate limit, trả `429` kèm `Retry-After`; một tenant chạm giới hạn không làm chậm tenant khác |
| NFR-6 | Bảo mật | Portal và public API | Đạt OWASP ASVS mức 2; pentest bên thứ ba trước khi bán |
| NFR-7 | Bảo mật | Dữ liệu lưu | Mã hoá lúc lưu và trên đường truyền; bản sao lưu mã hoá |
| NFR-8 | Tương thích | Public API | Có version; thay đổi phá vỡ chỉ ở version mới, báo trước ≥ 6 tháng |
| NFR-9 | Ngôn ngữ | Portal, email, hoá đơn | Tiếng Việt và tiếng Anh |
| NFR-10 | Trình duyệt | Portal | Hai bản mới nhất của Chrome, Edge, Firefox, Safari |
| NFR-11 | Truy cập | Portal | WCAG 2.1 AA cho luồng chính ([OQ-11](#9-câu-hỏi-mở-cho-review)) |

## 7. Giao tiếp với hệ thống ngoài

| ID | Hệ thống | Chiều | Yêu cầu | Mức |
|---|---|---|---|---|
| IR-1 | IdP của khách | Vào | SAML 2.0, OIDC; SCIM 2.0 (`/Users`, `/Groups`) | MVP / MR |
| IR-2 | Cổng thanh toán | Hai chiều | Thẻ quốc tế và cổng nội địa; webhook kết quả có chữ ký; idempotent ([OQ-12](#9-câu-hỏi-mở-cho-review)) | MVP |
| IR-3 | Nhà cung cấp hoá đơn điện tử | Ra | Xuất hoá đơn theo Nghị định 123/2020/NĐ-CP | MVP |
| IR-4 | CA ACME | Ra | Let's Encrypt và một CA dự phòng | MVP |
| IR-5 | DNS công cộng | Ra | Kiểm bản ghi xác minh, trạng thái trỏ | MVP |
| IR-6 | SIEM, kho lưu của khách | Ra | Splunk HEC, S3-compatible, HTTP, Kafka, syslog | MVP |
| IR-7 | Kênh thông báo | Ra | Email, webhook có chữ ký, Slack, Teams, PagerDuty, SMS | MVP / MR |
| IR-8 | Terraform, Git | Vào | Provider dùng public API; GitOps đọc repo của khách | MR |
| IR-9 | Feed threat intel | Vào | IP reputation, CVE | MR |

## 8. Truy vết

Phân rã theo plane, thành phần và các liên kết `LK-…` giữa plane ở [requirements §1](01-functional-decomposition.md#phân-rã-theo-plane).

| Nhóm | Building block | Runtime | Database |
|---|---|---|---|
| F1, F2, F5, F8.4, F9, F10, F12 (phần M) | [5.1](../architecture/05-building-blocks/01-management-plane.md), [5.8.12](../architecture/05-building-blocks/08-components/12-management.md) | [6](../architecture/06-runtime/00-index.md) | [5.8.14](../architecture/05-building-blocks/08-components/14-management-database.md) |
| F6, F11, F2.3 | [5.2](../architecture/05-building-blocks/02-control-plane.md), [5.8.5](../architecture/05-building-blocks/08-components/05-config-and-compiler.md)–[5.8.7](../architecture/05-building-blocks/08-components/07-cert-ruleset-release.md) | [8.15](../architecture/08-cross-cutting/15-config-propagation-and-apply.md) | Chưa thiết kế |
| F3, F4 | [5.3](../architecture/05-building-blocks/03-edge-plane.md), [5.8.1](../architecture/05-building-blocks/08-components/01-l7.md)–[5.8.4](../architecture/05-building-blocks/08-components/04-edge-agent-and-state.md) | [6](../architecture/06-runtime/00-index.md) | Không có database (PoP store) |
| F7, F8.1–F8.3 | [5.4](../architecture/05-building-blocks/04-analytics-plane.md), [5.8.8](../architecture/05-building-blocks/08-components/08-ingestion-and-storage.md)–[5.8.11](../architecture/05-building-blocks/08-components/11-notification-fanout.md), [5.8.13](../architecture/05-building-blocks/08-components/13-realtime-gateway.md) | [6.10](../architecture/06-runtime/10-analytics-flows.md) | ClickHouse ([5.8.8](../architecture/05-building-blocks/08-components/08-ingestion-and-storage.md)) |

## 9. Câu hỏi mở cho review

| ID | Câu hỏi | Đề xuất hiện tại | Ảnh hưởng |
|---|---|---|---|
| OQ-1 | Phân cấp org → account → zone có đủ không, hay cần thêm cấp (đơn vị, dự án) cho khách lớn? | Đủ cho MVP; nhóm zone thay cho cấp thứ tư | DT-1, quyền |
| OQ-2 | Tenant B muốn bảo vệ `shop.example.com` khi tenant A giữ `example.com`: từ chối, hay cho phép khi A đồng ý? | Từ chối ở MVP | BR-1, DT-2 |
| OQ-3 | Mất bản ghi xác minh thì làm gì, sau bao lâu? | Cảnh báo ở ngày 7, tạm dừng ở ngày 14 | FR-2-02 |
| OQ-4 | Hết 7 ngày log-only: tự chuyển sang chặn, hay chờ khách bấm? | Chờ khách bấm, nhắc mỗi tuần | FR-2-11 |
| OQ-5 | Hạn tối đa của rule tạm qua kênh khẩn | 72 giờ | BR-6 |
| OQ-6 | Xoá org và xoá dữ liệu cá nhân khi audit không được xoá: ẩn danh danh tính trong audit, hay giữ nguyên vì nghĩa vụ pháp lý? Thời hạn giữ sau khi xoá org? | Ẩn danh trường danh tính (thay bằng mã), giữ bản ghi; giữ 30 ngày sau yêu cầu xoá org | BR-12, FR-1-03, FR-9-05, DT-7 |
| OQ-7 | Giữ bao nhiêu PolicyVersion mỗi zone? | Tất cả trong 13 tháng, sau đó 50 bản gần nhất + bản đang chạy | DT-5, dung lượng |
| OQ-8 | Sai lệch cho phép của rate limit toàn fleet | ≤ 10% | FR-3-06 |
| OQ-9 | Mục tiêu độ trễ truy vấn event cho khoảng dài (30 ngày, 365 ngày) | 24 giờ ≤ 3 s; 30 ngày ≤ 15 s | FR-7-04 |
| OQ-10 | Hạ gói: hiệu lực ngay hay cuối kỳ; xử lý phần vượt giới hạn gói mới thế nào? | Cuối kỳ; chặn hạ gói tới khi khách giảm bớt | FR-10-04, BR-11 |
| OQ-11 | WCAG có bắt buộc cho khách khu vực công không? | AA cho luồng chính | NFR-11 |
| OQ-12 | Cổng thanh toán cụ thể và tính phí theo kỳ trả trước hay trả sau? | Phí nền trả trước, phí vượt trả sau | IR-2, FR-10-11 |
| OQ-13 | Developer có được tạo exception không cần duyệt trong zone được giao không? | Không, luôn qua change request | FR-1-06, F5.1.4 |
| OQ-14 | Billing theo account hay theo org (một hoá đơn cho cả org)? | Theo account, gộp được ở cấp org | DT-8 |
| OQ-15 | Chốt các hợp đồng tạm ở [5.5](../architecture/05-building-blocks/05-contracts.md#hợp-đồng-bổ-sung-tạm-2026-11-06) (purge, kết quả biên dịch, rollout, cert, kiểm zone, kiểm origin, `PopState`, `PermissionsChanged`, cấu hình alert/export, nhãn chặn nhầm, replay, xoá dữ liệu, cách ly, reputation) | Đã thêm tạm; chốt dần theo từng nghiệp vụ ở §3 | 5.5, outbox của Management |
| OQ-16…OQ-165 | OQ-16…OQ-20 thuộc bản nháp onboarding cũ; OQ-21…OQ-126 và OQ-143…OQ-165 theo từng nghiệp vụ ([§3](03-business-analysis/00-index.md)); OQ-127…OQ-142 về định hướng sản phẩm ([§4](04-product-direction/00-index.md)) | Một số đã quyết hoặc có đề xuất chờ duyệt | Theo trang |
