"""Read-only production identity and authenticated health guard for pool tests."""
import json
import pathlib
import subprocess

from backup_air_reference import aws_command

SOURCE = r'''
import ast,hashlib,http.client,json,re,subprocess
pids={}
for role,pid in [('gateway',481177),('queue',1038746)]:
 raw=subprocess.check_output(['sudo','cat','/proc/'+str(pid)+'/stat']).decode()
 pids[role]={'pid':pid,'start_ticks':raw.rsplit(')',1)[1].split()[19]}
binary=subprocess.check_output(['sudo','sha256sum','/proc/481177/exe']).decode().split()[0]
queue=subprocess.check_output(['sudo','sha256sum','/opt/inferno-queue/attest_queue.py']).decode().split()[0]
raw=subprocess.check_output(['sudo','cat','/proc/481177/environ'])
env=dict(x.split(b'=',1) for x in raw.split(b'\0') if b'=' in x)
token=subprocess.check_output(['sudo','cat','/proc/481177/root'+env[b'LIVE_ATTESTATION_TOKEN_FILE'].decode()]).decode().strip()
health={}
for port in (19085,19095):
 c=http.client.HTTPConnection('127.0.0.1',port,timeout=3)
 try:
  c.request('GET','/health',headers={'Authorization':'Bearer '+token})
  response=c.getresponse();body=json.loads(response.read(4096))
  health[str(port)]={'status':response.status,'supported':body.get('supported') is True}
 finally:c.close()
c=http.client.HTTPConnection('127.0.0.1',19095,timeout=3)
try:
 c.request('GET','/queue-health');response=c.getresponse();stats=ast.literal_eval(response.read(4096).decode())
 if response.status!=200 or not isinstance(stats,dict) or any(type(x) not in (int,float) for x in stats.values()):raise RuntimeError('queue counters invalid')
finally:c.close()
listeners=[]
for line in subprocess.check_output(['sudo','ss','-lntp'],text=True).splitlines():
 fields=line.split()
 if len(fields)>4 and fields[3].endswith(':19085'):
  listeners.extend(int(x) for x in re.findall(r'pid=(\d+)',line))
print(json.dumps({'processes':pids,'gateway_sha256':binary,'queue_sha256':queue,'air_reverse_listener_pids':sorted(set(listeners)),'health':health,'queue_stats':stats}))
'''


def snapshot(root):
    result = subprocess.run(aws_command(root / 'private'), input=SOURCE,
                            capture_output=True, text=True, timeout=20, check=True)
    row = json.loads(result.stdout)
    if row['gateway_sha256'] != '4bb9f392de4cfc17586223d6e903c4b3403d5afaf87c8d07f21e254f8fc3267c':
        raise RuntimeError('production gateway identity changed')
    if row['queue_sha256'] != '9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d':
        raise RuntimeError('production queue identity changed')
    if not row['air_reverse_listener_pids'] or not all(x['status'] == 200 and x['supported'] for x in row['health'].values()):
        raise RuntimeError('production Air/queue health failed')
    baseline = root / 'production-pool-baseline.json'
    if baseline.exists():
        before = json.loads(baseline.read_text())
        for field in ('processes', 'gateway_sha256', 'queue_sha256', 'air_reverse_listener_pids'):
            if row[field] != before[field]:
                raise RuntimeError('production identity differs from pool baseline')
    else:
        baseline.write_text(json.dumps(row, indent=2))
    counters = root / 'production-queue-counter-baseline.json'
    if counters.exists():
        previous = json.loads(counters.read_text())
        for field in ('failed', 'unreachable_retries', 'busy_retries'):
            if row['queue_stats'][field] > previous['queue_stats'][field]:
                raise RuntimeError('production queue error/retry counter increased')
    else:
        counters.write_text(json.dumps(row, indent=2))
    return row


if __name__ == '__main__':
    import sys
    print(json.dumps(snapshot(pathlib.Path(sys.argv[1]).resolve())))


def production_case(function):
    """Keep production checks outside fault-mode scope, including exceptional exits."""
    import functools
    import sys
    @functools.wraps(function)
    def wrapped(*args, **kwargs):
        root, scenario = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2]
        enabled = scenario.startswith('pool-') or scenario in {'healthy-air', 'air-tunnel-down', 'two-down', 'all-down', 'air-recovered'}
        if not enabled:
            return function(*args, **kwargs)
        before = snapshot(root)
        evidence = {'scenario': scenario, 'before': before, 'passed': False}
        try:
            result = function(*args, **kwargs)
            evidence['case_completed'] = True
            return result
        finally:
            try:
                evidence['after'] = snapshot(root)
                evidence['production_guard_passed'] = True
                evidence['passed'] = evidence.get('case_completed') is True
            finally:
                (root / ('production-case-' + scenario + '.json')).write_text(json.dumps(evidence, indent=2))
    return wrapped
