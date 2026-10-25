import concurrent.futures
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import subprocess
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location(
    "rollback_guard", Path(__file__).resolve().parents[1] / "rollback_guard.py"
)
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class RollbackGuardTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.home = Path(self.temporary.name)
        self.state = self.home / "Library/Application Support/InfernoAttestationAWS/test"
        self.state.mkdir(parents=True)
        self.label = "com.opencomputer.inferno-attestation-aws-tunnel-candidate"
        self.target = self.home / "Library/LaunchAgents" / (self.label + ".plist")
        self.target.parent.mkdir(parents=True)
        arguments = ["/usr/bin/ssh", "-NT", "ubuntu@100.90.154.71"]
        self.backup = plistlib.dumps({"Label": self.label, "value": "original", "ProgramArguments": arguments})
        self.candidate = plistlib.dumps({"Label": self.label, "value": "candidate", "ProgramArguments": arguments})
        self.target.write_bytes(self.candidate)
        (self.state / "backup.plist").write_bytes(self.backup)
        (self.state / "backup.plist").chmod(0o600)
        self.config = {
            "label": self.label,
            "backup_sha256": hashlib.sha256(self.backup).hexdigest(),
            "candidate_sha256": hashlib.sha256(self.candidate).hexdigest(),
            "delay_seconds": 0,
        }
        self.write_config()
        self.home_patch = mock.patch.object(guard.Path, "home", return_value=self.home)
        self.home_patch.start()
        self.addCleanup(self.home_patch.stop)
        self.run_patch = mock.patch.object(guard.subprocess, "run")
        self.run = self.run_patch.start()
        self.addCleanup(self.run_patch.stop)
        self.run.return_value.returncode = 0

    def write_config(self):
        (self.state / "config.json").write_text(json.dumps(self.config))

    def test_timeout_restores_exact_bytes_and_only_expected_job(self):
        self.assertEqual(guard.guard(self.state)["outcome"], "rolled_back")
        self.assertEqual(self.target.read_bytes(), self.backup)
        self.assertEqual(self.target.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.run.call_args_list[0].args[0], [
            "launchctl", "bootout", "gui/{}/{}".format(os.getuid(), self.label)
        ])
        self.assertEqual(self.run.call_args_list[1].args[0], [
            "launchctl", "bootstrap", "gui/{}".format(os.getuid()), str(self.target)
        ])

    def test_confirmation_prevents_rollback(self):
        self.assertTrue(guard.claim_decision(self.state, "confirm"))
        self.assertEqual(guard.guard(self.state)["outcome"], "decision_already_claimed")
        self.assertEqual(self.target.read_bytes(), self.candidate)
        self.run.assert_not_called()

    def test_only_one_concurrent_decision_can_win(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(
                lambda _: guard.claim_decision(self.state, "confirm"), range(16)
            ))
        self.assertEqual(sum(results), 1)

    def test_confirmation_cannot_override_claimed_rollback(self):
        self.assertTrue(guard.claim_decision(self.state, "rollback"))
        self.assertFalse(guard.claim_decision(self.state, "confirm"))

    def test_external_target_edit_is_preserved(self):
        self.target.write_bytes(b"another operator changed this file")
        with self.assertRaises(ValueError):
            guard.restore(self.state)
        self.assertEqual(self.target.read_bytes(), b"another operator changed this file")
        self.run.assert_not_called()

    def test_corrupt_backup_does_not_unload_job(self):
        (self.state / "backup.plist").write_bytes(self.candidate)
        with self.assertRaises(ValueError):
            guard.restore(self.state)
        self.run.assert_not_called()

    def test_unrelated_label_is_rejected(self):
        self.config["label"] = "com.opencomputer.inferno-attestation-aws"
        self.write_config()
        with self.assertRaises(ValueError):
            guard.restore(self.state)
        self.run.assert_not_called()

    def test_failed_unload_preserves_active_file(self):
        self.run.side_effect = [mock.Mock(returncode=5), mock.Mock(returncode=0)]
        with self.assertRaises(RuntimeError):
            guard.restore(self.state)
        self.assertEqual(self.target.read_bytes(), self.candidate)

    def test_missing_job_is_restored(self):
        self.run.side_effect = [mock.Mock(returncode=3), mock.Mock(returncode=3), mock.Mock(returncode=0)]
        self.assertEqual(guard.restore(self.state)["outcome"], "rolled_back")
        self.assertEqual(self.target.read_bytes(), self.backup)

    def test_state_outside_runtime_is_rejected(self):
        with self.assertRaises(ValueError):
            guard.restore(self.home)
        self.run.assert_not_called()

    def test_bootstrap_failure_restores_file_but_does_not_report_success(self):
        self.run.side_effect = [
            mock.Mock(returncode=0),
            subprocess.CalledProcessError(5, ["launchctl", "bootstrap"]),
        ]
        with mock.patch.object(guard.sys, "argv", ["rollback_guard.py", "guard", str(self.state)]):
            self.assertEqual(guard.main(), 1)
        self.assertEqual(self.target.read_bytes(), self.backup)
        self.assertEqual(json.loads((self.state / "result.json").read_text()), {
            "outcome": "failed", "error_class": "CalledProcessError"
        })

    def test_transient_launchd_bootstrap_delay_is_retried(self):
        self.run.side_effect = [
            mock.Mock(returncode=0),
            mock.Mock(returncode=5),
            mock.Mock(returncode=113),
            mock.Mock(returncode=0),
        ]
        with mock.patch.object(guard.time, "sleep"):
            self.assertEqual(guard.restore(self.state)["outcome"], "rolled_back")
        self.assertEqual(self.run.call_count, 4)

    def test_already_loaded_different_arguments_are_not_accepted(self):
        self.run.side_effect = [
            mock.Mock(returncode=0),
            mock.Mock(returncode=5),
            mock.Mock(returncode=0, stdout=b"arguments = {\nwrong-job\n}"),
        ]
        with self.assertRaises(RuntimeError):
            guard.restore(self.state)

    def test_already_loaded_matching_arguments_are_accepted(self):
        self.run.side_effect = [
            mock.Mock(returncode=0),
            mock.Mock(returncode=5),
            mock.Mock(returncode=0, stdout=b"arguments = {\n/usr/bin/ssh\n-NT\nubuntu@100.90.154.71\n}"),
        ]
        self.assertEqual(guard.restore(self.state)["outcome"], "rolled_back")


if __name__ == "__main__":
    unittest.main()
