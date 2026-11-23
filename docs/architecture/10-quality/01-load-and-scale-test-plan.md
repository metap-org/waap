# 10.2 Kế hoạch test tải, stress, dữ liệu lớn: management service (BẢN NHÁP)

**Phạm vi hiện tại: chỉ management service** (`m-management`, một process, cổng 3000, Postgres qua metap).
Distributor, edge, analytics-api và đường end-to-end chưa chốt thiết kế nên **chưa lên kế hoạch**; những gì
đã thấy khi đọc mã của chúng được ghi ở §9 để không mất, không phải để làm ngay.

Mục tiêu: có số đo thật cho API quản lý ở quy mô dữ liệu lớn (hướng tới giả định 1.000.000 zone ở
[7.5](../07-deployment/05-capacity-and-sharding.md)), tìm nút thắt trước khi khách thật gặp, và có bộ test
chạy lại được để thấy hồi quy.

Số mục tiêu ở §3 do tôi đề xuất, chưa được duyệt: [10.1](00-index.md) chưa có số cho management plane.

## 1. Đối tượng đo

| Phần | Vì sao đáng đo |
|---|---|
| Đọc: list/get/`?ids=`/aggregate/`search`/`jql` trên `waf.zones`, `firewall_rules`, `ip_access_lists`, `policy_versions` | Bảng lớn nhất khi có 1M zone; mọi màn hình portal đi qua đây |
| Ghi qua chuỗi guard: quota → field → domain → account → version | Mỗi lần ghi chạy nhiều truy vấn phụ (đếm quota, snapshot version, kiểm tham chiếu) |
| Action: `exportData`, `planUsage`, `validatePolicy`, `applyPreset`, `rollback`, `verifyDns`/`probeOrigin` (có mạng ngoài) | Đọc cả tenant hoặc cả zone trong một lần gọi: nguy cơ RAM/thời gian theo kích thước |
| Audit + outbox | Chuỗi băm `FOR UPDATE` mỗi tenant; outbox tích backlog khi ghi nhanh |
| Phân quyền | Vai trò nạp từ DB ở mỗi request (không cache), policy theo entity/record/scope |
| Bộ giới hạn tốc độ + nhận diện người gọi | Phải đo cùng tải nhưng không để nó che số đo (xem §5) |

Đã có sẵn: bộ k6 của metap (`metap/testing/performance/k6`: seed, list, filter+sort, cursor) và
`seed_10m.sql`; chưa có `Benchmark*` nào trong repo waap.

## 2. Môi trường

| Bậc | Cấu hình | Dùng cho |
|---|---|---|
| Local | `m-deploy` compose, Postgres + RabbitMQ trên máy dev | Soạn script, T1, T2 ở 1k–10k zone |
| Perf cố định | 2 máy cùng phần cứng (tải / service + Postgres), NVMe, mạng 10 GbE, tắt đổi tần số | T2 từ 100k zone, T3, T4, T5 |

Chưa rõ có máy cố định không; nếu không, mọi số chỉ để so giữa các lần chạy, không so với mục tiêu tuyệt đối.
Postgres chỉnh gần production (không dùng cấu hình compose mặc định): `shared_buffers`, `max_connections`,
`pg_stat_statements` bật; ghi lại cấu hình trong báo cáo.

## 3. Mục tiêu đề xuất (cần chốt)

| Mã | Mục tiêu |
|---|---|
| P-MGMT-1 | `GET /api/waf.zones` (limit 50, cursor) ở 1M zone / 10k tenant: p95 ≤ 200 ms ở 200 rps |
| P-MGMT-2 | List có `search`/filter/`withTotal`: p95 ≤ 400 ms ở 100 rps |
| P-MGMT-3 | Tạo/sửa rule qua guard: p95 ≤ 250 ms ở 50 rps; tăng dữ liệu 10 lần không tăng p95 quá 2 lần |
| P-MGMT-4 | Get theo id và `?ids=` 50: p95 ≤ 50 ms ở 500 rps |
| P-DATA-1 | Không action nào giữ toàn bộ dữ liệu một tenant trong RAM: `exportData`/`planUsage`/`validatePolicy` ở 100k zone/tenant dùng ≤ 1 GB, hoặc trả lỗi có kiểm soát |
| P-WRITE-1 | Import 10.000 zone / 50.000 rule của một tenant: có số thông lượng và biết trần (§5 T3) |
| P-STRESS-1 | Quá tải thì load-shed có kiểm soát (429/503), không treo/OOM; phục hồi trong 60 s sau khi bỏ tải |

## 4. Dữ liệu lớn: cách sinh

Bắt chước phân bố ở 7.5 G4:

- **Phân khúc**: 65% Free, 15% Pro, 18% Business, 2% Enterprise. Số rule mỗi zone log-normal: Free 2–5, Pro 5–15,
  Business 20–60, Enterprise 100–1.000 (một ít zone tới 5.000). IP list 0–10.000 mục.
