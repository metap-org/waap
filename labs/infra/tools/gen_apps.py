#!/usr/bin/env python3
"""Sinh playbook cài từng ứng dụng đứng riêng (labs/ansible/apps/*.yml). Sửa danh sách ở đây rồi chạy lại, đừng sửa tay."""
import json
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent.parent / "ansible" / "apps"

SIMPLE = {
    "postgres": ("svc_postgres", [("tcp", [5432])], {"pg_allowed_cidrs": "{{ app_allow_from }}"}, "PostgreSQL từ repo PGDG chính thức (pg_version, mặc định 17), scram, TLS, chỉ nhận từ app_allow_from."),
    "rabbitmq": ("svc_rabbitmq", [("tcp", [5672])], {}, "RabbitMQ và Erlang từ repo Team RabbitMQ (khoá ký ghim fingerprint), không guest."),
    "valkey": ("svc_valkey", [("tcp", [6379])], {}, "Valkey (binary chính thức, ghim sha256) có ACL: user control ghi, user edge chỉ đọc waf:*. Cần valkey_version, valkey_sha256."),
    "clickhouse": ("svc_clickhouse", [("tcp", [9000])], {"clickhouse_allowed_cidrs": "{{ app_allow_from }}"}, "ClickHouse từ repo apt chính thức (khoá ký ghim fingerprint). Nâng version bằng clickhouse_pkg_version."),
    "egress": ("svc_perimeter", [("tcp", [3128])], {"egress_allowed_src": "{{ app_allow_from }}"}, "squid ra Internet có allowlist, chặn dải riêng và metadata."),
    "rke2-node": ("profile_rke2_node", [("tcp", [6443, 9345, 10250])], {}, "Node RKE2 (server hoặc agent: -e rke2_role=agent -e rke2_api_endpoint=...). Cần rke2_version, rke2_sha256, rke2_token."),
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
    print(f"đã sinh {len(SIMPLE)} playbook ở {OUT}")


if __name__ == "__main__":
    main()
