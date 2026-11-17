# Hàm dùng chung cho các test. source file này sau env.sh.
. "$(dirname "${BASH_SOURCE[0]}")/env.sh"
# CPU (giây) đã dùng bởi mọi tiến trình khớp mẫu pgrep -f
cpu_of() { for p in $(pgrep -f "$1"); do awk '{print $14+$15}' /proc/$p/stat; done | awk '{s+=$1} END{print s/100}'; }
rss_mb() { for p in $(pgrep -f "$1"); do awk '/VmRSS/{print $2}' /proc/$p/status; done | awk '{s+=$1} END{print int(s/1024)}'; }
# Ghi snapshot Pingora NGUYÊN TỬ (ghi file tạm rồi rename). Ghi thẳng dễ làm watcher đọc file dở dang.
swap_snapshot() { cp "$1" "$WORK/snapshot.tmp" && mv "$WORK/snapshot.tmp" "$WORK/snapshot.json"; }
# bench <url-path> <host> <port> <rate> <dur> <conns> [extra flags]  -> in tóm tắt + timeline ra $OUT
bench_bg() { local out=$1; shift; ( "$BIN/bench" "$@" > "$out" 2>&1 & ); }
summary() { head -2 "$1"; echo "giây có lỗi hoặc p99>2.5ms:"; awk -v T="${P99_FLAG_MS:-2.5}" '/^t=/{split($3,a,"=");sub("ms","",a[2]); if ($4!="err=0" || a[2]+0>T) print "  "$0}' "$1"; }
