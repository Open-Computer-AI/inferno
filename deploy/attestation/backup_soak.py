#!/usr/bin/env python3
"""Bounded native candidate soak: 16 simultaneous requests, at most one batch/sec."""
import concurrent.futures
import hashlib
import http.client
import json
import pathlib
import re
import shlex
import statistics
import subprocess
import sys
import time

from backup_helpers import OTHER, ssh_prefix
from backup_latency import percentile
from backup_modes import mode_guard, set_modes, wait_modes
from backup_router import validate_payload

RESOURCE = r'''
import json,re,subprocess,time
groups=subprocess.check_output(['ps','-axo','pgid='],text=True).split()
if str(HELPER_GROUP) not in groups:raise RuntimeError('candidate helper group absent')
pressure=int(subprocess.check_output(['sysctl','-n','kern.memorystatus_vm_pressure_level'],text=True))
vm=subprocess.check_output(['vm_stat'],text=True)
swap=int(re.search(r'Swapouts:\s+(\d+)',vm).group(1))
top=subprocess.check_output(['top','-l','2','-s','1','-n','0'],text=True)
cpu=re.findall(r'CPU usage: ([\d.]+)% user, ([\d.]+)% sys, ([\d.]+)% idle',top)[-1]
rows=[]
for line in subprocess.check_output(['ps','-axo','pid=,pgid=,rss=,pcpu=,comm='],text=True).splitlines():
 a=line.strip().split(None,4)
 if len(a)==5 and int(a[1])==HELPER_GROUP:
  rows.append({'pid':int(a[0]),'rss_kib':int(a[2]),'cpu_percent_one_core':float(a[3]),'kind':'helper' if 'inferno-backup-qualification-' in a[4] else 'helper-child'})
print(json.dumps({'at':time.time(),'pressure':pressure,'swapouts':swap,'cpu_busy_percent':round(float(cpu[0])+float(cpu[1]),2),'processes':rows}))
'''


def resource(private, backend):
    receipt = pathlib.Path((private / 'active-helper-receipt-path').read_text())
    group = next(x['helper_pid'] for x in reversed(json.loads(receipt.read_text())) if x.get('ready') and x['host'] == backend)
    program = RESOURCE.replace('HELPER_GROUP', str(group))
    cmd = ([sys.executable, '-c', program] if backend == 'this-mac' else
           ssh_prefix(private) + [OTHER, 'python3 -c ' + shlex.quote(program)])
    return json.loads(subprocess.check_output(cmd, text=True, timeout=15))


@mode_guard
def main():
    root, backend = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2]
    if backend not in ('this-mac', 'other-pro'):
        raise ValueError('candidate backend only')
    private = root / 'private'
    token = (private / 'router-token').read_text().strip()
    mode = ('healthy', 'refuse') if backend == 'this-mac' else ('refuse', 'healthy')
    samples, latency, seen, errors = [], [], set(), []
    reason = None
    set_modes(private, *mode)
    initial_modes = wait_modes(private, *mode)
    time.sleep(3.3)
    audit = pathlib.Path(json.loads((private / 'router-config.json').read_text())['audit'])
    prior = len(audit.read_text().splitlines())

    def proof(_):
        start = time.monotonic()
        conn = http.client.HTTPConnection('127.0.0.1', 19495, timeout=8)
        try:
            conn.request('POST', '/v1/attestation', body=b'', headers={'Authorization': 'Bearer ' + token})
            response = conn.getresponse()
            body = response.read(20481)
            if response.status != 200:
                return {'error': 'http_' + str(response.status)}
            validate_payload('/v1/attestation', body)
            return {'ms': (time.monotonic() - start) * 1000,
                    'hash': hashlib.sha256(json.loads(body)['attestation'].encode()).digest()}
        except Exception as exc:
            return {'error': type(exc).__name__}
        finally:
            conn.close()

    started = time.monotonic()
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
            for batch in range(120):
                tick = time.monotonic()
                if batch % 5 == 0:
                    sample = resource(private, backend)
                    samples.append(sample)
                    if sample['pressure'] != 1:
                        reason = 'memory_pressure_gate'
                    elif sample['cpu_busy_percent'] > 85:
                        reason = 'host_cpu_gate'
                    elif sample['swapouts'] > samples[0]['swapouts']:
                        reason = 'swap_activity_gate'
                    if reason:
                        break
                for result in pool.map(proof, range(16)):
                    if result.get('error'):
                        errors.append(result['error'])
                    else:
                        if result['hash'] in seen:
                            errors.append('duplicate')
                        seen.add(result['hash'])
                        latency.append(result['ms'])
                if errors or (latency and max(latency[-16:]) >= 4000):
                    reason = 'proof_error_or_latency_gate'
                    break
                if batch % 10 == 0:
                    print(json.dumps({'backend': backend, 'batches': batch + 1, 'valid': len(seen)}), flush=True)
                time.sleep(max(0, 1 - (time.monotonic() - tick)))
        samples.append(resource(private, backend))
        final = samples[-1]
        if final['pressure'] != 1 or final['cpu_busy_percent'] > 85 or final['swapouts'] > samples[0]['swapouts']:
            reason = reason or 'final_resource_gate'

        time.sleep(.3)
        final_modes = wait_modes(private, *mode)
        refused = 'other-pro' if backend == 'this-mac' else 'this-mac'
        unused_backend_untouched = all(final_modes['counts'][refused][key] == initial_modes['counts'][refused][key] for key in ('accepted', 'forwarded'))
        records = [json.loads(x) for x in audit.read_text().splitlines()[prior:]]
        selected = [x.get('selected') for x in records if x.get('status') == 200]
        evidence = {'backend': backend, 'concurrency': 16, 'max_batches': 120,
                    'paced_batch_interval_s': 1, 'elapsed_s': round(time.monotonic() - started, 3),
                    'valid_distinct_proofs': len(seen), 'errors': errors, 'stop_reason': reason,
                    'all_selections_expected': bool(selected) and all(x == backend for x in selected),
                    'audit_count': len(records),
                    'unused_backend_untouched': unused_backend_untouched,
                    'exact_audit_match': len(records) == len(seen) == len(latency) and all(x.get('status') == 200 and x.get('selected') == backend for x in records),
                    'swapouts_delta': final['swapouts'] - samples[0]['swapouts'],
                    'resource_samples': samples,
                    'latency': {('p' + str(int(p * 100)) + '_ms'): round(percentile(latency, p), 3) for p in (.5, .95, .99)} if latency else {}}
        if latency:
            evidence['latency']['max_ms'] = round(max(latency), 3)
        evidence['passed'] = reason is None and not errors and len(seen) == 1920 and evidence['all_selections_expected'] and evidence['exact_audit_match'] and unused_backend_untouched
        (root / ('soak-' + backend + '.json')).write_text(json.dumps(evidence, indent=2))
        print(json.dumps({k: v for k, v in evidence.items() if k != 'resource_samples'}), flush=True)
    finally:
        set_modes(private, 'healthy', 'healthy')


if __name__ == '__main__':
    main()
