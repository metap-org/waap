# Bản đồ: phần nào của metap, phần nào của ứng dụng (BẢN NHÁP)

Đọc trang này trước. Mỗi file Go của service có dòng đầu `// Layer: ...` cho biết nó thuộc phần
nào; trang này là bản đồ tổng.

## 1. Một câu

**metap chạy service. Ứng dụng (waap) chỉ cắm vào 6 điểm của metap.** Mọi thứ khác (HTTP server,
đăng nhập, phân quyền, CRUD, workflow, audit, event, bảng DB) là của metap, không nằm trong repo waap.

## 2. Một request đi qua những gì

```
 Portal ──► api-gateway ─────────────────────────────────────────────  (metap)
                │
                ▼   management-service  (một process, do metap dựng: app.Main)
   ┌───────────────────────────────────────────────────────────────────────────┐
   │ METAP  router /api/{entity}  ·  xác thực JWT  ·  kiểm quyền  ·  rate limit │
   │                                                                           │
   │   ghi (create/update/delete)          action (POST .../actions/{tên})     │
   │        │                                      │                           │
   │        ▼                                      ▼                           │
   │ APP  chuỗi guard (guard/)             APP  handler mỏng (actions.go)      │
   │   quota→field→domain→account→version         │                            │
   │        │                                      ▼                           │
   │        │                              APP  logic (netcheck/, presets/,    │
   │        │                                   policy/, versions/)            │
   │        ▼                                      │                           │
   │ METAP  crud.RecordBackend  (state.Crud) ◄─────┘   đọc/ghi dữ liệu         │
   │        validate field · workflow · audit · outbox event · SQL             │
   └───────────────────────────────────────────────────────────────────────────┘
                │
                ▼
            Postgres / RabbitMQ                                          (metap)
```

Quy tắc đọc: mũi tên xuống đáy luôn đi vào `state.Crud` của metap. Code ứng dụng **không** viết SQL
và **không** tự dựng router.

## 3. Sáu điểm cắm (app.Options) và ứng dụng cắm gì vào

| Điểm cắm của metap | Việc nó làm | Ứng dụng cắm gì | Ở đâu (zones) |
|---|---|---|---|
| `SubmitEntity` (trong `init()`) | Khai báo entity: metap tạo bảng, route CRUD, kiểm quyền, workflow | Định nghĩa entity | `entities/*.go` |
| `Options.WrapBackend` | Bọc backend ghi dữ liệu | Chuỗi guard | `zones/guard/` (`backend.go` = chuỗi, mỗi guard một file) |
| `Options.Actions` | Phục vụ `POST /api/{entity}/actions/{tên}` | Handler từng action | `zones/actions.go` (nối dây) → `netcheck/`, `presets/`, `policy/`, `versions/` (logic) |
| `Options.Routes` | Route ngoài entity | Chỉ route cho dịch vụ gọi (`/internal/*`, health) | `actions.go` (`MountRoutes`) |
| `bootstrap.Submit` | Cấp quyền cho role | Danh sách grant theo persona | `m-bootstrap/personas.go` |
| `Options.ExternalEntities` | Entity mà dữ liệu ở hệ khác | (chỉ alerting dùng, cho analytics) | `m-management/alerting/cmd/...` |

Cộng thêm `cmd/<service>/main.go`: chỗ duy nhất ráp các điểm trên lại và đọc cấu hình (`Config`).

## 4. Thư viện metap để dùng (không viết lại)

| Cần | Dùng của metap | Đừng làm |
|---|---|---|
| Đọc/ghi dữ liệu | `state.Crud` (`crud.RecordBackend`), luôn truyền `rc` của người gọi | SQL, đọc bảng service khác |
| Đổi trạng thái | `Transition(...)` | `Update` thẳng field trạng thái |
| Gọi service khác (record) | `recordclient` | tự dựng HTTP |
| Gọi service khác (việc khác) | `httpx.InternalClient` + `httpx.RequireSecret` | tự kiểm secret |
| Gọi địa chỉ khách nhập | `safehttp.NewClient` | `http.Client{}` tự dựng |
| Client HTTP thường | `httpx.NewClient(timeout)` | `http.DefaultClient` |
| Lỗi | `crud.NewServiceErrorMsg`, mã ở `errcode`/`model` | tự viết JSON lỗi |
| Env | `kit/env`, một lần ở `cmd/` | `env.*` trong nghiệp vụ |
| Event | metap tự phát `<entity>.record.*`, `.workflow.transitioned` | publish RabbitMQ thẳng |

