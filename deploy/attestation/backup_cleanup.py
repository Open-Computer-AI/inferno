#!/usr/bin/env python3
"""Independent verifier for EOF/watchdog teardown, including a hard-killed controller."""
import json
import pathlib
import re
import hashlib
import os
import shutil
import subprocess
import sys
import time

from backup_helpers import OTHER, owned_file, port_free, ssh_prefix


def group_gone(pid):
    rows = subprocess.check_output(['ps', '-axo', 'pgid='], text=True).split()
    return str(pid) not in rows


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    from backup_phase_cleanup import stop_marker
    stop_marker(private / 'stop-helpers')
    pointer = private / 'active-helper-receipt-path'
    owned_file(pointer, 0o600)
    receipt = pathlib.Path(pointer.read_text()).resolve()
    if receipt.parent != root or not re.fullmatch(r'helper-receipts-[0-9]+\.json', receipt.name):
        raise ValueError('unexpected candidate receipt')
    owned_file(receipt, 0o600)
    ready = {x['host']: x for x in json.loads(receipt.read_text()) if x.get('ready')}
    if set(ready) != {'this-mac', 'other-pro'}:
        raise ValueError('both candidate identities required')
    for row in ready.values():
        if not pathlib.Path(row['temp_root']).name.startswith('inferno-backup-qualification-'):
            raise ValueError('unexpected candidate root')
    deadline = time.monotonic() + 30
    result = {}
    while time.monotonic() < deadline:
        local = ready['this-mac']
        result = {'local_group_gone': group_gone(local['helper_pid']),
                  'local_root_removed': not pathlib.Path(local['temp_root']).exists()}
        try:
            for port in (19485, 19486):
                port_free(port)
            result['local_ports_closed'] = True
        except OSError:
            result['local_ports_closed'] = False
        local_root = pathlib.Path(local['temp_root'])
        if result['local_group_gone'] and result['local_ports_closed'] and local_root.exists():
            info = local_root.lstat()
            if (info.st_uid == os.getuid() and not local_root.is_symlink()
                    and set(x.name for x in local_root.iterdir()) <= {'helper', 'token'}
                    and hashlib.sha256((local_root / 'helper').read_bytes()).hexdigest() == local['binary_sha']):
                shutil.rmtree(local_root)
                result['local_root_removed'] = not local_root.exists()
        other = ready['other-pro']
        program = """import hashlib,json,os,pathlib,shutil,socket,subprocess
groups=subprocess.check_output(['ps','-axo','pgid='],text=True).split()
s=socket.socket();s.settimeout(.2);closed=s.connect_ex(('127.0.0.1',19485))!=0;s.close()
root=pathlib.Path(ROOT)
gone=str(PID) not in groups
if gone and closed and root.exists():
 info=root.lstat()
 if info.st_uid==os.getuid() and not root.is_symlink() and set(x.name for x in root.iterdir())<={'helper','token'} and hashlib.sha256((root/'helper').read_bytes()).hexdigest()==SHA:
  shutil.rmtree(root)
print(json.dumps({'remote_group_gone':gone,'remote_root_removed':not root.exists(),'remote_port_closed':closed}))
""".replace('PID', repr(other['helper_pid'])).replace('ROOT', repr(other['temp_root'])).replace('SHA', repr(other['binary_sha']))
        check = subprocess.run(ssh_prefix(private) + [OTHER, 'python3 -'], input=program,
                               capture_output=True, text=True, timeout=15)
        if check.returncode == 0:
            try:
                result.update(json.loads(check.stdout))
            except json.JSONDecodeError:
                result['remote_verified'] = False
        else:
            result['remote_verified'] = False
        if all(result.values()):
            (private / 'backend-tokens.json').unlink(missing_ok=True)
            pointer.unlink()
            result['token_removed'] = not (private / 'backend-tokens.json').exists()
            break
        time.sleep(1)
    path = root / ('independent-cleanup-' + str(time.time_ns()) + '.json')
    path.write_text(json.dumps(result, indent=2))
    path.chmod(0o600)
    print(json.dumps(result))
    if not result.get('token_removed') or not all(result.values()):
        raise SystemExit(1)


if __name__ == '__main__':
    main()
