#!/usr/bin/env python3
"""Sinh luật nftables cho fw-1 từ flows.yaml và profile (7.6 §4).

Dùng:
  gen_nft.py gen   <profile> [--bootstrap] -o <dir>   # ghi ruleset.nft và inventory.json
  gen_nft.py check <profile>                          # T07: ma trận <-> luật, và không vi phạm cấm tuyệt đối

Quy ước: default-deny; chỉ luật có ID trong ma trận mới được sinh. Luồng cùng vùng (enforce: host) đi
cầu L2, không qua fw-1, nên không sinh luật ở đây (firewall máy chủ làm ở P2/P4).
"""
import argparse, ipaddress, json, re, sys
from pathlib import Path
import yaml

HERE = Path(__file__).resolve().parent.parent
DOC = HERE.parent.parent / "docs/architecture/07-deployment/06-network-zones-and-flows.md"


def load(profile):
    zones = yaml.safe_load((HERE / "zones.yaml").read_text())
    prof = yaml.safe_load((HERE / "profiles" / f"{profile}.yaml").read_text())
    flows = yaml.safe_load((HERE / "flows.yaml").read_text())
    return zones, prof, flows


def vm_ips(zones, prof):
    """{vm: {zone: ip}}"""
    out = {}
    for vm in prof["vms"]:
        out[vm["name"]] = {}
        for z in vm["zones"]:
            net = ipaddress.ip_network(zones["zones"][z]["cidr"])
            out[vm["name"]][z] = str(net.network_address + vm["host"])
    return out


UPLINK = "uplink"


def resolve(spec, zones, prof, ips):
    """Trả về tập (host_label, zone_or_uplink, ip). host_label là tên VM hoặc 'uplink'."""
    if spec == UPLINK:
        return {(UPLINK, UPLINK, zones["uplink"]["cidr"])}
    kind, _, arg = spec.partition(":")
    res = set()
    vms = [v for v in prof["vms"] if "firewall" not in v["roles"]]
    if spec == "all-vms":
        pick = lambda v, z: True
    elif kind == "all-vms-except":
        ex = set(yaml.safe_load(arg))
        pick = lambda v, z: z not in ex
    elif kind == "zone":
        pick = lambda v, z: z == arg
    elif kind == "zones":
        zs = set(yaml.safe_load(arg))
        pick = lambda v, z: z in zs
    elif kind == "role":
        pick = lambda v, z: arg in v["roles"]
    else:
        sys.exit(f"đầu mối không hiểu: {spec}")
    for v in vms:
        for z in v["zones"]:
            if pick(v, z):
                res.add((v["name"], z, ips[v["name"]][z]))
    if not res:
        sys.exit(f"đầu mối rỗng trong profile này: {spec}")
    return res


def fw_flows(flows):
    return [f for f in flows["flows"] if f.get("enforce", "fw") == "fw"]


def pairs(a, b):
    return {(x[0], y[0]) for x in a for y in b if x[0] != y[0] or x[0] == y[0]}


def build_rules(zones, prof, flows, bootstrap=False):
    ips = vm_ips(zones, prof)
    rules = []
    for f in fw_flows(flows):
        src = resolve(f["src"], zones, prof, ips)
        dst = resolve(f["dst"], zones, prof, ips)
        # Bỏ cặp cùng vùng (cầu L2, không qua fw-1).
        s_ips = sorted({s[2] for s in src if any(d[1] != s[1] for d in dst)})
        d_ips = sorted({d[2] for d in dst if any(s[1] != d[1] for s in src)})
        if not s_ips or not d_ips:
            continue
        rules.append((f, s_ips, d_ips))
    return rules, ips


