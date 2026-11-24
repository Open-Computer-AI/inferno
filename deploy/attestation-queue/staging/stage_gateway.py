#!/usr/bin/env python3
"""Exercise the real gateway binary with synthetic data on an internal Docker network."""

import argparse
import asyncio
import hashlib
import http.server
import json
import pathlib
import secrets
import ssl
import subprocess
import sys
import tempfile
import threading
import time

from stage_attestation import Fixture, candidate, queue_state, ready, summary, write_token
from attest_race import read_head


GATEWAY_PORT = 19480
STUB_ANSWER = b"v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=stage-fixture\r\nt=0 0\r\n"
START_BODY = json.dumps({"sdp": STUB_ANSWER.decode(), "session": {"model": "gpt-live"}}).encode()
GATEWAY_SHA256 = "6890e38552c77ee0ebc086c75f729597a173e732a353f09d078afd53a050bc2d"


class Upstream(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    proofs = set()
    mutex = threading.Lock()

    def log_message(self, *_):
        pass

    def respond(self, status, data=b"", location=""):
        self.send_response(status)
        self.send_header("Content-Length", str(len(data)))
        if location:
            self.send_header("Location", location)
            self.send_header("Content-Type", "application/sdp")
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        try:
            token = json.loads(self.headers["x-oai-attestation"])["t"]
            assert token.startswith("v1.stage-fixture-backup-")
            assert self.path.startswith("/backend-api/codex/realtime/calls?")
            assert self.headers["Authorization"] == "Bearer isolated-fake-oauth"
            assert json.loads(body)["session"]["model"] == "gpt-live"
            with self.mutex:
                assert token not in self.proofs
                self.proofs.add(token)
        except (KeyError, ValueError, TypeError, AssertionError):
            self.respond(422, b"isolated upstream contract failed")
            return
        self.respond(201, STUB_ANSWER, "/backend-api/codex/call_stage_" + secrets.token_hex(10))

    def do_GET(self):
        # An isolated upstream session has already ended when the gateway's
        # background observer dials it. Its terminal error releases staging leases.
        self.respond(404)

    def do_DELETE(self):
        self.respond(204)


class Relay:
    """Test-only switch between independently warmed candidate subprocesses."""

    def __init__(self):
        self.port = None
        self.connections = set()

    def select(self, port):
        self.port = port
        for writer in tuple(self.connections):
            writer.close()

    async def handle(self, incoming, client):
        self.connections.add(client)
        upstream = None
        try:
            if self.port is None:
                return
            outgoing, upstream = await asyncio.open_connection("127.0.0.1", self.port)

            async def copy(source, target):
                while data := await source.read(32768):
                    target.write(data)
                    await target.drain()

            tasks = [asyncio.create_task(copy(incoming, upstream)), asyncio.create_task(copy(outgoing, client))]
            try:
                await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
            finally:
                for task in tasks:
                    task.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
        except OSError:
            pass
        finally:
            self.connections.discard(client)
            client.close()
            if upstream is not None:
                upstream.close()


async def start_call(api_key, event, reader, writer):
    try:
        await event.wait()
        started = asyncio.get_running_loop().time()
        writer.write(
            f"POST /v1/live HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer {api_key}\r\n"
            f"Content-Type: application/json\r\nContent-Length: {len(START_BODY)}\r\nConnection: close\r\n\r\n".encode()
            + START_BODY
        )
        await writer.drain()
        first, headers = await read_head(reader)
        data = await reader.readexactly(int(headers["content-length"]))
        status = int(first.split()[1])
        elapsed = (asyncio.get_running_loop().time() - started) * 1000
        ok = status == 200 and data == STUB_ANSWER and "/v1/live/call_stage_" in headers.get("location", "")
        if not ok:
            # Fixture-only response has no real credentials or customer data.
            print(f"gateway fixture failure {status}: {data[:300].decode(errors='replace')}", file=sys.stderr)
        return {"status": status, "ok": ok, "ms": round(elapsed, 3)}
    finally:
        writer.close()
        await writer.wait_closed()


async def live_wave(api_key, width):
    connections = await asyncio.gather(
        *(asyncio.open_connection("127.0.0.1", GATEWAY_PORT) for _ in range(width))
    )
    event = asyncio.Event()
    calls = [asyncio.create_task(start_call(api_key, event, *connection)) for connection in connections]
    event.set()
    return await asyncio.wait_for(asyncio.gather(*calls), 9)


async def run_fixture(directory, waves):
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(directory / "ca.pem", directory / "ca-key.pem")
    upstream = http.server.ThreadingHTTPServer(("0.0.0.0", 443), Upstream)
    upstream.socket = tls.wrap_socket(upstream.socket, server_side=True)
    thread = threading.Thread(target=upstream.serve_forever, daemon=True)
    thread.start()
    relay = Relay()
    relay_server = await asyncio.start_server(relay.handle, "127.0.0.1", 19095)
    air_token, air_auth = write_token(directory, "fixture-air")
    backup_token, backup_auth = write_token(directory, "fixture-backup")
    front_token = directory / "gateway.token"
    api_key = (directory / "stage-api-key").read_text().strip()
    rows = []
    warmup = 0
    try:
        async with relay_server, Fixture("air", air_auth, .08) as air, Fixture("backup", backup_auth, .09) as backup:
            (directory / "fixture-ready").touch()
            deadline = asyncio.get_running_loop().time() + 90
            while not (directory / "run-live").exists():
                if asyncio.get_running_loop().time() >= deadline:
                    raise RuntimeError("gateway startup did not complete")
                await asyncio.sleep(.1)
            for fault in ("stall", "reset"):
                for width in (1, 4, 16):
                    for trial in range(waves):
                        for hedge in ((0, 200) if trial % 2 == 0 else (200, 0)):
                            cfg = directory / "race.json"
                            cfg.write_text(json.dumps({
                                "frontend_token_file": front_token.name, "hedge_ms": hedge,
                                "helpers": [
                                    {"name": "air", "port": air.port, "token_file": air_token.name, "concurrency": 16},
                                    {"name": "backup", "port": backup.port, "token_file": backup_token.name, "concurrency": 16},
                                ],
                            }))
                            air.mode = "healthy"
                            queue_file = pathlib.Path(__file__).with_name("attest_queue.py")
                            async with candidate(queue_file, cfg, width) as port:
                                await ready(port, width)
                                air.mode = fault
                                relay.select(port)
                                if not warmup:
                                    preliminary = await live_wave(api_key, 1)
                                    if not preliminary[0]["ok"]:
                                        raise RuntimeError("isolated gateway warmup failed")
                                    warmup = 1
                                    await ready(port, width)
                                completed = await live_wave(api_key, width)
                                observed = await queue_state(port)
                                expected = width + int(not rows)
                                if (observed["backends"]["backup"].get("wins") != expected or
                                        observed["backends"]["air"].get("attempts", 0) == 0 or
                                        observed["in_flight"] != 0):
                                    raise RuntimeError("missing fallback evidence from a Live trial")
                                for entry in completed:
                                    rows.append(dict(entry, fault=fault, concurrency=width, hedge_ms=hedge, wave=trial))
                                relay.select(None)
                                if not all(entry["ok"] for entry in completed):
                                    raise RuntimeError("isolated gateway Live creation failed")
            if len(Upstream.proofs) != len(rows) + warmup:
                raise RuntimeError("expected one upstream Live create per attestation")
    finally:
        upstream.shutdown()
        upstream.server_close()
        thread.join()
        relay.select(None)
    groups = [
        {"fault": fault, "concurrency": width, "hedge_ms": hedge,
         **summary([row for row in rows if (row["fault"], row["concurrency"], row["hedge_ms"]) == (fault, width, hedge)])}
        for fault in ("stall", "reset") for width in (1, 4, 16) for hedge in (0, 200)
    ]
    (directory / "gateway-result.json").write_text(json.dumps({
        "gateway_sha256": GATEWAY_SHA256, "upstream": "synthetic TLS/SDP fixture inside internal Docker network",
        "waves_per_cell": waves, "excluded_warmup": warmup,
        "one_upstream_create_per_call": True, "groups": groups, "requests": rows,
    }, indent=2) + "\n")


def docker(*args, data=None, check=True, timeout=50):
    proc = subprocess.run(["docker", *args], input=data, text=True, capture_output=True, timeout=timeout)
    if check and proc.returncode:
        # Avoid echoing an invocation containing fixture credentials.
        raise RuntimeError("docker " + args[0] + " failed: " + proc.stderr.strip()[-550:])
    return proc


def seed(postgres, key):
    return docker("exec", "-i", postgres, "psql", "-XAt", "-U", "sub2api", "-d", "sub2api", "-v", "ON_ERROR_STOP=1",
                  data=f"""
INSERT INTO accounts(id,name,platform,type,credentials,extra,concurrency,priority,status,schedulable,created_at,updated_at)
VALUES (28,'Isolated stage','openai','oauth',
 '{{"access_token":"isolated-fake-oauth","expires_at":"2099-01-01T00:00:00Z","chatgpt_account_id":"stage-test-account"}}',
 '{{}}',16,50,'active',true,now(),now());
WITH g AS (INSERT INTO groups(name,platform,status,subscription_type,allow_live,created_at,updated_at)
 VALUES ('Isolated stage','openai','active','standard',true,now(),now()) RETURNING id),
u AS (INSERT INTO users(email,password_hash,role,balance,concurrency,status,created_at,updated_at)
 VALUES ('attestation-stage@example.invalid','disabled-test-login','user',1000,16,'active',now(),now()) RETURNING id),
b AS (INSERT INTO account_groups(account_id,group_id,priority,created_at) SELECT 28,id,50,now() FROM g)
INSERT INTO api_keys(user_id,key,name,group_id,status,expires_at,created_at,updated_at)
 SELECT u.id,'{key}','stage',g.id,'active',now()+interval '10 minutes',now(),now() FROM u,g;
""")


def wait(check, seconds=30):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(.25)
    raise RuntimeError("isolated gateway fixture readiness timed out")


def live_leases(redis):
    script = """
local count = 0
for _, key in ipairs(redis.call('KEYS', 'concurrency:*')) do
    if string.match(key, '^concurrency:live:%a+_?%a*:%d+$')
        or string.match(key, '^concurrency:%a+_?%a*:%d+$') then
        if redis.call('TYPE', key).ok == 'zset' then
            count = count + redis.call('ZCARD', key)
        end
    end
end
return count
"""
    return int(docker("exec", redis, "redis-cli", "--raw", "EVAL", script, "0", timeout=5).stdout)


def run(args):
    if hashlib.sha256(args.gateway.read_bytes()).hexdigest() != GATEWAY_SHA256:
        raise RuntimeError("unexpected gateway binary")
    code = pathlib.Path(__file__).resolve().parent
    label = "inferno-attest-stage-" + secrets.token_hex(5)
    names = {role: f"{label}-{role}" for role in ("fixture", "gateway", "pg", "redis")}
    started = []
    with tempfile.TemporaryDirectory(prefix="inferno-gateway-stage-") as temp:
        directory = pathlib.Path(temp)
        # The temp directory is mounted by Docker Desktop. Its files contain
        # staging-only credentials and the private key for the synthetic upstream.
        write_token(directory, "gateway")
        key = "sk-" + secrets.token_urlsafe(36)
        (directory / "stage-api-key").write_text(key)
        password = secrets.token_urlsafe(30)
        (directory / "postgres.env").write_text(
            f"POSTGRES_USER=sub2api\nPOSTGRES_DB=sub2api\nPOSTGRES_PASSWORD={password}\n"
        )
        config = {
            "server": {"host": "0.0.0.0", "port": GATEWAY_PORT, "mode": "release"},
            "database": {"host": names["pg"], "port": 5432, "user": "sub2api", "password": password,
                         "dbname": "sub2api", "sslmode": "disable"},
            "redis": {"host": names["redis"], "port": 6379, "db": 0},
            "jwt": {"secret": secrets.token_urlsafe(48)}, "totp": {"encryption_key": secrets.token_hex(32)},
            "token_refresh": {"enabled": False}, "gateway": {"live": {"max_session_duration_seconds": 15}},
            "timezone": "UTC",
        }
        (directory / "config.yaml").write_text(json.dumps(config))
        subprocess.run([
            "openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048", "-days", "1",
            "-subj", "/CN=chatgpt.com", "-addext", "subjectAltName=DNS:chatgpt.com,DNS:api.openai.com",
            "-keyout", str(directory / "ca-key.pem"), "-out", str(directory / "ca.pem"),
        ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=7)
        docker("network", "create", "--internal", "--label", f"inferno.attest.stage={label}", label)

        def start(role, *params, net=None):
            name = names[role]
            started.append(name)
            docker("run", "-d", "--rm", "--name", name, "--network", net or label,
                   "--label", f"inferno.attest.stage={label}", *params)

        def gateway():
            start("gateway", "--memory", "768m", "-e", "SKIP_SETUP=true", "-e", "TOKEN_REFRESH_ENABLED=false",
                  "-e", "CONFIG_FILE=/stage/config.yaml", "-e", "SSL_CERT_FILE=/stage/ca.pem",
                  "-e", "LIVE_ATTESTATION_URL=http://127.0.0.1:19095",
                  "-e", "LIVE_ATTESTATION_TOKEN_FILE=/stage/gateway.token",
                  "-v", f"{directory}:/stage:ro", "-v", f"{args.gateway}:/gateway:ro",
                  "--tmpfs", "/app/data", "-w", "/app", "python:3.13-slim", "/gateway",
                  net="container:" + names["fixture"])
            wait(lambda: docker("exec", names["fixture"], "python", "-c",
                               f'import urllib.request; urllib.request.urlopen("http://127.0.0.1:{GATEWAY_PORT}/health", timeout=1)',
                               check=False, timeout=5).returncode == 0, 40)

        try:
            start("pg", "--memory", "384m", "--env-file", str(directory / "postgres.env"),
                  "--tmpfs", "/var/lib/postgresql/data", "postgres:17")
            start("redis", "--memory", "128m", "--tmpfs", "/data", "redis:7-alpine", "redis-server", "--save", "")
            start("fixture", "--network-alias", "chatgpt.com", "--network-alias", "api.openai.com",
                  "-v", f"{directory}:/stage", "-v", f"{code}:/code:ro",
                  "python:3.13-slim", "python", "-B", "/code/stage_gateway.py", "--fixture", "/stage",
                  "--waves", str(args.waves))
            wait(lambda: (directory / "fixture-ready").exists())
            wait(lambda: docker("exec", names["pg"], "pg_isready", "-U", "sub2api", check=False).returncode == 0)
            gateway()
            seed(names["pg"], key)
            docker("stop", "-t", "2", names["gateway"])
            wait(lambda: docker("container", "inspect", names["gateway"], check=False).returncode != 0)
            docker("exec", names["redis"], "redis-cli", "FLUSHDB")
            gateway()
            (directory / "run-live").touch()
            wait(lambda: (directory / "gateway-result.json").is_file()
                 or docker("container", "inspect", names["fixture"], check=False).returncode != 0, 45 + 10 * args.waves)
            result = directory / "gateway-result.json"
            if not result.is_file():
                error = directory / "fixture-error.txt"
                raise RuntimeError(error.read_text() if error.is_file() else "isolated Live fixture exited")
            wait(lambda: live_leases(names["redis"]) == 0, 30)
            evidence = json.loads(result.read_text())
            evidence["remaining_gateway_concurrency_leases"] = 0
            args.output.write_text(json.dumps(evidence, indent=2) + "\n")
            print(json.dumps(evidence["groups"], indent=2))
        finally:
            for name in dict.fromkeys(reversed(started)):
                docker("stop", "-t", "2", name, check=False, timeout=12)
            docker("network", "rm", label, check=False, timeout=12)
            if docker("ps", "-aq", "--filter", f"label=inferno.attest.stage={label}").stdout.strip():
                raise RuntimeError("staging container cleanup incomplete")
            if docker("network", "ls", "-q", "--filter", f"label=inferno.attest.stage={label}").stdout.strip():
                raise RuntimeError("staging network cleanup incomplete")
            print("Isolation cleanup verified.")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--gateway", type=lambda name: pathlib.Path(name).resolve())
    parser.add_argument("--output", type=pathlib.Path)
    parser.add_argument("--fixture", type=pathlib.Path)
    parser.add_argument("--waves", type=int, default=2)
    args = parser.parse_args()
    if not 1 <= args.waves <= 20:
        parser.error("gateway fixture supports one to twenty waves per cell")
    if args.fixture:
        try:
            asyncio.run(run_fixture(args.fixture, args.waves))
        except Exception as exc:
            (args.fixture / "fixture-error.txt").write_text(str(exc))
            raise
    else:
        if not args.gateway or not args.output:
            parser.error("provide --gateway and --output")
        run(args)


if __name__ == "__main__":
    main()
