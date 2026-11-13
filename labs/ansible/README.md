# Bộ cài và hardening (P4, lát 1)

Đề xuất của Claude, chờ duyệt. Hiện thực [ADR-0048](../../docs/architecture/09-adr/0048-node-baseline-and-hardening-profiles.md): một chuẩn nền (`baseline`) cho mọi node, cộng hồ sơ theo vai trò.

## Cấu trúc

| Role | Nội dung |
|---|---|
| `baseline` | SSH chỉ cert ngắn hạn, không mật khẩu, không root; nhóm `ops` và sudo có ghi log; `/tmp` và `/dev/shm` `noexec`; chặn module kernel; sysctl chuẩn, tắt IPv6; AppArmor; auditd; AIDE; chrony; vá bảo mật tự động; gỡ trình biên dịch |
| `hostfw` | Firewall máy chủ nftables default-deny, **sinh từ ma trận 7.6** (`gen_nft.py`); phủ cả luồng cùng vùng mà `fw-1` không thấy; khoá đường ra cho vai trò nhạy cảm (`edge-waf`, `openbao`, kho dữ liệu) |
| `hardened_unit` | Drop-in systemd chuẩn: `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, tập capability tối thiểu, `MemoryMax`, bộ lọc syscall |
| `disks` | Đĩa phụ cho dữ liệu, WAL, audit, cache (xfs, `noatime,nodev,nosuid`, `noexec` cho dữ liệu); LUKS có sẵn, mặc định tắt ở lab |
| `profile_k8s_node` | k3s: kubelet khoá (`protect-kernel-defaults`, tắt cổng chỉ đọc, `anonymous-auth=false`), mã hoá secret, audit log API, tắt traefik và servicelb; không swap |
| `profile_edge_l7` | sysctl mạng, user riêng, chỉ `CAP_NET_BIND_SERVICE`, cache `noatime`, script RSS cho bare metal (tắt ở VM) |
| `profile_edge_waf` | Slice systemd riêng (CPU và RAM), không capability, không đường ra |
| `profile_data` | Đĩa riêng, tài khoản backup, tắt THP |
| `profile_secret` | Không swap, không core dump, `CAP_IPC_LOCK` và `LimitMEMLOCK`, đĩa audit riêng |
| `profile_bastion` | WireGuard, ghi phiên bằng `pam_tty_audit`, MFA TOTP (mặc định tắt), kiểm không có khoá riêng |
| `profile_ops` | Runner không giữ secret, đường ra qua `egress-1` |
| `aide_refresh` | Chạy cuối: thay đổi do Ansible là hợp lệ, thay đổi ngoài Ansible bị AIDE báo |

## Cài từng ứng dụng đứng riêng (`waap-install`)

Mỗi ứng dụng cài được **độc lập trên một máy bất kỳ**, bất kể vùng mạng, không cần lab, inventory hay ma trận luồng. Lab chỉ là một nơi gọi cùng các role này.

```
labs/bin/waap-install list                                   # 18 ứng dụng
labs/bin/waap-install redis --host local                     # cài lên chính máy này
labs/bin/waap-install postgres --host 10.0.1.5 --harden \
    -e 'app_allow_from=["10.0.2.0/24"]' -e 'app_ssh_from=["10.0.9.5/32"]'
