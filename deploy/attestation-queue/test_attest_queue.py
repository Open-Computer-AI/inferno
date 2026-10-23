#!/usr/bin/env python3
"""Tests for attest_queue.py v2 against a fake helper (stdlib only): python3 -I test_attest_queue.py

The fake helper answers POST /v1/attestation after 0.2 s, counts TCP connections and peak in-flight requests, and can
refuse beyond a limit with 429 like the old helper. Each case starts the queue as a subprocess with its env switches.
"""
import http.client
import http.server
import os
import re
import socket
import subprocess
import sys
import threading
import time
import unittest
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
# QUEUE_UNDER_TEST=/path/to/attest_queue.py runs the same cases against another version (e.g. to show a test fails
# before a fix). Default: the queue next to this file.
QUEUE_FILE = os.environ.get("QUEUE_UNDER_TEST") or os.path.join(HERE, "attest_queue.py")
V1_FILE = os.path.join(HERE, "testdata", "attest_queue_v1.py")  # unchanged production copy (dograh md5 06a02f7d...)
# What the gateway's Go client (remote.go: POST, nil body, Authorization + Accept) puts on the wire, captured
# 2026-10-23 from the same construction (cmd/attest-load); the token is a test value.
GATEWAY_REQUEST = (b"POST /v1/attestation HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nUser-Agent: Go-http-client/1.1\r\n"
                   b"Content-Length: 0\r\nAccept: application/json\r\n"
                   b"Authorization: Bearer test-token-0123456789abcdef0123456789\r\nAccept-Encoding: gzip\r\n\r\n")


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


