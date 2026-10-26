#!/usr/bin/env python3
"""Temporary AWS candidate supervisor. First stdin row contains private setup.

Subsequent stdin rows are Air telemetry. Stdout contains sanitized observations only.
"""
import ast
import atexit
import datetime
import hashlib
import http.client
import json
import os
import pathlib
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time

HERE = pathlib.Path(__file__).resolve().parent


def main():
    config = json.loads(sys.stdin.readline())
    root = pathlib.Path(tempfile.mkdtemp(prefix="inferno-capacity-20261026-"))
    atexit.register(shutil.rmtree, root, ignore_errors=True)
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: (_ for _ in ()).throw(SystemExit(130)))
    root.chmod(0o700)
    token = root / "token"
    token.write_text(config["token"])
    token.chmod(0o600)
    telemetry = root / "telemetry.json"
    stop_file = root / "stop"
    marker = "inferno-capacity-20261026"
    auth = pathlib.Path.home() / ".ssh/authorized_keys"
    expiry = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(minutes=30)).strftime("%Y%m%d%H%M%SZ")
    line = ('command="/bin/false",restrict,port-forwarding,from="100.70.23.11",'
            'expiry-time="' + expiry + '",permitopen="127.0.0.1:1",'
            'permitlisten="127.0.0.1:19385" ' + " ".join(config["public_key"].split()[:2]) + " " + marker)
    if marker in auth.read_text():
        raise RuntimeError("Candidate key already exists")
    for port in (19385, 19395):
        with socket.socket() as check:
            check.bind(("127.0.0.1", port))
    source = pathlib.Path("/opt/inferno-queue/attest_queue.py").read_bytes()
    if hashlib.sha256(source).hexdigest() != "9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d":
        raise RuntimeError("Queue source drift")
    queue_source = root / "queue.py"
    queue_source.write_bytes(source)
    gateway = json.loads(subprocess.check_output(["sudo", "docker", "inspect", "inferno-validation-gateway"]))[0]
    env = dict(x.split("=", 1) for x in gateway["Config"]["Env"] if "=" in x)
    production_token = subprocess.check_output(["sudo", "docker", "exec", "inferno-validation-gateway",
                                               "cat", env["LIVE_ATTESTATION_TOKEN_FILE"]], text=True).strip()
    latest = {}
    production = {"ok": False, "observed_at": 0}
    done = threading.Event()
    output_lock = threading.Lock()
    queue = None
    queue_transition = True
    started = time.monotonic()
    cleanup_lock = threading.Lock()

    def terminate(child):
        if child and child.poll() is None:
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                return
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait(timeout=2)
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    def cleanup():
        failures = []
        with cleanup_lock:
            try:
                terminate(queue)
            except Exception as exc:
                failures.append("queue_" + type(exc).__name__)
            try:
                lines = auth.read_text().splitlines()
                auth.write_text("\n".join(x for x in lines if x.strip() != line) + "\n")
            except Exception as exc:
                failures.append("key_" + type(exc).__name__)
            try:
                if root.exists():
                    shutil.rmtree(root)
            except Exception as exc:
                failures.append("files_" + type(exc).__name__)
        return failures
    atexit.register(cleanup)

    def emit(value):
        with output_lock:
            print(json.dumps(value), flush=True)

    def stop(reason):
        stop_file.touch(mode=0o600)
        emit({"stop_reason": reason})

    def request(port, path, method="GET", bearer=None):
        connection = http.client.HTTPConnection("127.0.0.1", port, timeout=8)
        try:
            connection.request(method, path, body=b"" if method == "POST" else None,
                               headers={"Authorization": "Bearer " + bearer} if bearer else {})
            response = connection.getresponse()
            return response.status, response.read(20481)
        finally:
            connection.close()

    def stats(port):
        status, body = request(port, "/queue-health")
        if status != 200:
            raise RuntimeError("Queue health failed")
        return ast.literal_eval(body.decode())

    baseline_stats = stats(19095)

    def ingest():
        try:
            for text in sys.stdin:
                row = json.loads(text)
                latest.clear()
                latest.update(row)
                sample = dict(row, production_ok=production["ok"] and time.time() - production["observed_at"] < 5)
                temp = root / "telemetry.tmp"
                temp.write_text(json.dumps(sample))
                temp.chmod(0o600)
                os.replace(temp, telemetry)
            if not done.is_set():
                stop("air_stream_closed")
        except Exception:
            stop("air_stream_invalid")

    def production_monitor():
        from capacity_probe import valid_proof
        previous_cpu = None
        last_proof = 0
        while not done.is_set():
            try:
                tick = time.monotonic()
                status, body = request(19095, "/health", bearer=production_token)
                health_ms = (time.monotonic() - tick) * 1000
                health_ok = status == 200 and json.loads(body).get("supported") is True and health_ms < 2000
                proof_ok = True
                proof_ms = None
                if tick - last_proof > 10:
                    proof_started = time.monotonic()
                    proof_status, body = request(19095, "/v1/attestation", "POST", production_token)
                    proof_ok = proof_status == 200 and valid_proof(json.loads(body))
                    proof_ms = round((time.monotonic() - proof_started) * 1000, 1)
                    proof_ok = proof_ok and proof_ms < 2000
                    last_proof = tick
                current_stats = stats(19095)
                production["ok"] = health_ok and proof_ok and current_stats["failed"] == baseline_stats["failed"]
                production["observed_at"] = time.time()
                cpu = [int(x) for x in pathlib.Path("/proc/stat").read_text().splitlines()[0].split()[1:9]]
                busy = None
                if previous_cpu:
                    delta = [b - a for a, b in zip(previous_cpu, cpu)]
                    busy = round(100 * (1 - (delta[3] + delta[4]) / max(1, sum(delta))), 2)
                previous_cpu = cpu
                queue_metrics = {}
                try:
                    if queue and queue.poll() is None:
                        parts = pathlib.Path(f"/proc/{queue.pid}/stat").read_text().rsplit(")", 1)[1].split()
                        queue_metrics = {"pid": queue.pid, "cpu_ticks": int(parts[11]) + int(parts[12]),
                                         "rss_bytes": int(parts[21]) * os.sysconf("SC_PAGE_SIZE")}
                        queue_metrics["stats"] = stats(19395)
                except (OSError, http.client.HTTPException):
                    if not queue_transition:
                        raise
                emit({"production_monitor": True, "observed_at": time.time(), "healthy": production["ok"],
                      "health_ms": round(health_ms, 1), "proof_ms": proof_ms,
                      "production_stats": current_stats, "aws_cpu_busy_percent": busy, "candidate_queue": queue_metrics})
                if not production["ok"] or (busy is not None and busy > 80):
                    stop("production_or_aws_headroom")
            except Exception as exc:
                stop("production_monitor_" + type(exc).__name__)
            done.wait(1)

    def watchdog():
        stop_at = None
        while not done.wait(.1):
            if time.monotonic() - started > 1350:
                stop("global_deadline")
            if stop_file.exists() and stop_at is None:
                stop_at = time.monotonic()
            if stop_at is not None and time.monotonic() - stop_at > 10:
                cleanup()
                os._exit(2)

    with auth.open("a") as stream:
        stream.write("\n" + line + "\n")
    threading.Thread(target=ingest, daemon=True).start()
    threading.Thread(target=production_monitor, daemon=True).start()
    threading.Thread(target=watchdog, daemon=True).start()
    emit({"aws_ready": True, "supervisor_pid": os.getpid(), "runtime_root": str(root)})
    results = []
    passed = False
    baseline_p95 = None
    try:
        for limit in (16, 32, 33):
            queue_transition = True
            if queue:
                terminate(queue)
            queue = subprocess.Popen([sys.executable, str(queue_source)], env=dict(os.environ,
                QUEUE_PORT="19395", QUEUE_UPSTREAM_PORT="19385", QUEUE_CONCURRENCY=str(limit), QUEUE_KEEPALIVE="0"),
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
            emit({"need_helper_limit": limit, "candidate_queue_pid": queue.pid})
            ready_deadline = time.monotonic() + 40
            while True:
                if stop_file.exists():
                    raise RuntimeError("Stopped by monitor")
                if time.monotonic() > ready_deadline:
                    raise RuntimeError("Candidate readiness timeout")
                try:
                    if latest.get("candidate_limit") == limit and production["ok"]:
                        status, body = request(19395, "/health", bearer=config["token"])
                        from capacity_probe import resource_problem, private_file
                        current = json.loads(private_file(telemetry, 16384))
                        if (status == 200 and json.loads(body).get("supported") is True
                                and resource_problem(current, time.time()) is None
                                and current.get("candidate_limit") == limit):
                            break
                except (OSError, http.client.HTTPException):
                    pass
                time.sleep(.2)
            queue_transition = False
            if config.get("setup_only"):
                emit({"setup_only_complete": True})
                passed = True
                break
            for burst in (True, False):
                if stop_file.exists():
                    raise RuntimeError("Stopped by monitor")
                before = stats(19395)
                args = [sys.executable, str(HERE / "capacity_probe.py"), "--token-file", str(token),
                        "--telemetry", str(telemetry), "--stop-file", str(stop_file),
                        "--workers", str(limit), "--seconds", "300"] + (["--burst"] if burst else [])
                emit({"stage_started": True, "limit": limit, "burst": burst})
                process = subprocess.run(args, text=True, capture_output=True, timeout=312)
                result = json.loads(process.stdout.strip().splitlines()[-1])
                result["queue_before"] = before
                result["queue_after"] = stats(19395)
                result["queue_failed_delta"] = result["queue_after"]["failed"] - before["failed"]
                result["queue_busy_delta"] = result["queue_after"]["busy_retries"] - before["busy_retries"]
                results.append(result)
                emit({"stage_result": result})
                if (process.returncode or not result["passed"] or result["queue_failed_delta"]
                        or result["queue_busy_delta"]):
                    raise RuntimeError("Candidate acceptance failed")
                if not burst:
                    if limit == 16:
                        baseline_p95 = result["latency_ms"]["p95"]
                    elif result["latency_ms"]["p95"] > max(2000, 2 * baseline_p95):
                        raise RuntimeError("Candidate latency regressed against baseline")
        if not config.get("setup_only"):
            emit({"all_stages_passed": True, "results": results})
            passed = True
    except Exception as exc:
        emit({"all_stages_passed": False, "error_class": type(exc).__name__, "results": results})
    finally:
        done.set()
        cleanup_errors = cleanup()
        if cleanup_errors:
            passed = False
        emit({"aws_candidate_cleaned": not cleanup_errors, "cleanup_errors": cleanup_errors})
    if not passed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
