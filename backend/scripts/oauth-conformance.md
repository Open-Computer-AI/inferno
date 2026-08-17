# OAuth authorization-server conformance runbook

Drives the REAL hermes Python CLI (`hermes_cli/auth.py`, repo
`/Users/saksham/OpenComputerV2/OpenComputerV2`, **read-only** — never edit it,
never commit to it) against Inferno's OAuth 2.0 authorization server, end to
end: device authorization request → human approval → token exchange →
signature verification → `slow_down` backoff.

**⚠️ `HERMES_HOME` warning — read before running anything below.** The real
CLI persists credentials to `$HERMES_HOME/auth.json`, defaulting to
`~/.hermes/auth.json` — the developer's REAL Hermes config. Every command
below sets `HERMES_HOME` to a throwaway scratch directory for the entire run.
**Never** run the CLI commands in this runbook without `HERMES_HOME` set to
something disposable, or you will clobber your own working Hermes install.

## Two defects this runbook depends on being fixed

Both are fixed in this repo (migration `905` + the scope literal below) —
this section exists so a future conformance run against a fresh checkout
knows what to check first if step 1 or step 2 below fails.

1. **`hermes-cli` must exist as an `oauth_clients` row.** The real CLI
   hardcodes `client_id="hermes-cli"` (`hermes_cli/auth.py:77`,
   `DEFAULT_NOUS_CLIENT_ID`) — a first-party public client baked into the
   binary, not a per-agent `agent:{id}` minted through
   `POST /api/oauth/self-hosted-client`. Without a matching row,
   `POST /api/oauth/device/code` 400s `invalid_client`. Seeded by
   `backend/migrations/905_hermes_cli_first_party_client.sql` — see that
   file's header comment for the full reasoning on `kind='FIRST_PARTY'`,
   `owner_user_id=0`/`org_id=0` as sentinels, and the OOB redirect URI
   placeholder.
2. **Scope spelling: `inference:invoke`, not `inference`.** The real CLI
   sends `scope=inference:invoke` (`hermes_cli/auth.py:78-80`,
   `NOUS_INFERENCE_INVOKE_SCOPE`/`DEFAULT_NOUS_SCOPE`) and, after login,
   client-side-asserts the granted `scope` response field contains that
   exact string (`_nous_invoke_jwt_status`, `auth.py:2132`) before treating
   the token as usable — an exact-equality check, mirroring our own
   `scopeSatisfies` (`internal/server/middleware/oauth_scope.go`). Our
   `RequestCode`/`mintAccessToken` path is pass-through (it echoes back
   whatever `scope` the client sent — see `oauth_device_service.go` and
   `oauth_token_service.go`), so this does not block the wire protocol
   mechanically, but the design doc and several tests previously named the
   scope `inference`; corrected to `inference:invoke` throughout
   (`docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md`'s
   scope table, and the scope literals in `oauth_handler_test.go`,
   `oauth_device_service_test.go`, `oauth_token_service_test.go`,
   `oauth_scope_test.go`, `refresh_token_cache_test.go`) so future
   scope-enforcement work is built against the vocabulary the real client
   actually uses.

## 1. Bring up the server against throwaway infra

```bash
docker network create t8net
docker run -d --name t8-pg --network t8net -e POSTGRES_USER=t8 -e POSTGRES_PASSWORD=t8pass \
    -e POSTGRES_DB=t8db -p 127.0.0.1:15932:5432 postgres:18-alpine
docker run -d --name t8-redis --network t8net -p 127.0.0.1:16979:6379 redis:8-alpine

cd backend && go build -o /tmp/sub2api-t8 ./cmd/server

# A stale backend/config.yaml from an earlier local run marks setup as
# already "completed" and silently skips AUTO_SETUP — point DATA_DIR at an
# empty scratch dir so this run gets a fresh admin bootstrap.
mkdir -p /tmp/t8-data

AUTO_SETUP=true \
DATA_DIR=/tmp/t8-data \
DATABASE_HOST=127.0.0.1 DATABASE_PORT=15932 DATABASE_USER=t8 \
DATABASE_PASSWORD=t8pass DATABASE_DBNAME=t8db DATABASE_SSLMODE=disable \
REDIS_HOST=127.0.0.1 REDIS_PORT=16979 \
ADMIN_EMAIL=admin@t8.local ADMIN_PASSWORD='t8AdminPass!2345' \
JWT_SECRET=t8-jwt-secret-not-for-prod-0123456789abcdef \
SERVER_HOST=127.0.0.1 SERVER_PORT=18480 \
SERVER_FRONTEND_URL=http://127.0.0.1:18480 \
SERVER_MODE=debug TZ=UTC \
/tmp/sub2api-t8
```

