# ADR-0049: GitOps và DevSecOps cho từng plane, chuỗi cung ứng có ký và kiểm chính sách

- Trạng thái: **Đề xuất, chờ chọn** (2026-11-13)
- Liên quan: [ADR-0001](0001-four-planes.md), [ADR-0023](0023-kubernetes-for-central-planes-edge-outside.md), [ADR-0024](0024-licensing-policy-for-shipped-components.md), [ADR-0037](0037-config-apply-classes.md), [7.4](../07-deployment/00-index.md#74-môi-trường), [8.15](../08-cross-cutting/15-config-propagation-and-apply.md)

## Bối cảnh

Bốn plane có bốn chu kỳ triển khai riêng; một thay đổi ở portal không được làm edge deploy lại. Chủ dự án yêu cầu GitOps và DevSecOps trong từng plane. Hiện tại CI mới
có build, vet, test, fmt, `govulncheck`, CodeQL, Semgrep và hai guard `depguard` (edge và analytics chỉ import `metap/pkg/kit/*`). Chưa có quét secret, SBOM, quét và ký image,
kiểm chính sách, hay đường triển khai.

## Quyết định (đề xuất)

1. **Git là nguồn sự thật.** Mã nằm ở repo của từng plane; **cấu hình triển khai tách ra một repo riêng** (`waap-deploy`) với Helm và Kustomize, một thư mục cho mỗi
   môi trường (`dev`, `staging`, `canary`, `prod`). Thay đổi môi trường là sửa file trong repo này, không phải nhánh riêng cho từng môi trường.
2. **Argo CD kéo** (Apache-2.0), mẫu app-of-apps, mỗi plane một ứng dụng; tự hoàn lại khi cụm lệch Git (`selfHeal`) và cảnh báo.
3. **Thăng cấp bằng pull request** đổi digest image trong thư mục môi trường: staging tự động sau khi pipeline qua; canary và prod cần người duyệt (cùng nguyên tắc 4-eyes của BR-26).
4. **Edge (ngoài K8s) không đi qua Argo:** pipeline phát hành artifact có ký (binary, Vector) tới registry và relay; việc rollout lên node theo cơ chế canary PoP và gate của chính nền tảng
   (7.4, 8.15 lớp D). Git chỉ ghi *phiên bản mong muốn* của edge; một job đăng ký phiên bản đó vào fleet registry.
5. **Mỗi plane một pipeline.** Chặng bắt buộc theo bảng dưới. Không có đường tắt triển khai không qua pipeline.
6. **Chuỗi cung ứng:** image tham chiếu theo digest; base image tối giản, chạy không root; SBOM cho mọi image; ký bằng cosign với khoá ký nằm trong OpenBao transit (ADR-0050); cụm chỉ nhận image đã ký
   (Kyverno `verifyImages`).
7. **Không có secret trong Git.** Quét bằng gitleaks ở CI và pre-commit; bootstrap duy nhất (khoá unseal) giữ ngoại tuyến.
8. **Công cụ chọn theo license** (ADR-0024): OpenTofu (MPL-2.0) thay Terraform (BSL); Argo CD, Kyverno, cosign, Syft, Trivy (Apache-2.0); gitleaks (MIT).

### Chặng pipeline

| Chặng | Công cụ | Hiện có | Ghi chú |
|---|---|---|---|
| Lint, kiểu, fmt | `golangci-lint`, `gofmt` | Một phần | Thêm lint cho IaC (`tflint`) và manifest |
| Unit test | `go test` | Có | |
| Test tích hợp | `go test -tags integration` (testkit) | Một phần | Chạy trong job riêng có Postgres |
| SAST | Semgrep, CodeQL | Có | |
| SCA | `govulncheck` | Có | Chặn lỗ hổng mức cao |
| Quét secret | gitleaks | Chưa | Chặn merge |
| Kiểm license | `go-licenses` | Chưa | Theo ADR-0024; chặn BSL, SSPL, AGPL |
| Ranh giới plane | `depguard` | Có (edge, analytics) | Giữ nguyên |
| Build container | Dockerfile nhiều tầng, distroless | Có Dockerfile | Không root, chỉ đọc hệ thống tệp |
| SBOM | Syft | Chưa | Đính kèm image |
| Quét image | Trivy | Chưa | Chặn lỗ hổng mức cao |
| Ký | cosign, khoá trong OpenBao | Chưa | |
| Kiểm chính sách | Kyverno (admission), `conftest` (trước merge) | Chưa | Ví dụ: không chạy root, có giới hạn tài nguyên, NetworkPolicy bắt buộc |
| Triển khai | Argo CD | Chưa | |

## Phương án đã cân nhắc

| Phương án | Nhận xét |
|---|---|
| **Argo CD, repo triển khai riêng (đề xuất)** | Một nguồn sự thật cho cụm, tách chu kỳ bốn plane; có giao diện xem trôi |
| Flux | Nhẹ và gần K8s hơn, không có giao diện; chọn lại được, hai công cụ tương đương cho nhu cầu này |
| Đẩy từ CI vào cụm | Đơn giản nhưng CI giữ quyền vào cụm, không tự hoàn lại khi bị sửa tay |
| Nhánh riêng cho từng môi trường | Dễ lệch và khó so sánh; thư mục theo môi trường gọn hơn |
| Terraform, Packer | Bị loại: BSL (ADR-0024 áp cho thành phần phát hành; chọn bản tuân thủ cho đồng nhất) |

## Hệ quả

- Runner CI ở vùng Vận hành, ra Internet qua `egress-1` (luồng N29, [7.6](../07-deployment/06-network-zones-and-flows.md)); không có đường nào từ Internet vào runner.
- Có hai loại "triển khai": K8s qua Argo, edge qua fleet registry. Hai đường phải cho ra cùng bản ghi "phiên bản nào đang chạy ở đâu", nếu không audit sẽ thiếu.
- Repo triển khai riêng thêm một nơi để duy trì, đổi lại bốn plane không kéo nhau.
- Quét và ký làm pipeline dài hơn; chặng nặng (quét image, SBOM) chỉ chạy khi image đổi.
