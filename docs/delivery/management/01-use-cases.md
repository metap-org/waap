# Use case của Management plane

> **Trạng thái: bản đề xuất chờ duyệt (2026-11-18).** Bám sát [bản đồ chức năng](../../requirements/01-functional-decomposition.md) (cột "Tính năng")
> và các [luồng đã duyệt](../../requirements/03-business-analysis/00-index.md) (cột "Luồng"). Thứ tự và phạm vi lát cắt ở [kế hoạch](00-plan.md).
> Từ use case suy ra database ở [02-database.md](02-database.md).

**Cách đọc.** Mỗi use case ngắn: ai làm, làm gì, hỏng ở đâu. Không lặp lại quy tắc đã có ở luồng nghiệp vụ; cột "Luồng" trỏ về nơi có quy tắc đầy đủ.
Vai trò dùng tên tạm: **Owner** (chủ org), **Admin** (quản trị org), **QTBM** (quản trị bảo mật), **SOC**, **DevOps**, **Billing**, **Viewer**,
**End-user** (người dùng website của khách, không có tài khoản), **Hệ thống**, **NCC** (nhân viên nhà cung cấp). Tên vai trò chính thức theo [09-access-control](../../management/09-access-control.md).

**Quy tắc chung (không viết lại ở từng use case):**

- Mọi thao tác ghi sinh một dòng **audit** (ai, đối tượng, trước/sau, IP, qua UI hay API).
- Mọi thao tác **ảnh hưởng traffic** sinh một sự kiện **outbox** (cùng transaction với ghi dữ liệu).
- Mọi thao tác kiểm **quyền theo vai trò và phạm vi** (org, account, nhóm zone, zone) và **giới hạn theo gói** trước khi làm.
- Lỗi trả về theo dạng `{error:{code,message}}` kèm `fieldErrors` khi sai dữ liệu.

---

## S0 Nền: tenant và danh tính

| UC | Tên (ai) | Luồng chính | Ngoại lệ | Tính năng · Luồng |
|---|---|---|---|---|
| UC-01 | **Đăng ký org** (người dùng mới) | Nhập email, mật khẩu, tên org → tạo org (tenant), account đầu tiên, user là Owner → gửi email xác nhận → vào portal | Email đã có tài khoản: đăng nhập; email chưa xác nhận: không tạo zone | F1.1.1 · 3.6 |
| UC-02 | **Tạo, đổi tên, đóng account** (Owner, Admin) | Tạo account (tên, slug); đổi tên; đóng khi không còn zone đang chạy | Đóng khi còn zone active: từ chối, liệt kê zone | F1.1.2 |
| UC-03 | **Đăng nhập** (thành viên) | Email + mật khẩu → MFA (TOTP) nếu bật → phiên; có thể chọn org nếu thuộc nhiều org | Sai mật khẩu nhiều lần: khoá tạm; org bắt buộc MFA mà chưa đăng ký: ép đăng ký | F1.3.1, F1.2.6 · 3.6 |
| UC-04 | **Quản lý phiên** (thành viên) | Xem danh sách phiên, đăng xuất từ xa, phiên hết hạn theo cấu hình | Phiên đã thu hồi: lần gọi tiếp trả 401 | F1.3.4 |
| UC-05 | **Mời thành viên** (Owner, Admin) | Nhập email + vai trò + phạm vi (org/account/nhóm zone/zone) → gửi lời mời có hạn | Email đã là thành viên: báo; mời quá hạn mức gói: từ chối | F1.2.1 · 3.6 |
| UC-06 | **Chấp nhận lời mời** (người được mời) | Mở link → tạo tài khoản hoặc đăng nhập → trở thành thành viên với vai trò đã gán | Link hết hạn hoặc đã dùng: xin mời lại | F1.2.1 |
| UC-07 | **Gán, đổi vai trò** (Owner, Admin) | Chọn thành viên → gán vai trò có sẵn ở phạm vi (org/account/nhóm zone/zone) → hiệu lực ngay | Hạ quyền Owner cuối cùng: từ chối | F1.2.2 |
| UC-08 | **Gỡ thành viên** (Owner, Admin) | Gỡ → thu hồi mọi phiên và API key của họ trong org | Gỡ Owner duy nhất: từ chối (dùng khôi phục ở S6) | F1.2.4 |
| UC-09 | **Cấp quyền xem dữ liệu request** (Owner, QTBM) | Gán quyền riêng "xem dữ liệu request" cho thành viên; mặc định chỉ QTBM | Thành viên khác: dữ liệu request bị che | F1.2.8 · 3.4 |
| UC-10 | **Tạo API key** (thành viên có quyền) | Chọn phạm vi (quyền, zone) + hạn → hệ thống sinh key, **hiện một lần**, lưu hash + tiền tố | Org bắt buộc hạn mà không nhập: từ chối | F1.4.1, F1.4.4 · 3.6 |
| UC-11 | **Xoay, thu hồi API key** (chủ key, Admin) | Xoay: tạo key mới, key cũ hết hạn sau thời gian chồng lấn; thu hồi: vô hiệu ngay; xem lần dùng cuối | Key lộ công khai: hệ thống tự thu hồi và báo Owner (S6) | F1.4.2 · 3.6 |
| UC-12 | **Service account** (Admin, DevOps) | Tạo service account với quyền giới hạn; cấp key cho CI/CD | Xoá service account: thu hồi key của nó | F1.4.3 |
| UC-13 | **Xem, lọc audit log** (Owner, Admin, QTBM) | Lọc theo người, đối tượng, thời gian → xem trước/sau | Người xem không có quyền với zone: dòng của zone đó ẩn | F1.5.2 |

