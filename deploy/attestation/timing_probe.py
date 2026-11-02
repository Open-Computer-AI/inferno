#!/usr/bin/env python3
"""Low-rate HTTP phase measurements. Never retain bearer tokens or proof bodies."""
import http.client
import json
import math
import statistics
import time


def quantiles(values):
    values = sorted(values)
    result = {name: round(values[max(0, math.ceil(len(values) * p) - 1)], 3)
              for name, p in (("min", 0), ("p95", .95), ("p99", .99), ("max", 1))} if values else {}
    if values:
        result["p50"] = round(statistics.median(values), 3)
    return result


class TimedConnection(http.client.HTTPConnection):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.connect_ms = 0
        self.connect_count = 0

    def connect(self):
        started = time.monotonic()
        super().connect()
        self.connect_count += 1
        self.connect_ms += (time.monotonic() - started) * 1000


def valid_payload(path, payload):
    if path == "/health":
        return payload.get("supported") is True
    proof = json.loads(payload["attestation"])
    return (type(proof.get("v")) is int and proof["v"] == 1
            and type(proof.get("s")) is int and proof["s"] == 0
            and isinstance(proof.get("t"), str) and proof["t"].startswith("v1."))


def measure(port, path, token, connection=None):
    if port not in (19085, 19095, 19395, 19389):
        raise ValueError("Unexpected measurement port")
    conn = connection or TimedConnection("127.0.0.1", port, timeout=8)
    reused = conn.sock is not None
    old_connect = conn.connect_ms
    old_connect_count = conn.connect_count
    start = time.monotonic()
    try:
        if not reused:
            conn.connect()
        connected = time.monotonic()
        connect_after_explicit = conn.connect_ms
        conn.request("GET" if path == "/health" else "POST", path,
                     body=None if path == "/health" else b"",
                     headers={"Authorization": "Bearer " + token})
        sent = time.monotonic()
        response = conn.getresponse()
        headers = time.monotonic()
        body = response.read(20481)
        ended = time.monotonic()
        payload = json.loads(body) if response.status == 200 and len(body) <= 20480 else {}
        valid = response.status == 200 and valid_payload(path, payload)
        timing_header = response.getheader("X-Inferno-Diagnostic-Timing")
        result = {"observed_at": time.time(), "port": port, "path": path,
                  "status": response.status, "valid_envelope": valid,
                  "reused_connection": reused and conn.connect_count == old_connect_count,
                  "reconnected_during_request": reused and conn.connect_count != old_connect_count,
                  "connection_closed_by_server": response.will_close,
                  "chunked_response": response.chunked,
                  "content_length_header": response.getheader("Content-Length"),
                  "response_body_bytes": len(body),
                  "connect_ms": round(conn.connect_ms - old_connect, 3),
                  "request_send_ms": round((sent - connected) * 1000 - (conn.connect_ms - connect_after_explicit), 3),
                  "headers_wait_ms": round((headers - sent) * 1000, 3),
                  "body_read_ms": round((ended - headers) * 1000, 3),
                  "total_ms": round((ended - start) * 1000, 3)}
        if timing_header:
            result["queue_server"] = json.loads(timing_header)
        return result
    finally:
        if connection is None:
            conn.close()


def summarize(rows):
    result = {"count": len(rows), "valid_count": sum(x.get("valid_envelope") is True for x in rows),
              "connection_reuse_count": sum(x.get("reused_connection") is True for x in rows)}
    for name in ("connect_ms", "request_send_ms", "headers_wait_ms", "body_read_ms", "total_ms"):
        result[name] = quantiles([x[name] for x in rows if name in x])
    return result


def instrument_queue(module, upstream_port=19085):
    """Instrument a separately bound copy, preserving its forwarding/retry code."""
    import threading
    state = threading.local()
    original_forward = module.forward
    original_handle = module.Handler._handle
    original_send = module.Handler._send

    class TimedSemaphore:
        def __init__(self):
            self.lock = threading.BoundedSemaphore(16)

        def acquire(self, *args, **kwargs):
            started = time.monotonic()
            answer = self.lock.acquire(*args, **kwargs)
            state.row["admission_wait_ms"] += (time.monotonic() - started) * 1000
            return answer

        def release(self):
            self.lock.release()

    class ForwardConnection(TimedConnection):
        def request(self, *args, **kwargs):
            started = time.monotonic()
            previous_connect = self.connect_ms
            super().request(*args, **kwargs)
            state.attempt["connect_ms"] = self.connect_ms - previous_connect
            state.attempt["send_including_connect_ms"] = (time.monotonic() - started) * 1000

        def getresponse(self):
            started = time.monotonic()
            response = super().getresponse()
            state.attempt["headers_wait_ms"] = (time.monotonic() - started) * 1000
            state.attempt["chunked_response"] = response.chunked
            state.attempt["content_length_header"] = response.getheader("Content-Length")
            original_read = response.read

            def read(*args, **kwargs):
                began = time.monotonic()
                try:
                    return original_read(*args, **kwargs)
                finally:
                    state.attempt["body_read_ms"] = (time.monotonic() - began) * 1000
            response.read = read
            return response

    def forward(*args, **kwargs):
        state.attempt = {}
        state.row["attempts"].append(state.attempt)
        started = time.monotonic()
        try:
            return original_forward(*args, **kwargs)
        finally:
            state.attempt["total_ms"] = (time.monotonic() - started) * 1000

    def handle(handler):
        state.row = {"admission_wait_ms": 0, "attempts": []}
        state.started = time.monotonic()
        return original_handle(handler)

    def send(handler, status, headers, body, close=False):
        state.row["server_before_response_ms"] = (time.monotonic() - state.started) * 1000
        return original_send(handler, status, list(headers) + [("X-Inferno-Diagnostic-Timing", json.dumps(state.row))], body, close)

    module.lock = TimedSemaphore()
    module._new_connection = lambda: ForwardConnection("127.0.0.1", upstream_port, timeout=15)
    module.forward = forward
    module.Handler._handle = handle
    module.Handler.do_GET = handle
    module.Handler.do_POST = handle
    module.Handler._send = send


