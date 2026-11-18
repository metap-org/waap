#!/bin/bash
# Cùng kịch bản với 01 cho NGINX: reload cùng cấu hình (CRS 844 rule), rồi reload với 10.000 server block + 1.000 ssl.
# Kỳ vọng: 0 lỗi; reload 0.43s / 1.72s; RSS tạm thời 204->377MB / 610MB.
. "$(dirname "$0")/../lib.sh"
: > "$WORK/extra.conf"; "$NGINX_BIN" -s reload -c "$WORK/nginx.conf"; sleep 2
python3 - <<PY
w="$WORK"
p=[f'server {{ listen 127.0.0.1:8081; server_name z{i}.test; location / {{ proxy_pass http://127.0.0.1:9300; }} }}' for i in range(10000)]
p+=[f'server {{ listen 127.0.0.1:8444 ssl; server_name s{i}.test; ssl_certificate {w}/a.test.crt; ssl_certificate_key {w}/a.test.key; location / {{ proxy_pass http://127.0.0.1:9300; }} }}' for i in range(1000)]
open(w+"/extra.big.conf","w").write("\n".join(p)+"\n")
PY
echo "RSS nginx rảnh: $(rss_mb 'nginx: (master|worker)')MB"
bench_bg "$WORK/t02.out" -url $N/static/x.js -host b.test -rate 3000 -dur 20s -conns 64 -timeline
sleep 6; echo "t=6s reload cùng cấu hình"; s=$(date +%s.%N); "$NGINX_BIN" -s reload -c "$WORK/nginx.conf"; sleep 0.3
for i in $(seq 100); do [ "$(pgrep -f 'worker process is shutting down' | wc -l)" = 0 ] && break; sleep 0.1; done
python3 -c "print('  reload + worker cũ thoát: %.2fs'%($(date +%s.%N)-$s))"; echo "  RSS sau: $(rss_mb 'nginx: (master|worker)')MB"
sleep 3; cp "$WORK/extra.big.conf" "$WORK/extra.conf"; echo "t~10s reload với 10.000 server block"; s=$(date +%s.%N)
"$NGINX_BIN" -s reload -c "$WORK/nginx.conf"; peak=0
for i in $(seq 60); do r=$(rss_mb 'nginx: (master|worker)'); [ "$r" -gt "$peak" ] && peak=$r
  [ "$i" -gt 3 ] && [ "$(pgrep -f 'worker process is shutting down' | wc -l)" = 0 ] && break; sleep 0.2; done
python3 -c "print('  reload lớn + drain: %.2fs, RSS đỉnh ${peak}MB'%($(date +%s.%N)-$s))"
sleep 9; summary "$WORK/t02.out"; : > "$WORK/extra.conf"; "$NGINX_BIN" -s reload -c "$WORK/nginx.conf"
