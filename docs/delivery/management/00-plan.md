# Kế hoạch thực thi Management plane

> **Trạng thái: bản đề xuất chờ duyệt (2026-11-18).** Chưa có dòng mã nào của Management được viết theo kế hoạch này.
> Bước kế tiếp sau khi duyệt: [use case](01-use-cases.md) rồi [thiết kế database](02-database.md) (cả hai đã viết, cùng PR).

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
