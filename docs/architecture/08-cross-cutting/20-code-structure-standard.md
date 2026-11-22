# Tiêu chuẩn viết code trên metap (BẢN NHÁP, chưa áp dụng)

> Đọc trước: `21-metap-vs-app-map.md` (phần nào của metap, phần nào của ứng dụng).

Trạng thái: **phase 1 đã duyệt (2026-11-22)**, chủ dự án sẽ review tiếp qua nhiều phase sau,
nên nội dung còn có thể đổi. Chưa refactor gì, chưa viết cookbook. Khi chốt hẳn thì chuyển thành
ADR và chuyển phần "cách làm" sang `metap-docs` (metap là nơi giữ cookbook, waap chỉ dẫn link).
Ngày ghi: 2026-11-22.

## 1. Nguyên tắc (theo chủ dự án)

Có ba tầng tái sử dụng, mỗi tầng một việc:

| Tầng | Chứa gì | Ai được dùng |
|---|---|---|
| `metap/pkg/kit/*` (env, httpx, errcode, standard, shutdown), `pkg/runtime` | Tiện ích thuần, không biết nghiệp vụ, không biết metadata/DB | Mọi Go service, kể cả `e-edge`, `a-api` |
| Nền tảng metap: `metadata`, `crud`, `permission`, `workflow`, `events`, `bootstrap`, `app`, `httpserver`, `recordclient` | Entity, router CRUD, workflow, phân quyền, outbox/event | Mọi service quản trị (`m-*`, `c-*`) |
| Sản phẩm (waap): `entities/`, hành động, guard, logic thuần | **Chỉ khai báo và điền nghiệp vụ** vào các điểm mở của nền tảng | |

Hệ quả: nghiệp vụ ngoài CRUD (onboarding, plan, rollback, export...) vẫn phải đi qua **cùng**
router, **cùng** kiểu handler, **cùng** repository, **cùng** phân quyền như CRUD. Không có
"đường riêng" cho code mới.

Đề xuất phân lớp transport/service/port/adapter ở bản trước bị bỏ: metap đã là lớp
transport (router, auth, lỗi) và là lớp repository (`crud.RecordBackend`). Dựng thêm một bộ lớp
song song chỉ làm loạn thêm.

## 2. Một cách duy nhất cho từng việc

Mỗi mục: **cách chuẩn** (điểm mở có sẵn của metap), **cấm**.

### 2.1 Entity
- Khai báo bằng `metadata.EntityDefinition` + `metadata.SubmitEntity` trong `init()` cạnh định
  nghĩa, trong `<service>/entities/`. Tên entity, field, action, state nằm ở `model/` (không gõ lại
  chuỗi).
- Cấm: tạo bảng/migration tay cho entity; field trùng tên cột hệ thống.

### 2.2 Router / endpoint
- Mọi thao tác do người dùng gọi: **action khai báo** trên entity (`EntityDefinition.Actions`) +
  `RegisterActions`, phục vụ ở `POST /api/{entity}[/{id}]/actions/{action}`. CRUD do metap lo.
- Route tự viết (`Options.Routes`/`MountRoutes`) **chỉ** cho người gọi không phải người dùng:
  `/internal/*` giữa service, webhook, ingest, health. Không gắn quyền entity vào đó (xem 2.8).
- Cấm: route REST song song cho một action đã khai báo; tự dựng `chi.NewRouter`/`ServeMux`.

### 2.3 Handler
- Một chữ ký: `crud.ActionHandler func(ctx, crud.ActionCall) (crud.JsonObject, error)`.
  Handler mỏng: đọc `call.Input`, gọi **hàm nghiệp vụ**, trả `crud.ToObject(kết quả)`.
- Hàm nghiệp vụ là hàm thường `(ctx, backend, rc, tham số) (kết quả, error)`, test được không cần
  HTTP. Handler không chứa `if` nghiệp vụ.
- Lỗi: `crud.ServiceError` + mã trong `kit/errcode` hoặc `model`; lỗi khác là 500. Cấm viết
  `{"error":...}` bằng tay.
- Cấm: handler dựng `GuardedZonesBackend`, client, đọc env mỗi lần gọi (đưa vào dây nối ở `cmd`).

### 2.4 Repository (truy cập dữ liệu)
- Repository chính là `crud.RecordBackend` (`state.Crud`, hoặc bản đã bọc), **luôn** gọi với
  `rc` của người gọi, nên quyền của họ giới hạn việc đọc/ghi.
- Cần quyền cao hơn (ví dụ ghi version): dùng bản sao ngữ cảnh admin theo mẫu có sẵn, ghi rõ lý do
  ở chỗ gọi, không tự thêm đường tắt.
