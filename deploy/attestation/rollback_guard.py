#!/usr/bin/env python3
"""Restore one attestation tunnel LaunchAgent unless a cutover is confirmed."""

import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys
import tempfile
import time


LABELS = {
    "com.opencomputer.inferno-attestation-aws-tunnel",
    "com.opencomputer.inferno-attestation-aws-tunnel-candidate",
}


def claim_decision(state, decision):
    """Confirmation and timeout compete for one atomic decision file."""
    try:
        fd = os.open(state / "decision.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        return False
    with os.fdopen(fd, "w") as handle:
        json.dump({"decision": decision}, handle)
        handle.flush()
        os.fsync(handle.fileno())
    return True


def atomic_write(path, data, mode=0o600):
    fd, temporary = tempfile.mkstemp(prefix=".attestation-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as handle:
            os.fchmod(handle.fileno(), mode)
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def bootstrap_agent(target, label):
    """Allow launchd's short unload delay, verifying any already loaded job."""
    command = ["launchctl", "bootstrap", "gui/{}".format(os.getuid()), str(target)]
    expected = plistlib.loads(target.read_bytes()).get("ProgramArguments")
    for attempt in range(20):
        result = subprocess.run(command, capture_output=True)
        if result.returncode == 0:
            return
        if result.returncode != 5 or attempt == 19:
            raise subprocess.CalledProcessError(result.returncode, command)
        loaded = subprocess.run(
            ["launchctl", "print", "gui/{}/{}".format(os.getuid(), label)],
            capture_output=True,
        )
        if loaded.returncode == 0:
            match = re.search(rb"arguments = \{\n(.*?)\n\s*\}", loaded.stdout, re.S)
            actual = [line.strip().decode() for line in match[1].splitlines()] if match else None
            if expected is not None and actual == expected:
                return
            raise RuntimeError("An unexpected version of the LaunchAgent is loaded")
        time.sleep(0.1)


def restore(state):
    home = Path.home()
    runtime = home / "Library/Application Support/InfernoAttestationAWS"
    state.resolve().relative_to(runtime.resolve())
    config = json.loads((state / "config.json").read_text())
    label = config["label"]
    if label not in LABELS:
        raise ValueError("Refusing to restore an unrelated LaunchAgent")
    target = home / "Library/LaunchAgents" / (label + ".plist")
    backup = (state / "backup.plist").read_bytes()
    if plistlib.loads(backup).get("Label") != label:
        raise ValueError("Backup label does not match")
    if hashlib.sha256(backup).hexdigest() != config["backup_sha256"]:
        raise ValueError("Backup changed after preparation")
    if hashlib.sha256(target.read_bytes()).hexdigest() != config["candidate_sha256"]:
        raise ValueError("Target changed outside this cutover; refusing to overwrite")
    domain = "gui/{}/{}".format(os.getuid(), label)
    stopped = subprocess.run(["launchctl", "bootout", domain], capture_output=True)
    if stopped.returncode:
        present = subprocess.run(["launchctl", "print", domain], capture_output=True)
        if present.returncode == 0:
            raise RuntimeError("LaunchAgent could not be unloaded")
    atomic_write(target, backup, (state / "backup.plist").stat().st_mode & 0o777)
    bootstrap_agent(target, label)
    return {"outcome": "rolled_back", "label": label}


def guard(state):
    config = json.loads((state / "config.json").read_text())
    delay = float(config["delay_seconds"])
    if not 0 <= delay <= 600:
        raise ValueError("Rollback delay must be between zero and 600 seconds")
    deadline = time.monotonic() + delay
    while time.monotonic() < deadline:
        if (state / "decision.json").exists():
            return {"outcome": "decision_already_claimed"}
        time.sleep(min(0.2, max(0, deadline - time.monotonic())))
    if not claim_decision(state, "rollback"):
        return {"outcome": "decision_already_claimed"}
    return restore(state)


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in {"guard", "confirm", "restore"}:
        raise SystemExit("Usage: rollback_guard.py guard|confirm|restore STATE_DIRECTORY")
    action, state = sys.argv[1], Path(sys.argv[2])
    if action == "confirm":
        confirmed = claim_decision(state, "confirm")
        print(json.dumps({"confirmed": confirmed}))
        return 0 if confirmed else 1
    try:
        result = guard(state) if action == "guard" else restore(state)
    except Exception as error:
        result = {"outcome": "failed", "error_class": type(error).__name__}
        atomic_write(state / "result.json", json.dumps(result).encode())
        print(json.dumps(result))
        return 1
    atomic_write(state / "result.json", json.dumps(result).encode())
    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    sys.exit(main())