## S1 Zone và onboarding

| UC | Tên (ai) | Luồng chính | Ngoại lệ | Tính năng · Luồng |
|---|---|---|---|---|
| UC-20 | **Thêm zone** (DevOps, QTBM) | Nhập domain → chuẩn hoá, kiểm danh sách đen → tra DNS hiện tại (gợi ý origin, CDN đang đứng trước) → tạo zone `pending` và claim domain | Domain thuộc zone đang active của org khác: chặn; domain trong danh sách đen: chặn; vượt hạn mức zone: từ chối | F2.1.1, F2.1.9, F2.1.10 · 3.1 |
| UC-21 | **Xác minh sở hữu domain** (DevOps) | Chọn cách (CNAME `_acme-challenge` mặc định, hoặc TXT, hoặc file HTTP) → hệ thống đưa bản ghi cần tạo → gọi `VerifyDomain` → `verified` | Không thấy bản ghi: giữ `verifying`, kiểm lại định kỳ, hiện bản ghi còn thiếu | F2.1.1, F2.1.3 · 3.1 |
| UC-22 | **Hướng dẫn trỏ DNS** (DevOps) | Chọn cách kết nối (CNAME, ALIAS, A tĩnh, nameserver, sau CDN của khách) → hệ thống hiện đúng bản ghi cần đổi | Apex không CNAME được: gợi ý ALIAS hoặc A | F2.1.3 |
| UC-23 | **Thêm hostname** (DevOps) | Thêm FQDN (hoặc wildcard) vào zone → claim hostname | Hostname đang active ở zone khác: chặn | F2.2.1 · 3.1 |
| UC-24 | **Khai báo và kiểm origin** (DevOps) | Nhập địa chỉ, cổng, giao thức, SNI, header host, timeout/retry → gọi `CheckOrigin` → kết quả | Origin không tới được: báo lý do, không cho kích hoạt | F2.2.2, F2.2.4 · 3.1 |
| UC-25 | **Cấu hình TLS và nguồn IP client** (DevOps) | Chọn TLS tối thiểu, HSTS, TLS tới origin; nếu đứng sau CDN của khách: chọn CDN (dải tin cậy) và header IP client | Dải tuỳ chỉnh sai định dạng: từ chối | F2.2.5, F2.3.4 · 3.1 |
| UC-26 | **Chọn preset** (QTBM) | Chọn loại ứng dụng → áp preset: managed ruleset log-only, DDoS L7 và lọc sớm bật chặn, exception sẵn có (tạo **bản nháp** policy đầu tiên) | Không chọn: dùng preset chung | F2.6.1 · 3.1 |
| UC-27 | **Kích hoạt zone** (DevOps, QTBM) | Hệ thống kiểm cổng OG1 (đã xác minh, cert sẵn sàng, origin đạt, có preset) → phát hành `PolicyVersion` đầu tiên → zone `active` (log-only) | Thiếu điều kiện: liệt kê điều kiện chưa đạt | F2.4.1 · 3.1 |
| UC-28 | **Theo dõi tiến độ onboarding** (Hệ thống) | Ghi từng bước đã xong, traffic thật đầu tiên theo hostname; nhắc ở ngày 1, 3, 7 nếu bỏ dở | Hết hạn claim chưa xác minh sau 14 ngày: huỷ claim, giải phóng domain (job) | F2.7.2, F2.7.6, F2.1.8 · 3.1 |
| UC-29 | **Bật chặn theo bậc** (QTBM) | Từ log-only, chọn nhóm rule chuyển sang chặn → tạo change request (đi tiếp UC-43) | Không đủ dữ liệu log-only: cảnh báo trước khi gửi | F2.7.4 · 3.1 |
| UC-30 | **Khoá origin** (DevOps) | Hướng dẫn chỉ cho dải IP edge; ghi nhận "đã khoá" hoặc "bỏ qua" | | F2.7.5 |
| UC-31 | **Hoàn tất onboarding** (Hệ thống) | Chỉ cho hoàn tất khi không còn họ IP (A hoặc AAAA) trỏ thẳng origin | Còn một họ IP trỏ origin: cảnh báo đỏ, không hoàn tất | F2.7.7 · 3.1 |
| UC-32 | **Tạm dừng, mở lại zone** (DevOps, QTBM) | Tạm dừng: edge chuyển tiếp không bảo vệ; mở lại: bảo vệ trở lại | Zone `pending`: không tạm dừng được | F2.4.2 |
| UC-33 | **Gỡ zone** (Owner, Admin) | Gỡ → zone `removed`, giải phóng domain và hostname; giữ cấu hình 30 ngày để khôi phục | Còn thay đổi đang chờ: huỷ chúng | F2.4.3 · 3.7 |
| UC-34 | **Gắn tag, nhóm zone** (Admin, DevOps) | Gắn tag cho zone; tạo nhóm zone để phân quyền | Tag trùng: dùng lại | F2.5.1 |

