#!/usr/bin/env python3
"""Temporary two-Mac qualification helpers; no launchd or production changes."""
import base64
import hashlib
import http.client
import json
import os
import pathlib
import secrets
import select
import shlex
import signal
import socket
import stat
import subprocess
import sys
import threading
import time

OTHER = 'architsakri@100.96.58.114'
FINGERPRINT = 'SHA256:VNpevfyqwYHSzO/HgU7d8GVNl2dsqxa+tPcOjF0NMJI'


def owned_file(path, mode):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != mode:
        raise ValueError('private file ownership/mode mismatch')


def ssh_prefix(private):
    keyfile = private / 'other_known_hosts'
    owned_file(keyfile, 0o600)
    lines = [x.split() for x in keyfile.read_text().splitlines() if x and not x.startswith('#')]
    if len(lines) != 1 or len(lines[0]) != 3 or lines[0][:2] != ['100.96.58.114', 'ssh-ed25519']:
        raise ValueError('unexpected host key')
    digest = hashlib.sha256(base64.b64decode(lines[0][2], validate=True)).digest()
    if 'SHA256:' + base64.b64encode(digest).decode().rstrip('=') != FINGERPRINT:
        raise ValueError('host key mismatch')
    # Tailscale SSH authenticates tailnet identity; no private key is installed.
    return ['ssh', '-F', '/dev/null', '-T', '-o', 'BatchMode=yes', '-o',
            'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + str(keyfile),
            '-o', 'ConnectTimeout=8', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
            '-o', 'ServerAliveInterval=5', '-o', 'ServerAliveCountMax=2']


def port_free(port):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', port))


def authenticated_health(port, token):
    start = time.monotonic()
    conn = http.client.HTTPConnection('127.0.0.1', port, timeout=1)
    try:
        conn.request('GET', '/health', headers={'Authorization': 'Bearer ' + token})
        response = conn.getresponse()
        body = response.read(20481)
        return {'port': port, 'status': response.status,
                'supported': response.status == 200 and len(body) <= 20480 and json.loads(body).get('supported') is True,
                'ms': round((time.monotonic() - start) * 1000, 3)}
    finally:
        conn.close()


def stop_group(child):
    if child.poll() is None:
        os.killpg(child.pid, signal.SIGTERM)
        try:
            child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait(timeout=5)


TUNNEL = r'''
import json,os,select,signal,subprocess,sys,time
stopping=False
def stop(*args):
 global stopping
 stopping=True
for sig in (signal.SIGINT,signal.SIGTERM,signal.SIGHUP):signal.signal(sig,stop)
command=json.loads(sys.stdin.readline())
if command[:4]!=['ssh','-F','/dev/null','-T']:raise SystemExit('unexpected tunnel prefix')
child=subprocess.Popen(command,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,start_new_session=True)
print(json.dumps({'ssh_pid':child.pid,'ssh_process_group':child.pid}),flush=True)
start=time.monotonic();heartbeat=start
try:
 while not stopping and child.poll() is None and time.monotonic()-start<3500 and time.monotonic()-heartbeat<20:
  if select.select([sys.stdin],[],[],.2)[0]:
   line=sys.stdin.readline()
   if not line or line.strip()=='stop':break
   heartbeat=time.monotonic()
finally:
 if child.poll() is None:
  os.killpg(child.pid,signal.SIGTERM)
  try:child.wait(timeout=2)
  except subprocess.TimeoutExpired:os.killpg(child.pid,signal.SIGKILL);child.wait(timeout=2)
'''


def spawn_tunnel(command):
    child = subprocess.Popen([sys.executable, '-u', '-c', TUNNEL], stdin=subprocess.PIPE,
                             stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                             text=True, start_new_session=True)
    child.stdin.write(json.dumps(command) + '\n')
    child.stdin.flush()
    if not select.select([child.stdout], [], [], 5)[0]:
        stop_group(child)
        raise RuntimeError('tunnel supervisor metadata unavailable')
    child.tunnel_state = json.loads(child.stdout.readline())
    return child


def heartbeat_tunnel(child):
    child.stdin.write('heartbeat\n')
    child.stdin.flush()


