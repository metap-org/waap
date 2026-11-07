# 3. Phân tích nghiệp vụ

> **Trạng thái (2026-11-07): 14 nghiệp vụ ở bước 1.** Đã có một số quyết định của chủ dự án (3.2–3.6); các câu
> còn lại của 3.2–3.6 có đề xuất của Claude chờ duyệt. Chưa nghiệp vụ nào có luồng được duyệt.

Bản đồ phân rã nói hệ thống **có** những chức năng gì. Mục này đi qua từng nghiệp vụ theo trình tự khách
thật làm. Khi xong, mỗi nghiệp vụ trả lời được các câu hỏi sau:

- Bước nào phải làm trước bước nào, và vì sao.
- Khách phải chuẩn bị gì trước khi bắt đầu.
- Điều kiện nào được kiểm ở mỗi cổng.
- Bước nào hay hỏng, và hỏng thì xử lý thế nào.
- Mỗi bước sinh ra dữ liệu gì.

Thứ tự phân tích đi theo vòng đời của khách: onboarding trước, vì đó là lần đầu sản phẩm chạm vào traffic
thật và cũng là lúc dễ gây sự cố nhất.

Mỗi nghiệp vụ đi qua hai bước, và **không viết bước 2 khi chủ dự án chưa trả lời bước 1**:

1. **Concern**: kịch bản theo từng trục (mã `KB<số trang>-<trục><số>`) và câu hỏi cần chủ dự án quyết
   (`OQ-…`). Ý kiến lúc ghi để ở cột "ghi chú ban đầu", không phải quyết định. Quyết định của chủ dự án ghi ở
   mục "Đã quyết" của từng trang; câu chủ dự án giao tự phân tích có mục "Phân tích các câu còn mở" với đề
   xuất chờ duyệt.
2. **Luồng**: sau khi có trả lời; khi đó cập nhật [§1](../01-functional-decomposition.md),
   [§2](../02-requirements.md) và kiến trúc.

| Mục | Nghiệp vụ | Câu hỏi | Trạng thái |
|---|---|---|---|
| [3.1](01-onboarding.md) | Onboarding một website | OQ-21…OQ-32, OQ-147…OQ-150 | Bước 1 mở rộng (trục K–N), có hồ sơ khách, cây quyết định kết nối và phân tích mọi câu, đề xuất chờ duyệt (bản nháp luồng cũ vẫn ở phụ lục) |
| [3.2](02-policy-change.md) | Viết, duyệt, phát hành một thay đổi policy | OQ-33…OQ-40 | Đã quyết OQ-35; các câu còn lại có đề xuất chờ duyệt |
| [3.3](03-attack-response.md) | Phản ứng khi bị tấn công | OQ-41…OQ-47 | Đã quyết OQ-41 (NCC bật được mitigation); các câu còn lại có đề xuất chờ duyệt |
| [3.4](04-false-positive.md) | Xử lý chặn nhầm | OQ-48…OQ-53 | Đã quyết cách xử lý và OQ-49; các câu còn lại có đề xuất chờ duyệt |
| [3.5](05-plans-billing.md) | Mua, đổi gói, vượt hạn mức, thanh toán | OQ-54…OQ-61 | Đã quyết một phần OQ-55 (có trần chi tiêu); trừ phần vượt quota để sau; các câu còn lại có đề xuất chờ duyệt |
| [3.6](06-members-access.md) | Quản lý thành viên, SSO, API key | OQ-62…OQ-68 | Đã quyết OQ-62 (một người nhiều org); các câu còn lại có đề xuất chờ duyệt |
| [3.7](07-offboarding.md) | Rời dịch vụ | OQ-69…OQ-74, OQ-94…OQ-99 | Bước 1, đã thêm kịch bản F–M |
| [3.8](08-monitoring-investigation.md) | Giám sát, điều tra, cảnh báo, báo cáo | OQ-75…OQ-80 | **Tạm hoãn** theo chủ dự án |
| [3.9](09-bot-management.md) | Bot management và challenge | OQ-81…OQ-83, OQ-85, OQ-100…OQ-106, OQ-124…OQ-126 | Bước 1 mở rộng (trục J–L), có mô hình quyết định và phân tích mọi câu, đề xuất chờ duyệt |
| [3.10](10-cache-performance.md) | Cache, hiệu năng, nội dung (gồm chính sách abuse) | OQ-86…OQ-88, OQ-112…OQ-117, OQ-143…OQ-146 | Bước 1 mở rộng (trục J–N), có mô hình quyết định và phân tích mọi câu, đề xuất chờ duyệt |
| [3.11](11-provider-operations.md) | Vận hành của NCC | OQ-89…OQ-93, OQ-118…OQ-123 | Bước 1, đã thêm trục F–N |
| [3.12](12-api-security.md) | API security | OQ-84, OQ-107…OQ-111 | Bước 1, tách từ trang bot cũ |
| [3.13](13-configuration-surface.md) | Bề mặt cấu hình: rule và setting trên portal, cấu hình nâng cao cho admin | OQ-151…OQ-162 | Đã quyết nhóm tính năng theo plane (trang lỗi, emergency override, Trust/Skip, audit export, rollout, dry-run, failover, tarpit, IP thật sau CDN, xử lý một bấm, live tail, chống bão, lọc export); danh mục và các câu còn lại ở bước 1 |
| [3.14](14-config-tiers-attack-history-thresholds.md) | Cấu hình nhiều tầng, lịch sử tấn công, ngưỡng cảnh báo | OQ-163…OQ-165 | Đã quyết hướng (ba tầng, hai loại đợt tấn công, ngưỡng ba mức); luồng chưa vẽ |

## Câu hỏi nối giữa các nghiệp vụ

Một số câu hỏi xuất hiện ở nhiều trang; trả lời một lần cho tất cả:

| Chủ đề | Câu hỏi |
|---|---|
| Một khách nhỏ nhiều domain, agency, reseller, MSSP | OQ-27, OQ-54, OQ-59, ~~OQ-62~~ (đã quyết: một người nhiều org), OQ-63, OQ-75 |
| Ai được xem dữ liệu request (dữ liệu cá nhân) | OQ-50, OQ-68, OQ-84, OQ-104 |
| NCC làm thay khách (bật mitigation, sửa policy, khôi phục quyền) | OQ-40, ~~OQ-41~~ (đã quyết: NCC bật được mitigation), OQ-64, OQ-118, OQ-119 |
| Pháp lý: cơ quan chức năng, lưu log, abuse | OQ-45, OQ-73, OQ-79, OQ-88, OQ-98 |
| Dữ liệu cá nhân trong audit và khi xoá | OQ-6, OQ-70, OQ-71 |
