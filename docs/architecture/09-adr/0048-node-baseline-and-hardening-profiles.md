# ADR-0048: Chuẩn node chung, hồ sơ theo vai trò, hardening kiểm được

- Trạng thái: **Đề xuất, chờ chọn** (2026-11-13)
- Liên quan: [ADR-0023](0023-kubernetes-for-central-planes-edge-outside.md), [ADR-0024](0024-licensing-policy-for-shipped-components.md), [ADR-0047](0047-network-zones-and-default-deny.md), [7.6](../07-deployment/06-network-zones-and-flows.md)

## Bối cảnh

Lab và production có nhiều loại máy: node K8s, edge (bare metal hoặc VM), kho dữ liệu, OpenBao, bastion. Chủ dự án yêu cầu các service chạy bare metal hay không đều cần
bộ cài riêng và chuẩn chung: mạng, hệ điều hành, phần mềm, phân quyền, chia đĩa, hardening đầy đủ. Edge là một binary cộng Vector chạy bằng systemd, nên không phụ thuộc hệ điều hành
cụ thể (ADR-0023).

## Quyết định (đề xuất)

1. **Một hệ điều hành cho mọi node ở lab:** Ubuntu 24.04 LTS. Lý do: có benchmark CIS và nội dung OpenSCAP sẵn, vá bảo mật dài hạn. Artifact edge là binary tĩnh nên không buộc khách dùng cùng bản.
2. **Một chuẩn nền (`baseline`) áp cho mọi node**, cộng một **hồ sơ** theo vai trò. Chuẩn nền là một role Ansible duy nhất; hồ sơ là role chồng lên.
3. **Cấp phát:** image cloud gốc cộng cloud-init (tên máy, CA của SSH, khoá bootstrap) cộng Ansible chạy từ Git. Chưa dùng golden image ở MVP (Packer là BSL, xem ADR-0049).
4. **Không cấu hình tay trên máy.** Máy lệch chuẩn thì dựng lại, không vá.
5. **Kiểm được:** mỗi hồ sơ có quét CIS (OpenSCAP) và kiểm trôi (Ansible check mode) chạy định kỳ; kết quả là cổng của giai đoạn P4 ([kế hoạch lab](../../../labs/PLAN.md)).

### Chuẩn nền (mọi node)

| Hạng mục | Quy định |
|---|---|
| Truy cập | SSH chỉ bằng cert ngắn hạn do OpenBao ký, không mật khẩu, không đăng nhập root, chỉ từ `bastion` |
| Phân quyền | Mỗi dịch vụ một user hệ thống không đăng nhập được; `sudo` chỉ cho nhóm vận hành, có ghi lại; không tài khoản dùng chung |
| Phân vùng đĩa | `/boot`, `/`, `/var`, `/var/log`, `/var/log/audit`, `/tmp` (`nodev,nosuid,noexec`), `/home` (`nodev,nosuid`); đĩa dữ liệu riêng theo hồ sơ |
| Dịch vụ | Chỉ gói cần thiết; không trình biên dịch, không công cụ mạng thừa; dịch vụ không dùng bị tắt |
| Kernel và mạng | Bộ `sysctl` chuẩn (chống giả mạo, chặn chuyển tiếp IP trừ router), tắt IPv6 ở lab, tắt module không dùng, tắt USB và FireWire |
| Kiểm soát truy cập bắt buộc | AppArmor ở chế độ enforce |
| Nhật ký và kiểm toán | `auditd` với bộ luật chuẩn; log hệ điều hành gửi về `obs` qua Vector; giờ đồng bộ bằng `chrony` tới `infra` |
| Toàn vẹn | AIDE cho thư mục hệ thống; cảnh báo khi đổi ngoài Ansible |
| Cập nhật | Vá bảo mật tự động, khởi động lại trong khung giờ; node trung tâm vá theo đợt để không mất đồng thời |
| Dịch vụ của chúng ta | Unit systemd: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, tập capability tối thiểu, `MemoryMax` |
| Tường lửa | nftables mặc định chặn, chỉ mở theo ma trận [7.6 §4](../07-deployment/06-network-zones-and-flows.md#4-ma-trận-luồng) |
| Khởi động | Secure Boot và TPM khi hạ tầng có; không có thì ghi lại là rủi ro (ADR-0045) |

### Hồ sơ theo vai trò

| Hồ sơ | Dùng cho | Khác biệt |
|---|---|---|
| `k8s-node` | Control-plane, worker K8s | Kubelet đã khoá, đọc-only trên port kubelet, tắt anonymous auth; cấu hình kernel cho container |
| `edge-l7` | Pool L7 (bare metal hoặc VM) | Chỉ capability `NET_BIND_SERVICE`; tinh chỉnh mạng (RSS, IRQ affinity, `somaxconn`); đĩa NVMe cache gắn `noatime`; có đường ra tới origin của khách (ngoại lệ duy nhất, ADR-0047) |
| `edge-waf` | Pool WAF | CPU và cgroup riêng; **không có đường ra Internet**; không nhận kết nối từ ngoài vùng |
| `data` | Postgres, ClickHouse, Kafka, RabbitMQ | Đĩa riêng cho dữ liệu, WAL, log; bộ lập lịch I/O phù hợp; mã hoá đĩa (LUKS); tài khoản backup riêng |
| `secret` | OpenBao | Không swap; mã hoá đĩa; tắt core dump; khoá bộ nhớ; đĩa audit riêng; không có đường ra |
| `bastion` | Cửa quản trị | Ghi phiên (session recording); MFA; không lưu khoá riêng của ai; không có quyền đọc dữ liệu khách |
| `ops` | CI, registry, giám sát, infra | Runner CI không giữ secret dài hạn; đường ra qua `egress-1` |

## Phương án đã cân nhắc

| Phương án | Nhận xét |
|---|---|
| **Image gốc cộng Ansible (đề xuất)** | Đơn giản, đủ cho lab, kiểm trôi được; chưa có image dựng sẵn |
| Golden image bằng Packer | Dựng nhanh hơn, nhưng Packer chuyển sang BSL; có thể dùng sau cho nội bộ |
| OS bất biến cho node K8s (Talos, Flatcar) | Hardening mạnh và rẻ; đổi cách vận hành và không dùng cho edge; xem xét lại khi chạy production |
| Mỗi vai trò một OS, một chuẩn riêng | Chi phí duy trì nhân lên; không có chuẩn chung như chủ dự án yêu cầu |

## Hệ quả

- Một nơi (role `baseline`) quyết định chuẩn cho mọi node; sửa ở đó là sửa cho cả fleet, qua pull request.
- Ngưỡng điểm CIS còn mở (L5 trong kế hoạch lab); chưa có số liệu để chọn con số.
- Hardening có thể ảnh hưởng hiệu năng edge; áp từng hạng mục và đo trước sau, không áp đồng loạt.
- Công cụ GPL (Ansible, Lynis) chỉ dùng nội bộ NCC, không phát hành tới khách (ADR-0024).