def render(zones, prof, flows, bootstrap):
    rules, ips = build_rules(zones, prof, flows, bootstrap)
    L = []
    L.append("#!/usr/sbin/nft -f")
    L.append(f"# Sinh tự động từ flows.yaml, profile {prof['profile']}. KHÔNG sửa tay.")
    L.append("flush ruleset")
    L.append("table inet lab {")
    L.append("  chain input {")
    L.append("    type filter hook input priority 0; policy drop;")
    L.append('    ct state established,related accept comment "stateful"')
    L.append('    iifname "lo" accept')
    bast = [ips[v["name"]][z] for v in prof["vms"] if "bastion" in v["roles"] for z in v["zones"]]
    L.append(f'    ip saddr {{ {", ".join(bast)} }} tcp dport 22 accept comment "N25 vào fw-1"')
    L.append("  }")
    L.append("  chain forward {")
    L.append("    type filter hook forward priority 0; policy drop;")
    L.append('    ct state established,related accept comment "stateful"')
    L.append('    ct state invalid drop')
    for f, s, d in rules:
        up = f["dst"] == UPLINK
        dport = ", ".join(str(p) for p in f["ports"])
        daddr = "" if up else f"ip daddr {{ {', '.join(d)} }} "
        oif = 'oifname "uplink" ' if up else ""
        cov = f.get("covers", [f["id"]])
        tag = f["id"] if cov == [f["id"]] else f'{f["id"]} ({",".join(cov) or "lab"})'
        L.append(f'    ip saddr {{ {", ".join(s)} }} {daddr}{oif}{f["proto"]} dport {{ {dport} }} accept comment "{tag}"')
    if bootstrap:
        z = ", ".join(zones["zones"][k]["cidr"] for k in zones["zones"] if k != "Z0")
        L.append(f'    ip saddr {{ {z} }} oifname "uplink" tcp dport {{ 53, 80, 443 }} accept comment "BOOTSTRAP gỡ bằng lab seal"')
        L.append(f'    ip saddr {{ {z} }} oifname "uplink" udp dport {{ 53, 123 }} accept comment "BOOTSTRAP gỡ bằng lab seal"')
    L.append('    counter drop comment "default-deny"')
    L.append("  }")
    L.append("}")
    L.append("table ip labnat {")
    L.append("  chain post {")
    L.append("    type nat hook postrouting priority 100; policy accept;")
    L.append('    oifname "uplink" masquerade')
    L.append("  }")
    L.append("}")
    return "\n".join(L) + "\n", ips


# Vai trò bị khoá đường ra ở firewall máy chủ (7.6 X04, X05; ADR-0048 hồ sơ edge-waf, secret, data).
EGRESS_LOCKED = {"edge-waf", "openbao", "postgres", "clickhouse", "kafka", "rabbitmq", "object-storage", "valkey-central"}


def render_host(zones, prof, flows, vm_name, bootstrap=False):
    """Firewall máy chủ cho một VM: input default-deny theo ma trận (cả luồng cùng vùng), output chỉ khoá với vai trò nhạy cảm."""
    ips = vm_ips(zones, prof)
    vm = next(v for v in prof["vms"] if v["name"] == vm_name)
    mine = {ips[vm_name][z] for z in vm["zones"]}
    L = [f"#!/usr/sbin/nft -f", f"# Sinh tự động từ flows.yaml cho {vm_name} (profile {prof['profile']}). KHÔNG sửa tay.",
         "flush ruleset", "table inet host {", "  chain input {",
         "    type filter hook input priority 0; policy drop;",
         '    ct state established,related accept comment "stateful"',
         '    ct state invalid drop',
         '    iifname "lo" accept',
         '    icmp type echo-request limit rate 5/second accept comment "ping"']
    for f in flows["flows"]:
        if f.get("lab_only") and f["dst"] == UPLINK:
            continue
        dst = resolve(f["dst"], zones, prof, ips) if f["dst"] != UPLINK else set()
        if not any(d[0] == vm_name for d in dst):
            continue
        src = resolve(f["src"], zones, prof, ips)
        s_ips = sorted({x[2] for x in src if x[0] != vm_name or f["src"] == f["dst"]})
        if not s_ips:
            continue
        ports = ", ".join(str(p) for p in f["ports"])
        L.append(f'    ip saddr {{ {", ".join(s_ips)} }} {f["proto"]} dport {{ {ports} }} accept comment "{f["id"]}"')
    for loc in flows.get("local", []):
        members = resolve(loc["hosts"], zones, prof, ips)
        if any(m[0] == vm_name for m in members):
            L.append(f'    ip saddr {{ {", ".join(sorted({m[2] for m in members}))} }} accept comment "{loc["id"]} {loc["note"]}"')
    L.append('    counter drop comment "default-deny"')
    L.append("  }")
    roles = set(vm["roles"])
    locked = bool(roles & EGRESS_LOCKED) and "edge-l7" not in roles and not bootstrap
    L.append("  chain output {")
    if locked:
        L.append("    type filter hook output priority 0; policy drop;")
        L.append('    ct state established,related accept comment "stateful"')
        L.append('    oifname "lo" accept')
        for f in flows["flows"]:
            if f["dst"] == UPLINK:
                continue
            src = resolve(f["src"], zones, prof, ips)
            if not any(x[0] == vm_name for x in src):
                continue
            dst = resolve(f["dst"], zones, prof, ips)
            d_ips = sorted({d[2] for d in dst if d[0] != vm_name or f["src"] == f["dst"]})
            if d_ips:
                ports = ", ".join(str(p) for p in f["ports"])
                L.append(f'    ip daddr {{ {", ".join(d_ips)} }} {f["proto"]} dport {{ {ports} }} accept comment "{f["id"]}"')
        for loc in flows.get("local", []):
            members = resolve(loc["hosts"], zones, prof, ips)
            if any(m[0] == vm_name for m in members):
                L.append(f'    ip daddr {{ {", ".join(sorted({m[2] for m in members}))} }} accept comment "{loc["id"]}"')
        L.append('    counter drop comment "egress-locked"')
    else:
        L.append("    type filter hook output priority 0; policy accept;")
    L.append("  }")
    L.append("}")
    return "\n".join(L) + "\n"


