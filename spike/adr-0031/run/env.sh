# Biến môi trường dùng chung cho mọi script. Ghi đè bằng cách export trước khi chạy.
SPIKE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export WORK="${WORK:-/tmp/adr0031}"            # thư mục chạy: cert, snapshot, log, binary
export TARGET_DIR="${TARGET_DIR:-$WORK/target}"   # CARGO_TARGET_DIR (build Rust nặng, dùng lại giữa các lần)
export BIN="$WORK/bin"
export PINGORA_BIN="${PINGORA_BIN:-$TARGET_DIR/release/l7-pingora}"
export NGINX_BIN="${NGINX_BIN:-/usr/sbin/nginx}"   # PHẢI là đường dẫn tuyệt đối (USR2 exec lại đúng binary này)
export P=http://127.0.0.1:8080                  # Pingora (HTTP)
export N=http://127.0.0.1:8081                  # NGINX  (HTTP)
export PATH="$BIN:$PATH:/root/go/bin:$HOME/go/bin"
export NO_PROXY='*' no_proxy='*'                # sandbox có HTTPS_PROXY; curl tới 127.0.0.1 không được đi qua proxy
