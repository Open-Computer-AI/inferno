#!/usr/bin/env python3
"""Actual candidate-helper HTTP fault matrix. This is not a Live-session test."""
import hashlib
import http.client
import json
import pathlib
import socket
import sys
import time


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    token = (private / 'router-token').read_text().strip()
    audit = pathlib.Path(json.loads((private / 'router-config.json').read_text())['audit'])
    seen, rows = set(), []

    def modes(first, second, cooldown=True):
        temp = private / 'fault-mode.next'
        temp.write_text(json.dumps({'this-mac': first, 'other-pro': second}))
        temp.chmod(0o600)
        temp.replace(private / 'fault-mode.json')
        time.sleep(3.3 if cooldown else .3)

    def request(name, expected_status, selected):
        before = len(audit.read_text().splitlines())
        started = time.monotonic()
        conn = http.client.HTTPConnection('127.0.0.1', 19495, timeout=8)
        try:
            conn.request('POST', '/v1/attestation', body=b'', headers={'Authorization': 'Bearer ' + token})
            response = conn.getresponse()
            body = response.read(20481)
            status = response.status
        finally:
            conn.close()
        elapsed = (time.monotonic() - started) * 1000
        deadline = time.monotonic() + 1
        while len(audit.read_text().splitlines()) <= before and time.monotonic() < deadline:
            time.sleep(.02)
        entry = json.loads(audit.read_text().splitlines()[-1])
        valid, duplicate = False, False
        if status == 200:
            header = json.loads(body)['attestation']
            proof = json.loads(header)
            valid = proof.get('v') == 1 and proof.get('s') == 0 and isinstance(proof.get('t'), str) and proof['t'].startswith('v1.')
            digest = hashlib.sha256(header.encode()).digest()
            duplicate = digest in seen
            seen.add(digest)
        row = {'scenario': name, 'status': status, 'selected': entry['selected'], 'ms': round(elapsed, 3),
               'valid': valid, 'duplicate': duplicate, 'attempts': entry['attempts'],
               'slots_released': entry['in_flight_after'] == 0 and not any(entry['backend_in_flight_after'])}
        row['passed'] = (status == expected_status and entry['selected'] == selected and row['slots_released']
                         and (status != 200 or valid and not duplicate) and elapsed < 7900)
        rows.append(row)
        print(json.dumps(row), flush=True)
        if not row['passed']:
            raise RuntimeError('candidate fault scenario failed')

    try:
        modes('healthy', 'healthy')
        request('healthy-primary', 200, 'this-mac')
        for mode in ('refuse', 'drop', 'hang', 'busy', 'malformed'):
            modes(mode, 'healthy')
            request('primary-' + mode, 200, 'other-pro')
        modes('refuse', 'refuse')
        request('both-refused', 503, None)
        modes('hang', 'hang')
        request('both-hanging', 503, None)

        modes('hang', 'healthy')
        before_count = json.loads((private / 'fault-status.json').read_text())['counts']['other-pro']['accepted']
        conn = socket.create_connection(('127.0.0.1', 19495), timeout=2)
        conn.sendall(('POST /v1/attestation HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer ' + token + '\r\nContent-Length: 0\r\n\r\n').encode())
        time.sleep(.2)
        conn.close()
        time.sleep(.5)
        entry = json.loads(audit.read_text().splitlines()[-1])
        after = json.loads((private / 'fault-status.json').read_text())
        row = {'scenario': 'caller-cancellation', 'cancelled': entry['cancelled'],
               'no_backup_attempt': after['counts']['other-pro']['accepted'] == before_count,
               'proxy_connections_closed': not any(after['active'].values()),
               'slots_released': entry['in_flight_after'] == 0 and not any(entry['backend_in_flight_after'])}
        row['passed'] = all(row[x] for x in ('cancelled', 'no_backup_attempt', 'proxy_connections_closed', 'slots_released'))
        rows.append(row)
        print(json.dumps(row), flush=True)
        if not row['passed']:
            raise RuntimeError('caller cancellation failed')

        modes('refuse', 'healthy')
        request('recovery-before', 200, 'other-pro')
        modes('healthy', 'healthy', cooldown=False)
        request('recovery-during-cooldown', 200, 'other-pro')
        time.sleep(3.3)
        request('recovery-after-cooldown', 200, 'this-mac')
    finally:
        modes('healthy', 'healthy', cooldown=False)
        (root / 'actual-helper-fault-matrix.json').write_text(json.dumps(rows, indent=2))


if __name__ == '__main__':
    main()
