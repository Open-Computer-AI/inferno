#!/usr/bin/env python3
"""Isolated Docker actor. Startup input is private stdin; output is metadata only."""
import hashlib
import json
import os
import pathlib
import re
import select
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

ALLOWED_FILES = {'manifest.json', 'database.dump', 'restore.list', 'server', 'probe',
                 'postgres.env', 'config.yaml', 'attestation-token'}


class Stopped(Exception):
    pass


class CommandFailed(Exception):
    pass


class Runner:
    def __init__(self):
        self.stopping = threading.Event()
        self.lock = threading.Lock()
        self.children = set()

    def start(self, args, stdin=None, stdout=None, stderr=subprocess.DEVNULL):
        with self.lock:
            if self.stopping.is_set():
                raise Stopped()
            child = subprocess.Popen(args, stdin=stdin, stdout=stdout,
                                     stderr=stderr, start_new_session=True,
                                     env=docker_environment())
            self.children.add(child)
            return child

    @staticmethod
    def terminate(child):
        for sig in (signal.SIGTERM, signal.SIGKILL):
            if child.poll() is not None:
                return
            try:
                os.killpg(child.pid, sig)
            except ProcessLookupError:
                return
            except PermissionError:
                # macOS can deny signaling a group containing only zombies.
                # Reap/observe the owned child; never mask a live-child denial.
                if child.poll() is not None:
                    return
                raise
            try:
                child.wait(timeout=.5)
                return
            except subprocess.TimeoutExpired:
                pass
        raise subprocess.TimeoutExpired(child.args, .5)

    def stop(self):
        self.stopping.set()
        with self.lock:
            children = list(self.children)
        for child in children:
            self.terminate(child)

    def run(self, args, data=None, stream=None, timeout=60):
        with tempfile.TemporaryFile() as output:
            child = self.start(args, stdin=stream if stream else subprocess.PIPE, stdout=output)
            try:
                if not stream:
                    if data:
                        child.stdin.write(data)
                    child.stdin.close()
                deadline = time.monotonic() + timeout
                while child.poll() is None:
                    if self.stopping.wait(.05):
                        raise Stopped()
                    if time.monotonic() > deadline:
                        raise TimeoutError()
                if child.returncode:
                    raise CommandFailed()
                output.seek(0)
                return output.read()
            finally:
                self.terminate(child)
                with self.lock:
                    self.children.discard(child)


def validate_root(root, run_id):
    if (not re.fullmatch('[a-f0-9]{12}', run_id)
            or root.parent.resolve() != pathlib.Path('/tmp').resolve()
            or not re.fullmatch('inferno-backup-live-[a-z0-9_]+', root.name)):
        raise ValueError('unexpected candidate root identity')
    info = root.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        raise ValueError('candidate root ownership/mode')
    manifest_path = root / 'manifest.json'
    private_file(manifest_path)
    manifest = json.loads(manifest_path.read_text())
    if manifest.get('run_id') != run_id or manifest.get('root') != str(root):
        raise ValueError('manifest identity mismatch')
    files = list(root.iterdir())
    if any(path.name not in ALLOWED_FILES for path in files):
        raise ValueError('unexpected candidate root contents')
    for path in files:
        private_file(path, executable=path.name in {'server', 'probe'})
    return manifest


def private_file(path, executable=False):
    info = path.lstat()
    modes = {0o700, 0o755} if executable else {0o600}
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) not in modes:
        raise ValueError('candidate file ownership/mode')


def sql_string(value):
    return "'" + str(value).replace("'", "''") + "'"


def docker_environment():
    return {key: value for key, value in os.environ.items()
            if not key.startswith('DOCKER_') and key.lower() not in
            {'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy'}}


def missing_object(kind, name, error):
    message = error.decode(errors='replace').lower()
    if name.lower() not in message:
        return False
    return 'no such' in message or bool(re.search(re.escape(kind + ' ' + name.lower()) + r' not found\b', message))


