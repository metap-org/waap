# analytics-plane

Plane thứ tư theo [ADR-0001](../docs/architecture/09-adr/0001-four-planes.md): log và số liệu traffic
(khối lượng lớn, giữ lâu, truy vấn nhiều kiểu), tách khỏi database nghiệp vụ của Management. Không xây
trên `metap` ([ADR-0009](../docs/architecture/09-adr/0009-metap-for-management-plane-only.md)): từ
`metap` chỉ được import `pkg/kit/*` (depguard trong `.golangci.yml`).

## Bản đầu (2026-11-08): `analytics-api`

Một service Go, dữ liệu trong ClickHouse ([ADR-0020](../docs/architecture/09-adr/0020-clickhouse-and-parquet-for-analytics.md)).

```
edge ──batch──▶ control-plane ──POST /v1/security-events/ingest──▶ analytics-api ──▶ ClickHouse
portal ──GraphQL──▶ gateway (management) ──GET /v1/security-events, POST …/aggregate──▶ analytics-api
alerting-service (correlate / evaluate) ──GET /v1/security-events?since=…──▶ analytics-api
```

| Route | Việc |
|---|---|
| `POST /v1/security-events/ingest` | `{"events":[…]}`, tối đa 1000 event; kiểm từng event (enum, IP, thời gian, độ dài); trả `accepted` và danh sách `rejected` theo vị trí |
| `GET /v1/security-events` | Lọc bằng so sánh bằng trên các trường cho phép (`zoneId`, `action`, `triggeredBy`, `triggeredById`, `triggeredByName`, `sourceIp`, `requestPath`), `since`/`until`, `sort=occurredAt\|-occurredAt`, `limit` (≤ 200), `cursor` (keyset) |
| `GET /v1/security-events/{id}` | Một event |
| `POST /v1/security-events/aggregate` | Cùng dạng spec với aggregate của metap (`groupBy`, `bucket` hour/day/week/month, `filters`, `since`, `until`, `limit` ≤ 500, metric `count` / `count:<field>`), trả dòng `{bucket?, group?, count}` nên portal không phải sửa |
| `GET /health` | Không cần token; kiểm ClickHouse |

**Ranh giới bảo mật**
- Mọi route `/v1/*` cần token của platform (EdDSA, JWKS của zones-service; kiểm `iss`, `aud`, `exp`).
  Tenant luôn lấy từ token, mọi câu SQL bắt đầu bằng `tenant_id = ?`, mọi giá trị đều là tham số.
- Không nhận SQL hay tên cột từ request: chỉ các trường trong allowlist; trường lạ trả `400`.
- Ingest cần thêm `X-Analytics-Ingest-Token` (`ANALYTICS_INGEST_TOKEN`), để token người dùng
  portal không tự ghi event giả được. Chỉ để trống khi dev.
- Giới hạn chi phí: `max_execution_time` 10 s và `max_memory_usage` 1 GiB cho mỗi truy vấn đọc.
- Retention là TTL của bảng (`ANALYTICS_RETENTION_DAYS`, mặc định 30 ngày); đổi giá trị thì lần khởi
  động sau tự `ALTER … MODIFY TTL`.

**Chưa làm (giai đoạn sau, theo 5.8.8–5.8.9)**: ingest gateway mTLS, Kafka, enricher (geo/ASN,
dedupe), archiver Parquet, detection/alerting chạy trong Analytics (hiện correlation và alert
evaluation vẫn ở alerting-service, chỉ đổi nguồn đọc), exporter. Quyền đọc mới khoanh theo tenant;
chưa lọc theo zone/vai trò (RBAC/ABAC) như thiết kế. Mọi event control-plane gửi lên đều thuộc
tenant của service account của nó, y như đường ghi cũ.

## Chạy

Compose của `../management-plane` đã chạy kèm `clickhouse` và `analytics-api` (cổng 3030).
Chạy riêng trên host:

```bash
docker run -d --name clickhouse -p 9000:9000 -p 8123:8123 -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:25.8
JWKS_URL=http://localhost:3000/.well-known/jwks.json go run ./cmd/analytics-api
```

| Env | Mặc định |
|---|---|
| `PORT` | `3030` |
| `CLICKHOUSE_URL` | `clickhouse://default:@localhost:9000` |
| `ANALYTICS_DATABASE` | `waf_analytics` |
| `ANALYTICS_RETENTION_DAYS` | `30` |
| `JWKS_URL` | bắt buộc |
| `JWKS_CACHE_TTL_SECONDS` | `300` |
| `ANALYTICS_INGEST_TOKEN` | trống = không kiểm (chỉ dev) |

## Test

```bash
go test ./...                                                     # unit
CLICKHOUSE_URL=clickhouse://default:@127.0.0.1:9000 go test -tags integration ./...   # ClickHouse thật
```
