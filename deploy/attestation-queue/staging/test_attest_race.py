import asyncio
import contextlib
import json
import pathlib
import tempfile
import unittest

from attest_race import HEALTH, PROOF, load_queue
from stage_attestation import Fixture, candidate, queue_state, ready, request, request_bytes, wait_for, wave, write_token


class RaceTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.exits = contextlib.AsyncExitStack()
        self.addAsyncCleanup(self.exits.aclose)
        directory = pathlib.Path(self.exits.enter_context(tempfile.TemporaryDirectory()))
        front, self.auth = write_token(directory, "gateway")
        self.fixture = {}
        self.helpers = []
        for role in ("air", "backup", "third"):
            path, token = write_token(directory, role)
            fixture = await self.exits.enter_async_context(Fixture(role, token, .03))
            self.fixture[role] = fixture
            self.helpers.append({"name": role, "token_file": path.name, "port": fixture.port, "concurrency": 16})
        self.config_path = directory / "race.json"
        self.front = front.name

    async def launch(self, hedge=0, count=2, capacity=16, attempt=220, budget=650):
        self.config_path.write_text(json.dumps({
            "frontend_token_file": self.front, "helpers": self.helpers[:count],
            "hedge_ms": hedge, "deadline_ms": budget, "attempt_ms": attempt,
        }))
        path = pathlib.Path(__file__).with_name("attest_queue.py")
        port = await self.exits.enter_async_context(candidate(path, self.config_path, capacity))
        await ready(port, capacity)
        return port

    async def test_head_start_avoids_duplicate_for_a_fast_primary(self):
        port = await self.launch(hedge=160)
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-air", data)
        self.assertEqual(self.fixture["backup"].attempts, 0)

    async def test_instant_race_starts_both_paths_and_uses_first_valid_proof(self):
        port = await self.launch()
        self.fixture["air"].delay = .15
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-backup", data)
        self.assertEqual(self.fixture["air"].attempts, 1)
        self.assertEqual(self.fixture["backup"].attempts, 1)

    async def test_stalled_air_at_sixteen_concurrent_requests(self):
        port = await self.launch()
        self.fixture["air"].mode = "stall"
        results = await wave(port, 16, self.auth)
        self.assertEqual([status for status, _, _ in results], [200] * 16)
        self.assertEqual(len(set(proof for _, proof, _ in results)), 16)
        state = await queue_state(port)
        self.assertEqual(state["backends"]["backup"]["pooled_proofs"], 16)
        self.assertEqual(state["backends"]["backup"]["wins"], 16)
        self.assertEqual(state["in_flight"], 0)
        self.assertTrue(all(helper["in_use"] == 0 for helper in state["backends"].values()))
        await wait_for(lambda: self.fixture["air"].pending == 0)

    async def test_reset_skips_head_start_and_failed_air_recovers(self):
        port = await self.launch(hedge=180)
        self.fixture["air"].mode = "reset"
        self.fixture["air"].hang_health = True
        status, data, elapsed = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-backup", data)
        self.assertLess(elapsed, 170)
        self.assertFalse((await queue_state(port))["backends"]["air"]["ready"])
        self.fixture["air"].mode = "healthy"
        self.fixture["air"].hang_health = False
        await ready(port, 1)
        self.assertTrue((await queue_state(port))["backends"]["air"]["ready"])

    async def test_two_stalls_can_fall_through_to_third_helper(self):
        port = await self.launch(count=3)
        self.fixture["air"].mode = self.fixture["backup"].mode = "stall"
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-third", data)

    async def test_all_down_fails_closed_and_releases_capacity(self):
        port = await self.launch()
        self.fixture["air"].mode = self.fixture["backup"].mode = "stall"
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 503)
        self.assertEqual(json.loads(data), {"error": "attestation unavailable"})
        state = await queue_state(port)
        self.assertEqual((state["in_flight"], state["failed"], state["served"]), (0, 1, 0))
        self.assertTrue(all(helper["in_use"] == 0 for helper in state["backends"].values()))

    async def test_malformed_proof_does_not_win(self):
        port = await self.launch(hedge=180)
        self.fixture["air"].mode = "malformed"
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-backup", data)

    async def test_busy_helper_does_not_disable_healthy_backend(self):
        port = await self.launch(hedge=180)
        self.fixture["air"].mode = "busy"
        status, data, _ = await request(port, PROOF, self.auth)
        self.assertEqual(status, 200)
        self.assertIn(b"stage-fixture-backup", data)
        state = await queue_state(port)
        self.assertTrue(state["backends"]["air"]["ready"])
        self.assertEqual(state["backends"]["air"]["busy"], 1)

    async def test_client_cancel_closes_both_connections_and_releases_slots(self):
        port = await self.launch()
        self.fixture["air"].mode = self.fixture["backup"].mode = "stall"
        _, writer = await asyncio.open_connection("127.0.0.1", port)
        writer.write(request_bytes(PROOF, self.auth))
        await writer.drain()
        await wait_for(lambda: all(self.fixture[role].pending == 1 for role in ("air", "backup")))
        writer.close()
        await writer.wait_closed()
        await wait_for(lambda: all(self.fixture[role].pending == 0 for role in ("air", "backup")))
        state = await queue_state(port)
        self.assertEqual(state["in_flight"], 0)
        self.assertTrue(all(helper["in_use"] == 0 for helper in state["backends"].values()))
        self.assertEqual(state["backends"]["air"]["cancelled"], 1)

    async def test_frontend_auth_and_health(self):
        port = await self.launch()
        self.assertEqual((await request(port, PROOF, b"Bearer invalid"))[0], 401)
        self.assertEqual((await request(port, HEALTH, self.auth))[0], 200)
        self.assertEqual(self.fixture["air"].attempts, 0)
        self.assertEqual(self.fixture["backup"].attempts, 0)

    async def test_unsafe_budget_is_rejected(self):
        self.config_path.write_text(json.dumps({
            "frontend_token_file": self.front, "helpers": self.helpers[:2], "deadline_ms": 8000,
        }))
        with self.assertRaisesRegex(ValueError, "invalid race timeouts"):
            load_queue(self.config_path, 16, 16, 3)


if __name__ == "__main__":
    unittest.main()
