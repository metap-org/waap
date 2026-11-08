# 05 — Ánh xạ kỹ thuật sang `metap`

Bước tiếp theo của `02-domain-model.md` (business-level) — chuyển sang `EntityDefinition`/
`EntityWorkflow`/`PolicyRow` cụ thể của `../metap` (`crates/metap-metadata`, `metap-workflow`,
`metap-permission`, `metap-cron`), dựa trên struct thật đã đọc trong mã nguồn `../metap`
(không phải suy đoán từ tài liệu). Mục tiêu: đủ cụ thể để bắt đầu viết `*_entity.rs`, không phải
tài liệu nghiệp vụ nữa.

## Nguyên tắc chung

- Entity author bằng code (`*_entity.rs`, theo mẫu `apps/jira-server`), **không** dùng đường
  low-code do DB tự author — đường đó còn ở giai đoạn sớm hơn (`docs/low-code-platform-v1.md`
  gọi là "exploratory"), workflow guard trên entity DB-authored mới bỏ skip serialize
  2026-08-17.
- `FieldKind` sẵn có: `Id, String, Number, Boolean, Date, Datetime, Money, Enum, Reference, Json`.
  Không có kiểu long-text hay multi-select riêng — mảng string (vd `channels`) dùng `Json`, đúng
  cách `jira.issues.labels` đã làm.
- Quan hệ N:1 là 1 field `kind: Reference` + `ref_entity` (+ `ref_display_field`); không có field
  1:N khai báo riêng — chiều "N" đọc qua list-view filter trên field reference phía kia (đúng cách
  `jira.issues.parentIssue` làm self-reference).
- Đa tenant: kế thừa cơ chế `metap` sẵn có (không khai báo `tenantId` như 1 `EntityField` thường).

## Entity → `EntityDefinition`

### `waf.zones`
| Field | Kind | Ghi chú |
|---|---|---|
| `hostname` | String, required, unique, indexed, searchable | |
| `originAddress` | String, required | |
| `status` | Enum(`pending`,`active`,`paused`,`suspended`) | `state_field` của workflow |
| `protectionMode` | Enum(`monitor`,`enforce`) | |
| `configVersion` | Number | |
| `hasConfig` | Boolean | **field kỹ thuật**, xem Workflow bên dưới |
| `verificationToken` | String | sinh tự động lúc tạo Zone — `06-onboarding-rules-lists.md` mục 2 |
| `verificationMethod` | Enum(`dnsTxt`,`httpFile`) | |
| `verificationStatus` | Enum(`unverified`,`verified`), indexed | |
| `dnsRoutingStatus` | Enum(`notRouted`,`routed`,`unknown`) | thuần hiển thị, không gate workflow — `11-onboarding-dns-resolution.md` |
| `lastDnsCheckAt` | Datetime, sortable | |

**Đã build + test qua API thật** (2026-08-30): `zone_entity.rs` khai đủ các field trên, guard
`activate` = `PolicyCondition::All([hasConfig eq true, verificationStatus eq "verified"])` — xác
nhận đúng hành vi qua request thật (chặn khi thiếu 1 trong 2, báo rõ điều kiện nào thiếu, pass khi
đủ cả 2).

### `waf.ddos_policies`
| Field | Kind | Ghi chú |
|---|---|---|
| `zoneId` | Reference → `waf.zones`, required, **unique** | `unique: true` chính là cách enforce "1 zone 1 policy hiệu lực" — không cần business rule riêng |
| `sensitivity` | Enum(`low`,`medium`,`high`,`aggressive`) | |
| `requestRateThreshold` | Number | |
| `burstWindow` | Number | |
| `action` | Enum(`log`,`challenge`,`block`) | |
| `enabled` | Boolean | |

### `waf.firewall_rules`
| Field | Kind | Ghi chú |
|---|---|---|
| `zoneId` | Reference → `waf.zones`, required, indexed | |
| `name` | String, required | |
| `priority` | Number, sortable | |
| `matchCondition` | **Json** | grammar riêng, xem mục "matchCondition" bên dưới — **không** tái dùng `PolicyCondition` type |
| `ruleType` | Enum(`waf`,`rateLimit`,`ipFirewall`,`geoFirewall`) | |
| `rateLimitThreshold` | Number, optional | |
| `rateLimitWindow` | Number, optional | |
| `action` | Enum(`allow`,`block`,`challenge`,`log`) | |
| `enabled` | Boolean | |

