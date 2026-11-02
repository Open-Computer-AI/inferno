import importlib.util
import json
import pathlib
import tempfile
import threading
import subprocess
import sys
import time
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "capacity_probe", pathlib.Path(__file__).resolve().parents[1] / "capacity_probe.py")
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


class CapacityProbeTests(unittest.TestCase):
    def sample(self, **updates):
        value = {"observed_at": 100, "sequence": 1, "host": "100.70.23.11",
                 "pressure": 1, "cpu_busy_percent": 20, "swapouts_delta": 0,
                 "thermal_warning": False, "production_ok": True}
        return dict(value, **updates)

    def test_normal_resources(self):
        self.assertIsNone(probe.resource_problem(self.sample(), 101))

    def test_resource_stops(self):
        cases = [(dict(pressure=2), "air_memory_pressure"),
                 (dict(observed_at=94), "air_telemetry_stale"),
                 (dict(observed_at=102), "air_telemetry_stale"),
                 (dict(cpu_busy_percent=81), "air_cpu_headroom"),
                 (dict(swapouts_delta=1), "air_swap_activity"),
                 (dict(thermal_warning=True), "air_thermal_warning"),
                 (dict(production_ok=False), "production_probe_failed"),
                 (dict(host="AWS"), "air_telemetry_invalid"),
                 (dict(cpu_busy_percent=float("nan")), "air_telemetry_invalid"),
                 (dict(cpu_busy_percent=-1), "air_telemetry_invalid")]
        for update, reason in cases:
            with self.subTest(update=update):
                self.assertEqual(probe.resource_problem(self.sample(**update), 101), reason)

    def test_proof_validation(self):
        self.assertTrue(probe.valid_proof({"attestation": '{"v":1,"s":0,"t":"v1.fake"}'}))
        self.assertFalse(probe.valid_proof({"attestation": '{"v":true,"s":0,"t":"v1.fake"}'}))
        self.assertFalse(probe.valid_proof({"attestation": '{"v":1,"s":1,"t":"v1.fake"}'}))

    def test_quantiles(self):
        self.assertIsNone(probe.percentile([], .99))
        self.assertEqual(probe.percentile(range(1, 101), .95), 95)

    def test_production_ports_rejected(self):
        for port in (19085, 19095, 19385):
            with self.assertRaises(ValueError):
                probe.Probe(port, "fake", 32, 300, pathlib.Path("unused"), pathlib.Path("unused"))

    def test_bad_telemetry_never_starts_load(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            runner = probe.Probe(19395, "fake", 32, 300, path / "missing", path / "stop")
            result = runner.run()
            self.assertFalse(result["passed"])
            self.assertEqual(result["completed"], 0)
            self.assertEqual(result["abort_reason"], "air_telemetry_invalid")

    def test_stop_and_sequence_regression(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            telemetry = path / "telemetry"
            telemetry.touch(mode=0o600)
            runner = probe.Probe(19395, "fake", 32, 300, telemetry, path / "stop")
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time(), sequence=2)))
            self.assertIsNone(runner.check_resources())
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time(), sequence=1)))
            self.assertEqual(runner.check_resources(), "air_telemetry_regressed")
            (path / "stop").touch()
            self.assertEqual(runner.check_resources(), "controller_stop")

    def burst(self, response_body, status=200, workers=1):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            telemetry = path / "telemetry"
            telemetry.touch(mode=0o600)
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time())))
            runner = probe.Probe(19395, "test-token", workers, 1, telemetry, path / "stop", True)
            connection = mock.MagicMock()
            connection.getresponse.return_value.status = status
            connection.getresponse.return_value.read.return_value = response_body
            with mock.patch.object(probe.http.client, "HTTPConnection", return_value=connection):
                return runner.run()

    def test_successful_synthetic_burst(self):
        result = self.burst(b'{"attestation":"{\\"v\\":1,\\"s\\":0,\\"t\\":\\"v1.fake\\"}"}')
        self.assertTrue(result["passed"])
        self.assertEqual(result["valid_distinct"], 1)
        self.assertNotIn("v1.fake", json.dumps(result))
        self.assertNotIn("test-token", json.dumps(result))

    def test_bad_http_stops(self):
        result = self.burst(b'{}', status=429)
        self.assertFalse(result["passed"])
        self.assertEqual(result["errors"], {"http_429": 1})

    def test_malformed_response_stops(self):
        result = self.burst(b'private-response-not-json')
        self.assertFalse(result["passed"])
        self.assertEqual(result["errors"], {"JSONDecodeError": 1})
        self.assertNotIn("private-response", json.dumps(result))

    def test_oversized_response_stops(self):
        result = self.burst(b'x' * 20481)
        self.assertEqual(result["errors"], {"invalid_proof": 1})

    def test_token_file_rules(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "token"
            path.touch(mode=0o600)
            path.write_text("x" * 32 + "\n")
            self.assertEqual(probe.read_token(path), "x" * 32)
            for content in ("short", "x" * 513, "x" * 32 + " internal-space"):
                path.write_text(content)
                with self.assertRaises(ValueError):
                    probe.read_token(path)
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                probe.read_token(path)
            link = pathlib.Path(tmp) / "link"
            link.symlink_to(path)
            with self.assertRaises(OSError):
                probe.read_token(link)

    def test_sequence_timestamp_cannot_be_refreshed_without_progress(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            telemetry = path / "telemetry"
            telemetry.touch(mode=0o600)
            runner = probe.Probe(19395, "fake", 32, 300, telemetry, path / "stop")
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time())))
            self.assertIsNone(runner.check_resources())
            self.assertIsNone(runner.check_resources())  # ordinary reread
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time() - 1)))
            self.assertEqual(runner.check_resources(), "air_telemetry_reused_sequence")

    def test_huge_numeric_telemetry_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            telemetry = path / "telemetry"
            telemetry.touch(mode=0o600)
            telemetry.write_text(json.dumps(self.sample(cpu_busy_percent=10 ** 1000)))
            runner = probe.Probe(19395, "fake", 32, 300, telemetry, path / "stop")
            self.assertEqual(runner.check_resources(), "air_telemetry_invalid")

    def parallel(self, workers, duplicate=False, fail_one=False, seconds=1, burst=True):
        barrier = threading.Barrier(workers)
        counter_lock = threading.Lock()
        counter = 0

        class Connection:
            def __init__(self, *args, **kwargs):
                nonlocal counter
                with counter_lock:
                    counter += 1
                    self.number = counter
                self.status = 503 if fail_one and self.number == 1 else 200

            def request(self, *args, **kwargs):
                if burst:
                    barrier.wait(timeout=2)

            def getresponse(self):
                return self

            def read(self, *args):
                return json.dumps({"attestation": json.dumps({"v": 1, "s": 0,
                    "t": "v1." + str(0 if duplicate else self.number)})}).encode()

            def close(self):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp)
            telemetry = path / "telemetry"
            telemetry.touch(mode=0o600)
            telemetry.write_text(json.dumps(self.sample(observed_at=time.time())))
            runner = probe.Probe(19395, "fake", workers, seconds, telemetry, path / "stop", burst)
            with mock.patch.object(probe.http.client, "HTTPConnection", Connection):
                return runner.run()

    def test_32_and_33_synthetic_concurrency(self):
        for workers in (32, 33):
            with self.subTest(workers=workers):
                result = self.parallel(workers)
                self.assertTrue(result["passed"])
                self.assertEqual(result["completed"], workers)
                self.assertEqual(result["peak_client_in_flight"], workers)

    def test_duplicate_parallel_proofs_fail(self):
        result = self.parallel(32, duplicate=True)
        self.assertFalse(result["passed"])
        self.assertEqual(result["valid_distinct"], 1)
        self.assertEqual(result["errors"].get("duplicate_proof"), 31)

    def test_one_failed_worker_fails_entire_stage(self):
        result = self.parallel(33, fail_one=True)
        self.assertFalse(result["passed"])
        self.assertEqual(result["errors"], {"http_503": 1})

    def test_short_synthetic_sustained_load(self):
        result = self.parallel(1, seconds=.05, burst=False)
        self.assertTrue(result["passed"])
        self.assertGreater(result["completed"], 1)
        self.assertGreaterEqual(result["elapsed_s"], .05)

    def test_independent_watchdog_kills_stalled_probe(self):
        code = ("import importlib.util,threading,time\n"
                f"s=importlib.util.spec_from_file_location('probe',{str(spec.origin)!r})\n"
                "p=importlib.util.module_from_spec(s);s.loader.exec_module(p)\n"
                "class Stalled:\n"
                " duration=-8.9\n"
                " stop=threading.Event()\n"
                " def run(self):time.sleep(60)\n"
                "p.run_bounded(Stalled())\n")
        result = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(json.loads(result.stdout)["abort_reason"], "watchdog_deadline")


if __name__ == "__main__":
    unittest.main()
