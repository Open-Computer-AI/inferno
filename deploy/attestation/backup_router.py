#!/usr/bin/env python3
"""Candidate-only loopback attestation selector. Never a production entrypoint."""
import asyncio
import dataclasses
import hmac
import json
import os
import pathlib
import signal
import subprocess
import sys
import time

from backup_helpers import owned_file

MAX_BODY = 20480


class BackendFailure(Exception):
    pass


def validate_payload(path, body):
    if len(body) > MAX_BODY:
        raise BackendFailure('oversize')
    try:
        data = json.loads(body)
        if not isinstance(data, dict) or set(data) - {'supported', 'attestation'}:
            raise ValueError()
        if path == '/health':
            if data.get('supported') is not True:
                raise ValueError()
        else:
            header = data['attestation']
            if not isinstance(header, str) or not 20 <= len(header) <= 16384 or '\r' in header or '\n' in header:
                raise ValueError()
            proof = json.loads(header)
            if (type(proof.get('v')) is not int or proof['v'] != 1 or
                    type(proof.get('s')) is not int or proof['s'] != 0 or
                    not isinstance(proof.get('t'), str) or not proof['t'].startswith('v1.') or len(proof['t']) <= 3):
                raise ValueError()
    except (ValueError, KeyError, TypeError, AttributeError, RecursionError):
        raise BackendFailure('invalid_payload') from None
    return body


@dataclasses.dataclass
class Backend:
    name: str
    port: int
    token: str = dataclasses.field(repr=False)
    limit: int = 16
    in_flight: int = 0
    unavailable_until: float = 0
    failed: bool = False
    probing: bool = False

    def acquire(self):
        now = time.monotonic()
        if self.in_flight >= self.limit or now < self.unavailable_until or (self.failed and self.probing):
            return False
        self.in_flight += 1
        if self.failed:
            self.probing = True
        return True

    def release(self, success, cooldown):
        self.in_flight -= 1
        self.probing = False
        if success is True:
            self.failed = False
            self.unavailable_until = 0
        elif success is False:
            self.failed = True
            self.unavailable_until = time.monotonic() + cooldown


async def exchange(backend, path):
    writer = None
    try:
        reader, writer = await asyncio.open_connection('127.0.0.1', backend.port, limit=8192)
        method = 'GET' if path == '/health' else 'POST'
        request = (f'{method} {path} HTTP/1.1\r\nHost: 127.0.0.1:{backend.port}\r\n'
                   f'Authorization: Bearer {backend.token}\r\nAccept: application/json\r\n'
                   'Content-Length: 0\r\nConnection: close\r\n\r\n').encode()
        writer.write(request)
        await writer.drain()
        header = await reader.readuntil(b'\r\n\r\n')
        lines = header.decode('latin1').split('\r\n')
        status_line = lines[0].split()
        if status_line[0] not in {'HTTP/1.0', 'HTTP/1.1'} or len(status_line[1]) != 3:
            raise BackendFailure('response_status')
        status = int(status_line[1])
        if status != 200:
            raise BackendFailure('busy' if status == 429 else 'http_' + str(status))
        headers = {}
        for line in lines[1:]:
            if not line:
                continue
            key, value = line.split(':', 1)
            key = key.lower()
            if key in headers:
                raise BackendFailure('duplicate_response_header')
            headers[key] = value.strip()
        length = headers.get('content-length', '')
        if 'transfer-encoding' in headers or not length.isascii() or not length.isdecimal() or int(length) > MAX_BODY:
            raise BackendFailure('response_framing')
        return validate_payload(path, await reader.readexactly(int(length)))
    except (OSError, asyncio.IncompleteReadError, asyncio.LimitOverrunError, ValueError, IndexError):
        raise BackendFailure('transport') from None
    finally:
        if writer:
            writer.close()
            try:
                await asyncio.wait_for(writer.wait_closed(), .1)
            except (OSError, asyncio.TimeoutError):
                pass


