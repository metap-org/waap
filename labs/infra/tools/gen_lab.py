#!/usr/bin/env python3
"""Sinh inventory Ansible, sơ đồ mermaid và bảng RAM của lab từ profile.

  gen_lab.py inventory <profile> -o <dir>
  gen_lab.py diagram   <profile> [--inject <tệp.md>]   # chèn giữa <!-- lab-diagram:start --> và <!-- lab-diagram:end -->
"""
import argparse, re, sys
from pathlib import Path
import yaml
sys.path.insert(0, str(Path(__file__).parent))
import gen_nft as g

ZONE_LABEL = {"Z0": "Z0 Internet giả", "Z1": "Z1 Edge DMZ", "Z2": "Z2 Biên", "Z3": "Z3 Ứng dụng", "Z4": "Z4 Dữ liệu", "Z5": "Z5 Secret", "Z6": "Z6 Vận hành"}
HOST_RESERVE_GB = 8   # Hệ điều hành máy chủ, Incus, bộ nhớ đệm đĩa (ước lượng thô)


def mem_gb(s):
    m = re.fullmatch(r"(\d+(?:\.\d+)?)(GiB|MiB)", str(s))
    return float(m[1]) if m[2] == "GiB" else float(m[1]) / 1024


def inventory(profile, out):
    zones, prof, _ = g.load(profile)
    ips = g.vm_ips(zones, prof)
    groups = {}
    for v in prof["vms"]:
        for r in v["roles"]:
            groups.setdefault(r, []).append(v["name"])
    L = ["# Sinh tự động bởi gen_lab.py. KHÔNG sửa tay.", "[all]"]
    for v in prof["vms"]:
        z = v["zones"][0]
        L.append(f'{v["name"]} ansible_host={ips[v["name"]][z]} lab_zone={z} lab_ip={ips[v["name"]][z]} lab_roles="{",".join(v["roles"])}"')
    L.append("")
    for r, hosts in sorted(groups.items()):
        L.append(f"[role_{r.replace('-', '_')}]")
        L += hosts
        L.append("")
    # Hồ sơ hardening theo vai trò (ADR-0048). Một VM có thể thuộc nhiều hồ sơ.
    L.append("[all:vars]")
    L.append(f"lab_profile={prof['profile']}")
    def ip_of(role):
        for v in prof["vms"]:
            if role in v["roles"]:
                return ips[v["name"]][v["zones"][0]]
        return ""
    for var, role in (("lab_infra_ip", "infra"), ("lab_egress_ip", "egress-1"), ("lab_bastion_ip", "bastion"), ("lab_obs_ip", "obs"), ("lab_registry_ip", "registry")):
        L.append(f"{var}={ip_of(role)}")
    Path(out).mkdir(parents=True, exist_ok=True)
    (Path(out) / "inventory.ini").write_text("\n".join(L) + "\n")
    hv = Path(out) / "host_vars"; hv.mkdir(exist_ok=True)
    for v in prof["vms"]:
        hvars = dict(v.get("vars") or {})
        if v.get("disks"):
            hvars["lab_disks"] = v["disks"]
        if hvars:
            (hv / f"{v['name']}.yml").write_text(yaml.safe_dump(hvars, sort_keys=False))
    print(f"ghi {out}/inventory.ini")


