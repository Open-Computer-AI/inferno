#!/usr/bin/env python3
"""Stop all candidate actors, verify independently, then remove phase credentials."""
import json
import os
import pathlib
import socket
import stat
import subprocess
import sys
import time

from backup_helpers import owned_file


def process_gone(pid):
    try:
        os.kill(pid, 0)
        return False
    except ProcessLookupError:
        return True
    except PermissionError:
        return False


def stop_marker(path):
    flags = os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW
    fd = os.open(path, flags, 0o600)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError('invalid candidate stop marker')
        os.fchmod(fd, 0o600)
    finally:
        os.close(fd)


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    info = private.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        raise ValueError('invalid candidate private directory')
    owned_file(private / 'live-manifest.json', 0o600)
    manifest = json.loads((private / 'live-manifest.json').read_text())
    source = pathlib.Path(__file__).parent
    for name in ('stop-live', 'stop-helpers', 'stop-router', 'stop-fault-proxy', 'stop-air-reference', 'stop-aws-latency'):
        stop_marker(private / name)
    evidence = {}
    programs = ['backup_live_verify.py', 'backup_cleanup.py']
    if (private / 'aws-latency-active.json').exists():
        programs.append('backup_aws_verify.py')
    for program in programs:
        if program == 'backup_cleanup.py' and not (private / 'active-helper-receipt-path').exists():
            evidence['helper_cleanup_not_needed'] = not (private / 'backend-tokens.json').exists()
            continue
        result = subprocess.run([sys.executable, str(source / program), str(root)],
                                capture_output=True, text=True, timeout=90)
        evidence[program] = result.returncode == 0
        # The verifiers output sanitized metadata only.
        if result.stdout:
            print(result.stdout.strip(), flush=True)
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        gone = True
        expected = {'router': 'router-process.json', 'fault-proxy': 'fault-status.json'}
        for role in manifest.get('actor_roles', []):
            if role not in expected:
                continue
            filename = expected[role]
            path = private / filename
            if not path.exists():
                gone = False
                continue
            owned_file(path, 0o600)
            process = json.loads(path.read_text())
            if process.get('run_id') != manifest['run_id'] or not process.get('started'):
                gone = False
                continue
            if not process_gone(process['pid']):
                start = subprocess.check_output(['ps', '-p', str(process['pid']), '-o', 'lstart='], text=True).strip()
                gone &= start != process['started']
        for port in (19480, 19485, 19486, 19487, 19488, 19489, 19490, 19495):
            with socket.socket() as sock:
                sock.settimeout(.2)
                gone &= sock.connect_ex(('127.0.0.1', port)) != 0
        if gone:
            break
        time.sleep(.2)
    evidence['candidate_processes_and_ports_gone'] = gone
    air_records = list(root.glob('air-reference-process-*.json'))
    air_gone = True
    groups = subprocess.check_output(['ps', '-axo', 'pgid='], text=True).split()
    for path in air_records:
        owned_file(path, 0o600)
        record = json.loads(path.read_text())
        for field, start_field in (('controller_pid', 'controller_started'),
                                   ('tunnel_supervisor_pid', 'supervisor_started'),
                                   ('ssh_pid', 'ssh_started')):
            pid = record[field]
            if not process_gone(pid):
                started = subprocess.check_output(['ps', '-p', str(pid), '-o', 'lstart='], text=True).strip()
                air_gone &= started != record[start_field]
            if field != 'controller_pid':
                air_gone &= str(pid) not in groups
    if 'air-reference' in manifest.get('actor_roles', []):
        air_gone &= bool(air_records)
    evidence['air_reference_actors_gone'] = air_gone
    secrets = ('router-token', 'live-test-key', 'test-account-access.json',
               'live-gateway-config.yaml', 'live-postgres.env', 'backend-tokens.json',
               'air-reference-token', 'pool-backend-tokens.json')
    if all(evidence.values()):
        for name in secrets:
            path = private / name
            if path.exists():
                owned_file(path, 0o600)
                path.unlink()
        for name in ('live-processes.json', 'router-process.json', 'fault-status.json', 'air-reference-process.json',
                     'start-live-gateway', 'stop-live', 'stop-helpers', 'stop-router', 'stop-fault-proxy', 'stop-air-reference', 'stop-aws-latency'):
            (private / name).unlink(missing_ok=True)
        evidence['phase_credentials_removed'] = not any((private / name).exists() for name in secrets)
    else:
        evidence['phase_credentials_removed'] = False
    target = root / ('phase-cleanup-' + str(time.time_ns()) + '.json')
    target.write_text(json.dumps(evidence, indent=2))
    target.chmod(0o600)
    print(json.dumps(evidence))
    if not all(evidence.values()):
        raise SystemExit(1)


if __name__ == '__main__':
    main()
