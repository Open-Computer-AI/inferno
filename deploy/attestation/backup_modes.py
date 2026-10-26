"""Independent EOF/lifetime guard restores candidate-only fault modes."""
import functools
import json
import os
import threading
import pathlib
import select
import signal
import subprocess
import sys
import tempfile
import time

from backup_helpers import owned_file


def set_modes(private, first, second):
    owned_file(private / 'fault-mode.json', 0o600)
    with tempfile.NamedTemporaryFile(mode='w', dir=private, prefix='fault-', delete=False) as stream:
        json.dump({'this-mac': first, 'other-pro': second}, stream)
        path = pathlib.Path(stream.name)
    path.replace(private / 'fault-mode.json')


def wait_modes(private, first, second):
    expected = {'this-mac': first, 'other-pro': second}
    run_id = json.loads((private / 'live-remote-root.json').read_text())['run_id']
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        path = private / 'fault-status.json'
        owned_file(path, 0o600)
        row = json.loads(path.read_text())
        if row['run_id'] == run_id and 0 <= time.time() - row['at'] < 1 and row['modes'] == expected:
            return row
        time.sleep(.05)
    raise RuntimeError('candidate proxy modes not confirmed')


def guard(private):
    # This process has its own session. Killing the caller closes its pipe;
    # the guard then resets only candidate proxy modes, never native helpers.
    stopping = False

    def stop(*_):
        nonlocal stopping
        stopping = True

    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, stop)
    print('ready', flush=True)
    deadline = time.monotonic() + 600
    try:
        while not stopping and time.monotonic() < deadline:
            if select.select([sys.stdin], [], [], .2)[0]:
                if not sys.stdin.readline():
                    break
    finally:
        set_modes(private, 'healthy', 'healthy')


def mode_guard(function):
    @functools.wraps(function)
    def wrapped(*args, **kwargs):
        private = pathlib.Path(sys.argv[1]).resolve() / 'private'
        child = subprocess.Popen([sys.executable, str(pathlib.Path(__file__).resolve()), str(private)],
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                 stderr=subprocess.DEVNULL, text=True, start_new_session=True)
        previous = {}
        done = threading.Event()
        guard_failed = threading.Event()
        try:
            if not select.select([child.stdout], [], [], 5)[0] or child.stdout.readline().strip() != 'ready':
                raise RuntimeError('candidate mode guard unavailable')
            def stop(*_):
                raise SystemExit('candidate test interrupted')
            for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
                previous[sig] = signal.signal(sig, stop)
            def monitor():
                while not done.wait(.1):
                    if child.poll() is not None:
                        guard_failed.set()
                        os.kill(os.getpid(), signal.SIGTERM)
                        return
            watcher = threading.Thread(target=monitor, daemon=True)
            watcher.start()
            result = function(*args, **kwargs)
            if guard_failed.is_set():
                raise RuntimeError('candidate reset guard exited')
            return result
        finally:
            done.set()
            try:
                child.stdin.close()
            except BrokenPipeError:
                pass
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                set_modes(private, 'healthy', 'healthy')
                child.kill()
                child.wait(timeout=3)
                raise RuntimeError('candidate reset guard did not exit')
            if child.returncode:
                set_modes(private, 'healthy', 'healthy')
                raise RuntimeError('candidate reset guard failed')
            for sig, handler in previous.items():
                signal.signal(sig, handler)
    return wrapped


if __name__ == '__main__':
    guard(pathlib.Path(sys.argv[1]))