## S2 Policy và thay đổi

| UC | Tên (ai) | Luồng chính | Ngoại lệ | Tính năng · Luồng |
|---|---|---|---|---|
| UC-40 | **Soạn đối tượng policy** (QTBM) | Tạo, sửa, xoá **bản nháp** của custom rule, rate limit, access list, exception (có phạm vi, có hạn) bằng builder hoặc biểu thức | Sai cú pháp, vượt số rule theo gói: báo theo dòng, giữ nháp | F5.1.1, F5.1.3, F5.1.4, F5.2.1 · 3.2 |
| UC-41 | **Cấu hình managed ruleset, DDoS, log-only** (QTBM) | Chọn profile, paranoia, ngưỡng, rule tắt; DDoS L7; bật log-only theo rule hoặc zone | Giá trị ngoài khoảng: từ chối | F5.1.2, F5.1.5, F5.1.6 |
| UC-42 | **Kiểm nháp** (QTBM) | Bấm kiểm → `CompilePolicy` → kết quả (cú pháp, rule đè nhau, rule không bao giờ khớp, ước lượng chi phí) | Lỗi: trả theo dòng, quay lại UC-40 | F5.2.1, F5.2.2 · 3.2 |
| UC-43 | **Gửi change request** (QTBM, DevOps) | Từ bản nháp → chụp diff với version đang chạy + kết quả kiểm → tạo change request | Thay đổi không ảnh hưởng traffic (đổi tên, mô tả, tag): áp thẳng, chỉ audit (UC-52) | F5.3.1 · 3.2 |
| UC-44 | **Duyệt change request** (người duyệt, khác người gửi) | Xem diff → duyệt hoặc từ chối kèm lý do | Người duyệt trùng người gửi: từ chối; 4-eyes chưa đủ hai người có quyền duyệt: không bật được | F5.3.2, F5.3.6 · 3.2 |
| UC-45 | **Tự xác nhận diff** (org một người) | Người gửi xác nhận diff của chính mình, không có thời gian chờ | Account bật 4-eyes: dùng UC-44 | F5.3.6 · 3.2 |
| UC-46 | **Phát hành** (Hệ thống) | Sau duyệt: tạo `PolicyVersion` bất biến (băm nội dung) → phát sự kiện `PolicyPublished` (outbox) → change request `published` | Nháp đã đổi so với diff đã duyệt: yêu cầu chụp lại; xung đột với change request khác: báo | F5.3.4 · 3.2 |
| UC-47 | **Theo dõi phát hành** (QTBM) | Hiện trạng thái: đã biên dịch, % node đã áp, canary đang ở đâu, lý do nếu dừng | Zone biên dịch lỗi: bỏ qua và báo, không chặn nhóm | F5.3.5 · 3.2 |
| UC-48 | **Xem và so sánh version** (QTBM, Viewer) | Xem lịch sử version của zone (ai duyệt, khi nào), chọn hai version để so diff | | F5.4.1, F5.4.2 |
| UC-49 | **Rollback** (QTBM, DevOps) | Chọn version đã từng được duyệt → tạo version mới từ nội dung cũ → phát hành không cần duyệt lại | Version đã quá hạn giữ: báo | F5.4.3, F5.4.4 · 3.2 |
| UC-50 | **Bật Under Attack mode** (QTBM, DevOps) | Một bấm, không cần duyệt → áp ngay → audit | | F5.5.1 · 3.3 |
| UC-51 | **Rule tạm có hạn** (QTBM, SOC) | Tạo rule chặn tạm từ case hoặc SOC, có giờ hết hạn → áp ngay → tự hết hạn | Hết hạn: gỡ và báo; muốn giữ: chuyển thành rule có chủ qua UC-43 | F5.5.2 · 3.3 |
| UC-52 | **Đổi thông tin không ảnh hưởng traffic** (mọi người có quyền sửa) | Đổi tên, mô tả, tag → áp thẳng | | F5.3.1 · 3.2 |

