# Nâng version thành phần hạ tầng

Đề xuất của Claude, chờ duyệt. Nguyên tắc: **cài từ nguồn chính thức của vendor** (repo apt có khoá ký ghim fingerprint, hoặc binary ghim sha256), **gói bị hold** để không tự đổi, và **nâng bằng cách đổi một biến rồi chạy lại**. Không dùng gói của kho Ubuntu cho database.

| Thành phần | Nguồn | Biến | Nâng version nhỏ | Nâng version lớn | Hoàn tác |
|---|---|---|---|---|---|
| PostgreSQL | repo PGDG (`apt.postgresql.org`) | `pg_version`, `pg_pkg_version` | Đặt `pg_pkg_version` rồi `lab services --tags postgres` (gỡ hold, cài, hold lại) | **Không tự động.** Role từ chối nếu đang có cụm khác version. Làm tay: cài gói version mới song song, `pg_upgradecluster`, kiểm, rồi xoá cụm cũ | Version nhỏ: đặt `pg_pkg_version` cũ. Version lớn: giữ cụm cũ đến khi chắc chắn |
| RabbitMQ và Erlang | repo Team RabbitMQ | `rabbitmq_pkg_version`, `erlang_pkg_version` | Đặt cả hai (Erlang phải tương thích, [which-erlang](https://www.rabbitmq.com/docs/which-erlang)); chú ý epoch `1:` của gói Erlang | Theo hướng dẫn nâng cấp của RabbitMQ (có thể cần bật feature flag trước) | Đặt version cũ |
| ClickHouse | repo `packages.clickhouse.com` | `clickhouse_channel`, `clickhouse_pkg_version` | Đặt version cho **cả ba** gói | Theo hướng dẫn nâng cấp của ClickHouse | Đặt version cũ |
| Valkey | binary chính thức, `/opt/valkey/<version>` | `valkey_version`, `valkey_sha256` | Đổi hai biến rồi `lab services --tags valkey` (giải nén bản mới, đổi symlink, restart) | Như trên; đọc ghi chú phát hành | Trỏ lại `current` về thư mục cũ rồi restart (bản cũ vẫn còn trên đĩa) |
| RKE2 | tarball, `/usr/local` | `rke2_version`, `rke2_sha256` | Chưa tự động (role chỉ giải nén lần đầu); theo hướng dẫn nâng cấp của RKE2, mỗi lần một node | Như trên | Theo hướng dẫn RKE2 |

## Trước khi nâng

1. Chụp snapshot VM (`incus snapshot create <vm> pre-upgrade`). `lab down` cũng tự snapshot.
2. Đọc ghi chú phát hành của vendor, đặc biệt thay đổi phá vỡ tương thích.
3. Chạy bước kiểm tương ứng trước và sau (`lab step verify P8.x`).

## Mirror nội bộ ở registry (quyết định 2026-11-13: giữ repo vendor, thêm mirror)

Sau `lab seal`, Z4 và Z5 không có đường ra (X04), nên không tới được repo vendor. Giải pháp: `registry` (Z6) giữ bản sao có kiểm soát; các máy khác chỉ nói chuyện với nó (flow M01). Dựng bằng `lab services --tags registry` (bước P8.8).

**Cách hoạt động**
- aptly mirror 4 repo (PGDG, RabbitMQ Erlang, RabbitMQ server, ClickHouse) có **lọc** (`-filter`, `-filter-with-deps`) nên chỉ lấy gói cần, không cả kho.
- Mỗi lần đồng bộ tạo một **snapshot**. Snapshot chưa tới client cho tới khi bạn **promote**.
- Mirror **ký lại** bằng khoá riêng của nó. Client ghim fingerprint khoá đó (`infra/generated/mirror-key.fpr`) và vẫn chỉ tin repo đã ký.
- Tarball (Valkey, RKE2) nằm ở `/artifacts/<tên>/`, kiểm sha256 khi nhận và khi client tải.
- apt-cacher-ng (cổng 3142) cache bản vá Ubuntu cho Z2, Z3, Z4, Z6.
- Timer hằng tuần (Chủ nhật 02:17) chỉ đồng bộ và chụp snapshot, **không bao giờ promote**.

**Quy trình nâng một database**
1. Trên registry: `waap-mirror-sync` rồi `waap-mirror-list` xem snapshot mới và gói mới.
2. Đọc ghi chú phát hành, chụp snapshot VM đích.
3. `waap-mirror-promote <repo> <snapshot|latest>` cho phép client thấy bản mới.
4. Đổi đúng một biến pin (`pg_pkg_version`, ...) rồi `lab services --tags postgres`.
5. `lab step verify P8.x`.

**Hoàn tác:** `waap-mirror-promote <repo> <snapshot cũ>` (publish switch) rồi pin về bản cũ.

**Mirror chưa phủ Z1 và Z5.** X01 và X05 cấm khởi tạo kết nối tới Z6, nên hai vùng này không kéo được. Cần quyết (đề xuất: mô hình đẩy từ Z6, cùng loại vấn đề với N27); xem `infra/README.md`.
