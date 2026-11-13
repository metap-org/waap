# ADR-0047: Phân vùng mạng theo mức tin, mặc định chặn, một cổng cho edge, một đường ra Internet

- Trạng thái: **Đề xuất, chờ chọn** (2026-11-13)
- Liên quan: [ADR-0001](0001-four-planes.md), [ADR-0006](0006-saas-first-on-prem-capable.md), [ADR-0017](0017-openbao-for-signing-keys-and-node-pki.md), [ADR-0045](0045-short-lived-node-certs-and-reenroll.md), [7.6](../07-deployment/06-network-zones-and-flows.md)

## Bối cảnh

Lab bốn plane chạy trên máy ảo và cần một thiết kế mạng cô lập, an toàn, dùng được cho production sau này. Ràng buộc đã có:

- edge không nói chuyện trực tiếp với Management; kết nối đi từ edge ra (ADR-0001, ADR-0006);
- OpenBao không lộ ra mạng edge; node xin cert qua node identity service (ADR-0045);
- edge nhận traffic công khai, phải coi như có thể bị chiếm;
- webhook do khách khai là một đường SSRF nếu ứng dụng ra Internet tự do.

## Quyết định (đề xuất)

1. **Tám vùng** theo mức tin: Internet, Edge DMZ, Biên, Ứng dụng nội bộ, Dữ liệu, Secret, Vận hành, hypervisor ([7.6 §2](../07-deployment/06-network-zones-and-flows.md#2-vùng-tin-cậy)).
2. **Mặc định chặn tất cả**; mỗi luồng là một dòng trong ma trận luồng; ma trận và luật firewall được kiểm khớp nhau bằng script.
3. **Một bộ định tuyến L3 duy nhất** giữa các vùng; mỗi VM chỉ có một card mạng.
4. **Edge chỉ đi ra một cổng:** `egw-1` ở vùng Biên, chuyển tiếp mTLS vào Control và ingest; không có đường từ edge tới dữ liệu, secret, vận hành hay Management.
5. **Một đường ra Internet cho ứng dụng:** máy `egress-1` có allowlist, cũng là nơi áp SSRF guard; ngoại lệ duy nhất là `edge-l7` tới origin của khách.
6. **Quản trị chỉ qua bastion** (WireGuard, MFA, cert SSH ngắn hạn của OpenBao, ghi phiên).
7. **Ba lớp chặn** chồng nhau: firewall vùng, firewall máy chủ, NetworkPolicy. mTLS cho mọi liên kết nội bộ.

## Phương án đã cân nhắc

| Phương án | Ưu | Nhược |
|---|---|---|
| **Vùng theo mức tin (đề xuất)** | Edge bị chiếm không với tới dữ liệu; ma trận kiểm được; ánh xạ thẳng sang production | Nhiều VLAN và luật; cần bộ định tuyến riêng |
| Mạng phẳng, chỉ firewall máy chủ | Đơn giản nhất | Một máy bị chiếm thấy cả mạng; luật nằm rải rác, khó kiểm |
| Một VLAN cho mỗi plane | Khớp tên bốn plane | Plane không cùng mức tin (Control vừa nhận edge vừa nối dữ liệu); không tách được cổng edge khỏi nội bộ |
| Service mesh mọi nơi | Danh tính theo dịch vụ, chính sách theo workload | Thêm một lớp vận hành; không giải quyết cô lập mức mạng cho node ngoài K8s |

## Hệ quả

- Lab dựng được bố cục mạng giống production và thử được các tình huống tấn công nội bộ (edge bị chiếm).
- Cần thêm các máy phụ: `fw-1`, `ingress-1`, `egw-1`, `egress-1`; đã tính trong [7.6 §5](../07-deployment/06-network-zones-and-flows.md#5-bố-cục-máy-ảo-theo-vùng).
- `egw-1` thành điểm nghẽn và điểm lỗi của mọi edge: cần HA ở production; lab có bản thu gọn một máy.
- Đổi số cổng hay thêm luồng thì sửa ma trận trước, rồi luật; không sửa luật trước.