def check(profile):
    zones, prof, flows = load(profile)
    ips = vm_ips(zones, prof)
    errs = []
    # 1. Không luồng cho phép nào trùng cấm tuyệt đối (so theo cặp máy; luồng cùng vùng không qua fw-1 bỏ qua).
    for f in fw_flows(flows):
        s, d = resolve(f["src"], zones, prof, ips), resolve(f["dst"], zones, prof, ips)
        for x in flows["forbidden"]:
            if f["id"] in x.get("except", []):
                continue
            fs = resolve(x["src"], zones, prof, ips)
            fd = resolve(x["dst"], zones, prof, ips)
            hit = {(a[0], b[0]) for a in s for b in d if a[1] != b[1]} & {(a[0], b[0]) for a in fs for b in fd}
            if hit:
                errs.append(f'{f["id"]} vi phạm {x["id"]}: {sorted(hit)[:3]}')
    # 2. T07 hai chiều với 7.6: mọi dòng ma trận có luật, mọi luật có dòng ma trận.
    md = DOC.read_text(encoding="utf-8")
    doc_ids = set(re.findall(r"^\| (N\d\d) \|", md, re.M))
    covered = {c for f in flows["flows"] for c in f.get("covers", [f["id"]])}
    for i in sorted(doc_ids - covered):
        errs.append(f"{i} có trong 7.6 nhưng không có luật")
    for i in sorted(covered - doc_ids):
        errs.append(f"{i} có luật nhưng không có trong 7.6")
    ids = [f["id"] for f in flows["flows"]]
    for i in {x for x in ids if ids.count(x) > 1}:
        errs.append(f"id luật trùng: {i}")
    return errs


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("gen"); g.add_argument("profile"); g.add_argument("--bootstrap", action="store_true"); g.add_argument("-o", required=True)
    c = sub.add_parser("check"); c.add_argument("profile")
    a = ap.parse_args()
    if a.cmd == "check":
        errs = check(a.profile)
        for e in errs:
            print("LỖI:", e)
        print("T07 đạt" if not errs else f"T07 hỏng ({len(errs)})")
        sys.exit(1 if errs else 0)
    zones, prof, flows = load(a.profile)
    text, ips = render(zones, prof, flows, a.bootstrap)
    out = Path(a.o); out.mkdir(parents=True, exist_ok=True)
    (out / "ruleset.nft").write_text(text)
    hd = out / "hosts"; hd.mkdir(exist_ok=True)
    for v in prof["vms"]:
        if "firewall" not in v["roles"]:
            (hd / f"{v['name']}.nft").write_text(render_host(zones, prof, flows, v["name"], a.bootstrap))
    (out / "inventory.json").write_text(json.dumps({"profile": prof["profile"], "ips": ips}, indent=2))
    print(f"ghi {out}/ruleset.nft ({len(text.splitlines())} dòng)")


if __name__ == "__main__":
    main()
