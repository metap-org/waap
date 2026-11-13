import json, subprocess, sys, tempfile, unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent))
import gen_nft as g


class GenTests(unittest.TestCase):
    def setUp(self):
        self.zones, self.prof, self.flows = g.load("compact")

    def test_matrix_matches_doc_and_no_forbidden(self):
        self.assertEqual(g.check("compact"), [])

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


if __name__ == "__main__":
    unittest.main()
