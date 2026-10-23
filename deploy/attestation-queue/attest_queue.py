#!/usr/bin/env python3
"""Attestation queue for the AWS Inferno box (owner 2026-10-18).

The remote DeviceCheck helper behind 127.0.0.1:19085 serves one request at a time and answers any concurrent request
with 429 {"error":"helper busy"}; the gateway turns that into a failed call (503). The Air's built-in attestation is
just as serial but makes callers wait on a lock. This proxy gives the AWS gateway the same behaviour: it listens on
127.0.0.1:19095, forwards POST /v1/attestation to the helper one at a time, and retries "busy" until the deadline.
A helper it cannot reach (tunnel reconnecting, refused or reset connection) is retried for UNREACHABLE_S, so a blip
does not fail the call; a real outage still fails fast. A slow answer (timeout) is not retried: it would stall the line.
Headers (including Authorization) are passed through and never logged. Stdlib only.

v2 (2026-10-23), two switches, both off by default (off = the behaviour above, unchanged):
- QUEUE_CONCURRENCY=N (default 1): forward up to N requests at once. Only useful with a helper that serves N at once
  (attestation helper v2, -max-concurrent >= N); against the old helper the extra ones get 429 and are retried as before.
- QUEUE_KEEPALIVE=1 (default 0): reuse idle connections to the helper (kept at most KEEPALIVE_IDLE_S, below the helper's
  15 s idle timeout). Through the ssh reverse tunnel every new connection costs one extra round trip to the Mac (channel
  open) before the request; a reused one does not. A reused connection that turns out closed is retried once on a new
  one (each POST generates a new proof, so a retry never reuses a proof).
"""
import http.client
import http.server
import threading
import time

UPSTREAM_HOST, UPSTREAM_PORT = "127.0.0.1", 19085
LISTEN = ("127.0.0.1", 19095)
DEADLINE_S = 25.0     # longest a caller waits for its turn
RETRY_S = 0.05        # pause before retrying a "busy" answer
UNREACHABLE_S = 6.0   # how long a caller keeps retrying a helper it cannot reach
UNREACHABLE_RETRY_S = 0.25
UNREACHABLE = (ConnectionRefusedError, ConnectionResetError, http.client.RemoteDisconnected)
CONCURRENCY = 1
KEEPALIVE = False
KEEPALIVE_IDLE_S = 10.0
lock = threading.BoundedSemaphore(CONCURRENCY)  # CONCURRENCY 1 behaves exactly like the old threading.Lock()
stats = {"served": 0, "waited_ms_max": 0, "busy_retries": 0, "unreachable_retries": 0, "failed": 0}
idle = []  # (connection, time it went idle); only used with KEEPALIVE
idle_lock = threading.Lock()
STALE = (ConnectionResetError, BrokenPipeError, http.client.RemoteDisconnected, http.client.BadStatusLine)


def _new_connection():
    return http.client.HTTPConnection(UPSTREAM_HOST, UPSTREAM_PORT, timeout=15)


def _take_idle():
    now = time.monotonic()
    with idle_lock:
        while idle:
            conn, since = idle.pop()
            if now - since < KEEPALIVE_IDLE_S:
                return conn
            conn.close()
    return None


def _give_back(conn):
    with idle_lock:
        idle.append((conn, time.monotonic()))


def _exchange(conn, method, path, headers, body):
    conn.request(method, path, body=body, headers=headers)
    resp = conn.getresponse()
    data = resp.read()
    return resp.status, resp.getheaders(), data, resp.will_close


def forward(method, path, headers, body):
    if not KEEPALIVE:
        conn = _new_connection()
        try:
            conn.request(method, path, body=body, headers=headers)
            resp = conn.getresponse()
            return resp.status, resp.getheaders(), resp.read()
        finally:
            conn.close()
    conn = _take_idle()
    if conn is not None:
        try:
            status, rheaders, data, will_close = _exchange(conn, method, path, headers, body)
        except STALE:
            conn.close()  # closed while idle: retry once on a new connection below
        except Exception:
            conn.close()
            raise
        else:
            _give_back(conn) if not will_close else conn.close()
            return status, rheaders, data
    conn = _new_connection()
    try:
        status, rheaders, data, will_close = _exchange(conn, method, path, headers, body)
    except Exception:
        conn.close()
        raise
    _give_back(conn) if not will_close else conn.close()
    return status, rheaders, data


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):  # no request logging (headers carry a token)
        pass

    def _send(self, status, headers, body):
        self.send_response(status)
        for k, v in headers:
            if k.lower() not in ("transfer-encoding", "connection", "content-length"):
                self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _handle(self):
        if self.path == "/queue-health":
            body = (str(stats) + "\n").encode()
            return self._send(200, [("Content-Type", "text/plain")], body)
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else None
        headers = {k: v for k, v in self.headers.items() if k.lower() not in ("host", "connection", "content-length")}
        started = time.monotonic()
        last = (502, [("Content-Type", "application/json")], b'{"error":"attestation queue timeout"}')
        while time.monotonic() - started < DEADLINE_S:
            remaining = DEADLINE_S - (time.monotonic() - started)
            if not lock.acquire(timeout=max(0.01, remaining)):
                break
            unreachable = False
            try:
                status, rheaders, rbody = forward(self.command, self.path, headers, body)
            except UNREACHABLE:
                unreachable = True
                status, rheaders, rbody = 502, [("Content-Type", "application/json")], b'{"error":"helper unreachable"}'
            except Exception:  # noqa: BLE001 - timeout or other failure: answered 502 at once
                status, rheaders, rbody = 502, [("Content-Type", "application/json")], b'{"error":"helper unreachable"}'
            finally:
                lock.release()
            last = (status, rheaders, rbody)
            if unreachable and time.monotonic() - started < UNREACHABLE_S:
                stats["unreachable_retries"] += 1
                time.sleep(UNREACHABLE_RETRY_S)
                continue
            if status == 429:
                stats["busy_retries"] += 1
                time.sleep(RETRY_S)
                continue
            break
        waited = int((time.monotonic() - started) * 1000)
        stats["waited_ms_max"] = max(stats["waited_ms_max"], waited)
        stats["served" if last[0] == 200 else "failed"] += 1
        self._send(*last)

    do_POST = _handle
    do_GET = _handle


if __name__ == "__main__":
    import os  # ports overridable so the test can run it next to a fake helper
    LISTEN = (LISTEN[0], int(os.environ.get("QUEUE_PORT", LISTEN[1])))
    UPSTREAM_PORT = int(os.environ.get("QUEUE_UPSTREAM_PORT", UPSTREAM_PORT))
    CONCURRENCY = int(os.environ.get("QUEUE_CONCURRENCY", "1"))
    if not 1 <= CONCURRENCY <= 64:
        raise SystemExit("QUEUE_CONCURRENCY must be 1-64")
    KEEPALIVE = os.environ.get("QUEUE_KEEPALIVE", "0") == "1"
    lock = threading.BoundedSemaphore(CONCURRENCY)
    server = http.server.ThreadingHTTPServer(LISTEN, Handler)
    server.daemon_threads = True
    server.serve_forever()