# (thành phần, vai trò trong profile, role Ansible, lệnh lab, ghi chú). Chỉ liệt kê cái đã có bộ cài hoặc cần nói rõ là chưa có.
COMPONENTS = [
    ("PostgreSQL (PGDG)", "postgres", "svc_postgres", "lab services --tags postgres", "waap-install postgres", "Repo PGDG chính thức, gói bị hold; đĩa dữ liệu và WAL riêng. Profile full có node thứ hai nhưng sao chép (replication) CHƯA viết, nên chỉ cài node đầu"),
    ("RabbitMQ (repo Team RabbitMQ)", "rabbitmq", "svc_rabbitmq", "lab services --tags rabbitmq", "waap-install rabbitmq", "Erlang và RabbitMQ cùng repo, gói bị hold. Cổng 5672 không TLS; N15 chỉ mở 5671, nên chưa ai ngoài máy này vào được cho tới khi có TLS (P2/P5)"),
    ("Valkey (valkey-central)", "valkey-central", "svc_valkey", "lab services --tags valkey", "waap-install valkey", "Binary chính thức ghim sha256, /opt/valkey/<version>; cache và hot key trung tâm"),
    ("Valkey (valkey-edge)", "valkey-edge", "svc_valkey", "lab services --tags valkey", "waap-install valkey", "Nằm ở vùng Edge DMZ theo thiết kế (ADR-0021)"),
    ("ClickHouse", "clickhouse", "svc_clickhouse", "lab services --tags clickhouse", "waap-install clickhouse", "Repo apt chính thức của ClickHouse, 3 gói cùng version, bị hold"),
    ("RKE2 server", "k8s-cp", "profile_rke2_node", "lab provision --tags k8s --limit <vm>", "waap-install rke2-node", "Cần ghim phiên bản và sha256 của tarball"),
    ("RKE2 agent", "k8s-app", "profile_rke2_node", "lab provision --tags k8s --limit <vm>", "waap-install rke2-node -e rke2_role=agent", ""),
    ("Mirror nội bộ (aptly + artifact + apt-cacher-ng)", "registry", "svc_registry", "lab services --tags registry", "waap-install registry", "Nguồn gói duy nhất cho vùng không có đường ra"),
    ("egress-1 (squid)", "egress-1", "svc_perimeter", "lab services --tags egress", "waap-install egress", "Đường ra duy nhất có allowlist"),
    ("Kafka", "kafka", "(chưa có)", "(chưa có)", "(chưa có)", "VM đã có (đĩa dữ liệu) nhưng CHƯA có bộ cài; không dịch vụ nào dùng Kafka ở code hiện tại"),
    ("Object storage (SeaweedFS)", "object-storage", "(chưa có)", "(chưa có)", "(chưa có)", "Vai trò gắn vào data-1 nhưng CHƯA có bộ cài"),
    ("OpenBao", "openbao", "(chưa có)", "(chưa có)", "(chưa có)", "VM đã hardening (không swap, không core dump); cài OpenBao là P5"),
    ("ingress-1, egw-1", "ingress-1", "(chưa có)", "(chưa có)", "(chưa có)", "Chờ triển khai dịch vụ ứng dụng"),
]


def placement(profile):
    zones, prof, flows = g.load(profile)
    ips = g.vm_ips(zones, prof)
    L = ["# Bản đồ cài đặt: thành phần nào ở máy nào, vùng nào, ai được vào",
         "",
         f"Sinh tự động bởi `labs/infra/tools/gen_lab.py placement` từ profile `{prof['profile']}` và ma trận luồng. **Không sửa tay.**",
         "",
         "| Thành phần | Máy (vùng, IP) | Ai được kết nối tới (từ ma trận) | Cài bằng (lab) | Cài bằng (đứng riêng) | Ghi chú |",
         "|---|---|---|---|---|---|"]
    for name, role, ansible, labcmd, solo, note in COMPONENTS:
        vms = [v for v in prof["vms"] if role in v["roles"]]
        where = "; ".join(f"`{v['name']}` ({v['zones'][0]}, {ips[v['name']][v['zones'][0]]})" for v in vms) or "(không có trong profile)"
        reach = []
        for f in flows["flows"]:
            if f["dst"] == g.UPLINK or f.get("lab_only"):
                continue
            try:
                dst = g.resolve(f["dst"], zones, prof, ips)
            except SystemExit:
                continue
            if any(role in vm_["roles"] for vm_ in prof["vms"] if vm_["name"] in {d[0] for d in dst}) and f["dst"] in (f"role:{role}", "all-vms", "zone:Z3") or f["dst"] == f"role:{role}":
                ports = ",".join(str(x) for x in f["ports"])
                reach.append(f"{f['id']}: {f['src'].replace('role:', '').replace('zone:', 'vùng ')} → {f['proto']}/{ports}")
        reach = [r for r in reach if not r.startswith(("N25", "N26", "N27"))]
        reach_txt = "<br>".join(reach) if reach else "(chưa có luồng nào mở tới đây)"
        L.append(f"| {name} | {where} | {reach_txt} | `{labcmd}` | `{solo}` | {note} |")
    L += ["", "Luồng quản trị chung (SSH từ `bastion` N25, giám sát từ `obs` N26, NTP và DNS tới `infra` N27) áp dụng cho mọi máy và không lặp lại trong bảng.", "",
          "## Mỗi thành phần được cài ra sao",
          "",
          "- **Lab:** `lab services --tags <thành phần>` chạy Ansible theo nhóm vai trò trong inventory; VM nào mang vai trò đó thì được cài. Từng bước và giá trị mục tiêu ở [CHECKLIST.md](CHECKLIST.md) (P8).",
          "- **Đứng riêng:** `waap-install <thành phần> --host <ip|local>` cài lên một máy bất kỳ, không cần inventory; truyền `app_allow_from` (CIDR được vào) và `app_ssh_from` để bật firewall riêng.",
          "- Cả hai đường dùng chung các role `svc_*`, nên cấu hình giống nhau."]
    return "\n".join(L) + "\n"


