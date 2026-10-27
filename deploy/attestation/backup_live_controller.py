#!/usr/bin/env python3
"""Bounded isolated Live actor. Production routing and Docker objects are never changed."""
import json
import os
import pathlib
import re
import shlex
import signal
import stat
import subprocess
import sys
import threading
import time

from backup_helpers import OTHER, owned_file, ssh_prefix, stop_group, spawn_tunnel, heartbeat_tunnel


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    info = private.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        raise ValueError('private directory ownership/mode')
    for name in ('live-remote-root.json', 'live-manifest.json', 'test-account-access.json', 'live-test-key', 'router-token'):
        owned_file(private / name, 0o600)
    state = json.loads((private / 'live-remote-root.json').read_text())
    manifest = json.loads((private / 'live-manifest.json').read_text())
    remote, run_id = state['root'], state['run_id']
    if manifest['root'] != remote or manifest['run_id'] != run_id or not re.fullmatch('[a-f0-9]{12}', run_id):
        raise ValueError('isolated root/run identity mismatch')
    account = json.loads((private / 'test-account-access.json').read_text())
    config = {'account': account, 'test_key': (private / 'live-test-key').read_text().strip(),
              'frontend_token': (private / 'router-token').read_text().strip()}
    source = pathlib.Path(__file__).with_name('backup_docker_host.py').read_text()
    prefix = ssh_prefix(private)
    stopping = threading.Event()
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, lambda *_: stopping.set())
    child = tunnel = None
    receipts = []
    reader_thread = None

    def reader():
        for line in child.stdout:
            try:
                row = json.loads(line)
            except ValueError:
                continue
            receipts.append(row)
            path = root / ('live-host-receipts-' + run_id + '.json')
            path.write_text(json.dumps(receipts, indent=2))
            path.chmod(0o600)
            print(json.dumps(row), flush=True)

    try:
        tunnel = spawn_tunnel(prefix + ['-N', '-o', 'ExitOnForwardFailure=yes', '-L',
                                       '127.0.0.1:19480:127.0.0.1:19480', '-R',
                                       '127.0.0.1:19495:127.0.0.1:19495', OTHER])
        command = 'python3 -u -c ' + shlex.quote(source) + ' ' + shlex.quote(remote) + ' ' + run_id
        child = subprocess.Popen(prefix + [OTHER, command], stdin=subprocess.PIPE,
                                 stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                 text=True, start_new_session=True)
        metadata = {'controller_pid': os.getpid(), 'remote_actor_ssh_pid': child.pid,
                    'tunnel_supervisor_pid': tunnel.pid, 'tunnel_ssh_pid': tunnel.tunnel_state['ssh_pid'],
                    'run_id': run_id, 'remote_root': remote}
        path = private / 'live-processes.json'
        path.write_text(json.dumps(metadata))
        path.chmod(0o600)
        reader_thread = threading.Thread(target=reader, daemon=True)
        reader_thread.start()
        child.stdin.write(json.dumps(config) + '\n')
        child.stdin.flush()
        started = time.monotonic()
        while time.monotonic() - started < 1750 and not stopping.is_set() and not (private / 'stop-live').exists():
            if child.poll() is not None:
                raise RuntimeError('isolated live host exited; inspect sanitized receipts')
            if tunnel.poll() is not None:
                raise RuntimeError('candidate gateway tunnel exited')
            heartbeat_tunnel(tunnel)
            child.stdin.write('start\n' if (private / 'start-live-gateway').exists() else 'heartbeat\n')
            child.stdin.flush()
            time.sleep(2)
    finally:
        errors = []
        if child:
            try:
                child.stdin.close()
                child.wait(timeout=65)
            except (OSError, subprocess.TimeoutExpired) as exc:
                errors.append(type(exc).__name__)
                try:
                    stop_group(child)
                except (OSError, subprocess.TimeoutExpired) as failure:
                    errors.append(type(failure).__name__)
        if reader_thread:
            reader_thread.join(timeout=3)
        if tunnel:
            try:
                stop_group(tunnel)
            except (OSError, subprocess.TimeoutExpired) as failure:
                errors.append(type(failure).__name__)
        print(json.dumps({'live_controller_stopped': True, 'errors': errors,
                          'verified_host_cleanup': any(x.get('candidate_live_cleaned') for x in receipts)}), flush=True)


if __name__ == '__main__':
    main()
