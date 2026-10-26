#!/usr/bin/env python3
"""Prepare one fresh-database phase with one approved access-only account."""
import hashlib
import json
import os
import pathlib
import secrets
import shlex
import subprocess
import sys
import tarfile

from backup_helpers import OTHER, owned_file, ssh_prefix
from backup_air_reference import aws_command


def write_private(path, text):
    if path.exists():
        raise ValueError('previous phase credential/config still exists')
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        stream.write(text)


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    mode = sys.argv[2]
    if mode not in {'db-only', 'live'}:
        raise ValueError('expected db-only or live phase')
    private = root / 'private'
    source = """import subprocess
q="SELECT json_build_object('id',id,'credentials',jsonb_build_object('access_token',credentials->'access_token','expires_at',credentials->'expires_at','chatgpt_account_id',credentials->'chatgpt_account_id')) FROM accounts WHERE id=28 AND platform='openai' AND type='oauth' AND status='active' AND schedulable=true AND deleted_at IS NULL"
p=subprocess.run(['sudo','docker','exec','inferno-validation-postgres','psql','-U','sub2api','-d','sub2api','-At','-c',q],capture_output=True,text=True,timeout=10,check=True)
print(p.stdout)
"""
    command = aws_command(private)
    result = subprocess.run(command, input=source, capture_output=True, text=True, timeout=20, check=True)
    account = json.loads(result.stdout)
    fingerprint = hashlib.sha256(account['credentials']['chatgpt_account_id'].encode()).hexdigest()
    scope = json.loads((root / 'test-account-access-scope.json').read_text())
    if fingerprint != scope['stable_account_fingerprint'] or set(account['credentials']) != {'access_token', 'expires_at', 'chatgpt_account_id'}:
        raise ValueError('access-only account identity mismatch')
    if not account['credentials']['access_token']:
        raise ValueError('approved access snapshot unavailable')
    write_private(private / 'test-account-access.json', json.dumps(account))
    write_private(private / 'router-token', secrets.token_urlsafe(32))
    write_private(private / 'live-test-key', 'sk-' + secrets.token_urlsafe(32))
    run_id = secrets.token_hex(6)
    prefix = ssh_prefix(private)
    create = "import tempfile,pathlib,json;r=pathlib.Path(tempfile.mkdtemp(prefix='inferno-backup-live-'));r.chmod(0o700);print(json.dumps({'root':str(r)}))"
    result = subprocess.run(prefix + [OTHER, 'python3 -c ' + shlex.quote(create)], capture_output=True, text=True, timeout=15, check=True)
    remote = json.loads(result.stdout)['root']
    state = private / 'live-remote-root.json'
    state.write_text(json.dumps({'root': remote, 'run_id': run_id}))
    state.chmod(0o600)
    password = secrets.token_urlsafe(32)
    config = {'server': {'host': '0.0.0.0', 'port': 19480, 'mode': 'release'},
              'database': {'host': 'inferno-backup-postgres-' + run_id, 'port': 5432,
                           'user': 'sub2api', 'password': password, 'dbname': 'sub2api', 'sslmode': 'disable'},
              'redis': {'host': 'inferno-backup-redis-' + run_id, 'port': 6379, 'db': 0},
              'jwt': {'secret': secrets.token_urlsafe(48)}, 'totp': {'encryption_key': secrets.token_hex(32)},
              'token_refresh': {'enabled': False}, 'gateway': {'live': {'max_session_duration_seconds': 180}}, 'timezone': 'UTC'}
    # JSON flow mappings are valid YAML; no extra local Python dependency.
    write_private(private / 'live-gateway-config.yaml', json.dumps(config, indent=2))
    write_private(private / 'live-postgres.env', 'POSTGRES_USER=sub2api\nPOSTGRES_DB=sub2api\nPOSTGRES_PASSWORD=' + password + '\n')
    manifest = {'root': remote, 'run_id': run_id, 'imported_snapshot': False,
                'hashes': {'server': json.loads((root / 'gateway-binary-identity.json').read_text())['sha256'],
                           'probe': hashlib.sha256((private / 'attestation-probe-linux').read_bytes()).hexdigest()},
                'actor_roles': ['router', 'fault-proxy'] if mode == 'live' else [], 'fresh_database': True}
    manifest.update(json.loads((root / 'candidate-docker-identity.json').read_text()))
    path = private / 'live-manifest.json'
    path.write_text(json.dumps(manifest))
    path.chmod(0o600)
    if mode == 'live':
        router = {'run_id': run_id, 'frontend_token': str(private / 'router-token'),
                  'backend_tokens': str(private / 'backend-tokens.json'),
                  'backends': [{'name': 'this-mac', 'port': 19488, 'token_key': 'this-mac'},
                               {'name': 'other-pro', 'port': 19489, 'token_key': 'other-pro'}],
                  'audit': str(root / ('router-audit-' + run_id + '.jsonl'))}
        for name, data in [('router-config.json', router),
                           ('fault-mode.json', {'this-mac': 'healthy', 'other-pro': 'healthy'})]:
            target = private / name
            if target.exists():
                owned_file(target, 0o600)
            target.write_text(json.dumps(data))
            target.chmod(0o600)
    files = [(private / 'aws-gateway-server', 'server'),
             (private / 'attestation-probe-linux', 'probe'), (private / 'live-gateway-config.yaml', 'config.yaml'),
             (private / 'live-postgres.env', 'postgres.env'),
             (private / 'live-manifest.json', 'manifest.json')]
    child = subprocess.Popen(prefix + [OTHER, 'umask 077; tar -xzf - -C ' + shlex.quote(remote)],
                             stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        with tarfile.open(fileobj=child.stdin, mode='w|gz') as archive:
            for path, name in files:
                archive.add(path, arcname=name, recursive=False)
        child.stdin.close()
        if child.wait(timeout=180):
            raise RuntimeError('candidate asset transfer failed')
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=5)
    for marker in ('stop-live', 'start-live-gateway', 'stop-helpers', 'stop-router', 'stop-fault-proxy'):
        (private / marker).unlink(missing_ok=True)
    print(json.dumps({'candidate_phase_prepared': mode, 'run_id': run_id, 'remote_root': remote,
                      'account_id': 28, 'source_read_only': True, 'refresh_exported': False}), flush=True)


if __name__ == '__main__':
    main()