labs/bin/waap-install zones-service --host 10.0.3.7 -e @labs/ansible/apps/examples/zones-service.yml
```

| Nhóm | Ứng dụng |
|---|---|
| Kho dữ liệu | `postgres`, `rabbitmq`, `redis`, `clickhouse` |
| Biên | `ingress`, `egw`, `egress` |
| Edge | `edge` (cần `redis` cùng máy hoặc `edge_redis_url`) |
| K8s | `k3s-node` (server hoặc agent) |
| Dịch vụ Go (systemd, không cần K8s) | `zones-service`, `scanning-service`, `alerting-service`, `graphql-gateway`, `analytics-api`, `config-distributor` |
| Hỗ trợ lab | `origin`, `loadgen`, `dns-mock` |

- **Tham số rõ ràng, không ngầm theo vùng:** IP nghe mặc định là IP của máy; `app_allow_from` là danh sách CIDR được tới ứng dụng (cũng là danh sách PostgreSQL, ClickHouse, squid chấp nhận); thiếu địa chỉ bắt buộc thì báo lỗi ngay.
- **Mật khẩu tự sinh** lần đầu, lưu ở `~/.waap-install/secrets/<host>/` trên máy chạy lệnh, dùng lại các lần sau (idempotent). Ghi đè bằng `-e`.
- **`--harden`** áp thêm chuẩn nền (`baseline`) trước khi cài. Khi chưa có CA SSH thì giữ khoá SSH tĩnh, **không tự khoá bạn**.
- **Firewall riêng:** chỉ bật khi có `app_allow_from`, bắt buộc kèm `app_ssh_from` (nếu không, từ chối chạy). Luật input default-deny, chỉ mở cổng của ứng dụng.
- **Dịch vụ Go** chạy bằng systemd với drop-in hardening chuẩn (`NoNewPrivileges`, `ProtectSystem=strict`, bỏ hết capability, `MemoryMax`), biến môi trường ở `/etc/waap/<app>.env` (0640 root:nhóm dịch vụ). Cần nhị phân từ `labs/bin/lab build` (hoặc `-e app_bin_dir=...`). `apps/examples/` có danh sách khoá bắt buộc; thiếu khoá thì dừng sớm.
- Danh sách ứng dụng sinh từ `labs/infra/tools/gen_apps.py`; sửa ở đó rồi chạy lại, CI kiểm không lệch.
- **Đã chạy thật** (trong sandbox không có systemd): `zones-service` chạy tới bước nạp systemd, tạo đúng người dùng, tệp env 0640, unit và drop-in (đọc lại được); `systemd-analyze verify` không báo lỗi. Thiếu khoá bắt buộc thì dừng với thông báo rõ. **Chưa chạy được:** cài gói apt (sandbox thiếu `python3-apt`) và bật dịch vụ systemd, nên `postgres`, `redis`... mới qua kiểm cú pháp, lint và chế độ `--check`.

## Dịch vụ (P1 lát 2, `services.yml`)

| Role | Nội dung |
|---|---|
| `svc_postgres` | PostgreSQL 16 trên đĩa dữ liệu và đĩa WAL riêng, checksum, scram, `hostssl` chỉ từ Z3 |
| `svc_rabbitmq` | RabbitMQ, xoá `guest`, tài khoản ứng dụng |
| `svc_redis` | Redis 7.0 (BSD, từ apt) với ACL: `control` ghi, `edge` chỉ đọc `waf:*`; không loại bỏ khoá (`noeviction`) |
| `svc_clickhouse` | Chỉ cài gói `.deb` đã ghim kèm sha256 (như k3s); không tải gói chưa kiểm |
| `svc_perimeter` | nginx cho `ingress-1` và `egw-1` (relay telemetry), squid cho `egress-1` (allowlist, chặn dải riêng và metadata) |
| `svc_origin`, `svc_loadgen`, `svc_dns_mock` | Origin giả, kịch bản tải, dns-mock |
| `svc_edge_app` | Triển khai `waap-edge` (ngoài K8s theo ADR-0023), biến môi trường chỉ nhóm dịch vụ đọc được |

## Cách dùng

```
labs/bin/lab ssh-ca                      # CA SSH của lab
export LAB_K3S_VERSION=v1.xx.y+k3s1      # ghim: lấy từ trang phát hành k3s
export LAB_K3S_SHA256=<sha256 của nhị phân>   # tự kiểm, bộ cài không tải nhị phân chưa ghim
labs/bin/lab provision                   # site.yml trên mọi node
labs/bin/lab seal                        # gỡ bootstrap, đổi sang apt qua egress, firewall chỉ còn ma trận
labs/bin/lab audit                       # Lynis, chỉ số hardening mỗi node
cd labs/ansible && ansible-playbook site.yml --check --diff   # kiểm trôi
```

Kết nối ở lab dùng `community.general.incus` (không cần SSH); sau P2 chuyển sang SSH qua `bastion`.

## Đã kiểm chứng và chưa

- **Đã chạy:** `ansible-lint` (profile production) sạch; `ansible-playbook --syntax-check` cho `site.yml` và `audit.yml`; dựng thật các template mẫu (sshd, sysctl, audit, chrony, sudoers, k3s config, wg0, drop-in systemd) và đọc lại kết quả (k3s config là YAML hợp lệ); luật nftables cho `fw-1` và cho 15 máy qua `nft -c`. Có job CI `labs-infra`.
- **Chưa chạy được (không có Incus, quyền ảo hoá, Ubuntu thật ở đây):** chạy `site.yml` trên máy thật, `sshd -t` thật, nạp luật auditd, `aideinit`, k3s lên được, `lab seal` không tự khoá mình. Kỳ vọng phải sửa khi chạy lần đầu.
- **Lát 2 đã kiểm:** `ansible-lint` và `--syntax-check` cho `services.yml`; các binary `waap-edge`, `dns-mock`, `waf-config-distributor`, `db-migrate` biên dịch thật bằng Go 1.26; manifest K8s render và đọc lại được 16 tài liệu YAML. **Chưa kiểm:** `docker build` các ảnh (không có Docker daemon ở đây), `kubectl apply`, mọi role `svc_*` trên máy thật.
- **Chưa viết:** RKE2, Vector gửi log về `obs`, phân vùng `/var` `/var/log` `/var/log/audit` riêng (image cloud chỉ có một phân vùng gốc), SSH cert do OpenBao ký (P5), ngưỡng điểm CIS (L5 chưa chốt), OpenSCAP với nội dung CIS (chưa xác minh gói nào có cho Ubuntu 24.04 mà không cần Ubuntu Pro).

## Lưu ý an toàn

- **Bootstrap:** khoá SSH tĩnh còn tồn tại cho tới `lab seal`; apt ra Internet trực tiếp. Sau seal, chỉ cert do CA lab ký mới vào được qua SSH (lab vẫn dùng `incus exec`).
- `bastion_mfa` mặc định tắt: bật khi mọi người đã có TOTP, nếu không sẽ tự khoá ra ngoài.
- `/tmp` `noexec` có thể làm hỏng script cài đặt của một số gói; nếu gặp thì đặt `tmp_size` hoặc ghi lại ngoại lệ, đừng bỏ `noexec` ngầm.
- **Cập nhật cho vùng không có đường ra:** Z1, Z4, Z5 không có luồng nào tới `egress-1` theo ma trận, nên `unattended-upgrades` ở đó không lấy được gói sau `seal`. Cần một bản sao apt nội bộ ở `registry` (Z6) và các luồng mới để tới đó; chưa có trong 7.6, xem README của `infra`.
