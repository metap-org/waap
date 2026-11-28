# 4.1 Sản phẩm đang định hướng gì, đã phù hợp WAAP hiện đại chưa

> **Trạng thái: đánh giá của Claude, chờ duyệt (2026-11-07).**

## Câu 1. Sản phẩm này đang định hướng gì?

Theo tài liệu hiện có ([kiến trúc §1](../../architecture/01-introduction/00-index.md), 8.18, các ADR):

| Trục | Định hướng hiện tại |
|---|---|
| Khách | Doanh nghiệp trong nước là đích chính; 80% zone là người dùng nhỏ kiểu Cloudflare, 10–20% là doanh nghiệp tạo phần lớn doanh thu |
| Hình thức | SaaS trước, giữ đường cho hybrid và on-prem (ADR-0006); chạy trên hạ tầng tự có và hạ tầng đối tác (ADR-0005) |
| Phạm vi | WAAP đủ bốn mảng (WAF, DDoS L7, bot, API) cộng CDN và DNS điều hướng (ADR-0035): khách đến vì hiệu năng, ở lại vì bảo vệ |
| Kiến trúc | 4 plane tách hẳn (ADR-0001): edge chạy độc lập khi mọi thứ khác sập; edge hai tầng L7 và WAF (ADR-0033); thiết kế cho 1 triệu zone (ADR-0040, ADR-0041) |
| Thương mại | Phí nền + hạn mức + phí vượt (ADR-0043), giá VND, gói theo phân khúc (8.18) |
| Khác biệt dự kiến | Dữ liệu ở trong nước, tiếng Việt, giá VND và hoá đơn điện tử, chạy được on-prem cho khách bị ràng buộc pháp lý, hỗ trợ gần |

**Nhận xét.** Định hướng này hợp lý cho một nhà cung cấp trong nước: không cạnh tranh trực diện với các hãng toàn
cầu về độ phủ mạng, mà cạnh tranh ở chỗ họ yếu: dữ liệu trong nước, on-prem, hỗ trợ tại chỗ, giá và hoá đơn theo
quy định trong nước. Điểm yếu tự nhiên là **lượng dữ liệu nhỏ hơn** để học (bot, reputation), nên chiến lược dữ
liệu ở [4.4](04-data-strategy.md) quan trọng hơn với sản phẩm này so với một hãng toàn cầu.

## Câu 2. Một WAAP hiện đại (2026) phải có gì?

Bốn mảng lõi theo định nghĩa phổ biến của thị trường là WAF, chống DDoS tầng ứng dụng, quản lý bot, bảo vệ API.
Tới 2026, khách doanh nghiệp còn kỳ vọng thêm:

| # | Năng lực | Vì sao khách kỳ vọng |
|---|---|---|
| M1 | Bảo vệ API theo kho API (discovery, schema, posture), không chỉ rule | Phần lớn traffic là API |
| M2 | Bot management dựa trên mô hình, ít captcha | Captcha hình ảnh đã bị AI giải; người dùng ghét captcha |
| M3 | Chống chiếm tài khoản (ATO), kiểm mật khẩu đã lộ | Tấn công phổ biến và gây thiệt hại trực tiếp |
| M4 | Phân biệt và quản lý crawler AI, tác tử AI | Lượng traffic từ AI tăng nhanh; khách muốn kiểm soát và có thể thu tiền |
| M5 | Bảo vệ ứng dụng dùng LLM (prompt injection, rò dữ liệu qua câu trả lời, lạm dụng chi phí token) | Khách đang đưa chatbot, tác tử vào sản phẩm |
| M6 | Bảo vệ phía trình duyệt (script bên thứ ba, PCI DSS 4.0 mục 6.4.3 và 11.6.1) | Yêu cầu tuân thủ cho trang thanh toán |
| M7 | Virtual patching nhanh khi có CVE | Khách không vá kịp |
| M8 | Tự động hoá vận hành: tự giảm thiểu, tự đề xuất tinh chỉnh, trợ lý bằng ngôn ngữ tự nhiên | Thiếu nhân lực SOC |
| M9 | Policy as code, tích hợp CI/CD | DevOps quản lý bằng code |
| M10 | Kiểm soát dữ liệu: residency, che dữ liệu nhạy cảm, xuất log | Pháp lý |

## Câu 3. Sản phẩm hiện tại đứng ở đâu so với danh sách đó?

