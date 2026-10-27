#!/usr/bin/env python3
"""Independent remote Docker/root and local/remote forward teardown verification."""
import json
import pathlib
import re
import shlex
import socket
import subprocess
import sys
import time

from backup_helpers import OTHER, owned_file, ssh_prefix


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    owned_file(private / 'live-remote-root.json', 0o600)
    owned_file(private / 'live-manifest.json', 0o600)
    state = json.loads((private / 'live-remote-root.json').read_text())
    manifest = json.loads((private / 'live-manifest.json').read_text())
    remote, run_id = state['root'], state['run_id']
    if manifest['root'] != remote or manifest['run_id'] != run_id or not re.fullmatch('[a-f0-9]{12}', run_id):
        raise ValueError('isolated verifier identity mismatch')
    owned_file(private / 'live-processes.json', 0o600)
    processes = json.loads((private / 'live-processes.json').read_text())
    if processes['run_id'] != run_id or processes['remote_root'] != remote:
        raise ValueError('candidate process receipt identity mismatch')
    source = pathlib.Path(__file__).with_name('backup_docker_host.py').read_text()
    source = source.rsplit("if __name__ == '__main__':", 1)[0]
    source += "\nimport socket\nroot=pathlib.Path(sys.argv[1]);run_id=sys.argv[2];host=Host(root,run_id,daemon=" + repr(manifest['docker']) + ")\n"
    source += "objects=[('container',name) for name in host.names.values()]+[('network',host.network),('volume',host.volume)]\n"
    source += "result={'remote_root_removed':not root.exists()}\n"
    source += "try:\n host.pin_daemon(force=True)\n result['docker_objects_absent']=all(host.inspect(kind,name) is None for kind,name in objects)\nexcept Exception:result['docker_verified']=False\n"
    source += "with socket.socket() as s:\n s.settimeout(.2);result['remote_forward_closed']=s.connect_ex(('127.0.0.1',19495))!=0\nprint(json.dumps(result))\n"
    deadline = time.monotonic() + 65
    final = {}
    while time.monotonic() < deadline:
        command = 'python3 -c ' + shlex.quote(source) + ' ' + shlex.quote(remote) + ' ' + run_id
        check = subprocess.run(ssh_prefix(private) + [OTHER, command], capture_output=True, text=True, timeout=20)
        final = json.loads(check.stdout) if check.returncode == 0 else {'remote_verified': False}
        with socket.socket() as sock:
            sock.settimeout(.2)
            final['local_forward_closed'] = sock.connect_ex(('127.0.0.1', 19480)) != 0
        groups = set(subprocess.check_output(['ps', '-axo', 'pgid='], text=True).split())
        final['local_actor_groups_gone'] = all(str(processes[key]) not in groups
                                               for key in ('remote_actor_ssh_pid', 'tunnel_supervisor_pid', 'tunnel_ssh_pid'))
        if all(final.values()):
            break
        time.sleep(1)
    final['passed'] = all(final.values())
    final['run_id'] = run_id
    target = root / ('live-independent-cleanup-' + run_id + '.json')
    target.write_text(json.dumps(final, indent=2))
    target.chmod(0o600)
    print(json.dumps(final))
    if not final['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
