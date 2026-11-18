#!/bin/bash
# Lớp B (ADR-0037): build ruleset Coraza mới ở nền rồi swap, dưới tải trên zone có WAF. Kỳ vọng: build 11-22ms, 0 lỗi.
. "$(dirname "$0")/../lib.sh"
bench_bg "$WORK/t03.out" -url $P/page -host a.test -rate 800 -dur 14s -conns 32 -timeline
sleep 5; for p in 2 1; do echo "reload paranoia=$p: $(curl -s "127.0.0.1:9101/reload?paranoia=$p")"; sleep 3; done
sleep 4; P99_FLAG_MS=10 summary "$WORK/t03.out"
