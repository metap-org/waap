#!/usr/bin/env python3
"""Chạy lab từng bước nhỏ, mỗi bước có checklist "cấu hình đúng phải là bao nhiêu" và lệnh kiểm tự động.

  steps.py list [--profile compact]       danh sách bước và trạng thái (chưa chạy / đã chạy / đã kiểm)
  steps.py next                           bước kế tiếp cần làm
  steps.py show <id>                      mô tả, lệnh, checklist (không chạy gì)
  steps.py run <id> [--yes] [--force]     chạy lệnh của bước (hỏi xác nhận; từ chối nếu bước phụ thuộc chưa kiểm)
  steps.py verify <id> [--ack-manual]     chạy từng lệnh kiểm, in bảng PASS/FAIL (thực tế so với mong đợi)
  steps.py doc                            sinh labs/CHECKLIST.md từ steps.yaml và bảng tuning
  steps.py validate                       kiểm steps.yaml nhất quán (CI chạy lệnh này)

Giá trị tuning lấy từ labs/ansible/roles/os_tuning/defaults/main.yml (một nguồn sự thật với Ansible).
"""
import argparse, json, os, re, subprocess, sys, time
from pathlib import Path
import yaml

HERE = Path(__file__).resolve().parent
INFRA = HERE.parent
LABS = INFRA.parent
STEPS_FILE = INFRA / "steps.yaml"
TUNING_FILE = LABS / "ansible/roles/os_tuning/defaults/main.yml"
STATE = LABS / ".state/steps.json"
CHECKLIST = LABS / "CHECKLIST.md"
sys.path.insert(0, str(HERE))
import gen_nft as g  # noqa: E402


# ---------- dữ liệu ----------
def load(profile="compact"):
    zones, prof, flows = g.load(profile)
    steps = yaml.safe_load(STEPS_FILE.read_text(encoding="utf-8"))
    tuning = yaml.safe_load(TUNING_FILE.read_text(encoding="utf-8"))
    return {"zones": zones, "prof": prof, "flows": flows, "steps": steps, "tuning": tuning, "ips": g.vm_ips(zones, prof)}


def vm_of(ctx, name):
    return next(v for v in ctx["prof"]["vms"] if v["name"] == name)


def groups_of(vm):
    return {"role_" + r.replace("-", "_") for r in vm["roles"]}


def merge_tuning(ctx, vm_name):
    """Gộp bảng tuning cho một VM, đúng thứ tự tuning_precedence như role Ansible (vai trò sau thắng)."""
    t, vm = ctx["tuning"], vm_of(ctx, vm_name)
    sysctl, thp, limits, conflicts = {}, "", {}, []
    for grp in [x for x in t["tuning_precedence"] if x in groups_of(vm)]:
        spec = t["tuning_by_role"].get(grp, {})
        for k, v in (spec.get("sysctl") or {}).items():
            if k in sysctl and sysctl[k] != v:
                conflicts.append((k, sysctl[k], v, grp))
            sysctl[k] = v
        if spec.get("thp") == "never":
            thp = "never"
        for unit, lim in (spec.get("limits") or {}).items():
            limits.setdefault(unit, {}).update(lim)
    return sysctl, thp, limits, conflicts


def expand_on(ctx, on):  # selector at
    """Selector -> danh sách 'host' hoặc tên VM."""
    vms = [v["name"] for v in ctx["prof"]["vms"]]
    if on == "host":
        return ["host"]
    if on == "all":
        return vms
    if on.startswith("all-except:"):
        ex = {x for x in on.split(":", 1)[1].split(",")}
        ex_roles = {x for x in ex}
        return [v["name"] for v in ctx["prof"]["vms"] if v["name"] not in ex and not (set(v["roles"]) & ex_roles)]
    if on.startswith("role:"):
        r = on.split(":", 1)[1]
        return [v["name"] for v in ctx["prof"]["vms"] if r in v["roles"]]
    if on in vms:
        return [on]
    raise SystemExit(f"selector không hiểu: {on}")


def fw_forward_accepts(ctx, bootstrap=True):
    text, _ = g.render(ctx["zones"], ctx["prof"], ctx["flows"], bootstrap)
    chain = text.split("chain forward")[1]
    return sum(1 for l in chain.splitlines() if " accept comment " in l and "stateful" not in l)


def subst(ctx, s):
    s = s.replace("{fw_rules}", str(fw_forward_accepts(ctx)))
    s = re.sub(r"\{ip:([a-z0-9-]+)\}", lambda m: next(iter(ctx["ips"][m[1]].values())), s)
    return s


