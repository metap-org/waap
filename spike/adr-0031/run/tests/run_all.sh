#!/bin/bash
# Chạy toàn bộ bộ đo theo thứ tự, ghi log vào $WORK/results/. Mất ~10 phút. Mỗi test độc lập, có thể chạy riêng.
. "$(dirname "$0")/../env.sh"; mkdir -p "$WORK/results"
"$(dirname "$0")/../start.sh"
for t in "$(dirname "$0")"/[0-9]*.sh; do n=$(basename "$t" .sh); echo "##### $n"; bash "$t" 2>&1 | tee "$WORK/results/$n.txt"
  "$(dirname "$0")/../start.sh" >/dev/null; done   # khởi động lại sạch giữa các test (RSS, snapshot, origin)