- Dữ liệu của service khác: `recordclient` (REST, chuyển tiếp token người gọi).
- Cấm: SQL/`pgx` trực tiếp trong service sản phẩm; đọc bảng của service khác.

### 2.5 Ràng buộc, kiểm tra, side-effect khi ghi (guard)
- Một điểm mở: `Options.WrapBackend` (decorator của `RecordBackend`). Chỉ **một** chuỗi decorator
  cho mỗi service, mỗi decorator một việc (validate, quota, version, account), ghép ở `cmd`.
- Cấm: guard nằm rải trong handler; một "backend khổng lồ" gom mọi thứ như `GuardedZonesBackend`
  hiện tại.

### 2.6 Phân quyền
- Quyền cấp bằng grant khai báo: `bootstrap.Submit` (role persona) + policy ABAC
  (`PolicyCondition`) + vai trò theo phạm vi (scoped role, dùng cho account). Admin có toàn quyền
  trong tenant của mình.
- Guard theo trạng thái dùng thuộc tính workflow (`permission.WorkflowAttributePrefix`).
- Cấm: kiểm tra `role == "..."` trong handler; guard quyền tự viết ngoài `permission`. Cấm dùng
  `platform_admin` cho tính năng của waap (chỉ lớp multi-tenant của metap).

### 2.7 Workflow
- Khai báo trong `EntityDefinition.Workflow` (state field + `metadata.Transition`). Đổi trạng thái
  **chỉ** qua `backend.Transition(...)` hoặc action chuyển trạng thái; metap kiểm guard, quyền,
  ghi audit và phát `<entity>.workflow.transitioned`.
- Cấm: ghi thẳng field trạng thái bằng `Update`; tự phát event chuyển trạng thái.

### 2.8 Event
- Phát: `events.Emit(ctx, tx, events.Event{Topic:"billing.plan.changed",...})` trong **cùng
  transaction** với ghi dữ liệu (outbox). Topic dạng `service.thực_thể.sự_kiện`, payload là struct
  có tag json và có version.
- Nhận: consumer đăng ký subscribe RabbitMQ theo một mẫu duy nhất (hiện có ở
  `c-config-distributor/subscribe`), luôn kèm resync định kỳ làm đảm bảo hội tụ.
- Cấm: publish thẳng RabbitMQ; phát event ngoài transaction ghi.

### 2.9 Gọi ra ngoài (HTTP, DNS, mail)
- Client HTTP: `httpx.NewClient(timeout)`; cấm `http.DefaultClient` và tự dựng `&http.Client{}`.
- Gọi service khác về record: `recordclient`. Gọi service khác về việc không phải record (ví dụ
  `plans/resolve`): một client nội bộ dùng chung, xác thực bằng token dịch vụ/secret theo mẫu
  `httpx` có sẵn.
- Địa chỉ đích do người dùng nhập (origin, hostname): qua client chống SSRF dùng chung.

### 2.10 Cấu hình và dây nối
- Đọc env **một lần** ở `cmd/` bằng `kit/env`, gom vào một struct `Config`, truyền xuống. Cấm gọi
  `env.Lookup` trong hàm nghiệp vụ.
- `cmd/<service>/main.go` chỉ dựng `app.Options{Name, Actions, WrapBackend, Routes}` rồi
  `app.Main`. Không logic.

## 3. Hiện trạng lệch chuẩn (bằng chứng trong code)

| # | Hiện trạng | Mục chuẩn |
|---|---|---|
| 1 | Cùng một action có **hai** đường: route REST ở `MountRoutes` (`/api/waf.zones/{id}/verify-dns`...) và `RegisterActions` | 2.2 |
| 2 | Ba kiểu handler cùng tồn tại: `RecordActionRoute`, handler `chi` thô (tự `RequireAuth`, parse id), `crud.ActionHandler` | 2.3 |
| 3 | `applyPreset` dựng `NewGuardedZonesBackend(...).WithPlanLimits(PlanClientFromEnv())` **trong handler**, mỗi lần gọi | 2.3, 2.10 |
| 4 | `GuardedZonesBackend` (390 dòng) gom validate + quota + version + account + chuẩn hoá + kiểm tra tham chiếu chéo service | 2.5 |
| 5 | Đọc env rải rác: `scanningURL()`, `alertingURL()`, `dohURL()`, `edgeCNAMETarget()`, `PlanClientFromEnv()`, `PlanSourceFromEnv()`, `BILLING_INTERNAL_SECRET` đọc trong handler | 2.10 |
| 6 | Client HTTP tự dựng ở 5 nơi: `m-management/zones/routes.go` (nay là `checks.go`), `outbound.go`, `plans.go`, `m-alerting`, `m-scanning` (chỉ `c-config-distributor` dùng `httpx.DefaultClient`) | 2.9 |
| 7 | Gọi service khác hai kiểu: `recordclient` (scanning/alerting) và `PlanClient` tự viết (billing) | 2.9 |
| 8 | `m-billing` tự kiểm secret bằng `SecretEqual` trong từng route thay vì một middleware nội bộ dùng chung | 2.2, 2.9 |
| 9 | Event tuỳ biến: waap chưa dùng `events.Emit`. Hoá ra không cần: metap đã phát event record/workflow; custom emit cần tx mà action chưa có | 2.8 |
| 10 | Workflow: chỉ `m-scanning` gọi `Transition` bằng code; chỗ khác đổi trạng thái qua action riêng | 2.7 |
| 11 | Một file làm nhiều lớp: `routes.go` (đã tách thành `actions.go` = nối dây và `checks.go` = logic) chứa DoH lookup, helper env, handler, đăng ký action | 2.3 |
| 12 | metap: tài liệu "cách làm" cho 2.1-2.10 không có một chỗ tập trung, mỗi service tự suy ra từ code | mục 5 |

