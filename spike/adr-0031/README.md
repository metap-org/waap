# Spike ADR-0031: L7 Pingora so với NGINX (+ WAF Coraza / ModSecurity)

Tài liệu này để **tự chạy lại, tự đo, và khoanh vùng vấn đề nằm ở đâu**. Kết luận và bảng số liệu nằm ở
[ADR-0031 §7.1](../../docs/architecture/09-adr/0031-edge-l7-and-waf-engine-options.md); log từng đợt ở [`RESULTS.md`](RESULTS.md).
Đây là **mã spike**, không phải mã sản phẩm: dùng để đo và so sánh, không để triển khai.

## 1. Có gì trong thư mục

| Thư mục | Vai trò |
|---|---|
| `l7-pingora/` | L7 viết bằng Pingora 0.9.1: snapshot cấu hình hoán đổi nguyên tử (ArcSwap), pre-filter ACL/rps, gọi WAF qua gRPC, cert theo SNI động, LB + health check, cache + PURGE, HTTP/2. Dùng jemalloc. `BASELINE=1` bỏ qua toàn bộ logic L7 (xem §5) |
| `waf/` | Dịch vụ WAF Coraza v3.8.1 + CRS 4.25 (gRPC `Inspect`, cổng 9100; admin `/reload`, `/stats` cổng 9101) |
| `origin/` | Origin giả: `/static/*`, `/page`, `/api/echo`, `/slow`, `/id`, `/cached`, `/stream?mb=N`, `/ws` (Upgrade echo) |
| `bench/` | Bộ sinh tải open-loop (tính cả độ trễ do xếp hàng, tránh coordinated omission) |
| `proto/` | Hợp đồng `Inspection.Inspect` (I1) |
| `run/` | Script dựng môi trường, khởi động, và 10 bài đo (`run/tests/`) |

Cổng: Pingora `8080` (HTTP) / `8443` (TLS+H2) · NGINX `8081` / `8445` (TLS+H2) · origin `9300`, `9301` · WAF `9100`/`9101`.

## 2. Chạy

Cần: `go`, `cargo`, `nginx` + `libnginx-mod-http-modsecurity` (apt), `openssl`, `python3`, `curl`. Docker không cần.

```bash
cd spike/adr-0031/run
./setup.sh          # build, cert, CRS 4.25, nginx.conf, snapshot  -> /tmp/adr0031  (đổi bằng WORK=...)
./start.sh          # origin x2, waf, pingora, nginx
tests/run_all.sh    # toàn bộ bài đo, ~10 phút, kết quả ở $WORK/results/
tests/08_flood.sh   # hoặc từng bài riêng
./stop.sh
```

`TARGET_DIR=<dir>` dùng lại thư mục build Rust giữa các lần. `NGINX_BIN` phải là đường dẫn **tuyệt đối** (USR2 exec lại đúng
binary đó). Sandbox có `HTTPS_PROXY`: `env.sh` đặt `NO_PROXY='*'` để curl tới 127.0.0.1 không đi qua proxy.

Lưu ý khi đối chiếu số với ADR: CRS cho NGINX trong `setup.sh` nạp **838** rule (lần đo gốc 844, dùng `modsecurity.conf` đầy đủ
hơn); `setup.sh` ghi bản rút gọn chỉ gồm chỉ thị ảnh hưởng tới phép đo. Máy khác sẽ ra số tuyệt đối khác; **so sánh Pingora với NGINX
trong cùng một lần chạy**, đừng so số tuyệt đối giữa các máy.

## 3. Bảng bài đo: chạy gì, đo gì, kỳ vọng, nghi vấn thì xem đâu