def runtime_subst(s):
    """Điền {secret:khoá} từ generated/secrets.yml lúc chạy (không bao giờ vào tài liệu)."""
    def sec(m):
        d = yaml.safe_load((INFRA / "generated/secrets.yml").read_text())["lab_secrets"]
        return d[m[1]]
    return re.sub(r"\{secret:([a-z_]+)\}", sec, s)


def expand_checks(ctx, step):
    """Mỗi phần tử: dict(at, what, cmd, op, value, manual). 'use:' sinh nhiều kiểm từ dữ liệu."""
    out = []
    for c in step.get("checks", []):
        if "use" in c:
            out += generated_checks(ctx, c)
        else:
            out.append(dict(c))
    return out


def generated_checks(ctx, c):
    kind, out = c["use"], []
    for name in c.get("vms", []):
        vm = vm_of(ctx, name)
        z = vm["zones"][0]
        ip = ctx["ips"][name][z]
        gw = ctx["ips"][name][z].rsplit(".", 1)[0] + ".1"
        if kind == "vm_boot":
            out += [
                dict(at="host", what=f"{name} đang chạy", cmd=f"incus list {name} -c s -f csv", op="eq", value="RUNNING"),
                dict(at="host", what=f"{name} RAM cấp theo profile", cmd=f"incus config get {name} limits.memory", op="eq", value=vm["mem"]),
                dict(at="host", what=f"{name} số vCPU theo profile", cmd=f"incus config get {name} limits.cpu", op="eq", value=str(vm["cpu"])),
                dict(at=name, what="cloud-init xong", cmd="cloud-init status | awk '{print $2}'", op="eq", value="done"),
                dict(at=name, what=f"IP đúng vùng {z}", cmd="ip -4 -o addr show lab0 | awk '{print $4}'", op="eq", value=f"{ip}/24"),
                dict(at=name, what="gateway là fw-1", cmd="ip route | awk '/^default/{print $3}'", op="eq", value=gw),
                dict(at=name, what="ping được fw-1", cmd=f"ping -c1 -W2 {gw} >/dev/null", op="rc0"),
                dict(at=name, what="tên máy đúng", cmd="hostname", op="eq", value=name),
            ]
        elif kind == "tuning":
            sysctl, thp, limits, conf = merge_tuning(ctx, name)
            for k, v in sorted(sysctl.items()):
                out.append(dict(at=name, what=f"sysctl {k}", cmd=f"sysctl -n {k}", op="eq", value=str(v)))
            if thp:
                out.append(dict(at=name, what="transparent hugepage", cmd="grep -o '\\[[a-z]*\\]' /sys/kernel/mm/transparent_hugepage/enabled", op="eq", value="[never]"))
            if "vm.nr_hugepages" in sysctl:
                out.append(dict(at=name, what="hugepage thực sự cấp được", cmd="awk '/HugePages_Total/{print $2}' /proc/meminfo", op="eq", value=str(sysctl["vm.nr_hugepages"])))
        elif kind == "limits":
            _, _, limits, _ = merge_tuning(ctx, name)
            for unit, lim in sorted(limits.items()):
                for k, v in lim.items():
                    out.append(dict(at=name, what=f"{unit}: {k}", cmd=f"systemctl show {unit} -p {k} --value", op="eq", value=str(v)))
        elif kind == "disks":
            for d in vm.get("disks", []):
                gb = int(re.match(r"\d+", d["size"])[0])
                opts = "noatime,nodev,nosuid" + (",noexec" if d["name"] in ("data", "wal", "audit") else "")
                out += [
                    dict(at=name, what=f"đĩa {d['name']} gắn ở {d['mount']}, hệ tệp xfs", cmd=f"findmnt -no FSTYPE {d['mount']}", op="eq", value="xfs"),
                    dict(at=name, what=f"{d['mount']} tuỳ chọn mount", cmd=f"findmnt -no OPTIONS {d['mount']}", op="contains_all", value=opts.split(",")),
                    dict(at=name, what=f"{d['mount']} dung lượng >= {gb - 2} GB (xfs tốn một phần)", cmd=f"df -BG --output=size {d['mount']} | tail -1 | tr -dc 0-9", op="ge", value=gb - 2),
                ]
    return out


# ---------- thực thi ----------
def run_check(ctx, ch, vm, timeout=40):
    cmd = runtime_subst(subst(ctx, ch["cmd"]))
    argv = ["bash", "-c", cmd] if vm == "host" else ["incus", "exec", vm, "--", "bash", "-c", cmd]
    try:
        p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
        return p.returncode, (p.stdout or "").strip(), (p.stderr or "").strip()
    except subprocess.TimeoutExpired:
        return 124, "", "hết thời gian"


