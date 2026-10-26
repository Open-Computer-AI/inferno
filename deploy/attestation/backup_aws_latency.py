#!/usr/bin/env python3
"""Matched AWS-origin candidate measurements. Backup paths explicitly use operator relay."""
import json
import pathlib
import os
import secrets
import shlex
import subprocess
import sys
import threading
import time

from backup_air_reference import aws_command
from backup_helpers import heartbeat_tunnel, spawn_tunnel, stop_group
from backup_production_check import snapshot

REMOTE = r'''
import asyncio,hashlib,json,os,signal,socket,statistics,sys,threading,time
payload=json.loads(sys.stdin.readline())
stopping=threading.Event()
for sig in (signal.SIGTERM,signal.SIGINT,signal.SIGHUP):signal.signal(sig,lambda *_:stopping.set())
def watch():
 while not stopping.wait(.1):
  import select
  if select.select([sys.stdin],[],[],.1)[0] and not sys.stdin.readline():stopping.set()
threading.Thread(target=watch,daemon=True).start()
source=payload.pop('source')
exec(compile(source,'candidate-router-memory-only','exec'),globals())
async def run():
 for port in (19495,):
  with socket.socket() as sock:sock.bind(('127.0.0.1',port))
 backends=[Backend(name,port,payload['tokens'][name]) for name,port in [('air',19085),('this-mac',19485),('other-pro',19486)]]
 class CountingRouter(Router):
  count=0
  def record(self,row):
   self.count+=1
   super().record(row)
 router=CountingRouter(payload['frontend'],backends)
 server=await asyncio.start_server(router.handle,'127.0.0.1',19495,limit=8192)
 print(json.dumps({'aws_candidate_actor_pid':os.getpid(),'aws_start_ticks':open('/proc/self/stat').read().rsplit(')',1)[1].split()[19],'aws_candidate_router_port':19495,'run_id':payload['run_id']}),flush=True)
 seen=set();output=[];deadline=time.monotonic()+600
 async def measure(backend,route):
  if stopping.is_set() or time.monotonic()>deadline:raise RuntimeError('candidate benchmark stopped')
  tick=time.monotonic();before=router.count
  if route=='direct':body=await asyncio.wait_for(exchange(backend,'/v1/attestation'),7)
  else:
   reader,writer=await asyncio.open_connection('127.0.0.1',19495)
   try:
    writer.write(('POST /v1/attestation HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer '+payload['frontend']+'\r\nContent-Length: 0\r\nConnection: close\r\n\r\n').encode());await writer.drain()
    header=await asyncio.wait_for(reader.readuntil(b'\r\n\r\n'),8)
    if header.split()[1]!=b'200':raise RuntimeError('candidate routed HTTP failure')
    fields=dict(line.lower().split(b':',1) for line in header.split(b'\r\n')[1:] if b':' in line)
    length=int(fields[b'content-length'].strip())
    if length>20480:raise RuntimeError('candidate response bound')
    body=await asyncio.wait_for(reader.readexactly(length),8)
    validate_payload('/v1/attestation',body)
   finally:writer.close();await writer.wait_closed()
   await asyncio.sleep(.001)
   if router.count!=before+1 or not router.records or router.records[-1]['selected']!=backend.name or router.records[-1]['status']!=200:raise RuntimeError('candidate selection mismatch')
  elapsed=(time.monotonic()-tick)*1000
  digest=hashlib.sha256(json.loads(body)['attestation'].encode()).digest()
  if digest in seen:raise RuntimeError('duplicate candidate proof')
  seen.add(digest)
  # Air is an existing production helper, so its reference rate stays<=1/sec.
  await asyncio.sleep(max(0,(1 if backend.name=='air' else .25)-(time.monotonic()-tick)))
  return round(elapsed,3)
 try:
  if payload.get('gate'):
   while not stopping.is_set() and time.monotonic()<deadline:await asyncio.sleep(.1)
   return
  for backend in backends[:]:
   router.backends=[backend]+[x for x in backends if x is not backend]
   pairs=[]
   for i in range(100):
    row={'pair':i,'order':['direct','router'] if i%2==0 else ['router','direct']}
    for route in row['order']:row[route+'_ms']=await measure(backend,route)
    row['delta_ms']=round(row['router_ms']-row['direct_ms'],3);pairs.append(row)
    if (i+1)%20==0:print(json.dumps({'aws_pairs_completed':i+1,'backend':backend.name}),flush=True)
   output.append({'backend':backend.name,'pairs':pairs})
   print(json.dumps({'aws_backend_measurement_complete':True,'backend':backend.name,'pairs':pairs}),flush=True)
  print(json.dumps({'aws_measurement_complete':True,'valid_distinct_proofs':len(seen),'routed_records':router.count,'measurements':output,'errors':[]}),flush=True)
 finally:
  server.close();await server.wait_closed()
  for task in list(router.active):task.cancel()
  await asyncio.gather(*list(router.active),return_exceptions=True)
  print(json.dumps({'aws_candidate_router_stopped':True,'in_flight':router.in_flight}),flush=True)
try:asyncio.run(run())
except BaseException as exc:print(json.dumps({'aws_measurement_failed':True,'error_class':type(exc).__name__}),flush=True);raise SystemExit(1)
'''


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    before = snapshot(root)
    run_id = secrets.token_hex(8)
    gate = sys.argv[2:] == ['--gate']
    records_path = root / ('aws-latency-records-' + run_id + '.jsonl')
    fd = os.open(records_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600);os.close(fd)
    command = aws_command(private)
    prefix = ['ssh', '-F', '/dev/null', '-T'] + command[1:-2]
    remote_target = command[-2]
    def identity(pid):
        return subprocess.check_output(['ps', '-p', str(pid), '-o', 'lstart='], text=True).strip()
    metadata = {'controller_pid': os.getpid(), 'controller_started': identity(os.getpid()),
                'started_at': time.time(), 'run_id': run_id, 'gate': gate, 'records': str(records_path),
                'phase': 'before-spawn'}
    path = root / ('aws-latency-processes-' + run_id + '.json')
    pointer = private / 'aws-latency-active.json'
    if pointer.exists():
        raise RuntimeError('previous AWS actor must be independently verified first')
    def save_metadata():
        import tempfile
        with tempfile.NamedTemporaryFile(mode='w', dir=root, prefix='aws-state-', delete=False) as stream:
            json.dump(metadata, stream)
            temporary = pathlib.Path(stream.name)
        temporary.chmod(0o600)
        temporary.replace(path)
    save_metadata()
    fd = os.open(pointer, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:json.dump({'metadata': str(path), 'run_id': run_id}, stream)
    tunnel = actor = None
    rows = []
    try:
        tunnel = spawn_tunnel(prefix + ['-o', 'ExitOnForwardFailure=yes', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
                                          '-o', 'ServerAliveInterval=5', '-o', 'ServerAliveCountMax=2', '-N',
                                          '-R', '127.0.0.1:19485:127.0.0.1:19485',
                                          '-R', '127.0.0.1:19486:127.0.0.1:19486', remote_target])
        metadata.update(tunnel_supervisor_pid=tunnel.pid, supervisor_started=identity(tunnel.pid),
                        tunnel_ssh_pid=tunnel.tunnel_state['ssh_pid'], ssh_started=identity(tunnel.tunnel_state['ssh_pid']), phase='tunnel-started')
        save_metadata()
        source = pathlib.Path(__file__).with_name('backup_router.py').read_text().rsplit("if __name__ == '__main__':", 1)[0]
        source = source.replace('from backup_helpers import owned_file', '')
        tokens = json.loads((private / 'backend-tokens.json').read_text())
        tokens['air'] = (private / 'air-reference-token').read_text().strip()
        payload = {'source': source, 'frontend': secrets.token_urlsafe(32), 'tokens': tokens, 'gate': gate, 'run_id': run_id}
        actor = subprocess.Popen(prefix + [remote_target, 'python3 -u -c ' + shlex.quote(REMOTE)],
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                 text=True, start_new_session=True)
        actor.stdin.write(json.dumps(payload) + '\n');actor.stdin.flush()
        metadata.update(actor_ssh_pid=actor.pid, actor_started=identity(actor.pid), phase='actor-started')
        save_metadata()
        def reader():
            for line in actor.stdout:
                row = json.loads(line)
                rows.append(row)
                with records_path.open('a') as stream:stream.write(json.dumps(row)+'\n')
                if 'measurements' not in row and 'pairs' not in row:
                    print(json.dumps(row), flush=True)
        thread = threading.Thread(target=reader, daemon=True);thread.start()
        started = time.monotonic()
        last_health = started
        while actor.poll() is None and time.monotonic() - started < 650 and not (private / 'stop-aws-latency').exists():
            if time.monotonic() - last_health >= 20:
                health = snapshot(root)
                with (root / ('aws-production-monitor-' + run_id + '.jsonl')).open('a') as stream:
                    stream.write(json.dumps({'at': time.time(), 'production': health}) + '\n')
                last_health = time.monotonic()
            heartbeat_tunnel(tunnel)
            actor.stdin.write('heartbeat\n');actor.stdin.flush()
            time.sleep(1)
        if actor.poll() is None:
            raise RuntimeError('AWS measurement lifetime exceeded')
        thread.join(timeout=3)
        complete = next((x for x in rows if x.get('aws_measurement_complete')), None)
        evidence = {'run_id': run_id, 'independent_cleanup_verified': False, 'before': before, 'after': snapshot(root), 'client_exit': actor.returncode,
                    'topology': {'air': 'AWS loopback19085->existing production Air reverse tunnel',
                                 'this-mac': 'AWS loopback19485->operator SSH reverse-forward->local candidate helper',
                                 'other-pro': 'AWS loopback19486->operator reverse-forward->operator local19486->Other SSH helper; operator relay required'},
                    'mode': '100 counterbalanced direct/router pairs per backend; freshHTTP; matchingtransport; Air reference<=1proof/sec',
                    'result': complete, 'records': [x for x in rows if 'measurements' not in x]}
        (root / 'aws-origin-paired-latency.json').write_text(json.dumps(evidence, indent=2))
        if not gate and (actor.returncode or not complete or complete['valid_distinct_proofs'] != 600):
            raise RuntimeError('AWS candidate measurement incomplete')
    finally:
        if actor:
            try:actor.stdin.close()
            except BrokenPipeError:pass
            stop_group(actor)
        if tunnel:stop_group(tunnel)
        # A separate caller must verify remote listener absence and process groups.
        print(json.dumps({'aws_measurement_local_actors_stopped': True}), flush=True)


if __name__ == '__main__':
    main()
