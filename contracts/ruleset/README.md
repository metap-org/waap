# Contract: rule-set trong Redis (control-plane → edge-plane)

`control-plane/ruleset` (producer) và `edge-plane/ruleset` (consumer) khai báo **hai bản
copy độc lập** của cùng một định dạng JSON + tên key Redis — cố ý, vì hai plane là hai Go module
riêng, deploy riêng, và `edge-plane` không được import code của `control-plane`.

Các file golden ở đây là thứ giữ hai bản copy đó khớp nhau:

- `zone.golden.json` — một `CompiledZone` dùng **mọi** `Action`, `Field`, `Op` và mọi dạng
  `MatchExpr` (`always`/`predicate`/`all`/`any`/`not`), cả `rateLimit` lẫn DDoS policy có
  `pathPrefix`/`httpMethod`.
- `keys.golden.json` — `schemaVersion` và định dạng các key Redis.

Hai test dùng chung các file này:

| Phía | Test | Kiểm tra |
|---|---|---|
| producer | `control-plane/ruleset/contract_test.go` | output `json.Marshal` của control-plane **bằng đúng** golden |
| consumer | `edge-plane/ruleset/contract_test.go` | edge parse được golden (`DisallowUnknownFields`) và đọc ra đúng giá trị |

Đổi định dạng (thêm field/op/action…): sửa producer, chạy
`go test ./ruleset/ -run Contract -update` trong `control-plane` để ghi lại golden, rồi sửa
edge cho đến khi test consumer pass. Đổi không tương thích ngược thì tăng `schemaVersion` ở **cả
hai** bên.
