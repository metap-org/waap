# 10.2 Kế hoạch test tải, stress, dữ liệu lớn (BẢN NHÁP)

Mục tiêu: có số đo thật cho các mục tiêu chất lượng ở [10.1](00-index.md) và cho giả định ở
[7.5](../07-deployment/05-capacity-and-sharding.md) (1.000.000 zone), trên **phần đã được code**: management
service, config distributor, edge, analytics API. Phần chưa có (cell, PoP store, WAF engine thật, realtime
gateway) nằm ngoài kế hoạch này, trừ khi ghi rõ.

Kế hoạch này là bản đề xuất để chủ dự án chốt: số mục tiêu ở §3 do tôi đề xuất, không phải số đã duyệt.

## 1. Đọc mã trước khi đo: ba chỗ chắc chắn vỡ ở quy mô

Ba điều dưới đây thấy ngay khi đọc `c-config-distributor`, nên là **việc đầu tiên** của kế hoạch (T0), không
phải thứ chờ test tìm ra:

| # | Phát hiện | Hệ quả |
|---|---|---|
| F1 | `management.Client.AllZones` chỉ lấy **trang đầu 50 zone** (comment trong mã tự nhận "known limitation", không theo `nextCursor`) | Zone thứ 51 trở đi **không bao giờ được publish ra edge**. Một tenant có 51 zone đã sai |
| F2 | `RulesFor`/`DdosPoliciesFor` giới hạn 50, `IPAccessListsFor` 100, rule toàn tenant 50 — không phân trang | Zone có hơn 50 rule bị **cắt im lặng**: edge chạy bộ rule thiếu mà không có lỗi nào |
| F3 | Mỗi lượt resync đọc **3 request mỗi zone** (policy, rule, IP list) và worker chỉ phục vụ **một tenant** (service account nó đăng nhập) | 1M zone = 3M request mỗi phút với `RESYNC_INTERVAL_SECONDS=60`; và không có đường nào publish zone của tenant khác |

F1/F2 là lỗi đúng đắn (correctness) chứ không phải hiệu năng: test dữ liệu lớn sẽ đỏ ngay ở bước đầu. Đề xuất
sửa (phân trang theo cursor, đọc theo lô `?ids=`/lọc theo `zoneId IN`, resync tăng dần theo `updatedAt`) làm
**trước** các test hiệu năng, để số đo không đo một hệ thống đang sai.

## 2. Phạm vi và những gì đã có

| Thành phần | Chạy ở đâu | Đo cái gì | Công cụ có sẵn |
|---|---|---|---|
| Management service (`m-management`, cổng 3000) | Postgres qua metap | API CRUD/list/action ở nhiều quy mô dữ liệu, ghi hàng loạt, guard | Bộ k6 của metap (`metap/testing/performance/k6`: seed, list, filter+sort, cursor) |
| Config distributor (`c-config-distributor`) | Đọc management, ghi Redis | Thời gian resync theo số zone, độ trễ truyền thay đổi, ổn định Redis | Chưa có |
| Edge (`e-edge`) | Đọc Redis, proxy tới origin | Độ trễ thêm (QS-1), thông lượng, nạp snapshot, bộ nhớ theo số zone | `spike/adr-0031/bench` (so sánh lựa chọn proxy, không phải edge Go hiện tại) |
| Analytics API (`a-api`) | ClickHouse | Ingest theo lô, truy vấn, retention | Chưa có |
| Toàn tuyến portal → edge | Cả bốn | Độ trễ truyền cấu hình khi đang tải | Lần chạy tay ở roadmap 98 (chưa tự động) |

Không có hàm `Benchmark*` nào trong repo waap (đã đếm), nên mức vi mô (microbench) cũng là việc mới.

## 3. Mục tiêu đề xuất (cần chốt)

Lấy nguyên từ 10.1 khi đã có; còn lại là **đề xuất** vì 10.1 chưa có số cho management plane.

