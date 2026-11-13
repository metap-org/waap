# Hạ tầng lab (P1, lát 1: mạng, `fw-1`, khung máy ảo)

Đề xuất của Claude, chờ duyệt. Thiết kế ở [7.6](../../docs/architecture/07-deployment/06-network-zones-and-flows.md), ADR-0047 đến 0050. Kế hoạch ở [../PLAN.md](../PLAN.md).

## Đã có

| Phần | Vai trò |
|---|---|
| `zones.yaml`, `profiles/compact.yaml` | Vùng mạng (dùng chung mọi profile) và danh sách máy theo RAM |
| `flows.yaml` | Ma trận luồng N01 đến N29 và cấm tuyệt đối X01 đến X06 dưới dạng dữ liệu |
| `tools/gen_nft.py` | Sinh luật nftables của `fw-1` từ ma trận. Lệnh `check` là T07 hai chiều: mọi dòng 7.6 có luật, mọi luật có dòng 7.6, và không luồng nào vi phạm cấm tuyệt đối |
| `tofu/`, `cloud-init/` | OpenTofu (provider `lxc/incus`): một cầu ảo mỗi vùng (máy chủ không có IP trên cầu), `fw-1` nhiều card mạng, mỗi VM khác một card, IP tĩnh qua cloud-init |
| `../bin/lab` | `check`, `gen`, `ssh-ca`, `up`, `provision`, `seal`, `audit`, `down`, `destroy`, `status` |
| `../ansible/` | Bộ cài và hardening, xem [README](../ansible/README.md) |

## Đã kiểm chứng và chưa

- Đã chạy: `python3 tools/test_gen_nft.py` (5 test), `gen_nft.py check compact` (T07 đạt), `nft -c` trên luật sinh ra (cú pháp hợp lệ).
- **Chưa chạy được:** `tofu validate/apply` và dựng VM thật, vì môi trường này không có Incus, OpenTofu và quyền ảo hoá. Mã OpenTofu và cloud-init chưa được chạy lần nào, kỳ vọng sẽ cần sửa khi chạy lần đầu trên máy lab.

## Cách dùng (trên máy lab)

```
export LAB_SSH_KEY="$(cat ~/.ssh/id_ed25519.pub)"
labs/bin/lab up      # fw-1 lên trước, rồi các máy còn lại; chế độ BOOTSTRAP cho cài gói
labs/bin/lab seal    # gỡ đường ra Internet tạm, fw-1 chỉ còn luật trong ma trận
labs/bin/lab down    # snapshot và tắt theo thứ tự ngược
```

## Điều cần biết

- **Luồng cùng vùng (N10, N11, N20, N23, N28c) không đi qua `fw-1`** vì cùng một cầu L2; `gen_nft.py` đánh dấu `enforce: host` và bỏ qua. Chặn bằng firewall máy chủ ở P2/P4 (ADR-0048).
- **Chế độ BOOTSTRAP** cho mọi vùng ra Internet để apt cài gói; nó là luật tạm ghi rõ trong ruleset và gỡ bằng `lab seal`. Chưa có `egress-1`/`registry` thật nên chưa thể cài gói qua đường đúng ADR.
- Pod K8s được giả định ra ngoài với IP của node (Cilium/Calico masquerade); nếu không thì luật theo IP node không đủ và cần NetworkPolicy ở P2.

## Mâu thuẫn trong 7.6 do bộ sinh luật tìm ra (cần chủ dự án chọn)

1. **N27 ("mọi VM" dùng NTP/DNS ở `infra` Z6) đụng X01, X05, X02.** Edge (Z1) không được tới Z6, OpenBao (Z5) không được khởi tạo kết nối ra, Z0 không được tới Z6. Tạm loại Z0, Z1, Z5 khỏi N27. Hai hướng: (a) đặt một nguồn NTP/DNS riêng ở Z1 (`dns-1` kiêm chrony) và ở Z5 cho phép riêng một ngoại lệ tới `infra`; (b) sửa X01/X05 thành "trừ NTP/DNS tới `infra`". Edge cần giờ chuẩn vì cert node sống 24 giờ, nên không thể bỏ.
2. **N24 (WireGuard từ Z0 vào `bastion` Z6) đụng X02 ("Z0 không tới Z6").** Tạm coi N24 là ngoại lệ duy nhất (`except: [N24]` trong `flows.yaml`); 7.6 nên ghi rõ.
3. **Vùng không có đường ra không vá được (phát hiện khi viết bộ cài).** Theo ma trận chỉ Z3 (N21) và `ci-runner` (N29) tới được `egress-1`; Z1, Z2, Z4, Z5 thì không, nên `unattended-upgrades` ở đó không lấy được bản vá sau khi gỡ bootstrap. Hướng đề xuất: bản sao apt nội bộ ở `registry` (Z6) cộng luồng mới "Z1, Z2, Z4, Z5 tới `registry` 443/TCP" (X01 cấm Z1 tới Z6, nên Z1 cần ngoại lệ hoặc nguồn khác, cùng loại vấn đề với N27). Chưa thêm vào ma trận vì cần chủ dự án chọn.
