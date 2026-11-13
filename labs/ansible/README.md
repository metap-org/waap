# Bộ cài và hardening (P4, lát 1)

Đề xuất của Claude, chờ duyệt. Hiện thực [ADR-0048](../../docs/architecture/09-adr/0048-node-baseline-and-hardening-profiles.md): một chuẩn nền (`baseline`) cho mọi node, cộng hồ sơ theo vai trò.

## Cấu trúc

| Role | Nội dung |
|---|---|
| `baseline` | SSH chỉ cert ngắn hạn, không mật khẩu, không root; nhóm `ops` và sudo có ghi log; `/tmp` và `/dev/shm` `noexec`; chặn module kernel; sysctl chuẩn, tắt IPv6; AppArmor; auditd; AIDE; chrony; vá bảo mật tự động; gỡ trình biên dịch |
| `hostfw` | Firewall máy chủ nftables default-deny, **sinh từ ma trận 7.6** (`gen_nft.py`); phủ cả luồng cùng vùng mà `fw-1` không thấy; khoá đường ra cho vai trò nhạy cảm (`edge-waf`, `openbao`, kho dữ liệu) |
| `hardened_unit` | Drop-in systemd chuẩn: `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, tập capability tối thiểu, `MemoryMax`, bộ lọc syscall |
| `disks` | Đĩa phụ cho dữ liệu, WAL, audit, cache (xfs, `noatime,nodev,nosuid`, `noexec` cho dữ liệu); LUKS có sẵn, mặc định tắt ở lab |
| `profile_rke2_node` | RKE2 (không dùng k3s): cài bằng tarball đã ghim sha256, profile CIS (kubelet khoá, audit policy mặc định, Pod Security theo profile), user `etcd`, mã hoá secret, CNI canal, tắt ingress mặc định; không swap |
| `profile_edge_l7` | sysctl mạng, user riêng, chỉ `CAP_NET_BIND_SERVICE`, cache `noatime`, script RSS cho bare metal (tắt ở VM) |
| `profile_edge_waf` | Slice systemd riêng (CPU và RAM), không capability, không đường ra |
| `profile_data` | Đĩa riêng, tài khoản backup, tắt THP |
| `profile_secret` | Không swap, không core dump, `CAP_IPC_LOCK` và `LimitMEMLOCK`, đĩa audit riêng |
| `profile_bastion` | WireGuard, ghi phiên bằng `pam_tty_audit`, MFA TOTP (mặc định tắt), kiểm không có khoá riêng |
| `profile_ops` | Runner không giữ secret, đường ra qua `egress-1` |
| `aide_refresh` | Chạy cuối: thay đổi do Ansible là hợp lệ, thay đổi ngoài Ansible bị AIDE báo |

## Cài từng thành phần hạ tầng đứng riêng (`waap-install`)

Phạm vi dừng ở **VM, mạng, platform, database và hardening OS**. Chưa có triển khai từng service ứng dụng (zones-service, edge, control-plane, ...): đó là bước sau, chưa làm.

Mỗi thành phần cài được **độc lập trên một máy bất kỳ**, bất kể vùng mạng, không cần lab hay inventory. Lab chỉ là một nơi gọi cùng các role này.

```
labs/bin/waap-install list
labs/bin/waap-install redis --host local
labs/bin/waap-install postgres --host 10.0.1.5 --harden \
    -e 'app_allow_from=["10.0.2.0/24"]' -e 'app_ssh_from=["10.0.9.5/32"]'
