#!/usr/bin/env python3
"""Loopback-only failover benchmark with deterministic faults and an optional native backup."""

import argparse
import ast
import asyncio
import contextlib
import itertools
import json
import os
import pathlib
import random
import secrets
import socket
import statistics
import struct
import sys
import tempfile

from attest_race import HEALTH, PROOF, read_head, response_size, valid_payload


async def wait_for(check, timeout=12):
    async def poll():
        while not check():
            await asyncio.sleep(0.01)
    await asyncio.wait_for(poll(), timeout)


def mint(suffix):
    proof = json.dumps({"v": 1, "s": 0, "t": "v1.stage-fixture-" + suffix})
    return json.dumps({"attestation": proof}).encode()


async def reply(writer, status, data):
    writer.write(f"HTTP/1.1 {status} Fixture\r\nContent-Length: {len(data)}\r\n\r\n".encode() + data)
    await writer.drain()


def request_bytes(operation, token):
    method, path = operation
    line = f"{method} {path} HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n".encode()
    return line + (b"Authorization: " + token + b"\r\n" if token else b"") + b"\r\n"


async def request(port, operation, token=None):
    reader, writer = await asyncio.open_connection("127.0.0.1", port)
    started = asyncio.get_running_loop().time()
    try:
        writer.write(request_bytes(operation, token))
        await writer.drain()
        status, data = await read_reply(reader)
        elapsed = (asyncio.get_running_loop().time() - started) * 1000
        return status, data, elapsed
    finally:
        writer.close()
        await writer.wait_closed()


async def read_reply(reader):
    first, headers = await read_head(reader)
    return int(first.split()[1]), await reader.readexactly(response_size(headers))


class Fixture:
    def __init__(self, name, token, delay):
        self.name = name
        self.token = token
        self.delay = delay
        self.mode = "healthy"
        self.hang_health = False
        self.attempts = 0
        self.pending = 0
        self.max_pending = 0
        self.writers = set()
        self.server = None

    async def __aenter__(self):
        self.server = await asyncio.start_server(self.handle, "127.0.0.1", 0)
        self.port = self.server.sockets[0].getsockname()[1]
        return self

    async def __aexit__(self, *_):
        self.server.close()
        await self.server.wait_closed()
        for writer in tuple(self.writers):
            writer.close()

    async def handle(self, reader, writer):
        self.writers.add(writer)
        connection = writer.get_extra_info("socket")
        try:
            while True:
                first, headers = await read_head(reader)
                method, path, _ = first.split()
                if (method, path) == HEALTH and self.hang_health:
                    await reader.read()
                    return
                if headers.get("authorization", "").encode() != self.token:
                    await reply(writer, 401, b'{"error":"unauthorized"}')
                    continue
                if (method, path) == HEALTH:
                    await reply(writer, 200, b'{"supported":true}')
                    continue
                if (method, path) != PROOF:
                    await reply(writer, 404, b'{}')
                    continue
                self.attempts += 1
                sequence = self.attempts
                self.pending += 1
                self.max_pending = max(self.pending, self.max_pending)
                try:
                    if self.mode == "stall":
                        await reader.read()
                        return
                    if self.mode == "reset":
                        connection.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack("ii", 1, 0))
                        writer.transport.abort()
                        return
                    if self.mode == "busy":
                        await reply(writer, 429, b'{}')
                        continue
                    await asyncio.sleep(self.delay + 0.003 * (sequence % 7))
                    await reply(writer, 200, b'{"attestation":"bad proof"}' if self.mode == "malformed"
                                else mint(f"{self.name}-{sequence}"))
                finally:
                    self.pending -= 1
        except (OSError, EOFError):
            pass
        finally:
            self.writers.discard(writer)
            writer.close()


def write_token(directory, name):
    key = secrets.token_urlsafe(40)
    path = directory / f"{name}.token"
    path.touch(mode=0o600)
    path.write_text(key)
    return path, ("Bearer " + key).encode()


