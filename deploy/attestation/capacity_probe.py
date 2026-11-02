#!/usr/bin/env python3
"""Bounded proof load probe. Run only against a separately provisioned loopback queue.

The controller must atomically refresh --telemetry with Air resource observations.
No production configuration is changed. Proof bodies and hashes are never emitted.
"""
import argparse
import collections
import concurrent.futures
import hashlib
import http.client
import json
import math
import os
import pathlib
import stat
import threading
import time


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * fraction) - 1)] if ordered else None


def resource_problem(sample, now):
    """Require a fresh, normal-pressure observation of the actual Air, not AWS."""
    for key in ("observed_at", "cpu_busy_percent", "swapouts_delta", "sequence"):
        if type(sample[key]) not in (int, float) or not math.isfinite(sample[key]):
            return "air_telemetry_invalid"
    if (sample["host"] != "100.70.23.11" or type(sample["sequence"]) is not int
            or sample["sequence"] < 0):
        return "air_telemetry_invalid"
    if not 0 <= sample["cpu_busy_percent"] <= 100 or sample["swapouts_delta"] < 0:
        return "air_telemetry_invalid"
    age = now - sample["observed_at"]
    if not 0 <= age <= 5:
        return "air_telemetry_stale"
    if type(sample["pressure"]) is not int or sample["pressure"] != 1:
        return "air_memory_pressure"
    if sample["cpu_busy_percent"] > 80:
        return "air_cpu_headroom"
    if sample["swapouts_delta"] > 0:
        return "air_swap_activity"
    if sample["thermal_warning"] is not False:
        return "air_thermal_warning"
    if sample["production_ok"] is not True:
        return "production_probe_failed"
    return None


def private_file(path, limit):
    """Reject symlinks and validate the opened inode, avoiding a stat/open race."""
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise ValueError("Expected an owner-only regular file")
        data = stream.read(limit + 1)
        if len(data) > limit:
            raise ValueError("Input exceeds size limit")
        return data


def read_token(path):
    token = private_file(path, 513).decode().strip()
    if not 32 <= len(token) <= 512 or any(c in token for c in " \t\r\n"):
        raise ValueError("Invalid helper token")
    return token


def valid_proof(payload):
    envelope = json.loads(payload["attestation"])
    return (type(envelope.get("v")) is int and envelope["v"] == 1
            and type(envelope.get("s")) is int and envelope["s"] == 0
            and isinstance(envelope.get("t"), str) and envelope["t"].startswith("v1."))


