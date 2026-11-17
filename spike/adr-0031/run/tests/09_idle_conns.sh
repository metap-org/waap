#!/bin/bash
# RAM cho 10.000 kết nối rảnh. Kỳ vọng: Pingora ~19-31 KB/kết nối (19 với jemalloc, cả ở BASELINE); NGINX ~0.5 KB.
# Đo bằng RSS nên nhiễu giữa các lần chạy liền nhau: restart tiến trình giữa các lần (./start.sh) và so delta, không so tuyệt đối.
. "$(dirname "$0")/../lib.sh"
ulimit -n 65000 2>/dev/null || echo "(cảnh báo: không nâng được ulimit -n; có thể không mở đủ 10.000 kết nối)"
python3 "$(dirname "$0")/../idle.py" 8080 'l7-pingora'
python3 "$(dirname "$0")/../idle.py" 8081 'nginx: (master|worker)'