class Router:
    def __init__(self, token, backends, budget=7, attempt=2, cooldown=3, limit=16, audit=None):
        if not 0 < attempt <= budget < 8 or not 0 <= cooldown <= 60 or not 1 <= limit <= 16:
            raise ValueError('candidate bounds invalid')
        self.token = token
        self.backends = backends
        self.budget, self.attempt, self.cooldown, self.limit = budget, attempt, cooldown, limit
        self.in_flight = 0
        self.active = set()
        self.audit = audit
        self.records = []

    def record(self, row):
        self.records.append(row)
        # Keep only bounded in-memory diagnostics during a sustained run.
        if len(self.records) > 100:
            del self.records[:-100]
        if self.audit:
            with self.audit.open('a') as stream:
                stream.write(json.dumps(row) + '\n')

    async def respond(self, writer, status, body):
        writer.write(f'HTTP/1.1 {status} Result\r\nContent-Type: application/json\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n'.encode() + body)
        await writer.drain()

    async def handle(self, reader, writer):
        task = asyncio.current_task()
        self.active.add(task)
        disconnect = operation = None
        admitted = False
        started = time.monotonic()
        row = {'at': time.time(), 'attempts': [], 'selected': None, 'status': 503, 'cancelled': False}
        try:
            raw = await asyncio.wait_for(reader.readuntil(b'\r\n\r\n'), 2)
            if len(raw) > 4096:
                await self.respond(writer, 431, b'{}')
                return
            lines = raw.decode('latin1').split('\r\n')
            method, path, version = lines[0].split()
            headers = {}
            for line in lines[1:]:
                if not line:
                    continue
                key, value = line.split(':', 1)
                key = key.lower()
                if key in headers:
                    raise ValueError('duplicate request header')
                headers[key] = value.strip()
            if not hmac.compare_digest(headers.get('authorization', ''), 'Bearer ' + self.token):
                row['status'] = 401
                await self.respond(writer, 401, b'{}')
                return
            if version != 'HTTP/1.1' or (method, path) not in {('GET', '/health'), ('POST', '/v1/attestation')}:
                row['status'] = 404
                await self.respond(writer, 404, b'{}')
                return
            if 'transfer-encoding' in headers or headers.get('content-length', '0') != '0':
                raise ValueError('unexpected request body')
            if self.in_flight >= self.limit:
                await self.respond(writer, 503, b'{"error":"candidate capacity"}')
                return
            self.in_flight += 1
            admitted = True
            # Reader EOF cancels pending upstream work; no fallback follows cancellation.
            disconnect = asyncio.create_task(reader.read(1))
            deadline = started + self.budget
            for index, backend in enumerate(self.backends):
                remaining = deadline - time.monotonic()
                if remaining <= 0 or disconnect.done():
                    row['cancelled'] = disconnect.done()
                    break
                acquired = backend.acquire()
                # A single half-open probe protects the recovering native helper.
                # If no alternative can run, admitted callers wait for its result
                # within their original budget instead of spuriously returning503.
                while not acquired and backend.probing and not disconnect.done():
                    alternative = any(other is not backend and
                                      other.in_flight < other.limit and
                                      time.monotonic() >= other.unavailable_until and
                                      not (other.failed and other.probing)
                                      for other in self.backends[index + 1:])
                    if alternative or time.monotonic() >= deadline:
                        break
                    await asyncio.wait({disconnect}, timeout=min(.01, max(0, deadline - time.monotonic())))
                    acquired = backend.acquire() if not disconnect.done() and time.monotonic() < deadline else False
                if not acquired:
                    continue
                remaining = deadline - time.monotonic()
                success = None
                attempt_start = time.monotonic()
                observation = {'backend': backend.name}
                operation = asyncio.create_task(exchange(backend, path))
                try:
                    timeout = remaining if index == len(self.backends) - 1 else min(self.attempt, remaining)
                    done, _ = await asyncio.wait({operation, disconnect}, timeout=timeout, return_when=asyncio.FIRST_COMPLETED)
                    if disconnect in done:
                        row['cancelled'] = True
                        observation['result'] = 'cancelled'
                        return
                    if operation not in done:
                        observation['result'] = 'timeout'
                        success = False
                        continue
                    try:
                        body = operation.result()
                    except BackendFailure as exc:
                        observation['result'] = str(exc)
                        success = False
                        continue
                    success = True
                    row['selected'], row['status'] = backend.name, 200
                    observation['result'] = 'success'
                    await self.respond(writer, 200, body)
                    return
                finally:
                    if operation and not operation.done():
                        operation.cancel()
                    if operation:
                        await asyncio.gather(operation, return_exceptions=True)
                    backend.release(success, self.cooldown)
                    observation['ms'] = round((time.monotonic() - attempt_start) * 1000, 3)
                    row['attempts'].append(observation)
            if not row['cancelled']:
                await self.respond(writer, 503, b'{"error":"attestation unavailable"}')
        except (OSError, ValueError, TypeError, asyncio.TimeoutError, asyncio.IncompleteReadError, asyncio.LimitOverrunError):
            row['request_error'] = True
        except asyncio.CancelledError:
            row['cancelled'] = True
            raise
        finally:
            for pending in (operation, disconnect):
                if pending and not pending.done():
                    pending.cancel()
            await asyncio.gather(*(x for x in (operation, disconnect) if x), return_exceptions=True)
            if admitted:
                self.in_flight -= 1
            row['in_flight_after'] = self.in_flight
            row['backend_in_flight_after'] = [backend.in_flight for backend in self.backends]
            row['ms'] = round((time.monotonic() - started) * 1000, 3)
            try:
                self.record(row)
            finally:
                writer.close()
                try:
                    await asyncio.wait_for(writer.wait_closed(), .1)
                except (OSError, asyncio.TimeoutError):
                    pass
                self.active.discard(task)


