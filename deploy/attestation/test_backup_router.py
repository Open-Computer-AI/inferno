import asyncio
import json
import time
import unittest
import uuid

from backup_router import Backend, BackendFailure, Router, validate_payload, valid_backend_ports

TOKEN = 'test-only-frontend-token-' + 'x' * 32


class Fixture:
    """Synthetic HTTP fixture; never evidence of a real DeviceCheck/Live session."""
    def __init__(self, mode='ok', delay=0):
        self.mode, self.delay = mode, delay
        self.count = self.active = self.peak = self.disconnected = 0
        self.received = asyncio.Event()
        self.tasks = set()
        self.auth = []

    async def handle(self, reader, writer):
        task = asyncio.current_task()
        self.tasks.add(task)
        self.count += 1
        self.active += 1
        self.peak = max(self.peak, self.active)
        try:
            request = await reader.readuntil(b'\r\n\r\n')
            self.auth.append(next(x for x in request.decode().split('\r\n') if x.startswith('Authorization:')))
            self.received.set()
            malformed = {
                'bad_status': b'NOTHTTP\r\nContent-Length: 0\r\n\r\n',
                'truncated_headers': b'HTTP/1.1 200 Test\r\nContent-Length:',
                'missing_length': b'HTTP/1.1 200 Test\r\n\r\n{}',
                'duplicate_length': b'HTTP/1.1 200 Test\r\nContent-Length: 2\r\nContent-Length: 2\r\n\r\n{}',
                'oversized_length': b'HTTP/1.1 200 Test\r\nContent-Length: 999999\r\n\r\n',
                'short_body': b'HTTP/1.1 200 Test\r\nContent-Length: 100\r\n\r\n{}',
                'invalid_header': b'HTTP/1.1 200 Test\r\nBADHEADER\r\nContent-Length: 0\r\n\r\n',
                'oversized_header': b'HTTP/1.1 200 Test\r\nX-Big: ' + b'x' * 9000 + b'\r\nContent-Length: 0\r\n\r\n',
            }
            deep = b'{"attestation":' + b'[' * 1500 + b'0' + b']' * 1500 + b'}'
            malformed['deep_json'] = f'HTTP/1.1 200 Test\r\nContent-Length: {len(deep)}\r\n\r\n'.encode() + deep
            if self.mode in malformed:
                writer.write(malformed[self.mode])
                await writer.drain()
                return
            if self.mode == 'hang':
                await reader.read()
                self.disconnected += 1
                return
            await asyncio.sleep(self.delay)
            status = 429 if self.mode == 'busy' else 200
            if self.mode == 'malformed':
                body = b'{"attestation":"bad"}'
            elif b'GET /health' in request:
                body = b'{"supported":true}'
            else:
                body = json.dumps({'attestation': json.dumps({'v': 1, 's': 0, 't': 'v1.synthetic-' + uuid.uuid4().hex})}).encode()
            writer.write(f'HTTP/1.1 {status} Test\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n'.encode() + body)
            await writer.drain()
        finally:
            self.active -= 1
            self.tasks.discard(task)
            writer.close()
            await writer.wait_closed()


class RouterTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.servers, self.fixtures, self.routers = [], [], []

    async def asyncTearDown(self):
        for server in self.servers:
            server.close()
            await server.wait_closed()
        tasks = set()
        for router in self.routers:
            tasks.update(router.active)
        for fixture in self.fixtures:
            tasks.update(fixture.tasks)
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)

    async def backend(self, name, mode='ok', delay=0, limit=16):
        fixture = Fixture(mode, delay)
        server = await asyncio.start_server(fixture.handle, '127.0.0.1', 0)
        self.servers.append(server)
        self.fixtures.append(fixture)
        return Backend(name, server.sockets[0].getsockname()[1], name + '-private-token', limit), fixture

    async def router(self, backends, **options):
        router = Router(TOKEN, backends, **options)
        server = await asyncio.start_server(router.handle, '127.0.0.1', 0)
        self.servers.append(server)
        self.routers.append(router)
        return router, server.sockets[0].getsockname()[1]

    async def request(self, port, token=TOKEN, path='/v1/attestation'):
        reader, writer = await asyncio.open_connection('127.0.0.1', port)
        method = 'GET' if path == '/health' else 'POST'
        writer.write(f'{method} {path} HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer {token}\r\nContent-Length: 0\r\n\r\n'.encode())
        await writer.drain()
        response = await asyncio.wait_for(reader.read(), 2)
        writer.close()
        await writer.wait_closed()
        header, body = response.split(b'\r\n\r\n', 1)
        return int(header.split()[1]), body

    async def test_sixteen_callers_wait_for_recovery_probe(self):
        first, a = await self.backend('primary')
        self.servers[-1].close()
        await self.servers[-1].wait_closed()
        second, b = await self.backend('backup', delay=.05)
        second.failed = True
        router, port = await self.router([first, second], budget=.5, attempt=.1)
        responses = await asyncio.gather(*(self.request(port) for _ in range(16)))
        self.assertEqual([x[0] for x in responses], [200] * 16)
        self.assertEqual(b.count, 16)
        self.assertLessEqual(b.peak, 16)
        self.assertFalse(second.failed)
        self.assertEqual(router.in_flight + first.in_flight + second.in_flight, 0)

    async def test_waiting_recovery_callers_obey_shared_deadline(self):
        first, a = await self.backend('primary', 'hang')
        first.failed = True
        router, port = await self.router([first], budget=.12, attempt=.05)
        start = time.monotonic()
        responses = await asyncio.gather(*(self.request(port) for _ in range(16)))
        self.assertEqual([x[0] for x in responses], [503] * 16)
        self.assertLess(time.monotonic() - start, .3)
        self.assertEqual(a.count, 1)
        self.assertEqual(router.in_flight + first.in_flight, 0)

    async def test_healthy_primary_and_independent_auth(self):
        first, a = await self.backend('primary')
        second, b = await self.backend('backup')
        router, port = await self.router([first, second])
        self.assertEqual((await self.request(port))[0], 200)
        self.assertEqual((a.count, b.count), (1, 0))
        self.assertEqual(a.auth, ['Authorization: Bearer primary-private-token'])
        self.assertEqual(router.records[-1]['selected'], 'primary')

    async def test_bad_frontend_auth_never_reaches_backends(self):
        first, a = await self.backend('primary')
        second, b = await self.backend('backup')
        _, port = await self.router([first, second])
        self.assertEqual((await self.request(port, 'wrong'))[0], 401)
        self.assertEqual(a.count + b.count, 0)

    async def test_refused_primary_falls_back(self):
        first, _ = await self.backend('primary')
        self.servers[-1].close()
        await self.servers[-1].wait_closed()
        second, b = await self.backend('backup')
        router, port = await self.router([first, second])
        self.assertEqual((await self.request(port))[0], 200)
        self.assertEqual(b.count, 1)
        self.assertEqual(router.records[-1]['selected'], 'backup')

    async def test_hanging_primary_is_cancelled_before_backup(self):
        first, a = await self.backend('primary', 'hang')
        second, _ = await self.backend('backup')
        router, port = await self.router([first, second], budget=.3, attempt=.05)
        start = time.monotonic()
        self.assertEqual((await self.request(port))[0], 200)
        self.assertLess(time.monotonic() - start, .3)
        await asyncio.sleep(.02)
        self.assertEqual(a.disconnected, 1)
        self.assertEqual(router.in_flight + first.in_flight + second.in_flight, 0)

    async def test_busy_and_malformed_primary_fall_back(self):
        for mode in ('busy', 'malformed'):
            first, _ = await self.backend(mode, mode)
            second, b = await self.backend('backup')
            router, port = await self.router([first, second])
            self.assertEqual((await self.request(port))[0], 200)
            self.assertEqual(b.count, 1)
            self.assertEqual(router.records[-1]['selected'], 'backup')

    async def test_all_hanging_uses_one_budget_and_releases_slots(self):
        first, a = await self.backend('primary', 'hang')
        second, b = await self.backend('backup', 'hang')
        router, port = await self.router([first, second], budget=.12, attempt=.05)
        start = time.monotonic()
        self.assertEqual((await self.request(port))[0], 503)
        self.assertLess(time.monotonic() - start, .25)
        await asyncio.sleep(.02)
        self.assertEqual(a.disconnected + b.disconnected, 2)
        self.assertEqual(router.in_flight + first.in_flight + second.in_flight, 0)

    async def test_all_backend_protocol_failures_fall_back(self):
        for mode in ('bad_status', 'truncated_headers', 'missing_length', 'duplicate_length',
                     'oversized_length', 'short_body', 'invalid_header', 'oversized_header', 'deep_json'):
            with self.subTest(mode=mode):
                first, _ = await self.backend(mode, mode)
                second, b = await self.backend('backup')
                router, port = await self.router([first, second])
                self.assertEqual((await self.request(port))[0], 200)
                self.assertEqual(b.count, 1)
                self.assertEqual(router.records[-1]['selected'], 'backup')

    async def test_caller_cancellation_prevents_second_attempt(self):
        first, a = await self.backend('primary', 'hang')
        second, b = await self.backend('backup')
        router, port = await self.router([first, second], budget=.4, attempt=.2)
        reader, writer = await asyncio.open_connection('127.0.0.1', port)
        writer.write(f'POST /v1/attestation HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer {TOKEN}\r\nContent-Length: 0\r\n\r\n'.encode())
        await writer.drain()
        await asyncio.wait_for(a.received.wait(), .2)
        writer.close()
        await writer.wait_closed()
        await asyncio.sleep(.05)
        self.assertEqual(b.count, 0)
        self.assertEqual(a.disconnected, 1)
        self.assertEqual(router.in_flight + first.in_flight + second.in_flight, 0)
        self.assertTrue(router.records[-1]['cancelled'])
        self.assertFalse(first.failed)

    async def test_recovery_restores_primary_after_cooldown(self):
        first, a = await self.backend('primary', 'busy')
        second, b = await self.backend('backup')
        router, port = await self.router([first, second], cooldown=.05)
        await self.request(port)
        a.mode = 'ok'
        await self.request(port)
        self.assertEqual(a.count, 1)
        await asyncio.sleep(.06)
        await self.request(port)
        self.assertEqual(router.records[-1]['selected'], 'primary')
        self.assertFalse(first.failed)

    async def test_two_failed_helpers_can_select_third(self):
        first, _ = await self.backend('one', 'busy')
        second, _ = await self.backend('two', 'malformed')
        third, c = await self.backend('three')
        router, port = await self.router([first, second, third])
        self.assertEqual((await self.request(port))[0], 200)
        self.assertEqual(c.count, 1)
        self.assertEqual(router.records[-1]['selected'], 'three')

    async def test_global16_and_per_backend8_limits(self):
        first, a = await self.backend('primary', delay=.1, limit=8)
        second, b = await self.backend('backup', delay=.1, limit=8)
        router, port = await self.router([first, second], limit=16)
        responses = await asyncio.gather(*(self.request(port) for _ in range(24)))
        self.assertEqual(sum(status == 200 for status, _ in responses), 16)
        self.assertEqual(sum(status == 503 for status, _ in responses), 8)
        self.assertLessEqual(a.peak, 8)
        self.assertLessEqual(b.peak, 8)
        self.assertEqual(router.in_flight + first.in_flight + second.in_flight, 0)
        tokens = [json.loads(json.loads(body)['attestation'])['t'] for status, body in responses if status == 200]
        self.assertEqual(len(set(tokens)), 16)

    def test_budget_cannot_equal_gateway_timeout(self):
        with self.assertRaises(ValueError):
            Router(TOKEN, [], budget=8)

    def test_only_distinct_candidate_ports_and_no_self_loop(self):
        self.assertTrue(valid_backend_ports([19485, 19486, 19487]))
        for ports in ([19495, 19486], [19085, 19486], [19095, 19485], [19485, 19485], [443, 19486]):
            self.assertFalse(valid_backend_ports(ports))

    def test_invalid_proof_does_not_pass_envelope_validation(self):
        for proof in ({'v': True, 's': 0, 't': 'v1.test'}, {'v': 1, 's': False, 't': 'v1.test'}, {'v': 1, 's': 0, 't': 'v1.'}):
            with self.assertRaises(BackendFailure):
                validate_payload('/v1/attestation', json.dumps({'attestation': json.dumps(proof)}).encode())


if __name__ == '__main__':
    unittest.main()
