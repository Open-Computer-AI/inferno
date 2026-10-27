#!/usr/bin/env python3
"""Run one real Live scenario and verify candidate records, billing and leases."""
import json
import pathlib
import socket
import shlex
import subprocess
import sys
import tempfile
import threading
import time

from backup_helpers import OTHER, ssh_prefix
from backup_modes import mode_guard, set_modes, wait_modes
from backup_phase_cleanup import stop_marker
from backup_production_check import production_case
from backup_air_reference import reference_state
from backup_direct_control import apply_case



def snapshot(root):
    private = root / 'private'
    manifest = json.loads((private / 'live-manifest.json').read_text())
    source = pathlib.Path(__file__).with_name('backup_docker_host.py').read_text().rsplit("if __name__ == '__main__':", 1)[0]
    source += '\nhost=Host(pathlib.Path(' + repr(manifest['root']) + '),' + repr(manifest['run_id']) + ',daemon=' + repr(manifest['docker']) + ');host.pin_daemon()\n'
    source += '''
q="SELECT json_build_object('usage_rows',count(*) FILTER (WHERE request_type=5),'other_usage_rows',count(*) FILTER (WHERE request_type<>5)) FROM usage_logs"
usage=json.loads(host.docker_run(['exec',host.names['postgres'],'psql','-U','sub2api','-d','sub2api','-At','-c',q]))
keys=host.docker_run(['exec',host.names['redis'],'redis-cli','--scan','--pattern','live:call:*']).decode().splitlines()
records=[]
for key in keys:
 state=host.docker_run(['exec',host.names['redis'],'redis-cli','HGET',key,'controller']).decode().strip()
 records.append({'hash':key.rsplit(':',1)[-1],'state':state})
leases={}
for pattern in ['concurrency:account:*','concurrency:user:*','concurrency:live:account:*','concurrency:live:user:*','concurrency:live:api_key:*']:
 names=host.docker_run(['exec',host.names['redis'],'redis-cli','--scan','--pattern',pattern]).decode().splitlines()
 leases[pattern]=sum(int(host.docker_run(['exec',host.names['redis'],'redis-cli','ZCARD',key]).decode().strip()) for key in names)
print(json.dumps({'usage':usage,'records':records,'leases':leases}))
'''
    for attempt in range(3):
        result = subprocess.run(ssh_prefix(private) + [OTHER, 'python3 -c ' + shlex.quote(source)],
                                input='', capture_output=True, text=True, timeout=30, check=True)
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError:
            # A transient empty SSH result is not a database/lease success.
            # Only this read-only observation is retried; never a Live creation.
            diagnostic = {'at': time.time(), 'attempt': attempt + 1, 'exit': result.returncode,
                          'stdout_bytes': len(result.stdout), 'stderr_bytes': len(result.stderr)}
            with (root / 'snapshot-transport-errors.jsonl').open('a') as stream:
                stream.write(json.dumps(diagnostic) + '\n')
            if result.stdout.strip() or attempt == 2:
                raise RuntimeError('candidate read-only snapshot unavailable') from None
            time.sleep(.5)



