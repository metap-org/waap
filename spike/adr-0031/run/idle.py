#!/usr/bin/env python3
"""Mở N kết nối TCP rảnh tới một cổng, đo RSS của tiến trình khớp mẫu trước/sau.
Dùng: idle.py <port> <mẫu pgrep -f> [N=10000]"""
import socket, subprocess, sys, time
port, pat = int(sys.argv[1]), sys.argv[2]
N = int(sys.argv[3]) if len(sys.argv) > 3 else 10000
def rss():
    out = subprocess.run("for p in $(pgrep -f '%s'); do awk '/VmRSS/{print $2}' /proc/$p/status; done" % pat,
                         shell=True, capture_output=True, text=True).stdout.split()
    return sum(int(x) for x in out) // 1024
b = rss(); ss = []
for i in range(N):
    try: ss.append(socket.create_connection(("127.0.0.1", port), 3))
    except Exception as e: print("không mở thêm được từ kết nối", i, e); break
time.sleep(2); a = rss()
s = socket.create_connection(("127.0.0.1", port), 3)
s.sendall(b"GET /id HTTP/1.1\r\nHost: b.test\r\nConnection: close\r\n\r\n"); r = s.recv(100)
print(f"port {port}: conns={len(ss)} rss {b}MB -> {a}MB (+{a-b}MB, {(a-b)*1024/max(len(ss),1):.1f} KB/conn) request mới sau đó: {r[:15]!r}")
