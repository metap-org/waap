#!/bin/bash
# T14: đổi cấu hình nóng của Pingora dưới tải. Kỳ vọng (lần chạy của tôi): 0 lỗi, p99 ~1.6ms mọi giây; áp dụng 0.35ms / 52ms.
. "$(dirname "$0")/../lib.sh"
cp "$WORK/snapshot.base.json" "$WORK/snapshot.json"; sleep 1
bench_bg "$WORK/t01.out" -url $P/static/x.js -host b.test -rate 3000 -dur 16s -conns 64 -timeline
sleep 6;  echo "t=6s  đổi ACL/rps"; swap_snapshot "$WORK/snapshot.rule.json"
sleep 4;  echo "t=10s 10.000 zone + 1.000 cert"; swap_snapshot "$WORK/snapshot.big.json"
sleep 8;  summary "$WORK/t01.out"
echo "--- thời gian áp dụng do Pingora tự log:"; grep "applied in" "$WORK/l7.log" | tail -2
cp "$WORK/snapshot.base.json" "$WORK/snapshot.json"