| Script | Đo | Kết quả của tôi (4 core, loopback) | Nếu số của bạn khác nhiều thì xem |
|---|---|---|---|
| `01_reload_pingora` | Đổi rule nóng; 10.000 zone + 1.000 cert, 3000 rps | áp dụng 0,34 ms / 52 ms; 0 lỗi; p99 ≤ 1,7 ms | `grep applied $WORK/l7.log`; lỗi "snapshot rejected" = file ghi dở (dùng `swap_snapshot`, ghi tạm rồi `mv`) |
| `02_reload_nginx` | Reload cùng cấu hình; reload 10.000 server block | 0,43 s / 1,72 s; RSS 204→377 / 610 MB; 0 lỗi | `$WORK/nginx-error.log` (cảnh báo `server_names_hash` vô hại) |
| `03_waf_class_b` | Swap ruleset Coraza dưới tải | build 11–22 ms, 0 lỗi | `curl 127.0.0.1:9101/stats` |
| `04_upgrade` | Nâng binary: Pingora `-u`, NGINX USR2 | 0 lỗi cả hai; NGINX bàn giao ~2 s | USR2 không đổi pid master: xem `execve()` trong `nginx-error.log` (đường dẫn `nginx` phải tuyệt đối) |
| `05_lb_failover` | Tắt/bật 1 origin ở 1500 rps | NGINX 0 lỗi; Pingora 0 lần đầu, **1/24.000 phản hồi 502** lần chạy lại; p99.9 Pingora 5–12 ms, NGINX ~3 ms. **Đọc dòng `codes=map[...]`, cột `errors=` của bench chỉ đếm lỗi truyền tải, không đếm 502** | Pingora: health check TCP mỗi giây + retry là mã tự viết trong `l7-pingora`; NGINX chỉ passive |
| `06_features` | H2, upload 200 MB, stream 200 MB, WebSocket | đều qua | NGINX 413 = thiếu `client_max_body_size 0`; WS 400 = thiếu 3 dòng Upgrade |
| `07_cache` | Cache + PURGE; hit ở 6000 rps | Pingora purge được; NGINX OSS không. Độ trễ hit **tương đương** (p99 ~1,4–1,5 ms) | Số hit-latency dao động mạnh nếu máy đang có tải khác (lần đo đầu nhiễu, xem ADR) |
| `08_flood` | CPU/request khi chặn ở pre-filter, 20k/40k/60k rps | Pingora 0,028/0,018/0,014 ms; NGINX 0,019/0,013/0,011 ms | §5: `BASELINE=1` để tách framework khỏi logic; kiểm tra keep-alive của 403 |
| `09_idle_conns` | RAM 10.000 kết nối rảnh | Pingora ~19–31 KB/kết nối, NGINX ~0,5 KB | §6; restart giữa các lần đo |
| `10_waf_cpu` | Độ chặn đúng CRS PL1 + CPU mỗi request qua WAF | Pingora+Coraza 1,24–1,44 ms/req (WAF 0,8–0,9 + L7 0,4–0,55); NGINX+ModSecurity 1,14–1,29 ms/req | xem chú thích trong script về giới hạn 2 worker |

Chưa đo: A2 thật (Pingora + libmodsecurity), `go-ftw`, JA4, fuzz 24 giờ, cô lập tenant, HTTP/3, **Envoy** (tải binary bị chặn
trong môi trường làm spike). Muốn thêm Envoy: dựng `envoy.yaml` cùng 4 zone, thêm cổng, và viết bản `*_envoy` của các bài trên.

## 4. Cách đọc kết quả: tránh những lỗi tôi đã mắc

Mỗi mục dưới đây đã làm sai một con số trong lần đo đầu. Kiểm tra trước khi tin một kết quả "X chậm hơn Y".

1. **Công bằng cấu hình.** Pingora `threads: 2` thì NGINX `worker_processes 2`. NGINX cần `worker_connections` đủ lớn (test 10.000 kết nối),
   `client_max_body_size 0` (upload lớn), 3 dòng Upgrade (WebSocket). Lần đầu tôi đo RAM NGINX = +0 MB vì cấu hình chỉ cho ~8.000 kết nối.
2. **Keep-alive của phản hồi tự sinh.** `Session::respond_error` của Pingora **luôn đóng kết nối** (có ghi chú trong pingora-core). Dùng nó
   cho 403/429 thì mỗi request bị chặn là một kết nối TCP mới: lần đo đầu cho Pingora 0,45 ms CPU/req và bão hòa ở 32k rps, sai 15–30 lần.
   Spike dùng hàm `deny()` giữ keep-alive. Kiểm tra: `curl -sv -H Host:d.test $P/x $P/x` phải thấy "Re-using existing connection".
3. **Đếm đủ CPU.** Pingora+Coraza là **hai** tiến trình (L7 + WAF); NGINX+ModSecurity là một. Cộng cả hai (script 10 làm vậy).
   NGINX bị chặn ở 2 worker nên khi hết CPU thì độ trễ vọt lên, không có nghĩa ModSecurity chậm hơn theo CPU/request.
