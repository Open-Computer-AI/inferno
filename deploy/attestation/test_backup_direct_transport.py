import base64
import hashlib
import json
import pathlib
import subprocess
import unittest
from unittest.mock import patch, Mock

import backup_direct_transport as direct


class DirectTransportTests(unittest.TestCase):
    def test_snapshot_executable_mode_is_prepared_after_hash_check(self):
        import tempfile
        from backup_live_prepare import prepare_executable
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder) / 'server'
            path.write_bytes(b'fixture executable')
            path.chmod(0o600)
            prepare_executable(path, hashlib.sha256(path.read_bytes()).hexdigest())
            self.assertEqual(path.stat().st_mode & 0o777, 0o700)

    def test_wrong_binary_is_never_made_executable(self):
        import tempfile
        from backup_live_prepare import prepare_executable
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder) / 'server'
            path.write_bytes(b'wrong fixture')
            path.chmod(0o600)
            with self.assertRaises(ValueError):prepare_executable(path, '0' * 64)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_cumulative_cpu_time_formats(self):
        from backup_soak_monitor import cpu_seconds
        self.assertAlmostEqual(cpu_seconds('0:03.25'), 3.25)
        self.assertAlmostEqual(cpu_seconds('1:02:03.5'), 3723.5)

    def valid_host(self):
        key = b'fixture-public-host-key'
        fingerprint = 'SHA256:' + base64.b64encode(hashlib.sha256(key).digest()).decode().rstrip('=')
        return '100.96.58.114 ssh-ed25519 ' + base64.b64encode(key).decode(), fingerprint

    def test_wrong_host_or_key_rejected(self):
        for line in ('other ssh-ed25519 dGVzdA==', '100.96.58.114 ssh-ed25519 dGVzdA=='):
            with self.assertRaises(ValueError):direct.validate_host(line)

    def test_invalid_run_rejected_before_any_root(self):
        host, fingerprint = self.valid_host()
        with patch.object(direct, 'FINGERPRINT', fingerprint), patch.object(direct.tempfile, 'mkdtemp') as create:
            with self.assertRaises(ValueError):direct.DirectSSH(host, '../bad', 'source')
            create.assert_not_called()

    def test_root_cleanup_without_starting_ssh(self):
        host, fingerprint = self.valid_host()
        with patch.object(direct, 'FINGERPRINT', fingerprint), patch.object(direct, 'verify_source', return_value='source'):
            transport = direct.DirectSSH(host, 'a' * 16, 'source')
            root = transport.root
            with patch.object(transport, 'port_open', return_value=False):transport.close()
            self.assertFalse(root.exists())

    def test_client_cannot_use_agent_key_or_password(self):
        host, fingerprint = self.valid_host()
        child = Mock(pid=12345)
        child.poll.return_value = None
        child.wait.return_value = 0
        with patch.object(direct, 'FINGERPRINT', fingerprint), patch.object(direct, 'verify_source', return_value='source'):
            transport = direct.DirectSSH(host, 'a' * 16, 'source')
            try:
                with patch.object(direct.subprocess, 'Popen', return_value=child) as spawn, patch.object(direct.socket, 'socket'), patch.object(pathlib.Path, 'read_text', return_value='12345 (ssh) ' + ' '.join(['0'] * 20)):
                    state = transport.start()
                command = spawn.call_args.args[0]
                for option in ('IdentityAgent=none', 'IdentitiesOnly=yes', 'PubkeyAuthentication=no', 'PasswordAuthentication=no', 'KbdInteractiveAuthentication=no', 'PreferredAuthentications=none'):
                    self.assertIn(option, command)
                self.assertEqual(command[-1], 'architsakri@100.96.58.114')
                self.assertIn('127.0.0.1:19486:127.0.0.1:19485', command)
                self.assertEqual(state['source_node_fingerprint'], 'source')
            finally:
                transport.child = None
                with patch.object(transport, 'port_open', return_value=False):transport.close()

    def test_control_is_bounded_and_closed_to_unknown_cases(self):
        for value in ('{', '[]', '{"case":"unknown","sequence":0}', '{"case":"direct-recovered","sequence":true}', 'x' * 4097):
            with self.assertRaises((ValueError, TypeError)):direct.parse_control(value)
        self.assertEqual(direct.parse_control('{"case":"direct-recovered","sequence":2}')['sequence'], 2)

    def test_source_identity_and_ip_are_pinned(self):
        node = {'Self': {'ID': 'fixture', 'UserID': 123, 'TailscaleIPs': ['100.90.154.71']}}
        fingerprint = hashlib.sha256(b'fixture:123').hexdigest()
        with patch.object(direct.subprocess, 'check_output', return_value=json.dumps(node).encode()):
            self.assertEqual(direct.verify_source(fingerprint), fingerprint)
            with self.assertRaises(ValueError):direct.verify_source('wrong')
