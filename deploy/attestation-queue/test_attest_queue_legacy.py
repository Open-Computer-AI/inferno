"""Local test of attest_queue.py next to a fake helper (one at a time, 429 "busy" to overlaps, ~1 s each).

Cases: 6 at once with the helper up (all 200); helper down for 2 s then up (the blip: 200 after ~2 s); helper down
the whole time (fails after ~6 s, not 25). Runs on spare loopback ports; no box, no token.
"""
import http.client
import http.server
import os
import subprocess
import sys
import threading
import time

HELPER_PORT, QUEUE_PORT = 19385, 19395
HERE = os.path.dirname(os.path.abspath(__file__))
busy = threading.Lock()


class Helper(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def do_POST(self):
        if not busy.acquire(blocking=False):
            body, code = b'{"error":"helper busy"}', 429
        else:
            try:
                time.sleep(1.0)
                body, code = b'{"attestation":"x"}', 200
            finally:
                busy.release()
        self.send_response(code)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def start_helper():
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", HELPER_PORT), Helper)
    srv.daemon_threads = True
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv


def post():
    t = time.monotonic()
    c = http.client.HTTPConnection("127.0.0.1", QUEUE_PORT, timeout=40)
    c.request("POST", "/v1/attestation", headers={"Authorization": "Bearer test"})
    r = c.getresponse(); r.read()
    return r.status, round(time.monotonic() - t, 1)


env = dict(os.environ, QUEUE_PORT=str(QUEUE_PORT), QUEUE_UPSTREAM_PORT=str(HELPER_PORT))
queue = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "attest_queue.py")], env=env)
time.sleep(0.5)
ok = True
try:
    # 1. six at once, helper up
    helper = start_helper()
    results = []
    ts = [threading.Thread(target=lambda: results.append(post())) for _ in range(6)]
    [t.start() for t in ts]; [t.join() for t in ts]
    good = sum(1 for s, _ in results if s == 200)
    print("1 six at once:", sorted(results, key=lambda r: r[1]), "->", "PASS" if good == 6 else "FAIL")
    ok &= good == 6
    helper.shutdown(); helper.server_close()

    # 2. blip: helper down, back after 2 s
    threading.Timer(2.0, start_helper).start()
    s, dt = post()
    print("2 blip (down 2 s):", (s, dt), "->", "PASS" if s == 200 and 2.0 <= dt <= 4.5 else "FAIL")
    ok &= s == 200 and 2.0 <= dt <= 4.5
finally:
    pass

# 3. outage: restart the queue against a port nobody serves
queue.terminate(); queue.wait()
env["QUEUE_UPSTREAM_PORT"] = "19386"
queue = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "attest_queue.py")], env=env)
time.sleep(0.5)
s, dt = post()
print("3 outage:", (s, dt), "->", "PASS" if s == 502 and 5.5 <= dt <= 7.5 else "FAIL")
ok &= s == 502 and 5.5 <= dt <= 7.5
c = http.client.HTTPConnection("127.0.0.1", QUEUE_PORT, timeout=5); c.request("GET", "/queue-health")
print("queue-health:", c.getresponse().read().decode().strip())
queue.terminate(); queue.wait()
print("RESULT:", "PASS" if ok else "FAIL")
sys.exit(0 if ok else 1)
