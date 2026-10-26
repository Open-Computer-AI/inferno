#!/usr/bin/env python3
"""Paired, sequential, fresh-connection measurement; no proof material retained."""
import hashlib
import http.client
import json
import math
import pathlib
import random
import statistics
import subprocess
import sys
import time


def percentile(values, fraction):
    return sorted(values)[max(0, math.ceil(len(values) * fraction) - 1)]


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    backend = sys.argv[2]
    port = {'this-mac': 19488, 'other-pro': 19489}[backend]
    private = root / 'private'
    direct_token = json.loads((private / 'backend-tokens.json').read_text())[backend]
    router_token = (private / 'router-token').read_text().strip()
    config = json.loads((private / 'router-config.json').read_text())
    if config['backends'][0]['name'] != backend:
        raise ValueError('selected backend must be preferred for matched healthy measurement')
    audit = pathlib.Path(config['audit'])
    seen, samples = set(), []
    pressure_before = int(subprocess.check_output(['sysctl', '-n', 'kern.memorystatus_vm_pressure_level'], text=True))

    def measure(route):
        previous_records = len(audit.read_text().splitlines()) if route == 'router' else 0
        start = time.monotonic()
        conn = http.client.HTTPConnection('127.0.0.1', 19495 if route == 'router' else port, timeout=8)
        try:
            conn.request('POST', '/v1/attestation', body=b'',
                         headers={'Authorization': 'Bearer ' + (router_token if route == 'router' else direct_token)})
            response = conn.getresponse()
            data = response.read(20481)
            if response.status != 200 or len(data) > 20480:
                raise RuntimeError('candidate proof HTTP failure')
            header = json.loads(data)['attestation']
            proof = json.loads(header)
            if proof.get('v') != 1 or proof.get('s') != 0 or not proof.get('t', '').startswith('v1.'):
                raise RuntimeError('candidate proof invalid')
            digest = hashlib.sha256(header.encode()).digest()
            if digest in seen:
                raise RuntimeError('duplicate candidate proof')
            seen.add(digest)
            elapsed = (time.monotonic() - start) * 1000
        finally:
            conn.close()
        if route == 'router':
            deadline = time.monotonic() + 1
            while time.monotonic() < deadline:
                records = audit.read_text().splitlines()
                if len(records) > previous_records:
                    row = json.loads(records[-1])
                    break
                time.sleep(.01)
            else:
                raise RuntimeError('routed observation missing')
            if row['selected'] != backend:
                raise RuntimeError('measurement selected different backend')
        return round(elapsed, 3)

    for index in range(100):
        order = ['direct', 'router'] if index % 2 == 0 else ['router', 'direct']
        row = {'pair': index, 'order': order}
        for route in order:
            row[route + '_ms'] = measure(route)
        row['delta_ms'] = round(row['router_ms'] - row['direct_ms'], 3)
        samples.append(row)
        if (index + 1) % 20 == 0:
            print(json.dumps({'backend': backend, 'pairs_completed': index + 1}), flush=True)
        time.sleep(.25)
    delta = [x['delta_ms'] for x in samples]
    rng = random.Random(1138)
    means = [statistics.mean(rng.choices(delta, k=len(delta))) for _ in range(2000)]
    summary = {'backend': backend, 'pairs': len(samples), 'valid_distinct_proofs': len(seen),
               'mode': 'sequential fresh HTTP connections through same fault proxy; healthy preferred backend',
               'operator_pressure_before': pressure_before,
               'operator_pressure_after': int(subprocess.check_output(['sysctl', '-n', 'kern.memorystatus_vm_pressure_level'], text=True)),
               'mean_paired_delta_ms': round(statistics.mean(delta), 3),
               'mean_delta_bootstrap95_ms': [round(percentile(means, x), 3) for x in (.025, .975)]}
    for route in ('direct', 'router'):
        values = [x[route + '_ms'] for x in samples]
        summary[route] = {'p50_ms': round(statistics.median(values), 3),
                          'p95_ms': round(percentile(values, .95), 3), 'max_ms': max(values)}
    (root / ('paired-latency-' + backend + '.json')).write_text(json.dumps({'summary': summary, 'pairs': samples}, indent=2))
    print(json.dumps(summary), flush=True)


if __name__ == '__main__':
    main()