| # | Năng lực | Trong tài liệu hiện tại | Đánh giá |
|---|---|---|---|
| M1 | API theo kho API | MR (8.17) | Đủ thiết kế, nhưng đến muộn so với kỳ vọng |
| M2 | Bot dựa trên mô hình | Luật trọng số ở MR, mô hình ở NC (8.16) | **Muộn**; nên kéo mô hình bot lên sớm hơn, chấp nhận mô hình đơn giản |
| M3 | ATO | NC (8.16 §9) | Muộn; bài toán mất tiền trực tiếp ở khách trong nước (đề xuất OQ-100, OQ-106) |
| M4 | Crawler AI, tác tử AI | Crawler AI có trong 8.16; tác tử chưa có | Thiếu một nửa (OQ-125) |
| M5 | Bảo vệ ứng dụng LLM | **Chưa có** | Khoảng trống lớn nhất (OQ-127) |
| M6 | Bảo vệ phía trình duyệt | NC | Hợp lý; nên ở MR cho khách thương mại điện tử cần PCI |
| M7 | Virtual patching | MR (F11.4) | Ổn |
| M8 | Tự động hoá vận hành | Có detection, đề xuất case (F8.1.3); chưa có khung tự hành | **Thiếu khung** ([4.3](03-autonomous-soc.md)) |
| M9 | Policy as code | MR (F5.6) | Ổn; UI là nguồn sự thật (OQ-35) |
| M10 | Kiểm soát dữ liệu | Che tại edge, residency ở NC (8.6) | Ổn cho MVP |

**Kết luận.** Phần nền (edge, phân phối cấu hình, vòng đời policy, cô lập tenant, đo lường) tốt và hiện đại; đó
là phần khó và tốn kém nhất, đã được thiết kế kỹ. Phần còn yếu nằm ở tầng "trí tuệ" phía trên: mô hình, tự động
hoá, và các mối đe doạ của kỷ nguyên AI. Tin tốt là kiến trúc hiện tại đã có chỗ cắm sẵn cho phần này (mô hình
trong bundle chung, pipeline phát hành mô hình, Analytics có detection), nên bổ sung không phải đập đi làm lại.

## Câu 4. Nên kéo cái gì lên sớm hơn?

| Ưu tiên | Đề xuất | Lý do |
|---|---|---|
| 1 | Bảo vệ đăng nhập và OTP (ATO cơ bản, SMS pumping) lên MR, bán add-on cho Pro | Mất tiền trực tiếp, dễ chứng minh giá trị |
| 2 | Mô hình bot đơn giản ngay từ MR, huấn luyện trên dữ liệu chéo tenant | Captcha không còn hiệu quả; dữ liệu chéo tenant là lợi thế của nền tảng |
| 3 | Khung tự hành SOC mức TH3 cho hành động tạm thời ở MVP ([4.3](03-autonomous-soc.md)): tự giảm thiểu tạm thời có hạn, gom cảnh báo thành sự cố | Đội nhỏ, không trực 24/7 được (OQ-122) |
| 4 | Trợ lý ngôn ngữ tự nhiên trên portal ở MR: hỏi bằng tiếng Việt, ra truy vấn có cấu trúc | Khách nhỏ không rành bảo mật; khác biệt rõ khi demo |
| 5 | Bảo vệ ứng dụng LLM ở MR, bắt đầu bằng giới hạn và quan sát (không phải chặn nội dung) | Thị trường mới, ít đối thủ trong nước |
| 6 | Bảo vệ phía trình duyệt lên MR cho gói Business | PCI DSS 4.0 |

## Câu 5. Rủi ro của định hướng này là gì?

| Rủi ro | Hệ quả | Giảm thiểu |
|---|---|---|
| Ít dữ liệu hơn hãng toàn cầu | Mô hình bot, reputation kém hơn | Học chéo tenant ngay từ đầu, mua feed threat intel, hợp tác CERT |
| Làm quá nhiều mảng cùng lúc | Không mảng nào đủ tốt | Giữ thứ tự: lõi chắc trước, AI sau; mỗi mảng có mức MVP rõ |
| Tự động hoá sai làm sập website khách | Mất niềm tin nhanh hơn bị tấn công | Chỉ tự động với hành động tạm thời, có hạn, tự gỡ ([4.3](03-autonomous-soc.md)) |
| Dùng dữ liệu khách sai mục đích | Mất khách, vi phạm Nghị định 13 | Điều khoản rõ, khử nhận dạng, không bán dữ liệu kinh doanh ([4.4](04-data-strategy.md)) |

## Câu hỏi cần chủ dự án quyết

| ID | Câu hỏi | Đề xuất |
|---|---|---|
| OQ-127 | Có đưa bảo vệ ứng dụng LLM và tác tử AI vào lộ trình không, ở mức nào? | MR, bắt đầu bằng quan sát và giới hạn chi phí |
| OQ-128 | Có kéo mô hình bot và bảo vệ đăng nhập/OTP lên MR không? | Có |
| OQ-129 | Bảo vệ phía trình duyệt (PCI DSS 4.0) có lên MR cho Business không? | Có |
| OQ-130 | Thứ tự ưu tiên ở Câu 4 có đúng với kế hoạch bán hàng không? | Cần chủ dự án và sales xác nhận |

> **Quyết định của chủ dự án (2026-11-28), OQ-127: không đưa vào lộ trình.** Bảo vệ ứng dụng LLM và tác tử AI
> nằm ở backlog, chỉ xét lại sau khi làm xong hết phần còn lại. Đề xuất ở trên (làm ở MR) không áp dụng. Việc
> dùng LLM bên trong sản phẩm (trợ lý, soạn nháp rule, [4.2](02-ai-ml.md)) là câu khác, quyết định này không đụng tới.

