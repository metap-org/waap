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
    for var, role in (("lab_infra_ip", "infra"), ("lab_egress_ip", "egress-1"), ("lab_bastion_ip", "bastion"), ("lab_obs_ip", "obs")):
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
    d = sub.add_parser("diagram"); d.add_argument("profile"); d.add_argument("--inject")
    a = ap.parse_args()
    if a.cmd == "inventory":
        inventory(a.profile, a.o)
    else:
        text = diagram(a.profile)
        if not a.inject:
            print(text); return
        p = Path(a.inject); s = p.read_text(encoding="utf-8")
        pat = re.compile(r"(<!-- lab-diagram:start -->\n).*?(<!-- lab-diagram:end -->)", re.S)
        if not pat.search(s):
            sys.exit("thiếu marker lab-diagram trong tệp")
        p.write_text(pat.sub(lambda m: m[1] + text + m[2], s), encoding="utf-8")
        print("đã chèn sơ đồ vào", p)


if __name__ == "__main__":
    main()