def run_aws_measurements(sample_pairs=12):
    """One request at a time; temporary instrumented queue never changes production."""
    import ast
    import hashlib
    import http.server
    import pathlib
    import subprocess
    import threading
    import types
    source = pathlib.Path("/opt/inferno-queue/attest_queue.py").read_bytes()
    if hashlib.sha256(source).hexdigest() != "9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d":
        raise RuntimeError("Queue source drift")
    queue = types.ModuleType("isolated_timing_queue")
    exec(compile(source, "<isolated-queue-copy>", "exec"), queue.__dict__)
    instrument_queue(queue)
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 19395), queue.Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    connection = TimedConnection("127.0.0.1", 19085, timeout=8)
    rows = []
    aborted = None

    def production_stats():
        conn = http.client.HTTPConnection("127.0.0.1", 19095, timeout=2)
        try:
            conn.request("GET", "/queue-health")
            return ast.literal_eval(conn.getresponse().read(4096).decode())
        finally:
            conn.close()

    try:
        container = json.loads(subprocess.check_output(["sudo", "docker", "inspect", "inferno-validation-gateway"]))[0]
        environment = dict(x.split("=", 1) for x in container["Config"]["Env"] if "=" in x)
        token = subprocess.check_output(["sudo", "docker", "exec", "inferno-validation-gateway", "cat",
                                         environment["LIVE_ATTESTATION_TOKEN_FILE"]], text=True).strip()
        before = production_stats()
        profiles = [("production-queue-fresh", 19095, None), ("helper-fresh", 19085, None),
                    ("helper-reused", 19085, connection), ("instrumented-queue-copy", 19395, None)]
        for pair in range(sample_pairs):
            for name, port, persistent in (profiles if pair % 2 == 0 else profiles[::-1]):
                for path in ("/health", "/v1/attestation"):
                    try:
                        row = measure(port, path, token, persistent)
                        row.update(profile=name, pair=pair)
                    except Exception as exc:
                        row = {"profile": name, "pair": pair, "path": path, "error_class": type(exc).__name__}
                    rows.append(row)
                    if row.get("valid_envelope") is not True or row.get("total_ms", 999999) >= 4000:
                        aborted = "response_error_or_latency_guard"
                        break
                    time.sleep(.25)
                if aborted:
                    break
            if production_stats()["failed"] != before["failed"]:
                aborted = "production_failed_counter_increased"
            print(json.dumps({"timing_progress": pair + 1, "pairs_planned": sample_pairs,
                              "samples": len(rows), "aborted": aborted}), flush=True)
            if aborted:
                break
        after = production_stats()
        groups = {}
        for name, _, _ in profiles:
            for path in ("/health", "/v1/attestation"):
                selected = [x for x in rows if x["profile"] == name and x["path"] == path
                            and (name != "helper-reused" or x.get("reused_connection"))]
                groups[name + path] = summarize(selected)
        return {"rows": rows, "summaries": groups, "aborted": aborted,
                "production_stats_before": before, "production_stats_after": after,
                "diagnostic_queue_stats": dict(queue.stats), "gateway_pid": container["State"]["Pid"],
                "queue_pid": subprocess.check_output(["systemctl", "show", "inferno-attestation-queue.service",
                                                       "-p", "MainPID", "--value"], text=True).strip()}
    finally:
        connection.close()
        server.shutdown()
        server.server_close()


def run_air_measurements(sample_pairs=12):
    """On-Air loopback timings include HTTP/helper/Node/native work, not pure DeviceCheck."""
    import pathlib
    token = (pathlib.Path.home() / "Library/Application Support/InfernoAttestationAWS/attestation-token").read_text().strip()
    rows = []
    aborted = None
    for pair in range(sample_pairs):
        for path in ("/health", "/v1/attestation"):
            try:
                row = measure(19085, path, token)
                row["pair"] = pair
            except Exception as exc:
                row = {"pair": pair, "path": path, "error_class": type(exc).__name__}
            rows.append(row)
            if row.get("valid_envelope") is not True or row.get("total_ms", 999999) >= 4000:
                aborted = "response_error_or_latency_guard"
                break
        if aborted:
            break
        time.sleep(1)
    return {"rows": rows, "aborted": aborted, "summaries": {path: summarize([x for x in rows if x["path"] == path])
            for path in ("/health", "/v1/attestation")}}
