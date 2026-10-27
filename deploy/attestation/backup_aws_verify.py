#!/usr/bin/env python3
"""Independent read-only verifier for temporary AWS latency actor and forwards."""
import json
import pathlib
import subprocess
import sys
import shlex
import time

from backup_air_reference import aws_command
from backup_helpers import owned_file
from backup_production_check import snapshot

REMOTE = r'''
import json,pathlib,socket,sys,subprocess
row=json.loads(sys.stdin.readline());path=pathlib.Path('/proc')/str(row['pid'])/'stat'
try:same=path.read_text().rsplit(')',1)[1].split()[19]==row['start_ticks']
except FileNotFoundError:same=False
ports={}
for port in (19485,19486,19487,19488,19495):
 with socket.socket() as sock:
  sock.settimeout(.2);ports[str(port)]=sock.connect_ex(('127.0.0.1',port))!=0
direct=[]
groups=subprocess.check_output(['ps','-axo','pgid='],text=True).split()
for item in row.get('direct',[]):
 path=pathlib.Path('/proc')/str(item['pid'])/'stat'
 try:present=path.read_text().rsplit(')',1)[1].split()[19]==item['start_ticks']
 except FileNotFoundError:present=False
 direct.append(not present and str(item['pid']) not in groups)
roots=row.get('roots',[])
if any(pathlib.Path(x).parent!=pathlib.Path('/tmp') or not pathlib.Path(x).name.startswith('inferno-backup-direct-') for x in roots):raise ValueError('unexpected direct root')
print(json.dumps({'remote_actor_identity_gone':not same,'remote_candidate_ports_closed':ports,'direct_ssh_groups_gone':all(direct),'direct_known_host_roots_removed':all(not pathlib.Path(x).exists() for x in roots)}))
'''


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    pointer = private / 'aws-latency-active.json'
    owned_file(pointer, 0o600)
    state = json.loads(pointer.read_text())
    path = pathlib.Path(state['metadata']).resolve()
    if path.parent != root or path.name != 'aws-latency-processes-' + state['run_id'] + '.json':
        raise ValueError('unexpected AWS metadata identity')
    owned_file(path, 0o600)
    metadata = json.loads(path.read_text())
    records = pathlib.Path(metadata['records']).resolve()
    if records.parent != root or records.name != 'aws-latency-records-' + state['run_id'] + '.jsonl':
        raise ValueError('unexpected AWS record identity')
    owned_file(records, 0o600)
    rows = [json.loads(x) for x in records.read_text().splitlines()]
    actor = next(x for x in rows if x.get('aws_candidate_actor_pid'))
    if actor['run_id'] != state['run_id'] or type(actor['aws_candidate_actor_pid']) is not int:
        raise ValueError('AWS actor identity mismatch')
    command = aws_command(private)[:-1] + ['python3 -c ' + shlex.quote(REMOTE)]
    deadline = time.monotonic() + 30
    while True:
        local = {}
        groups = subprocess.check_output(['ps', '-axo', 'pgid='], text=True).split()
        for key, start in (('controller_pid', 'controller_started'), ('actor_ssh_pid', 'actor_started'),
                           ('tunnel_supervisor_pid', 'supervisor_started'), ('tunnel_ssh_pid', 'ssh_started')):
            pid = metadata[key]
            result = subprocess.run(['ps', '-p', str(pid), '-o', 'lstart='], text=True, capture_output=True)
            same = result.returncode == 0 and result.stdout.strip() == metadata[start]
            local[key] = not same and (key == 'controller_pid' or str(pid) not in groups)
        result = subprocess.run(command, input=json.dumps({'pid': actor['aws_candidate_actor_pid'], 'start_ticks': actor['aws_start_ticks'], 'direct':[{'pid':x['aws_direct_other_ssh_pid'],'start_ticks':x['start_ticks']} for x in rows if x.get('aws_direct_other_ssh_pid')], 'roots':[x['aws_direct_transport_root'] for x in rows if x.get('aws_direct_transport_root')]}) + '\n',
                                capture_output=True, text=True, timeout=15, check=True)
        remote = json.loads(result.stdout)
        passed = all(local.values()) and remote['remote_actor_identity_gone'] and all(remote['remote_candidate_ports_closed'].values()) and remote['direct_ssh_groups_gone'] and remote['direct_known_host_roots_removed']
        if passed or time.monotonic() > deadline:
            break
        time.sleep(.5)
    evidence = {'run_id': state['run_id'], 'passed': passed, 'local_actor_identities_and_groups_gone': local,
                **remote, 'production_after': snapshot(root)}
    (root / ('aws-latency-independent-cleanup-' + state['run_id'] + '.json')).write_text(json.dumps(evidence, indent=2))
    print(json.dumps(evidence), flush=True)
    if not passed:
        raise SystemExit(1)
    measurement = root / ('aws-origin-direct-paired-latency.json' if metadata.get('direct_other') else 'aws-origin-paired-latency.json')
    if measurement.exists():
        data = json.loads(measurement.read_text())
        same_run = any(x.get('run_id') == state['run_id'] for x in data.get('records', []))
        if same_run:
            data['run_id'] = state['run_id']
            data['independent_cleanup_verified'] = True
            data['qualified_measurement'] = bool(data.get('client_exit') == 0 and data.get('result', {}).get('valid_distinct_proofs') == 600 and data.get('result', {}).get('routed_records') == 300)
            measurement.write_text(json.dumps(data, indent=2))
    pointer.unlink()


if __name__ == '__main__':
    main()