| Mã | Mục tiêu | Nguồn |
|---|---|---|
| P-EDGE-1 | Độ trễ thêm của edge, rule-set 50 rule: p99 ≤ 0,5 ms ở 10k rps/lõi (chỉ phần L7, chưa tính WAF engine) | QS-1 (phần L7) |
| P-EDGE-2 | Nạp snapshot 100.000 zone: ≤ 30 s, RAM tăng ≤ 4 GB, request đang chạy không tăng p99 quá 10% khi swap | Đề xuất, từ 7.5 §2 |
| P-EDGE-3 | Zone chưa có trong RAM: ≤ 10 ms p99 | QS-25 (đo khi có PoP store; hiện đo nạp từ Redis) |
| P-ISO-1 | Một zone bị flood (100× bình thường) làm p99 zone khác đổi ≤ 10% | QS-8, QS-32 (phiên bản không có cell) |
| P-MGMT-1 | `GET /api/waf.zones` (limit 50, cursor) ở 1M zone/10k tenant: p95 ≤ 200 ms, 200 rps | Đề xuất |
| P-MGMT-2 | Tạo/sửa rule qua guard (quota, validation, version): p95 ≤ 250 ms ở 50 rps | Đề xuất |
| P-PROP-1 | Từ lúc sửa rule ở portal đến lúc edge áp dụng: p99 ≤ 90 s (resync 60 s + refresh 10 s + dư) ở 10k zone | Đề xuất; hiện thiết kế là mốc hội tụ, không phải mốc nhanh |
| P-PROP-2 | Bulk 10.000 thay đổi trong 1 phút (Terraform): không mất thay đổi nào, hội tụ ≤ 5 phút | G10 (đỉnh 30/s) nhân hệ số 10 |
| P-TEL-1 | Telemetry: edge → distributor → a-api 20.000 security event/s liên tục, mất ≤ 0,1% khi ingest chậm, không làm edge chậm | Đề xuất, từ G11 (thu nhỏ) |
| P-DATA-1 | Không có thao tác nào giữ toàn bộ dữ liệu của tenant trong RAM (export, usage, validate) ở 100k zone/tenant: RAM ≤ 1 GB | Đề xuất |

## 4. Dữ liệu lớn: cách sinh

Bắt chước phân bố của 7.5 G4 để kết quả đại diện được:

- **Phân khúc**: 65% Free, 15% Pro, 18% Business, 2% Enterprise. Số rule mỗi zone log-normal: Free 2–5,
  Pro 5–15, Business 20–60, Enterprise 100–1.000 (có vài zone 5.000 để thử F2). IP list 0–10.000 mục.
- **Tenant**: 1M zone chia theo tenant lệch kiểu Pareto (đa số 1–3 zone, một ít tenant 10k+ zone).
- **Cách nạp**: ba tầng, vì mỗi tầng kiểm cái khác nhau.
  1. *Qua API metap* (k6 hoặc seeder Go gọi `recordclient`): đúng guard/audit/outbox, chậm — dùng cho 1k–10k zone và để đo chính việc ghi.
  2. *`COPY` thẳng vào bảng đã reconcile* (SQL `generate_series`, như `metap/testing/performance/seed_10m.sql`): cho 100k–1M zone. Phải chạy `reconcile` trước (bảng do metap tạo), và điền cả `audit` nếu muốn đo đúng kích thước bảng audit.
  3. *Snapshot Redis dựng sẵn* (Go, dùng `ruleset` của distributor): cho test edge một mình, không cần management.
- Dữ liệu sinh theo seed cố định, lưu số liệu thống kê (phân bố thật được sinh) cạnh kết quả.
- Dọn dẹp: mỗi lần chạy tạo database riêng (`testkit` cho phần nhỏ; database đặt tên theo lần chạy cho phần lớn), không dùng chung với dev.

## 5. Các bài test

Thứ tự là thứ tự thực hiện. Mỗi bài ghi: câu hỏi, cách làm, đo gì, giả thuyết cần kiểm.

