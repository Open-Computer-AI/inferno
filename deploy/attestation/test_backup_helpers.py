import base64
import ast
import hashlib
import pathlib
import socket
import subprocess
import sys
import tempfile
import unittest
import json
from unittest.mock import patch

import backup_helpers as helpers


class HelperSafetyTests(unittest.TestCase):
    def test_fault_guard_restores_modes_after_caller_killed(self):
        import os
        import signal
        import time
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            private = root / 'private'
            private.mkdir(mode=0o700)
            modes = private / 'fault-mode.json'
            modes.write_text(json.dumps({'this-mac': 'healthy', 'other-pro': 'healthy'}))
            modes.chmod(0o600)
            program = "from backup_modes import mode_guard,set_modes; import pathlib,sys,time\n@mode_guard\ndef run():\n set_modes(pathlib.Path(sys.argv[1])/'private','refuse','healthy'); print('ready',flush=True);time.sleep(60)\nrun()"
            child = subprocess.Popen([sys.executable, '-c', program, str(root)],
                                     cwd=pathlib.Path(__file__).parent, stdout=subprocess.PIPE, text=True)
            self.assertEqual(child.stdout.readline().strip(), 'ready')
            os.kill(child.pid, signal.SIGKILL)
            child.wait(timeout=3)
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline and json.loads(modes.read_text())['this-mac'] != 'healthy':
                time.sleep(.05)
            self.assertEqual(json.loads(modes.read_text()), {'this-mac': 'healthy', 'other-pro': 'healthy'})
            child.stdout.close()

    def test_aws_reference_rejects_injected_ssh_options(self):
        from backup_air_reference import aws_command
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            path = root / 'aws-command.json'
            path.write_text(json.dumps(['ssh', '-o', 'ProxyCommand=echo not-executed', 'ubuntu@100.90.154.71', 'python3 -']))
            path.chmod(0o600)
            with self.assertRaisesRegex(ValueError, 'verified pinned profile'):
                aws_command(root)

    def test_proxy_mode_confirmation_rejects_wrong_run(self):
        from backup_modes import wait_modes
        import time
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            (root / 'live-remote-root.json').write_text(json.dumps({'run_id': 'expected'}))
            status = root / 'fault-status.json'
            status.write_text(json.dumps({'run_id': 'wrong', 'at': time.time(), 'modes': {'this-mac':'healthy','other-pro':'refuse'}}))
            status.chmod(0o600)
            with patch('backup_modes.time.monotonic', side_effect=[0, 0, 4]):
                with self.assertRaisesRegex(RuntimeError, 'not confirmed'):
                    wait_modes(root, 'healthy', 'refuse')

    def test_failed_case_does_not_pass_production_report(self):
        from backup_production_check import production_case
        with tempfile.TemporaryDirectory() as folder:
            @production_case
            def candidate():
                raise RuntimeError('candidate failed')
            with patch('sys.argv', ['test', folder, 'pool-all-down']), patch('backup_production_check.snapshot', return_value={'safe': True}):
                with self.assertRaisesRegex(RuntimeError, 'candidate failed'):
                    candidate()
            evidence = json.loads((pathlib.Path(folder) / 'production-case-pool-all-down.json').read_text())
            self.assertFalse(evidence['passed'])
            self.assertTrue(evidence['production_guard_passed'])

    def test_successful_case_and_guard_are_reported_separately(self):
        from backup_production_check import production_case
        with tempfile.TemporaryDirectory() as folder:
            @production_case
            def candidate():
                return 42
            with patch('sys.argv', ['test', folder, 'pool-all-down']), patch('backup_production_check.snapshot', return_value={'safe': True}):
                self.assertEqual(candidate(), 42)
            evidence = json.loads((pathlib.Path(folder) / 'production-case-pool-all-down.json').read_text())
            self.assertTrue(evidence['passed'])
            self.assertTrue(evidence['case_completed'])
            self.assertTrue(evidence['production_guard_passed'])

    def test_embedded_aws_programs_compile(self):
        from backup_aws_latency import REMOTE as actor
        from backup_aws_verify import REMOTE as verifier
        compile(actor, 'AWS candidate', 'exec')
        compile(verifier, 'AWS independent verifier', 'exec')

    def test_all_candidate_scripts_compile(self):
        for path in pathlib.Path(__file__).parent.glob('backup*.py'):
            with self.subTest(path=path.name):
                ast.parse(path.read_text())

    def test_stop_marker_rejects_symlink(self):
        from backup_phase_cleanup import stop_marker
        with tempfile.TemporaryDirectory() as folder:
            other = pathlib.Path(folder) / 'unrelated'
            other.write_text('preserve')
            link = pathlib.Path(folder) / 'stop-live'
            link.symlink_to(other)
            with self.assertRaises(OSError):
                stop_marker(link)
            self.assertEqual(other.read_text(), 'preserve')
    def test_pinned_command_has_no_inherited_forwards(self):
        with tempfile.TemporaryDirectory() as folder:
            private = pathlib.Path(folder)
            key = b"test-public-host-key"
            (private / "other_known_hosts").write_text("100.96.58.114 ssh-ed25519 " + base64.b64encode(key).decode() + "\n")
            (private / "other_known_hosts").chmod(0o600)
            fingerprint = "SHA256:" + base64.b64encode(hashlib.sha256(key).digest()).decode().rstrip("=")
            with patch.object(helpers, "FINGERPRINT", fingerprint):
                command = helpers.ssh_prefix(private)
            self.assertEqual(command[:4], ["ssh", "-F", "/dev/null", "-T"])
            self.assertNotIn("-L", command)
            self.assertNotIn("-R", command)
            self.assertIn("StrictHostKeyChecking=yes", command)
            self.assertIn("ControlPath=none", command)

    def test_wrong_host_key_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            p = pathlib.Path(folder) / "other_known_hosts"
            p.write_text("100.96.58.114 ssh-ed25519 dGVzdA==\n")
            p.chmod(0o600)
            with self.assertRaisesRegex(ValueError, "host key mismatch"):
                helpers.ssh_prefix(p.parent)

    def test_wrong_hostname_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            p = pathlib.Path(folder) / "other_known_hosts"
            p.write_text("other-host ssh-ed25519 dGVzdA==\n")
            p.chmod(0o600)
            with self.assertRaisesRegex(ValueError, "unexpected host key"):
                helpers.ssh_prefix(p.parent)

    def test_public_private_file_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            p = pathlib.Path(folder) / "private"
            p.write_text("placeholder")
            p.chmod(0o644)
            with self.assertRaises(ValueError):
                helpers.owned_file(p, 0o600)

    def test_symlink_private_file_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            p = pathlib.Path(folder) / "file"
            p.write_text("placeholder")
            p.chmod(0o600)
            link = p.parent / "link"
            link.symlink_to(p)
            with self.assertRaises(ValueError):
                helpers.owned_file(link, 0o600)

    def test_occupied_listener_is_rejected(self):
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            with self.assertRaises(OSError):
                helpers.port_free(sock.getsockname()[1])

    def test_stop_reaps_owned_group(self):
        child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"], start_new_session=True)
        helpers.stop_group(child)
        self.assertIsNotNone(child.poll())

    def test_host_program_compiles(self):
        compile(helpers.HOST, "temporary-host-supervisor", "exec")

    def test_tunnel_program_compiles(self):
        compile(helpers.TUNNEL, "temporary-tunnel-supervisor", "exec")

    def test_tunnel_rejects_arbitrary_executable(self):
        result = subprocess.run([sys.executable, '-c', helpers.TUNNEL],
                                input=json.dumps(['/bin/sleep', '60']) + '\n',
                                capture_output=True, text=True, timeout=2)
        self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
