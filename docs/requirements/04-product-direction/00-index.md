# 4. Định hướng sản phẩm

> **Trạng thái: đánh giá và đề xuất của Claude, chờ chủ dự án duyệt (2026-11-07).** Viết theo yêu cầu "tự đặt
> câu hỏi và tự trả lời". Không có gì ở đây là quyết định cho tới khi chủ dự án duyệt (các câu đã quyết ghi ở mục "Câu hỏi cần chủ dự án quyết"). Nhận định về thị trường
> và đối thủ dựa trên hiểu biết chung tới giữa 2026, cần kiểm lại trước khi dùng cho tài liệu bán hàng.

Mục này nhìn sản phẩm từ trên xuống, không theo từng nghiệp vụ như [§3](../03-business-analysis/00-index.md).
Câu hỏi chung của cả mục: **sản phẩm đang đi đúng hướng của một WAAP hiện đại chưa, và nên đi tiếp thế nào.**

| Mục | Câu hỏi chính | Kết luận ngắn |
|---|---|---|
| [4.1](01-modern-waap-fit.md) | Sản phẩm đang định hướng gì, đã đủ hiện đại chưa? | Lõi WAF, DDoS, edge, vòng đời policy, kiến trúc 4 plane đã ngang các sản phẩm hiện đại. Thiếu: chưa có câu trả lời cho kỷ nguyên AI (bảo vệ ứng dụng LLM, tác tử AI), bot và API đến muộn, chưa có lớp tự động hoá vận hành bảo mật |
| [4.2](02-ai-ml.md) | AI/ML gắn vào đâu? | Ở 6 vị trí, mỗi vị trí có ngân sách độ trễ khác nhau. Nguyên tắc: ML đề xuất và chấm điểm, luật xác định quyết định; mô hình đi qua cùng pipeline phát hành như ruleset; LLM không bao giờ nằm trên đường request |
| [4.3](03-autonomous-soc.md) | SOC tự hành thế nào để ít phụ thuộc con người? | Chia 5 mức tự hành. Tự động hoàn toàn chỉ với hành động **tạm thời, có hạn, tự gỡ khi có dấu hiệu sai**; thay đổi lâu dài do người quyết (xác nhận 2026-11-28) |
| [4.4](04-data-strategy.md) | Dữ liệu request dùng được vào việc gì ngoài log và cảnh báo, làm sao không lộ dữ liệu nhạy cảm? | Dữ liệu chia 5 tầng; dữ liệu đi lên tầng trên chỉ qua bước khử nhận dạng. Dùng cho bảo vệ chung, huấn luyện mô hình, báo cáo ngành, nghiên cứu. **Không bán dữ liệu kinh doanh của một khách**, vì niềm tin là tài sản lớn hơn dữ liệu |

## Câu hỏi cần chủ dự án quyết

Các câu cụ thể nằm ở cuối từng trang (OQ-127…OQ-142). Câu lớn nhất:

**Trả lời của chủ dự án (2026-11-28):** OQ-127 **không**, để backlog. OQ-135: SOC (AI cộng người, **người ra
quyết định**) được đổi toàn bộ policy; hệ thống không tự quyết thay đổi lâu dài ([4.3](03-autonomous-soc.md)). OQ-138 **tuỳ loại dữ liệu**, chưa quyết; bảng rà ở [4.4](04-data-strategy.md). Riêng IP nguồn: **được** dùng cho bảo vệ chung, có điều kiện.

1. Có đưa **bảo vệ ứng dụng AI** (LLM, tác tử) vào lộ trình không, ở mức nào? (OQ-127)
2. Mức tự hành tối đa của SOC: có cho hệ thống tự đổi policy lâu dài với khách bật "tự động hoàn toàn" không? (OQ-135)
3. Dữ liệu chéo tenant dùng tới đâu: chỉ bảo vệ và huấn luyện, hay cả báo cáo ngành công khai, nghiên cứu xuất bản? (OQ-138)
