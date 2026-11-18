# NGINX đối chứng. 2 worker để ngang 2 thread của Pingora. Mỗi mục có chú thích vì sao cấu hình như vậy
# (nhiều kết quả "NGINX lỗi" lúc đầu thực ra là do thiếu các dòng này).
load_module modules/ngx_http_modsecurity_module.so;
worker_processes 2;
pid @WORK@/nginx.pid;
error_log @WORK@/nginx-error.log warn;
events { worker_connections 20000; }   # mặc định 512-4096 làm test 10.000 kết nối rảnh sai
http {
  access_log off;
  client_max_body_size 0;               # mặc định 1m: upload lớn trả 413
  map $http_upgrade $connection_upgrade { default upgrade; "" close; }   # WebSocket cần 3 dòng cấu hình
  proxy_cache_path @WORK@/nginx-cache levels=1:2 keys_zone=z:10m max_size=100m inactive=10m;
  upstream lb { server 127.0.0.1:9300 max_fails=1 fail_timeout=5s; server 127.0.0.1:9301 max_fails=1 fail_timeout=5s; }
  include @WORK@/extra.conf;            # test cấu hình lớn ghi 10.000 server block vào đây

  server { listen 127.0.0.1:8081; server_name a.test;      # zone có WAF: ModSecurity + CRS 4.25
    modsecurity on; modsecurity_rules_file @WORK@/modsec_main.conf;
    location / { proxy_pass http://127.0.0.1:9300; proxy_set_header Host $host; } }
  server { listen 127.0.0.1:8081; server_name b.test;      # zone không WAF
    location / { proxy_pass http://127.0.0.1:9300; proxy_set_header Host $host; proxy_http_version 1.1;
                 proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection $connection_upgrade; } }
  server { listen 127.0.0.1:8445 ssl http2; server_name b.test;
    ssl_certificate @WORK@/b.test.crt; ssl_certificate_key @WORK@/b.test.key;
    location / { proxy_pass http://127.0.0.1:9300; } }
  server { listen 127.0.0.1:8081; server_name lb.test;     # LB: chỉ có passive health check (OSS)
    location / { proxy_pass http://lb; proxy_next_upstream error timeout; proxy_connect_timeout 300ms; } }
  server { listen 127.0.0.1:8081; server_name d.test; deny 127.0.0.1;   # pre-filter ACL
    location / { proxy_pass http://127.0.0.1:9300; } }
  server { listen 127.0.0.1:8081; server_name ca.test;     # cache (OSS không có purge)
    location / { proxy_pass http://127.0.0.1:9300; proxy_cache z; proxy_cache_key $host$request_uri;
                 add_header X-Cache $upstream_cache_status; } }
}
