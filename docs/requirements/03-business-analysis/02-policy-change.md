# 3.2 Viết, duyệt, phát hành một thay đổi policy

> **Trạng thái (2026-11-17): chủ dự án đã duyệt các đề xuất của Claude** (OQ-33, 34, 36…40); ngoại lệ ghi ở mục phân tích bên dưới. Các luồng (bước 2) chưa viết. Trang này liệt kê kịch bản và câu hỏi; quyết định của
> chủ dự án ghi ở [§3](#3-đã-quyết). Cột "ghi chú ban đầu" là ý kiến lúc ghi, không phải quyết định.

## 1. Phạm vi

Bắt đầu khi ai đó muốn đổi cách edge xử lý traffic của một hoặc nhiều zone. Kết thúc khi thay đổi đã chạy
trên edge và được theo dõi, hoặc đã bị huỷ hay rollback. Không gồm: kênh khẩn khi đang bị tấn công (3.3),
exception sinh từ chặn nhầm (3.4), cập nhật ruleset do NCC phát hành (chỉ ghi phần ảnh hưởng tới khách, ở
trục I).

Liên quan: F5 ở [§1](../01-functional-decomposition.md#f5-vòng-đời-policy), FR-5-xx ở
[§2](../02-requirements.md#f5-vòng-đời-policy).

## 2. Kịch bản theo từng trục

### A. Ai thay đổi, qua kênh nào

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-A1 | Quản trị bảo mật sửa trên portal | Kênh chính |
| KB2-A2 | DevOps sửa qua public API hoặc Terraform | Cùng quyền, cùng kiểm tra như UI; drift giữa Terraform và UI |
| KB2-A3 | Policy đồng bộ từ Git (GitOps) | Git là nguồn sự thật thì sửa trên UI bị khoá hay bị ghi đè ở lần đồng bộ sau |
| KB2-A4 | Support của NCC sửa thay khách | Cần quyền khách cấp, audit hiện cho khách, có cần khách duyệt không |
| KB2-A5 | Developer chỉ được sửa trong phạm vi được giao | Phạm vi là zone, path hay loại rule |
| KB2-A6 | Org chỉ có một người | 4-eyes không áp được |

### B. Loại thay đổi

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-B1 | Custom rule, rate limit, access list | Ảnh hưởng trực tiếp traffic |
| KB2-B2 | Cấu hình managed ruleset: paranoia, ngưỡng anomaly, tắt rule | Một con số đổi có thể đổi kết quả hàng nghìn rule |
| KB2-B3 | Exception | Mở rộng quá tay là lỗ hổng; hẹp quá thì không hết chặn nhầm |
| KB2-B4 | Cài đặt zone (TLS, fail mode, origin, cache rule) | Có đi qua cùng change request với rule không; đổi origin sai làm sập website |
| KB2-B5 | Cấu hình bot, API schema | Ảnh hưởng client không chạy được JavaScript (app mobile, đối tác) |
| KB2-B6 | Chỉ đổi tên, mô tả, thứ tự hiển thị | Có cần duyệt không |
| KB2-B7 | Chuyển một rule từ log-only sang chặn | Là thay đổi rủi ro nhất trong thực tế |

### C. Phạm vi

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-C1 | Một zone | Mặc định |
| KB2-C2 | Nhóm zone theo tag | Một change request cho nhiều zone; một zone lỗi biên dịch thì cả nhóm dừng hay phát phần còn lại |
| KB2-C3 | Template ở cấp org, kế thừa xuống zone | Sửa template ảnh hưởng hàng nghìn zone; zone có quyền ghi đè không |
| KB2-C4 | Thay đổi hàng loạt 100.000 zone (Terraform, đổi mặc định) | QS-27; duyệt một lần hay từng zone |

### D. Kiểm tra trước khi phát hành

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-D1 | Kiểm cú pháp và biên dịch thử | Lỗi phải chỉ đúng vị trí trong rule |
| KB2-D2 | Mô phỏng trên traffic lịch sử | Chỉ gói có mẫu request; zone ít traffic không đủ mẫu; mẫu chứa dữ liệu cá nhân |
| KB2-D3 | Thay đổi vượt giới hạn gói (số rule, độ phức tạp) | Báo khi soạn hay khi phát hành |
| KB2-D4 | Rule đúng cú pháp nhưng chặn tất cả (ví dụ điều kiện luôn đúng) | Có cảnh báo "rule này khớp X% traffic" trước khi phát hành không |

### E. Duyệt

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-E1 | 4-eyes bật, người duyệt khác người tạo | Người duyệt vắng mặt, thay đổi kẹt |
| KB2-E2 | Duyệt theo mức rủi ro | Đổi mô tả tự duyệt; chuyển log-only sang chặn cần duyệt |
| KB2-E3 | Khung giờ cho phép thay đổi | Thay đổi khẩn ngoài giờ |
| KB2-E4 | Người duyệt chỉ có quyền trên một phần zone trong change request | Duyệt một phần hay từ chối cả request |
| KB2-E5 | Change request để lâu, bản nháp đã đổi tiếp | Duyệt bản nào; hết hạn duyệt |

### F. Phát hành và theo dõi

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-F1 | Phát hành đi qua canary theo cell | Khách thấy gì trong lúc canary; mất bao lâu |
| KB2-F2 | Gate canary của NCC tự rollback thay đổi của khách | Khách được báo thế nào, lý do có hiểu được không |
| KB2-F3 | Một phần node áp lỗi | Zone chạy hai version cùng lúc |
| KB2-F4 | Sau khi phát hành, tỉ lệ chặn tăng vọt | Có tự cảnh báo khách trong 15 phút đầu không |
| KB2-F5 | Khách muốn phát hành theo tỉ lệ traffic (1% → 10% → 100%) | Có làm không, ở gói nào |

### G. Xung đột

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-G1 | Hai người sửa cùng bản nháp | Khoá, gộp, hay báo xung đột version |
| KB2-G2 | Terraform apply trong lúc có change request đang chờ duyệt trên UI | Cái nào thắng |
| KB2-G3 | Rule tạm của kênh khẩn đang chạy khi có thay đổi thường | Phát hành thường có xoá rule tạm không |

### H. Rollback

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-H1 | Rollback về version trước | Có cần duyệt không |
| KB2-H2 | Rollback về version dùng tính năng đã hết sau khi hạ gói | Từ chối, hay rollback một phần |
| KB2-H3 | Rollback về version tham chiếu ruleset release đã ngừng | Dùng release nào |
| KB2-H4 | Rollback chỉ một rule, không rollback cả version | Có cần không |

### I. Thay đổi do NCC phát hành

| Mã | Kịch bản | Concern |
|---|---|---|
| KB2-I1 | Bản CRS hoặc system ruleset mới đổi hành vi zone dù khách không đổi gì | Tự áp, hay khách chọn khi nào nhận; có giai đoạn log-only cho rule mới không |
| KB2-I2 | Khách Enterprise muốn ghim version ruleset | Ghim được bao lâu; bản vá bảo mật có bắt buộc không |
| KB2-I3 | Rule khẩn của NCC cho CVE mới | Có hiện trong lịch sử thay đổi của zone không |

## 3. Đã quyết

| Ngày | Câu hỏi | Quyết định | Hệ quả cần phân tích tiếp |
|---|---|---|---|
| 2026-11-07 | Ai phải duyệt | **Cùng một luồng, mỗi người theo quyền thấy luồng khác nhau.** Khách **self-service** tự làm chủ, thao tác được mà **không cần ai duyệt**. Khách **doanh nghiệp** và **admin của NCC** có phân chia quyền và trách nhiệm rõ ràng (người gửi khác người duyệt) | Đã có cơ chế ở `metap-go` ([Phase 104 của `metap-docs`](https://github.com/metap-org/metap-docs/blob/master/docs/roadmap/104-workflow-per-caller-flows-and-separation-of-duties.md)): chế độ duyệt theo account đọc qua `$ctx.approvalRequired`, quyền theo từng bước qua policy `transition:<action>`, người duyệt khác người gửi qua `$workflow.actors.submit`; capabilities tự hiện đúng nút cho từng người, không cần entity thứ hai. Còn phải quyết: ai bật chế độ doanh nghiệp cho một account (mặc định theo gói, hay khách tự bật), và OQ-34 (org một người) coi như self-service |
| 2026-11-06 | OQ-35 | **UI là nguồn sự thật.** Public API, Terraform, GitOps vẫn có; khi xung đột, bản trên UI thắng. Terraform và GitOps chỉ báo drift, không ghi đè thay đổi đã làm trên UI | Drift được phát hiện lúc nào (lúc `plan`, lúc đồng bộ định kỳ) và báo cho ai; GitOps thành công cụ đề xuất thay đổi (tạo change request) thay vì áp thẳng; Terraform `apply` gặp bản UI mới hơn thì lỗi hay bỏ qua (KB2-A2, KB2-A3, KB2-G2) |

## 4. Câu hỏi cần chủ dự án quyết

| ID | Câu hỏi | Kịch bản | Ghi chú ban đầu |
|---|---|---|---|
| OQ-33 | Mọi thay đổi đều đi qua change request, hay chỉ thay đổi ảnh hưởng traffic? Cài đặt zone (origin, TLS) có đi cùng đường với rule không? | KB2-B4, KB2-B6 | |
| OQ-34 | Org một người: 4-eyes tắt hẳn, hay thay bằng bước tự xác nhận và thời gian chờ? | KB2-A6 | |
| ~~OQ-35~~ | Khi dùng GitOps hoặc Terraform: UI bị khoá chỉ đọc, hay cho sửa và báo drift? | KB2-A2, KB2-A3, KB2-G2 | **Đã quyết**, xem §3 |
| OQ-36 | Cập nhật ruleset của NCC: tự áp cho mọi zone, hay khách chọn thời điểm; rule mới có chạy log-only trước không? | KB2-I1, KB2-I2 | |
| OQ-37 | Có duyệt theo mức rủi ro (thay đổi nhỏ tự duyệt) không? | KB2-E2 | |
| OQ-38 | Thay đổi template ảnh hưởng nhiều zone: duyệt một lần, phát hành theo đợt? | KB2-C2, KB2-C3 | |
| OQ-39 | Khách có thấy và điều khiển được tốc độ phát hành (canary, phần trăm traffic) không? | KB2-F1, KB2-F5 | |
| OQ-40 | Support của NCC có được sửa policy thay khách không, có cần khách duyệt không? | KB2-A4 | |

## 5. Phân tích các câu còn mở (đã duyệt 2026-11-17)

> **Chủ dự án duyệt 2026-11-17:** mọi đề xuất ở bảng dưới được chốt thành quyết định


Chủ dự án giao tự phân tích các câu còn lại (2026-11-07). Mỗi đề xuất có phương án và đánh đổi; **chưa câu nào
được coi là quyết định** cho tới khi chủ dự án duyệt.

| Câu | Phương án | Đánh đổi | Đề xuất |
|---|---|---|---|
| OQ-33 | (a) Mọi thay đổi qua change request; (b) chỉ thay đổi ảnh hưởng traffic | (a) chậm cho việc đổi tên, mô tả; (b) phải xác định ranh giới | (b): mọi thay đổi ảnh hưởng traffic qua change request, **gồm cả cài đặt zone** (origin, TLS, fail mode, cache rule) vì đổi origin sai cũng làm sập website. Đổi tên, mô tả, tag hiển thị không cần change request, vẫn ghi audit. Purge là lệnh, không phải cấu hình, không qua change request |
| OQ-34 | (a) Tắt 4-eyes; (b) tự xác nhận diff; (c) thời gian chờ | (c) làm chậm đúng lúc cần sửa nhanh | 4-eyes chỉ bật được khi account có ít nhất hai người có quyền duyệt. Org một người vẫn tạo change request (để có diff, kiểm tra, rollback) và tự phát hành sau bước xác nhận diff; không có thời gian chờ |
| OQ-36 | (a) Tự áp mọi zone; (b) khách chọn thời điểm; (c) tự áp theo giai đoạn | (a) đổi hành vi không báo trước; (b) zone lâu không cập nhật thì kém an toàn | (c): rule mới của release vào zone ở log-only 7 ngày, có báo cáo, rồi tự chặn. Business trở lên chọn được "duyệt thủ công": release thành change request chờ duyệt. Enterprise ghim version tối đa 90 ngày. Rule khẩn cho CVE luôn áp ngay ở chế độ chặn; khách tắt được bằng tuning rule (BR-25) |
| OQ-37 | (a) Có duyệt theo mức rủi ro; (b) không | (a) khó giải thích với kiểm toán, ranh giới rủi ro dễ bị lách | (b) ở MVP: khi bật 4-eyes thì mọi change request đều cần duyệt. Ngoại lệ duy nhất: rollback về một version đã từng được duyệt không cần duyệt lại, vì đó là bước đưa về trạng thái đã được chấp nhận |
| OQ-38 | (a) Duyệt từng zone; (b) duyệt một lần, phát hành theo đợt | (a) không làm được với nghìn zone | (b): một change request cho template, duyệt một lần; phát hành theo đợt (vài zone trước, rồi phần còn lại); zone biên dịch lỗi bị bỏ qua và báo, không chặn cả nhóm |
| OQ-39 | (a) Khách chỉ xem trạng thái; (b) khách điều khiển tốc độ | (b) thêm độ phức tạp cho rollout của Control | (a) ở MVP: khách thấy canary đang ở đâu, bao nhiêu % node đã áp. Phát hành theo phần trăm traffic (KB2-F5) để mức Nâng cao |
| OQ-40 | (a) Support không sửa; (b) support sửa, khách duyệt; (c) support sửa thẳng | (c) khách mất kiểm soát | (b): chỉ khi khách đã cấp quyền support (BR-20); thay đổi do support tạo là change request đứng tên support, cần một người của khách duyệt. Riêng mitigation khi bị tấn công thì NCC bật được không cần duyệt (đã quyết ở 3.3) |

## 6. Phụ thuộc

FR-5-01…FR-5-19, BR-4, BR-5, BR-6, BR-18, BR-21 ở [§2](../02-requirements.md) viết trước phân tích này;
sẽ đối chiếu lại khi các câu hỏi trên có trả lời.