## 5. Management plane: một service, nhiều domain

Management plane chạy **một process** (`m-management/cmd/management-service`), portal gọi thẳng nó,
không có gateway. Mỗi domain là một package trong `m-management/`, cùng một khuôn:

| Domain | Thực thể của nó | Ứng dụng viết gì (ngoài `entities/`) |
|---|---|---|
| `zones` | zone, domain, policy, rule, IP list, account, version | guard (quota, field, domain, account, version, reference), logic DNS/origin/onboarding/preset/validate/export/rollback; `rules/` là logic thuần |
| `billing` | plan, subscription, yêu cầu đổi gói | logic chọn gói, duyệt đổi gói; `PlanSource` là thứ zones hỏi giới hạn |
| `scanning` | scan job, finding | action `run`, callback `findings` của scanner |
| `alerting` | alert policy, notification, incident, false positive | action `correlate`, `evaluate`, `test` |
| `analytics` | `waf.security_events` | read model trên Analytics plane (`ExternalEntities`) |
| (`m-bootstrap`) | (không) | grant theo persona, chạy một lần |

Các domain gọi nhau **trong process**: zones hỏi billing giới hạn gói (`PlanLookup`), kiểm tra tham chiếu
scan job/incident bằng chính backend. Dependency một chiều: `zones → billing`; không domain nào import
`cmd/`. Ba plane còn lại (control, edge, analytics) vẫn là process riêng.

Bên trong `zones/`, mỗi thư mục là một lớp, import một chiều từ trên xuống:

```
zones/                 nối dây: Config, RegisterActions, Guarded        (chỉ gọi xuống)
├── guard/             chuỗi decorator của RecordBackend, mỗi guard một file
├── presets/           áp bộ rule mẫu (ghi qua guard)
├── policy/            validate, export, usage, syncConfigState
├── netcheck/          mọi thứ gọi ra mạng: DNS, origin, hostname, SSRF
├── versions/          snapshot, ghi version, rollback
├── entities/          khai báo entity (không logic)
├── rules/             vòng trong: logic thuần
└── internal/jx/       đọc/dựng giá trị JSON của record
```

`guard` và `presets` dùng `versions`; `policy` dùng `versions`; `netcheck`, `entities`, `rules` không
dùng package nào trong số đó. Không có vòng import.

`zones/rules/` là **vòng trong**: chỉ stdlib, depguard (`waap/.golangci.yml`) cấm import metap, HTTP, DB.
`*/entities/` chỉ được import `metadata`, `permission`, `reconciler`, `model`.

Các domain khác cùng khuôn nhưng phẳng (chưa cần package con): `actions.go` là nối dây, `config.go` đọc
env, mỗi việc một file (`scanning`: `run.go`, `findings.go`; `alerting`: `correlate.go`, `evaluate.go`,
`notify.go`, `helpers.go`; `billing`: `plans.go`, `changes.go`).

## 6. Cách nhận biết trong code

- Dòng đầu mọi file: `// Layer: APP · <loại>`. Các loại: `entity`, `guard`, `declared-action logic`,
  `pure business logic`, `outbound call`, `wiring`.
- Trong `m-*/` **không có** file nào của metap: nếu thấy mình viết router, server, SQL, middleware
  xác thực hay phân quyền ở đây, đó là sai chỗ.
- `import` có `github.com/metap-org/metap/pkg/...` là chỗ **dùng** metap; không có file nào ở đây
  *định nghĩa lại* thứ metap đã có.

## 7. Còn chưa sạch (nói thẳng)

- `billing` còn SQL trực tiếp vào `control.tenants` (lấy tên và danh sách tenant): thiếu một API
  thư mục tenant ở metap. Đây là chỗ ranh giới đang bị phá.
- Action chưa có transaction để phát event tuỳ biến (xem cookbook mục 6).
