#!/usr/bin/env python3
"""Low-rate, temporary attestation timing diagnostic; production settings stay unchanged."""
import json
import time
import subprocess
from timing_probe import measure, instrument_queue, TimedConnection

def collect_capture(capture):
    """Collect output with bounded cleanup of the capture's dedicated process group."""
    try:
        return capture.communicate(timeout=15)
    except subprocess.TimeoutExpired:
        for sig in ('TERM', 'KILL'):
            subprocess.run(['sudo', '-n', '/bin/kill', '-' + sig, '--', '-' + str(capture.pid)], capture_output=True, timeout=3)
            try:
                return capture.communicate(timeout=3)
            except subprocess.TimeoutExpired:
                pass
        raise RuntimeError('Packet capture did not finish after bounded cleanup')

def main():
    import pathlib, subprocess, signal, re
    c = json.loads(subprocess.check_output(['sudo', 'docker', 'inspect', 'inferno-validation-gateway']))[0]
    e = dict((x.split('=', 1) for x in c['Config']['Env'] if '=' in x))
    token = subprocess.check_output(['sudo', 'docker', 'exec', 'inferno-validation-gateway', 'cat', e['LIVE_ATTESTATION_TOKEN_FILE']], text=True).strip()

    def tcpinfo():
        p = subprocess.run(['sudo', 'ss', '-tinp', 'sport = :22'], capture_output=True, text=True, timeout=5)
        lines = p.stdout.splitlines()
        return [lines[i:i + 2] for i, line in enumerate(lines) if '100.70.23.11:' in line]
    out = {'ssh_tcp_before': tcpinfo(), 'requests': []}
    capture = subprocess.Popen(['sudo', '-n', 'timeout', '--signal=INT', '--kill-after=2s', '10s', 'tcpdump', '-i', 'tailscale0', '-nn', '-tt', '-q', '-l', '-s', '96', 'host 100.70.23.11 and tcp port 22'], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
    time.sleep(0.5)
    conn = TimedConnection('127.0.0.1', 19085, timeout=8)
    try:
        r = measure(19085, '/v1/attestation', token)
        r['label'] = 'fresh-proof-1'
        out['requests'].append(r)
        time.sleep(0.5)
        r = measure(19085, '/health', token, conn)
        r['label'] = 'connection-warmup-health'
        out['requests'].append(r)
        time.sleep(0.25)
        r = measure(19085, '/v1/attestation', token, conn)
        r['label'] = 'reused-proof'
        out['requests'].append(r)
        time.sleep(0.5)
        r = measure(19085, '/v1/attestation', token)
        r['label'] = 'fresh-proof-2'
        out['requests'].append(r)
    finally:
        conn.close()
        text, err = collect_capture(capture)
        out['packet_metadata'] = text.splitlines()
        out['capture_summary'] = err.splitlines()
        out['capture_exit'] = capture.returncode
        out['ssh_tcp_after'] = tcpinfo()
    print(json.dumps(out))
if __name__ == '__main__':
    main()
