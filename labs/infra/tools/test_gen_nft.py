import json, subprocess, sys, tempfile, unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent))
import gen_nft as g


class GenTests(unittest.TestCase):
    def setUp(self):
        self.zones, self.prof, self.flows = g.load("compact")

    def test_matrix_matches_doc_and_no_forbidden(self):
        for p in ("compact", "full"):
            with self.subTest(profile=p):
                self.assertEqual(g.check(p), [])

    def test_full_profile_is_consistent(self):
        z, prof, flows = g.load("full")
        names = [v["name"] for v in prof["vms"]]
        self.assertEqual(len(names), len(set(names)))
        seen = set()
        for v in prof["vms"]:
            for zn in v["zones"]:
                key = (zn, v["host"])
                self.assertNotIn(key, seen, f"trùng IP {key}")
                seen.add(key)
        self.assertEqual(len([v for v in prof["vms"] if "k8s-cp" in v["roles"]]), 3)
        self.assertEqual(len([v for v in prof["vms"] if "openbao" in v["roles"]]), 3)
        ips = g.vm_ips(z, prof)
        # etcd chỉ giữa server với nhau; worker không bao giờ có trong nguồn
        cp1 = g.render_host(z, prof, flows, "k8s-cp-1")
        self.assertIn("L02d", cp1)
        for wk in ("k8s-wk-1", "k8s-wk-2", "k8s-wk-3"):
            self.assertNotIn(ips[wk]["Z3"], next(l for l in cp1.splitlines() if "L02d" in l))
        self.assertNotIn("2379", g.render_host(z, prof, flows, "k8s-wk-1"))
        self.assertIn("L02e", cp1)  # WireGuard cho pod
        # full-only: compact không có
        _, _, fc = g.load("compact")
        self.assertFalse(any(f["id"] in ("L02d", "L02e") for f in fc["flows"]))

    def test_every_profile_renders_valid_nft_files(self):
        import shutil
        for p in ("compact", "full"):
            with self.subTest(profile=p):
                z, prof, flows = g.load(p)
                for bs in (True, False):
                    text, _ = g.render(z, prof, flows, bs)
                    self.assertIn("policy drop", text)
                    for v in prof["vms"]:
                        if "firewall" not in v["roles"]:
                            self.assertIn("policy drop", g.render_host(z, prof, flows, v["name"], bs))

    def test_default_deny_and_no_host_enforced_rules(self):
        text, _ = g.render(self.zones, self.prof, self.flows, bootstrap=False)
        self.assertIn("policy drop", text)
        self.assertNotIn("BOOTSTRAP", text)
        # N10 (edge-l7 -> edge-waf, cùng vùng) đi cầu L2: không được có luật ở fw-1.
        self.assertNotIn('"N10"', text)

    def test_bootstrap_only_when_asked(self):
        text, _ = g.render(self.zones, self.prof, self.flows, bootstrap=True)
        self.assertIn("BOOTSTRAP", text)

    def test_edge_cannot_reach_data(self):
        ips = g.vm_ips(self.zones, self.prof)
        rules, _ = g.build_rules(self.zones, self.prof, self.flows)
        edge = ips["edge-1"]["Z1"]
        data = {ips[n]["Z4"] for n in ("data-1", "ch-1", "kafka-1")} | {ips["bao-1"]["Z5"]}
        for f, s, d in rules:
            if edge in s:
                self.assertFalse(data & set(d), f["id"])

    def test_forbidden_violation_is_detected(self):
        self.flows["flows"].append({"id": "BAD", "src": "zone:Z1", "dst": "role:postgres", "proto": "tcp", "ports": [5432]})
        orig = g.load
        g.load = lambda p: (self.zones, self.prof, self.flows)
        try:
            self.assertTrue(any("X01" in e for e in g.check("compact")))
        finally:
            g.load = orig

    def test_probes_cover_allowed_and_forbidden(self):
        pr = g.probes("compact")
        self.assertTrue(any(p["id"] == "N14" and p["expect"] == "reach" for p in pr))
        self.assertTrue(any(p["id"] == "X01" and p["expect"] == "blocked" and p["port"] == 22 for p in pr))
        # Không thăm dò nào đòi "reach" mà nguồn và đích cùng vùng (fw-1 không thấy).
        self.assertTrue(all(p["from"] != "fw-1" for p in pr))

    def test_interim_flows_have_no_doc_ids(self):
        for f in self.flows["flows"]:
            if f.get("interim"):
                self.assertEqual(f.get("covers"), [], f["id"])


if __name__ == "__main__":
    unittest.main()
