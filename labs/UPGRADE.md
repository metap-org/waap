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

## Lỗ hổng cần quyết: vùng dữ liệu không có đường ra

Theo ma trận 7.6, Z4 (dữ liệu) và Z5 (secret) **không có đường ra Internet** (X04), nên sau khi `lab seal` các máy này **không tới được repo của vendor** để nâng version. Hai hướng:
- **(a) Bản sao nội bộ**: đặt mirror apt và nơi lưu tarball ở `registry` (Z6), thêm luồng từ Z4 và Z5 tới `registry`. Phù hợp production; cần chốt thiết kế (cùng loại vấn đề với vá bảo mật ở `infra/README.md`).
- **(b) Cửa sổ nâng**: mở tạm đường ra cho đúng máy cần nâng bằng `lab fw bootstrap`, nâng, rồi `lab seal`. Chấp nhận được cho lab, không cho production.