### `waf.scan_jobs`
| Field | Kind | Ghi chú |
|---|---|---|
| `zoneId` | Reference → `waf.zones`, required | |
| `scanType` | Enum(`quickScan`,`fullScan`,`apiScan`) | |
| `schedule` | String, optional | cron expression — **không** tự sync, xem mục metap-cron |
| `status` | Enum(`idle`,`queued`,`running`,`completed`,`failed`) | `state_field`, `terminal_states: []` (job lặp lại, không state nào thật sự cuối) |
| `lastRunAt` | Datetime, optional | |

Workflow: `run: idle→queued`, `run: completed→queued`, `run: failed→queued`, `start:
queued→running`, `complete: running→completed`, `fail: running→failed`. **Đã build + test qua
API thật** (2026-08-30): chạy hết vòng `idle→queued→running→completed→queued` (lần 2) — xác nhận
lại đúng phát hiện ở `08` quyết định #1 (`terminal_states` không chặn gì, `completed→queued` chạy
bình thường dù `completed` "nghe" như terminal).

### `waf.scan_findings`
| Field | Kind | Ghi chú |
|---|---|---|
| `scanJobId` | Reference → `waf.scan_jobs`, required | |
| `severity` | Enum(`info`,`low`,`medium`,`high`,`critical`) | |
| `category` | String | không enum cố định, đúng ý docs 02 |
| `endpoint` | String | |
| `description` | String | không giới hạn `max_length` |
| `remediationStatus` | Enum(`open`,`confirmed`,`falsePositive`,`fixed`,`accepted`) | `state_field`, terminal: `fixed`,`falsePositive`,`accepted` |
| `firstSeenAt` / `lastSeenAt` | Datetime | cập nhật `lastSeenAt` là ghi field thường, không qua transition |

**Đã build + test qua API thật**: tạo finding → `confirm` (open→confirmed) → `markFixed`
(confirmed→fixed), list lọc theo `scanJobId` trả về đúng kèm `relatedDisplay.scanJobId` (display
field của Reference, tự động).