### T0. Sửa F1/F2/F3 và khoá bằng test
Không phải test hiệu năng nhưng là điều kiện: test tích hợp "tenant có 120 zone, zone có 80 rule và 150 IP entry → edge nhận đủ"
(phải đỏ trên mã hiện tại). Làm bài này trước khi chạy T3–T6.

### T1. Microbench (Go `testing.B`, chạy được trong CI)
- `e-edge/evaluate`: một request qua bộ rule 10/50/500/5.000 rule, có/không IP list lớn (10k CIDR), regex.
- `e-edge/ratelimit`, `e-edge/regexcache`, `e-edge/clearance`: cấp phát bộ nhớ, tranh chấp khoá khi nhiều goroutine.
- `c-config-distributor/ruleset` (`ContentVersion`, mã hoá) và `compile.CompileZone` với zone 5.000 rule.
- `m-management/zones/rules` (chuẩn hoá hostname, matchCondition), `policy.Validate` với 5.000 rule.
- Ra: ns/op, B/op, allocs/op; so sánh bằng `benchstat`; ngưỡng hồi quy +20% làm đỏ CI.

### T2. Management API — đường cong theo kích thước dữ liệu
- Dữ liệu 1k → 10k → 100k → 1M zone (tầng 1 rồi tầng 2 ở §4).
- Kịch bản (k6, **arrival-rate** executor để tránh coordinated omission): list + cursor, list có `withTotal`,
  filter theo `status`/`accountId`, `search` theo hostname, `jql`, get theo id, `?ids=` lô 50, aggregate.
- Ghi: tạo zone, tạo/sửa rule (qua guard: quota đếm, version snapshot), transition, delete (kiểm tham chiếu).
- Đo: p50/p95/p99, lỗi, CPU/RAM service, `pg_stat_statements` (câu nào tăng theo N), kích thước bảng/chỉ mục, `EXPLAIN` các câu top.
- Giả thuyết: (a) `withTotal` đếm trên bảng lớn là câu chậm nhất; (b) mỗi request nạp role từ DB không cache là trần
  thông lượng ở nhiều kết nối; (c) `jx.Count` của quota quét theo tenant ở tenant 100k zone; (d) snapshot version
  của zone 1.000 rule làm một lần sửa rule thành 1.000 dòng ghi (khuếch đại ghi).
- **Rate limit**: bucket chung 5 req/s/IP (burst 300) và bucket auth (burst 20, 1 lượt/3 s) sẽ chặn bài test từ
  một IP. Cấu hình đúng cho bài test: nâng `http.rateLimit*` qua platform config và đặt `trusted_proxies` để tải đến từ
  nhiều IP giả (`X-Forwarded-For`), **không** tắt bộ giới hạn — một bài riêng (T9) đo chính bộ giới hạn.

### T3. Ghi hàng loạt và audit
- Import 10.000 zone/50.000 rule của **một** tenant bằng 1, 8, 32 luồng song song.
- Giả thuyết: chuỗi audit băm `FOR UPDATE` trên một dòng mỗi tenant tuần tự hoá mọi ghi của tenant đó → thông
  lượng một tenant bị trần bởi chuỗi audit, thêm luồng không giúp. Đo tách: có/không `audit.enabled` (A/B), nhiều tenant song song.
- Đo cả backlog outbox và RabbitMQ (`outbox_events` chưa publish), vì ghi nhanh hơn publish sẽ tích backlog.

### T4. Config distributor
- Sau T0. Dữ liệu 1k/10k/100k zone (một tenant, rồi nhiều tenant khi đã hỗ trợ).
- Đo: thời gian một lượt resync, số request tới management, ghi Redis (bao nhiêu khoá thực sự đổi so với bị ghi lại), CPU/RAM.
- Độ trễ truyền (P-PROP-1): sửa rule, đo đến khi Redis có version mới và đến khi edge áp dụng; chạy khi management đang chịu tải T2.
- Bulk (P-PROP-2): 10.000 thay đổi trong 1 phút; kiểm không mất, không áp dụng sai thứ tự; sự kiện bị bỏ (kill subscriber giữa chừng) thì resync có gánh được không.
- Giả thuyết: lượt resync tăng tuyến tính theo số zone và vượt `RESYNC_INTERVAL_SECONDS` ở khoảng vài chục nghìn zone → các lượt chồng nhau; cần kiểm có khoá "đang chạy" hay không.

