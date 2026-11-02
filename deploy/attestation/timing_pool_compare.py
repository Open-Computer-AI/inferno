#!/usr/bin/env python3
"""Low-rate, temporary attestation timing diagnostic; production settings stay unchanged."""
import json
import time
from timing_probe import measure, instrument_queue, TimedConnection

def main():
    import pathlib, subprocess, types, http.server, threading, ast, shutil
    container = json.loads(subprocess.check_output(['sudo', 'docker', 'inspect', 'inferno-validation-gateway']))[0]
    env = dict((x.split('=', 1) for x in container['Config']['Env'] if '=' in x))
    token = subprocess.check_output(['sudo', 'docker', 'exec', 'inferno-validation-gateway', 'cat', env['LIVE_ATTESTATION_TOKEN_FILE']], text=True).strip()
    q = types.ModuleType('timing_pooled_queue_copy')
    exec(compile(pathlib.Path('/opt/inferno-queue/attest_queue.py').read_bytes(), '<isolated-copy>', 'exec'), q.__dict__)
    instrument_queue(q)
    q.KEEPALIVE = True
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 19395), q.Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    result = {'rows': [], 'tcpdump_available': bool(shutil.which('tcpdump'))}
    try:
        result['warmup'] = measure(19395, '/health', token)
        for pair in range(12):
            for keepalive in [False, True] if pair % 2 == 0 else [True, False]:
                q.KEEPALIVE = keepalive
                r = measure(19395, '/v1/attestation', token)
                r.update(pair=pair, upstream_keepalive=keepalive, idle_pool=len(q.idle))
                result['rows'].append(r)
                if not r['valid_envelope'] or r['total_ms'] >= 4000:
                    raise RuntimeError('Timing safety guard')
                time.sleep(0.25)
        result['queue_stats'] = dict(q.stats)
        pid = subprocess.check_output(['systemctl', 'show', 'inferno-attestation-queue.service', '-p', 'MainPID', '--value'], text=True).strip()
        raw = subprocess.check_output(['sudo', 'cat', '/proc/' + pid + '/environ'])
        result['production_queue_environment'] = {x.split(b'=', 1)[0].decode(): x.split(b'=', 1)[1].decode() for x in raw.split(b'\x00') if x.startswith((b'QUEUE_CONCURRENCY=', b'QUEUE_KEEPALIVE='))}
        result['production_queue_pid'] = pid
    finally:
        server.shutdown()
        server.server_close()
        for conn, _ in q.idle:
            conn.close()
    print(json.dumps(result))
if __name__ == '__main__':
    main()