async def serve(config_path):
    owned_file(config_path, 0o600)
    config = json.loads(config_path.read_text())
    tokens_path = pathlib.Path(config['backend_tokens'])
    token_path = pathlib.Path(config['frontend_token'])
    for path in (tokens_path, token_path):
        owned_file(path, 0o600)
    tokens = json.loads(tokens_path.read_text())
    token = token_path.read_text().strip()
    if not 32 <= len(token) <= 512 or not token.isascii() or any(x.isspace() for x in token):
        raise ValueError('invalid frontend token')
    backends = []
    if not valid_backend_ports([item['port'] for item in config['backends']]):
        raise ValueError('candidate backend ports invalid')
    for item in config['backends']:
        if not re_valid_name(item['name']):
            raise ValueError('candidate backend invalid')
        secret = tokens[item['token_key']]
        if not 32 <= len(secret) <= 512 or not secret.isascii() or any(x.isspace() for x in secret):
            raise ValueError('invalid backend token')
        backends.append(Backend(item['name'], item['port'], secret))
    if not 2 <= len(backends) <= 3 or len({x.name for x in backends}) != len(backends):
        raise ValueError('two or three distinct candidate backends required')
    if len({x.token for x in backends}) != len(backends) or token in {x.token for x in backends}:
        raise ValueError('independent frontend and backend tokens required')
    audit = pathlib.Path(config['audit'])
    audit_parent = audit.parent.resolve()
    if audit_parent != config_path.parent.parent.resolve() or audit_parent.stat().st_uid != os.getuid() or audit_parent.stat().st_mode & 0o077:
        raise ValueError('audit must be in private candidate artifact root')
    fd = os.open(audit, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    router = Router(token, backends, audit=audit)
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        loop.add_signal_handler(sig, stop.set)
    server = await asyncio.start_server(router.handle, '127.0.0.1', 19495, limit=8192)
    process_file = config_path.parent / 'router-process.json'
    started = subprocess.check_output(['ps', '-p', str(os.getpid()), '-o', 'lstart='], text=True).strip()
    process_file.write_text(json.dumps({'pid': os.getpid(), 'started': started, 'run_id': config['run_id']}))
    process_file.chmod(0o600)
    print(json.dumps({'candidate_router_ready': True, 'port': 19495, 'pid': os.getpid()}), flush=True)
    deadline = time.monotonic() + 3500
    try:
        while time.monotonic() < deadline and not stop.is_set() and not (config_path.parent / 'stop-router').exists():
            await asyncio.sleep(.2)
    finally:
        server.close()
        await server.wait_closed()
        active = list(router.active)
        for task in active:
            task.cancel()
        await asyncio.gather(*active, return_exceptions=True)
        print(json.dumps({'candidate_router_stopped': True, 'in_flight': router.in_flight,
                          'backend_in_flight': [x.in_flight for x in backends]}), flush=True)


def re_valid_name(value):
    return isinstance(value, str) and 0 < len(value) <= 40 and all(x in 'abcdefghijklmnopqrstuvwxyz0123456789-' for x in value)


def valid_backend_ports(ports):
    return (len(set(ports)) == len(ports)
            and all(type(port) is int and 19400 <= port <= 19499 and port != 19495 for port in ports))


if __name__ == '__main__':
    asyncio.run(serve(pathlib.Path(sys.argv[1])))