### `waf.security_events`
| Field | Kind | Ghi chú |
|---|---|---|
| `zoneId` | Reference → `waf.zones`, required, indexed | |
| `triggeredBy` | Enum(`ddosPolicy`,`firewallRule`) | |
| `triggeredById` | **String** (không phải `Reference`) | `Reference` chỉ trỏ 1 `ref_entity` cố định — không polymorphic được sang 2 entity khác nhau tuỳ `triggeredBy`. Đây là điểm khác biệt so với docs 02 (ngầm coi là FK) |
| `triggeredByName` | String | denormalize tên rule/policy ngay lúc ghi event — bắt buộc vì volume lớn, tránh N+1 lookup lúc portal hiện list (`08` quyết định #4) |
| `action` | Enum(`logged`,`challenged`,`blocked`) | |
| `sourceIp` | String, indexed | |
| `requestPath` | String | |
| `occurredAt` | Datetime, indexed, sortable | |

**Table-per-entity**: cơ chế có thật, code-complete. ~~Trước đó ghi "chưa có orchestrator đa
tenant" — đã lỗi thời.~~ `docs/roadmap/44-reconciler-orchestrator-service.md` xác nhận
`reconciler-orchestrator` **đã Done** (2026-08-27, đóng nốt gap 2026-08-28): crate riêng chạy
thật, `run_tick` fan-out cả tenant kiểu `Schema` lẫn `DedicatedDb`, topo-sort theo FK, có API
`POST /platform/reconciler/wave-rollout`. Vậy cho `waf.security_events`: **không cần** copy cách
gọi `reconcile()` thủ công tại boot như `jira-server` (cách đó có trước orchestrator, giờ là cách
cũ) — nên gọi qua orchestrator (`enqueue_deployment`/API `wave-rollout`) để tự động chạy đúng theo
từng tenant, kể cả tenant mới thêm sau. Không còn là điểm cần MR/giới hạn phải chấp nhận.

### `waf.incidents`
| Field | Kind | Ghi chú |
|---|---|---|
| `zoneId` | Reference → `waf.zones`, required | |
| `title` | String | |
| `severity` | Enum(`low`,`medium`,`high`,`critical`) | |
| `status` | Enum(`open`,`acknowledged`,`mitigating`,`resolved`) | `state_field`, terminal: `resolved` |
| `eventCount` | Number | |
| `assignedTo` | Reference → user entity (`metap` control), optional | |

### `waf.alert_policies`
| Field | Kind | Ghi chú |
|---|---|---|
| `name` | String, required | |
| `thresholdCount` | Number | **tách khỏi `condition` tự do** — cần structured field để 1 cron job đọc và so sánh được, xem bên dưới |
| `windowMinutes` | Number | |
| `channels` | Json (mảng `"email"`/`"webhook"`) | |
| `enabled` | Boolean | |

### `waf.alert_notifications`
| Field | Kind | Ghi chú |
|---|---|---|
| `alertPolicyId` | Reference → `waf.alert_policies`, required | |
| `triggeredAt` | Datetime | |
| `channel` | Enum(`email`,`webhook`) | |
| `deliveryStatus` | Enum(`sent`,`failed`) | |

## Workflow (`EntityWorkflow`)

### `waf.zones`
```
state_field: "status", initial_state: "pending", terminal_states: ["suspended"]
transitions:
  activate: pending → active, guard: All([
    Attribute{attribute:"hasConfig", op:Eq, value:true},
    Attribute{attribute:"verificationStatus", op:Eq, value:"verified"},
  ])
  pause:    active  → paused
  resume:   paused  → active
  suspend:  active → suspended
  suspend:  paused  → suspended   (2 transition riêng cùng action "suspend", WorkflowTransition.from
                                    chỉ nhận 1 state — không phải union; permission-gated: chỉ
                                    Platform Admin, không phải guard)
```
**Vấn đề đã xác nhận qua code**: `PolicyCondition` (grammar dùng cho `guard`) chỉ so sánh 1
attribute path (tối đa 1 hop quan hệ) với 1 literal — **không có toán tử đếm/aggregate** trên tập
`DdosPolicy`/`FirewallRule` liên quan. Guard "activate khi đã có ≥1 policy hoặc rule" **không thể**
viết trực tiếp bằng `PolicyCondition`. Giải pháp: thêm field kỹ thuật `hasConfig: Boolean` trên
`Zone`, cập nhật (true/false) bằng app logic mỗi khi 1 `DdosPolicy`/`FirewallRule` được tạo/xoá cho
zone đó (hook ở `CrudService` layer hoặc app-level, không phải declarative), rồi guard so field đó.
`verificationStatus` cùng cơ chế — cập nhật bằng app logic sau khi job/nút "Verify now" xác nhận
DNS TXT/HTTP file challenge thành công (`06-onboarding-rules-lists.md` mục 2), không phải
declarative.

**Đã build + test qua API thật** (2026-08-30): tạo Zone → activate bị chặn (báo đúng thiếu
`hasConfig`) → set `hasConfig=true` → activate vẫn bị chặn (báo đúng thiếu `verificationStatus`)
→ set `verificationStatus="verified"` → activate thành công, `status: pending → active`. `All`
combinator hoạt động đúng như tài liệu `metap-permission` mô tả, báo lỗi rõ từng điều kiện.

### `waf.scan_findings`
```
state_field: "remediationStatus", initial_state: "open"
terminal_states: ["fixed", "falsePositive", "accepted"]
transitions: open → confirmed → fixed | open → falsePositive | open → accepted
```

### `waf.incidents`
```
state_field: "status", initial_state: "open", terminal_states: ["resolved"]
transitions: open → acknowledged → mitigating → resolved
```

**Đã kiểm tra lại, đảo ngược kết luận trước**: `terminal_states` **không được enforce ở runtime**.
Đọc `crates/metap-workflow/src/lib.rs` + call site `crates/metap-crud/src/crud_service/
transition.rs:83` — `find_transition()` chỉ match theo `(action, from_state)` trong danh sách
`transitions` khai báo, không hề đọc `terminal_states`. Field này chỉ được `metap-metadata/
compiler.rs` validate tĩnh lúc build metadata (tên state không rỗng, có trong `enumValues`) — thuần
mô tả (UI hint), không chặn gì. Vậy `waf.scan_jobs.status` dùng `EntityWorkflow` chuẩn bình thường,
khai `terminal_states: ["completed","failed"]` (đúng ý nghĩa "1 lần chạy dừng ở đây") **và** khai
transition `completed → queued`/`failed → queued` cho lần chạy tiếp theo — cả hai cùng lúc, không
mâu thuẫn, không cần patch gì.

## Permission (`PolicyRow`, `metap-permission`)

Default-deny (đã xác nhận qua code) — chỉ cần khai *Allow* rõ ràng cho từng role, không cần khai
Deny trừ khi phải chặn 1 Allow rộng hơn đã có.

Ví dụ minh hoạ đúng ma trận ở `03-personas-workflows.md`:
```
{ entity: "waf.scan_findings", action: "update", field: "remediationStatus",
  roles: ["developer"], effect: Allow }
# Developer không có Allow row nào cho waf.ddos_policies / waf.firewall_rules → default-deny lo phần đó

{ entity: "waf.zones", action: "delete", roles: ["soc_analyst"], effect: Deny }
# SOC có thể có Allow rộng ("update" mọi field) nhưng cần Deny riêng cho action delete

{ entity: "waf.incidents", action: "*", roles: ["viewer"], effect: Deny }
{ entity: "waf.incidents", action: "read", roles: ["viewer"], effect: Allow }
```
(Cú pháp minh hoạ ý tưởng — field/action string thật cần khớp đúng theo `metap-permission` khi
code, không copy nguyên văn.)

## `matchCondition` của `FirewallRule` — quyết định

**Không tái dùng type `PolicyCondition`.** Lý do xác nhận qua code: resolver của
`PolicyCondition` chỉ đi qua object JSON của 1 `record`/`RequestContext`, không có khái niệm
namespace `uri.*`/`header.*`/`body.*`, và thiếu toán tử WAF cần (regex, CIDR, contains/startswith)
— chỉ có `Eq/Neq/In/NotIn/Gt/Gte/Lt/Lte`. Ép field text như `"uri.path"` vào có thể "chạy" (vì
resolver chỉ tách dấu chấm) nhưng là dùng sai domain — trộn lẫn "permission decision" với "traffic
match", 2 khái niệm `metap-permission` cố tình tách.

**Khuyến nghị**: tự định nghĩa 1 grammar riêng cho `matchCondition`, học theo *hình dạng* JSON của
`PolicyCondition` (`All`/`Any` combinator lồng nhau) vì hình dạng đó đã chứng minh dùng được, nhưng
với field namespace và operator set riêng của request-matching (`uri.path`, `header.<name>`,
`body.<jsonpath>`, `sourceIp`; operator thêm `Contains`, `StartsWith`, `Regex`, `CidrMatch`). Lưu
field này ở kiểu `Json`, không có validation cấp `EntityDefinition` — validate ở tầng app khi
save/evaluate.

## `metap-cron` cho `ScanJob.schedule`

Xác nhận: `CronJob` là 1 row DB độc lập (`cron_expr: Option<String>` theo từng row, không cố định
theo definition) — khớp đúng nhu cầu "mỗi `ScanJob` tự có `schedule` riêng". Nhưng **không tự
sync** — `cron_jobs` là bảng ops riêng, tách khỏi `records` generic. App logic phải tự gọi
`metap_cron::store::create_job`/`update_job`/`delete_job` mỗi khi 1 `ScanJob` record được tạo/sửa/
xoá field `schedule` (qua hook ở `CrudService` layer hoặc listener trên outbox event
`waf.scan_jobs.record.updated`).

`trigger_type: OnRecordEvent` (xác nhận có thật) là điểm đáng chú ý thêm — dùng được cho 2 chỗ
khác trong domain model không phải scheduling:
- **Incident correlation**: job `trigger_type: OnRecordEvent` trên `waf.security_events.created`,
  `target_type: Steps` chạy logic gộp (đọc N event gần nhất theo zone+khung giờ, tạo/update
  `waf.incidents` qua `CrudService`). Giải quyết câu hỏi "cần 1 job phân tích" mà `02-domain-model.md`
  để ngỏ — có, và có sẵn cơ chế trigger đúng nhu cầu, không cần tự viết listener riêng.
- **AlertNotification gửi thật**: `target_type: Webhook`/`Email` là 2 target có sẵn (không phải chỉ
  `WorkflowTransition`) — 1 job `OnRecordEvent` trên `waf.incidents.created` (hoặc theo threshold,
  `Schedule`) có thể **gửi alert trực tiếp** không cần code worker riêng, miễn `AlertPolicy` được
  đọc để quyết định channel/threshold trong bước dựng job đó.

## `SecurityEvent` — hướng ghi từ edge-plane (vẫn 1 phần chưa chốt, nhưng thu hẹp lại)

Xác nhận: gRPC `RecordService.Create` (generic, entity-agnostic — `CreateRequest{entity_name,
google.protobuf.Struct data}`) **đã có thật**, không phải giả định trong docs 04. Cả 2 hướng
(edge gọi thẳng, hay qua `control-plane`) **đều gọi cùng 1 endpoint này ở phía `management-plane`** —
nghĩa là phần việc của `management-plane` (đăng ký `waf.security_events` là `EntityDefinition`, bật
table-per-entity, expose `RecordService`) **giống hệt nhau dù chọn hướng nào**. Quyết định
"ai gọi" (edge trực tiếp hay qua control-plane) là quyết định của `control-plane`/`edge-plane`,
không chặn việc bắt đầu code `management-plane` — vẫn để `04-architecture-boundary.md` giữ nguyên là
điểm chưa chốt, nhưng không còn là blocker cho `management-plane`.

## MR backlog vào `metap`

Đào sâu 5 điểm nghi ngờ là gap của `metap` (đọc mã nguồn `metap-workflow`, `metap-permission`,
`metap-cron`, `metap-reconciler`, và grep `docs/roadmap*`) — chỉ 2/5 thật sự là gap đáng đề xuất
MR, phần còn lại hoặc không phải gap, hoặc nên giữ workaround ở app thay vì đổi platform.

| # | Điểm | Kết luận | Đề xuất |
|---|---|---|---|
| 1 | `terminal_states` chặn transition | **Không phải gap** — field này chỉ mô tả, không enforce runtime (xem mục Workflow ở trên) | Không cần MR, đã sửa lại cách dùng ở doc này |
| 2a | `PolicyCondition` thiếu operator (`Contains`/`StartsWith`/`Regex`/`CidrMatch`) | Gap thật, nhưng sửa **nhỏ** — `evaluate_condition` nhận thẳng `subject: &serde_json::Value` thuần in-memory, không cần DB; thêm operator chỉ là thêm arm cho `ConditionOp` + `match_operator` | **Nên đề xuất MR** — hữu ích chung (không riêng WAF, vd điều kiện permission "email chứa domain"), không đụng kiến trúc |
| 2b | `PolicyCondition` thiếu `Count`/`Exists` trên reverse-relation (để guard "Zone có ≥1 policy" không cần field `hasConfig` đệm) | Gap thật, sửa **vừa** — cross-record hiện tại (`CrudService::enrich_record_for_actions`) chỉ resolve 1 hop thuận (Reference→cha), không có index reverse-relation nào; cần thêm bước pre-fetch mới trước evaluate (không sửa `evaluate_condition`), chạm `crud_service.rs` + `metap-metadata` + `metap-permission` | **Cân nhắc**, không gấp — giữ workaround `hasConfig: Boolean` cho v1, đề xuất MR này sau nếu có ≥2 chỗ khác trong sản phẩm cũng cần đếm quan hệ (đủ trigger để đáng làm chung) |
| 3 | Polymorphic `Reference` (field trỏ entity khác tuỳ giá trị field cùng record, cho `SecurityEvent.triggeredById`) | **Gap mới, chưa ai note** — roadmap chỉ có "entity variant" (khác hẳn, discriminated-union trong 1 collection), không phải polymorphic FK | Đáng viết feature brief, nhưng **rủi ro vừa–lớn** (đụng `metap-metadata`, `enrich_record_for_actions`, OpenAPI/codegen, FE generated-types) — v1 giữ workaround `triggeredById: String` không FK, không MR ngay |
| 4 | Field-driven cron sync (metadata tự đồng bộ `CronJob` theo field lịch trên entity) | Gap thật nhưng **đi ngược nguyên tắc kiến trúc đã chốt** của metap ("không `metap-*` crate nào biết business entity") — doc-comment `metap-cron/src/lib.rs` xác nhận model chốt là "operator tự tạo job qua admin API" | **Không đề xuất MR** — khả năng bị từ chối cao vì phá nguyên tắc cố ý; giữ hẳn workaround app-level gọi `create_job`/`update_job` |
| 5 | Orchestrator đa tenant cho table-per-entity | **Không phải gap** — `reconciler-orchestrator` đã Done (2026-08-27/28), có API `wave-rollout` | Không cần MR, chỉ cần management-plane gọi đúng API thay vì tự `reconcile()` thủ công như `jira-server` (cách cũ, có trước orchestrator) |

**Tóm lại**: chỉ 1 MR nên làm sớm (2a — mở rộng operator, nhỏ, rủi ro thấp) nếu muốn tận dụng lại
`PolicyCondition`-shape cho việc khác ngoài WAF; `matchCondition` của `FirewallRule` vẫn tự viết
type riêng như đã chốt ở trên (không phụ thuộc MR 2a). 2b và 3 để dành nghiên cứu kỹ hơn khi có
nhu cầu thật rõ hơn (không phải WAF là use-case duy nhất). 4 không nên theo đuổi.

## Thứ tự build đề xuất

0. ✅ **Done** — scaffold từ `templates/metap-app` (local path dep vào `../../metap/crates/metap`,
   xem `management-plane/README.md`).
1. ✅ **Done, tested qua API thật** — `waf.zones` (+ verification/DNS fields) + `waf.ddos_policies`
   + `waf.firewall_rules` + workflow (guard `All(hasConfig, verificationStatus)`).
2. ✅ **Done, tested qua API thật** (2026-08-30) — `waf.scan_jobs` (workflow lặp
   `idle↔queued↔running↔completed/failed`) + `waf.scan_findings` (workflow remediation). **Chưa
   làm**: wiring `metap-cron` cho `schedule` thật (field có, chưa tự sync `CronJob`), permission
   policy cho role (đang test bằng admin bypass permission).
3. ✅ **Done, tested qua API thật** (2026-08-30) — `waf.security_events` (không workflow, để
   `table_name: "records"` chung — chưa bật table-per-entity, volume demo còn thấp) +
   `waf.incidents` (workflow `open→acknowledged→mitigating→resolved`, test hết cả chuỗi). **Chưa
   làm**: job correlation thật (`OnRecordEvent`, gộp `SecurityEvent` → `Incident` tự động) — ngoài
   phạm vi portal/management-plane, xem `13-screen-api-map.md`.
4. ✅ **Done, tested qua API thật** (2026-08-30) — `waf.alert_policies` + `waf.alert_notifications`.
   **Chưa làm**: job gửi alert thật (`target_type: Email|Webhook`, `metap-cron`), nút test alert.
5. Analytics dashboard trên `waf.security_events` — cần thiết kế pre-aggregation riêng (docs 03 đã
   ghi nhận, không giải quyết ở `EntityDefinition` cấp cơ bản) — để sau, không chặn 4 pillar chính.

**Cả 9 entity chính (pillar 1-4) đã đăng ký + build sạch + test qua API thật** — xác nhận qua
`GET /metadata/openapi.json`: `waf.zones`, `waf.ddos_policies`, `waf.firewall_rules`,
`waf.scan_jobs`, `waf.scan_findings`, `waf.security_events`, `waf.incidents`,
`waf.alert_policies`, `waf.alert_notifications`. Còn lại là phần **Custom** (không phải CRUD) đã
liệt ở `13-screen-api-map.md` — DNS/verify, dashboard aggregate, scan engine thật, correlation
logic, gửi alert thật, access control theo domain, quota/billing.