HOST = r'''
import base64,hashlib,http.client,json,os,pathlib,platform,select,shutil,signal,socket,subprocess,sys,tempfile,time
stopping=False

def stop(*args):
 global stopping
 stopping=True

for sig in (signal.SIGINT,signal.SIGTERM,signal.SIGHUP):signal.signal(sig,stop)
c=json.loads(sys.stdin.readline())
r=pathlib.Path(tempfile.mkdtemp(prefix="inferno-backup-qualification-"));r.chmod(0o700)
helper=None;group=None;errors=[];runtime={}

def members():
 if group is None:return []
 # Reap the parent before inspecting its group: macOS can report a zombie
 # in ps while refusing to signal a group containing only zombies.
 if helper is not None:helper.poll()
 result=[]
 for line in subprocess.check_output(['ps','-axo','pid=,pgid=,comm='],text=True).splitlines():
  a=line.strip().split(None,2)
  if len(a)==3 and int(a[1])==group:result.append((int(a[0]),a[2]))
 return result

def emit(row):
 print(json.dumps(row),flush=True)

try:
 if platform.machine()!='arm64':raise RuntimeError('not Apple Silicon')
 for name in ['cua_node/bin/node','native/devicecheck.node']:
  path=pathlib.Path('/Applications/ChatGPT.app/Contents/Resources')/name
  runtime[name]=hashlib.sha256(path.read_bytes()).hexdigest()
 if runtime!=c['runtime_sha']:raise RuntimeError('runtime changed since preflight')
 binary=r/'helper';data=base64.b64decode(c['binary'],validate=True)
 if hashlib.sha256(data).hexdigest()!=c['binary_sha']:raise RuntimeError('binary identity mismatch')
 binary.write_bytes(data);binary.chmod(0o700)
 token=r/'token';token.write_text(c['token']);token.chmod(0o600)
 with socket.socket() as s:s.bind(('127.0.0.1',19485))
 helper=subprocess.Popen([str(binary),'-listen','127.0.0.1:19485','-token-file',str(token),'-max-concurrent','16'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,start_new_session=True)
 group=helper.pid
 ready=False;start=time.monotonic();heartbeat=start
 while not stopping and time.monotonic()-start<3600 and time.monotonic()-heartbeat<20 and helper.poll() is None:
  if not ready:
   conn=http.client.HTTPConnection('127.0.0.1',19485,timeout=.5)
   try:
    conn.request('GET','/health',headers={'Authorization':'Bearer '+c['token']})
    resp=conn.getresponse();body=resp.read(20481)
    ready=resp.status==200 and len(body)<=20480 and json.loads(body).get('supported') is True
   except (OSError,ValueError,http.client.HTTPException):pass
   finally:conn.close()
   if ready:emit({'ready':True,'host':c['name'],'helper_pid':group,'temp_root':str(r),'binary_sha':c['binary_sha'],'runtime_sha':runtime})
  if select.select([sys.stdin],[],[],.2)[0]:
   line=sys.stdin.readline()
   if not line or line.strip()=='stop':break
   heartbeat=time.monotonic()
except Exception as exc:
 errors.append(type(exc).__name__)
finally:
 remaining=members()
 allowed=[str(r/'helper'),'/Applications/ChatGPT.app/Contents/Resources/cua_node/bin/node','/usr/bin/osascript']
 if remaining and all(path in allowed for pid,path in remaining):
  try:os.killpg(group,signal.SIGTERM)
  except ProcessLookupError:pass
  deadline=time.monotonic()+3
  while members() and time.monotonic()<deadline:time.sleep(.05)
  if members():
   try:os.killpg(group,signal.SIGKILL)
   except ProcessLookupError:pass
   except PermissionError:
    if members():errors.append('SignalPermissionDenied')
 elif remaining:errors.append('UnexpectedProcessGroupMember')
 if helper:
  try:helper.wait(timeout=5)
  except subprocess.TimeoutExpired:errors.append('HelperDidNotExit')
 deadline=time.monotonic()+3
 while members() and time.monotonic()<deadline:time.sleep(.05)
 gone=not members()
 shutil.rmtree(r)
 emit({'cleaned':True,'host':c['name'],'helper_group_gone':gone,'temp_removed':not r.exists(),'cleanup_errors':errors})
'''


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    info = private.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        raise ValueError('private directory must be owned and mode0700')
    prefix = ssh_prefix(private)
    identities_path = private / 'helper-approved-identities.json'
    owned_file(identities_path, 0o600)
    identities = json.loads(identities_path.read_text())
    for port in (19485, 19486):
        port_free(port)
    binary_data = (private / 'attestation-helper').read_bytes()
    binary = base64.b64encode(binary_data).decode()
    binary_sha = hashlib.sha256(binary_data).hexdigest()
    if binary_sha != identities['binary_sha']:
        raise ValueError('helper changed since preflight')
    tokens = {name: secrets.token_urlsafe(32) for name in ('this-mac', 'other-pro')}
    token_path = private / 'backend-tokens.json'
    if token_path.exists() or (private / 'active-helper-receipt-path').exists():
        raise RuntimeError('stale candidate artifacts: run backup_cleanup.py first')
    fd = os.open(token_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(tokens, f)
    children, readers, receipts, cleanup_errors = [], [], [], []
    tunnel = None
    lock, stopping = threading.Lock(), threading.Event()
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, lambda *_: stopping.set())
    run_id = str(time.time_ns())
    receipt_path = root / ('helper-receipts-' + run_id + '.json')

    def reader(child):
        for line in child.stdout:
            try:
                row = json.loads(line)
            except ValueError:
                continue
            with lock:
                receipts.append(row)
                receipt_path.write_text(json.dumps(receipts, indent=2))
                receipt_path.chmod(0o600)
                pointer = private / 'active-helper-receipt-path'
                pointer.write_text(str(receipt_path))
                pointer.chmod(0o600)
            print(json.dumps(row), flush=True)

    try:
        for name in tokens:
            command = [sys.executable, '-u', '-c', HOST] if name == 'this-mac' else prefix + [OTHER, 'python3 -u -c ' + shlex.quote(HOST)]
            child = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, text=True, start_new_session=True)
            children.append(child)
            thread = threading.Thread(target=reader, args=(child,), daemon=True)
            readers.append(thread)
            thread.start()
            child.stdin.write(json.dumps({'name': name, 'binary': binary, 'binary_sha': binary_sha,
                                         'runtime_sha': identities['runtime'][name], 'token': tokens[name]}) + '\n')
            child.stdin.flush()
        start = time.monotonic()
        while time.monotonic() - start < 30 and not stopping.is_set():
            for child in children:
                if child.poll() is not None:
                    raise RuntimeError('qualification helper exited')
                child.stdin.write('heartbeat\n')
                child.stdin.flush()
            if len([x for x in receipts if x.get('ready')]) == 2:
                break
            time.sleep(1)
        else:
            raise RuntimeError('helpers did not become ready')
        tunnel = spawn_tunnel(prefix + ['-N', '-o', 'ExitOnForwardFailure=yes', '-L',
                                       '127.0.0.1:19486:127.0.0.1:19485', OTHER])
        print(json.dumps({'candidate_tunnel_pid': tunnel.pid, 'run_id': run_id}), flush=True)
        deadline = time.monotonic() + 10
        while True:
            try:
                health = [authenticated_health(port, tokens[name]) for port, name in ((19485, 'this-mac'), (19486, 'other-pro'))]
                if all(row['supported'] for row in health):
                    print(json.dumps({'authenticated_candidate_paths': health}), flush=True)
                    break
            except (OSError, ValueError, http.client.HTTPException):
                pass
            if time.monotonic() > deadline:
                raise RuntimeError('candidate tunnel authentication failed')
            for child in children:
                child.stdin.write('heartbeat\n')
                child.stdin.flush()
            time.sleep(.2)
        while time.monotonic() - start < 3500 and not stopping.is_set() and not (private / 'stop-helpers').exists():
            if tunnel.poll() is not None:
                raise RuntimeError('candidate tunnel exited')
            heartbeat_tunnel(tunnel)
            for child in children:
                if child.poll() is not None:
                    raise RuntimeError('qualification helper exited')
                child.stdin.write('heartbeat\n')
                child.stdin.flush()
            time.sleep(2)
    finally:
        if tunnel:
            try:
                stop_group(tunnel)
            except (OSError, subprocess.TimeoutExpired) as exc:
                cleanup_errors.append(type(exc).__name__)
        for child in children:
            try:
                child.stdin.close()
                child.wait(timeout=12)
            except (OSError, subprocess.TimeoutExpired):
                try:
                    stop_group(child)
                except (OSError, subprocess.TimeoutExpired) as exc:
                    cleanup_errors.append(type(exc).__name__)
        for reader_thread in readers:
            reader_thread.join(timeout=3)
        token_path.unlink(missing_ok=True)
        cleaned = [x for x in receipts if x.get('cleaned')]
        verified = len(cleaned) == 2 and all(x.get('helper_group_gone') and x.get('temp_removed') and not x.get('cleanup_errors') for x in cleaned)
        row = {'controller_cleaned': verified and not cleanup_errors, 'cleanup_errors': cleanup_errors,
               'token_removed': not token_path.exists(), 'run_id': run_id}
        cleanup_path = root / ('controller-cleanup-' + run_id + '.json')
        cleanup_path.write_text(json.dumps(row))
        cleanup_path.chmod(0o600)
        print(json.dumps(row), flush=True)


if __name__ == '__main__':
    main()
