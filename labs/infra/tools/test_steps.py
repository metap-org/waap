import json, subprocess, sys, tempfile, unittest
from pathlib import Path
from unittest import mock
sys.path.insert(0, str(Path(__file__).parent))
import steps as S

CTX = S.load("compact")


class StepsTests(unittest.TestCase):
    def test_steps_yaml_is_valid(self):
        self.assertEqual(S.cmd_validate(CTX), 0)

    def test_every_check_has_expected_value_or_is_manual(self):
        for st in CTX["steps"]["steps"]:
            for ch in S.expand_checks(CTX, st):
                if ch.get("manual"):
                    continue
                if ch["op"] in ("eq", "ne", "ge", "le", "contains", "contains_all", "re"):
                    self.assertIn("value", ch, f"{st['id']} {ch['what']}")

    def test_selectors(self):
        self.assertEqual(S.expand_on(CTX, "host"), ["host"])
        self.assertIn("edge-1", S.expand_on(CTX, "all"))
        ex = S.expand_on(CTX, "all-except:firewall,k8s-cp,k8s-app")
        self.assertNotIn("fw-1", ex); self.assertNotIn("k8s-cp-1", ex); self.assertNotIn("k8s-wk-1", ex); self.assertIn("data-1", ex)
        self.assertEqual(S.expand_on(CTX, "role:k8s-app"), ["k8s-wk-1", "k8s-wk-2"])

    def test_compare_ops(self):
        self.assertTrue(S.compare("eq", "1", 0, "1")[0]); self.assertFalse(S.compare("eq", "1", 0, "2")[0])
        self.assertTrue(S.compare("ge", 8, 0, "16")[0]); self.assertFalse(S.compare("ge", 8, 0, "x")[0])
        self.assertTrue(S.compare("contains_all", ["noexec", "nosuid"], 0, "rw,nosuid,noexec")[0])
        self.assertTrue(S.compare("rc_nonzero", None, 124, "")[0]); self.assertFalse(S.compare("rc0", None, 1, "")[0])
        self.assertTrue(S.compare("re", "^(200|301)$", 0, "301")[0])

    def test_tuning_merge_precedence_and_values(self):
        sysctl, thp, limits, conflicts = S.merge_tuning(CTX, "data-1")
        self.assertEqual(sysctl["vm.nr_hugepages"], 300)
        self.assertEqual(thp, "never")
        self.assertEqual(limits["postgresql@"]["LimitNOFILE"], 65536)
        self.assertEqual(S.merge_tuning(CTX, "k8s-wk-1")[0]["net.ipv4.ip_forward"], 1)
        self.assertEqual(S.merge_tuning(CTX, "bao-1")[0]["kernel.core_pattern"], "|/bin/false")

    def test_tuning_file_loads_after_baseline_hardening(self):
        # sysctl --system nạp theo thứ tự tên tệp: tuning phải đứng sau baseline để giá trị theo dịch vụ thắng khi trùng khoá
        tasks = (S.LABS / "ansible/roles/os_tuning/tasks/main.yml").read_text(encoding="utf-8")
        base = (S.LABS / "ansible/roles/baseline/tasks/kernel.yml").read_text(encoding="utf-8")
        import re
        t_name = re.search(r"/etc/sysctl.d/([\w.-]+)", tasks)[1]
        b_name = re.search(r"/etc/sysctl.d/([\w.-]+)", base)[1]
        self.assertGreater(t_name, b_name)

    def test_ansible_merge_equals_python_merge(self):
        import shutil
        if not shutil.which("ansible-playbook"):
            self.skipTest("không có ansible")
        import gen_nft as g3
        allvms = [(CTX, v) for v in CTX["prof"]["vms"]]
        zf, pf, ff = g3.load("full")
        fctx = {"prof": pf, "tuning": CTX["tuning"]}
        allvms += [(fctx, v) for v in pf["vms"]]
        for ctx_, vm in allvms:
            groups = sorted(S.groups_of(vm))
            with tempfile.TemporaryDirectory() as d:
                out = Path(d) / "m.json"
                ev = json.dumps({"tuning_groups": groups, "out_file": str(out)})
                r = subprocess.run(["ansible-playbook", "-c", "local", "-i", "localhost,", "tests/tuning_merge.yml", "-e", ev],
                                   cwd=S.LABS / "ansible", capture_output=True, text=True, env={**__import__("os").environ, "ANSIBLE_ROLES_PATH": "roles"})
                self.assertEqual(r.returncode, 0, r.stdout[-400:] + r.stderr[-400:])
                got = json.loads(out.read_text())
            sysctl, thp, limits, _ = S.merge_tuning(ctx_, vm["name"])
            self.assertEqual({k: str(v) for k, v in got["sysctl"].items()}, {k: str(v) for k, v in sysctl.items()}, vm["name"])
            self.assertEqual(got["thp"], thp, vm["name"])
            self.assertEqual(got["modules"], S.merge_modules(ctx_, vm["name"]), vm["name"])
            self.assertEqual(got["limits"], limits, vm["name"])

    def test_vm_boot_checks_follow_profile(self):
        chs = S.generated_checks(CTX, {"use": "vm_boot", "vms": ["edge-1"]})
        mem = next(c for c in chs if "RAM" in c["what"])
        self.assertEqual(mem["value"], "3GiB")
        gw = next(c for c in chs if c["what"] == "gateway là fw-1")
        self.assertEqual(gw["value"], "10.10.10.1")

    def test_disks_checks(self):
        chs = S.generated_checks(CTX, {"use": "disks", "vms": ["data-1"]})
        self.assertTrue(any("/var/lib/waap/wal" in c["what"] for c in chs))
        wal_opts = next(c for c in chs if "tuỳ chọn mount" in c["what"] and "wal" in c["what"])
        self.assertIn("noexec", wal_opts["value"])

    def test_verify_pass_fail_and_state(self):
        with tempfile.TemporaryDirectory() as d:
            S.STATE = Path(d) / "s.json"
            step = {"id": "T.1", "title": "t", "why": "w", "checks": [{"at": "host", "what": "x", "cmd": "echo 1", "op": "eq", "value": "1"}]}
            ctx = {**CTX, "steps": {"phases": {"T": "t"}, "steps": [step]}}
            self.assertEqual(S.cmd_verify(ctx, "T.1", False), 0)
            self.assertIn("verified", json.loads(S.STATE.read_text())["T.1"])
            step["checks"][0]["value"] = "2"
            S.STATE = Path(d) / "s2.json"
            self.assertEqual(S.cmd_verify(ctx, "T.1", False), 1)
            self.assertFalse(S.STATE.exists())

    def test_run_refuses_unverified_dependency(self):
        with tempfile.TemporaryDirectory() as d:
            S.STATE = Path(d) / "s.json"
            with self.assertRaises(SystemExit) as cm:
                S.cmd_run(CTX, "P1.2", True, False)
            self.assertIn("P1.1", str(cm.exception))

    def test_placement_doc_is_current(self):
        import gen_lab
        self.assertEqual(gen_lab.placement("compact"), (S.LABS / "PLACEMENT.md").read_text(encoding="utf-8"), "chạy `gen_lab.py placement compact -o labs/PLACEMENT.md` rồi commit")
        self.assertEqual(gen_lab.placement("full"), (S.LABS / "PLACEMENT.full.md").read_text(encoding="utf-8"), "chạy `gen_lab.py placement full -o labs/PLACEMENT.full.md` rồi commit")

    def test_steps_for_full_not_yet_available_is_explicit(self):
        with self.assertRaises(SystemExit) as cm:
            S.load("full")
        self.assertIn("chưa có bước", str(cm.exception))

    def test_rke2_nodes_have_port_scoped_rules_not_blanket(self):
        import gen_nft as g2
        z, p, f = g2.load("compact")
        self.assertNotIn("local", f)  # luật "mở hết Z3" đã bỏ
        wk = g2.render_host(z, p, f, "k8s-wk-1")
        cp = g2.render_host(z, p, f, "k8s-cp-1")
        self.assertIn('"L02a"', cp); self.assertNotIn('"L02a"', wk)           # chỉ server nhận 6443 và 9345
        self.assertNotIn("2379", cp + wk)                                       # etcd không mở cho node nào khác
        self.assertIn("flannel.1", cp); self.assertIn("flannel.1", wk)          # pod network theo giao diện
        pr = g2.probes("compact")
        self.assertTrue(any(x["id"] == "XL1" and x["port"] == 22 and x["expect"] == "blocked" for x in pr))
        self.assertTrue(any(x["id"] == "XL2" and x["port"] == 2379 for x in pr))

    def test_intra_forbidden_violation_is_detected(self):
        import gen_nft as g2
        z, p, f = g2.load("compact")
        f["flows"].append({"id": "BAD", "src": "role:k8s-app", "dst": "role:k8s-cp", "proto": "tcp", "ports": [2379], "enforce": "host", "covers": []})
        orig = g2.load
        g2.load = lambda prof: (z, p, f)
        try:
            self.assertTrue(any("XL2" in e for e in g2.check("compact")))
        finally:
            g2.load = orig

    def test_checklist_doc_is_current(self):
        with tempfile.TemporaryDirectory() as d:
            orig = S.CHECKLIST
            S.CHECKLIST = Path(d) / "C.md"
            try:
                S.cmd_doc(CTX)
                self.assertEqual(S.CHECKLIST.read_text(encoding="utf-8"), orig.read_text(encoding="utf-8"), "chạy `steps.py doc` rồi commit")
            finally:
                S.CHECKLIST = orig


if __name__ == "__main__":
    unittest.main()
