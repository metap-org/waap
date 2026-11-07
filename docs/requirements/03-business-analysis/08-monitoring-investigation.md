# 3.8 Giám sát, điều tra, cảnh báo, báo cáo

> **Trạng thái: tạm hoãn theo chủ dự án (2026-11-07), "giám sát thì tính sau".** Kịch bản và câu hỏi dưới đây giữ
> nguyên để quay lại sau, không phân tích thêm lúc này. **Ngoại lệ:** lịch sử tấn công (hai loại đợt, màn event WAF)
> và ngưỡng cảnh báo đã được mở lại ở [3.14](14-config-tiers-attack-history-thresholds.md).

## 1. Phạm vi

Công việc thường ngày sau onboarding: xem tình hình, điều tra một sự việc, nhận cảnh báo, đẩy log sang hệ
thống của khách, làm báo cáo cho lãnh đạo và kiểm toán. Liên quan: F7, F8, F9 ở
[§1](../01-functional-decomposition.md).

## 2. Kịch bản theo từng trục

### A. Ai dùng

| Mã | Kịch bản | Concern |
|---|---|---|
| KB8-A1 | Chủ shop xem mỗi tuần một lần | Cần tóm tắt dễ hiểu, không phải bảng log |
| KB8-A2 | SOC trực 24/7 của doanh nghiệp | Màn hình realtime, điều tra nhanh, phím tắt |
| KB8-A3 | Đơn vị giám sát an ninh thuê ngoài (MSSP) giám sát nhiều khách | Một màn hình cho nhiều org |
| KB8-A4 | Lãnh đạo xem báo cáo tháng | Chứng minh giá trị: đã chặn gì, tiết kiệm gì |
| KB8-A5 | Kiểm toán viên | Bằng chứng tuân thủ |

### B. Xem và điều tra

| Mã | Kịch bản | Concern |
|---|---|---|
| KB8-B1 | Từ một cảnh báo đi tới event, request, rule, version | Mấy bấm |
| KB8-B2 | Tìm theo IP, user agent, path, quốc gia, ASN | Truy vấn nhanh trên khoảng dài |
| KB8-B3 | Điều tra vượt retention của gói | Archive có khôi phục được không, tính phí |
| KB8-B4 | So sánh trước và sau một thay đổi policy | |
| KB8-B5 | Xem traffic theo thời gian thực khi đang có sự cố | Nhiều người cùng xem (QS-38) |

### C. Cảnh báo

| Mã | Kịch bản | Concern |
|---|---|---|
| KB8-C1 | Cảnh báo bật sẵn khi tạo zone | Bộ nào; quá nhiều cảnh báo thì khách tắt hết |
| KB8-C2 | Cảnh báo về chính dịch vụ: cert sắp hết hạn, origin lỗi, usage gần hạn mức | Khác cảnh báo bảo mật |
| KB8-C3 | Kênh theo thị trường Việt Nam (Zalo, Telegram, SMS) | Ngoài email, Slack, Teams |
| KB8-C4 | Gộp cảnh báo, giờ im lặng, người trực | Lịch trực có tích hợp không |

### D. Đẩy log ra ngoài

| Mã | Kịch bản | Concern |
|---|---|---|
| KB8-D1 | SIEM của khách (Splunk, Elastic, QRadar, SIEM trong nước) | Định dạng, giao thức |
| KB8-D2 | Đích hỏng nhiều ngày | Giữ bao lâu, báo ai |
| KB8-D3 | Chi phí băng thông của việc đẩy log | Ai chịu |

### E. Báo cáo và tuân thủ

| Mã | Kịch bản | Concern |
|---|---|---|
| KB8-E1 | Báo cáo tháng cho lãnh đạo | Mẫu báo cáo |
| KB8-E2 | Bằng chứng PCI DSS, SOC 2, ISO 27001 | Mục nào của tiêu chuẩn |
| KB8-E3 | Quy định Việt Nam về lưu trữ dữ liệu và log (Luật An ninh mạng, Nghị định 53/2022) | Có áp cho log của khách không, lưu ở đâu |
| KB8-E4 | Báo cáo SLA của NCC | Theo gói |

## 3. Câu hỏi cần chủ dự án quyết

| ID | Câu hỏi | Kịch bản | Ghi chú ban đầu |
|---|---|---|---|
| OQ-75 | Có hỗ trợ MSSP giám sát nhiều khách từ một màn hình không? | KB8-A3 | Liên quan OQ-62, OQ-63 |
| OQ-76 | Bộ cảnh báo bật sẵn gồm những gì? | KB8-C1, KB8-C2 | |
| OQ-77 | Kênh Zalo, Telegram, SMS có ở MVP không? | KB8-C3 | |
| OQ-78 | Truy vấn ngoài retention: khôi phục từ archive có làm không, tính phí không? | KB8-B3 | |
| OQ-79 | Nghĩa vụ lưu log theo quy định Việt Nam áp cho phần nào? | KB8-E3 | Cần tư vấn pháp lý |
| OQ-80 | Báo cáo tuân thủ nhắm tiêu chuẩn nào trước? | KB8-E2 | |

## 4. Phụ thuộc

FR-7-01…FR-7-09, FR-8-01…FR-8-07, FR-9-01…FR-9-06, IR-6, IR-7 ở [§2](../02-requirements.md).