### Đã làm trên nhánh `refactor/code-standard` (chờ review, chưa commit; waap và metap cùng tên nhánh)

Chỉ gom về cách chuẩn, không đổi hành vi; test đơn vị và tích hợp của m-zones/m-billing/m-scanning/
m-alerting và `pkg/kit` của metap đều xanh.
- #1: bỏ route REST trùng action ở zones, scanning, alerting; bỏ luôn hai route `/internal` của
  alerting trùng action `correlate`/`evaluate` (không có người gọi nào khác). Test gọi action qua
  `crud.ActionRegistry`. Còn lại `/internal/*` chỉ cho người gọi là dịch vụ: `plans/resolve`,
  callback của scanner, health sâu của zones.
- #2, #11: chỉ còn hai kiểu: action (`crud.ActionHandler`) và route `/internal` cho dịch vụ.
  `routes.go` của zones không còn route REST, chỉ còn handler action và hàm nghiệp vụ.
- #3, #5: `Config` đọc env một lần ở `cmd/`; `cfg.Guarded(...)` là chỗ duy nhất dựng backend có guard.
- #4: `GuardedZonesBackend` tách thành chuỗi decorator trong `m-management/zones/guard/` (một file mỗi guard: quota → field
  → domain → account → version, và reference cho delete), mỗi guard một việc, một danh sách thứ tự.
- #6, #7, #8 và mục 4 (đã đẩy lên metap, `pkg/kit`): `httpx.RequireSecret`, `httpx.InternalClient`
  (billing và `PlanClient` dùng), `safehttp.NewClient` (client chống SSRF của m-zones), cùng
  `httpx.NewClient` cho mọi client còn lại.
- #9: **không thêm event tuỳ biến.** Metap đã phát `<entity>.record.*` và
  `<entity>.workflow.transitioned` cho mọi entity cùng transaction, đủ cho đổi plan, rollback;
  còn `events.Emit` cần `tx` mà action chưa có. Ghi thành khoảng trống của metap (cookbook mục 6).
- #10: entity có workflow (zone, scan job, incident, finding, account, false positive) không bị ghi
  thẳng field trạng thái (đã grep). Ngoại lệ: `waf.plan_change_requests.status` là enum thường do code
  đặt (pending/approved/rejected), nên khai báo thành workflow; để sau vì đổi hành vi.
- #12: cookbook ở `metap-docs/docs/guides/01-xay-dung-tren-metap.md` (nháp, 9 bài, mẫu là code thật).
- Gộp management plane (2026-11-22, theo chủ dự án): zones/scanning/alerting/billing thành **một** process
  `m-management/cmd/management-service`, bỏ gateway (`m-gateway`); thư mục `m-management/{zones,scanning,
  alerting,billing,analytics}`, package đổi tên theo thư mục. Các lời gọi REST giữa service thành gọi trong
  process (`PlanLookup`, kiểm tra tham chiếu qua backend); bỏ `/internal/plans/resolve`, `BILLING_*`,
  `SCANNING_URL`, `ALERTING_URL`. Lưu ý: `?expand=` chỉ do gateway làm, portal waap không dùng.
- `zones/rules/` (hostname, apex, ip, match condition) là logic thuần, stdlib only; `waap/.golangci.yml` ép
  bằng depguard (cả `entities/` chỉ khai báo), có bước trong CI. Đã thử: thêm import metap vào `rules/` thì lint báo.
- Chưa làm: depguard cho phần còn lại (mục 7), phân tầng `pkg/` của metap (mục 6), API thư mục tenant của
  metap để billing hết SQL trực tiếp (xem cuối cookbook).

