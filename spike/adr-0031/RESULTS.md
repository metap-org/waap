# Spike ADR-0031 — kết quả đo reload / upgrade (một phần)

Môi trường: 1 máy sandbox, loopback, open-loop load generator (`bench/`). Số liệu mang tính **so sánh tương đối**, không phải benchmark production.

## Đã đo

| # | Thử nghiệm | A1 Pingora (+Coraza callout) | B NGINX 1.24 + libmodsecurity 3.0.12 + CRS 4.25 |
|---|---|---|---|
| R1 | Đổi rule ACL/rps của 1 zone dưới tải 3000 rps | Áp dụng ~0.35 ms (swap snapshot, không reload). 0 lỗi, p99 ≤ 1.6 ms | Phải `nginx -s reload` |
| R2 | Đổi cấu hình lớn: 10 000 zone + 1 000 cert | Snapshot swap ~52 ms, 0 lỗi, p99 1.6 ms, RSS 15→27 MB | Reload 1.72 s gồm drain worker cũ; RSS 204→610 MB đỉnh (CRS parse lại + 2 thế hệ worker); 0 lỗi nhưng p99 lên tối đa 2.9 ms |
| R3 | Reload cùng cấu hình (CRS 844 rule) | n/a | 0.43 s, RSS 204→377 MB tạm thời |
| R4 | Class B: build-then-swap ruleset Coraza (đổi paranoia) dưới 800 rps trên zone có WAF | 11–22 ms build, swap atomic, 0 lỗi, p99 không đổi (≈4–5 ms) | n/a (là reload) |
| R5 | Nâng cấp binary dưới tải 2000 rps | `-u` graceful upgrade: 0 lỗi, p99 ≤ 1.75 ms (1 giây 3.2 ms). Tiến trình cũ còn sống >10 s (grace period) | USR2 → WINCH → QUIT: handover ~2 s, 0 lỗi, p99 1.75 ms |

Kết luận tạm về tiêu chí "ít reload": cả hai đều **không rớt request** khi reload ở quy mô thử; khác biệt nằm ở **chi phí** — nginx reload tốn RAM tạm thời gấp ~2–3 lần và thời gian tỷ lệ với số server block/rule, còn Pingora đổi cấu hình bằng swap snapshot không tạo tiến trình mới (class A) và ruleset WAF bằng swap nguyên tử (class B).

## Lỗi trong quá trình đo (ghi lại để minh bạch)
- Lần USR2 đầu tiên cho 15 000/24 000 lỗi: **lỗi script của tôi** (gọi `nginx` bằng đường dẫn tương đối nên `execve()` thất bại, sau đó tôi gửi QUIT cho master cũ và giết luôn nginx). Đã chạy lại với `/usr/sbin/nginx`; kết quả ở bảng là lần chạy lại.
- Một lần swap snapshot Pingora bị từ chối ("EOF while parsing") do script ghi file không nguyên tử; watcher NACK đúng, giữ snapshot cũ. Cần ghi temp + rename.

## Chưa đo (không kết luận)
- Flood / prefilter, bộ nhớ trên 10k kết nối rảnh, cache + purge (pingora-cache vs proxy_cache).
- HTTP/3, JA4, go-ftw (độ tương thích CRS) chưa chạy.
- A2 (Pingora + libmodsecurity) mới chỉ xấp xỉ bằng nginx + libmodsecurity, chưa có callout riêng.
- So sánh CPU/request giữa A1 và B đã có số lẻ trong phiên làm việc nhưng chưa tổng hợp vào đây.


## Bổ sung (cùng ngày): tính năng L7, flood, RAM kết nối

Xem bảng đầy đủ và nhận định ở ADR-0031 §7.1. Mã thêm vào spike: LB + health check + retry, cache + PURGE (`pingora-cache`, `MemCache`), HTTP/2, đọc `:authority`.

Đính chính: số flood ban đầu của Pingora sai do mã spike dùng `respond_error` (đóng kết nối); sau khi sửa, Pingora ~1,5 lần CPU của NGINX ở pre-filter. RAM kết nối rảnh của Pingora (31 KB; 20 KB với jemalloc) cao hơn nhiều so với NGINX (0,5 KB), chưa giải thích được. NGINX OSS thiếu purge và health check chủ động; Pingora thiếu HTTP/3 và tầng cache đĩa.

Profile (đợt 3): chế độ BASELINE cho thấy logic L7 của spike chỉ thêm 1–10% CPU; chênh NGINX (~1,2x ở 60k rps) và RAM 19 KB/kết nối rảnh là của framework Pingora. Envoy chưa đo (tải binary bị chặn).
