#!/bin/bash
# Cache + purge. /cached trả "<origin> <nanogiây>": hai lần liền cho cùng giá trị = cache hit.
# Kỳ vọng: Pingora (MemCache, test-only) hit, PURGE xoá được; NGINX OSS hit nhưng KHÔNG có purge (PURGE chỉ tới origin).
. "$(dirname "$0")/../lib.sh"
echo "== pingora: 3 GET, PURGE, 2 GET"; for i in 1 2 3; do curl -s -H Host:ca.test $P/cached; echo; done
curl -s -X PURGE -o /dev/null -w "PURGE -> %{http_code}\n" -H Host:ca.test $P/cached; for i in 1 2; do curl -s -H Host:ca.test $P/cached; echo; done
echo "== nginx: 3 GET, PURGE (không có module nên vô tác dụng)"; for i in 1 2 3; do curl -s -H Host:ca.test $N/cached; echo; done
curl -s -X PURGE -o /dev/null -w "PURGE -> %{http_code} (là phản hồi của origin, không phải purge)\n" -H Host:ca.test $N/cached; curl -s -H Host:ca.test $N/cached; echo
echo "== cache hit 6000 rps (kỳ vọng p99: Pingora ~4.2ms, NGINX ~2.7ms)"
for t in "$P pingora" "$N nginx"; do set -- $t; "$BIN/bench" -url $1/cached -host ca.test -rate 6000 -dur 8s -conns 64 | sed -n 1,2p | sed "s/^/$2: /"; done
