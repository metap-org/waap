#!/bin/bash
# Khởi động: 2 origin (9300 o1, 9301 o2), WAF Coraza (gRPC 9100, admin 9101), Pingora (8080/8443), NGINX (8081/8445).
# Biến: BASELINE=1 (Pingora từ chối ngay, đo chi phí framework), PINGORA_BIN=<binary khác>, ONLY=pingora|nginx.
. "$(dirname "$0")/env.sh"
cd "$WORK"
run() { local name=$1; shift; nohup "$@" > "$WORK/$name.log" 2>&1 & echo $! > "$WORK/$name.pid.sh"; }
pgrep -f "$BIN/origin" >/dev/null || { run o1 "$BIN/origin" -addr 127.0.0.1:9300 -name o1; run o2 "$BIN/origin" -addr 127.0.0.1:9301 -name o2; }
pgrep -f "$BIN/waf" >/dev/null || run waf "$BIN/waf"
if [ "${ONLY:-}" != nginx ]; then
  pkill -f "$PINGORA_BIN" 2>/dev/null; sleep 0.5
  SNAPSHOT="$WORK/snapshot.json" run l7 env ${BASELINE:+BASELINE=1} SNAPSHOT="$WORK/snapshot.json" "$PINGORA_BIN" -c "$WORK/pingora.yaml"
fi
if [ "${ONLY:-}" != pingora ]; then
  [ -f "$WORK/nginx.pid" ] && "$NGINX_BIN" -s stop -c "$WORK/nginx.conf" 2>/dev/null; sleep 0.5
  "$NGINX_BIN" -c "$WORK/nginx.conf"
fi
sleep 2
echo "pingora: $(curl -s -o /dev/null -w '%{http_code}' -H Host:b.test $P/id)  nginx: $(curl -s -o /dev/null -w '%{http_code}' -H Host:b.test $N/id)  (200 = sống)"