@contextlib.asynccontextmanager
async def launch_native(binary, directory, name, token=None):
    if token is None:
        token = write_token(directory, name)
    path, auth = token
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    proc = await asyncio.create_subprocess_exec(
        binary, "-listen", f"127.0.0.1:{port}", "-token-file", str(path),
        "-max-concurrent", "16", stdout=asyncio.subprocess.DEVNULL, stderr=asyncio.subprocess.DEVNULL,
    )
    try:
        for _ in range(60):
            if proc.returncode is not None:
                raise RuntimeError("native staging helper exited on startup")
            try:
                status, data, _ = await asyncio.wait_for(request(port, HEALTH, auth), 0.5)
                if status == 200 and valid_payload(HEALTH, data):
                    break
            except (OSError, EOFError, asyncio.TimeoutError):
                pass
            await asyncio.sleep(0.15)
        else:
            raise RuntimeError("native staging helper did not become ready")
        yield port
    finally:
        if proc.returncode is None:
            proc.terminate()
        try:
            await asyncio.wait_for(proc.wait(), 5)
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()


@contextlib.asynccontextmanager
async def candidate(queue_file, config_path, width):
    # Port zero makes the candidate self-select a free loopback port. The
    # subprocess prints that port only for the bounded staging harness.
    env = dict(PATH=os.environ.get("PATH", os.defpath), QUEUE_PORT="0", QUEUE_CONCURRENCY="16",
               QUEUE_KEEPALIVE="1", QUEUE_WARM=str(width), QUEUE_WARM_REFRESH_S="5",
               QUEUE_RACE_CONFIG=str(config_path))
    proc = await asyncio.create_subprocess_exec(
        sys.executable, "-B", str(queue_file), env=env,
        stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
    )
    try:
        first = await asyncio.wait_for(proc.stdout.readline(), 5)
        if not first.startswith(b"staging loopback port "):
            raise RuntimeError("candidate did not report its loopback port")
        yield int(first.split()[-1])
    finally:
        if proc.returncode is None:
            proc.terminate()
        try:
            await asyncio.wait_for(proc.communicate(), 5)
        except asyncio.TimeoutError:
            proc.kill()
            await proc.communicate()


async def queue_state(port):
    status, data, _ = await request(port, ("GET", "/queue-health"))
    if status != 200:
        raise RuntimeError("queue status unavailable")
    return ast.literal_eval(data.decode())


async def ready(port, width, timeout=8):
    async def poll():
        while True:
            state = await queue_state(port)
            if all(item["ready"] and item["idle"] >= width and not item["in_use"]
                   for item in state["backends"].values()):
                return state
            await asyncio.sleep(0.025)
    return await asyncio.wait_for(poll(), timeout)


async def wave(port, width, token):
    connections = await asyncio.gather(*(asyncio.open_connection("127.0.0.1", port) for _ in range(width)))
    event = asyncio.Event()
    loop = asyncio.get_running_loop()

    async def one(reader, writer):
        try:
            await event.wait()
            started = loop.time()
            writer.write(request_bytes(PROOF, token))
            await writer.drain()
            status, data = await read_reply(reader)
            elapsed = (loop.time() - started) * 1000
            proof = json.loads(data).get("attestation") if status == 200 and valid_payload(PROOF, data) else None
            return status, proof, elapsed
        finally:
            writer.close()
            await writer.wait_closed()

    callers = [asyncio.create_task(one(reader, writer)) for reader, writer in connections]
    event.set()
    return await asyncio.wait_for(asyncio.gather(*callers), 9)


def percentile(values, fraction):
    values = sorted(values)
    index = fraction * (len(values) - 1)
    lower = int(index)
    return values[lower] + (values[min(lower + 1, len(values) - 1)] - values[lower]) * (index - lower)


def summary(samples):
    values = [row["ms"] for row in samples if row["ok"]]
    return {"requests": len(samples), "failed": len(samples) - len(values),
            "p50_ms": round(statistics.median(values), 2) if values else None,
            "p95_ms": round(percentile(values, .95), 2) if values else None,
            "p99_ms": round(percentile(values, .99), 2) if values else None,
            "max_ms": round(max(values), 2) if values else None}