## 4. Chỗ nào nên đẩy lên metap

Những thứ waap tự viết nhưng mang tính chung, đưa lên đúng tầng (theo luật "waap cần thì sửa metap
cùng đợt, không thêm tính năng không liên quan"):

- Client chống SSRF (`zones/netcheck/outbound.go`): chung, stdlib-only → `kit`.
- Middleware/route nội bộ bằng secret dịch vụ (billing) + client tương ứng (`PlanClient`) → `httpx`
  hoặc `recordclient` mở rộng.
- Helper gọi action của service khác (hiện chưa có; mỗi nơi tự dựng request).
- Mẫu "chuỗi decorator của RecordBackend" (compose nhiều guard nhỏ) → `crud`.
- Mẫu "snapshot + hash + rollback" và "hết hạn" chỉ nên lên metap nếu có service thứ hai cần; hiện
  chỉ waap dùng nên giữ ở waap.

## 5. Tài liệu cần viết (đây là chỗ thiếu nhiều nhất)

Cookbook trong `metap-docs/docs/guides/`, mỗi bài là một mẫu chạy được, có đường dẫn tới code thật:

1. Thêm entity mới (khai báo, field, tham chiếu, unique, index).
2. Thêm action không phải CRUD (khai báo, handler mỏng, hàm nghiệp vụ, test).
3. Thêm guard khi ghi (một decorator của `RecordBackend`).
4. Thêm workflow và guard theo trạng thái; gọi `Transition` từ code.
5. Phân quyền: persona (`bootstrap.Submit`), ABAC, scoped role, quyền theo trạng thái.
6. Phát và nhận event (outbox, topic, payload, consumer, resync).
7. Gọi service khác (record và không phải record), gọi ra ngoài, chống SSRF.
8. Cấu hình và dây nối ở `cmd`.
9. Test: unit (hàm nghiệp vụ), tích hợp (`testkit`), cách đặt tên.

Kèm một service mẫu nhỏ làm chuẩn để sao chép.

## 6. Cấu trúc thư mục một service sản phẩm (nhẹ, bám nền tảng)

```
m-<service>/
  cmd/<service>/main.go   chỉ dây nối: Config → app.Options
  entities/               khai báo entity (đã có)
  actions.go              RegisterActions: bảng (entity, action, handler mỏng)
  <usecase>.go            hàm nghiệp vụ thuần, mỗi use case một file (versions, quota, onboarding)
  guards/                 mỗi decorator RecordBackend một file
  clients/                client tới service khác, dùng client chuẩn ở 2.9
  config.go               struct Config, đọc env một lần
```

Chỉ tách thành thư mục con khi một service vượt khoảng 15 file hoặc có hai nhóm use case độc lập.

## 7. Ép bằng máy

- depguard cho `m-*`/`c-*`: cấm `database/sql`, `pgx`, `net/http` Client dựng tay
  (`http.Client{`), `http.DefaultClient`, `amqp` ở package nghiệp vụ; chỉ cho `cmd/` và `clients/`.
- Linter tuỳ chỉnh hoặc test kiến trúc (một test duyệt AST): mỗi action khai báo phải có đúng một
  handler; không có route `/api/{entity}/...` ngoài action.
- Cùng cách cho metap: định nghĩa tầng `kit` < lõi dữ liệu < dịch vụ lõi < lắp ghép và đặt luật
  import một chiều (cần đọc `go list -deps` trước khi chốt, sẽ có chỗ vi phạm).

## 8. Thứ tự làm đề xuất (khi được duyệt)

1. Duyệt tài liệu này (bạn sửa).
2. Viết cookbook mục 5 (docs, không đụng code), dùng code waap hiện có làm ví dụ và ghi rõ
   chỗ nào lệch.
3. Đưa các phần mục 4 lên metap.
4. depguard chế độ cảnh báo để đếm vi phạm.
5. Chỉnh `m-zones` làm mẫu, chỉ gom về cách chuẩn, không đổi hành vi. Rồi `m-billing`,
   `m-alerting`, `m-scanning`, `c-config-distributor`.
6. metap: phân tầng `pkg/`.

## 9. Câu hỏi còn mở

- `GuardedZonesBackend` hiện gom nhiều guard: tách thành chuỗi decorator nhỏ có chấp nhận được
  về hiệu năng/độ phức tạp ghi (mỗi lần ghi qua nhiều lớp) không?
- Route REST cũ song song với action (mục 3 #1): xoá hẳn, hay giữ làm tương thích cho client cũ?
- Có đưa event tuỳ biến vào waap ngay (đổi plan, rollback, xoá workspace) hay chỉ khi có consumer?
- Frontend `m-web/src` cần chuẩn tương tự không (pages/components/api)?
