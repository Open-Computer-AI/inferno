#!/usr/bin/env python3
"""Concurrent host/owned-process telemetry while the direct16 soak runs."""
import json
import pathlib
import os
import stat
import signal
import subprocess
import sys
import time

from backup_phase_cleanup import stop_marker
from backup_soak import resource


def cpu_seconds(value):
    result = 0.0
    for part in value.split(':'):
        result = result * 60 + float(part)
    return result


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    marker = private / 'stop-soak'
    if marker.exists():raise RuntimeError('stale independent soak stop marker')
    report = root / 'soak-direct-other-pro.json'
    if report.exists():
        info = report.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():raise ValueError('unexpected prior soak report')
        report.rename(root / ('soak-direct-other-pro-before-' + str(time.time_ns()) + '.json'))
    samples, child, reason = [], None, None
    stopping = False

    def stop(*_):
        nonlocal stopping
        stopping = True

    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):signal.signal(sig, stop)
    began = time.monotonic()
    try:
        while time.monotonic() - began < 420 and not stopping:
            sample = resource(private, 'other-pro')
            samples.append(sample)
            if (sample['pressure'] != 1 or sample['cpu_busy_percent'] > 85
                    or sample['swapouts'] != samples[0]['swapouts']):
                reason = 'concurrent_resource_gate'
                stop_marker(marker)
                break
            if child is None:
                child = subprocess.Popen([sys.executable, str(pathlib.Path(__file__).with_name('backup_soak.py')),
                                          str(root), 'other-pro', '--direct'], start_new_session=True)
            if child.poll() is not None:break
            time.sleep(1)
        else:
            reason = 'monitor_lifetime_or_signal'
            stop_marker(marker)
    finally:
        if child and child.poll() is None:
            stop_marker(marker)
            try:child.wait(timeout=20)
            except subprocess.TimeoutExpired:
                child.terminate()
                child.wait(timeout=20)
        intervals = []
        for before, after in zip(samples, samples[1:]):
            old = {x['pid']:cpu_seconds(x['cpu_time']) for x in before['processes']}
            new = {x['pid']:cpu_seconds(x['cpu_time']) for x in after['processes']}
            seconds = after['at'] - before['at']
            if old.keys() == new.keys() and seconds > 0 and all(new[k] >= old[k] for k in old):
                intervals.append(round(sum(new[k] - old[k] for k in old) / seconds * 100, 3))
        report = root / 'soak-direct-other-pro.json'
        result = json.loads(report.read_text()) if report.exists() else {}
        evidence = {'passed':bool(child and child.returncode == 0 and result.get('passed') and not reason),
                    'reason':reason, 'elapsed_s':round(time.monotonic()-began,3), 'samples':samples,
                    'helper_group_interval_cpu_percent_one_core':intervals,
                    'scope':'concurrent whole-host top samples and cumulative CPU deltas for matching extant helper-group PIDs; exited child CPU excluded'}
        (root / 'soak-direct-concurrent-resources.json').write_text(json.dumps(evidence, indent=2))
        print(json.dumps({k:v for k,v in evidence.items() if k!='samples'}),flush=True)
    if not evidence['passed']:raise SystemExit(1)


if __name__ == '__main__':
    main()