@production_case
@mode_guard
def main():
    root, scenario = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2]
    private = root / 'private'
    cases = {'pool-air-healthy': ('healthy', 'healthy', 'air'),
             'pool-air-down-this-mac': ('healthy', 'healthy', 'this-mac'),
             'pool-two-down-other-pro': ('refuse', 'healthy', 'other-pro'),
             'pool-all-down': ('refuse', 'refuse', None),
             'pool-air-recovered': ('healthy', 'healthy', 'air'),
             'this-mac-healthy': ('healthy', 'healthy', 'this-mac'),
             'other-pro-after-refusal': ('refuse', 'healthy', 'other-pro'),
             'other-pro-after-hang': ('hang', 'healthy', 'other-pro'),
             'other-pro-after-busy': ('busy', 'healthy', 'other-pro'),
             'other-pro-after-malformed': ('malformed', 'healthy', 'other-pro'),
             'both-unavailable': ('refuse', 'refuse', None),
             'recovered-primary': ('healthy', 'healthy', 'this-mac'),
             'established-session-helpers-down': ('healthy', 'healthy', 'this-mac')}
    direct_cases = {'direct-other-healthy': 'other-pro', 'direct-tunnel-down-air': 'air',
                    'direct-two-down-this': 'this-mac', 'direct-all-down': None,
                    'direct-recovered': 'other-pro', 'direct-air-healthy': 'air',
                    'direct-air-down-this': 'this-mac', 'direct-air-this-down-other': 'other-pro'}
    is_direct = scenario in direct_cases
    if is_direct:
        first, second, expected_backend = 'healthy', 'healthy', direct_cases[scenario]
        direct_state = apply_case(root, scenario)
    else:
        first, second, expected_backend = cases[scenario]
        direct_state = None

    if scenario.startswith('pool-'):
        config = json.loads((private / 'router-config.json').read_text())
        if [x['name'] for x in config['backends']] != ['air', 'this-mac', 'other-pro']:
            raise ValueError('three-node candidate required')
    if scenario in {'pool-air-down-this-mac', 'pool-two-down-other-pro', 'pool-all-down'}:
        stop_marker(private / 'stop-air-reference')
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            with socket.socket() as sock:
                if sock.connect_ex(('127.0.0.1', 19487)) != 0:
                    break
            time.sleep(.05)
        else:
            raise RuntimeError('candidate Air forward did not close')
    air_reference = reference_state(private, expected_backend == 'air') if scenario.startswith('pool-') else None
    if not is_direct:
        set_modes(private, first, second)
        wait_modes(private, first, second)
    time.sleep(3.3)
    before = snapshot(root)
    if any(before['leases'].values()) or any(x['state'] != 'closed' for x in before['records']):
        raise RuntimeError('previous candidate call still active')
    audit = pathlib.Path(json.loads((private / 'router-config.json').read_text())['audit'])
    prior = len(audit.read_text().splitlines())
    payload = {'key': (private / 'live-test-key').read_text().strip(), 'scenario': scenario,
               'expectUnavailable': expected_backend is None,
               'workerDist': '/Users/saksham/OpenComputerV2/athena-livekit/services/athena-agent-worker/dist/src'}
    child = subprocess.Popen(['node', str(pathlib.Path(__file__).with_name('backup_live_smoke.mjs'))],
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    observations = []
    timer = threading.Timer(105, lambda: child.kill() if child.poll() is None else None)
    timer.daemon = True
    timer.start()
    try:
        child.stdin.write(json.dumps(payload))
        child.stdin.close()
        for line in child.stdout:
            try:
                row = json.loads(line)
            except ValueError:
                continue
            observations.append(row)
            print(json.dumps(row), flush=True)
            if row.get('stage') == 'session_ready' and scenario == 'established-session-helpers-down':
                set_modes(private, 'refuse', 'refuse')
        child.wait(timeout=105)
        result = next((x for x in reversed(observations) if 'success' in x), {})
        # Persist the sanitized client observation before any remote verification.
        (root / ('live-client-observation-' + scenario + '-' + str(time.time_ns()) + '.json')).write_text(
            json.dumps({'scenario': scenario, 'client_exit': child.returncode, 'client': result,
                        'observations': observations, 'verified': False}, indent=2))
        expected_usage = before['usage']['usage_rows'] + (0 if expected_backend is None else 1)
        deadline = time.monotonic() + 200
        while True:
            after = snapshot(root)
            settled = (after['usage']['usage_rows'] == expected_usage and not any(after['leases'].values())
                       and all(x['state'] == 'closed' for x in after['records']))
            if settled or time.monotonic() > deadline:
                break
            time.sleep(2)
        records = [json.loads(line) for line in audit.read_text().splitlines()[prior:]]
        proofs = [row for row in records if row.get('path', '/v1/attestation') == '/v1/attestation']
        selections = [row['selected'] for row in proofs if row.get('status') == 200]
        passed = bool(result.get('success') and settled and
                      (len(proofs) == 1 and selections == [expected_backend] if expected_backend else not selections))
        evidence = {'scenario': scenario, 'passed': passed, 'air_reference': air_reference, 'client_exit': child.returncode,
                    'direct_control': direct_state, 'client': result, 'selected_backends': selections, 'proof_requests': len(records),
                    'attestation_records': records,
                    'usage_before': before['usage'], 'usage_after': after['usage'],
                    'closed_records_after': sum(x['state'] == 'closed' for x in after['records']),
                    'open_records_after': sum(x['state'] != 'closed' for x in after['records']),
                    'leases_after': after['leases'], 'observations': observations}
        (root / ('live-case-' + scenario + '.json')).write_text(json.dumps(evidence, indent=2))
        print(json.dumps({'scenario': scenario, 'passed': passed, 'air_reference': air_reference, 'selected_backends': selections,
                          'usage_rows': after['usage']['usage_rows'], 'open_records': evidence['open_records_after'],
                          'lease_members': sum(after['leases'].values())}), flush=True)
        if not passed:
            raise SystemExit(1)
    finally:
        timer.cancel()
        if child.poll() is None:
            child.kill()
            child.wait(timeout=5)
        if not is_direct:set_modes(private, 'healthy', 'healthy')


if __name__ == '__main__':
    main()
