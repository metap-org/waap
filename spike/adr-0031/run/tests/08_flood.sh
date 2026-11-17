#!/bin/bash
# Pre-filter: mọi request bị ACL chặn 403 (zone d.test). Đo CPU mỗi request của từng bên ở 20k/40k/60k rps.
# Kỳ vọng (jemalloc, 2 thread/worker): Pingora 0.026/0.021/0.017 ms; NGINX 0.017/0.013/0.011 ms.
# Để tách "framework" khỏi "mã của spike": chạy lại với  BASELINE=1 ./start.sh  (Pingora từ chối ngay đầu request_filter).
# Nếu CPU/req cao bất thường: kiểm tra phản hồi 403 có giữ keep-alive không (curl -v hai lần liền phải "Re-using").
. "$(dirname "$0")/../lib.sh"
for r in 20000 40000 60000; do
  for t in "8080 l7-pingora pingora" "8081 nginx:.(master|worker) nginx"; do set -- $t
    c0=$(cpu_of "$2"); "$BIN/bench" -url http://127.0.0.1:$1/x -host d.test -rate $r -dur 10s -conns 256 > "$WORK/t08.out" 2>&1
    c1=$(cpu_of "$2"); n=$(grep -o 'sent=[0-9]*' "$WORK/t08.out" | cut -d= -f2)
    echo "$3 rate=$r $(grep -o 'errors=[0-9]*' "$WORK/t08.out") $(grep -o 'achieved=[0-9]* rps' "$WORK/t08.out") $(sed -n 2p "$WORK/t08.out") cpu_ms/req=$(python3 -c "print(round(($c1-$c0)*1000/$n,4))")"
  done; done