class Probe:
    def __init__(self, port, token, workers, duration, telemetry, stop_file, burst=False):
        if port != 19395:
            raise ValueError("Only isolated queue port 19395 is permitted")
        if workers not in (1, 4, 8, 16, 32, 33) or not 0 < duration <= 300:
            raise ValueError("Unsupported load or duration")
        self.port, self.token, self.workers = port, token, workers
        self.duration, self.telemetry, self.stop_file = duration, telemetry, stop_file
        self.burst = burst
        self.stop = threading.Event()
        self.lock = threading.Lock()
        self.latencies, self.errors, self.hashes = [], collections.Counter(), set()
        self.active = self.peak = self.completed = 0
        self.reason = None
        self.started = self.deadline = 0
        self.last_sequence = -1
        self.last_observed_at = None
        self.barrier = threading.Barrier(workers + 1)

    def abort(self, reason):
        with self.lock:
            if self.reason is None:
                self.reason = reason
        self.stop.set()

    def check_resources(self):
        try:
            if self.stop_file.exists():
                return "controller_stop"
            sample = json.loads(private_file(self.telemetry, 16384))
            problem = resource_problem(sample, time.time())
            if problem:
                return problem
            if sample["sequence"] < self.last_sequence:
                return "air_telemetry_regressed"
            # Polls may reread the same sample. A refreshed timestamp must carry
            # a new sequence, so a stuck producer cannot pretend to be healthy.
            if sample["sequence"] == self.last_sequence and sample["observed_at"] != self.last_observed_at:
                return "air_telemetry_reused_sequence"
            self.last_sequence = sample["sequence"]
            self.last_observed_at = sample["observed_at"]
            return None
        except (OSError, ValueError, KeyError, TypeError, OverflowError):
            return "air_telemetry_invalid"

    def worker(self):
        # Match production: fresh connection per POST; no client retries.
        self.barrier.wait()
        while not self.stop.is_set() and time.monotonic() < self.deadline:
            started = time.monotonic()
            conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=8)
            error = None
            digest = None
            with self.lock:
                self.active += 1
                self.peak = max(self.peak, self.active)
            try:
                conn.request("POST", "/v1/attestation", body=b"",
                             headers={"Authorization": "Bearer " + self.token})
                response = conn.getresponse()
                body = response.read(20481)
                if response.status != 200:
                    error = "http_" + str(response.status)
                elif len(body) > 20480 or not valid_proof(json.loads(body)):
                    error = "invalid_proof"
                else:
                    digest = hashlib.sha256(json.loads(body)["attestation"].encode()).digest()
            except Exception as exc:
                # Only the exception class is retained: exception text may contain data.
                error = type(exc).__name__
            finally:
                conn.close()
                elapsed = (time.monotonic() - started) * 1000
                with self.lock:
                    self.active -= 1
                    self.completed += 1
                    self.latencies.append(elapsed)
                    if digest is not None:
                        if digest in self.hashes:
                            error = "duplicate_proof"
                        self.hashes.add(digest)
                    if error:
                        self.errors[error] += 1
                if error:
                    self.abort("proof_error")
                elif elapsed >= 4000:
                    self.abort("proof_latency_headroom")
            if self.burst:
                break

    def run(self):
        problem = self.check_resources()
        if problem:
            self.abort(problem)
            return self.result()
        self.started = time.monotonic()
        self.deadline = self.started + self.duration
        with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
            futures = [pool.submit(self.worker) for _ in range(self.workers)]
            self.barrier.wait()
            while not all(f.done() for f in futures):
                problem = self.check_resources()
                if problem:
                    self.abort(problem)
                if time.monotonic() >= self.deadline:
                    self.stop.set()
                time.sleep(.1)
            for future in futures:
                future.result()
        return self.result()

    def result(self):
        elapsed = time.monotonic() - self.started if self.started else 0
        completed_load = self.completed == self.workers if self.burst else elapsed >= self.duration
        return {"workers": self.workers, "burst": self.burst,
                "elapsed_s": round(elapsed, 3), "completed": self.completed,
                "valid_distinct": len(self.hashes), "peak_client_in_flight": self.peak,
                "errors": dict(self.errors), "abort_reason": self.reason,
                "latency_ms": {"p50": percentile(self.latencies, .5),
                               "p95": percentile(self.latencies, .95),
                               "p99": percentile(self.latencies, .99),
                               "max": max(self.latencies, default=None)},
                "passed": self.reason is None and completed_load and self.completed > 0
                and self.completed == len(self.hashes) and not self.errors}


def run_bounded(runner):
    """Independent process watchdog, including at most 8 seconds to drain sockets."""
    finished = threading.Event()
    deadline = time.monotonic() + runner.duration + 9

    def watchdog():
        stop_deadline = None
        while not finished.wait(.05):
            if runner.stop.is_set() and stop_deadline is None:
                stop_deadline = time.monotonic() + 8
            if time.monotonic() >= min(deadline, stop_deadline or deadline):
                os.write(1, b'{"passed":false,"abort_reason":"watchdog_deadline"}\n')
                os._exit(2)

    threading.Thread(target=watchdog, daemon=True).start()
    try:
        return runner.run()
    finally:
        finished.set()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=19395)
    parser.add_argument("--token-file", required=True, type=pathlib.Path)
    parser.add_argument("--telemetry", required=True, type=pathlib.Path)
    parser.add_argument("--stop-file", required=True, type=pathlib.Path)
    parser.add_argument("--workers", required=True, type=int)
    parser.add_argument("--seconds", type=float, default=300)
    parser.add_argument("--burst", action="store_true")
    args = parser.parse_args()
    try:
        token = read_token(args.token_file)
    except (OSError, ValueError):
        parser.error("Invalid owner-only helper token file")
    result = run_bounded(Probe(args.port, token, args.workers,
                              args.seconds, args.telemetry, args.stop_file, args.burst))
    print(json.dumps(result), flush=True)
    raise SystemExit(0 if result["passed"] else 1)


if __name__ == "__main__":
    main()
