# Rà soát tham số theo best practice

Đề xuất của Claude, chờ duyệt. Tài liệu này ghi **mỗi nhóm tham số, giá trị, cơ sở và mức tin cậy**, để người duyệt biết số nào đã đối chiếu nguồn, số nào còn dựa vào kiến thức chung. Giá trị cụ thể từng bước nằm ở [CHECKLIST.md](CHECKLIST.md).

**Mức tin cậy:**
- **A**: đã đối chiếu tài liệu chính thức trong phiên rà soát này (nguồn ở dưới).
- **B**: theo benchmark hoặc tài liệu vendor mà tôi nắm, **chưa đối chiếu lại từng điều khoản** trong phiên này; cần người duyệt xác nhận.
- **C**: lựa chọn kỹ thuật cho lab (ước lượng theo kích thước VM), chưa đo.

Nguồn đã đối chiếu: [RKE2 CIS Hardening Guide](https://docs.rke2.io/security/hardening_guide), [RKE2 Installation Methods](https://docs.rke2.io/install/methods), [RKE2 CIS self-assessment 1.23](https://docs.rke2.io/security/cis_self_assessment123), [RabbitMQ Production Checklist](https://www.rabbitmq.com/docs/production-checklist). Nền K8s là **RKE2** (chủ dự án chọn; không dùng k3s).

## Đã sửa trong lần rà soát này

| Lỗi | Hậu quả nếu để nguyên | Sửa |
|---|---|---|
| Cấu hình audit và Pod Security tự viết cho k3s (k3s không bật audit mặc định) | Không còn áp dụng: nền K8s đổi sang RKE2, profile CIS của RKE2 tự tạo audit policy mặc định và cấu hình Pod Security theo profile (A) | Bỏ phần tự viết; dùng `profile: cis` |
| `loopback_users = none` trong RabbitMQ | Tài liệu RabbitMQ nói rõ không dùng cho production (A) | Bỏ; thêm ngưỡng bộ nhớ và `disk_free_limit` tường minh |
| Redis: unit Debian khoá ghi ngoài `/var/lib/redis`, lab đặt dữ liệu ở đĩa riêng | Redis không ghi được AOF (B) | Drop-in `ReadWritePaths` |
| ClickHouse cấu hình user trong `config.d` và dùng `default` không mật khẩu | `config.d/<users>` có thể không được áp dụng; tài khoản mặc định yếu (B) | `users.d`; `default` chỉ localhost; tài khoản `analytics` có sha256 và giới hạn nguồn |
| sysctl của K8s thiếu `kernel.keys.root_maxkeys/root_maxbytes`; chưa nạp `nf_conntrack` | Profile CIS bật `protect-kernel-defaults`, kubelet có thể từ chối chạy; khoá conntrack không tồn tại (B) | Thêm khoá và module (`overlay`, `br_netfilter`, `nf_conntrack`) |
| `ip_forward=0` của baseline ghi đè K8s; `ptrace_scope=1` ghi đè OpenBao | Node K8s mất forwarding; OpenBao yếu hơn dự định | Tuning nạp sau baseline (`99-zz-*`), có test |
| `unprivileged_bpf_disabled=1` | Ubuntu mặc định 2, không hạ được nên `sysctl --system` lỗi (B) | Đặt 2 |
| `sysctl -n` in giá trị nhiều trường bằng tab (`ip_local_port_range`, `tcp_rmem`) | Lệnh kiểm so sánh sai | Chuẩn hoá khoảng trắng trước khi so |
| squid: `dstdomain` chỉ khớp đúng tên; thiếu `.ubuntu.com` | Vùng ứng dụng không vá được sau seal; tên miền con bị chặn | Dấu chấm đầu (gồm tên miền con), thêm `.ubuntu.com`, `.githubusercontent.com`; `cache deny all`; chặn thêm dải CGNAT và IPv6 nội bộ |

## Hệ điều hành chung (baseline)

| Nhóm | Giá trị | Cơ sở | Tin cậy |
|---|---|---|---|
| SSH | Không root, không mật khẩu, `AllowGroups ops`, `MaxAuthTries 3`, `LoginGraceTime 30`, `ClientAliveInterval 300 / CountMax 2`, `MaxStartups 10:30:60`, tắt hostbased, rhosts, tunnel, GSSAPI, X11, forwarding (trừ bastion), `LogLevel VERBOSE`; mã hoá chacha20 và AES-GCM, MAC etm | CIS Ubuntu 24.04 mục SSH, Mozilla "modern" | B |
| Sysctl mạng | `rp_filter=1`, chặn source route, redirect (cả `secure_`), `log_martians=1`, `tcp_syncookies=1`, `tcp_rfc1337=1`, tắt IPv6 | CIS mục network | B |
| Sysctl kernel | ASLR 2, `kptr_restrict=2`, `dmesg_restrict=1`, `ptrace_scope=1`, `sysrq=0`, `perf_event_paranoid=3`, `kexec_load_disabled=1`, `unprivileged_bpf_disabled=2`, `bpf_jit_harden=2`, `suid_dumpable=0`, `protected_*` | CIS và Ubuntu hardening | B |
| Mount | `/tmp` và `/dev/shm`: `nodev,nosuid,noexec` | CIS | B |
| Module kernel | Chặn `cramfs freevxfs hfs hfsplus jffs2 udf usb-storage firewire-* dccp sctp rds tipc` | CIS | B |
| auditd | Luật danh tính, sudoers, sshd, PAM, giờ, hostname, module, mount, xoá tệp, phiên đăng nhập, AppArmor, giả danh; `max_log_file=50`, `num_logs=10`, xoay vòng; đầy đĩa thì `suspend` | CIS mục auditd | B |
| Thời gian | `pool ntp.ubuntu.com iburst maxsources 4` (mặc định của Ubuntu); `makestep 1.0 3`; cổng chrony đóng | Khuyến nghị chrony: từ 3 nguồn | B |
| Vá tự động | Chỉ security, reboot 03:30, không reboot khi có người đăng nhập, gỡ kernel cũ | Ubuntu unattended-upgrades | B |
| nftables | Default-deny cả input; log giới hạn 5 gói/giây trước khi drop | Thực hành chung | B |

## Tối ưu theo dịch vụ (bảng gốc: `ansible/roles/os_tuning/defaults/main.yml`)

| Dịch vụ | Giá trị chính | Cơ sở | Tin cậy |
|---|---|---|---|
| PostgreSQL | `swappiness=1`; `dirty_background_bytes=64MiB`, `dirty_bytes=512MiB`; hugepage 300 (shared_buffers 512MB / 2MB = 256, dư 15%); THP `enabled` và `defrag` = never; `random_page_cost=1.1`, `effective_io_concurrency=200`; `wal_compression`, `max_wal_size=2GB`, log checkpoint, lock wait, temp file; TLS ≥ 1.2; scram | Tài liệu PostgreSQL "Managing Kernel Resources", khuyến nghị SSD | B |
| PostgreSQL, ngoại lệ | `overcommit_memory=1` (PostgreSQL khuyến nghị 2) | data-1 gộp Redis, Redis cần 1. Đổi lại 2 khi PostgreSQL ở máy riêng | B |
| PostgreSQL, kích thước | `shared_buffers=512MB` (~10%, không phải 25%), `effective_cache_size=2GB`, `work_mem=4MB`, `max_connections=200` | VM 5 GiB dùng chung Redis và RabbitMQ | C |
| Redis | `overcommit_memory=1`, `somaxconn=1024` (≥ tcp-backlog 511), THP never, `noeviction`, AOF `everysec`, lệnh nguy hiểm chặn bằng ACL, user `default` tắt | Redis admin docs | B |
| RabbitMQ | `LimitNOFILE=65536`; keepalive 60/10/3; `vm_memory_high_watermark=0.4`, `disk_free_limit=2GB` (≥ ngưỡng bộ nhớ); bỏ `guest`; epmd và cổng phân tán chỉ loopback | Production checklist (watermark, disk, fd, loopback_users: **A**); keepalive và epmd: **B** | A, B |
| ClickHouse | `max_map_count=262144`, `LimitNOFILE=262144`, THP never, `max_server_memory_usage_to_ram_ratio=0.7`, `default` chỉ localhost, user riêng có sha256 | Tài liệu ClickHouse "Usage Recommendations" | B |
| Edge L7 | `somaxconn=65535`, `netdev_max_backlog=16384`, `tcp_max_syn_backlog=16384`, `ip_local_port_range 10240-65535`, `tcp_tw_reuse=1`, `tcp_fin_timeout=15`, buffer 16 MiB, `tcp_slow_start_after_idle=0`, BBR và `fq` | Thực hành proxy và CDN | B |
| WAF | Slice riêng: `AllowedCPUs`, `CPUQuota`, `MemoryMax`, không capability | ADR-0048 | C (số theo 2 vCPU) |
| OpenBao | Không swap, không core dump (`core_pattern`, `Storage=none`), `ptrace_scope=2`, `CAP_IPC_LOCK`, `LimitMEMLOCK=infinity` | Khuyến nghị hardening OpenBao/Vault | B |
| Node K8s (RKE2) | `ip_forward=1`, `bridge-nf-call-*=1`, `overcommit_memory=1`, `panic_on_oom=0`, `kernel.panic=10`, `panic_on_oops=1`, `keys.root_maxkeys=1000000`, `root_maxbytes=25000000`, inotify 8192/524288, `nf_conntrack_max=262144` | Profile CIS của RKE2 đòi sysctl cho kubelet (**A** về việc cần có); **danh sách và giá trị chính xác chưa đối chiếu** với tệp sysctl đi kèm tarball ở bản ghim; inotify, conntrack: **B** | B |

## RKE2

| Tham số | Giá trị | Cơ sở | Tin cậy |
|---|---|---|---|
| Cách cài | Tarball ghim phiên bản và sha256, giải nén vào `/usr/local` (nhị phân và unit systemd); không tải gì chưa kiểm | Installation Methods của RKE2 | A |
| Cổng | 6443 (API) và 9345 (supervisor, agent gia nhập) mở giữa các node | Requirements của RKE2 | A |
| `profile` | `cis` (biến `rke2_profile`; tên profile phụ thuộc phiên bản RKE2, hướng dẫn bản cũ dùng `cis-1.23`) | Hardening Guide, Server Config Reference nói hai giá trị khác nhau theo bản | A (có profile) / B (tên đúng theo bản ghim) |
| User `etcd` | Bắt buộc có (nologin) khi dùng profile CIS; thiếu thì RKE2 từ chối chạy; dữ liệu etcd thuộc `etcd:etcd` | CIS 1.23 self-assessment | A |
| Audit policy | Profile CIS tạo sẵn policy mặc định, vận hành viên tuỳ chỉnh sau | Hardening Guide | A |
| Pod Security | Chọn theo profile (Pod Security Standards từ v1.25) | Hardening Guide | A |
| `secrets-encryption` | `true` | Thực hành RKE2/CIS | B |
| `streaming-connection-idle-timeout` | `5m`; `terminated-pod-gc-threshold=10` | CIS | B |
| CNI | `canal` (mặc định RKE2, có NetworkPolicy) | Mặc định RKE2 | B |
| Ingress mặc định | Tắt (`rke2_disable`); tên thành phần đổi theo phiên bản (ingress-nginx ở bản cũ, có thể là Traefik ở bản mới) | Migration guide của RKE2 cho thấy mặc định đã đổi | B |
| sysctl kubelet | Bảng `role_k8s_*` ở `os_tuning` (panic_on_oom, overcommit, panic, panic_on_oops, keys, inotify, conntrack) | Profile CIS đòi các giá trị này | B |

## Giữa các node RKE2

| Hạng mục | Hiện trạng | Cơ sở | Tin cậy |
|---|---|---|---|
| Cổng giữa node | Firewall máy chủ chỉ mở: 6443 và 9345 trên server cho worker; 10250 (kubelet) và 9099 (Canal health) giữa mọi node; udp 8472 (VXLAN) giữa mọi node. **Không mở 2379 và 2380 (etcd)** vì chỉ có một server | [RKE2 Requirements](https://docs.rke2.io/install/requirements) | A |
| Cấm trong cùng vùng | Worker không SSH và không vào etcd, API, supervisor của worker khác; không SSH hay vào etcd của server. Kiểm bằng `lab nettest` | Nguyên tắc ít quyền nhất | C |
| Pod tới node | Chấp nhận theo giao diện (`cni0`, `flannel.1`), vì lưu lượng pod tới API và kubelet vào bằng giao diện CNI chứ không bằng IP node | Hành vi CNI | B |
| **NetworkPolicy mặc định chặn: làm ở P2** (đã chốt) | Tôi không thấy tài liệu RKE2 xác nhận profile CIS tự tạo chính sách chặn mặc định; P2 phải xác minh rồi viết. Cần có namespace và dịch vụ ứng dụng nên đứng sau bước triển khai dịch vụ. Xem `PLAN.md` mục P2 | CIS 1.23 self-assessment chỉ đòi CNI hỗ trợ NetworkPolicy | đã quyết |
| **Mã hoá lưu lượng pod: để profile `full`** (đã chốt) | Canal VXLAN không mã hoá; các node chung một cầu L2 (Z3) nên nguy cơ nằm ở hypervisor. Phương án khi làm: Canal với WireGuard, hoặc Cilium với mã hoá trong suốt | Thực hành chung | đã quyết |
| **HA control-plane: để profile `full`** (đã chốt) | Một server nên etcd một node; HA (3 server) cần mở 2379 và 2380 giữa các server | RKE2 HA docs | đã quyết |

## Chỗ lệch best practice có chủ đích

| Chỗ lệch | Vì sao | Khi nào sửa |
|---|---|---|
| Không có phân vùng riêng `/var`, `/var/log`, `/var/log/audit`, `/home` | Ảnh cloud chỉ có một phân vùng gốc | Máy thật hoặc đĩa phụ cho `/var/log/audit` |
| Secure Boot tắt | Ảnh `images:*` không ký (ADR-0045 ghi là rủi ro) | Khi có ảnh ký |
| TLS của PostgreSQL tự ký, `sslmode=require` không kiểm chứng chứng chỉ; RabbitMQ và Redis chưa TLS | Client Go của metap chưa chắc nhận CA tuỳ chỉnh | P2 và P5 (mTLS từ OpenBao) |
| `sudo NOPASSWD` cho nhóm `ops` | Tài khoản không có mật khẩu; xác thực bằng SSH cert ngắn hạn | Giữ, kèm ghi log vào/ra của sudo |
| `auditd -f 1` (ghi lỗi) thay vì `-f 2` (panic) | Lab; panic khi audit lỗi là quá mạnh | Production cân nhắc `-f 2` |
| Pod Security theo profile CIS của RKE2 | Profile CIS chọn mức Pod Security; mức cụ thể phải đọc ở bản ghim | Namespace cần nới phải đặt nhãn riêng; sẽ ảnh hưởng khi triển khai dịch vụ |
| `overcommit_memory=1` trên data-1 | Gộp PostgreSQL với Redis | Khi tách máy |
| Squid allowlist | Điểm xuất phát; CDN của registry đổi tên miền | Tinh chỉnh khi chạy thật |
| Chưa vá được vùng không có đường ra (Z1, Z2, Z4, Z5) | Thiếu bản sao apt nội bộ ở Z6 | Cần chốt thiết kế (xem `infra/README.md`) |

## Cần người duyệt xác nhận (mức B)

Số liệu theo benchmark nên đối chiếu với CIS Ubuntu 24.04 phiên bản đang dùng, tài liệu PostgreSQL 16, Redis, ClickHouse mới nhất, trước khi coi là chốt. Đặc biệt:
1. Danh sách sysctl mà profile CIS của RKE2 kiểm ở phiên bản sẽ ghim, tên profile (`cis` hay `cis-1.xx`), và tên thành phần ingress cần tắt.
2. `ERL_EPMD_ADDRESS` và `inet_dist_use_interface` của RabbitMQ ở phiên bản trong Ubuntu 24.04.
3. Cú pháp `users.d` và `password_sha256_hex` của ClickHouse ở phiên bản sẽ ghim.
4. Các giá trị kích thước (`shared_buffers`, hugepage, `MemoryMax`) cần đo lại trên VM thật.
