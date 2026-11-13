#!/usr/bin/env python3
"""Sinh playbook cài từng ứng dụng đứng riêng (labs/ansible/apps/*.yml). Sửa danh sách ở đây rồi chạy lại, đừng sửa tay."""
import json
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent.parent / "ansible" / "apps"

SIMPLE = {
    "postgres": ("svc_postgres", [("tcp", [5432])], {"pg_allowed_cidrs": "{{ app_allow_from }}"}, "PostgreSQL 16, scram, TLS, chỉ nhận từ app_allow_from."),
    "rabbitmq": ("svc_rabbitmq", [("tcp", [5672])], {}, "RabbitMQ, không guest, tài khoản ứng dụng."),
    "redis": ("svc_redis", [("tcp", [6379])], {}, "Redis có ACL: user control ghi, user edge chỉ đọc waf:*. Dùng cho valkey-edge và valkey-central."),
    "clickhouse": ("svc_clickhouse", [("tcp", [9000])], {"clickhouse_allowed_cidrs": "{{ app_allow_from }}"}, "ClickHouse. Cần ghim clickhouse_debs (url, sha256)."),
    "ingress": ("svc_perimeter", [("tcp", [443])], {"perimeter_ingress": True, "perimeter_egw": False, "perimeter_egress": False}, "nginx cổng vào portal. Cần perimeter_backend_nodes."),
    "egw": ("svc_perimeter", [("tcp", [8443])], {"perimeter_ingress": False, "perimeter_egw": True, "perimeter_egress": False}, "Relay telemetry của edge tới control-plane. Cần perimeter_backend_nodes."),
    "egress": ("svc_perimeter", [("tcp", [3128])], {"perimeter_ingress": False, "perimeter_egw": False, "perimeter_egress": True, "egress_allowed_src": "{{ app_allow_from }}"}, "squid ra Internet có allowlist, chặn dải riêng và metadata."),
    "origin": ("svc_origin", [("tcp", [80])], {}, "Origin giả để thử proxy."),
    "loadgen": ("svc_loadgen", [], {}, "Máy tải. Cần loadgen_edge_ip."),
    "dns-mock": ("svc_dns_mock", [("tcp", [53, 8053]), ("udp", [53])], {"dns_mock_binary_src": "{{ app_bin_dir }}/dns-mock"}, "DNS và DoH giả cho dev/staging. Binary từ lab build."),
    "edge": ("svc_edge_app", [("tcp", [8080])], {"edge_binary_src": "{{ app_bin_dir }}/waap-edge"}, "waap-edge ngoài K8s. Cần edge_ingest_url; chạy cùng apps/redis hoặc trỏ edge_redis_url."),
    "k3s-node": ("profile_k8s_node", [("tcp", [6443, 10250])], {}, "Node k3s (server hoặc agent: -e k3s_role=agent -e k3s_api_endpoint=...). Cần k3s_version, k3s_sha256, k3s_token."),
}

# name -> (lệnh/nhị phân, cổng, cổng phụ, env mặc định, khoá bắt buộc, mô tả, health)
SERVICES = {
    "zones-service": ("zones-service", 3000, [3001], {"PORT": "3000", "GRPC_ENABLED": "true"}, ["DATABASE_URL", "RABBITMQ_URL", "JWKS_KEYS_DIR"], "Issuer JWKS duy nhất; cần thư mục khoá (app_files) và JWKS_KEYS_DIR.", "http://127.0.0.1:3000/health"),
    "scanning-service": ("scanning-service", 3010, [3011], {"PORT": "3010", "GRPC_ENABLED": "true"}, ["DATABASE_URL", "RABBITMQ_URL", "JWKS_URL"], "Dịch vụ quét lỗ hổng.", "http://127.0.0.1:3010/health"),
    "alerting-service": ("alerting-service", 3020, [3021], {"PORT": "3020", "GRPC_ENABLED": "true"}, ["DATABASE_URL", "RABBITMQ_URL", "JWKS_URL", "ANALYTICS_URL"], "Cảnh báo và sự cố.", "http://127.0.0.1:3020/health"),
    "graphql-gateway": ("waf-graphql-gateway", 4000, [], {"PORT": "4000"}, ["JWKS_URL", "ANALYTICS_URL", "UPSTREAM_1_NAME"], "Cổng GraphQL gom ba dịch vụ. Khai UPSTREAM_n_* như management-plane/docker-compose.yml.", ""),
    "analytics-api": ("analytics-api", 3030, [], {"PORT": "3030"}, ["CLICKHOUSE_URL", "JWKS_URL"], "analytics-api (Analytics plane).", "http://127.0.0.1:3030/health"),
    "config-distributor": ("waf-config-distributor", 4100, [], {"PORT": "4100"}, ["ZONES_URL", "CONTROL_LOGIN_URL", "CONTROL_SERVICE_EMAIL", "CONTROL_SERVICE_PASSWORD", "REDIS_URL", "AMQP_URL", "ANALYTICS_URL"], "Biên dịch luật và ghi Redis cho edge; nhận telemetry.", "http://127.0.0.1:4100/health"),
}