class Host:
    def __init__(self, root, run_id, emit=print, daemon=None):
        self.root, self.run_id, self.emit = root, run_id, emit
        self.runner = Runner()
        self.cleanup_lock = threading.Lock()
        self.docker = '/usr/local/bin/docker'
        if not pathlib.Path(self.docker).exists():
            self.docker = '/Applications/Docker.app/Contents/Resources/bin/docker'
        suffix = run_id
        self.names = {kind: 'inferno-backup-' + kind + '-' + suffix
                      for kind in ('postgres', 'redis', 'gateway', 'probe')}
        self.network = 'inferno-backup-network-' + suffix
        self.volume = 'inferno-backup-postgres-' + suffix
        self.labels = ['--label', 'inferno.backup.run=' + run_id,
                       '--label', 'inferno.backup.root=' + str(root)]
        self.refresh_disabled = threading.Event()
        self.refresh_started = False
        self.cleanup_result = None
        self.daemon = daemon
        self.daemon_checked = False
        self.docker_prefix = None
        self.phase = 'initialization'

    def event(self, **row):
        self.emit(json.dumps(row), flush=True)

    def docker_run(self, args, **options):
        self.pin_daemon()
        return self.runner.run(self.docker_prefix + args, **options)

    def pin_daemon(self, force=False):
        if self.daemon_checked and not force:
            return
        if self.daemon is None:
            self.daemon = validate_root(self.root, self.run_id)['docker']
        allowed = {'unix://' + str(pathlib.Path.home() / '.docker/run/docker.sock'), 'unix:///var/run/docker.sock'}
        endpoint = self.daemon['endpoint']
        if endpoint not in allowed:
            raise ValueError('local candidate Docker Unix socket required')
        socket_path = pathlib.Path(endpoint.removeprefix('unix://'))
        if not stat.S_ISSOCK(socket_path.stat().st_mode):
            raise ValueError('candidate Docker endpoint is not a Unix socket')
        self.docker_prefix = [self.docker, '--host', endpoint]
        result = subprocess.run(self.docker_prefix + ['info', '--format', '{{json .ID}}'],
                                stdin=subprocess.DEVNULL, capture_output=True, timeout=5,
                                env=docker_environment(), check=True)
        if json.loads(result.stdout) != self.daemon['id']:
            raise ValueError('candidate Docker daemon identity changed')
        self.daemon_checked = True

    def inspect(self, kind, name):
        self.pin_daemon()
        template = '{{json .Config.Labels}}' if kind == 'container' else '{{json .Labels}}'
        command = self.docker_prefix + [kind, 'inspect', '--format', template, name]
        result = subprocess.run(command, stdin=subprocess.DEVNULL, capture_output=True,
                                timeout=3, env=docker_environment())
        if result.returncode:
            if missing_object(kind, name, result.stderr):
                return None
            raise RuntimeError('Docker inspection unavailable')
        return json.loads(result.stdout) or {}

    def owned(self, labels):
        return labels.get('inferno.backup.run') == self.run_id and labels.get('inferno.backup.root') == str(self.root)

    def cleanup(self):
        stop_error = None
        try:
            self.runner.stop()
        except Exception as exc:
            stop_error = type(exc).__name__
        with self.cleanup_lock:
            if self.cleanup_result is not None and self.cleanup_result.get('candidate_live_cleaned'):
                return self.cleanup_result
            errors, absent = ([stop_error] if stop_error else []), False
            objects = [('container', name) for name in self.names.values()]
            objects += [('network', self.network), ('volume', self.volume)]
            deadline = time.monotonic() + 40
            try:
                self.pin_daemon(force=True)
                stable = 0
                while time.monotonic() < deadline and stable < 2:
                    for kind, name in objects:
                        if time.monotonic() > deadline:
                            raise TimeoutError('cleanup deadline')
                        labels = self.inspect(kind, name)
                        if labels is None:
                            continue
                        if not self.owned(labels):
                            raise RuntimeError('foreign object at candidate identity')
                        command = self.docker_prefix + (['rm', '-f', name] if kind == 'container' else [kind, 'rm', name])
                        result = subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                                stdin=subprocess.DEVNULL, timeout=3, env=docker_environment())
                        if result.returncode:
                            # Confirm absence; a second cleanup actor may have removed it.
                            if self.inspect(kind, name) is not None:
                                raise RuntimeError('candidate Docker removal failed')
                    checks = []
                    for kind, name in objects:
                        if time.monotonic() > deadline:
                            raise TimeoutError('cleanup deadline')
                        checks.append(self.inspect(kind, name) is None)
                    absent = all(checks)
                    stable = stable + 1 if absent else 0
                    time.sleep(.3)
                if not absent or stable < 2:
                    raise RuntimeError('Docker cleanup did not stabilize')
                if self.root.exists() and not errors:
                    validate_root(self.root, self.run_id)
                    shutil.rmtree(self.root)
            except Exception as exc:
                errors.append(type(exc).__name__)
                if isinstance(exc, subprocess.TimeoutExpired):
                    errors.append('daemon-info' if 'info' in exc.cmd else 'inspect' if 'inspect' in exc.cmd else 'remove')
                safe_reasons = {'Docker inspection unavailable', 'foreign object at candidate identity',
                                'candidate Docker removal failed', 'Docker cleanup did not stabilize',
                                'unexpected candidate root contents', 'candidate Docker daemon identity changed'}
                if str(exc) in safe_reasons:
                    errors.append(str(exc))
            result = {'candidate_live_cleaned': absent and not self.root.exists() and not errors,
                      'objects_absent': absent, 'root_removed': not self.root.exists(),
                      'errors': errors, 'run_id': self.run_id, 'root': str(self.root)}
            self.cleanup_result = result
            self.event(**result)
            return result

    def sql(self, text):
        return self.docker_run(['exec', '-i', self.names['postgres'], 'psql', '-v', 'ON_ERROR_STOP=1',
                               '-U', 'sub2api', '-d', 'sub2api', '-At'], data=text.encode())

    def provision(self, config):
        manifest = validate_root(self.root, self.run_id)
        for name in self.names.values():
            if self.inspect('container', name) is not None:
                raise RuntimeError('candidate container exists')
        for kind, name in [('network', self.network), ('volume', self.volume)]:
            if self.inspect(kind, name) is not None:
                raise RuntimeError('candidate storage exists')
        for name in ('server', 'probe'):
            if hashlib.sha256((self.root / name).read_bytes()).hexdigest() != manifest['hashes'][name]:
                raise RuntimeError('candidate executable identity mismatch')
        token = self.root / 'attestation-token'
        fd = os.open(token, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as stream:
            stream.write(config['frontend_token'])
        self.images = {}
        for image in ('postgres:18-alpine', 'redis:8-alpine', 'alpine:3.22'):
            expected_id = manifest['images'][image]['id']
            data = self.docker_run(['image', 'inspect', '--format', '{"Id":{{json .Id}},"Architecture":{{json .Architecture}},"RepoDigests":{{json .RepoDigests}}}', expected_id])
            details = json.loads(data)
            if details.get('Architecture') != 'arm64' or details['Id'] != expected_id:
                raise RuntimeError('candidate image architecture mismatch')
            self.images[image] = expected_id
            self.event(candidate_image=image, image_id=details['Id'], digests=details.get('RepoDigests', []), architecture='arm64')
        self.phase = 'create-storage'
        self.docker_run(['network', 'create'] + self.labels + [self.network])
        self.docker_run(['volume', 'create'] + self.labels + [self.volume])
        self.phase = 'database-start'
        self.docker_run(['run', '-d', '--name', self.names['postgres']] + self.labels +
                        ['--log-driver', 'none', '--memory', '512m', '--cpus', '1', '--network', self.network,
                         '--env-file', str(self.root / 'postgres.env'), '--mount',
                         'type=volume,source=' + self.volume + ',target=/var/lib/postgresql', self.images['postgres:18-alpine']])
        for _ in range(60):
            try:
                self.docker_run(['exec', self.names['postgres'], 'pg_isready', '-U', 'sub2api', '-d', 'sub2api'], timeout=2)
                break
            except CommandFailed:
                if self.runner.stopping.wait(.25):
                    raise Stopped()
        else:
            raise RuntimeError('candidate database readiness failed')
        self.phase = 'fresh-database'
        if manifest.get('fresh_database') is not True:
            raise ValueError('fresh candidate database required')
        for unused in ('database.dump', 'restore.list'):
            (self.root / unused).unlink(missing_ok=True)
        self.config = config
        self.docker_run(['run', '-d', '--name', self.names['redis']] + self.labels +
                        ['--log-driver', 'none', '--memory', '64m', '--network', self.network,
                         '--tmpfs', '/data:rw,size=32m', self.images['redis:8-alpine']])
        self.event(candidate_database_ready=True, fresh_database=True, imported_snapshot=False,
                   root=str(self.root), run_id=self.run_id, containers=self.names)

    def seed_fixture(self):
        self.phase = 'synthetic-fixture'
        access = self.config['account']['credentials']
        if (self.config['account']['id'] != 28 or
                set(access) != {'access_token', 'expires_at', 'chatgpt_account_id'} or
                not access['access_token']):
            raise ValueError('reviewed access-only account required')
        statement = "INSERT INTO accounts(id,name,platform,type,credentials,extra,concurrency,priority,status,schedulable,created_at,updated_at) VALUES (28,'Isolated backup test','openai','oauth'," + sql_string(json.dumps(access)) + "::jsonb,'{}'::jsonb,16,50,'active',true,now(),now());\n"
        self.sql(statement)
        selected = self.sql("WITH g AS (INSERT INTO groups(name,platform,status,subscription_type,allow_live,created_at,updated_at) VALUES ('Isolated backup validation','openai','active','standard',true,now(),now()) RETURNING id), u AS (INSERT INTO users(email,password_hash,role,balance,concurrency,status,created_at,updated_at) VALUES ('backup-" + self.run_id + "@example.invalid','disabled-test-login','user',1000,16,'active',now(),now()) RETURNING id), b AS (INSERT INTO account_groups(account_id,group_id,priority,created_at) SELECT 28,id,50,now() FROM g RETURNING group_id) INSERT INTO api_keys(user_id,key,name,group_id,status,expires_at,created_at,updated_at) SELECT u.id," + sql_string(self.config['test_key']) + ",'Isolated backup validation',g.id,'active',now()+interval '2 hours',now(),now() FROM u,g RETURNING json_build_object('id',id,'user_id',user_id,'group_id',group_id);\n").decode().splitlines()
        key_info = next((json.loads(line) for line in selected if line.startswith('{')), None)
        metadata = self.sql("SELECT json_build_object('refresh_keys',(SELECT count(*) FROM accounts WHERE credentials ? 'refresh_token'),'credential_accounts',(SELECT count(*) FROM accounts WHERE credentials<>'{}'::jsonb),'active_keys',(SELECT count(*) FROM api_keys WHERE status='active'),'historical_usage',(SELECT count(*) FROM usage_logs));\n").decode().strip()
        evidence = json.loads(metadata)
        if evidence != {'refresh_keys': 0, 'credential_accounts': 1, 'active_keys': 1, 'historical_usage': 0}:
            raise RuntimeError('minimal fixture audit failed')
        self.event(synthetic_fixture_ready=True, account_id=28, key=key_info, credential_audit=evidence)

    def boot(self):
        self.phase = 'container-origin-proof'
        result = self.docker_run(['run', '--rm', '--name', self.names['probe']] + self.labels +
                                 ['--log-driver', 'none', '--memory', '96m', '--network', self.network,
                                  '-e', 'LIVE_ATTESTATION_URL=http://host.docker.internal:19495',
                                  '-e', 'LIVE_ATTESTATION_TOKEN_FILE=/run/secrets/attestation-token',
                                  '-v', str(self.root / 'probe') + ':/app/probe:ro',
                                  '-v', str(self.root / 'attestation-token') + ':/run/secrets/attestation-token:ro',
                                  self.images['alpine:3.22'], '/app/probe', '-generate'])
        evidence = json.loads(result)
        if not evidence.get('supported') or not evidence.get('generated'):
            raise RuntimeError('container-origin proof failed')
        self.event(container_origin_proof={k: evidence[k] for k in ('os', 'arch', 'supported', 'generated')})
        self.phase = 'gateway-boot'
        self.docker_run(['create', '--name', self.names['gateway']] + self.labels +
                        ['--log-driver', 'none', '--memory', '768m', '--cpus', '2', '--network', self.network,
                         '-p', '127.0.0.1:19480:19480', '-e', 'SKIP_SETUP=true', '-e', 'CONFIG_FILE=/run/secrets/config.yaml',
                         '-e', 'GOMEMLIMIT=512MiB', '-e', 'TOKEN_REFRESH_ENABLED=false',
                         '-e', 'LIVE_ATTESTATION_URL=http://host.docker.internal:19495',
                         '-e', 'LIVE_ATTESTATION_TOKEN_FILE=/run/secrets/attestation-token',
                         '-v', str(self.root / 'server') + ':/app/server:ro', '-v', str(self.root / 'config.yaml') + ':/run/secrets/config.yaml:ro',
                         '-v', str(self.root / 'attestation-token') + ':/run/secrets/attestation-token:ro',
                         '--tmpfs', '/app/data:rw,size=64m', self.images['alpine:3.22'], '/app/server'])
        self.start_gateway()
        self.seed_fixture()
        self.docker_run(['stop', '--time', '2', self.names['gateway']])
        self.docker_run(['exec', self.names['redis'], 'redis-cli', 'FLUSHDB'])
        self.start_gateway()
        self.event(candidate_live_gateway_ready=True, port=19480, refresh_disabled_marker=True,
                   containers=self.names, run_id=self.run_id)

    def start_gateway(self):
        self.refresh_disabled.clear()
        self.refresh_started = False
        attached = self.runner.start(self.docker_prefix + ['start', '-a', self.names['gateway']],
                                     stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

        def startup_markers():
            # Every other byte is discarded in memory; Docker logging is disabled.
            while line := attached.stdout.readline(65536):
                if b'token_refresh.service_started' in line:
                    self.refresh_started = True
                    self.runner.stop()
                if b'token_refresh.service_disabled' in line:
                    self.refresh_disabled.set()
        threading.Thread(target=startup_markers, daemon=True).start()
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if self.runner.stopping.is_set():
                raise Stopped()
            try:
                with urllib.request.urlopen('http://127.0.0.1:19480/health', timeout=.5) as response:
                    data = json.loads(response.read(4096))
                    healthy = response.status == 200 and data.get('status') == 'ok'
                if healthy and self.refresh_disabled.is_set() and not self.refresh_started:
                    return
            except Exception:
                pass
            if self.runner.stopping.wait(.25):
                raise Stopped()
        raise RuntimeError('candidate gateway did not pass effective boot checks')


def main():
    root, run_id = pathlib.Path(sys.argv[1]), sys.argv[2]
    host = Host(root, run_id)
    for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(sig, lambda *_: host.runner.stopping.set())
    start = threading.Event()
    watch = None
    try:
        validate_root(root, run_id)
        if not select.select([sys.stdin], [], [], 10)[0]:
            raise RuntimeError('startup input timeout')
        raw = sys.stdin.readline(8192)
        if not raw:
            raise RuntimeError('startup input EOF')
        config = json.loads(raw)

        def watchdog():
            heartbeat = began = time.monotonic()
            while not host.runner.stopping.is_set() and time.monotonic() - heartbeat < 20 and time.monotonic() - began < 1800:
                if select.select([sys.stdin], [], [], .1)[0]:
                    line = sys.stdin.readline()
                    if not line or line.strip() == 'stop':
                        break
                    heartbeat = time.monotonic()
                    if line.strip() == 'start':
                        start.set()
            host.cleanup()
        watch = threading.Thread(target=watchdog, daemon=True)
        watch.start()
        host.provision(config)
        while not start.wait(.1):
            if host.runner.stopping.is_set():
                raise Stopped()
        host.boot()
        while not host.runner.stopping.wait(.1):
            pass
    except Exception as exc:
        host.event(candidate_live_error=type(exc).__name__, phase=host.phase)
    finally:
        host.cleanup()
        if watch:
            watch.join(timeout=3)


if __name__ == '__main__':
    main()
