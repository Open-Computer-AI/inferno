import importlib.util
import http.server
import json
import pathlib
import threading
import sys
import subprocess
import unittest
from unittest import mock

DIRECTORY = pathlib.Path(__file__).resolve().parents[1]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


timing = load("timing_probe", DIRECTORY / "timing_probe.py")


class Helper(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def do_GET(self):
        body = b'{"supported":true}'
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        body = json.dumps({"attestation": '{"v":1,"s":0,"t":"v1.synthetic"}'}).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class TimingTests(unittest.TestCase):
    def setUp(self):
        self.helper = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Helper)
        threading.Thread(target=self.helper.serve_forever, daemon=True).start()

    def tearDown(self):
        self.helper.shutdown()
        self.helper.server_close()

    def test_phase_totals_and_reuse(self):
        connection = timing.TimedConnection("127.0.0.1", self.helper.server_port, timeout=2)
        try:
            first = timing.measure(19389, "/health", "synthetic-private-token", connection)
            second = timing.measure(19389, "/v1/attestation", "synthetic-private-token", connection)
        finally:
            connection.close()
        self.assertFalse(first["reused_connection"])
        self.assertTrue(second["reused_connection"])
        self.assertTrue(second["valid_envelope"])
        self.assertEqual(second["connect_ms"], 0)
        for row in (first, second):
            parts = sum(row[k] for k in ("connect_ms", "request_send_ms", "headers_wait_ms", "body_read_ms"))
            self.assertAlmostEqual(parts, row["total_ms"], delta=1)
            self.assertNotIn("synthetic-private-token", json.dumps(row))
            self.assertNotIn("v1.synthetic", json.dumps(row))

    def test_instrumented_copy_routes_and_measures(self):
        queue = load("timing_test_queue", DIRECTORY.parent / "attestation-queue/attest_queue.py")
        timing.instrument_queue(queue, upstream_port=self.helper.server_port)
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), queue.Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            for path in ("/health", "/v1/attestation"):
                connection = timing.TimedConnection("127.0.0.1", server.server_port, timeout=2)
                try:
                    result = timing.measure(19395, path, "synthetic-private-token", connection)
                finally:
                    connection.close()
                self.assertTrue(result["valid_envelope"])
                phases = result["queue_server"]
                self.assertGreaterEqual(phases["admission_wait_ms"], 0)
                self.assertEqual(len(phases["attempts"]), 1)
                attempt = phases["attempts"][0]
                for key in ("connect_ms", "send_including_connect_ms", "headers_wait_ms", "body_read_ms", "total_ms"):
                    self.assertGreaterEqual(attempt[key], 0)
                self.assertGreaterEqual(phases["server_before_response_ms"], attempt["total_ms"])
        finally:
            server.shutdown()
            server.server_close()

    def test_invalid_port_rejected(self):
        with self.assertRaises(ValueError):
            timing.measure(80, "/health", "unused")

    def test_quantile_counts(self):
        self.assertEqual(timing.quantiles([]), {})
        self.assertEqual(timing.quantiles(range(1, 101))["p95"], 95)
        self.assertEqual(timing.summarize([])["count"], 0)

    def test_boolean_versions_are_not_valid_proofs(self):
        self.assertFalse(timing.valid_payload("/v1/attestation", {"attestation": '{"v":true,"s":false,"t":"v1.fake"}'}))

    def test_reconnect_is_not_reported_as_connection_reuse(self):
        connection = timing.TimedConnection("127.0.0.1", self.helper.server_port, timeout=2)
        connection.connect()
        original_request = connection.request

        def reconnect(*args, **kwargs):
            connection.sock.close()
            connection.sock = None
            return original_request(*args, **kwargs)

        try:
            with mock.patch.object(connection, "request", side_effect=reconnect):
                row = timing.measure(19389, "/health", "synthetic-private-token", connection)
        finally:
            connection.close()
        self.assertFalse(row["reused_connection"])
        self.assertTrue(row["reconnected_during_request"])
        parts = sum(row[k] for k in ("connect_ms", "request_send_ms", "headers_wait_ms", "body_read_ms"))
        self.assertAlmostEqual(parts, row["total_ms"], delta=1)

    def test_isolated_queue_can_reuse_upstream_without_reusing_client(self):
        queue = load("timing_pooled_test_queue", DIRECTORY.parent / "attestation-queue/attest_queue.py")
        timing.instrument_queue(queue, upstream_port=self.helper.server_port)
        queue.KEEPALIVE = True
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), queue.Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        rows = []
        try:
            for _ in range(2):
                connection = timing.TimedConnection("127.0.0.1", server.server_port, timeout=2)
                try:
                    rows.append(timing.measure(19395, "/v1/attestation", "synthetic-private-token", connection))
                finally:
                    connection.close()
            self.assertGreater(rows[0]["queue_server"]["attempts"][0]["connect_ms"], 0)
            self.assertEqual(rows[1]["queue_server"]["attempts"][0]["connect_ms"], 0)
            self.assertTrue(all(x["valid_envelope"] for x in rows))
            self.assertEqual(len(queue.idle), 1)
        finally:
            server.shutdown()
            server.server_close()
            for connection, _ in queue.idle:
                connection.close()

    def test_capture_timeout_terminates_only_owned_group(self):
        with mock.patch.dict(sys.modules, {"timing_probe": timing}):
            capture_module = load("capture_timeout_test", DIRECTORY / "timing_ssh_capture.py")
        child = mock.Mock(pid=12345)
        child.communicate.side_effect = [subprocess.TimeoutExpired("capture", 15), ("metadata", "summary")]
        with mock.patch.object(capture_module.subprocess, "run") as run:
            self.assertEqual(capture_module.collect_capture(child), ("metadata", "summary"))
        self.assertEqual(run.call_args.args[0], ["sudo", "-n", "/bin/kill", "-TERM", "--", "-12345"])

    def test_capture_timeout_escalates_to_group_kill(self):
        with mock.patch.dict(sys.modules, {"timing_probe": timing}):
            capture_module = load("capture_kill_test", DIRECTORY / "timing_ssh_capture.py")
        child = mock.Mock(pid=12345)
        child.communicate.side_effect = [subprocess.TimeoutExpired("capture", 15),
                                         subprocess.TimeoutExpired("capture", 3), ("metadata", "summary")]
        with mock.patch.object(capture_module.subprocess, "run") as run:
            self.assertEqual(capture_module.collect_capture(child), ("metadata", "summary"))
        self.assertEqual(run.call_args.args[0], ["sudo", "-n", "/bin/kill", "-KILL", "--", "-12345"])


if __name__ == "__main__":
    unittest.main()
