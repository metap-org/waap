# 9. Architecture Decisions

Mỗi quyết định một file, định dạng: trạng thái, ngày, bối cảnh, quyết định, hệ quả, phương án đã
cân nhắc. Quyết định bị thay thế giữ lại với trạng thái "Bị thay thế bởi ADR-xxxx".

| ADR | Quyết định | Trạng thái |
|---|---|---|
| [0001](0001-four-planes.md) | Bốn plane, edge độc lập runtime | Chấp nhận |
| [0002](0002-central-compile-signed-snapshots.md) | Compile tập trung, snapshot bất biến có chữ ký, rollout theo đợt | Chấp nhận |
| [0003](0003-rule-language-existing-syntax-with-builder.md) | Ngôn ngữ rule dùng cú pháp có sẵn, builder ↔ text qua AST | Chấp nhận |
| [0004](0004-managed-ruleset-crs-on-seclang.md) | Managed ruleset: OWASP CRS trên engine SecLang + system ruleset riêng | Chấp nhận |
| [0005](0005-edge-on-own-and-existing-infra.md) | Edge chạy trên PoP riêng và hạ tầng có sẵn | Chấp nhận |
| [0006](0006-saas-first-on-prem-capable.md) | SaaS trước, giữ đường hybrid và on-prem | Chấp nhận |
| [0007](0007-log-by-subscription-entitlements.md) | Log và giới hạn theo subscription qua entitlements | Chấp nhận |
| [0008](0008-under-attack-mode.md) | Under Attack mode theo zone | Chấp nhận |
| [0009](0009-metap-for-management-plane-only.md) | `metap` chỉ cho Management plane | Chấp nhận |
| [0010](0010-analytics-one-write-path-many-stores.md) | Analytics: một luồng ghi, nhiều store | Chấp nhận |
| [0011](0011-approximate-fleet-rate-limiting.md) | Rate limit toàn fleet là xấp xỉ | Chấp nhận |
| [0012](0012-go-as-primary-language.md) | Go là ngôn ngữ chính cho mọi plane | Bị thay thế bởi 0030 |
| [0013](0013-edge-gateway-custom-go-with-coraza.md) | Edge gateway tự xây bằng Go, nhúng Coraza + CRS v4 | Thay bằng thiết kế hai tầng; Go + Coraza nằm trong tổ hợp S3 của 0031 |
| [0014](0014-cel-as-rule-language.md) | CEL là ngôn ngữ custom rule | Chấp nhận, sửa bởi 0032 |
| [0015](0015-snapshot-format-and-distribution.md) | Snapshot protobuf + zstd ký Ed25519; gRPC desired state + HTTPS pull qua relay | Chấp nhận, sửa bởi 0040 |
| [0016](0016-postgres-and-s3-compatible-storage.md) | PostgreSQL cho trạng thái điều khiển, object storage chuẩn S3 (SeaweedFS khi tự host) | Chấp nhận |
| [0017](0017-openbao-for-signing-keys-and-node-pki.md) | OpenBao cho khoá ký, mã hoá key cert, CA của node | Chấp nhận |
| [0018](0018-kafka-as-telemetry-bus.md) | Apache Kafka làm ingestion bus | Chấp nhận |
| [0019](0019-vector-as-node-log-shipper.md) | Vector làm log shipper trên node | Chấp nhận |
| [0020](0020-clickhouse-and-parquet-for-analytics.md) | ClickHouse cho hot search và số liệu gộp, Parquet cho archive | Chấp nhận |
| [0021](0021-valkey-for-hot-key-rate-limit-sync.md) | Valkey theo region cho counter hot key | Chấp nhận |
| [0022](0022-rabbitmq-for-control-events-kafka-for-telemetry.md) | RabbitMQ cho sự kiện điều khiển, Kafka cho telemetry | Chấp nhận |
| [0023](0023-kubernetes-for-central-planes-edge-outside.md) | Kubernetes cho plane trung tâm, edge ngoài Kubernetes | Chấp nhận |
| [0024](0024-licensing-policy-for-shipped-components.md) | Không BSL/SSPL/AGPL trong thành phần phát hành | Chấp nhận |
| [0025](0025-geo-asn-data-commercial-license.md) | Dữ liệu geo/ASN dùng giấy phép thương mại | Chấp nhận |
| [0026](0026-replay-on-request-samples-with-shared-pipeline.md) | Replay trên mẫu request đầy đủ, dùng chung pipeline | Chấp nhận |
| [0027](0027-explicit-alert-formulas.md) | Cảnh báo theo template có công thức tường minh, baseline median/MAD theo phút trong tuần | Chấp nhận |
| [0028](0028-notification-adapters-and-plan-gated-channels.md) | Notification: adapter theo kênh, webhook CloudEvents ký HMAC, email luôn có, kênh khác theo gói | Chấp nhận |
| [0029](0029-independent-platform-observability.md) | Giám sát nền tảng độc lập với Analytics, SLO + burn rate, probe theo hành vi | Chấp nhận |
| [0030](0030-language-per-workload.md) | Ngôn ngữ theo đặc tính tải: Go cho Control/Analytics/Management, edge theo 0031; Elixir cho realtime gateway, notifier (0044) | Chấp nhận (phần edge chờ 0031) |
| [0031](0031-edge-l7-and-waf-engine-options.md) | Nền tầng L7 × engine tầng WAF × cách gọi; shortlist K2 / K3 / K1, spike để chọn | **Đề xuất, chờ chọn** |
| [0032](0032-cel-checked-at-control-ir-at-edge.md) | CEL kiểm tra ở Control, edge thực thi IR | Đề xuất, theo 0031 |
| [0033](0033-two-tier-edge-proxy-and-waf.md) | Edge hai tầng: L7 (RAM) và WAF inspection (CPU) | Chấp nhận |
| [0034](0034-pre-filter-and-rule-split-across-tiers.md) | Pre-filter và challenge ở L7; custom rule chia tầng giữ first-match | Chấp nhận |
| [0035](0035-cdn-and-dns-strategy.md) | Chiến lược CDN và DNS: tự xây theo giai đoạn + chế độ đứng sau CDN khách | **Đề xuất, chờ chọn** |
| [0036](0036-request-timing-and-error-attribution.md) | Đo thời gian theo chặng; mỗi response có `status_source`, mã `edge_error`; status chuẩn + header | Chấp nhận |
| [0037](0037-config-apply-classes.md) | Phân lớp cấu hình A/B/C/D theo cách áp dụng; fast lane chỉ lớp A; rule khẩn là lớp riêng | Chấp nhận |
| [0038](0038-bot-management-layered-scoring.md) | Bot management chấm điểm nhiều lớp ở L7; challenge tự phát; dữ liệu bot phát hành như ruleset | Chấp nhận |
| [0039](0039-api-security-positive-model.md) | API security: discovery, schema chia L7/WAF, JWT/mTLS ở edge, danh tính consumer | Chấp nhận |
| [0040](0040-config-distribution-at-scale.md) | Phân phối cấu hình ở 1 triệu zone: manifest theo bucket, routing index, PoP store, nạp khi cần, ACK theo version tổng | Chấp nhận |
| [0041](0041-cells-and-zone-placement.md) | Cell và cách đặt zone: so sánh 4 phương án, đề xuất cell theo VIP | **Đề xuất, chờ chọn** |
| [0042](0042-plan-tiers-drive-placement-and-isolation.md) | Gói quyết định pool cell, tài nguyên, thứ tự hy sinh khi quá tải, thứ tự rollout | Chấp nhận (giá chờ business) |
| [0043](0043-usage-based-pricing-and-metering.md) | Giá theo phí nền + hạn mức + phí vượt; đo usage không lấy mẫu; trần cứng ở edge; chưa mở Free khi ra mắt | Chấp nhận (giá chờ business) |
| [0044](0044-realtime-gateway-and-notifier-runtime.md) | Realtime gateway cho hàng chục nghìn phiên portal và runtime của notifier: Go, Centrifugo, Elixir/Phoenix, managed; chọn Elixir + Phoenix | Chấp nhận |
| [0045](0045-short-lived-node-certs-and-reenroll.md) | Cert node 24 h tự gia hạn qua node identity service, re-enroll credential cho node offline, thu hồi bằng thời hạn ngắn + danh sách chặn, cert-manager cho dịch vụ, Origin CA cho edge → origin | Chấp nhận |
| [0046](0046-management-database-tenancy-and-storage.md) | Database Management: tenant = org, bảng chung + database riêng cho Enterprise, PolicyVersion là tài liệu bất biến, registry toàn cục, bảng partition | Chấp nhận |
| [0047](0047-network-zones-and-default-deny.md) | Phân vùng mạng theo mức tin, mặc định chặn, một cổng cho edge, một đường ra Internet; lab trên máy ảo | **Đề xuất, chờ chọn** |
| [0048](0048-node-baseline-and-hardening-profiles.md) | Chuẩn node chung: Ubuntu 24.04 LTS, role Ansible `baseline` cộng 7 hồ sơ theo vai trò, hardening CIS kiểm được | **Đề xuất, chờ chọn** |
| [0049](0049-gitops-and-devsecops-per-plane.md) | GitOps (Argo CD, repo triển khai riêng) và DevSecOps từng plane: quét secret, license, SBOM, quét và ký image, kiểm chính sách; OpenTofu thay Terraform | **Đề xuất, chờ chọn** |
| [0050](0050-secret-management-and-rotation.md) | OpenBao là secret manager duy nhất, secret động ngắn hạn, bảng xoay theo 12 loại, unseal Shamir ở lab | **Đề xuất, chờ chọn** |

Quyết định đang chờ: xem [11. Risks and Technical Debt](../11-risks/00-index.md#113-quyết-định-còn-mở).
