#!/bin/bash
# Nâng cấp binary không rớt kết nối. Kỳ vọng: 0 lỗi ở cả hai.
# Pingora: tiến trình mới chạy với -u, rồi QUIT tiến trình cũ.  NGINX: USR2 -> WINCH -> QUIT (cần NGINX_BIN tuyệt đối).
. "$(dirname "$0")/../lib.sh"
old=$(pgrep -f "$PINGORA_BIN -c" | head -1); echo "Pingora cũ pid=$old"
bench_bg "$WORK/t04p.out" -url $P/static/x.js -host b.test -rate 2000 -dur 14s -conns 64 -timeline
sleep 4; nohup "$PINGORA_BIN" -c "$WORK/pingora.yaml" -u -d > "$WORK/l7.up.log" 2>&1 &
sleep 2; kill -QUIT "$old"; sleep 9; echo "== Pingora"; summary "$WORK/t04p.out"
bench_bg "$WORK/t04n.out" -url $N/static/x.js -host b.test -rate 2000 -dur 12s -conns 64 -timeline
sleep 3; m=$(cat "$WORK/nginx.pid"); kill -USR2 "$m"
for i in $(seq 30); do [ -f "$WORK/nginx.pid.oldbin" ] && break; sleep 0.2; done; sleep 1
n=$(cat "$WORK/nginx.pid"); echo "nginx master cũ=$m mới=$n"; [ "$m" != "$n" ] || { echo "USR2 không có tác dụng: xem $WORK/nginx-error.log (execve)"; exit 1; }
kill -WINCH "$m"; sleep 1; kill -QUIT "$m"; sleep 8; echo "== NGINX"; summary "$WORK/t04n.out"