### T5. Edge
- Máy đo cố định (xem §7). Origin giả đáp nhanh (spike đã có `origin`). Tải từ máy riêng.
- Công cụ: `wrk2`/`vegeta` ở **tốc độ cố định** (đồ thị độ trễ theo rps, tìm đầu gối), HTTP/1.1 keepalive và HTTP/2; body 1 KB/100 KB/10 MB; hostname phân bố Zipf trên N zone.
- Snapshot 1k → 100k → 300k zone (dựng ở tầng 3 §4): RAM, thời gian nạp, p99 khi swap (P-EDGE-2). Pause GC (`GODEBUG=gctrace=1`), pprof.
- Hành vi: rule block/challenge/log, rate limit theo IP khi 100k IP khác nhau (kích thước map), cookie clearance, body vượt `MAX_REQUEST_BODY_BYTES`, slowloris/giữ kết nối, origin chậm/đứt (timeout, kết nối không rò).
- Đo telemetry buffer (mặc định 10.000, lô 100): khi ingest chậm buffer đầy thì **bỏ gì, có đếm không, có chặn request không** — phải là "bỏ và đếm", không bao giờ chặn đường request.
- Cô lập (P-ISO-1): zone A bị flood 100× trong khi zone B tải bình thường; đo p99 của B.

### T6. Analytics API
- Ingest lô 100–5.000 bản ghi, 1–32 client; mục tiêu 20.000 event/s trên một node; TTL/retention; kích thước ClickHouse theo ngày.
- Truy vấn: lọc theo zone + khoảng thời gian trên 1 tỉ dòng, aggregate, cursor sâu; truy vấn song song khi đang ingest.
- Sweep của alerting (`correlate`/`evaluate`, `eventScanLimit = 200`): ở tốc độ tấn công, 200 sự kiện mỗi lượt có phản ánh đúng không (đúng đắn khi quá tải, không chỉ tốc độ).

### T7. Toàn tuyến (end-to-end) dưới tải
- Tự động hoá lần chạy tay ở roadmap 98 thành script: tạo zone + rule qua portal API → chờ edge áp dụng → bắn request → kiểm phản hồi và event ở a-api.
- Chạy với tải nền T2 + T5 cùng lúc, đo P-PROP-1 và tỉ lệ request bị xử lý bằng cấu hình cũ trong cửa sổ hội tụ.

### T8. Soak (12–24 giờ)
- Tải ổn định 50% mức đầu gối T5 + thay đổi cấu hình 2/s + telemetry. Đo rò bộ nhớ/goroutine/fd, tăng kích thước Postgres/Redis/ClickHouse, độ trễ trôi theo giờ, log lỗi.

### T9. Stress, chaos, cô lập
- Tăng tải đến khi vỡ (T2/T5/T6) và ghi **cách vỡ**: có load-shed (429/503 có kiểm soát) hay treo/OOM; phục hồi sau khi bỏ tải.
- Bộ giới hạn: một IP tấn công `/auth/login` và `/api` — IP khác không bị ảnh hưởng (bucket theo `ClientIP`), kiểm `trusted_proxies`.
- Lỗi phụ thuộc: Redis chậm/đứt, Postgres chậm (`tc netem` 100 ms), RabbitMQ đứt, management 5xx → edge giữ cấu hình cuối cùng (QS-2/QS-3), distributor không xoá zone vì một lượt đọc thất bại.
- Kill -9 distributor giữa lượt resync, restart edge khi đang tải (thời gian lạnh đến lúc phục vụ đúng).

## 6. Quan sát và báo cáo

