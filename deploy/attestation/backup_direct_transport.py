"""Temporary AWS-origin SSH forward to the pinned Other-Pro native helper."""
import base64
import hashlib
import json
import os
import pathlib
import re
import shutil
import signal
import socket
import stat
import subprocess
import tempfile

FINGERPRINT = 'SHA256:VNpevfyqwYHSzO/HgU7d8GVNl2dsqxa+tPcOjF0NMJI'


def validate_host(line):
    fields = line.strip().split()
    if len(fields) != 3 or fields[:2] != ['100.96.58.114', 'ssh-ed25519']:
        raise ValueError('unexpected direct SSH host')
    fingerprint = 'SHA256:' + base64.b64encode(hashlib.sha256(base64.b64decode(fields[2], validate=True)).digest()).decode().rstrip('=')
    if fingerprint != FINGERPRINT:
        raise ValueError('direct SSH host key mismatch')
    return ' '.join(fields) + '\n'


CASES = {'direct-other-healthy', 'direct-tunnel-down-air', 'direct-two-down-this',
         'direct-all-down', 'direct-recovered', 'direct-air-healthy',
         'direct-air-down-this', 'direct-air-this-down-other'}


def parse_control(line):
    if len(line) > 4096:
        raise ValueError('oversized direct control')
    value = json.loads(line)
    if (not isinstance(value, dict) or set(value) != {'case', 'sequence'}
            or value['case'] not in CASES or type(value['sequence']) is not int or value['sequence'] < 0):
        raise ValueError('invalid direct control')
    return value


def verify_source(expected):
    node = json.loads(subprocess.check_output(['tailscale', 'status', '--json']))['Self']
    fingerprint = hashlib.sha256((str(node['ID']) + ':' + str(node['UserID'])).encode()).hexdigest()
    if fingerprint != expected or '100.90.154.71' not in node['TailscaleIPs']:
        raise ValueError('direct SSH source node identity changed')
    return fingerprint


class DirectSSH:
    def __init__(self, known_host, run_id, source_fingerprint):
        line = validate_host(known_host)
        if not re.fullmatch('[a-f0-9]{16}', run_id):
            raise ValueError('unexpected direct SSH run identity')
        self.source_fingerprint = verify_source(source_fingerprint)
        self.root = pathlib.Path(tempfile.mkdtemp(prefix='inferno-backup-direct-'))
        self.root.chmod(0o700)
        self.known = self.root / 'known_hosts'
        try:
            self.known.write_text(line)
            self.known.chmod(0o600)
        except BaseException:
            if set(x.name for x in self.root.iterdir()) <= {'known_hosts'}:shutil.rmtree(self.root)
            raise
        self.child = None
        self.run_id = run_id

    def start(self):
        self.stop()
        with socket.socket() as sock:
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            sock.bind(('127.0.0.1', 19486))
        args = ['ssh', '-F', '/dev/null', '-T', '-o', 'BatchMode=yes', '-o',
                'IdentityAgent=none', '-o', 'IdentitiesOnly=yes', '-o', 'PubkeyAuthentication=no', '-o',
                'PasswordAuthentication=no', '-o', 'KbdInteractiveAuthentication=no', '-o',
                'GSSAPIAuthentication=no', '-o', 'HostbasedAuthentication=no', '-o',
                'PreferredAuthentications=none', '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + str(self.known),
                '-o', 'ConnectTimeout=8', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
                '-o', 'ExitOnForwardFailure=yes', '-o', 'ServerAliveInterval=5', '-o',
                'ServerAliveCountMax=2', '-N', '-L', '127.0.0.1:19486:127.0.0.1:19485',
                'architsakri@100.96.58.114']
        self.child = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                      stderr=subprocess.DEVNULL, start_new_session=True)
        pid = self.child.pid
        ticks = pathlib.Path('/proc', str(pid), 'stat').read_text().rsplit(')', 1)[1].split()[19]
        return {'aws_direct_other_ssh_pid': pid, 'start_ticks': ticks,
                'known_hosts_root': str(self.root), 'run_id': self.run_id, 'source_node_fingerprint': self.source_fingerprint}

    @staticmethod
    def port_open():
        with socket.socket() as sock:
            sock.settimeout(.2)
            return sock.connect_ex(('127.0.0.1', 19486)) == 0

    def stop(self):
        if self.child and self.child.poll() is None:
            os.killpg(self.child.pid, signal.SIGTERM)
            try:
                self.child.wait(timeout=2)
            except subprocess.TimeoutExpired:
                os.killpg(self.child.pid, signal.SIGKILL)
                self.child.wait(timeout=2)

    def close(self):
        self.stop()
        if self.port_open():raise RuntimeError('direct SSH listener remained open')
        info = self.root.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
                or stat.S_IMODE(info.st_mode) != 0o700 or set(x.name for x in self.root.iterdir()) != {'known_hosts'}):
            raise RuntimeError('direct SSH cleanup root identity changed')
        info = self.known.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise RuntimeError('direct SSH cleanup file identity changed')
        validate_host(self.known.read_text())
        shutil.rmtree(self.root)