- **Tenant**: lệch kiểu Pareto (đa số 1–3 zone, ít tenant 10k+ zone) — quan trọng vì quota/export/usage tính theo tenant.
- **Ba tầng nạp**, mỗi tầng kiểm một thứ:
  1. *Qua API* (seeder Go dùng `recordclient`, hoặc k6): đúng guard/audit/outbox, chậm — 1k–10k zone, và để đo chính việc ghi.
  2. *`COPY`/`generate_series` thẳng vào bảng đã reconcile*: 100k–1M zone. Chạy reconcile trước để bảng do metap tạo; điền cả bảng audit và `policy_versions` để kích thước bảng và chỉ mục đúng thực tế.
  3. *Hỗn hợp*: nạp tầng 2 làm nền rồi đo ghi tầng 1 lên trên (đo ghi trên bảng đã lớn — đúng tình huống vận hành).
- Seed cố định; lưu thống kê phân bố thật sinh ra cạnh kết quả. Mỗi lần chạy một database riêng.

## 5. Các bài test

Thứ tự là thứ tự thực hiện.

### T1. Microbench (Go `testing.B`, chạy trong CI)
`zones/rules` (chuẩn hoá hostname, `matchCondition`), `policy.Validate` với 5.000 rule, `presets`, mã hoá/giải mã
snapshot trong `versions`, so khớp `guard` không đụng DB. Ra ns/op, B/op, allocs/op; so bằng `benchstat`,
ngưỡng hồi quy +20% làm đỏ CI.

### T2. Đường cong theo kích thước dữ liệu
- Dữ liệu 1k → 10k → 100k → 1M zone (tầng 1 rồi 2).
- k6 với **arrival-rate executor** (tải cố định theo rps, tránh coordinated omission): list + cursor, `withTotal`,
  filter theo `status`/`accountId`, `search` hostname, `jql`, get, `?ids=`, aggregate.
- Ghi: tạo zone, tạo/sửa rule (guard), transition, delete (kiểm tham chiếu).
- Đo p50/p95/p99, lỗi, CPU/RAM/kết nối DB, `pg_stat_statements` (câu nào tăng theo N), `EXPLAIN (ANALYZE, BUFFERS)` các câu top, kích thước bảng/chỉ mục.
- Giả thuyết cần kiểm:
  a. `withTotal` đếm trên bảng lớn là câu chậm nhất.
  b. Nạp vai trò ở mỗi request là trần thông lượng khi nhiều kết nối đồng thời.
  c. Quota `jx.Count` quét theo tenant; chậm ở tenant 100k zone.
  d. `versions` snapshot cả zone sau mỗi lần sửa: zone 1.000 rule thì một lần sửa thành 1.000 dòng ghi (khuếch đại ghi).
  e. Truy vấn `search` hostname dùng được chỉ mục trigram hay quét tuần tự.
  f. Phân quyền theo scope (`PlanAccess`) làm câu list phức tạp hơn đáng kể ở tenant có nhiều role giới hạn theo zone.
- **Rate limit**: bucket chung (200 ms/burst 300 mỗi IP) và bucket auth (burst 20, 1 lượt/3 s) sẽ chặn bài test
  chạy từ một IP. Cấu hình bài test: nâng `http.rateLimit*` qua platform config và đặt `trusted_proxies` để tải đến từ
  nhiều IP giả qua `X-Forwarded-For`; **không** tắt bộ giới hạn. Bộ giới hạn được đo riêng ở T5.
- Đăng nhập một lần rồi dùng token/API key cho toàn bộ tải (không đo `/auth/login` trong T2: argon2 chủ ý chậm).

### T3. Ghi hàng loạt và audit
- Import 10.000 zone / 50.000 rule của **một** tenant bằng 1, 8, 32 luồng; rồi 100 tenant song song.
- Giả thuyết: chuỗi audit `FOR UPDATE` trên một dòng mỗi tenant tuần tự hoá ghi của tenant đó → thêm luồng không tăng thông lượng một tenant. Đo A/B có/không `audit.enabled`, một tenant so với nhiều tenant.
- Đo backlog `outbox_events` chưa publish và RabbitMQ: ghi nhanh hơn publish thì backlog tăng không giới hạn; ghi kích thước hàng đợi theo thời gian.
- Đo kích thước bảng audit và `policy_versions` theo số lần sửa (tăng trưởng, vì bảng audit không bao giờ cắt).