- **Metrics**: `/metrics` của từng service (đã có CPU/RAM/fd), `pg_stat_statements`, `redis INFO`, ClickHouse `system.query_log`, `pprof` (bật bằng cờ, không mở ở production), `go tool trace` khi swap snapshot.
- **Chạy lặp**: mỗi cấu hình ≥ 3 lần, báo trung vị + khoảng; bỏ 60 s khởi động; ghi phiên bản mã (commit), cấu hình, phần cứng, seed dữ liệu.
- **Báo cáo** mỗi đợt: một file `docs/delivery/perf/<ngày>-<phạm vi>.md` — bảng so với mục tiêu §3, đồ thị độ trễ-theo-tải, danh sách nút thắt xếp theo tác động, chỗ nào lệch giả định 7.5.
- **CI**: chỉ T1 (microbench, có ngưỡng hồi quy) và một bản thu nhỏ T2/T5 hàng tuần để thấy lệch tương đối. Runner chia sẻ có nhiễu (như đã ghi ở `metap/testing/performance/scenarios.md`): không dùng CI để so với mục tiêu tuyệt đối.

## 7. Môi trường

| Bậc | Cấu hình | Dùng cho |
|---|---|---|
| Local | Docker compose (`m-deploy`), Postgres + Redis + ClickHouse + RabbitMQ trên máy dev | T0, T1, T2 nhỏ, soạn script |
| Perf cố định | 3 máy cùng phần cứng: tải / hệ thống / origin+DB; 16+ lõi, NVMe, mạng 10 GbE; tắt turbo/thay đổi tần số, CPU pinning | T2 lớn, T3–T6, T8 |
| Cô lập chaos | Như trên + `tc`/`toxiproxy` | T9 |

Cần chủ dự án cho biết có máy cố định không; nếu chưa, các số ở T5 chỉ là **tương đối** (so giữa các lần chạy), không so với 7.5.

## 8. Thứ tự, ước lượng, tiêu chí xong

| Pha | Nội dung | Ước lượng | Xong khi |
|---|---|---|---|
| A | T0 (sửa F1–F3 + test) và T1 (microbench + CI) | 3–4 ngày | Test "120 zone/80 rule" xanh; có số microbench đầu tiên làm mốc |
| B | Công cụ dữ liệu (§4 tầng 1–3) và T2 | 4–5 ngày | Đường cong p95 theo 1k–1M zone, danh sách câu SQL top, nút thắt đầu tiên có số |
| C | T3 và T4 | 4 ngày | Trần ghi một tenant, thời gian resync theo số zone, P-PROP-1 có số |
| D | T5 và T6 | 5 ngày | Đồ thị rps-độ trễ edge, RAM theo zone, ingest 20k/s đạt hoặc có nút thắt |
| E | T7, T9 | 4 ngày | Script end-to-end tự động; bảng "cách vỡ" của từng thành phần |
| F | T8 và báo cáo tổng | 3 ngày + chạy máy | Soak sạch hoặc có danh sách rò rỉ; báo cáo so §3 và 7.5 |

Tổng khoảng 4–5 tuần cho một người, nếu có máy perf cố định; không có thì pha D–F kém tin cậy.

## 9. Câu hỏi cần chủ dự án chốt

1. Mục tiêu ở §3 (nhất là P-MGMT-*, P-PROP-*, P-TEL-1) có dùng được không, hay đã có số từ product?
2. Có máy perf cố định cho pha B–F không?
3. Mô hình tenant của distributor (F3): một worker một tenant có phải chủ ý? Nếu có, 1M zone nhiều tenant cần mô hình khác (một worker theo shard/cell) — ảnh hưởng T4.
4. Có chấp nhận sửa F1/F2 (T0) trước khi đo không? (Đề xuất: có — nếu không, mọi số ở T4/T5 đo một hệ thống sai.)
5. Phạm vi WAF engine: bài test edge hiện chỉ đo phần L7 Go; số QS-1 đầy đủ chờ engine thật.