def diagram(profile):
    zones, prof, flows = g.load(profile)
    ips = g.vm_ips(zones, prof)
    M = ["```mermaid", "flowchart TB", "  subgraph Z0[\"Z0 Internet giả lập\"]"]
    by = {}
    for v in prof["vms"]:
        if "firewall" in v["roles"]:
            continue
        by.setdefault(v["zones"][0], []).append(v)
    def node(v):
        z = v["zones"][0]
        return f'{v["name"].replace("-", "_")}["{v["name"]}<br/>{ips[v["name"]][z].split(".", 2)[2]} · {v["mem"]}"]'
    first = True
    for z in ["Z0", "Z1", "Z2", "Z3", "Z4", "Z5", "Z6"]:
        if z != "Z0":
            M.append(f'  subgraph {z}["{ZONE_LABEL[z]} · {zones["zones"][z]["cidr"]}"]')
        for v in by.get(z, []):
            M.append("    " + node(v))
        M.append("  end")
    M.append('  FW{{"fw-1<br/>nftables, router L3 duy nhất"}}')
    for z in ["Z0", "Z1", "Z2", "Z3", "Z4", "Z5", "Z6"]:
        M.append(f"  {z} --- FW")
    M.append("```")
    total = sum(mem_gb(v["mem"]) for v in prof["vms"])
    T = ["| Vùng | Máy | RAM (GB) |", "|---|---|---|"]
    for z in ["Z0", "Z1", "Z2", "Z3", "Z4", "Z5", "Z6"]:
        vs = by.get(z, [])
        T.append(f'| {z} | {", ".join(v["name"] for v in vs)} | {sum(mem_gb(v["mem"]) for v in vs):.1f} |')
    fw = next(v for v in prof["vms"] if "firewall" in v["roles"])
    T.append(f'| (router) | fw-1 | {mem_gb(fw["mem"]):.1f} |')
    T.append(f'| **Tổng VM** | {len(prof["vms"])} máy | **{total:.1f}** |')
    T.append(f"| Dự trữ máy chủ | hệ điều hành, Incus, đệm đĩa | {HOST_RESERVE_GB} (ước lượng) |")
    T.append(f"| **Cần** | | **{total + HOST_RESERVE_GB:.1f}** |")
    disk = sum(float(re.match(r"\d+", v["disk"])[0]) for v in prof["vms"])
    return "\n".join(M) + "\n\n" + "\n".join(T) + f"\n\nĐĩa cấp phát tối đa khoảng {disk:.0f} GB (thin provisioning nên dùng thực tế ít hơn). Số RAM là ước lượng thô, chưa đo.\n"


def main():
    ap = argparse.ArgumentParser(); sub = ap.add_subparsers(dest="cmd", required=True)
    i = sub.add_parser("inventory"); i.add_argument("profile"); i.add_argument("-o", required=True)
    vmq = sub.add_parser("vm"); vmq.add_argument("profile"); vmq.add_argument("role")
    pl = sub.add_parser("placement"); pl.add_argument("profile"); pl.add_argument("-o")
    d = sub.add_parser("diagram"); d.add_argument("profile"); d.add_argument("--inject"); d.add_argument("--marker", default="lab-diagram")
    a = ap.parse_args()
    if a.cmd == "vm":
        _, prof_, _ = g.load(a.profile)
        print(next((v["name"] for v in prof_["vms"] if a.role in v["roles"]), ""))
    elif a.cmd == "placement":
        text = placement(a.profile)
        if a.o:
            Path(a.o).write_text(text, encoding="utf-8")
            print("đã ghi", a.o)
        else:
            print(text)
    elif a.cmd == "inventory":
        inventory(a.profile, a.o)
    else:
        text = diagram(a.profile)
        if not a.inject:
            print(text); return
        p = Path(a.inject); s = p.read_text(encoding="utf-8")
        pat = re.compile(rf"(<!-- {a.marker}:start -->\n).*?(<!-- {a.marker}:end -->)", re.S)
        if not pat.search(s):
            sys.exit(f"thiếu marker {a.marker} trong tệp")
        p.write_text(pat.sub(lambda m: m[1] + text + m[2], s), encoding="utf-8")
        print("đã chèn sơ đồ vào", p)


if __name__ == "__main__":
    main()