async def benchmark(args):
    source = pathlib.Path(__file__).resolve().parent
    rng = random.Random(4798)
    rows = []
    proofs = set()
    with tempfile.TemporaryDirectory(prefix="inferno-attest isolated ") as temp:
        tmp = pathlib.Path(temp)
        air_path, air_auth = write_token(tmp, "air")
        backup_path, backup_auth = write_token(tmp, "backup")
        front_path, front_auth = write_token(tmp, "gateway")
        async with contextlib.AsyncExitStack() as exits:
            air = await exits.enter_async_context(Fixture("air", air_auth, 0.08))
            if args.native_helper:
                backup_port = await exits.enter_async_context(
                    launch_native(args.native_helper, tmp, "backup", (backup_path, backup_auth))
                )
                code, body, _ = await asyncio.wait_for(request(backup_port, PROOF, backup_auth), 7.5)
                if code != 200 or not valid_payload(PROOF, body):
                    raise RuntimeError("native staging proof warmup failed")
            else:
                backup = await exits.enter_async_context(Fixture("backup", backup_auth, 0.09))
                backup_port = backup.port
            for mode, width in itertools.product(args.faults, (1, 4, 16)):
                order = [0, args.head_start_ms]
                rng.shuffle(order)
                for repeat in range(args.waves):
                    for hedge in (order if repeat % 2 == 0 else order[::-1]):
                        config = {
                            "frontend_token_file": front_path.name, "hedge_ms": hedge,
                            "helpers": [
                                {"name": "air", "port": air.port, "token_file": air_path.name, "concurrency": 16},
                                {"name": "backup", "port": backup_port, "token_file": backup_path.name, "concurrency": 16},
                            ],
                        }
                        config_path = tmp / "stage.json"
                        config_path.write_text(json.dumps(config))
                        air.mode = "healthy"
                        async with candidate(source / "attest_queue.py", config_path, width) as port:
                            await ready(port, width)
                            air.mode = mode
                            before_air = air.attempts
                            results = await wave(port, width, front_auth)
                            after = await queue_state(port)
                            await wait_for(lambda: air.pending == 0)
                            expected_air = after["backends"]["air"].get("attempts", 0)
                            if not expected_air or not air.attempts - before_air:
                                raise RuntimeError("Air fault was never exercised")
                            if after["in_flight"] or any(b["in_use"] for b in after["backends"].values()):
                                raise RuntimeError("stage requests leaked helper slots")
                            for status, proof, ms in results:
                                unique = proof is not None and proof not in proofs
                                if proof is not None:
                                    proofs.add(proof)
                                rows.append({"fault": mode, "concurrency": width, "hedge_ms": hedge, "wave": repeat,
                                             "status": status, "ok": bool(unique), "ms": round(ms, 3)})
                            if mode in {"stall", "reset"} and after["backends"]["backup"].get("wins", 0) != width:
                                raise RuntimeError("staging fallback did not win every faulted request")
                            if mode in {"stall", "reset"} and after["backends"]["backup"].get("pooled_proofs", 0) != width:
                                raise RuntimeError("a fallback proof used a cold socket")
    groups = []
    for mode, width, hedge in itertools.product(args.faults, (1, 4, 16), (0, args.head_start_ms)):
        subset = [row for row in rows if (row["fault"], row["concurrency"], row["hedge_ms"]) == (mode, width, hedge)]
        groups.append({"fault": mode, "concurrency": width, "hedge_ms": hedge, **summary(subset)})
    evidence = {"backend": "native macOS backup over loopback" if args.native_helper else "synthetic loopback helpers",
                "frontdoor": "real queue HTTP, concurrent attestation requests", "waves_per_cell": args.waves,
                "groups": groups, "requests": rows}
    args.output.write_text(json.dumps(evidence, indent=2) + "\n")
    print(json.dumps(groups, indent=2))
    if any(row["failed"] for row in groups):
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--native-helper")
    parser.add_argument("--waves", type=int, default=15)
    parser.add_argument("--head-start-ms", type=int, default=200)
    parser.add_argument("--faults", nargs="+", choices=("stall", "reset", "healthy"), default=("stall", "reset"))
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    if not 1 <= args.waves <= 100 or not 1 <= args.head_start_ms <= 1000:
        parser.error("invalid waves or head start")
    asyncio.run(benchmark(args))


if __name__ == "__main__":
    main()
