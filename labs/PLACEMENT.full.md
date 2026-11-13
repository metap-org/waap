# Bản đồ cài đặt: thành phần nào ở máy nào, vùng nào, ai được vào

Sinh tự động bởi `labs/infra/tools/gen_lab.py placement` từ profile `full` và ma trận luồng. **Không sửa tay.**

| Thành phần | Máy (vùng, IP) | Ai được kết nối tới (từ ma trận) | Cài bằng (lab) | Cài bằng (đứng riêng) | Ghi chú |
|---|---|---|---|---|---|
| PostgreSQL (PGDG) | `pg-1` (Z4, 10.10.40.11); `pg-2` (Z4, 10.10.40.12) | N14: vùng Z3 → tcp/5432<br>N23: postgres → tcp/5432 | `lab services --tags postgres` | `waap-install postgres` | Repo PGDG chính thức, gói bị hold; đĩa dữ liệu và WAL riêng. Profile full có node thứ hai nhưng sao chép (replication) CHƯA viết, nên chỉ cài node đầu |
| RabbitMQ (repo Team RabbitMQ) | `mq-1` (Z4, 10.10.40.15) | N15: vùng Z3 → tcp/5671 | `lab services --tags rabbitmq` | `waap-install rabbitmq` | Erlang và RabbitMQ cùng repo, gói bị hold. Cổng 5672 không TLS; N15 chỉ mở 5671, nên chưa ai ngoài máy này vào được cho tới khi có TLS (P2/P5) |
| Valkey (valkey-central) | `cache-1` (Z4, 10.10.40.16) | N18: vùng Z3 → tcp/6379 | `lab services --tags valkey` | `waap-install valkey` | Binary chính thức ghim sha256, /opt/valkey/<version>; cache và hot key trung tâm |
| Valkey (valkey-edge) | `valkey-edge` (Z1, 10.10.10.15) | N11: edge-l7 → tcp/6379 | `lab services --tags valkey` | `waap-install valkey` | Nằm ở vùng Edge DMZ theo thiết kế (ADR-0021) |
| ClickHouse | `ch-1` (Z4, 10.10.40.13) | N17: vùng Z3 → tcp/8443,9440 | `lab services --tags clickhouse` | `waap-install clickhouse` | Repo apt chính thức của ClickHouse, 3 gói cùng version, bị hold |
| RKE2 server | `k8s-cp-1` (Z3, 10.10.30.11); `k8s-cp-2` (Z3, 10.10.30.12); `k8s-cp-3` (Z3, 10.10.30.13) | L02a: k8s-app → tcp/6443,9345<br>L02b: vùng Z3 → tcp/10250,9099<br>L02c: vùng Z3 → udp/8472<br>L02d: k8s-cp → tcp/2379,2380<br>L02e: vùng Z3 → udp/51820 | `lab provision --tags k8s --limit <vm>` | `waap-install rke2-node` | Cần ghim phiên bản và sha256 của tarball |
| RKE2 agent | `k8s-wk-1` (Z3, 10.10.30.21); `k8s-wk-2` (Z3, 10.10.30.22); `k8s-wk-3` (Z3, 10.10.30.23) | N04: ingress-1 → tcp/443<br>N09: egw-1 → tcp/443<br>L02b: vùng Z3 → tcp/10250,9099<br>L02c: vùng Z3 → udp/8472<br>L02e: vùng Z3 → udp/51820 | `lab provision --tags k8s --limit <vm>` | `waap-install rke2-node -e rke2_role=agent` |  |
| Mirror nội bộ (aptly + artifact + apt-cacher-ng) | `registry` (Z6, 10.10.60.13) | N28: vùng Z3 → tcp/443<br>N28c: ci-runner → tcp/443<br>M01: all-vms-except:[Z0,Z1,Z5] → tcp/80,3142 | `lab services --tags registry` | `waap-install registry` | Nguồn gói duy nhất cho vùng không có đường ra |
| egress-1 (squid) | `egress-1` (Z2, 10.10.20.13) | N21: vùng Z3 → tcp/3128<br>N29: ci-runner → tcp/3128<br>M02: registry → tcp/3128 | `lab services --tags egress` | `waap-install egress` | Đường ra duy nhất có allowlist |
| Kafka | `kafka-1` (Z4, 10.10.40.14) | N16: vùng Z3 → tcp/9093 | `(chưa có)` | `(chưa có)` | VM đã có (đĩa dữ liệu) nhưng CHƯA có bộ cài; không dịch vụ nào dùng Kafka ở code hiện tại |
| Object storage (SeaweedFS) | `s3-1` (Z4, 10.10.40.17) | N18o: vùng Z3 → tcp/443 | `(chưa có)` | `(chưa có)` | Vai trò gắn vào data-1 nhưng CHƯA có bộ cài |
| OpenBao | `bao-1` (Z5, 10.10.50.11); `bao-2` (Z5, 10.10.50.12); `bao-3` (Z5, 10.10.50.13) | N19: vùng Z3 → tcp/8200<br>N20: openbao → tcp/8200,8201 | `(chưa có)` | `(chưa có)` | VM đã hardening (không swap, không core dump); cài OpenBao là P5 |
| ingress-1, egw-1 | `ingress-1` (Z2, 10.10.20.11) | N01: vùng Z0 → tcp/443 | `(chưa có)` | `(chưa có)` | Chờ triển khai dịch vụ ứng dụng |

Luồng quản trị chung (SSH từ `bastion` N25, giám sát từ `obs` N26, NTP và DNS tới `infra` N27) áp dụng cho mọi máy và không lặp lại trong bảng.

## Mỗi thành phần được cài ra sao

- **Lab:** `lab services --tags <thành phần>` chạy Ansible theo nhóm vai trò trong inventory; VM nào mang vai trò đó thì được cài. Từng bước và giá trị mục tiêu ở [CHECKLIST.md](CHECKLIST.md) (P8).
- **Đứng riêng:** `waap-install <thành phần> --host <ip|local>` cài lên một máy bất kỳ, không cần inventory; truyền `app_allow_from` (CIDR được vào) và `app_ssh_from` để bật firewall riêng.
- Cả hai đường dùng chung các role `svc_*`, nên cấu hình giống nhau.
