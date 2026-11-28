# Migration bảng entity của management-service

File ở đây tạo và sửa bảng `waf.*`. Chúng được nhúng vào binary. Service không tự chạy chúng.

Chạy từ gốc repo `waap`:

```bash
go run ./m-management/cmd/management-service migrate generate <ten_thay_doi>   # sau khi đổi entity
go run ./m-management/cmd/management-service migrate up                        # áp
go run ./m-management/cmd/management-service migrate down [--yes]              # xem / rollback
```

- Review cả `.up.sql` và `.down.sql`, commit cùng `schema.snapshot.json`. **Không sửa** file đã áp dụng ở đâu đó.
- Đổi tên field: thêm `--rename waf.<bảng>:<cũ>=<mới>`, nếu không sẽ thành thêm cột mới.
- CI (`TestTheMigrationsCoverTheEntities`) fail nếu đổi entity mà chưa sinh migration.

Hướng dẫn đầy đủ: `../../../metap-docs/docs/migrations.md`.