4. **Nhiễu khi chạy bài khác song song.** Số p99 của cache lần đầu (4,15 vs 2,66 ms) là nhiễu; chạy lại cho 1,46 vs 1,40. Chỉ chạy một bài một lúc,
   lặp ≥ 2 lần, và so sánh trong cùng lần chạy.
5. **Ghi snapshot nguyên tử.** Ghi thẳng vào `snapshot.json` làm watcher đọc file dở: Pingora báo "rejected" và giữ bản cũ (đúng thiết kế), nhưng phép đo mất ý nghĩa.
6. **RSS nhiễu.** Bộ nhớ đã cấp không trả ngay cho OS; luôn restart tiến trình giữa các lần đo RAM và đọc **delta**.
7. **H2 dùng `:authority`, không có header `Host`.** Lần đầu Pingora trả 404 trên H2 vì mã spike chỉ đọc `Host` (đã sửa).

## 5. Khoanh vùng: chậm là do framework, do mã L7 của spike, hay do cấu hình?

1. Chạy `08_flood.sh` bình thường, ghi `cpu_ms/req`.
2. `BASELINE=1 ./start.sh` rồi chạy lại `08_flood.sh`: Pingora từ chối ngay đầu `request_filter`, không tra zone, không ACL, không rps.
   - Số gần như không đổi → chi phí là của **framework Pingora** (kết quả của tôi: logic spike chỉ thêm 1–10%).
   - Số giảm nhiều → chi phí nằm trong mã L7 của spike (`request_filter`: cấp phát chuỗi host/IP, ACL quét tuyến tính).
3. So với NGINX cùng `rate`. Chênh vài chục phần trăm ở pre-filter là bình thường giữa hai kiến trúc; chênh nhiều lần thường là cấu hình (mục 4).
4. Profile CPU (cần `perf`, không có trong sandbox của tôi): 
   ```bash
   perf record -F 999 -g -p $(pgrep -f 'release/l7-pingora -c' | head -1) -- sleep 15   # trong lúc chạy 08_flood
   perf report --no-children | head -60      # hoặc: perf script | inferno-collapse-perf | inferno-flamegraph > flame.svg
   ```
   Tìm tỉ trọng: `tokio` scheduler, `openssl`/TLS, `http` parser, `malloc`/`free`, syscall `epoll`/`write`.
5. Nếu nghi mã spike: các điểm đáng sửa trước là `request_filter` (host/IP `to_string`, `acl_deny` là `Vec<String>`), mỗi cái là một lần cấp phát mỗi request.

## 6. Khoanh vùng RAM mỗi kết nối rảnh (Pingora ~19–31 KB, NGINX ~0,5 KB)

- `BASELINE=1` vẫn ~19 KB/kết nối → là của framework (task tokio + session HTTP/1 + buffer header), **không** phải mã spike.
- jemalloc (đã bật trong `l7-pingora`) giảm từ ~31 KB xuống ~19–20 KB; glibc cho 31 KB.
- Chưa tách được thành phần bên trong. Cách làm: build `l7-pingora` với `MALLOC_CONF=prof:true,lg_prof_interval:30` (jemalloc
  profiling) hoặc `heaptrack` trong lúc `09_idle_conns.sh` đang giữ 10.000 kết nối, rồi xem các call site cấp phát lớn nhất.
- Khi nào đáng lo: 1 triệu kết nối rảnh trên một node ≈ 19 GB (Pingora) so với ≈ 0,5 GB (NGINX). Với vài chục nghìn kết nối mỗi node thì không đáng kể.

## 7. Giới hạn của phép đo

- Một máy, loopback, 4 core, bench và server chia chung CPU; số **tương đối**, không phải benchmark production.
- Không có TLS trong các bài tải (chỉ H2 kiểm tra chức năng); không có mạng thật (RTT = 0), không có origin chậm.
- Pingora cache chỉ có `MemCache` (dùng cho test), chưa có tầng đĩa, eviction, cache lock; so sánh cache chỉ là cache hit trên bộ nhớ.
- `waf/` dùng một ruleset toàn cục + `RemoveRuleByID` theo zone; chưa kiểm chứng go-ftw (độ khớp CRS).
