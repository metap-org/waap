#!/bin/bash
# Tính năng L7: HTTP/2, upload 200MB, tải stream 200MB, WebSocket. Kỳ vọng: đều qua ở cả hai (NGINX cần cấu hình đúng, xem template).
. "$(dirname "$0")/../lib.sh"
echo "--- H2 (TLS+ALPN)"
curl -sk --http2 -o /dev/null -w "pingora: http=%{http_version} code=%{http_code}\n" --resolve b.test:8443:127.0.0.1 https://b.test:8443/id
curl -sk --http2 -o /dev/null -w "nginx:   http=%{http_version} code=%{http_code}\n" --resolve b.test:8445:127.0.0.1 https://b.test:8445/id
[ -f "$WORK/big200.bin" ] || head -c 209715200 /dev/zero > "$WORK/big200.bin"
echo "--- upload 200MB"
for t in "$P pingora" "$N nginx"; do set -- $t; s=$(date +%s.%N)
  r=$(curl -s -H Host:b.test --data-binary @"$WORK/big200.bin" -H "Content-Type: application/octet-stream" $1/api/echo | head -c 60)
  python3 -c "print('$2: $r in %.2fs'%($(date +%s.%N)-$s))"; done
echo "--- stream 200MB"
for t in "$P pingora" "$N nginx"; do set -- $t
  curl -s -o /dev/null -H Host:b.test -w "$2: %{size_download}B in %{time_total}s ttfb=%{time_starttransfer}s\n" "$1/stream?mb=200"; done
echo "--- WebSocket (Upgrade tunnel, echo thô)"
python3 - <<'PY'
import socket
for name,p in (("pingora",8080),("nginx",8081)):
    s=socket.create_connection(("127.0.0.1",p),3)
    s.sendall(b"GET /ws HTTP/1.1\r\nHost: b.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: x\r\nSec-WebSocket-Version: 13\r\n\r\n")
    r=s.recv(200); s.sendall(b"hello"); s.settimeout(2)
    try: e=s.recv(20)
    except Exception: e=b"ERR"
    print(name,"101" if b"101" in r else r[:30],e)
PY
