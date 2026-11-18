#!/bin/bash
. "$(dirname "$0")/env.sh"
pkill -f "$PINGORA_BIN"; pkill -f "$BIN/origin"; pkill -f "$BIN/waf"
[ -f "$WORK/nginx.pid" ] && "$NGINX_BIN" -s stop -c "$WORK/nginx.conf"
echo stopped
