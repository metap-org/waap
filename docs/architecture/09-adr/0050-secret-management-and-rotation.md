# ADR-0050: Secret manager duy nhất, cấp secret ngắn hạn, xoay tự động theo loại

- Trạng thái: **Đề xuất, chờ chọn** (2026-11-13)
- Liên quan: [ADR-0017](0017-openbao-for-signing-keys-and-node-pki.md), [ADR-0045](0045-short-lived-node-certs-and-reenroll.md), [ADR-0047](0047-network-zones-and-default-deny.md), [ADR-0048](0048-node-baseline-and-hardening-profiles.md), [8.19](../08-cross-cutting/19-node-identity-and-mtls.md)

## Bối cảnh

Chủ dự án yêu cầu có secret manager và tự động xoay secret theo nghiệp vụ và hạ tầng. OpenBao đã được chọn cho khoá ký và PKI (ADR-0017). Hiện còn nhiều secret nằm ngoài nó:
khoá ký JWKS đặt trong thư mục khoá gắn vào container, secret ingest giữa control-plane và analytics-plane, mật khẩu cơ sở dữ liệu tĩnh trong cấu hình compose. Xoay JWKS đã làm được không cần restart,
nhưng bằng tay.

## Quyết định (đề xuất)

1. **OpenBao là secret manager duy nhất** cho cả bốn plane và hạ tầng. Chạy ở vùng Secret ngoài K8s, ba node Raft, không có đường ra ([ADR-0047](0047-network-zones-and-default-deny.md)).
2. **Ưu tiên secret động, ngắn hạn** hơn secret tĩnh: mật khẩu cơ sở dữ liệu cấp theo yêu cầu với TTL, thu hồi khi hết hạn; cert 24 giờ cho node, 7 ngày cho dịch vụ (ADR-0045).
3. **Xác thực vào OpenBao theo loại chủ thể:** pod bằng K8s auth; VM bằng cert (TLS auth); CI bằng AppRole với TTL ngắn và secret ID bọc (response wrapping); người bằng OIDC cộng MFA, chỉ cho quản trị.
   **Edge không nói chuyện với OpenBao** (ADR-0045).
4. **Giao secret không qua biến môi trường dài hạn và không qua Git:** pod nhận qua Secrets Store CSI hoặc agent sidecar của OpenBao ghi vào bộ nhớ (tmpfs), không tạo K8s Secret để khỏi lưu trong etcd; VM dùng OpenBao Agent chạy như dịch vụ.
5. **Mọi secret có chủ, chu kỳ và cách nạp lại** trong bảng xoay dưới; dịch vụ phải nạp lại không cần khởi động lại.
6. **Mở khoá (unseal):** lab dùng Shamir 5 phần, cần 3, giữ riêng ngoại tuyến bởi các người khác nhau; root token thu hồi sau bootstrap. Production tự mở khoá bằng transit của một OpenBao thứ hai hoặc KMS.
7. **Audit bật từ đầu**, gửi về `obs`; sao lưu snapshot Raft mã hoá lên object storage theo lịch.

### Bảng xoay theo loại

| Loại | Nơi giữ | Chu kỳ đề xuất | Cách xoay | Cách nạp lại |
|---|---|---|---|---|
| Khoá ký JWKS | OpenBao KV; agent ghi thư mục khoá của issuer | Theo lịch (ví dụ quý) và khi lộ | Quy trình 3 bước thành job tự động: thêm khoá, đợi mọi verifier làm mới, chuyển `signing-kid`, đợi token cũ hết hạn, gỡ khoá cũ | Issuer đọc lại thư mục khoá; verifier lấy lại khi gặp `kid` lạ (đã có) |
| Cert node (edge) | OpenBao PKI qua node identity service | 24 giờ | Tự gia hạn ở 2/3 thời hạn | Agent trên node tải lại (ADR-0045) |
| Cert dịch vụ | OpenBao PKI qua cert-manager | 7 ngày | Tự gia hạn | Dịch vụ tải lại cert khi đổi file |
| Mật khẩu Postgres | OpenBao database engine | Cấp theo yêu cầu, TTL một giờ, tối đa 24 giờ | Cấp tài khoản tạm, thu hồi khi hết hạn | Connection pool mở kết nối mới bằng tài khoản mới |
| Tài khoản RabbitMQ, Kafka, ClickHouse | OpenBao (động nếu engine hỗ trợ, nếu không thì tĩnh có xoay) | Một đến ba mươi ngày | Xoay theo lịch | Client đọc lại trước khi hết hạn |
| Secret ingest control-plane tới analytics-plane | OpenBao KV | Ba mươi ngày | Hai giá trị song song trong thời gian chuyển | Hai phía đọc lại |
| Khoá ký manifest snapshot | OpenBao transit | Theo lịch, phiên bản khoá | Thêm phiên bản khoá; edge tin nhiều khoá song song | Edge cập nhật danh sách tin cậy qua bundle |
| Cert Origin CA theo zone | OpenBao PKI | 90 ngày | Tự gia hạn (ADR-0045) | Edge nhận qua bundle |
| Secret của khách (webhook, kênh thông báo) | `SecretStore` theo tenant, backend OpenBao | Do khách chọn | Qua API, ghi audit | Đọc lại mỗi lần gửi |
| SSH | OpenBao SSH CA | Cert vài giờ | Không có khoá dài hạn | — |
| Secret CI (AppRole) | OpenBao | Mỗi lần chạy | Secret ID dùng một lần | — |
| Khoá unseal, root token | Ngoại tuyến | Khi có thay đổi người giữ | Rekey theo quy trình | — |

## Phương án đã cân nhắc

| Phương án | Nhận xét |
|---|---|
| **OpenBao cho mọi thứ, secret động (đề xuất)** | Một nơi quản lý và audit; secret ngắn hạn giảm thiệt hại khi lộ; cần OpenBao luôn sẵn sàng |
| External Secrets tạo K8s Secret | Dễ dùng nhưng secret nằm trong etcd; cần mã hoá etcd và kiểm soát RBAC chặt |
| SOPS trong Git | Không cần dịch vụ, nhưng không xoay động, khoá giải mã thành điểm yếu |
| KMS của cloud | Không dùng được on-prem; ADR-0006 yêu cầu plane không phụ thuộc một cloud |

## Hệ quả

- OpenBao ngoài K8s nên cụm có thể khởi động mà không cần OpenBao, nhưng dịch vụ cần secret thì chờ; thứ tự khởi động phải ghi trong runbook: OpenBao, rồi dữ liệu, rồi ứng dụng.
- Vài dịch vụ phải đổi để nạp lại secret động mà không khởi động lại; hiện chưa dịch vụ nào của management-plane làm vậy cho mật khẩu cơ sở dữ liệu. Đây là công việc code, ngoài phạm vi tài liệu.
- Khoá JWKS hiện là tệp khoá thô nên không đi qua transit được; ký trực tiếp bằng transit là hướng sau, đổi mã của issuer.
- Cần diễn tập: lộ khoá thì thu hồi và xoay trong một thao tác; mất đa số người giữ khoá unseal thì khôi phục thế nào.