```

| Nhóm | Thành phần |
|---|---|
| Database | `postgres`, `rabbitmq`, `valkey`, `clickhouse` |
| Platform | `rke2-node` (server hoặc agent) |
| Mạng | `egress` (squid, đường ra duy nhất có allowlist) |

- **Tham số rõ ràng, không ngầm theo vùng:** IP nghe mặc định là IP của máy; `app_allow_from` là danh sách CIDR được tới thành phần (cũng là danh sách PostgreSQL, ClickHouse, squid chấp nhận).
- **Mật khẩu tự sinh** lần đầu, lưu ở `~/.waap-install/secrets/<host>/` trên máy chạy lệnh, dùng lại các lần sau. Ghi đè bằng `-e`.
- **`--harden`** áp thêm chuẩn nền (`baseline`). Khi chưa có CA SSH thì giữ khoá SSH tĩnh, **không tự khoá bạn**.
- **Firewall riêng:** chỉ bật khi có `app_allow_from`, bắt buộc kèm `app_ssh_from` (nếu không, từ chối chạy). Luật input default-deny, chỉ mở cổng của thành phần.
- Danh sách sinh từ `labs/infra/tools/gen_apps.py`; sửa ở đó rồi chạy lại, CI kiểm không lệch.
- **Đã chạy thật** (sandbox không có systemd): cơ chế biến, mật khẩu tự sinh và chạy tới bước nạp systemd; `--check` chạy qua nhiều tác vụ. **Chưa chạy được:** cài gói apt (sandbox thiếu `python3-apt`) và bật dịch vụ systemd, nên các role mới qua syntax, lint và `--check`.

## Dịch vụ (P1 lát 2, `services.yml`)

| Role | Nội dung |
|---|---|
| `vendor_repo` | Repo apt của vendor với khoá ký **ghim fingerprint** (tải khoá, đọc fingerprint, so với giá trị đã ghim, lệch thì dừng; dùng `Signed-By`, không `apt-key`) |
| `svc_postgres` | PostgreSQL từ repo **PGDG** chính thức (`pg_version`, mặc định 17), gói bị `hold`; đĩa dữ liệu và WAL riêng, checksum, scram, `hostssl` chỉ từ nguồn khai báo; chặn nâng version lớn ngầm |
| `svc_rabbitmq` | RabbitMQ và Erlang từ repo **Team RabbitMQ** (`deb1`/`deb2.rabbitmq.com`, cùng một khoá), ưu tiên hơn gói Ubuntu bằng apt pin, gói bị `hold`; xoá `guest`; epmd chỉ loopback |
| `svc_valkey` | **Valkey** (thay Redis) từ binary chính thức ghim sha256, `/opt/valkey/<version>` và symlink `current`; ACL, `noeviction`, AOF; unit có sandbox |
| `svc_clickhouse` | ClickHouse từ repo apt chính thức (kênh `lts` hoặc `stable`), ba gói cùng version và bị `hold`; user `analytics` có mật khẩu |
| `svc_registry` | Mirror nội bộ trên `registry`: aptly (mirror có lọc, snapshot, promote, ký lại), kho artifact ghim sha256, apt-cacher-ng, nginx. Script `waap-mirror-{sync,promote,list,artifact}`; timer hằng tuần chỉ đồng bộ |
| `pkg_push` | Đẩy bản vá vào máy không có đường kéo (Z5), chạy qua `push.yml` hoặc `lab push`: danh sách gói đang cài -> registry chọn gói mới hơn và không hold -> tar kèm sha256 -> máy đích kiểm và `apt-get install ./*.deb` không cần mạng |
| `svc_perimeter` | Chỉ `egress-1`: squid có allowlist, chặn dải riêng và metadata. `ingress-1` và `egw-1` chưa cấu hình (chờ triển khai dịch vụ) |

## Cách dùng

```
labs/bin/lab ssh-ca                      # CA SSH của lab
export LAB_VALKEY_VERSION=9.x.y; export LAB_VALKEY_SHA256=<sha256 tarball noble x86_64 trên trang release>
export LAB_RKE2_VERSION=v1.xx.y+rke2r1   # ghim: lấy từ trang phát hành RKE2
export LAB_RKE2_SHA256=<sha256 của rke2.linux-amd64.tar.gz>   # tự kiểm, bộ cài không tải tarball chưa ghim
labs/bin/lab provision                   # site.yml trên mọi node
labs/bin/lab seal                        # gỡ bootstrap, đổi sang apt qua egress, firewall chỉ còn ma trận
labs/bin/lab audit                       # Lynis, chỉ số hardening mỗi node
cd labs/ansible && ansible-playbook site.yml --check --diff   # kiểm trôi
```

Kết nối ở lab dùng `community.general.incus` (không cần SSH); sau P2 chuyển sang SSH qua `bastion`.

## Đã kiểm chứng và chưa

- **Đã chạy:** `ansible-lint` (profile production) sạch; `ansible-playbook --syntax-check` cho `site.yml` và `audit.yml`; dựng thật các template mẫu (sshd, sysctl, audit, chrony, sudoers, RKE2 config, wg0, drop-in systemd) và đọc lại kết quả (RKE2 config là YAML hợp lệ); luật nftables cho `fw-1` và cho 15 máy qua `nft -c`. Có job CI `labs-infra`.
- **Chưa chạy được (không có Incus, quyền ảo hoá, Ubuntu thật ở đây):** chạy `site.yml` trên máy thật, `sshd -t` thật, nạp luật auditd, `aideinit`, RKE2 lên được, `lab seal` không tự khoá mình. Kỳ vọng phải sửa khi chạy lần đầu.
- **Lát 2 đã kiểm:** `ansible-lint` và `--syntax-check` cho `services.yml` và 6 playbook `apps/`. **Chưa kiểm:** mọi role `svc_*` và `rke2-node` trên máy thật.
- **Chưa viết:** RKE2, Vector gửi log về `obs`, phân vùng `/var` `/var/log` `/var/log/audit` riêng (image cloud chỉ có một phân vùng gốc), SSH cert do OpenBao ký (P5), ngưỡng điểm CIS (L5 chưa chốt), OpenSCAP với nội dung CIS (chưa xác minh gói nào có cho Ubuntu 24.04 mà không cần Ubuntu Pro).

## Lưu ý an toàn

- **Bootstrap:** khoá SSH tĩnh còn tồn tại cho tới `lab seal`; apt ra Internet trực tiếp. Sau seal, chỉ cert do CA lab ký mới vào được qua SSH (lab vẫn dùng `incus exec`).
- `bastion_mfa` mặc định tắt: bật khi mọi người đã có TOTP, nếu không sẽ tự khoá ra ngoài.
- `/tmp` `noexec` có thể làm hỏng script cài đặt của một số gói; nếu gặp thì đặt `tmp_size` hoặc ghi lại ngoại lệ, đừng bỏ `noexec` ngầm.
- **Cập nhật cho vùng không có đường ra:** Z1, Z4, Z5 không có luồng nào tới `egress-1` theo ma trận, nên `unattended-upgrades` ở đó không lấy được gói sau `seal`. Cần một bản sao apt nội bộ ở `registry` (Z6) và các luồng mới để tới đó; chưa có trong 7.6, xem README của `infra`.