### T4. Action nặng và đọc cả tenant
- `exportData`, `planUsage`, `validatePolicy` ở tenant 1k/10k/100k zone: thời gian, RAM đỉnh (`pprof` heap), số truy vấn (đọc theo trang 50 → N truy vấn).
- `rollback` về version có 1.000 rule; `applyPreset` lên zone đã có nhiều rule.
- Phân quyền kết hợp: người dùng chỉ có role theo scope zone thấy dữ liệu qua list có nhanh như admin không.
- Kiểm điều này ở mức đúng đắn chứ không chỉ tốc độ: kết quả `exportData` của tenant lớn phải đủ, không cắt im lặng.

### T5. Stress, giới hạn, chaos
- Tăng tải đến khi vỡ (T2 đọc và ghi) và ghi **cách vỡ**: load-shed có kiểm soát hay treo/OOM; phục hồi sau khi bỏ tải (P-STRESS-1). Theo dõi pool kết nối pgx (cạn kết nối thì request xếp hàng hay lỗi).
- Bộ giới hạn: một IP đập `/auth/login` và `/api` — IP khác không bị ảnh hưởng; `trusted_proxies` đúng (một IP không đội lốt IP khác bằng `X-Forwarded-For`).
- Postgres chậm (`tc netem` +100 ms), mất kết nối giữa chừng, RabbitMQ đứt (ghi vẫn thành công, outbox tích lại rồi xả), restart service khi đang ghi (không dở dang: version, audit, outbox khớp nhau).
- Cô lập tenant: một tenant gửi tải 100× — p99 của tenant khác đổi bao nhiêu (tinh thần QS-8, không có cell).

### T6. Soak (12–24 giờ)
Tải ổn định 50% mức đầu gối T2 + ghi 5/s. Đo rò bộ nhớ/goroutine/fd, tăng kích thước Postgres, độ trễ trôi theo giờ, log lỗi, vacuum/bloat của bảng bị ghi nhiều.

## 6. Quan sát và báo cáo

- `/metrics` của service (CPU/RAM/fd), `pg_stat_statements`, `pg_stat_activity` (chờ khoá — kiểm chuỗi audit), `pprof` bật bằng cờ (không mở ở production).
- Mỗi cấu hình chạy ≥ 3 lần, báo trung vị + khoảng; bỏ 60 s khởi động; ghi commit, cấu hình, phần cứng, seed.
- Báo cáo mỗi đợt: `docs/delivery/perf/<ngày>-management.md` — bảng so §3, đồ thị độ trễ theo tải và theo kích thước dữ liệu, nút thắt xếp theo tác động, chỗ lệch giả định 7.5.
- CI: chỉ T1 (có ngưỡng hồi quy) và một bản thu nhỏ T2 hàng tuần để thấy lệch tương đối. Runner chia sẻ có nhiễu: không dùng CI so với mục tiêu tuyệt đối (như `metap/testing/performance/scenarios.md` đã ghi).

## 7. Thứ tự và ước lượng

| Pha | Nội dung | Ước lượng | Xong khi |
|---|---|---|---|
| A | T1 + công cụ dữ liệu tầng 1–2 | 3 ngày | Có microbench làm mốc; nạp được 1M zone |
| B | T2 | 3–4 ngày | Đường cong p95 theo 1k–1M zone; danh sách câu SQL top; nút thắt đầu tiên có số |
| C | T3 + T4 | 4 ngày | Trần ghi một tenant, tăng trưởng audit/outbox, RAM của action nặng |
| D | T5 + T6 + báo cáo | 3 ngày + chạy máy | Bảng "cách vỡ", soak sạch hoặc có danh sách rò rỉ, báo cáo so §3 |

Tổng khoảng 2–3 tuần cho một người, nếu có máy perf cố định.

## 8. Câu hỏi cần chốt

1. Các số ở §3 dùng được không, hay product đã có số?
2. Có máy perf cố định cho pha B–D không?
3. Quy mô mục tiêu cho management là 1M zone thật (một DB) hay chia theo tenant/shard? Ảnh hưởng cách nạp và nội dung T2.
4. Cho phép sửa nút thắt tìm được ngay trong đợt này (ví dụ chỉ mục, khuếch đại ghi của `versions`), hay chỉ báo cáo?

## 9. Ghi nhận cho sau (ngoài phạm vi hiện tại)

Khi lên kế hoạch cho distributor/edge sẽ cần xử lý, đã thấy khi đọc `c-config-distributor` (chưa kiểm bằng test):

- `management.Client.AllZones` chỉ lấy trang đầu 50 zone (comment trong mã tự nhận là "known limitation"): zone thứ 51 trở đi không được publish.
- `RulesFor`/`DdosPoliciesFor` giới hạn 50, `IPAccessListsFor` 100, rule toàn tenant 50, không phân trang: zone nhiều hơn bị cắt im lặng.
- Mỗi lượt resync gọi 3 request mỗi zone và worker chỉ phục vụ một tenant (service account nó đăng nhập).

Đây là lỗi đúng đắn, nên khi quay lại các phần đó nên sửa và khoá bằng test trước khi đo hiệu năng.
