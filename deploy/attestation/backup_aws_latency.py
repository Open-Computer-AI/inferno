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
from backup_helpers import heartbeat_tunnel, spawn_tunnel, stop_group, owned_file, ssh_prefix
from backup_production_check import snapshot
from backup_direct_transport import parse_control

REMOTE = r'''
import asyncio,hashlib,json,os,signal,socket,statistics,sys,threading,time,queue
payload=json.loads(sys.stdin.readline())
stopping=threading.Event();controls=queue.SimpleQueue()
for sig in (signal.SIGTERM,signal.SIGINT,signal.SIGHUP):signal.signal(sig,lambda *_:stopping.set())
def watch():
 while not stopping.wait(.1):
  import select
  if select.select([sys.stdin],[],[],.1)[0]:
   line=sys.stdin.readline(4097)
   if not line:stopping.set()
   elif line.startswith('{'):
    try:controls.put(parse_control(line) if payload.get('direct_other') else json.loads(line))
    except (ValueError,TypeError):
     print(json.dumps({'aws_control_failed':True,'run_id':payload['run_id']}),flush=True);stopping.set()
source=payload.pop('source')
exec(compile(source,'candidate-router-memory-only','exec'),globals())
if payload.get('direct_other'):exec(compile(payload.pop('direct_source'),'candidate-direct-SSH-memory-only','exec'),globals())
threading.Thread(target=watch,daemon=True).start()
async def run():
 for port in (19495,):
  with socket.socket() as sock:sock.bind(('127.0.0.1',port))
 backends=[Backend(name,port,payload['tokens'][name]) for name,port in [('air',19085),('this-mac',19485),('other-pro',19486)]]
 class CountingRouter(Router):
  count=0
  def record(self,row):
   self.count+=1
   super().record(row)
   if payload.get('serve'):print(json.dumps({'aws_router_audit':row}),flush=True)
 router=CountingRouter(payload['frontend'],backends)
 server=await asyncio.start_server(router.handle,'127.0.0.1',19495,limit=8192)
 print(json.dumps({'aws_candidate_actor_pid':os.getpid(),'aws_start_ticks':open('/proc/self/stat').read().rsplit(')',1)[1].split()[19],'aws_candidate_router_port':19495,'run_id':payload['run_id']}),flush=True)
 direct=None;reserved=[]
 seen=set();output=[];deadline=time.monotonic()+(1700 if payload.get('serve') else 600)
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
  if payload.get('direct_other'):
   for port in (19487,19488):
    held=socket.socket();held.bind(('127.0.0.1',port));reserved.append(held)
   direct=DirectSSH(payload['other_known_host'],payload['run_id'],payload['source_node_fingerprint'])
   print(json.dumps({'aws_direct_transport_root':str(direct.root),'run_id':payload['run_id']}),flush=True)
   print(json.dumps(direct.start()),flush=True)
   ready=time.monotonic()+10
   while True:
    try:await asyncio.wait_for(exchange(backends[2],'/health'),1);break
    except (BackendFailure,asyncio.TimeoutError):
     if direct.child.poll() is not None or time.monotonic()>ready:raise RuntimeError('direct helper path unavailable')
     await asyncio.sleep(.1)
   print(json.dumps({'aws_direct_helper_authenticated':True,'run_id':payload['run_id']}),flush=True)
  if payload.get('serve'):
   cases={'direct-other-healthy':False,'direct-tunnel-down-air':True,'direct-two-down-this':True,'direct-all-down':True,'direct-recovered':False,'direct-air-healthy':False,'direct-air-down-this':False,'direct-air-this-down-other':False}
   last=None
   while not stopping.is_set() and time.monotonic()<deadline:
    if not controls.empty():
     control=controls.get();case=control.get('case');seq=control.get('sequence')
     if case not in cases or type(seq) is not int:raise RuntimeError('invalid direct case control')
     if seq!=last:
      if router.in_flight or router.active:raise RuntimeError('case change while candidate request active')
      if cases[case]:
       direct.stop()
       if direct.port_open():raise RuntimeError('direct SSH port did not close')
      elif direct.child.poll() is not None:
       print(json.dumps(direct.start()),flush=True)
       ready=time.monotonic()+10
       while True:
        try:await asyncio.wait_for(exchange(backends[2],'/health'),1);break
        except (BackendFailure,asyncio.TimeoutError):
         if direct.child.poll() is not None or time.monotonic()>ready:raise RuntimeError('recovered direct helper unavailable')
         await asyncio.sleep(.1)
      backends[0].port=19487 if case in {'direct-two-down-this','direct-all-down','direct-air-down-this','direct-air-this-down-other'} else 19085
      backends[1].port=19488 if case in {'direct-all-down','direct-air-this-down-other'} else 19485
      router.backends=backends[:] if case.startswith('direct-air-') else [backends[2],backends[0],backends[1]]
      for backend in backends:backend.unavailable_until=0;backend.failed=False;backend.probing=False
      last=seq
      print(json.dumps({'aws_direct_control_applied':True,'case':case,'sequence':seq,'run_id':payload['run_id'],'direct_ssh_alive':direct.child.poll() is None,'direct_port_open':direct.port_open()}),flush=True)
    await asyncio.sleep(.1)
   return
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
  if direct:direct.close()
  for held in reserved:held.close()
  print(json.dumps({'aws_candidate_router_stopped':True,'in_flight':router.in_flight}),flush=True)
try:asyncio.run(run())
except BaseException as exc:print(json.dumps({'aws_measurement_failed':True,'error_class':type(exc).__name__}),flush=True);raise SystemExit(1)
'''


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    before = snapshot(root)
    run_id = secrets.token_hex(8)
    flags = set(sys.argv[2:])
    if flags - {'--gate', '--direct-other', '--serve'}:raise ValueError('unexpected candidate flags')
    gate, direct_other, serve = '--gate' in flags, '--direct-other' in flags, '--serve' in flags
    if serve and not direct_other:raise ValueError('serve requires direct transport')
    records_path = root / ('aws-latency-records-' + run_id + '.jsonl')
    fd = os.open(records_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600);os.close(fd)
    command = aws_command(private)
    prefix = ['ssh', '-F', '/dev/null', '-T'] + command[1:-2]
    remote_target = command[-2]
    def identity(pid):
        return subprocess.check_output(['ps', '-p', str(pid), '-o', 'lstart='], text=True).strip()
    metadata = {'controller_pid': os.getpid(), 'controller_started': identity(os.getpid()),
                'started_at': time.time(), 'run_id': run_id, 'gate': gate, 'records': str(records_path),
                'phase': 'before-spawn', 'direct_other': direct_other, 'serve': serve,
                'phase_run_id': json.loads((private / 'live-manifest.json').read_text())['run_id']}
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
        forwards = ['-R', '127.0.0.1:19485:127.0.0.1:19485']
        if not direct_other:forwards += ['-R', '127.0.0.1:19486:127.0.0.1:19486']
        if serve:forwards += ['-L', '127.0.0.1:19495:127.0.0.1:19495']
        tunnel = spawn_tunnel(prefix + ['-o', 'ExitOnForwardFailure=yes', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
                                       '-o', 'ServerAliveInterval=5', '-o', 'ServerAliveCountMax=2', '-N'] + forwards + [remote_target])
        metadata.update(tunnel_supervisor_pid=tunnel.pid, supervisor_started=identity(tunnel.pid),
                        tunnel_ssh_pid=tunnel.tunnel_state['ssh_pid'], ssh_started=identity(tunnel.tunnel_state['ssh_pid']), phase='tunnel-started')
        save_metadata()
        source = pathlib.Path(__file__).with_name('backup_router.py').read_text().rsplit("if __name__ == '__main__':", 1)[0]
        source = source.replace('from backup_helpers import owned_file', '')
        tokens = json.loads((private / 'backend-tokens.json').read_text())
        tokens['air'] = (private / 'air-reference-token').read_text().strip()
        payload = {'source': source, 'frontend': (private / 'router-token').read_text().strip() if serve else secrets.token_urlsafe(32),
                   'tokens': tokens, 'gate': gate, 'run_id': run_id, 'direct_other': direct_other, 'serve': serve}
        if direct_other:
            ssh_prefix(private)
            payload.update(direct_source=pathlib.Path(__file__).with_name('backup_direct_transport.py').read_text(),
                           other_known_host=(private / 'other_known_hosts').read_text(),
                           source_node_fingerprint=json.loads((root / 'direct-client-identity-preflight.json').read_text())['source_node_fingerprint'])
        audit = pathlib.Path(json.loads((private / 'router-config.json').read_text())['audit']) if serve else None
        if audit:
            fd = os.open(audit, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600);os.close(fd)

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
                if audit and 'aws_router_audit' in row:
                    with audit.open('a') as stream:stream.write(json.dumps(row['aws_router_audit'])+'\n')
                with records_path.open('a') as stream:stream.write(json.dumps(row)+'\n')
                if 'measurements' not in row and 'pairs' not in row and 'aws_router_audit' not in row:
                    print(json.dumps(row), flush=True)
        thread = threading.Thread(target=reader, daemon=True);thread.start()
        started = time.monotonic()
        last_health = started
        while actor.poll() is None and time.monotonic() - started < (1750 if serve else 650) and not (private / 'stop-aws-latency').exists():
            if time.monotonic() - last_health >= 20:
                health = snapshot(root)
                with (root / ('aws-production-monitor-' + run_id + '.jsonl')).open('a') as stream:
                    stream.write(json.dumps({'at': time.time(), 'production': health}) + '\n')
                last_health = time.monotonic()
            heartbeat_tunnel(tunnel)
            control = private / 'aws-direct-control.json'
            if serve and control.exists():
                owned_file(control, 0o600);message=json.dumps(parse_control(control.read_text()))+'\n'
            else:message='heartbeat\n'
            actor.stdin.write(message);actor.stdin.flush()
            time.sleep(1)
        if actor.poll() is None and not serve:
            raise RuntimeError('AWS measurement lifetime exceeded')
        thread.join(timeout=3)
        complete = next((x for x in rows if x.get('aws_measurement_complete')), None)
        evidence = {'run_id': run_id, 'independent_cleanup_verified': False, 'before': before, 'after': snapshot(root), 'client_exit': actor.returncode,
                    'topology': {'air': 'AWS loopback19085->existing production Air reverse tunnel',
                                 'this-mac': 'AWS loopback19485->operator SSH reverse-forward->local candidate helper',
                                 'other-pro': 'AWS direct SSH local19486->Other loopback19485; no operator proof relay' if direct_other else 'AWS loopback19486->operator reverse-forward->operator local19486->Other SSH helper; operator relay required'},
                    'mode': '100 counterbalanced direct/router pairs per backend; freshHTTP; matchingtransport; Air reference<=1proof/sec',
                    'result': complete, 'records': [x for x in rows if 'measurements' not in x]}
        artifact = 'aws-direct-serve-' + run_id + '.json' if serve else ('aws-origin-direct-paired-latency.json' if direct_other else 'aws-origin-paired-latency.json')
        (root / artifact).write_text(json.dumps(evidence, indent=2))
        if not gate and not serve and (actor.returncode or not complete or complete['valid_distinct_proofs'] != 600):
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
