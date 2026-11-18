#!/bin/bash
# CPU mỗi request đi qua WAF + độ chặn đúng. CHƯA có số tổng hợp trong ADR: chạy để lấy số của bạn.
#  - Pingora+Coraza: CPU của tiến trình waf (qua :9101/stats) CỘNG CPU tiến trình Pingora, chia số request.
#    (Chỉ tính waf thì thiếu: so với nginx, nơi proxy và WAF cùng một tiến trình, là sai.)
#  - Lưu ý: nginx bị giới hạn 2 worker, còn tiến trình waf tách riêng có thể dùng thêm core; ở tải cao độ trễ nginx tăng
#    vì hết CPU của 2 worker, không phải vì ModSecurity chậm hơn Coraza theo CPU/request.
#  - NGINX+ModSecurity: CPU của worker nginx chia số request.
. "$(dirname "$0")/../lib.sh"
echo "--- độ chặn đúng (CRS PL1): benign=200, SQLi/XSS=403"
for t in "$P pingora" "$N nginx"; do set -- $t
  echo "$2: benign=$(curl -s -o /dev/null -w %{http_code} -H Host:a.test "$1/") sqli=$(curl -s -o /dev/null -w %{http_code} -H Host:a.test "$1/?id=1%27%20OR%20%271%27=%271") xss=$(curl -s -o /dev/null -w %{http_code} -H Host:a.test "$1/?x=%3Cscript%3Ealert(1)%3C/script%3E") (zone không WAF: $(curl -s -o /dev/null -w %{http_code} -H Host:b.test "$1/?x=%3Cscript%3E"))"; done
for r in 500 1500; do
  s0=$(curl -s 127.0.0.1:9101/stats); l0=$(cpu_of 'l7-pingora')
  "$BIN/bench" -url $P/page -host a.test -rate $r -dur 10s -conns 64 > "$WORK/t10p.out"; s1=$(curl -s 127.0.0.1:9101/stats); l1=$(cpu_of 'l7-pingora')
  python3 - "$r" "$s0" "$s1" "$(sed -n 2p "$WORK/t10p.out")" "$l0" "$l1" <<'PY'
import json,sys; r,a,b,o=sys.argv[1],json.loads(sys.argv[2]),json.loads(sys.argv[3]),sys.argv[4]
l=float(sys.argv[6])-float(sys.argv[5])
n=b['requests']-a['requests']; c=b['cpu_seconds']-a['cpu_seconds']
print(f"pingora+coraza rate={r}: {o} waf={c/max(n,1)*1000:.2f} + l7={l/max(n,1)*1000:.2f} = {(c+l)/max(n,1)*1000:.2f} ms/req (n={n})")
PY
  c0=$(cpu_of 'nginx: worker'); "$BIN/bench" -url $N/page -host a.test -rate $r -dur 10s -conns 64 > "$WORK/t10n.out"; c1=$(cpu_of 'nginx: worker')
  python3 -c "print(f'nginx+modsec  rate=$r: $(sed -n 2p "$WORK/t10n.out") nginx_cpu={($c1-$c0)/($r*10)*1000:.2f} ms/req')"
done