## S3 Vận hành bảo mật của khách

| UC | Tên (ai) | Luồng chính | Ngoại lệ | Tính năng · Luồng |
|---|---|---|---|---|
| UC-60 | **Xem sự kiện chặn** (SOC, QTBM) | Chọn zone, thời gian, lọc → `QueryEvents` → danh sách và chi tiết một sự kiện (theo request ID) | Người không có quyền xem dữ liệu request: payload bị che | F7.3.3 · 3.4 |
| UC-61 | **Báo chặn nhầm** (End-user, từ gói Pro) | Trên trang chặn bấm "báo chặn nhầm" kèm request ID và mô tả → vào hộp thư của zone | Quá giới hạn tần suất hoặc nghi spam: yêu cầu challenge | F8.4.7 · 3.4 |
| UC-62 | **Xử lý báo chặn nhầm** (SOC, QTBM) | Mở báo cáo → tra sự kiện (UC-60) → quyết: bỏ qua, hoặc tạo exception qua change request (UC-43) → trả lời | Exception cho IP nằm trong CGNAT: cảnh báo trước khi gửi | F7.5.1 · 3.4 |
| UC-63 | **Tạo alert policy và ngưỡng** (QTBM) | Chọn loại cảnh báo, phạm vi zone, ngưỡng ba mức (mặc định có sẵn), giờ im lặng, chu kỳ lặp | Ngưỡng không hợp lệ: từ chối | F8.2.1, F8.2.3, F8.2.5 · 3.14 |
| UC-64 | **Kênh thông báo và gửi thử** (QTBM, Admin) | Thêm kênh (email, webhook, Slack...) → gửi thử → kết quả | Gửi lỗi: hiện lý do; credential lưu ở secret store | F8.2.2, F8.2.4, F8.2.6, F8.2.7 |
| UC-65 | **Nhận cảnh báo và mở case** (SOC) | Hệ thống nhận `AlertRaised` từ Analytics → tạo cảnh báo trong portal; SOC mở case từ cảnh báo | Kênh gửi lỗi: thử kênh khác, hiện trên portal | F8.4.1 · 3.3 |
| UC-66 | **Phân loại và xử lý case** (SOC, NCC) | Case `mở → tiếp nhận → giảm thiểu → đóng`; phân loại ảnh hưởng zone hay nền tảng; gắn sự kiện và mitigation | Không rõ phạm vi: coi là ảnh hưởng nền tảng | F8.4.2, F8.4.3, F8.4.5 · 3.3 |
| UC-67 | **Bật mitigation** (SOC, QTBM, NCC) | Bật mitigation (Under Attack, rule tạm, challenge) từ case, không cần duyệt | Vẫn quá khả năng: challenge mọi request, trần rps; **không bao giờ trả DNS về origin** | F8.4.5 · 3.3 |
| UC-68 | **Đóng case** (SOC) | Đóng kèm tóm tắt; mitigation còn giữ chuyển thành rule có chủ (UC-43) | | F8.4.4 · 3.3 |

