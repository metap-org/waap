#!/bin/bash
# LB 2 origin; tắt o1 ở t=4s, bật lại ở t=10s, 1500 rps. Kỳ vọng: 0 lỗi cả hai; Pingora p99.9 ~12ms lúc chuyển, NGINX ~3ms.
# Pingora dùng ~40 dòng mã tự viết (round robin + health TCP mỗi giây + retry); NGINX dùng passive max_fails.
. "$(dirname "$0")/../lib.sh"
for t in "$P pingora" "$N nginx"; do set -- $t
  bench_bg "$WORK/t05.out" -url $1/id -host lb.test -rate 1500 -dur 16s -conns 64 -timeline
  sleep 4; pkill -f "$BIN/origin -addr 127.0.0.1:9300"; echo "$2: o1 tắt"
  sleep 6; (nohup "$BIN/origin" -addr 127.0.0.1:9300 -name o1 > "$WORK/o1.log" 2>&1 &); echo "$2: o1 bật"
  sleep 8; echo "== $2"; summary "$WORK/t05.out"; sleep 2
done
