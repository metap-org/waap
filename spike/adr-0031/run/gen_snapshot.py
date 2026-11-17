#!/usr/bin/env python3
"""Sinh snapshot cho l7-pingora. Dùng: gen_snapshot.py base|rule|big  (in JSON ra stdout).
base: các zone dùng chung cho mọi test.   rule: như base nhưng đổi ACL/rps của b.test (test đổi rule nóng).
big : thêm 10.000 zone và 1.000 cert (test cấu hình lớn)."""
import json, os, sys
W = os.environ.get("WORK", "/tmp/adr0031")
o1, o2 = "127.0.0.1:9300", "127.0.0.1:9301"
z = lambda host, **k: dict(host=host, id="z-" + host, origin=o1, **k)
zones = [
    z("a.test", waf=True), z("b.test", waf=False), z("c.test", waf=True),
    z("lb.test", origins=[o1, o2]), z("lbw.test", origins=[o1, o2], waf=True),
    z("d.test", acl_deny=["127.0.0.1"]),       # flood / pre-filter: chặn mọi request từ loopback
    z("ca.test", cache=True),                  # cache + purge
]
certs = {f"{h}.test": dict(cert=f"{W}/{h}.test.crt", key=f"{W}/{h}.test.key") for h in "abc"}
version = 1
mode = sys.argv[1] if len(sys.argv) > 1 else "base"
if mode in ("rule", "big"):
    version = 2
    for zz in zones:
        if zz["host"] == "b.test":
            zz["rps"] = 100000
            zz["acl_deny"] = ["203.0.113.9"]
if mode == "big":
    version = 100
    for i in range(10000):
        zones.append(dict(host=f"z{i}.test", id=f"zz{i}", origin=o1))
    for i in range(1000):
        certs[f"s{i}.test"] = dict(cert=f"{W}/a.test.crt", key=f"{W}/a.test.key")
print(json.dumps(dict(version=version, zones=zones, certs=certs)))
