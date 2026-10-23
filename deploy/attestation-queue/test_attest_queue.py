#!/usr/bin/env python3
"""Tests for attest_queue.py v2 against a fake helper (stdlib only): python3 -I test_attest_queue.py

The fake helper answers POST /v1/attestation after 0.2 s, counts TCP connections and peak in-flight requests, and can
refuse beyond a limit with 429 like the old helper. Each case starts the queue as a subprocess with its env switches.
"""
import http.client
import http.server
import os
import socket
import subprocess
import sys
import threading
import time
import unittest
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class FakeHelper:
    def __init__(self, limit=64, delay=0.2, idle_close_s=None):
        self.limit, self.delay, self.idle_close_s = limit, delay, idle_close_s
        self.lock = threading.Lock()
        self.in_flight = self.peak = self.connections = self.served = 0
        self.seq = 0
        fake = self

        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"
            timeout = idle_close_s  # server closes an idle keep-alive connection after this

            def setup(self):
                super().setup()
                with fake.lock:
                    fake.connections += 1

            def log_message(self, *_):
                pass

            def do_POST(self):
                assert self.headers.get("Authorization") == "Bearer t", "auth header not passed through"
                with fake.lock:
                    if fake.in_flight >= fake.limit:
                        busy = True
                    else:
                        busy = False
                        fake.in_flight += 1
                        fake.peak = max(fake.peak, fake.in_flight)
                        fake.seq += 1
                        n = fake.seq
                if busy:
                    body = b'{"error":"helper busy"}'
                    self.send_response(429)
                else:
                    time.sleep(fake.delay)
                    with fake.lock:
                        fake.in_flight -= 1
                        fake.served += 1
                    body = ('{"attestation":"proof-%d"}' % n).encode()
                    self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.port = free_port()
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", self.port), Handler)
        self.server.daemon_threads = True
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


class Queue:
    def __init__(self, upstream_port, **env):
        self.port = free_port()
        full_env = dict(os.environ, QUEUE_PORT=str(self.port), QUEUE_UPSTREAM_PORT=str(upstream_port), **env)
        self.proc = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "attest_queue.py")], env=full_env)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                socket.create_connection(("127.0.0.1", self.port), timeout=0.2).close()
                return
            except OSError:
                time.sleep(0.05)
        raise RuntimeError("queue did not start")

    def post(self):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=30)
        try:
            conn.request("POST", "/v1/attestation", headers={"Authorization": "Bearer t"})
            resp = conn.getresponse()
            return resp.status, resp.read()
        finally:
            conn.close()

    def burst(self, k):
        started = time.monotonic()
        with ThreadPoolExecutor(k) as pool:
            results = list(pool.map(lambda _: self.post(), range(k)))
        return results, time.monotonic() - started

    def close(self):
        self.proc.terminate()
        self.proc.wait(5)


class QueueTests(unittest.TestCase):
    def run_case(self, helper_kwargs, queue_env, k):
        helper = FakeHelper(**helper_kwargs)
        queue = Queue(helper.port, **queue_env)
        try:
            results, wall = queue.burst(k)
            return helper, results, wall
        finally:
            queue.close()
            helper.close()

    def test_default_is_one_at_a_time(self):
        helper, results, wall = self.run_case({}, {}, 4)
        self.assertEqual([s for s, _ in results], [200] * 4)
        self.assertEqual(helper.peak, 1)
        self.assertGreaterEqual(wall, 0.8)
        self.assertEqual(helper.connections, 4)  # a new connection per request, as before

    def test_concurrency_forwards_in_parallel(self):
        helper, results, wall = self.run_case({}, {"QUEUE_CONCURRENCY": "4"}, 4)
        self.assertEqual([s for s, _ in results], [200] * 4)
        self.assertEqual(helper.peak, 4)
        self.assertLess(wall, 0.6)
        self.assertEqual(len({b for _, b in results}), 4)  # every caller its own proof

    def test_concurrency_never_exceeds_the_setting(self):
        helper, results, _ = self.run_case({}, {"QUEUE_CONCURRENCY": "3"}, 8)
        self.assertEqual([s for s, _ in results], [200] * 8)
        self.assertEqual(helper.peak, 3)

    def test_concurrency_against_old_serial_helper_still_succeeds(self):
        helper, results, _ = self.run_case({"limit": 1}, {"QUEUE_CONCURRENCY": "4"}, 4)
        self.assertEqual([s for s, _ in results], [200] * 4)
        self.assertEqual(helper.served, 4)

    def test_keepalive_reuses_connections_and_survives_idle_close(self):
        helper = FakeHelper(delay=0.01, idle_close_s=0.3)
        queue = Queue(helper.port, QUEUE_KEEPALIVE="1")
        try:
            for _ in range(5):
                self.assertEqual(queue.post()[0], 200)
            self.assertEqual(helper.connections, 1)
            time.sleep(0.6)  # the helper closes the idle connection
            self.assertEqual(queue.post()[0], 200)  # stale one dropped, retried on a new connection
            self.assertEqual(helper.connections, 2)
            self.assertEqual(helper.served, 6)
        finally:
            queue.close()
            helper.close()

    def test_bad_concurrency_refused(self):
        proc = subprocess.run([sys.executable, "-I", os.path.join(HERE, "attest_queue.py")],
                              env=dict(os.environ, QUEUE_PORT=str(free_port()), QUEUE_CONCURRENCY="0"),
                              capture_output=True, timeout=10)
        self.assertNotEqual(proc.returncode, 0)


if __name__ == "__main__":
    unittest.main(verbosity=2)
