# ADR-0031: Lựa chọn nền cho tầng L7 và engine cho tầng WAF

- Trạng thái: **Đề xuất, chờ chọn** sau spike (§7). **Hướng spike chốt 2026-11-17:** Pingora làm L7 chính, NGINX chạy song song để đối chứng, Envoy hoặc NGINX là dự phòng ([§6.1](#61-hướng-spike-chốt-2026-11-17)). Phương án cũ ở [ADR-0013](0013-edge-gateway-custom-go-with-coraza.md) nằm trong các lựa chọn dưới đây.
- Ngày: 2026-11-06

## 1. Bối cảnh

Edge có hai tầng ([ADR-0033](0033-two-tier-edge-proxy-and-waf.md)), nên có **ba lựa chọn**:

1. **Nền cho tầng L7:** kết nối, TLS, pre-filter, challenge, cache, proxy tới origin. Tốn RAM, kết
   nối, băng thông.
2. **Engine cho tầng WAF:** chạy SecLang/CRS và custom rule phần inspect. Tốn CPU.
3. **Cách L7 gọi WAF:** giao thức kiểm tra giữa hai tầng.

Hai tầng là hai process riêng, nên hai lựa chọn đầu gần như độc lập: L7 viết bằng một ngôn ngữ, WAF
có thể viết bằng ngôn ngữ khác.

## 2. Tiêu chí

| # | Tiêu chí | Áp cho | Trọng số |
|---|---|---|---|
| T1 | Độ trễ p99, bộ nhớ mỗi kết nối, CPU mỗi request | Cả hai | Cao |
| T2 | Độ trung thành với CRS (`go-ftw`) | WAF | Cao |
| T3 | An toàn bộ nhớ trên đường traffic | L7 (nhận traffic internet) cao hơn WAF (đã cô lập process) | Cao |
| T4 | Mức kiểm soát pre-filter, challenge, governor theo tenant, fail mode, swap snapshot | L7 | Cao |
| T5 | **Khả năng cache** (RAM + đĩa, purge, cache key) | L7 | Cao ([ADR-0035](0035-cdn-and-dns-strategy.md)) |
| T6 | **Gọi dịch vụ kiểm tra ngoài kèm body** (stream, timeout, fail mode) | L7 | Cao |
| T7 | Độ chín, vận hành ở quy mô lớn | Cả hai | Trung bình |
| T8 | Kỹ năng đội (chủ yếu Go) | Cả hai | Trung bình |
| T9 | Dùng lại code cho replay và `edge-verify` | Cả hai | Trung bình |
| T10 | HTTP/3 | L7 | Thấp |
| T12 | Truy cập ClientHello và frame HTTP/2 thô (để tính JA4, fingerprint HTTP/2 cho bot management, [ADR-0038](0038-bot-management-layered-scoring.md)) | L7 | Trung bình |
| T13 | Body processor: XML (tắt thực thể ngoài), multipart, JSON; khả năng gắn decoder riêng (GraphQL, protobuf theo descriptor cho gRPC) | WAF | Trung bình |
| T11 | License phát hành on-prem | Cả hai | Điều kiện loại |
| T14 | **Ít reload**: cấu hình zone, cert, ruleset áp dụng không reload process, không rớt kết nối ([ADR-0015](0015-snapshot-format-and-distribution.md), [ADR-0037](0037-config-apply-classes.md)) | Cả hai | Cao, điều kiện loại cho cấu hình của zone |

## 3. Nền cho tầng L7

| | N1. Rust + Pingora | N2. Envoy | N3. NGINX / OpenResty | N4. HAProxy | N5. Go tự xây |
|---|---|---|---|---|---|
| T1 | Tốt, không GC | Tốt | Rất tốt | Rất tốt | Tốt; GC ít ảnh hưởng hơn trước vì CRS đã ra khỏi process |
| T3 | Cao | Lõi C++ đã kiểm chứng; logic của mình trong wasm/Lua | Lõi C đã kiểm chứng; logic Lua | Lõi C đã kiểm chứng | Cao |
| T4 | Cao (code của mình) | Trung bình (filter chain) | Trung bình (phase của NGINX, Lua) | Thấp–trung bình (Lua, cấu hình) | Cao |
| T5 cache | Có (`pingora-cache`), cần tự hoàn thiện purge, tầng đĩa | Yếu (cache filter cơ bản) | **Rất mạnh** (`proxy_cache`, đĩa, purge qua module) | Yếu (cache nhỏ trong RAM) | Phải tự viết |
| T6 gọi WAF | Tự viết client gRPC | **Có sẵn** (`ext_proc`) | Yếu (không có cơ chế gửi body sang dịch vụ ngoài sẵn) | **Có sẵn** (SPOE) | Tự viết client gRPC |
| T7 | Dùng ở quy mô Cloudflare; cộng đồng nhỏ | Rất chín | Rất chín, phổ biến nhất | Rất chín | L7 tự viết |
| T8 | Cần người Rust | C++ / wasm | Lua | Cấu hình + Lua | Có sẵn |
| T10 | Chưa xác nhận | Có | Có | Có | Có (`quic-go`) |

Nhận xét: không nền nào mạnh cả về cache lẫn gọi WAF sẵn. N3 mạnh nhất về cache nhưng yếu về gọi dịch
vụ kiểm tra; N2/N4 ngược lại. N1 và N5 phải tự viết cả hai nhưng kiểm soát hoàn toàn.

## 4. Engine cho tầng WAF

| | E1. Coraza (Go) | E2. libmodsecurity v3 (C++) | E3. SecLang thuần Rust |
|---|---|---|---|
| T2 | Tương thích CRS đầy đủ, được CRS test | Engine tham chiếu của CRS | Tự công bố tương thích (`zentinel-modsec`, `sentinel-modsec`, 0.x), chưa kiểm chứng độc lập |
| T1 | Có GC, nhưng giờ chỉ ảnh hưởng tầng WAF | Nhanh; cần wrapper (Rust/Go/C++) làm dịch vụ gRPC | Chưa có số liệu độc lập |
| T3 | Cao | Thấp, nhưng đã cô lập trong process riêng nên crash không kéo theo L7 | Cao |
| T7 | Trung bình–cao; có sẵn `coraza-spoa` cho HAProxy, proxy-wasm cho Envoy | Cao (dùng rộng với NGINX) | Thấp |
| T8 | Có sẵn (Go) | Cần C++ cho wrapper hoặc FFI | Cần Rust |
| T9 | Replay chạy cùng engine dễ | Cần wrapper chung | Như E1 nếu replay viết Rust |

Vì tầng WAF đã tách process, nhược điểm lớn nhất của E1 (GC) và E2 (an toàn bộ nhớ) đều giảm nhiều.
**E1 là mặc định hợp lý**; E2 đưa vào spike để so CPU mỗi request, vì CPU tầng WAF là chi phí hạ tầng lớn.

## 5. Cách L7 gọi WAF

### WAF là callout, không phải một chặng (đã chốt)

L7 gửi request sang WAF và chỉ nhận lại **verdict**; WAF không nằm trên đường đi của traffic tới origin.
Phương án "WAF làm reverse proxy nối tiếp" (L7 → WAF → origin) đã bị loại ở
[ADR-0033](0033-two-tier-edge-proxy-and-waf.md). Lý do:

| | WAF là một chặng (forward HTTP) | WAF là callout, chỉ trả verdict |
|---|---|---|
| Response của origin | Đi qua WAF (response ≈ 25 KB so với request ≈ 1 KB): nhiều lần băng thông nội bộ PoP và RAM ở tầng WAF | Không đi qua WAF |
| Tài nguyên tầng WAF | Giữ kết nối, đệm response, TLS tới origin: lại thành proxy tốn RAM | Thuần CPU |
| WAF chết hoặc quá tải | Đường đi của request đứt; fail-open cần L7 tự đi vòng | L7 vẫn giữ request, áp fail mode ngay |
| Verdict sớm | WAF tự trả trang chặn, tự đóng kết nối | WAF trả `BLOCK` giữa body, L7 dừng gửi |
| Ngữ cảnh từ L7 (bot, consumer, deadline, version) | Nhét vào header, phải lọc header giả từ client | Trường có kiểu trong message |
| Thực thi hành động, ghi log | Cả hai tầng cùng làm | Chỉ L7 |

Dạng lai kiểu `auth_request` (gửi bản copy request, lấy status code làm verdict) vẫn là callout; chỉ khác
giao thức, thuộc câu hỏi transport bên dưới.

### Giao thức callout (còn mở)


| | I1. Inspection API riêng (gRPC stream) | I2. Envoy `ext_proc` | I3. HAProxy SPOE |
|---|---|---|---|
| Mô tả | Proto của mình ([5.5](../05-building-blocks/05-contracts.md#inspection-api-l7--waf)) | Giao thức có sẵn của Envoy | Giao thức có sẵn của HAProxy |
| Ưu | Độc lập nền L7; đúng thứ mình cần (verdict sớm, ngân sách thời gian) | Không phải viết client | Không phải viết client; có `coraza-spoa` |
| Nhược | Phải viết client ở L7 | Gắn với Envoy | Gắn với HAProxy; giới hạn kích thước body |

Hướng hiện tại: **I1 là hợp đồng chuẩn**, chạy trên gRPC; transport còn mở (IA13); chi tiết hợp đồng còn mở (IA1–IA13 ở [5.5](../05-building-blocks/05-contracts.md#inspection-api-l7--waf)) và được nghiên cứu trong spike này. Nếu chọn N2 hoặc N4, viết adapter nhỏ ở phía WAF nhận `ext_proc`
hoặc SPOE rồi chuyển sang I1, để tầng WAF chỉ có một giao diện.

## 6. Tổ hợp và shortlist

| Tổ hợp | L7 | WAF | Gọi | Mạnh | Yếu |
|---|---|---|---|---|---|
| **K1** | N2 Envoy | E1 Coraza | I2 → I1 | L7 rất chín, tích hợp kiểm tra sẵn, HTTP/3 | Cache yếu: cần thêm tầng cache (ví dụ Varnish) hoặc chờ CDN tự xây |
| **K2** | N1 Pingora | E1 Coraza (hoặc E2) | I1 | Kiểm soát cao, không GC ở L7, có nền cache | Cần người Rust; tự viết client, purge, cache đĩa |
| **K3** | N5 Go | E1 Coraza | I1 | Đội làm được ngay; replay dùng chung code cả hai tầng | Tự viết cache và L7 |
| **K4** | N4 HAProxy | E1 Coraza | I3 (`coraza-spoa`) | L7 rất nhanh và chín, tích hợp WAF có sẵn | Cache yếu; pre-filter phải viết bằng Lua/cấu hình |
| **K5** | N3 NGINX | E1 Coraza | Module/njs tự viết → I1 | Cache mạnh nhất, phổ biến nhất | Gọi WAF kèm body không có sẵn; pre-filter bằng Lua |

**Shortlist ban đầu:** K2, K3, K1; K5 làm mốc cache, K4 làm mốc gọi WAF. **Đã thu hẹp ngày 2026-11-17 theo [§6.1](#61-hướng-spike-chốt-2026-11-17):** chạy A1, A2 và B. K1 (Envoy) và K3 (Go) là dự phòng, không dựng trong spike trừ khi điều kiện bỏ Pingora xảy ra. K4 (HAProxy) bị loại khỏi ứng viên vì license cần xác minh theo ADR-0024, cache yếu và pre-filter phải viết bằng Lua.
Tầng WAF dùng E1 ở A1; A2 đo E2 trên cùng giao diện I1 để so CPU.

### 6.1 Hướng spike (chốt 2026-11-17)

Chủ dự án chọn hướng thử nghiệm: **Pingora làm L7 chính**, tầng WAF thử **Coraza và libmodsecurity**, **NGINX chạy song song để đối chứng**; nếu Pingora không hiệu quả thì dùng **NGINX hoặc Envoy**. Tiêu chí hàng đầu của cấu hình là **càng ít reload càng tốt** (T14).

| Nhánh | L7 | WAF | Mục đích |
|---|---|---|---|
| **A1** | Pingora | Coraza, qua I1 | Hướng chính |
| **A2** | Pingora | libmodsecurity (wrapper gRPC), qua I1 | So CPU và độ khớp CRS trên cùng giao diện |
| **B** | NGINX + ModSecurity | libmodsecurity trong NGINX | Mốc đo cache, mốc đo reload |

Vì sao T14 nghiêng về Pingora: ADR-0015 đặt policy agent trong binary của từng tầng và swap con trỏ snapshot nguyên khối; ADR-0037 bắt mọi cấu hình của zone là lớp A. NGINX nạp cấu hình và quy tắc ModSecurity theo file và reload, nên chỉ đạt lớp A khi chuyển logic sang Lua và shared memory (tự viết lại policy agent bằng Lua); release CRS mới (lớp B) làm reload cả proxy. Envoy đổi nóng bằng xDS nhưng ADR-0015 đã loại dùng nguyên xDS vì tài nguyên khác hẳn, nên vẫn cần adapter hoặc filter đọc snapshot.

Ở tầng WAF, T14 yêu cầu: build instance ruleset ở nền rồi swap con trỏ (lớp B); ngoại lệ theo zone bằng một lớp luật nhỏ chạy trước CRS phát lệnh bỏ rule (`ctl:ruleRemoveById`), không tạo instance riêng cho mỗi zone. Cần xác minh engine nào hỗ trợ tới đâu.

**Điều kiện bỏ Pingora (đề xuất, chờ duyệt):**

| Điều kiện | Chuyển sang |
|---|---|
| Một điều kiện bắt buộc không làm được trong 3 tuần (cert theo SNI động, chọn upstream theo từng request, nâng binary không rớt kết nối); hoặc công sức hoàn thiện cache, purge, tầng đĩa quá 2 lần ước lượng; hoặc không tuyển được người Rust | Envoy hoặc NGINX theo hai dòng dưới |
| Lý do là tốn công viết L7 và nhu cầu cache nhẹ ([ADR-0035](0035-cdn-and-dns-strategy.md) phương án B, C hoặc D) | **Envoy** (đổi nóng tốt hơn NGINX) |
| Cache là yêu cầu bán hàng chính **và** tầng WAF đã tách process qua I1 (để ruleset không buộc reload) | **NGINX** |

**Chưa xác minh, việc đầu tiên của tuần 1:** Pingora hỗ trợ HTTP/3 chưa; chọn cert theo SNI trong lúc handshake; graceful upgrade; libmodsecurity chạy trong wrapper gRPC (tạo Rules mới, swap); Envoy cache filter tới đâu; license thật của HAProxy nếu còn dùng làm mốc ([ADR-0024](0024-licensing-policy-for-shipped-components.md)).

**Hệ quả với replay và `edge-verify`:** theo [ADR-0030](0030-language-per-workload.md), nếu L7 là Pingora thì hai thành phần này viết bằng Rust, tầng WAF vẫn Go.

## 7. Spike

Thời hạn 3 tuần, 2 người. Mỗi nhánh (A1, A2, B) dựng tối thiểu:

- Ở L7: access list, rate limit, challenge, custom rule phần pre, cache cơ bản.
- Gọi WAF với CRS PL1/PL2 và custom rule phần inspect.
- Swap snapshot nóng, governor theo tenant đơn giản.

| Đo | Ngưỡng để đạt |
|---|---|
| **Reload (T14):** đổi một rule của một zone dưới tải | Có hiệu lực ≤ 5 giây, 0 kết nối rớt, p99 tăng ≤ 10% |
| **Reload:** thêm 10.000 zone và cert dưới tải | Không reload process; RAM tăng theo kế hoạch |
| **Reload:** release CRS mới (lớp B) | Swap không lỗi request; ghi thời gian build và RAM đỉnh |
| **Reload:** số lần reload thật mỗi ngày với 100 thay đổi mỗi phút | 0 ở nhánh A; nhánh B đếm số lần reload và thời gian worker cũ thoát |
| **Reload:** nâng binary | Không rớt kết nối trong thời gian drain |
| Độ trễ thêm, request đi qua WAF, CRS PL1, tải 50% | p99 ≤ 2 ms |
| Độ trễ thêm, request bị chặn ở pre-filter hoặc cache hit được miễn WAF | p99 ≤ 0,5 ms |
| Tỉ lệ request bị loại ở L7 trong kịch bản flood từ ít nguồn | ≥ 95%, CPU tầng WAF không tăng quá 20% |
| `go-ftw`, CRS v4 | ≥ 99% pass ở PL1–PL2 |
| Cô lập tenant (QS-8) | p99 tenant khác tăng ≤ 10% |
| Cache: hit ratio trên corpus static, purge theo URL tới mọi node | Đo và so sánh; purge ≤ 5 s trong PoP |
| Bộ nhớ mỗi 10.000 kết nối giữ mở | Đo và so sánh |
| CPU tầng WAF mỗi 1.000 request (E1 vs E2) | Đo và so sánh |
| Fuzz L7 24 giờ | Không crash |
| Công sức tới đủ tính năng MVP | Ước người-tuần |

Kết quả ghi thành bản sửa của ADR này, trạng thái chuyển sang "Chấp nhận".

**Gợi ý nếu không làm spike:** **K3** (Go cả hai tầng) để ra MVP nhanh với đội hiện tại, giữ I1 làm
hợp đồng để sau thay L7 bằng N1 mà không đụng tầng WAF; **K2** nếu độ trễ đuôi của L7 và hiệu quả
RAM là cam kết bán hàng ngay từ đầu.

## 8. Ảnh hưởng tới ADR khác

| ADR | Phụ thuộc |
|---|---|
| [0030](0030-language-per-workload.md) | Ngôn ngữ L7 theo N đã chọn; tầng WAF mặc định Go (E1) |
| [0032](0032-cel-checked-at-control-ir-at-edge.md) | Custom rule phần pre chạy ở L7: cần IR nếu L7 không phải Go; phần inspect chạy ở WAF (Go) có thể dùng `cel-go` |
| [0026](0026-replay-on-request-samples-with-shared-pipeline.md) | Replay chạy code của cả hai tầng (thư viện, hoặc binary ở chế độ offline nếu khác ngôn ngữ) |
| [0035](0035-cdn-and-dns-strategy.md) | T5 (cache) nặng hay nhẹ tuỳ chiến lược CDN được chọn |