`SERVER_FRONTEND_URL` MUST be set and non-empty — the device flow builds
`verification_uri` from it (`{frontend_url}/device`) and
`OAuthDeviceService.RequestCode` refuses with `ErrPortalNotConfigured` (500
`server_error`) otherwise.

**Expected:** `Admin user created: admin@t8.local` and `Auto setup completed
successfully!` in the log; `curl -s http://127.0.0.1:18480/setup/status`
returns `{"code":0,"data":{"needs_setup":false,"step":"completed"}}`.

Confirm the seeded client and JWKS:

```bash
curl -s http://127.0.0.1:18480/.well-known/jwks.json
docker exec t8-pg psql -U t8 -d t8db -c \
  "SELECT client_id, kind, status FROM oauth_clients;"
```

**Expected:** one JWKS key, `alg: ES256`; one row,
`client_id=hermes-cli, kind=FIRST_PARTY, status=active`.

## 2. Seed a personal org for the auto-setup admin

The auto-setup admin bypasses the normal signup path (`EnsurePersonalOrg`
only runs on email/OAuth signup), so it has no org by default. Approving a
device authorization and later resolving `GET /api/oauth/account` both need
one — seed it directly (mirrors Tasks 3/5/6/7's own verification):

```bash
docker exec t8-pg psql -U t8 -d t8db -c \
  "INSERT INTO orgs (slug, name, is_personal, personal_user_id) VALUES ('admin-t8', 'admin@t8.local', true, 1);"
docker exec t8-pg psql -U t8 -d t8db -c \
  "INSERT INTO org_members (org_id, user_id, role) SELECT id, 1, 'OWNER' FROM orgs WHERE personal_user_id=1;"
```

## 3. Run the real hermes CLI's device-code login against the server

`uv run oc setup` is the full **interactive** setup wizard
(`hermes_cli/setup.py:run_setup_wizard`) — with no real TTY attached
(`is_interactive_stdin()` false) it hits the non-interactive guard and exits
immediately printing config-file guidance, before touching the network.
`oc setup --portal` hits the exact same guard first. Forcing a real TTY
(`script`/pty) gets past the guard, but the one-shot portal flow then moves
into a curated Nous-model picker plus free-tier/account-info calls — Nous
Portal endpoints this OAuth-AS plan explicitly defers (`task-8-brief.md`'s
"Deferred to later sub-projects") and that would require automating an
interactive arrow-key menu unrelated to the OAuth mechanics under test.

Instead, this runbook imports and calls the exact same functions
`_login_nous` (`hermes_cli/auth.py`) would call for the OAuth portion —
`_nous_device_code_login` for the RFC 8628 exchange, then the identical
persistence calls (`_save_provider_state` + `_save_auth_store`) — directly,
skipping only the unrelated model-picker step. This is 100% real, unedited
client code making real HTTP requests to the real server; nothing about the
device-code/token-endpoint conformance under test is bypassed.

```bash
mkdir -p /tmp/t8-hermes-home   # throwaway HERMES_HOME — NEVER your real ~/.hermes

cat > /tmp/t8-drive-login.py <<'PYEOF'
import sys
sys.path.insert(0, "/Users/saksham/OpenComputerV2/OpenComputerV2")
from hermes_cli.auth import (
    _nous_device_code_login, _load_auth_store, _save_auth_store,
    _save_provider_state, _auth_store_lock, PROVIDER_REGISTRY,
)
pconfig = PROVIDER_REGISTRY["nous"]
print(f"[driver] client_id={pconfig.client_id!r} scope={pconfig.scope!r}")
auth_state = _nous_device_code_login(open_browser=False, timeout_seconds=20.0)
with _auth_store_lock():
    store = _load_auth_store()
    _save_provider_state(store, "nous", auth_state)
    saved_to = _save_auth_store(store)
print(f"[driver] wrote auth store to {saved_to}")
print("[driver] DONE")
PYEOF

cd /Users/saksham/OpenComputerV2/OpenComputerV2 && \
HERMES_HOME=/tmp/t8-hermes-home \
HERMES_PORTAL_BASE_URL=http://127.0.0.1:18480 \
uv run python /tmp/t8-drive-login.py
```

**Expected:** prints `user_code` and `verification_uri_complete` and begins
polling every 1s (client caps the interval at
`DEVICE_AUTH_POLL_INTERVAL_CAP_SECONDS=1`, ignoring the server's advertised
`interval:5` — see step 5 for why the server's own `slow_down` still holds).
**It must NOT raise `Device code response missing fields`** — that error
means the `/api/oauth/device/code` response is missing one of the six
RFC 8628 fields, or is wrapped in the `{code,message,data}` envelope. It
blocks waiting for approval — proceed to step 4 while it polls (or open
`verification_uri_complete` in a real browser; Task 7 already verified that
screen end to end with screenshots, so this run uses the approve API
directly — see `oauth-conformance` task 8 report for why).

## 4. Approve the device authorization

Log in as the seeded admin and approve by `user_code` (read the `user_code`
the driver script printed):

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:18480/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@t8.local","password":"t8AdminPass!2345"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['access_token'])")

curl -s -X POST http://127.0.0.1:18480/api/oauth/device/approve \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"user_code":"<USER_CODE_FROM_STEP_3>"}'
```

**Expected:** `{"code":0,"message":"success","data":{"status":"approved"}}`.
The driver process's poll loop then returns within ~1s and prints
`[driver] wrote auth store to /tmp/t8-hermes-home/auth.json` and
`[driver] DONE`.

## 5. Verify the stored token

```bash
python3 -c "
import json, base64, pathlib
s = json.load(open('/tmp/t8-hermes-home/auth.json'))
tok = s['providers']['nous']['access_token']
h = json.loads(base64.urlsafe_b64decode(tok.split('.')[0] + '=='))
print(h)
assert h['alg'] == 'ES256', h
assert h.get('kid'), 'missing kid'
print('OK')
"
```

**Expected:** `{'alg': 'ES256', 'kid': '<22-char base64url>', 'typ': 'JWT'}`
then `OK`.

## 6. Verify the token against the JWKS endpoint

```bash
python3 -c "
import json, base64, urllib.request, jwt
from jwt import PyJWK
s = json.load(open('/tmp/t8-hermes-home/auth.json'))
tok = s['providers']['nous']['access_token']
kid = json.loads(base64.urlsafe_b64decode(tok.split('.')[0] + '=='))['kid']
jwks = json.loads(urllib.request.urlopen('http://127.0.0.1:18480/.well-known/jwks.json').read())
match = [k for k in jwks['keys'] if k['kid'] == kid]
assert match, f'no JWKS key with kid={kid}'
claims = jwt.decode(tok, key=PyJWK.from_dict(match[0]).key, algorithms=['ES256'], options={'verify_aud': False})
print('signature valid')
print(claims)
"
```

Requires `pip install pyjwt cryptography` if not already available.

**Expected:** `signature valid` then claims including
`aud: hermes-cli`, `scope: inference:invoke`, `sub: "1"` (the approving
admin's user id), a future `exp`.

## 7. Verify the poll loop honours `slow_down`

Start a second device authorization and poll the token endpoint twice
within one second, direct against the wire (not through the CLI, so the
1-second gap is guaranteed rather than a race with the client's own 1s poll
cap):

```bash
RESP=$(curl -s -X POST http://127.0.0.1:18480/api/oauth/device/code \
  -d "client_id=hermes-cli" -d "scope=inference:invoke")
DEVICE_CODE=$(python3 -c "import json,sys; print(json.loads('''$RESP''')['device_code'])")

curl -s -X POST http://127.0.0.1:18480/api/oauth/token \
  -d "grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  -d "client_id=hermes-cli" -d "device_code=$DEVICE_CODE"
echo
curl -s -X POST http://127.0.0.1:18480/api/oauth/token \
  -d "grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  -d "client_id=hermes-cli" -d "device_code=$DEVICE_CODE"
```

**Expected:** first call `{"error":"authorization_pending"}`, immediate
second call `{"error":"slow_down"}`.

## Cleanup

```bash
kill %1 2>/dev/null   # or: pkill -f /tmp/sub2api-t8
docker rm -f t8-pg t8-redis
docker network rm t8net
rm -rf /tmp/t8-data /tmp/t8-hermes-home /tmp/t8-drive-login.py /tmp/sub2api-t8
```

Verify nothing named `t8*` remains (`docker ps -a`) and ports
15932/16979/18480 are free before considering the run finished.
