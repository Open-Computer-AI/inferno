"""Opt-in, loopback-only multi-helper attestation queue."""

import asyncio
import collections
import hmac
import json
import os
import pathlib
import re
import stat


HEALTH = ("GET", "/health")
PROOF = ("POST", "/v1/attestation")
MAX_BODY = 20 * 1024
HEADER = re.compile(r"[!#$%&'*+.^_`|~0-9A-Za-z-]+")


class InvalidReply(ValueError):
    pass


IO_ERRORS = (OSError, ValueError, EOFError, asyncio.TimeoutError, asyncio.LimitOverrunError)


def read_token(path):
    with open(path, "rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077:
            raise ValueError("token files must be regular and owner-only")
        value = source.read(1025).strip()
    if not re.fullmatch(rb"[\x21-\x7e]{32,512}", value):
        raise ValueError("invalid attestation token")
    return b"Bearer " + value


async def read_head(reader):
    raw = await reader.readuntil(b"\r\n\r\n")
    if len(raw) > 8192:
        raise ValueError("HTTP header too large")
    first, *lines = raw[:-4].decode("latin-1").split("\r\n")
    headers = {}
    for line in lines:
        name, sep, value = line.partition(":")
        key = name.lower()
        if not sep or not HEADER.fullmatch(name) or key in headers:
            raise ValueError("invalid or repeated HTTP header")
        headers[key] = value.strip()
    if "transfer-encoding" in headers:
        raise ValueError("unsupported HTTP framing")
    return first, headers


def response_size(headers):
    value = headers.get("content-length", "")
    if not re.fullmatch(r"[0-9]{1,6}", value) or int(value) > MAX_BODY:
        raise InvalidReply("invalid helper response length")
    return int(value)


def valid_payload(operation, data):
    try:
        body = json.loads(data)
        if not isinstance(body, dict) or body.keys() - {"supported", "attestation"}:
            return False
        if "supported" in body and type(body["supported"]) is not bool:
            return False
        proof = body.get("attestation", "")
        if not isinstance(proof, str):
            return False
        if operation == HEALTH:
            return body.get("supported") is True
        if not 20 <= len(proof.encode()) <= 16 * 1024 or "\r" in proof or "\n" in proof:
            return False
        envelope = json.loads(proof)
        return (
            type(envelope["v"]) is int and envelope["v"] == 1
            and type(envelope["s"]) is int and envelope["s"] == 0
            and isinstance(envelope["t"], str) and envelope["t"].startswith("v1.")
            and len(envelope["t"]) > 3
        )
    except (ValueError, TypeError, KeyError, UnicodeError, RecursionError):
        return False


class Helper:
    def __init__(self, name, port, token, limit, warm, refresh):
        self.name = name
        self.port = port
        self.token = token
        self.limit = limit
        self.warm = warm
        self.refresh = refresh
        self.idle = []
        self.in_use = 0
        self.ready = False
        self.latency = None
        self.generation = 0
        self.counters = collections.Counter()

    @property
    def available(self):
        return self.ready and self.in_use < self.limit

    def down(self):
        self.ready = False
        self.generation += 1
        for _, writer, _ in self.idle:
            writer.close()
        self.idle.clear()

    def take(self):
        while self.idle:
            reader, writer, since = self.idle.pop()
            if not reader.at_eof() and not writer.is_closing() and asyncio.get_running_loop().time() - since < 10:
                return reader, writer
            writer.close()
        return None

    async def exchange(self, operation, connection=None, authenticated=True, use_idle=False):
        if use_idle:
            connection = self.take()
        if operation == PROOF and connection is not None:
            self.counters["pooled_proofs"] += 1
        reader, writer = connection or await asyncio.open_connection("127.0.0.1", self.port, limit=8192)
        reusable = False
        generation = self.generation
        try:
            method, path = operation
            request = f"{method} {path} HTTP/1.1\r\nHost: 127.0.0.1:{self.port}\r\n".encode()
            if authenticated:
                request += b"Authorization: " + self.token + b"\r\n"
            writer.write(request + b"Accept: application/json\r\nContent-Length: 0\r\n\r\n")
            await writer.drain()
            first, headers = await read_head(reader)
            version, code, *_ = first.split()
            if version not in {"HTTP/1.0", "HTTP/1.1"} or not re.fullmatch(r"[1-5][0-9]{2}", code):
                raise InvalidReply("invalid helper response status")
            data = await reader.readexactly(response_size(headers))
            reusable = version == "HTTP/1.1" and "close" not in headers.get("connection", "").lower()
            return int(code), data
        finally:
            proof_slot = int(operation == PROOF)
            if reusable and generation == self.generation and len(self.idle) + self.in_use < self.warm + proof_slot:
                self.idle.append((reader, writer, asyncio.get_running_loop().time()))
            else:
                writer.close()

    async def attempt(self, timeout):
        started = asyncio.get_running_loop().time()
        self.counters["attempts"] += 1
        try:
            status, data = await asyncio.wait_for(self.exchange(PROOF, use_idle=True), timeout)
            if status == 429:
                self.counters["busy"] += 1
                return None
            if status != 200 or not valid_payload(PROOF, data):
                raise InvalidReply("proof rejected")
            done = asyncio.get_running_loop().time()
            latency = done - started
            self.latency = latency if self.latency is None else 0.25 * latency + 0.75 * self.latency
            return done, data
        except asyncio.CancelledError:
            self.counters["cancelled"] += 1
            raise
        except IO_ERRORS:
            self.counters["errors"] += 1
            self.down()
            return None

    def release(self, _):
        self.in_use -= 1

    async def maintain(self):
        """Authenticate on startup/recovery, then refresh a bounded batch of idle sockets."""
        loop = asyncio.get_running_loop()
        next_check = 0.0
        try:
            while True:
                now = loop.time()
                if not self.ready or now >= next_check:
                    generation = self.generation
                    try:
                        code, data = await asyncio.wait_for(self.exchange(HEALTH, use_idle=True), 1.5)
                        if code != 200 or not valid_payload(HEALTH, data):
                            raise InvalidReply("helper unavailable")
                    except IO_ERRORS:
                        if self.generation == generation:
                            self.down()
                        await asyncio.sleep(0.5)
                        continue
                    if self.generation == generation:
                        self.ready = True
                    next_check = loop.time() + self.refresh
                available = max(0, self.warm - self.in_use)
                overdue = []
                while self.idle and len(overdue) < 4 and now - self.idle[0][2] >= self.refresh:
                    reader, writer, since = self.idle.pop(0)
                    if now - since < 10 and not reader.at_eof() and not writer.is_closing():
                        overdue.append((reader, writer))
                    else:
                        writer.close()
                count = min(4 - len(overdue), max(0, available - len(self.idle) - len(overdue)))
                connections = overdue + [None] * count
                if connections:
                    await asyncio.gather(*(self.warm_socket(connection) for connection in connections))
                await asyncio.sleep(0.05 if count else 0.2)
        finally:
            self.down()

    async def warm_socket(self, connection):
        try:
            await asyncio.wait_for(self.exchange(HEALTH, connection, authenticated=False), 1.5)
        except IO_ERRORS:
            self.counters["warm_failed"] += 1


class RaceQueue:
    def __init__(self, helpers, token, concurrency, hedge, deadline, attempt):
        self.helpers = helpers
        self.token = token
        self.concurrency = concurrency
        self.hedge = hedge
        self.deadline = deadline
        self.attempt = attempt
        self.stats = {"served": 0, "failed": 0, "in_flight": 0, "waited_ms_max": 0}

    async def generate(self, disconnected):
        loop = asyncio.get_running_loop()
        started = loop.time()
        deadline = started + self.deadline
        # Compare measured proof latencies after obtaining a sample from each
        # ready helper. Until then the configured order provides the priority.
        helpers = [helper for helper in self.helpers if helper.available]
        if all(helper.latency is not None for helper in helpers):
            helpers.sort(key=lambda helper: helper.latency)
        active = {}

        def dispatch():
            while helpers:
                helper = helpers.pop(0)
                if helper.available:
                    budget = min(self.attempt, max(0, deadline - loop.time()))
                    if not budget:
                        return False
                    helper.in_use += 1
                    task = asyncio.create_task(helper.attempt(budget))
                    task.add_done_callback(helper.release)
                    active[task] = helper
                    return True
            return False

        try:
            if not dispatch():
                return None
            hedge_at = started + self.hedge
            while active and not disconnected.done():
                now = loop.time()
                if now >= deadline:
                    return None
                if len(active) == 1 and helpers and now >= hedge_at:
                    dispatch()
                wait_until = min(deadline, hedge_at) if len(active) == 1 and helpers else deadline
                done, _ = await asyncio.wait(
                    {*active, disconnected}, timeout=max(0, wait_until - loop.time()),
                    return_when=asyncio.FIRST_COMPLETED,
                )
                if disconnected in done:
                    return None
                winners = []
                for task in done:
                    helper = active.pop(task)
                    result = task.result()
                    if result is not None:
                        finished, data = result
                        winners.append((finished, data, helper))
                if winners:
                    _, data, helper = min(winners, key=lambda winner: winner[0])
                    helper.counters["wins"] += 1
                    return data
                if done:
                    hedge_at = loop.time()
                    if not active:
                        dispatch()
        finally:
            for task in active:
                task.cancel()
            await asyncio.gather(*active, return_exceptions=True)

    async def send(self, writer, status, body, close=False, content_type=b"application/json"):
        reason = {200: b"OK", 400: b"Bad Request", 401: b"Unauthorized", 404: b"Not Found",
                  503: b"Service Unavailable"}[status]
        writer.write(
            f"HTTP/1.1 {status} ".encode() + reason + b"\r\nContent-Type: " + content_type
            + f"\r\nContent-Length: {len(body)}\r\nCache-Control: no-store\r\n".encode()
            + (b"Connection: close\r\n" if close else b"") + b"\r\n" + body
        )
        await writer.drain()

    async def handle(self, reader, writer):
        try:
            while True:
                try:
                    first, headers = await asyncio.wait_for(read_head(reader), 15)
                    method, path, version = first.split()
                    if version != "HTTP/1.1" or headers.get("content-length", "0") != "0":
                        raise ValueError("invalid request")
                except (ValueError, asyncio.LimitOverrunError):
                    await self.send(writer, 400, b'{"error":"bad request"}', close=True)
                    return
                close = "close" in headers.get("connection", "").lower()
                operation = method, path
                if operation == ("GET", "/queue-health"):
                    queue = dict(self.stats, backends={
                        helper.name: dict(helper.counters, ready=helper.ready, idle=len(helper.idle),
                                          in_use=helper.in_use)
                        for helper in self.helpers
                    })
                    await self.send(writer, 200, str(queue).encode(), close, content_type=b"text/plain")
                elif not hmac.compare_digest(headers.get("authorization", "").encode("latin-1"), self.token):
                    await self.send(writer, 401, b'{"error":"unauthorized"}', close)
                elif operation == HEALTH:
                    ready = any(helper.ready for helper in self.helpers)
                    await self.send(writer, 200 if ready else 503,
                                    b'{"supported":true}' if ready else b'{"error":"attestation unavailable"}', close)
                elif operation == PROOF:
                    if self.stats["in_flight"] >= self.concurrency:
                        self.stats["failed"] += 1
                        await self.send(writer, 503, b'{"error":"attestation queue busy"}', close)
                    elif not await self.relay(reader, writer, close):
                        return
                else:
                    await self.send(writer, 404, b'{"error":"not found"}', close)
                if close:
                    return
        except (OSError, EOFError, asyncio.TimeoutError):
            pass
        finally:
            writer.close()

    async def relay(self, reader, writer, close):
        started = asyncio.get_running_loop().time()
        self.stats["in_flight"] += 1
        disconnected = asyncio.create_task(reader.read(1))
        try:
            data = await self.generate(disconnected)
            if disconnected.done():
                return False
        finally:
            self.stats["in_flight"] -= 1
            disconnected.cancel()
            await asyncio.gather(disconnected, return_exceptions=True)
        elapsed = int((asyncio.get_running_loop().time() - started) * 1000)
        self.stats["waited_ms_max"] = max(self.stats["waited_ms_max"], elapsed)
        self.stats["served" if data else "failed"] += 1
        await self.send(writer, 200 if data else 503, data or b'{"error":"attestation unavailable"}', close)
        return True


def load_queue(path, concurrency, warm, refresh):
    path = pathlib.Path(path).resolve()
    with path.open() as source:
        config = json.load(source)
    required = {"frontend_token_file", "helpers"}
    optional = {"hedge_ms", "deadline_ms", "attempt_ms"}
    if not isinstance(config, dict) or not required <= config.keys() or config.keys() - required - optional:
        raise ValueError("invalid race configuration")
    hedge = config.get("hedge_ms", 0)
    deadline = config.get("deadline_ms", 7000)
    attempt = config.get("attempt_ms", 3500)
    if any(type(value) is not int for value in (hedge, deadline, attempt)):
        raise ValueError("race timeouts must be integers")
    if not 0 <= hedge < attempt <= deadline < 8000:
        raise ValueError("invalid race timeouts")
    if not isinstance(config["helpers"], list) or not 2 <= len(config["helpers"]) <= 3:
        raise ValueError("configure two or three race helpers")
    helpers = []
    for spec in config["helpers"]:
        if not isinstance(spec, dict) or spec.keys() != {"name", "port", "token_file", "concurrency"}:
            raise ValueError("invalid race helper")
        name, port, limit = spec["name"], spec["port"], spec["concurrency"]
        if not isinstance(name, str) or not re.fullmatch(r"[a-z][a-z0-9-]{0,30}", name):
            raise ValueError("invalid helper name")
        if type(port) is not int or not 1 <= port <= 65535 or type(limit) is not int or not 1 <= limit <= 64:
            raise ValueError("invalid helper port or capacity")
        if any(name == helper.name or port == helper.port for helper in helpers):
            raise ValueError("duplicate name or port")
        helpers.append(Helper(name, port, read_token(path.parent / spec["token_file"]), limit, min(warm, limit), refresh))
    token = read_token(path.parent / config["frontend_token_file"])
    return RaceQueue(helpers, token, concurrency, hedge / 1000, deadline / 1000, attempt / 1000)


async def serve(queue, listen):
    if any(port == listen[1] for port in (helper.port for helper in queue.helpers)):
        raise ValueError("helper loopback port matches the queue listener")
    server = await asyncio.start_server(queue.handle, *listen, limit=8192, backlog=128)
    if not listen[1]:
        print("staging loopback port", server.sockets[0].getsockname()[1], flush=True)
    workers = [asyncio.create_task(helper.maintain()) for helper in queue.helpers]
    try:
        async with server:
            await server.serve_forever()
    finally:
        for worker in workers:
            worker.cancel()
        await asyncio.gather(*workers, return_exceptions=True)
