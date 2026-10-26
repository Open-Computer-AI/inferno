#!/usr/bin/env python3
"""Temporary read-only Air reference forward; no AWS or Air service changes."""
import json
import hashlib
import base64
import os
import subprocess
import pathlib
import signal
import stat
import sys
import time

from backup_helpers import heartbeat_tunnel, owned_file, port_free, spawn_tunnel, stop_group


def aws_command(private):
    path = private / 'aws-command.json'
    owned_file(path, 0o600)
    known_hosts = '/Users/saksham/Documents/Codex/2026-10-25/athena-production-ab-v16/private/stage_known_hosts'
    expected = ['ssh', '-i', '/Users/saksham/.ssh/inferno-independent-20261016',
                '-o', 'UserKnownHostsFile=' + known_hosts, '-o', 'HostKeyAlias=18.233.166.249',
                '-o', 'StrictHostKeyChecking=yes', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=8',
                'ubuntu@100.90.154.71', 'python3 -']
    if json.loads(path.read_text()) != expected:
        raise ValueError('AWS command differs from verified pinned profile')
    owned_file(pathlib.Path('/Users/saksham/.ssh/inferno-independent-20261016'), 0o600)
    info = pathlib.Path(known_hosts).lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise ValueError('untrusted AWS host-key file')
    if hashlib.sha256(pathlib.Path(known_hosts).read_bytes()).hexdigest() != 'dd1d8e28e7e92fb0203118f15d9924166b32998f2c0e1d8912ffa1cc80401cf6':
        raise ValueError('AWS pinned host-key content changed')
    public = subprocess.check_output(['ssh-keygen', '-y', '-f', expected[2]], stdin=subprocess.DEVNULL, text=True).split()
    fingerprint = 'SHA256:' + base64.b64encode(hashlib.sha256(base64.b64decode(public[1], validate=True)).digest()).decode().rstrip('=')
    if fingerprint != 'SHA256:w1jVqP/G2ls1cNLn7wSV02ORBezoXMDE5CYFF/p8liU':
        raise ValueError('AWS public identity fingerprint changed')
    return expected


def reference_state(private, expected_open):
    import socket
    receipt = private / 'air-reference-process.json'
    owned_file(receipt, 0o600)
    row = json.loads(receipt.read_text())
    if row['run_id'] != json.loads((private / 'live-remote-root.json').read_text())['run_id']:
        raise ValueError('Air reference run identity differs')
    deadline = time.monotonic() + 5
    while True:
        groups = subprocess.check_output(['ps', '-axo', 'pgid='], text=True).split()
        states = {}
        for field, started in (('controller_pid', 'controller_started'),
                               ('tunnel_supervisor_pid', 'supervisor_started'), ('ssh_pid', 'ssh_started')):
            result = subprocess.run(['ps', '-p', str(row[field]), '-o', 'lstart='], capture_output=True, text=True)
            same = result.returncode == 0 and result.stdout.strip() == row[started]
            states[field] = same
        with socket.socket() as sock:
            sock.settimeout(.2)
            listening = sock.connect_ex(('127.0.0.1', 19487)) == 0
        groups_present = any(str(row[k]) in groups for k in ('tunnel_supervisor_pid', 'ssh_pid'))
        passed = listening and all(states.values()) and groups_present if expected_open else not listening and not any(states.values()) and not groups_present
        if passed:
            return {'expected_open': expected_open, 'passed': True, 'process_identities_present': states,
                    'port_open': listening, 'ssh_pid': row['ssh_pid'], 'run_id': row['run_id']}
        if time.monotonic() >= deadline:
            raise RuntimeError('Air reference actor/forward identity mismatch')
        time.sleep(.05)


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    command = aws_command(private)
    port_free(19487)
    command = ['ssh', '-F', '/dev/null', '-T'] + command[1:-2] + [
        '-o', 'ExitOnForwardFailure=yes', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
        '-o', 'ServerAliveInterval=5', '-o', 'ServerAliveCountMax=2',
        '-N', '-L', '127.0.0.1:19487:127.0.0.1:19085', 'ubuntu@100.90.154.71']
    stopping = False

    def stop(*_):
        nonlocal stopping
        stopping = True

    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, stop)
    child = spawn_tunnel(command)
    started = time.monotonic()
    def identity(pid):
        return subprocess.check_output(['ps', '-p', str(pid), '-o', 'lstart='], text=True).strip()
    record = {'controller_pid': os.getpid(), 'controller_started': identity(os.getpid()),
              'tunnel_supervisor_pid': child.pid, 'supervisor_started': identity(child.pid),
              'ssh_pid': child.tunnel_state['ssh_pid'], 'ssh_started': identity(child.tunnel_state['ssh_pid']),
              'run_id': json.loads((private / 'live-remote-root.json').read_text())['run_id']}
    receipt = private / 'air-reference-process.json'
    history = root / ('air-reference-process-' + str(time.time_ns()) + '.json')
    history.write_text(json.dumps(record))
    history.chmod(0o600)
    receipt.write_text(json.dumps(record))
    receipt.chmod(0o600)
    print(json.dumps({'air_reference_started': True, 'local_port': 19487, 'aws_loopback_port': 19085, **record}), flush=True)
    try:
        while not stopping and time.monotonic() - started < 600 and not (private / 'stop-air-reference').exists():
            if child.poll() is not None:
                raise RuntimeError('candidate reference forward exited')
            heartbeat_tunnel(child)
            time.sleep(1)
    finally:
        stop_group(child)
        port_free(19487)
        print(json.dumps({'air_reference_stopped': True, 'candidate_forward_closed': True}), flush=True)


if __name__ == '__main__':
    main()