def compare(op, expected, rc, out):
    if op == "rc0":
        return rc == 0, f"rc={rc}"
    if op == "rc_nonzero":
        return rc != 0, f"rc={rc}"
    if op == "eq":
        return out == str(expected), out
    if op == "ne":
        return out != str(expected), out
    if op == "ge":
        try: return float(out) >= float(expected), out
        except ValueError: return False, out
    if op == "le":
        try: return float(out) <= float(expected), out
        except ValueError: return False, out
    if op == "contains":
        return str(expected) in out, out
    if op == "contains_all":
        return all(x in out for x in expected), out
    if op == "re":
        return re.search(str(expected), out) is not None, out
    raise SystemExit(f"op không hiểu: {op}")


def show_expected(ch):
    op, v = ch.get("op"), ch.get("value")
    return {"rc0": "lệnh thành công (rc=0)", "rc_nonzero": "lệnh thất bại (bị chặn)", "eq": f"= {v}", "ne": f"≠ {v}", "ge": f"≥ {v}", "le": f"≤ {v}",
            "contains": f"chứa {v}", "contains_all": "chứa " + ", ".join(v) if isinstance(v, list) else f"chứa {v}", "re": f"khớp /{v}/"}.get(op, "THỦ CÔNG" if ch.get("manual") else "?")


def state_load():
    return json.loads(STATE.read_text()) if STATE.exists() else {}


def state_save(s):
    STATE.parent.mkdir(parents=True, exist_ok=True)
    STATE.write_text(json.dumps(s, indent=2))


def find(ctx, sid):
    for st in ctx["steps"]["steps"]:
        if st["id"] == sid:
            return st
    raise SystemExit(f"không có bước {sid}")


# ---------- lệnh ----------
def cmd_list(ctx):
    s = state_load()
    cur = None
    for st in ctx["steps"]["steps"]:
        ph = st["id"].split(".")[0]
        if ph != cur:
            cur = ph
            print(f"\n{ph}  {ctx['steps']['phases'][ph]}")
        mark = "đã kiểm " if s.get(st["id"], {}).get("verified") else ("đã chạy " if s.get(st["id"], {}).get("run") else "chưa     ")
        print(f"  {st['id']:6} [{mark}] {st['title']}")


def cmd_next(ctx):
    s = state_load()
    for st in ctx["steps"]["steps"]:
        if not s.get(st["id"], {}).get("verified"):
            print(st["id"], "-", st["title"])
            return
    print("tất cả các bước đã kiểm")


def cmd_show(ctx, sid):
    st = find(ctx, sid)
    print(f"{st['id']}  {st['title']}\n\nVì sao: {st['why']}\nPhụ thuộc: {', '.join(st.get('depends', [])) or 'không'}\nThời gian dự kiến: {st.get('time', '?')}")
    print("\nLệnh:")
    for c in st.get("run", []) or ["(không có, bước chỉ kiểm hoặc làm tay)"]:
        print("  $", c)
    print("\nChecklist:")
    for ch in expand_checks(ctx, st):
        print(f"  [{ch['at']:11}] {ch['what']:55} {show_expected(ch)}")
    if st.get("rollback"):
        print("\nHoàn tác:", st["rollback"])


def cmd_run(ctx, sid, yes, force):
    st = find(ctx, sid)
    s = state_load()
    missing = [d for d in st.get("depends", []) if not s.get(d, {}).get("verified")]
    if missing and not force:
        raise SystemExit(f"chưa kiểm xong bước phụ thuộc: {', '.join(missing)} (chạy `steps.py verify <id>` hoặc --force)")
    cmd_show(ctx, sid)
    if not yes and input("\nChạy các lệnh trên? [y/N] ").lower() != "y":
        raise SystemExit("đã huỷ")
    for c in st.get("run", []):
        print("$", c)
        if subprocess.call(c, shell=True, cwd=LABS.parent) != 0:
            raise SystemExit(f"lệnh lỗi: {c}")
    s.setdefault(sid, {})["run"] = time.strftime("%F %T")
    state_save(s)
    print(f"\nxong. Kiểm bằng:  labs/bin/lab step verify {sid}")


