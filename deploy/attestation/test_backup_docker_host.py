import json
import os
import pathlib
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

from backup_docker_host import Host, Runner, Stopped, validate_root, sql_string, docker_environment, missing_object


class DockerActorTests(unittest.TestCase):
    def fixture(self):
        folder = tempfile.TemporaryDirectory(prefix='inferno-backup-live-', dir='/tmp')
        self.addCleanup(folder.cleanup)
        root = pathlib.Path(folder.name)
        root.chmod(0o700)
        manifest = root / 'manifest.json'
        manifest.write_text(json.dumps({'root': str(root), 'run_id': '012345abcdef'}))
        manifest.chmod(0o600)
        return root

    def test_reaped_group_permission_race_is_not_a_cleanup_failure(self):
        from unittest.mock import Mock, patch
        child = Mock(pid=12345)
        child.poll.side_effect = [None, 0]
        with patch('backup_docker_host.os.killpg', side_effect=PermissionError):
            Runner.terminate(child)

    def test_live_group_permission_denial_is_not_hidden(self):
        from unittest.mock import Mock, patch
        child = Mock(pid=12345)
        child.poll.return_value = None
        with patch('backup_docker_host.os.killpg', side_effect=PermissionError):
            with self.assertRaises(PermissionError):Runner.terminate(child)

    def test_root_manifest_identity(self):
        root = self.fixture()
        self.assertEqual(validate_root(root, '012345abcdef')['root'], str(root))

    def test_root_wrong_identity_rejected(self):
        root = self.fixture()
        with self.assertRaises(ValueError):
            validate_root(root, 'aaaaaaaaaaaa')

    def test_root_extra_content_prevents_deletion(self):
        root = self.fixture()
        (root / 'unrelated.txt').write_text('do not remove')
        with self.assertRaises(ValueError):
            validate_root(root, '012345abcdef')

    def test_symlink_asset_rejected(self):
        root = self.fixture()
        (root / 'config.yaml').symlink_to(root / 'manifest.json')
        with self.assertRaises(ValueError):
            validate_root(root, '012345abcdef')

    def test_runner_stops_blocked_command_without_long_lock(self):
        runner = Runner()
        outcomes = []
        def run():
            try:
                runner.run([sys.executable, '-c', 'import time; time.sleep(60)'], timeout=120)
            except Exception as exc:
                outcomes.append(type(exc).__name__)
        thread = threading.Thread(target=run)
        thread.start()
        deadline = time.monotonic() + 2
        while not runner.children and time.monotonic() < deadline:
            time.sleep(.01)
        start = time.monotonic()
        runner.stop()
        thread.join(timeout=2)
        self.assertFalse(thread.is_alive())
        self.assertLess(time.monotonic() - start, 2)
        self.assertTrue(outcomes)
        self.assertTrue(all(child.poll() is not None for child in runner.children))

    def test_stopped_runner_cannot_start_new_mutation(self):
        runner = Runner()
        runner.stop()
        with self.assertRaises(Stopped):
            runner.run([sys.executable, '-c', 'raise SystemExit(0)'])

    def test_foreign_docker_ownership_not_accepted(self):
        host = Host(self.fixture(), '012345abcdef')
        self.assertFalse(host.owned({'inferno.backup.run': 'other'}))
        self.assertTrue(host.owned({'inferno.backup.run': host.run_id, 'inferno.backup.root': str(host.root)}))

    def test_sql_literal_quoting(self):
        self.assertEqual(sql_string("can't"), "'can''t'")

    def test_missing_network_is_not_daemon_failure(self):
        self.assertTrue(missing_object('network', 'candidate', b'Error response from daemon: network candidate not found'))
        self.assertTrue(missing_object('volume', 'candidate', b'Error response from daemon: get candidate: no such volume'))
        self.assertFalse(missing_object('network', 'candidate', b'Cannot connect to Docker daemon'))
        self.assertFalse(missing_object('network', 'candidate', b'network unrelated not found'))

    def test_docker_environment_drops_remote_context_and_tls(self):
        with patch.dict(os.environ, {'DOCKER_HOST': 'ssh://prod', 'DOCKER_CONTEXT': 'prod',
                                    'DOCKER_TLS_VERIFY': '1', 'DOCKER_CERT_PATH': '/private', 'HTTP_PROXY': 'http://proxy'}):
            env = docker_environment()
        self.assertFalse(any(key.startswith('DOCKER_') for key in env))
        self.assertNotIn('HTTP_PROXY', env)

    def test_nonlocal_daemon_is_rejected_before_cli(self):
        host = Host(self.fixture(), '012345abcdef', daemon={'endpoint': 'ssh://production', 'id': 'fake'})
        with patch('backup_docker_host.subprocess.run') as command:
            with self.assertRaises(ValueError):
                host.pin_daemon()
            command.assert_not_called()

    def test_failed_cleanup_can_retry(self):
        host = Host(self.fixture(), '012345abcdef', emit=lambda *a, **k: None)
        host.pin_daemon = lambda force=False: None
        host.inspect = lambda *a: (_ for _ in ()).throw(RuntimeError('temporary unavailable'))
        self.assertFalse(host.cleanup()['candidate_live_cleaned'])
        host.inspect = lambda *a: None
        self.assertTrue(host.cleanup()['candidate_live_cleaned'])


if __name__ == '__main__':
    unittest.main()