class RawUpstream:
    """Records each request's raw bytes and connection; answers with a fixed length, chunked, or close-delimited body."""

    def __init__(self, mode="fixed"):
        self.mode, self.requests, self.connections = mode, [], 0
        self.sock = socket.socket()
        self.sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.sock.bind(("127.0.0.1", 0))
        self.sock.listen(16)
        self.port = self.sock.getsockname()[1]
        threading.Thread(target=self._accept, daemon=True).start()

    def _accept(self):
        while True:
            try:
                conn, _ = self.sock.accept()
            except OSError:
                return
            self.connections += 1
            threading.Thread(target=self._serve, args=(conn,), daemon=True).start()

    def _serve(self, conn):
        buf = b""
        with conn:
            while True:
                while b"\r\n\r\n" not in buf:
                    chunk = conn.recv(65536)
                    if not chunk:
                        return
                    buf += chunk
                head, buf = buf.split(b"\r\n\r\n", 1)
                self.requests.append(head + b"\r\n\r\n")
                body = b'{"attestation":"proof"}'
                if self.mode == "fixed":
                    conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s" % (len(body), body))
                elif self.mode == "chunked":
                    conn.sendall(b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n0\r\n\r\n" % (len(body), body))
                else:  # close-delimited
                    conn.sendall(b"HTTP/1.1 200 OK\r\n\r\n" + body)
                    return

    def close(self):
        self.sock.close()


def raw_exchange(port, data):
    """Sends raw bytes to the queue; returns the status line's code (0 if the queue answered nothing)."""
    with socket.create_connection(("127.0.0.1", port), timeout=10) as s:
        s.sendall(data)
        reply = b""
        try:  # read the whole response (head + Content-Length body) so the queue never writes into a closed socket
            while True:
                if b"\r\n\r\n" in reply:
                    head, rest = reply.split(b"\r\n\r\n", 1)
                    match = re.search(rb"(?i)\r\ncontent-length: *([0-9]+)", head)
                    if not match or len(rest) >= int(match.group(1)):
                        break
                chunk = s.recv(65536)
                if not chunk:
                    break
                reply += chunk
        except (ConnectionResetError, socket.timeout):
            pass
    return int(reply.split(b" ", 2)[1]) if reply.startswith(b"HTTP/") else 0


class Queue:
    def __init__(self, upstream_port, queue_file=None, **env):
        self.port = free_port()
        full_env = dict(os.environ, QUEUE_PORT=str(self.port), QUEUE_UPSTREAM_PORT=str(upstream_port), **env)
        self.proc = subprocess.Popen([sys.executable, "-I", queue_file or QUEUE_FILE], env=full_env)
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

    def test_health_reports_in_flight(self):
        helper = FakeHelper(delay=0.6)
        queue = Queue(helper.port, QUEUE_CONCURRENCY="4")
        try:
            def health():
                conn = http.client.HTTPConnection("127.0.0.1", queue.port, timeout=5)
                conn.request("GET", "/queue-health")
                text = conn.getresponse().read().decode()
                conn.close()
                return text
            self.assertIn("'in_flight': 0", health())
            pool = ThreadPoolExecutor(3)
            futures = [pool.submit(queue.post) for _ in range(3)]
            time.sleep(0.3)
            self.assertIn("'in_flight': 3", health())
            self.assertEqual([f.result()[0] for f in futures], [200] * 3)
            pool.shutdown()
            self.assertIn("'in_flight': 0", health())
        finally:
            queue.close()
            helper.close()

    def test_bad_concurrency_refused(self):
        proc = subprocess.run([sys.executable, "-I", QUEUE_FILE],
                              env=dict(os.environ, QUEUE_PORT=str(free_port()), QUEUE_CONCURRENCY="0"),
                              capture_output=True, timeout=10)
        self.assertNotEqual(proc.returncode, 0)



class HygieneTests(unittest.TestCase):
    """Request smuggling / response desync class (security review 2026-10-23), for both switch settings."""

    SWITCHES = ({}, {"QUEUE_CONCURRENCY": "4", "QUEUE_KEEPALIVE": "1"})

    def with_queue(self, env, fn, mode="fixed", queue_file=None):
        upstream = RawUpstream(mode)
        queue = Queue(upstream.port, queue_file=queue_file, **env)
        try:
            return fn(upstream, queue)
        finally:
            queue.close()
            upstream.close()

    def test_gateway_request_is_forwarded_byte_for_byte_like_v1(self):
        def capture(upstream, queue):
            self.assertEqual(raw_exchange(queue.port, GATEWAY_REQUEST.replace(b"{port}", str(queue.port).encode())), 200)
            return upstream.requests[0].replace(str(upstream.port).encode(), b"UPSTREAM")
        v1 = self.with_queue({}, capture, queue_file=V1_FILE)
        for env in self.SWITCHES:
            self.assertEqual(self.with_queue(env, capture), v1, env)

    def test_only_gateway_headers_are_forwarded(self):
        extra = (b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\nAuthorization: Bearer t\r\n"
                 b"TE: trailers\r\nUpgrade: h2c\r\nKeep-Alive: timeout=99\r\nTrailer: X-Evil\r\n"
                 b"Proxy-Authorization: Basic eA==\r\nProxy-Connection: keep-alive\r\nX-Other: 1\r\n\r\n")
        for env in self.SWITCHES:
            def check(upstream, queue):
                self.assertEqual(raw_exchange(queue.port, extra), 200)
                forwarded = upstream.requests[0].lower()
                for name in (b"te:", b"upgrade:", b"keep-alive:", b"trailer:", b"proxy-", b"x-other:"):
                    self.assertNotIn(b"\r\n" + name, forwarded, (env, name))
                self.assertIn(b"\r\nauthorization: bearer t\r\n", forwarded)
            self.with_queue(env, check)

    def refused_without_forwarding(self, data, want):
        for env in self.SWITCHES:
            def check(upstream, queue):
                self.assertEqual(raw_exchange(queue.port, data), want, env)
                time.sleep(0.2)
                self.assertEqual(upstream.requests, [], env)
            self.with_queue(env, check)

    def test_transfer_encoding_refused(self):
        self.refused_without_forwarding(
            b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n", 400)

    def test_content_length_with_transfer_encoding_refused(self):
        self.refused_without_forwarding(
            b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n", 400)

    def test_malformed_content_length_refused(self):
        for value in (b"abc", b"-1", b"+1", b"1.5", b""):
            self.refused_without_forwarding(
                b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nContent-Length: " + value + b"\r\n\r\n", 400)

    def test_duplicate_content_length_refused(self):
        self.refused_without_forwarding(
            b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\nContent-Length: 4\r\n\r\n", 400)

    def test_large_body_refused(self):
        self.refused_without_forwarding(
            b"POST /v1/attestation HTTP/1.1\r\nHost: x\r\nContent-Length: 5000\r\n\r\n", 413)

    def test_keepalive_never_pools_after_chunked_or_close_delimited_response(self):
        for mode in ("chunked", "close"):
            def check(upstream, queue):
                for _ in range(2):
                    self.assertEqual(queue.post()[0], 200, mode)
                self.assertEqual(upstream.connections, 2, mode)
            self.with_queue({"QUEUE_KEEPALIVE": "1"}, check, mode=mode)

    def test_keepalive_pools_after_fixed_length_response(self):
        def check(upstream, queue):
            for _ in range(3):
                self.assertEqual(queue.post()[0], 200)
            self.assertEqual(upstream.connections, 1)
        self.with_queue({"QUEUE_KEEPALIVE": "1"}, check)


if __name__ == "__main__":
    unittest.main(verbosity=2)
