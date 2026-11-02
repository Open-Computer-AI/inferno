#!/usr/bin/env python3
"""Operator-side temporary test controller; never changes production capacity."""
import argparse
import base64
import json
import pathlib
import subprocess
import threading
import time
import signal
from capacity_probe import private_file

HERE = pathlib.Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", required=True, type=pathlib.Path,
                        help="Owner-only JSON with air/aws SSH argument arrays and artifact root")
    parser.add_argument("--setup-only", action="store_true")
    parser.add_argument("--interrupt-aws", action="store_true")
    args = parser.parse_args()
    if args.interrupt_aws and not args.setup_only:
        parser.error("Interruption test requires --setup-only")
    config = json.loads(private_file(args.config, 16384))
    output = pathlib.Path(config["root"])
    air_command, aws_command = config["air"], config["aws"]
    for command, host in ((air_command, "ITAdmin@100.70.23.11"), (aws_command, "ubuntu@100.90.154.71")):
        if (command[0] not in ("ssh", "/usr/bin/ssh") or command[-2:] != [host, "python3 -"]
                or "StrictHostKeyChecking=yes" not in command or "BatchMode=yes" not in command):
            raise ValueError("Unexpected SSH target or host verification policy")
        is_air = host.startswith("ITAdmin@")
        home = pathlib.Path.home()
        key = str(output / "private/air-maintenance") if is_air else str(home / ".ssh/inferno-independent-20261016")
        known = (str(home / ".ssh/inferno-pro-to-air-helper-20261018.known_hosts") if is_air else
                 str(home / "Documents/Codex/2026-10-25/athena-production-ab-v16/private/stage_known_hosts"))
        expected = {"BatchMode=yes", "StrictHostKeyChecking=yes", "ConnectTimeout=8", "UserKnownHostsFile=" + known,
                    "IdentitiesOnly=yes" if is_air else "HostKeyAlias=18.233.166.249"}
        options = command[1:-2]
        pairs = list(zip(options[::2], options[1::2]))
        if (len(options) % 2 or len(pairs) != len(expected) + 1
                or [v for k, v in pairs if k == "-i"] != [key]
                or {v for k, v in pairs if k == "-o"} != expected
                or any(k not in ("-i", "-o") for k, _ in pairs)):
            raise ValueError("SSH identity, pinning, or options differ from audited test configuration")
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: (_ for _ in ()).throw(SystemExit(130)))
    done = threading.Event()
    air_root = aws_root = None
    air = aws = None
    errors = []
    aws_runtime_root = [None]
    terminal_success = [False]
    candidate_air_pids = set()
    candidate_aws_pids = set()

    def rpc(command, source, timeout=30):
        process = subprocess.run(command, input=source, text=True, capture_output=True, timeout=timeout)
        if process.returncode:
            raise RuntimeError("Remote operation failed: " + str(process.returncode))
        return process.stdout

    def air_control(limit=None, stop=False):
        code = "import pathlib\np=pathlib.Path(" + repr(air_root) + ")\n"
        if stop:
            code += "try:(p/'stop').touch(mode=0o600)\nexcept FileNotFoundError:pass\n"
        else:
            code += "(p/'concurrency').write_text(" + repr(str(limit)) + ")\n(p/'go').touch(mode=0o600)\n"
            code += "(p/'heartbeat').touch(mode=0o600)\n"
        rpc(air_command, code)

    try:
        files = {name: base64.b64encode((HERE / name).read_bytes()).decode() for name in
                 ("capacity_probe.py", "capacity_aws.py")}
        setup = ("import tempfile,pathlib,base64,json\n"
                 "p=pathlib.Path(tempfile.mkdtemp(prefix='inferno-capacity-scripts-20261026-'));p.chmod(0o700)\n"
                 "files=" + repr(files) + "\n"
                 "for name,data in files.items():(p/name).write_bytes(base64.b64decode(data))\n"
                 "print(json.dumps({'root':str(p)}))\n")
        aws_root = json.loads(rpc(aws_command, setup))["root"]
        air = subprocess.Popen(air_command, text=True, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=(output / "air-supervisor.stderr").open("w"))
        air.stdin.write((HERE / "capacity_air.py").read_text())
        air.stdin.close()
        setup = json.loads(air.stdout.readline())
        if setup.get("private_setup") is not True:
            raise RuntimeError("Unexpected Air setup")
        air_root = setup["root"]
        aws = subprocess.Popen(aws_command[:-1] + ["python3 " + aws_root + "/capacity_aws.py"], text=True,
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=(output / "aws-supervisor.stderr").open("w"))
        aws.stdin.write(json.dumps({"token": setup.pop("token"), "public_key": setup["public_key"],
                                   "setup_only": args.setup_only}) + "\n")
        aws.stdin.flush()
        setup.clear()

        def aws_reader():
            try:
                with (output / "aws-load-observations.jsonl").open("w") as stream:
                    for line in aws.stdout:
                        row = json.loads(line)
                        if row.get("aws_ready"):
                            aws_runtime_root[0] = row["runtime_root"]
                        if row.get("all_stages_passed") is True or row.get("setup_only_complete") is True:
                            terminal_success[0] = True
                        stream.write(line)
                        stream.flush()
                        if row.get("aws_ready") and args.interrupt_aws:
                            rpc(aws_command, "import os,signal\nos.kill(" + str(int(row["supervisor_pid"])) + ",signal.SIGTERM)\n")
                        if "need_helper_limit" in row:
                            candidate_aws_pids.add(row["candidate_queue_pid"])
                            air_control(row["need_helper_limit"])
                        if row.get("all_stages_passed") is False:
                            errors.append("CandidateAcceptanceFailed")
                            done.set()
                            air_control(stop=True)
                        if row.get("setup_only_complete"):
                            done.set()
                            air_control(stop=True)
                        if "production_monitor" not in row:
                            print(json.dumps(row), flush=True)
                        if row.get("aws_candidate_cleaned"):
                            if not terminal_success[0]:
                                errors.append("NoSuccessfulTestResult")
                            done.set()
                            air_control(stop=True)
                if not done.is_set():
                    raise RuntimeError("AWS supervisor exited early")
            except Exception as exc:
                errors.append(type(exc).__name__)
                done.set()
                air_control(stop=True)

        threading.Thread(target=aws_reader, daemon=True).start()
        def deadline():
            if not done.wait(1400):
                errors.append("ControllerDeadline")
                done.set()
                air_control(stop=True)
                aws.stdin.close()
        threading.Thread(target=deadline, daemon=True).start()
        with (output / "air-load-observations.jsonl").open("w") as stream:
            for line in air.stdout:
                row = json.loads(line)
                candidate_air_pids.update(x["pid"] for x in row["candidate_tree"])
                candidate_air_pids.add(row["candidate_tunnel_pid"])
                stream.write(line)
                stream.flush()
                if not done.is_set():
                    rpc(air_command, "import pathlib\n(pathlib.Path(" + repr(air_root) + ")/'heartbeat').touch(mode=0o600)\n", timeout=6)
                    aws.stdin.write(line)
                    aws.stdin.flush()
                if row["sequence"] % 30 == 0:
                    print(json.dumps({"air_progress": row}), flush=True)
        if not done.is_set():
            errors.append("AirSupervisorExitedEarly")
    finally:
        done.set()
        if air_root:
            try:
                air_control(stop=True)
            except Exception:
                pass  # Air has its own bounded lifetime and finally cleanup
        if aws and aws.poll() is None:
            try:
                aws.stdin.close()
                aws.wait(timeout=15)
            except (OSError, subprocess.TimeoutExpired):
                aws.terminate()
        if air and air.poll() is None:
            try:
                air.wait(timeout=15)
            except subprocess.TimeoutExpired:
                air.terminate()
        if aws_root:
            try:
                rpc(aws_command, "import shutil\nshutil.rmtree(" + repr(aws_root) + ",ignore_errors=True)\n")
            except Exception:
                errors.append("RemoteScriptCleanupFailed")
        for command, ports, temporary_root, name in (
                (air_command, [19385], air_root, "air"),
                (aws_command, [19385, 19395], aws_runtime_root[0], "aws")):
            code = ("import socket,pathlib,json,time\nports=" + repr(ports) + "\n"
                    "def closed(port):\n s=socket.socket();s.settimeout(.2);v=s.connect_ex(('127.0.0.1',port))!=0;s.close();return v\n"
                    "for _ in range(20):\n"
                    " if all(closed(p)for p in ports):break\n time.sleep(.2)\n"
                    "result={'ports_closed':all(closed(p)for p in ports)}\n"
                    + ("result['temp_removed']=not pathlib.Path(" + repr(temporary_root) + ").exists()\n" if temporary_root else "")
                    + ("result['candidate_key_absent']='inferno-capacity-20261026' not in (pathlib.Path.home()/'.ssh/authorized_keys').read_text()\n" if name == "aws" else "")
                    + ("import os\nalive=[]\nfor pid in " + repr(sorted(candidate_air_pids if name == "air" else candidate_aws_pids)) + ":\n"
                       " try:os.kill(pid,0);alive.append(pid)\n except ProcessLookupError:pass\n"
                       "result['candidate_processes_gone']=not alive\n")
                    + "print(json.dumps(result))\n")
            try:
                check = json.loads(rpc(command, code))
                (output / (name + "-last-cleanup-check.json")).write_text(json.dumps(check, indent=2))
                print(json.dumps({name + "_cleanup_verified": check}), flush=True)
                if not all(check.values()):
                    errors.append(name + "CleanupVerificationFailed")
            except Exception:
                errors.append(name + "CleanupVerificationUnavailable")
    print(json.dumps({"controller_done": True, "errors": errors}), flush=True)
    if errors:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