## S4 Gói và thanh toán (mức đề cương; viết chi tiết khi tới lát cắt)

| UC | Tên (ai) | Luồng chính | Tính năng · Luồng |
|---|---|---|---|
| UC-70 | Xem gói và quyền lợi hiệu lực | Hiện gói, hạn mức, mức dùng hiện tại | F10.1, F10.3 · 3.5 |
| UC-71 | Chọn gói, tạo subscription cho account | Chọn gói theo zone hoặc gói nhóm → thanh toán → subscription `active` | F10.2 · 3.5 |
| UC-72 | Thanh toán (thẻ, QR, chuyển khoản theo hoá đơn) | Token cổng thanh toán, không lưu số thẻ | F10.6 · 3.5 |
| UC-73 | Nâng gói (hiệu lực ngay), hạ gói (cuối kỳ, ân hạn 3 ngày nếu vượt) | Tính giá trị còn lại, bonus; cảnh báo khi vượt giới hạn gói mới | F10.2, F10.5 · 3.5 |
| UC-74 | Hạn mức và trần chi tiêu | Cảnh báo ở 80%, 100%, chạm trần thì áp hành vi theo trần | F10.4, F10.5 · 3.5 |
| UC-75 | Xem hoá đơn | Hoá đơn gộp theo account | F10.6 · 3.5 |

## S5 Rời dịch vụ, S6 NCC và SSO (mức đề cương)

| UC | Tên (ai) | Luồng chính | Tính năng · Luồng |
|---|---|---|---|
| UC-80 | Xuất cấu hình và log | Tạo job xuất JSON/CSV | F1.1.6 · 3.7 |
| UC-81 | Yêu cầu xoá org, huỷ yêu cầu | Chỉ Owner; giữ 30 ngày, huỷ được | F1.1.4, F1.1.5 · 3.7 |
| UC-82 | Chuyển zone sang org khác | Owner hai org xác nhận; policy đi theo; bí mật bị xoá | F2.1.12 · 3.7 |
| UC-90 | Cấu hình SSO theo domain email, bắt buộc SSO/MFA | SAML/OIDC; tài khoản khẩn miễn SSO | F1.3.2, F1.3.3, F1.3.5 · 3.6 |
| UC-91 | Khôi phục Owner duy nhất | Hai bằng chứng trong ba, chờ 72 giờ, báo mọi admin | F1.2.7 · 3.6 |
| UC-92 | NCC quản trị: support truy cập zone khi khách cấp quyền | Thay đổi là change request đứng tên support, một người của khách duyệt | F5.3.8, F12.3.2 · 3.2 |

---

## Ghi chú về phạm vi

- **Không có trong tài liệu này:** các use case của Control, Edge, Analytics (biên dịch, phân phối, thực thi, telemetry). Management chỉ gọi tới hoặc nhận kết quả (xem [cổng ở kế hoạch](00-plan.md#3-cổng-giữa-management-và-plane-khác-stub)).
- **UC-44 (4-eyes)** là Mở rộng trong bản đồ (F5.3.2) nhưng luồng 3.2 đã duyệt coi cùng một luồng với tự xác nhận; làm cùng S2 vì dùng chung bảng `approvals` với UC-45. Nếu muốn cắt phạm vi, bỏ UC-44 và giữ UC-45.
- **Hoãn có chủ đích:** 3.8 (giám sát, điều tra, báo cáo) theo chủ dự án; 3.13 và 3.14 chưa vẽ luồng nên chỉ lấy phần đã quyết (ngưỡng cảnh báo ba mức ở UC-63).
- **Mở rộng, Nâng cao** (kế thừa policy org → nhóm → zone, 4-eyes cấp account bằng khung giờ, SCIM, Terraform, GitOps, replay): để sau MVP.
