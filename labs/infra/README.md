# Hạ tầng lab (P1, lát 1: mạng, `fw-1`, khung máy ảo)

Đề xuất của Claude, chờ duyệt. Thiết kế ở [7.6](../../docs/architecture/07-deployment/06-network-zones-and-flows.md), ADR-0047 đến 0050. Kế hoạch ở [../PLAN.md](../PLAN.md).

## Profile full (bản nháp chờ review)

`profiles/full.yaml`: 33 máy, khoảng 95 GB RAM cho VM (cần khoảng 103 GB tính cả máy chủ 128 GB). Đã có: danh sách máy và IP, luật mạng riêng cho full (etcd HA 2379 và 2380 chỉ giữa các server, WireGuard udp 51820 giữa mọi node; compact không có hai luật này), cấu hình RKE2 cho server thứ hai trở đi gia nhập cụm và bật Canal WireGuard, bảng tuning, sơ đồ ở 7.6 §5.2, [bản đồ thành phần](../PLACEMENT.full.md). Test sinh luật, `check` và gộp tuning chạy cho cả hai profile.

**Chưa có cho full:** `steps.full.yaml` (checklist từng bước; `steps.yaml` viết cho tên máy của compact, `lab step` báo rõ khi chạy với full), bộ cài sao chép PostgreSQL (nên chỉ cài node đầu, `pg-2` mới hardening), Kafka, SeaweedFS, OpenBao (P5), ingress-1, egw-1, registry, infra, obs. Mã hoá lưu lượng pod bằng WireGuard dựa trên tài liệu RKE2 ([Network Options](https://docs.rke2.io/networking/basic_network_options)) nhưng chưa chạy thật. Dùng: `LAB_PROFILE=full labs/bin/lab ...`.

## Chạy từng bước (đọc phần này trước)

Bản đồ thành phần nào cài ở máy nào, vùng nào, ai được vào: [../PLACEMENT.md](../PLACEMENT.md) (compact), [../PLACEMENT.full.md](../PLACEMENT.full.md) (full).

Tham số đã rà theo best practice, kèm cơ sở và mức tin cậy: [../PARAMETERS.md](../PARAMETERS.md).

Lab **không** dựng một lần. Có 42 bước nhỏ chia 10 phase, mỗi bước có lệnh riêng, bảng "cấu hình đúng phải là bao nhiêu" và lệnh kiểm tự động: xem [../CHECKLIST.md](../CHECKLIST.md) (sinh tự động, không sửa tay).

```
labs/bin/lab step list             # tất cả bước và trạng thái
labs/bin/lab step next             # bước kế tiếp
labs/bin/lab step show P5.1        # xem lệnh và checklist, không chạy gì
labs/bin/lab step run P5.1         # chạy (hỏi xác nhận; từ chối nếu bước phụ thuộc chưa kiểm đạt)
labs/bin/lab step verify P5.1      # chạy từng lệnh kiểm, in PASS/FAIL, thực tế so với mong đợi
```

- Các phase: P0 máy chủ lab, P1 mạng ảo và `fw-1`, P2 boot VM từng vùng, P3 hardening OS từng lớp, P4 firewall máy chủ, P5 tối ưu OS theo dịch vụ (sysctl, hugepage, THP, giới hạn file, pin CPU), P6 đĩa, P7 hồ sơ OS theo vai trò, P8 platform và database, P9 seal.
- Giá trị tuning (swappiness, hugepage, `max_map_count`, `LimitNOFILE`, ...) nằm ở **một nơi**: `ansible/roles/os_tuning/defaults/main.yml`, kèm lý do từng nhóm; checklist đọc đúng tệp đó, và test đảm bảo Ansible gộp ra cùng kết quả với checklist cho cả 16 VM.
- Mục **THỦ CÔNG** trong checklist cần người đọc, rồi `verify --ack-manual`.
- Trạng thái lưu ở `labs/.state/steps.json` (ngoài Git).
- Dựng riêng từng phần bằng `lab tf net`, `lab tf fw`, `lab tf vm <tên>...`; nạp lại luật `fw-1` bằng `lab fw bootstrap|sealed`.

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

## P1 lát 2: database và proxy ra (phạm vi dừng ở hạ tầng)

Lát 2 dừng ở **VM, mạng, platform (RKE2), database và hardening OS**. Chưa triển khai service ứng dụng nào.

Thứ tự chạy (trên máy lab):

```
lab up && lab provision          # VM, firewall, hardening
lab services                     # PostgreSQL, RabbitMQ, Redis, ClickHouse, egress-1
lab nettest                      # thăm dò mạng sinh từ ma trận (cho phép phải đến được, cấm phải hết thời gian)
lab smoke                        # database, RKE2 và egress chạy
lab seal && lab nettest          # gỡ bootstrap rồi chạy lại
```

Bí mật của lab (`generated/secrets.yml`, mật khẩu ngẫu nhiên, 0600, ngoài Git) chỉ cho lab; P5 thay bằng OpenBao cấp động.

### Phát hiện cần chốt trước khi triển khai service (chưa làm gì ở đây)

Khi khảo sát code để dự định lát này, thấy code hiện có lệch thiết kế 7.6. Ghi lại để chốt trước bước triển khai service:
1. `waap-edge` đọc Redis trực tiếp (`REDIS_URL`); nếu Redis ở Z4 thì cần luồng Z1 tới Z4, trái X01. Hướng giải: đặt Redis (`valkey-edge`) ở Z1 cho control-plane ghi vào, hoặc làm relay Stream Distribution ở `egw-1`.
2. Edge POST telemetry thẳng tới ingest của control-plane, chưa qua relay `egw-1` với mTLS cert node (N05 đến N08).
3. Client Go của metap chưa chắc nhận CA tuỳ chỉnh qua URL, nên mTLS cho RabbitMQ, Redis, PostgreSQL cần kiểm client trước (P2, P5).
4. `analytics-api` dùng cổng native 9000 không TLS của ClickHouse, N17 chỉ mở 8443 và 9440.
5. Chưa có ingress controller trong K8s (N04, N09 nói cổng 443).
6. Kafka (ADR-0018) và OpenBao (P5) không dịch vụ nào dùng ở code hiện tại.

## Luồng mirror (đề xuất bổ sung 7.6, chờ duyệt)

- **M01** mọi VM trừ Z0, Z1, Z5 tới `registry` tcp 80 (mirror apt và artifact) và 3142 (apt-cacher-ng).
- **M02** `registry` tới `egress-1` tcp 3128 để kéo từ vendor.
- Cả hai `covers: []` vì không có trong 7.6; cần thêm vào 7.6 nếu được duyệt.
- **M03** `zone:Z1` tới `egress-1` tcp 3128 (cũng `covers: []`, đề xuất). Egress-1 nằm ở Z2, không có trong danh sách cấm của X01. Squid được mở thêm nguồn `10.10.10.0/24`.
- **Z5 không có luồng đi ra** (X05 cấm mọi vùng). Bản vá được **đẩy vào** bằng `lab push` (`ansible/push.yml`, role `pkg_push`) qua bastion (N25), không thêm luồng nào; một test khẳng định không luồng nào có nguồn là Z5.
- Chờ duyệt: ghi M01 đến M03 và cơ chế đẩy vào 7.6.
