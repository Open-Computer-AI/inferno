#!/usr/bin/env python3
"""Temporary isolated helper and Air telemetry; credential row is controller-only."""
import hashlib
import atexit
import json
import os
import pathlib
import re
import secrets
import shutil
import signal
import socket
import subprocess
import tempfile
import time


def command(args):
    return subprocess.check_output(args, text=True, timeout=10).strip()


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


def main():
    root = pathlib.Path(tempfile.mkdtemp(prefix="inferno-capacity-20261026-"))
    atexit.register(shutil.rmtree, root, ignore_errors=True)
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: (_ for _ in ()).throw(SystemExit(130)))
    root.chmod(0o700)
    helper = tunnel = None
    atexit.register(lambda: [terminate(child) for child in (helper, tunnel)])
    deadline = time.monotonic() + 1500
    setup_deadline = time.monotonic() + 45
    token = secrets.token_urlsafe(48)
    token_file = root / "token"
    token_file.write_text(token)
    token_file.chmod(0o600)
    binary = pathlib.Path.home() / "inferno-attest-v2/attestation-helper-v2"
    if hashlib.sha256(binary.read_bytes()).hexdigest() != "c030e757e0384e0b7ce1fedd13198be100cdee9dacd2f250a0186e5a79515c6e":
        raise RuntimeError("Production helper binary drift")
    key = root / "tunnel-key"
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(key)], check=True)
    (root / "heartbeat").touch(mode=0o600)
    print(json.dumps({"private_setup": True, "root": str(root), "supervisor_pid": os.getpid(), "token": token,
                      "public_key": key.with_suffix(".pub").read_text().strip()}), flush=True)
    try:
        while not (root / "go").exists():
            if (root / "stop").exists():
                return
            if time.monotonic() > setup_deadline or time.time() - (root / "heartbeat").stat().st_mtime > 8:
                raise RuntimeError("Setup deadline exceeded")
            time.sleep(.1)
        with socket.socket() as check:
            check.bind(("127.0.0.1", 19385))
        ssh = pathlib.Path.home() / ".ssh"
        tunnel = subprocess.Popen(["/usr/bin/ssh", "-NT", "-i", str(key), "-o", "IdentitiesOnly=yes",
            "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ExitOnForwardFailure=yes",
            "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ConnectTimeout=10",
            "-o", "UserKnownHostsFile=" + str(ssh / "inferno-attestation-aws-20261018.known_hosts"),
            "-o", "HostKeyAlias=18.233.166.249", "-R", "127.0.0.1:19385:127.0.0.1:19385",
            "ubuntu@100.90.154.71"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        concurrency = None
        sequence = 0
        first_swapouts = None
        while time.monotonic() < deadline and not (root / "stop").exists():
            if time.time() - (root / "heartbeat").stat().st_mtime > 8:
                raise RuntimeError("Controller heartbeat lost")
            requested = int((root / "concurrency").read_text()) if (root / "concurrency").exists() else 16
            if requested not in (16, 32, 33):
                raise RuntimeError("Invalid candidate capacity")
            if requested != concurrency:
                if helper:
                    terminate(helper)
                helper = subprocess.Popen([str(binary), "-listen", "127.0.0.1:19385", "-token-file",
                    str(token_file), "-max-concurrent", str(requested)], stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL, start_new_session=True)
                concurrency = requested
            if helper.poll() is not None or tunnel.poll() is not None:
                raise RuntimeError("Candidate process exited")
            top = command(["top", "-l", "2", "-s", "1", "-n", "600", "-stats", "pid,ppid,cpu,mem"])
            idle = re.findall(r"([\d.]+)% idle", top)
            if not idle:
                raise RuntimeError("CPU telemetry unavailable")
            # The second top sample reports interval CPU, unlike ps's lifetime average.
            rows = {}
            for line in top.splitlines():
                match = re.match(r"\s*(\d+)\s+(\d+)\s+([\d.]+)\s+(\S+)", line)
                if match:
                    pid, ppid, cpu, mem = match.groups()
                    rows[int(pid)] = {"pid": int(pid), "ppid": int(ppid), "cpu_core_percent": float(cpu), "footprint": mem}
            def tree(pid):
                ids = {pid}
                for _ in range(5):
                    ids.update(p for p, v in rows.items() if v["ppid"] in ids)
                return [rows[p] for p in ids if p in rows]
            vm = command(["vm_stat"])
            swapouts = int(re.search(r"Swapouts:\s+(\d+)", vm)[1])
            if first_swapouts is None:
                first_swapouts = swapouts
            thermal = command(["pmset", "-g", "therm"])
            if not any(x in thermal for x in ("No thermal warning level", "CPU_Speed_Limit", "Thermal_Level")):
                raise RuntimeError("Thermal status unavailable")
            warning = any(int(x) < 100 for x in re.findall(r"CPU_Speed_Limit\s*=\s*(\d+)", thermal))
            warning |= any(int(x) > 0 for x in re.findall(r"Thermal_Level\s*=\s*(\d+)", thermal))
            production_pid = int(command(["pgrep", "-f", "^/Users/ITAdmin/inferno-attest-v2/attestation-helper-v2 -listen 127.0.0.1:19085"]))
            if production_pid != 98414:
                raise RuntimeError("Production helper identity changed")
            candidate_tree, production_tree = tree(helper.pid), tree(production_pid)
            if len(candidate_tree) < 2 or len(production_tree) < 2:
                raise RuntimeError("Helper process tree telemetry incomplete")
            rss = command(["ps", "-p", ",".join(str(x["pid"]) for x in candidate_tree + production_tree), "-o", "pid=,rss="])
            rss_by_pid = {int(a): int(b) * 1024 for a, b in (line.split() for line in rss.splitlines())}
            for row in candidate_tree + production_tree:
                row["rss_bytes"] = rss_by_pid[row["pid"]]
            sequence += 1
            print(json.dumps({"observed_at": time.time(), "sequence": sequence, "host": "100.70.23.11",
                "pressure": int(command(["sysctl", "-n", "kern.memorystatus_vm_pressure_level"])),
                "cpu_busy_percent": round(100 - float(idle[-1]), 2), "swapouts_delta": swapouts - first_swapouts,
                "thermal_warning": warning, "candidate_limit": concurrency,
                "candidate_tunnel_pid": tunnel.pid,
                "swap_usage": command(["sysctl", "vm.swapusage"]),
                "candidate_tree": candidate_tree, "production_tree": production_tree}), flush=True)
    finally:
        for child in (helper, tunnel):
            terminate(child)
        shutil.rmtree(root)


if __name__ == "__main__":
    main()