PRE = """  pre_tasks:
    - name: Chuẩn nền hardening (khi --harden)
      ansible.builtin.import_role:
        name: baseline
      when: app_harden | bool
"""
POST = """  post_tasks:
    - name: Firewall riêng của ứng dụng (khi có app_allow_from)
      ansible.builtin.include_role:
        name: app_firewall
"""


def y(v):
    return json.dumps(v, ensure_ascii=False)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for name, (role, ports, extra, doc) in SIMPLE.items():
        v = dict(extra)
        v["app_fw_ports"] = [{"proto": p, "ports": pr} for p, pr in ports]
        vars_yaml = "\n".join(f"    {k}: {y(x)}" for k, x in v.items())
        (OUT / f"{name}.yml").write_text(
            f"---\n# {doc}\n# Chạy: labs/bin/waap-install {name} --host <ip|local> [-e biến=giá-trị ...] [--harden]\n"
            f"- name: Cài {name}\n  hosts: all\n  become: true\n  vars:\n{vars_yaml}\n{PRE}  roles:\n    - {role}\n{POST}", encoding="utf-8")
    for name, (binary, port, extra_ports, env, req, doc, health) in SERVICES.items():
        ports = [{"proto": "tcp", "ports": [port] + extra_ports}]
        text = f"""---
# {doc}
# Chạy: labs/bin/waap-install {name} --host <ip|local> -e @vars.yml [--harden]
# Cấu hình qua app_env (không bí mật) và app_env_secret (bí mật); xem apps/examples/{name}.yml.
# Bắt buộc có các khoá: {", ".join(req)}.
- name: Cài {name}
  hosts: all
  become: true
  vars:
    app_name: {binary}
    app_binary_src: "{{{{ app_bin_dir }}}}/{binary}"
    app_env_defaults: {y(env)}
    app_health_url: {y(health)}
    app_fw_ports: {y(ports)}
    required_keys: {y(req)}
{PRE}    - name: Đủ cấu hình bắt buộc
      ansible.builtin.assert:
        that: (required_keys | difference((app_env | default({{}})).keys() | list + (app_env_secret | default({{}})).keys() | list)) | length == 0
        fail_msg: "Thiếu khoá cấu hình, xem apps/examples/{name}.yml"
        quiet: true
  roles:
    - app_service
{POST}"""
        (OUT / f"{name}.yml").write_text(text, encoding="utf-8")
        ex = {"app_env": {k: "..." for k in req if "PASSWORD" not in k and "URL" not in k and "AMQP" not in k}, "app_env_secret": {k: "..." for k in req if "PASSWORD" in k or "URL" in k or "AMQP" in k}}
        ex_lines = ["---", f"# Ví dụ cho waap-install {name}: -e @apps/examples/{name}.yml (thay các dấu ...)"]
        for sec, d in ex.items():
            ex_lines.append(f"{sec}:")
            ex_lines += [f"  {k}: {y(v)}" for k, v in d.items()] or ["  {}"]
        (OUT / "examples" / f"{name}.yml").write_text("\n".join(ex_lines) + "\n", encoding="utf-8")
    print(f"đã sinh {len(SIMPLE) + len(SERVICES)} playbook ở {OUT}")


if __name__ == "__main__":
    main()