def cmd_verify(ctx, sid, ack_manual):
    st = find(ctx, sid)
    fails, manual = 0, 0
    print(f"{st['id']}  {st['title']}\n")
    for ch in expand_checks(ctx, st):
        if ch.get("manual"):
            manual += 1
            print(f"?    [{ch['at']}] {ch['what']}  (THỦ CÔNG: {ch['manual']})")
            continue
        for vm in expand_on(ctx, ch["at"]):
            rc, out, err = run_check(ctx, ch, vm)
            ok, got = compare(ch["op"], ch.get("value"), rc, out)
            if not ok:
                fails += 1
            tail = "" if ok else f"   thực tế: {got or err or '(rỗng)'}"
            print(f"{'PASS' if ok else 'FAIL'} [{vm:11}] {ch['what']}  ({show_expected(ch)}){tail}")
    s = state_load()
    if fails == 0 and (manual == 0 or ack_manual):
        s.setdefault(sid, {})["verified"] = time.strftime("%F %T")
        state_save(s)
        print(f"\n{sid} ĐẠT")
        return 0
    if fails == 0:
        print(f"\nmọi kiểm tự động đạt; còn {manual} mục thủ công. Đọc rồi chạy lại với --ack-manual.")
        return 2
    print(f"\n{sid} CHƯA ĐẠT: {fails} lỗi")
    return 1


def cmd_doc(ctx):
    L = ["# Checklist chạy lab từng bước", "",
         "Sinh tự động bởi `labs/infra/tools/steps.py doc` từ `labs/infra/steps.yaml` và bảng tuning ở `labs/ansible/roles/os_tuning/defaults/main.yml`. **Không sửa tay.**", "",
         "Cách dùng (trên máy lab): `labs/bin/lab step list`, rồi từng bước `show` (đọc), `run` (chạy), `verify` (kiểm). `run` từ chối nếu bước phụ thuộc chưa `verify` đạt.", "",
         "Giá trị trong cột **Phải là** là giá trị mục tiêu của profile `compact` (ước lượng, chưa đo trên máy thật). Mục **THỦ CÔNG** cần người đọc.", ""]
    cur = None
    for st in ctx["steps"]["steps"]:
        ph = st["id"].split(".")[0]
        if ph != cur:
            cur = ph
            L += [f"## {ph}. {ctx['steps']['phases'][ph]}", ""]
        L += [f"### {st['id']} {st['title']}", "", f"**Vì sao:** {st['why']}", "", f"**Phụ thuộc:** {', '.join(st.get('depends', [])) or 'không'}  ·  **Thời gian dự kiến:** {st.get('time', '?')}", ""]
        if st.get("run"):
            L += ["```", *[f"$ {c}" for c in st["run"]], "```", ""]
        L += ["| Kiểm | Ở đâu | Phải là | Lệnh |", "|---|---|---|---|"]
        seen = set()
        for ch in expand_checks(ctx, st):
            key = (ch["what"], ch["at"], ch.get("cmd"))
            on = ch["at"]
            # gom bản sao theo từng VM của cùng một kiểm sinh tự động để bảng không dài vô ích
            if ch["what"] in seen and on not in ("host",):
                pass
            seen.add(ch["what"])
            cmd = (ch.get("cmd") or "").replace("|", "\\|")
            exp = show_expected(ch).replace("|", "\\|")
            L.append(f"| {ch['what']} | `{on}` | {exp} | `{subst(ctx, cmd)}` |" if ch.get("cmd") else f"| {ch['what']} | `{on}` | {exp} ({ch.get('manual')}) | |")
        sysctl_conf = []
        for c in st.get("checks", []):
            if c.get("use") == "tuning":
                for name in c["vms"]:
                    sysctl_conf += [f"- `{name}`: {k} (đã chọn {a} → {b} theo vai trò {gname})" for k, a, b, gname in merge_tuning(ctx, name)[3]]
        why_lines = []
        for c in st.get("checks", []):
            if c.get("use") == "tuning":
                for name in c["vms"]:
                    for grp in [x for x in ctx["tuning"]["tuning_precedence"] if x in groups_of(vm_of(ctx, name))]:
                        w = ctx["tuning"]["tuning_by_role"].get(grp, {}).get("why")
                        if w:
                            why_lines.append(f"- `{name}` ({grp}): {w}")
        if why_lines:
            L += ["", "**Vì sao các giá trị này:**", *dict.fromkeys(why_lines)]
        if sysctl_conf:
            L += ["", "**Xung đột giá trị khi gộp vai trò (vai trò sau thắng, xem `tuning_precedence`):**", *sysctl_conf]
        if st.get("rollback"):
            L += ["", f"**Hoàn tác:** {st['rollback']}"]
        L.append("")
    CHECKLIST.write_text("\n".join(L) + "\n", encoding="utf-8")
    print("đã ghi", CHECKLIST)


