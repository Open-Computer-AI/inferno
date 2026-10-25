"""Local test of gateway_queue_v2.sh rollback with a fake systemctl (no box, no sudo, no token).

Cases: queue down (no /queue-health answer) -> rollback restores v1 and restarts the unit; queue busy -> rollback
touches nothing and says how to force; FORCE=1 under traffic -> rolls back; idle queue -> rolls back (the normal path).
"""
import http.server
import os
import shutil
import subprocess
import tempfile
import threading
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "gateway_queue_v2.sh")
V1 = os.path.join(HERE, "testdata", "attest_queue_v1.py")
V2 = os.path.join(HERE, "attest_queue.py")
FAKE_SYSTEMCTL = """#!/bin/bash
echo "$*" >> "$FAKE_DIR/calls.log"
case $1 in
  is-active) cat "$FAKE_DIR/state";;
  restart) echo active > "$FAKE_DIR/state";;
  show) echo "Environment=";;
esac
"""


class Health(http.server.BaseHTTPRequestHandler):
    body = b'{"served": 0, "in_flight": 0}'

    def log_message(self, *_):
        pass

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", str(len(self.body)))
        self.end_headers()
        self.wfile.write(self.body)


class RollbackTest(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        for sub in ("bin", "qdir", "drop"):
            os.mkdir(os.path.join(self.d, sub))
        fake = os.path.join(self.d, "bin", "systemctl")
        with open(fake, "w") as f:
            f.write(FAKE_SYSTEMCTL)
        os.chmod(fake, 0o755)
        shutil.copy(V2, os.path.join(self.d, "qdir", "attest_queue.py"))
        shutil.copy(V1, os.path.join(self.d, "qdir", "attest_queue.py.v1-bak"))
        with open(os.path.join(self.d, "drop", "t100-v2.conf"), "w") as f:
            f.write("[Service]\nEnvironment=QUEUE_CONCURRENCY=16\n")
        self.state("failed")
        self.server = None

    def tearDown(self):
        if self.server:
            self.server.shutdown()
            self.server.server_close()
        shutil.rmtree(self.d)

    def state(self, s):
        with open(os.path.join(self.d, "state"), "w") as f:
            f.write(s + "\n")

    def serve(self, body):
        Health.body = body
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Health)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        return "http://127.0.0.1:%d/queue-health" % self.server.server_address[1]

    def rollback(self, health_url, **extra):
        env = dict(os.environ, PATH=os.path.join(self.d, "bin") + ":" + os.environ["PATH"], FAKE_DIR=self.d,
                   QDIR=os.path.join(self.d, "qdir"), DROPDIR=os.path.join(self.d, "drop"), UNIT="fakeq", SUDO="",
                   IDLE_WAIT="0", HEALTH_URL=health_url, **extra)
        return subprocess.run(["bash", SCRIPT, "rollback"], env=env, capture_output=True, text=True, timeout=60)

    def restored(self):
        with open(os.path.join(self.d, "qdir", "attest_queue.py"), "rb") as a, open(V1, "rb") as b:
            return a.read() == b.read()

    def calls(self):
        p = os.path.join(self.d, "calls.log")
        return open(p).read() if os.path.exists(p) else ""

    def test_queue_down_rolls_back_and_restarts(self):
        # The unit failed (v2 or the drop-in broke it); after the restart the v1 queue answers again.
        done = self.rollback(self.serve(b'{"served": 0, "in_flight": 0}'))
        self.assertEqual(done.returncode, 0, done.stdout + done.stderr)
        self.assertIn("restart fakeq", self.calls())
        self.assertTrue(self.restored())
        self.assertFalse(os.path.exists(os.path.join(self.d, "drop", "t100-v2.conf")))

    def test_silent_queue_is_still_restarted(self):
        # No /queue-health answer at all: the old guard stopped before systemctl and left the unit down.
        self.rollback("http://127.0.0.1:1/queue-health")
        self.assertIn("restart fakeq", self.calls())
        self.assertTrue(self.restored())

    def test_busy_queue_touches_nothing(self):
        self.state("active")
        done = self.rollback(self.serve(b'{"served": 3, "in_flight": 2}'))
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("FORCE=1", done.stdout)
        self.assertNotIn("restart", self.calls())
        self.assertTrue(os.path.exists(os.path.join(self.d, "drop", "t100-v2.conf")))
        self.assertFalse(self.restored())

    def test_force_rolls_back_under_traffic(self):
        self.state("active")
        done = self.rollback(self.serve(b'{"served": 3, "in_flight": 2}'), FORCE="1")
        self.assertEqual(done.returncode, 0, done.stdout + done.stderr)
        self.assertTrue(self.restored())

    def test_idle_queue_rolls_back(self):
        self.state("active")
        done = self.rollback(self.serve(b'{"served": 3, "in_flight": 0}'))
        self.assertEqual(done.returncode, 0, done.stdout + done.stderr)
        self.assertIn("ROLLBACK_OK", done.stdout)
        self.assertTrue(self.restored())


if __name__ == "__main__":
    unittest.main()
