#!/usr/bin/env python3
"""Three native nodes; faults close candidate-only paths, never production Air."""
import hashlib
import http.client
import json
import pathlib
import socket
import sys
import time

from backup_modes import mode_guard, set_modes, wait_modes
from backup_phase_cleanup import stop_marker
from backup_router import validate_payload
from backup_production_check import production_case
from backup_air_reference import reference_state


@production_case
@mode_guard
def main():
    root, scenario = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2]
    private = root / 'private'
    config = json.loads((private / 'router-config.json').read_text())
    if [x['name'] for x in config['backends']] != ['air', 'this-mac', 'other-pro']:
        raise ValueError('three-node candidate order required')
    cases = {'healthy-air': ('healthy', 'healthy', 'air'),
             'air-tunnel-down': ('healthy', 'healthy', 'this-mac'),
             'two-down': ('refuse', 'healthy', 'other-pro'),
             'all-down': ('refuse', 'refuse', None),
             'air-recovered': ('healthy', 'healthy', 'air')}
    first, second, expected = cases[scenario]
    if scenario in {'air-tunnel-down', 'two-down', 'all-down'}:
        stop_marker(private / 'stop-air-reference')
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            with socket.socket() as sock:
                if sock.connect_ex(('127.0.0.1', 19487)) != 0:
                    break
            time.sleep(.05)
        else:
            raise RuntimeError('candidate Air forward did not close')
    air_reference = reference_state(private, expected in {'air'})
    set_modes(private, first, second)
    wait_modes(private, first, second)
    time.sleep(3.3)
    audit = pathlib.Path(config['audit'])
    prior = len(audit.read_text().splitlines())
    token = (private / 'router-token').read_text().strip()
    seen, samples = set(), []
    for _ in range(3):
        conn = http.client.HTTPConnection('127.0.0.1', 19495, timeout=8)
        start = time.monotonic()
        try:
            conn.request('POST', '/v1/attestation', body=b'', headers={'Authorization': 'Bearer ' + token})
            response = conn.getresponse()
            body = response.read(20481)
            if expected:
                if response.status != 200:
                    raise RuntimeError('candidate proof unavailable')
                validate_payload('/v1/attestation', body)
                digest = hashlib.sha256(json.loads(body)['attestation'].encode()).digest()
                if digest in seen:
                    raise RuntimeError('duplicate candidate proof')
                seen.add(digest)
            elif response.status != 503:
                raise RuntimeError('all-down did not fail closed')
            samples.append({'status': response.status, 'ms': round((time.monotonic() - start) * 1000, 3)})
        finally:
            conn.close()
        time.sleep(.5)
    time.sleep(.1)
    records = [json.loads(x) for x in audit.read_text().splitlines()[prior:]]
    passed = len(records) == 3 and all(x['selected'] == expected and x['status'] == (200 if expected else 503) for x in records)
    evidence = {'scenario': scenario, 'passed': passed, 'air_reference': air_reference, 'selected': expected,
                'valid_distinct_proofs': len(seen), 'samples': samples, 'router_records': records,
                'production_integrity_evidence': 'production-case-' + scenario + '.json',
                'topology': 'candidate Air via operator->AWS->existingAir; backups operator-local/SSH; not AWS direct pool'}
    (root / ('pool-case-' + scenario + '.json')).write_text(json.dumps(evidence, indent=2))
    print(json.dumps({k: v for k, v in evidence.items() if k != 'router_records'}), flush=True)
    if not passed:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
