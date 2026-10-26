#!/usr/bin/env python3
"""Isolated fault injection in front of candidate helpers, never production ports."""
import asyncio
import json
import os
import pathlib
import signal
import subprocess
import sys
import time

from backup_helpers import owned_file

TARGETS = {'this-mac': (19488, 19485), 'other-pro': (19489, 19486)}
MODES = {'healthy', 'refuse', 'drop', 'hang', 'busy', 'malformed'}


async def serve(mode_file):
    state_path = mode_file.parent / 'live-remote-root.json'
    owned_file(state_path, 0o600)
    run_id = json.loads(state_path.read_text())['run_id']
    started = subprocess.check_output(['ps', '-p', str(os.getpid()), '-o', 'lstart='], text=True).strip()
    stop = asyncio.Event()
    tasks = {name: set() for name in TARGETS}
    servers = {}
    modes = {}
    counts = {name: {'accepted': 0, 'forwarded': 0, 'closed': 0} for name in TARGETS}
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        loop.add_signal_handler(sig, stop.set)

    async def pipe(reader, writer):
        while data := await reader.read(8192):
            writer.write(data)
            await writer.drain()

    async def handle(name, reader, writer):
        task = asyncio.current_task()
        tasks[name].add(task)
        counts[name]['accepted'] += 1
        upstream = None
        pipes = []
        try:
            mode = modes[name]
            if mode in {'refuse', 'drop'}:
                return
            if mode == 'hang':
                await reader.read()
                return
            if mode in {'busy', 'malformed'}:
                await asyncio.wait_for(reader.readuntil(b'\r\n\r\n'), 2)
                body = b'{}' if mode == 'busy' else b'{"attestation":"invalid"}'
                status = 429 if mode == 'busy' else 200
                writer.write(f'HTTP/1.1 {status} Fixture\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n'.encode() + body)
                await writer.drain()
                return
            upstream_reader, upstream = await asyncio.open_connection('127.0.0.1', TARGETS[name][1])
            counts[name]['forwarded'] += 1
            pipes = [asyncio.create_task(pipe(reader, upstream)), asyncio.create_task(pipe(upstream_reader, writer))]
            await asyncio.wait(pipes, return_when=asyncio.FIRST_COMPLETED)
        except (OSError, asyncio.IncompleteReadError, asyncio.LimitOverrunError, asyncio.TimeoutError):
            pass
        finally:
            for item in pipes:
                item.cancel()
            await asyncio.gather(*pipes, return_exceptions=True)
            for stream in (upstream, writer):
                if stream:
                    stream.close()
                    try:
                        await asyncio.wait_for(stream.wait_closed(), .1)
                    except (OSError, asyncio.TimeoutError):
                        pass
            tasks[name].discard(task)
            counts[name]['closed'] += 1

    deadline = time.monotonic() + 3500
    print(json.dumps({'fault_proxy_started': True}), flush=True)
    try:
        while not stop.is_set() and time.monotonic() < deadline and not (mode_file.parent / 'stop-fault-proxy').exists():
            owned_file(mode_file, 0o600)
            new_modes = json.loads(mode_file.read_text())
            if set(new_modes) != set(TARGETS) or any(x not in MODES for x in new_modes.values()):
                raise ValueError('invalid candidate fault mode')
            modes.update(new_modes)
            for name, (port, _) in TARGETS.items():
                if modes[name] == 'refuse':
                    server = servers.pop(name, None)
                    if server:
                        server.close()
                        await server.wait_closed()
                    for task in list(tasks[name]):
                        task.cancel()
                elif name not in servers:
                    async def bound_handler(reader, writer, name=name):
                        await handle(name, reader, writer)
                    servers[name] = await asyncio.start_server(bound_handler, '127.0.0.1', port, limit=8192)
            status = {'pid': os.getpid(), 'started': started, 'run_id': run_id, 'at': time.time(), 'modes': modes,
                      'active': {name: len(group) for name, group in tasks.items()}, 'counts': counts}
            temp = mode_file.parent / 'fault-status.tmp'
            temp.write_text(json.dumps(status))
            temp.chmod(0o600)
            temp.replace(mode_file.parent / 'fault-status.json')
            await asyncio.sleep(.1)
    finally:
        for server in servers.values():
            server.close()
            await server.wait_closed()
        active = [task for group in tasks.values() for task in group]
        for task in active:
            task.cancel()
        await asyncio.gather(*active, return_exceptions=True)
        print(json.dumps({'fault_proxy_stopped': True, 'active': sum(len(group) for group in tasks.values())}), flush=True)


if __name__ == '__main__':
    asyncio.run(serve(pathlib.Path(sys.argv[1])))