def cmd_validate(ctx):
    errs, ids = [], [s["id"] for s in ctx["steps"]["steps"]]
    for i in {x for x in ids if ids.count(x) > 1}:
        errs.append(f"id trùng: {i}")
    vmnames = {v["name"] for v in ctx["prof"]["vms"]}
    for st in ctx["steps"]["steps"]:
        for k in ("id", "title", "why", "checks"):
            if not st.get(k):
                errs.append(f"{st.get('id')}: thiếu {k}")
        if st["id"].split(".")[0] not in ctx["steps"]["phases"]:
            errs.append(f"{st['id']}: phase không khai báo")
        for d in st.get("depends", []):
            if d not in ids:
                errs.append(f"{st['id']}: phụ thuộc không tồn tại {d}")
            elif ids.index(d) >= ids.index(st["id"]):
                errs.append(f"{st['id']}: phụ thuộc {d} đứng sau nó")
        for c in st.get("run", []) or []:
            if "labs/bin/lab" not in c and not c.startswith("incus "):
                errs.append(f"{st['id']}: lệnh lạ (chỉ labs/bin/lab hoặc incus): {c}")
        try:
            chs = expand_checks(ctx, st)
        except StopIteration:
            errs.append(f"{st['id']}: 'vms' có tên không có trong profile"); continue
        for c in st.get("checks", []):
            for n in c.get("vms", []):
                if n not in vmnames:
                    errs.append(f"{st['id']}: VM không có trong profile: {n}")
        for ch in chs:
            if ch.get("manual"):
                continue
            if ch.get("op") not in ("rc0", "rc_nonzero", "eq", "ne", "ge", "le", "contains", "contains_all", "re"):
                errs.append(f"{st['id']}: op sai ở '{ch.get('what')}'")
            try:
                expand_on(ctx, ch["at"])
            except SystemExit as e:
                errs.append(f"{st['id']}: {e}")
        if not any(not ch.get("manual") for ch in chs) and st["id"] not in ctx["steps"].get("manual_only", []):
            errs.append(f"{st['id']}: không có kiểm tự động nào")
    # tag Ansible và lệnh lab được dùng phải có thật
    tags = set()
    for f in (LABS / "ansible").rglob("*.yml"):
        for m in re.finditer(r"tags:\s*\[([^\]]+)\]", f.read_text(encoding="utf-8")):
            tags |= {t.strip() for t in m[1].split(",")}
    lab_text = (LABS / "bin/lab").read_text(encoding="utf-8")
    for st in ctx["steps"]["steps"]:
        for c in st.get("run", []) or []:
            for m in re.finditer(r"--tags ([\w,-]+)", c):
                for t in m[1].split(","):
                    if t not in tags:
                        errs.append(f"{st['id']}: tag Ansible không tồn tại: {t}")
            m = re.search(r"labs/bin/lab (\w[\w-]*)", c)
            if m and not re.search(rf"\b{re.escape(m[1])}\)", lab_text):
                errs.append(f"{st['id']}: lệnh lab không tồn tại: {m[1]}")
    # bảng tuning: mọi vai trò có trong profile phải có mục (có thể rỗng)
    for v in ctx["prof"]["vms"]:
        for r in v["roles"]:
            g_ = "role_" + r.replace("-", "_")
            if g_ in ctx["tuning"]["tuning_precedence"] and g_ not in ctx["tuning"]["tuning_by_role"]:
                errs.append(f"tuning thiếu mục cho {g_}")
    for e in errs:
        print("LỖI:", e)
    print("steps.yaml hợp lệ" if not errs else f"{len(errs)} lỗi")
    return 1 if errs else 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--profile", default=os.environ.get("LAB_PROFILE", "compact"))
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("list"); sub.add_parser("next"); sub.add_parser("doc"); sub.add_parser("validate")
    for n in ("show", "run", "verify"):
        p = sub.add_parser(n); p.add_argument("id")
        if n == "run": p.add_argument("--yes", action="store_true"); p.add_argument("--force", action="store_true")
        if n == "verify": p.add_argument("--ack-manual", action="store_true")
    a = ap.parse_args()
    ctx = load(a.profile)
    if a.cmd == "list": cmd_list(ctx)
    elif a.cmd == "next": cmd_next(ctx)
    elif a.cmd == "show": cmd_show(ctx, a.id)
    elif a.cmd == "run": cmd_run(ctx, a.id, a.yes, a.force)
    elif a.cmd == "verify": sys.exit(cmd_verify(ctx, a.id, a.ack_manual))
    elif a.cmd == "doc": cmd_doc(ctx)
    elif a.cmd == "validate": sys.exit(cmd_validate(ctx))


if __name__ == "__main__":
    main()
