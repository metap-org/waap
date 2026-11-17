#!/bin/bash
# Dựng toàn bộ nền thử nghiệm vào $WORK: build, cert, snapshot, cấu hình NGINX/ModSecurity, CRS 4.25.
# Cần: go, cargo (rust stable), protoc-gen-go/protoc-gen-go-grpc (đã có trong repo, không cần chạy lại),
#      nginx + libnginx-mod-http-modsecurity (apt), openssl, python3, curl.
set -euo pipefail
. "$(dirname "$0")/env.sh"
mkdir -p "$WORK" "$BIN"
S="$SPIKE_DIR"

echo "[1/5] build origin, waf, bench (Go)"
(cd "$S/origin" && go build -o "$BIN/origin" .)
(cd "$S/waf"    && go build -o "$BIN/waf" .)
(cd "$S/bench"  && go build -o "$BIN/bench" .)

echo "[2/5] build l7-pingora (Rust, lần đầu vài phút)"
(cd "$S/l7-pingora" && CARGO_TARGET_DIR="$TARGET_DIR" cargo build --release)

echo "[3/5] cert tự ký a/b/c.test"
for h in a b c; do
  [ -f "$WORK/$h.test.crt" ] || openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=$h.test" \
    -keyout "$WORK/$h.test.key" -out "$WORK/$h.test.crt" 2>/dev/null
done

echo "[4/5] CRS 4.25 + cấu hình ModSecurity cho NGINX (lấy từ module Go của Coraza, cùng bản với nhánh Coraza)"
CRSMOD=$(cd "$S/waf" && go list -m -f '{{.Dir}}' github.com/corazawaf/coraza-coreruleset/v4)
rm -rf "$WORK/crs" && mkdir -p "$WORK/crs"
cp "$CRSMOD"/rules/@owasp_crs/* "$WORK/crs/"
cp "$CRSMOD/rules/@crs-setup.conf.example" "$WORK/crs-setup.conf"
# libmodsecurity 3.0.12 không parse được SecRule nối (chain) không có action: thêm "t:none" vào các dòng đó.
sed -i -E 's/^(\s+SecRule [^ ]+ "[^"]*")$/\1 "t:none"/' "$WORK"/crs/*.conf
cp /etc/nginx/unicode.mapping "$WORK/unicode.mapping" 2>/dev/null || cp /usr/share/nginx/docs/modsecurity/unicode.mapping "$WORK/unicode.mapping"
cat > "$WORK/modsecurity.conf" <<EOT
# Bản rút gọn của modsecurity.conf-recommended: chỉ những chỉ thị ảnh hưởng tới phép đo.
SecRuleEngine On
SecRequestBodyAccess On
SecRule REQUEST_HEADERS:Content-Type "^application/json" "id:200001,phase:1,t:none,t:lowercase,pass,nolog,ctl:requestBodyProcessor=JSON"
SecRequestBodyLimit 13107200
SecRequestBodyNoFilesLimit 131072
SecResponseBodyAccess Off
SecPcreMatchLimit 500000
SecPcreMatchLimitRecursion 500000
SecAuditEngine Off
SecDebugLogLevel 0
SecUnicodeMapFile $WORK/unicode.mapping 20127
EOT
cat > "$WORK/modsec_main.conf" <<EOT
Include $WORK/modsecurity.conf
Include $WORK/crs-setup.conf
SecAction "id:900000,phase:1,pass,t:none,nolog,setvar:tx.blocking_paranoia_level=1"
Include $WORK/crs/*.conf
EOT

echo "[5/5] sinh nginx.conf, pingora.yaml, snapshot"
sed "s#@WORK@#$WORK#g" "$S/run/templates/nginx.conf.tpl" > "$WORK/nginx.conf"
: > "$WORK/extra.conf"
cat > "$WORK/pingora.yaml" <<EOT
---
version: 1
threads: 2
pid_file: $WORK/pingora.pid
upgrade_sock: $WORK/pingora_upgrade.sock
EOT
python3 "$S/run/gen_snapshot.py" base > "$WORK/snapshot.base.json"
python3 "$S/run/gen_snapshot.py" rule > "$WORK/snapshot.rule.json"
python3 "$S/run/gen_snapshot.py" big  > "$WORK/snapshot.big.json"
cp "$WORK/snapshot.base.json" "$WORK/snapshot.json"
"$NGINX_BIN" -t -c "$WORK/nginx.conf"
echo "OK. Tiếp theo: run/start.sh"
