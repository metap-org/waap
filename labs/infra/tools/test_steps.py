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
        self.assertEqual(limits["postgresql@16-main"]["LimitNOFILE"], 65536)
        self.assertEqual(S.merge_tuning(CTX, "k8s-wk-1")[0]["net.ipv4.ip_forward"], 1)
        self.assertEqual(S.merge_tuning(CTX, "bao-1")[0]["kernel.core_pattern"], "|/bin/false")

    def test_baseline_does_not_override_tuning(self):
        # file 99-waap-hardening chạy sau 95-waap-tuning: không khoá nào của tuning được trùng với khoá baseline (trừ ip_forward, đã xử lý qua sysctl_ip_forward)
        base = (S.LABS / "ansible/roles/baseline/templates/sysctl-hardening.conf.j2").read_text(encoding="utf-8")
        base_keys = {l.split("=")[0].strip() for l in base.splitlines() if "=" in l and not l.startswith("#")}
        for name in ("data-1", "ch-1", "edge-1", "bao-1", "k8s-cp-1", "k8s-wk-1"):
            for k in S.merge_tuning(CTX, name)[0]:
                if k in base_keys:
                    self.assertEqual(k, "net.ipv4.ip_forward", f"{name}: {k} bị baseline ghi đè")
        for g in ("role_k8s_cp", "role_k8s_app"):
            self.assertIn("sysctl_ip_forward: 1", (S.LABS / f"ansible/group_vars/{g}.yml").read_text(encoding="utf-8"))

    def test_ansible_merge_equals_python_merge(self):
        import shutil
        if not shutil.which("ansible-playbook"):
            self.skipTest("không có ansible")
        for vm in CTX["prof"]["vms"]:
            groups = sorted(S.groups_of(vm))
            with tempfile.TemporaryDirectory() as d:
                out = Path(d) / "m.json"
                ev = json.dumps({"tuning_groups": groups, "out_file": str(out)})
                r = subprocess.run(["ansible-playbook", "-c", "local", "-i", "localhost,", "tests/tuning_merge.yml", "-e", ev],
                                   cwd=S.LABS / "ansible", capture_output=True, text=True, env={**__import__("os").environ, "ANSIBLE_ROLES_PATH": "roles"})
                self.assertEqual(r.returncode, 0, r.stdout[-400:] + r.stderr[-400:])
                got = json.loads(out.read_text())
            sysctl, thp, limits, _ = S.merge_tuning(CTX, vm["name"])
            self.assertEqual({k: str(v) for k, v in got["sysctl"].items()}, {k: str(v) for k, v in sysctl.items()}, vm["name"])
            self.assertEqual(got["thp"], thp, vm["name"])
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
