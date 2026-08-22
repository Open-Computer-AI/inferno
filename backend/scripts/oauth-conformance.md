# OAuth authorization-server conformance runbook

Drives the REAL hermes Python CLI (`hermes_cli/auth.py`, repo
`/Users/saksham/OpenComputerV2/OpenComputerV2`, **read-only** — never edit it,
never commit to it) against Inferno's OAuth 2.0 authorization server, end to
end: device authorization request → human approval → token exchange →
signature verification → `slow_down` backoff → **refresh grant**.

Step 9 adds the **dashboard leg**: the authorization_code + PKCE grant,
driven through the real `plugins/dashboard_auth/nous` provider in the same
read-only client repo. Different client, different grant, different token
audience — and, unlike steps 1-8, it must be run against a **`-tags embed`
build**, because that is the only build in which the SPA middleware can
swallow a root-level route. Do not skip step 9 either.

**Run both grants, not just device_code.** The first pass of this runbook
tested device_code alone and shipped with the refresh grant silently
broken against the real client (see defect 3 below) — a token pair that
works for exactly 15 minutes and then can never be renewed looks
identical to full success until the first expiry. Steps 1-7 below cover
device_code; step 8 covers the refresh grant. Do not skip step 8.

**⚠️ `HERMES_HOME` warning — read before running anything below.** The real
CLI persists credentials to `$HERMES_HOME/auth.json`, defaulting to
`~/.hermes/auth.json` — the developer's REAL Hermes config. Every command
below sets `HERMES_HOME` to a throwaway scratch directory for the entire run.
**Never** run the CLI commands in this runbook without `HERMES_HOME` set to
something disposable, or you will clobber your own working Hermes install.

## Three defects this runbook depends on being fixed

All three are fixed in this repo — this section exists so a future
conformance run against a fresh checkout knows what to check first if a
step below fails.

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
3. **The refresh grant reads the credential from the wrong place.** The
   real CLI's `_refresh_access_token` (`hermes_cli/auth.py:5507-5521`)
   sends the refresh token in an **`x-nous-refresh-token` HEADER** — the
   request body carries only `grant_type` and `client_id`. This was missed
   by the first pass of this conformance run, which only exercised
   device_code: the refresh grant was RFC 6749 §5.2-correct (reads
   `refresh_token` from the form body) and fully covered by wire-contract
   tests, and still silently rejected every refresh from the real client,
   because that client never populates the body field at all. Fixed in
   `internal/handler/oauth_handler.go`'s `refreshTokenFromRequest`: reads
   the `x-nous-refresh-token` header first (trimmed; empty-after-trim
   counts as absent), falling back to the RFC 6749 body field — so this
   endpoint still works for any other RFC-conformant client that only
   knows the body field. This is Critical, not cosmetic: access tokens are
   short-lived (15 minutes) and the refresh grant is the entire mechanism
   behind "log in once, the agent runs forever" — with it broken, every
   agent would die 15 minutes after login and never recover, invisibly,
   because the device-code leg alone looks like complete success.

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
`server_error`) otherwise. It is ALSO the `iss` claim on every minted access
token, and `OAuthTokenService.mintAccessToken` now refuses with
`ErrIssuerNotConfigured` (a logged 500 `server_error`) rather than minting a
token carrying `iss: ""` — see step 9's defect table.

Write it **without a trailing slash**. The value is normalised now, but the
canonical spelling is what every expectation below is written against.

**Expected:** `Admin user created: admin@t8.local` and `Auto setup completed
successfully!` in the log; `curl -s http://127.0.0.1:18480/setup/status`
returns `{"code":0,"data":{"needs_setup":false,"step":"completed"}}`.

Confirm the seeded client and JWKS:

```bash
curl -s http://127.0.0.1:18480/.well-known/jwks.json
docker exec t8-pg psql -U t8 -d t8db -c \
  "SELECT client_id, kind, status FROM oauth_clients;"
```

**Expected:** one JWKS key, `kty: RSA`/`alg: RS256`/`use: sig`; one row,
`client_id=hermes-cli, kind=FIRST_PARTY, status=active`.

(`alg: ES256` until the authorization_code+PKCE plan's Task 1 migrated the
signing key to RS256 — `backend/migrations/907_oauth_rs256_key.sql`. The real
gateway plugin hard-codes `algorithms=["RS256"]`, so ES256 here is a
regression, not a variant.)

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
assert h['alg'] == 'RS256', h
assert h.get('kid'), 'missing kid'
print('OK')
"
```

**Expected:** `{'alg': 'RS256', 'kid': '<22-char base64url>', 'typ': 'JWT'}`
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
claims = jwt.decode(tok, key=PyJWK.from_dict(match[0]).key, algorithms=['RS256'], options={'verify_aud': False})
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

## 8. Verify the refresh grant — do not skip this

The device_code steps above prove login works. This step proves the
credential the CLI just obtained can actually be renewed — the property
the whole autonomous-agent design depends on. Drives the real, unedited
`hermes_cli.auth._refresh_access_token` directly (the exact function
`refresh_nous_oauth_pure` calls internally, which sends the refresh token
via the `x-nous-refresh-token` header — see defect 3 above):

```bash
mkdir -p /tmp/t8-hermes-home-2   # a second throwaway HERMES_HOME

cat > /tmp/t8-drive-refresh.py <<'PYEOF'
import sys
sys.path.insert(0, "/Users/saksham/OpenComputerV2/OpenComputerV2")
import httpx
from hermes_cli.auth import _nous_device_code_login, _refresh_access_token, PROVIDER_REGISTRY

pconfig = PROVIDER_REGISTRY["nous"]
auth_state = _nous_device_code_login(open_browser=False, timeout_seconds=20.0)
print("[driver] device_code login complete")

with httpx.Client(timeout=httpx.Timeout(20.0), headers={"Accept": "application/json"}) as client:
    refreshed = _refresh_access_token(
        client=client,
        portal_base_url=auth_state["portal_base_url"],
        client_id=auth_state["client_id"],
        refresh_token=auth_state["refresh_token"],
    )

assert "access_token" in refreshed
assert refreshed["access_token"] != auth_state["access_token"]
print("[driver] REFRESH_OK")
PYEOF

cd /Users/saksham/OpenComputerV2/OpenComputerV2 && \
HERMES_HOME=/tmp/t8-hermes-home-2 \
HERMES_PORTAL_BASE_URL=http://127.0.0.1:18480 \
uv run python /tmp/t8-drive-refresh.py
```

It blocks on `_nous_device_code_login` waiting for approval — read the
`user_code` it prints and approve exactly as in step 4, then it proceeds
straight into the refresh call.

**Expected:** `[driver] device_code login complete` then
`[driver] REFRESH_OK`. Before the header fix, this call raised
`AuthError: Refresh token exchange failed` (the server saw an empty
`refresh_token` form field and returned `invalid_grant`) — if you see that
here, the header isn't being read; check `refreshTokenFromRequest` in
`internal/handler/oauth_handler.go`.

## 9. The dashboard leg — authorization_code + PKCE, against `-tags embed`

Steps 1-8 exercise the hermes **CLI** (device_code + refresh). This step
exercises the hermes **gateway dashboard**: a different real client
(`plugins/dashboard_auth/nous/__init__.py`, same read-only repo, never
edited), a different grant (authorization_code + PKCE S256), a different
scope (`agent_dashboard:access`), and a token verified with
`algorithms=["RS256"]`, `audience=<bare client_id>`, `issuer=<portal base
URL>` and `options={"require": ["exp","iat","aud","iss","sub"]}`. One green
run therefore proves the RS256 switch, `kid` resolution, the audience shape,
the issuer and the required claim set simultaneously.

### 9.0 Why this leg MUST run against a `-tags embed` build

`deploy/Dockerfile` builds with `-tags embed`. That build installs
`FrontendServer.Middleware()` (`internal/web/embed_on.go`) **before** route
registration, so every root-level, non-`/api` path that is not in
`shouldBypassEmbeddedFrontend`'s allowlist (`internal/web/bypass.go`) is
silently served `index.html` instead of reaching its handler. Two routes this
leg depends on live at the root: `/oauth/authorize` and
`/.well-known/jwks.json`. The JWKS omission was a real bug on this branch and
was invisible to every non-embed test, because without the tag the middleware
is not even installed.

```bash
cd inferno-frontend && npm run build   # populates backend/internal/web/dist/
cd ../backend && go build -tags embed -o /tmp/sub2api-t8-embed ./cmd/server
```

Do the frontend build even though `backend/internal/web/dist/index.html`
already exists — the checked-in file is a 91-byte placeholder that exists only
so `//go:embed all:dist` has a match in CI. It will not exercise the SPA
fallback realistically.

Bring the server up exactly as in step 1 but with `/tmp/sub2api-t8-embed`.
Then, before driving anything, prove both routes reach their handlers:

```bash
curl -sS -D- -o /dev/null http://127.0.0.1:18480/.well-known/jwks.json | head -5
curl -sS -D- -o /dev/null "http://127.0.0.1:18480/oauth/authorize?client_id=x"
```

**Expected:** the first is `200` with `Content-Type: application/json`
(**not** `text/html` — `text/html` means the allowlist regressed and every
RS256 verifier in the fleet is being handed an HTML page); the second reaches
the handler (a `400` error page for the bogus `client_id`, not the SPA).

### 9.1 Register a client the login user actually owns

`/oauth/authorize` refuses a client the session user neither owns nor shares
an org with, and that refusal is a MUST-NOT-REDIRECT **403 error page** — easy
to misdiagnose as a protocol failure. Register through the API as the same
admin you will log in as, after seeding the personal org in step 2:

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:18480/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@t8.local","password":"t8AdminPass!2345"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['access_token'])")

curl -s -X POST http://127.0.0.1:18480/api/oauth/self-hosted-client \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"t8 dashboard","redirect_origin":"https://agent-t8.example.com"}'
```

**Expected:** `{"client_id":"agent:<32 hex>","name":"t8 dashboard"}`.

The redirect origin must be **https and non-loopback** (`ValidateRedirectOrigin`,
`internal/service/oauth_redirect_uri.go`) and the redirect_uri must end in
`/auth/callback`. `https://agent-t8.example.com/auth/callback` never has to
resolve — nothing in this run fetches it; the driver reads the code straight
out of the target URL, exactly as a browser would before following it.

### 9.2 Drive the real provider

**⚠️ Set `HERMES_HOME` to a throwaway directory** — the same warning as the
top of this file. The dashboard provider itself does not write an auth store,
but `hermes_cli` imports on the same path do read config, and one careless run
without it is how you clobber your own `~/.hermes`.

The driver below stands in for exactly one thing: the **browser**. It performs
the top-level navigation to `/oauth/authorize` a human would perform, and
carries the panel-session bearer that `AuthorizeConsentView.vue`'s axios
interceptor attaches after login. Everything else — the token POST, the JWKS
fetch, the RS256 verification, the refresh POST with its
`x-nous-refresh-token` header — is the provider's own unmodified code.

```bash
mkdir -p /tmp/t8-hermes-home-dash /tmp/t8-driver
# The full script is ~200 lines; see the Task 6 report
# (.superpowers/sdd/2026-08-18-authorization-code-pkce/task-6-report.md §1)
# for its shape. The load-bearing sequence is:
#
#   provider = NousDashboardAuthProvider(client_id=..., portal_url=...)
#   start    = provider.start_login(redirect_uri=".../auth/callback")
#   # browser leg, done with httpx:
#   #   1. GET start.redirect_url with NO Authorization header
#   #      -> MUST be 302 to /login?redirect=<original path+query>
#   #   2. GET start.redirect_url WITH the panel bearer
#   #      -> MUST be 200 text/plain whose body is the target URL
#   #         (202 means the consent screen was required -- see 9.3)
#   code     = parse_qs(urlparse(body).query)["code"][0]
#   session  = provider.complete_login(code=..., state=..., code_verifier=...,
#                                      redirect_uri=...)
#   provider.verify_session(access_token=session.access_token)
#   rotated  = provider.refresh_session(refresh_token=session.refresh_token)
#   provider.refresh_session(refresh_token=session.refresh_token)  # grace replay

cd /Users/saksham/OpenComputerV2/OpenComputerV2 && \
HERMES_HOME=/tmp/t8-hermes-home-dash \
T8_PORTAL=http://127.0.0.1:18480 \
T8_CLIENT_ID=agent:<from 9.1> \
T8_PANEL_TOKEN="$TOKEN" \
T8_REDIRECT_URI=https://agent-t8.example.com/auth/callback \
uv run python /tmp/t8-driver/t8_drive_dashboard.py
```

### 9.3 What to assert, and what each failure means

| Assertion | If it fails |
|---|---|
| anonymous GET `/oauth/authorize` → `302 /login?redirect=…` | under embed, a `200 text/html` means `shouldBypassEmbeddedFrontend` lost the path |
| authenticated GET → **200**, not 202 | 202 = consent required. Auto-approve needs scope EXACTLY `agent_dashboard:access` **and** `client.OwnerUserID == session user`. Confirm you took the auto-approve path; do not assume it |
| 403 error page | the login user does not own the client — 9.1 registered it as someone else |
| target URL echoes `state` verbatim, carries `code` | — |
| `complete_login` returns a **refresh token** | an empty one means the dashboard session silently dies at the first 15-minute expiry |
| `ProviderError: … Invalid issuer` | `iss` is not the canonical portal base URL. **This is Task 6 defect 1**: a trailing slash on `SERVER_FRONTEND_URL` used to land in `iss` verbatim, and the client rstrips `/` before comparing. `NewOAuthTokenService` normalises it now; if you see this again, that normalisation is gone |
| `500 server_error` on the token endpoint with `ErrIssuerNotConfigured` logged | `SERVER_FRONTEND_URL` is unset. **Task 6 defect 2**: this used to mint tokens with `iss: ""` instead of refusing, so the failure surfaced on the client as "Invalid issuer" with nothing in our logs |
| replayed authorization code → `invalid_grant` **and** the issued refresh family is revoked | RFC 6749 §10.5. Rejecting the replay without revoking leaves a thief's stolen pair alive |
| grace replay of a rotated RT returns the **current** pair | Task 5's 60s grace. A `RefreshExpiredError` here means the grace is gone |

Decode and assert the claim set explicitly rather than eyeballing it:

```bash
python3 -c "
import base64, json, sys
seg = sys.argv[1].split('.')[1]
print(json.dumps(json.loads(base64.urlsafe_b64decode(seg + '=' * (-len(seg) % 4))), indent=2, sort_keys=True))
" "<access_token>"
```

**Expected:** `aud` is the **bare** `client_id` (a string — not a list, not a
URL), `iss` is the portal base URL with **no trailing slash**, `exp`/`iat`/`sub`
present, `oauth_contract_version` is `1`, and `agent_instance_id` equals the
`client_id` suffix after `agent:`.

### 9.4 Known, accepted behaviour — do not file these as bugs

- **A refresh issued within the same second as the login returns a
  byte-identical access token.** There is no `jti` claim, `iat`/`exp` have
  one-second resolution, and RS256/PKCS#1 v1.5 is deterministic, so an
  identical claim set produces an identical JWT. The *refresh token* does
  rotate, which is the part that matters; a refresh a second or more later
  produces a distinct token with an advanced `exp`. Assert the latter.
- **`Session.org_id` is `""`.** The provider reads an `org_id` claim we do not
  emit. Nothing in the provider requires it.

## Cleanup

```bash
kill %1 2>/dev/null   # or: pkill -f /tmp/sub2api-t8
docker rm -f t8-pg t8-redis
docker network rm t8net
rm -rf /tmp/t8-data /tmp/t8-hermes-home /tmp/t8-hermes-home-2 \
       /tmp/t8-hermes-home-dash /tmp/t8-driver \
       /tmp/t8-drive-login.py /tmp/t8-drive-refresh.py \
       /tmp/sub2api-t8 /tmp/sub2api-t8-embed /tmp/sub2api-t8-noembed
```

Verify nothing named `t8*` remains (`docker ps -a`) and ports
15932/16979/18480 are free before considering the run finished.

---

# 10. OAuth-bearer inference on `/v1` (sub-project #0)

Added 2026-08-19. This is the leg that proves the access token Inferno issues
is accepted by Inferno's own inference endpoint. Before sub-project #0 every
row in the table below was `401 {"code":"INVALID_API_KEY"}` — the server
issued a credential its own money endpoint refused.

Run steps 1-3 above first (throwaway infra, personal org, real device login).
Add `OAUTH_BACKING_KEY_GROUP_NAME=<an active group name>` to the server env;
without it every OAuth request is refused with a deliberate, operator-readable
`403 oauth_backing_group_unavailable` rather than a nil-group panic downstream.

## The credential under test

Driven by the unmodified `hermes_cli.auth._nous_device_code_login`:

```
client_id 'hermes-cli'   scope 'inference:invoke'
header    {"alg":"RS256","kid":"P5vo96WcGCkuGC0UHyDc-Q","typ":"JWT"}
claims    {"aud":"hermes-cli","exp":…,"iat":…,"iss":"http://127.0.0.1:18480",
           "oauth_contract_version":1,"scope":"inference:invoke","sub":"1"}
agent_key == access_token: True
len("Bearer " + token) = 628
```

That last line is the point of finding F-A: `maxAPIKeyAuthorizationHeaderBytes`
is 256, so the API-key path aborts a real token before `GetByKey` is ever
called. The OAuth shape test must run ahead of that cap. A regression guard
that uses a short fake token proves nothing.

## Result — every endpoint past auth

| Request | Status | Body |
|---|---|---|
| `GET /v1/models` | **200** | real model list |
| `GET /models` | **200** | real model list |
| `POST /v1/messages` | 403 | `insufficient balance` (billing_error) |
| `POST /v1/chat/completions` | 403 | `insufficient balance` |
| `POST /v1/responses` | 403 | `insufficient balance` |
| `POST /responses` | 403 | `insufficient balance` |
| `POST /chat/completions` | 403 | `insufficient balance` |
| `POST /v1/alpha/search` | 404 | `only available for OpenAI groups` |
| `POST /alpha/search` | 404 | `only available for OpenAI groups` |
| `POST /embeddings` | 404 | `not supported for this platform` |

`403 insufficient balance` and `404 wrong platform` are PASSES: both are
reached only *after* authentication, and are what a real API key gets for the
same unfunded user on the same group. `401 INVALID_API_KEY` is the failure.

## The backing row

```
id | user_id | name                   | group_id | status | oauth_client_id | key_len
 1 |       1 | OAuth agent hermes-cli |        1 | active | hermes-cli      |      67
```

`user_id` is the token's `sub`; `oauth_client_id` is its `aud`; `group_id`
comes from the configured policy.

**Row reuse:** two independent device logins, in two separate `HERMES_HOME`s,
produced two different access tokens and still exactly ONE backing row. Rows
do not accumulate per login.

## Disclosure sweep — 0 leaks

With the backing row's real 67-character secret read straight out of Postgres:

| Request | Status | Secret present? |
|---|---|---|
| `GET /api/v1/keys` | 200 | clean — `items: []`, the row is hidden |
| `GET /api/v1/keys/1` | 404 | clean — `api key not found` |
| `PUT /api/v1/keys/1` | 403 | clean — `backs an OAuth agent and is managed by the server` |
| `DELETE /api/v1/keys/1` | 403 | clean — `cannot be deleted` |
| `PUT /api/v1/admin/api-keys/1` | 423 | clean — blocked earlier by the admin compliance ack |

The admin route returns 423 before reaching the backing-row guard added in
`f4917699`, so this run does not exercise that guard; it is mutation-proven at
the unit level instead (both guards reduced to `if false`, four tests fail).

## Ordinary API keys still work

A freshly created ordinary key reaches `403 INSUFFICIENT_BALANCE` on `/v1` —
past auth, same as before this sub-project.

## ADJUDICATED DIVERGENCE — the zero-balance non-billable class

`GET /v1/models` answers **200 for an OAuth token** and **403
`INSUFFICIENT_BALANCE` for an ordinary API key**, for the same user with the
same zero balance.

Cause: `api_key_auth.go` gates on balance at AUTH time
(`apiKeyBalanceBelowAuthThreshold`, `balance <= 0`). The OAuth branch does not
replicate that gate; it relies on `CheckBillingEligibility`, which runs
downstream inside the billing-bearing handlers — and `/v1/models` never calls it.

**Ruling (two independent whole-branch reviews, same recommendation, accepted):
document it. Add no auth-time balance gate to either path.**

- **Not to the OAuth branch.** An auth-time `balance <= 0` gate blocks
  non-billable endpoints too — model discovery, batch-status polling, video
  status, sideband reads. An agent that has run its balance to zero would lose
  the ability to *discover* it has run to zero, and the hermes client's
  model-listing call is on its startup path. That turns a recoverable "top up
  your account" into an agent that cannot start.
- **Not removed from the API-key path either.** It is load-bearing, pre-existing
  and outside this branch's blast radius, whose binding constraint is
  *"API-key auth keeps working everywhere it works today."*

**Read the direction correctly.** The OAuth branch is not *missing* a check. The
API-key path gates a **non-billable** endpoint at auth time — a pre-existing
over-restriction — and the OAuth branch declined to copy it. Every path that
does billable work is refused at zero balance on both credential types.

### It is a CLASS of routes, not one endpoint

The divergence appears wherever a route is (a) **not** in the API-key path's
`skipBilling` set and (b) reaches no handler that calls
`CheckBillingEligibility`. `skipBilling` (`api_key_auth.go`) is exactly
`/v1/usage`, `/v1/sub2api/billing` and `isAsyncImageTaskRead`
(`GET /v1/images/tasks/:task_id`) — those three are already equivalent on both
paths.

**This list was re-derived for the fix wave rather than copied from either
review, and both reviews over-counted it.** Membership was traced handler by
handler; the four groups that are NOT members are recorded below, because
getting this wrong in the safe direction is still getting it wrong.

| Route(s) | Handler | Evidence |
|---|---|---|
| `GET /v1/models`, `GET /models` | `modelsHandler` → `GatewayHandler.Models` (`gateway_handler.go:1073`) / `OpenAIGatewayHandler.CodexModels` | `gateway_handler.go`'s three `CheckBillingEligibility` calls are at `:242`, `:968` (both inside `Messages`) and `:2040` (`CountTokens`); `openai_codex_models_handler.go` has zero |
| `GET /v1/images/batches` | `BatchImage.List` | zero `CheckBillingEligibility` in `batch_image_handler.go` |
| `GET /v1/images/batches/models` | `BatchImage.Models` | as above |
| `GET /v1/images/batches/:id` | `BatchImage.Get` | as above |
| `GET /v1/images/batches/:id/items` | `BatchImage.Items` | as above |
| `GET /v1/images/batches/:id/items/:custom_id/content` | `BatchImage.ItemContent` | as above |
| `GET /v1/images/batches/:id/download` | `BatchImage.Download` | as above |
| `POST /v1/images/batches/:id/cancel` | `BatchImage.Cancel` | as above |
| `DELETE /v1/images/batches/:id` | `BatchImage.DeleteRecord` | as above |
| `DELETE /v1/images/batches/:id/outputs` | `BatchImage.DeleteOutputs` | as above |
| `GET /v1/live/:call_id` | `OpenAIGateway.LiveSideband` (`openai_live.go:188`) | the file's only `CheckBillingEligibility` is at `:71`, inside `Live` (the POST) |

Twelve routes. On each, a zero-balance user is refused `403
INSUFFICIENT_BALANCE` at auth with an API key and passes with an OAuth token.
None is billable and none reaches an upstream.

### Four groups both reviews listed that are NOT members

Both reviews warned that *"the handler has no `CheckBillingEligibility`" is not
a sufficient membership test* — and then both made the mirror-image error, by
reading the route's immediate handler rather than following it through:

| Route(s) | Actually reaches | Billing call |
|---|---|---|
| `GET /v1/videos/**` (status + content, 8 routes) | `videoStatusHandler` / `videoContentHandler` → `GrokVideoStatus` / `GrokVideoContent` → `handleGrokMedia` (`grok_media.go:54`) | **`grok_media.go:153`** |
| `GET /v1/custom-voices`, `/:voice_id`, `/:voice_id/audio` | `voiceHandler` / `customVoicePathHandler` → `GrokVoice` (`grok_audio.go:132`) | **`grok_audio.go:142`** |
| `GET /v1/realtime` | `GrokRealtime` (`grok_audio.go:24`) | **`grok_audio.go:38`** |
| `GET /v1/responses` (websocket upgrade) | `ResponsesWebSocket` (`openai_gateway_handler.go:1605`) | **`openai_gateway_handler.go:1824`** |

These are still divergent at **auth time** — an API key is refused before the
handler runs, an OAuth token reaches it — but the **outcome** is not divergent,
because the handler refuses a zero-balance caller itself. (On a non-Grok group
the first three gate on platform and answer `404 not supported for this
platform` before billing; a 404 is not billable work either.) The distinction is
worth keeping: for this group the documented divergence is about the error
*shape*, not about access. `TestZeroBalanceAuthDivergenceOnTheNearMissRoutes`
pins it.

The root-level aliases of the video, voice and realtime routes are not in either
group: they are mounted on the bare `apiKeyAuth`, so no OAuth token reaches them
at all.

### The one that would have made this negligent, and why it does not

`POST /v1/images/batches` (`BatchImage.Submit`) also has no
`CheckBillingEligibility`, and it *does* billable work. It is nevertheless **not
divergent**, and this was verified independently for this fix wave rather than
taken from either review:

- `BatchImagePublicService.Submit` (`internal/service/batch_image_public.go`)
  calls `reserveBatchImageBalanceHold` before anything is queued.
- `reserveBatchImageBalanceHold` (`internal/service/batch_image_billing_hold.go`)
  calls `UsageBillingRepository.ReserveBatchImageBalance`.
- `reserveUsageBillingBatchImageBalance` (`internal/repository/usage_billing_repo.go`)
  is literally
  `UPDATE users SET balance = balance - $1, frozen_balance = ... WHERE id = $2 AND deleted_at IS NULL AND balance >= $1 RETURNING ...`,
  and turns `sql.ErrNoRows` into `service.ErrBatchImageInsufficientBalance`.

So balance is enforced on that route by a different mechanism, at the database,
in a single atomic statement. A zero-balance caller is refused regardless of
which credential they present; only the error *shape* differs. (The one
short-circuit, `if cmd.HoldAmount <= 0 { return nil }`, means a job whose
estimated cost is zero skips the hold — a zero-cost job is by definition not
billable work.)

**Net: no billable work is reachable at zero balance via OAuth.** That is the
fact that makes documenting this acceptable rather than negligent. Note also
that "the handler has no `CheckBillingEligibility`" is **not** a sufficient test
for "unbilled" — both reviews raised this route as a suspected bypass and both
cleared it. A future reader who repeats the grep without repeating the trace
will repeat the false positive.

### It is pinned by a test, and the test exists to make a reversal loud

`internal/server/routes/gateway_billing_divergence_test.go`.

The reason for the test is not that the current behaviour is fragile — it is
that **adding the auth-time gate to the OAuth branch would break nothing
today.** A well-meaning future edit ("the two paths should agree") could
silently reverse a decision that took two whole-branch reviews to make, and
nothing would fail. The test makes that reversal loud, and makes a new /v1 route
joining the class impossible to add silently.

## Spec done-criterion 3 — this run DISPROVED it, and the spec was corrected

The "Row reuse" observation above (two logins → exactly ONE backing row) is the
precise negation of the design doc's original done-criterion 3, *"a second agent
instance for the same user resolves to a different backing row"*, which the same
document sold as a headline benefit ("one row per agent instance").

The implementation is right and the spec was wrong: identity is
`(user_id, oauth_client_id)` and `oauth_client_id` is the token's `aud` — the
**application's** client id. The device grant presents `hermes-cli` for every
install. The spec has been corrected
(`docs/superpowers/specs/2026-08-19-oauth-bearer-inference-design.md`, "What the
identity actually is"): per-**user** attribution is delivered and is what this
work exists for; per-**agent-instance** attribution is not, and the quota and
rate-limit ledger is shared across one user's instances.

## Spec done-criterion 2 (`usage_logs`) — NOT demonstrated, and precisely why

Criterion 2 is *"a `usage_logs` row exists for an OAuth-served request,
attributed to the right `user_id`, with its `api_key_id` naming the backing
row."* It is **not** demonstrated by this runbook, and the honest reason is:

**A `usage_logs` row is only written when a billable request COMPLETES**, and a
completed billable request needs a real upstream provider account with real
credit behind the configured group. This runbook stands up throwaway Postgres
and Redis; it does not and cannot stand up a funded upstream. Every billable
endpoint in the table above answered `403 insufficient balance` *before* any
metering write, which is exactly the expected outcome for an unfunded user — so
the metering path was never reached, on either credential type.

What IS pinned, and where, so this is not mistaken for "untested":

- The `usage_logs` **read/hydration** side is covered by
  `TestUsageLogHydrationBlanksOnlyTheBackingSecret`
  (`internal/repository/api_key_repo_oauth_backing_integration_test.go`), which
  inserts the row by hand and proves the backing row's secret is blanked while
  the attribution survives.
- The **write** side is unexercised with an OAuth-resolved backing row. Reading
  it, it should work — no metering code keys on `apiKey.Key` (which is blank on a
  backing row), and the row carries a real `ID`, `UserID` and `Group`, which is
  everything the write path reads. That is a code-reading argument, not
  evidence, and it is recorded here as such.

**To close it** requires one leg this runbook cannot supply: point the policy
group at a real upstream account with credit, fund the test user, issue a single
OAuth-served inference call, and
`SELECT user_id, api_key_id FROM usage_logs ORDER BY id DESC LIMIT 1`. Ten
minutes, on infrastructure that already exists — but it is a live-upstream
dependency, so it belongs to whoever runs this against a real deployment, not to
a hermetic runbook.

## Attribution demonstrated (spec done-criterion 2) — closed 2026-08-19

The earlier pass could not close this: a `usage_logs` row is written only when a
billable request *completes*, and every billable endpoint 403'd on balance before
reaching the metering write. Closed here by funding the user and pointing a group
at a mock upstream, so the write path actually runs.

Setup beyond steps 1-3: create an `accounts` row (`platform=anthropic`,
**`type=apikey`** — `api_key` is not a valid value and fails late, at
`gateway.forward_failed`, with `unsupported account type`), point its
`credentials.base_url` at a local mock returning a well-formed Anthropic response
with a real `usage` block, link it via `account_groups` to the policy group, and
fund `users.balance`. **Restart the server after editing `accounts`** — account
rows are cached in the scheduler snapshot and a live UPDATE is not picked up.

```
POST /v1/messages   (Authorization: Bearer <OAuth access token>)   status=200
```

```
usage_logs
 id | user_id | api_key_id | account_id | group_id | model                      | input_tokens | output_tokens | total_cost
  1 |       1 |          1 |          1 |        1 | claude-3-5-sonnet-20241022 |           11 |             7 | 0.0001380000

token claims   sub = 1   aud = hermes-cli   scope = inference:invoke
api_keys       id = 1    oauth_client_id = hermes-cli
users.balance  100.00000000 -> 99.99986200   (exactly total_cost)
```

Every link in the chain is closed by that row:

- `usage_logs.user_id` = the token's `sub`. **This is the per-user attribution the
  whole portal swap exists for** — inference is billed to the human whose OAuth
  token paid for it, with no shared API key baked into a VM anywhere.
- `usage_logs.api_key_id` = the backing row resolved from the token's `aud`.
- `input_tokens`/`output_tokens` are the values the upstream actually returned,
  so the metering read the real response rather than defaulting.
- The balance moved by exactly `total_cost`, so the money path is live, not just
  the log write.

What this does NOT show, and is a real limitation recorded in the spec: attribution
is per `(user, client application)`, not per agent instance. Every hermes install
presents the same first-party `client_id`, so one user's instances share a backing
row and its quota/rate-limit ledger. Per-instance attribution needs per-instance
`client_id`s via self-hosted client registration.

## Non-loopback hostname — the production configuration (added 2026-08-19)

Every earlier leg in this runbook used `127.0.0.1`, which is one of exactly three
hostnames hardcoded in the CLIENT's allowlist:

```
_NOUS_PORTAL_ALLOWED_HOSTS = {portal.nousresearch.com, localhost, 127.0.0.1}
```

So no earlier run ever exercised the configuration production actually uses: a
real hostname, permitted only by the operator env override. This leg does.

Use a hostname that resolves to 127.0.0.1 without touching `/etc/hosts` —
`portal.localtest.me`, `127.0.0.1.nip.io` and `oc.lvh.me` all do publicly. Bind
the server with `SERVER_HOST=0.0.0.0` and set
`SERVER_FRONTEND_URL=http://portal.localtest.me:18480`.

### Positive — WITH the override, the agent reaches Inferno

```
[driver] allowlist      = ['127.0.0.1', 'localhost', 'portal.nousresearch.com']
[driver] default portal = https://portal.nousresearch.com
[driver] env override   = http://portal.localtest.me:18480
```

Device grant completes against our server (a pending row appears in
`oauth_device_authorizations`; the access log records two `device/code` hits),
and the minted token carries:

```
iss   = http://portal.localtest.me:18480      <- ours, not Nous
aud   = hermes-cli   sub = 1   scope = inference:invoke
stored portal_base_url = http://portal.localtest.me:18480
agent_key == access_token: True
```

`/v1` through the same hostname: `GET /v1/models` **200**,
`POST /v1/chat/completions` 403 insufficient balance — both past auth, backing
row resolved as `id=1 user=1 client=hermes-cli`.

### Negative — WITHOUT the override, it silently goes to Nous

Same auth store, `HERMES_PORTAL_BASE_URL` and `NOUS_PORTAL_BASE_URL` unset:

```
[neg] env override           = None
[neg] stored portal_base_url = http://portal.localtest.me:18480
[neg] stored host            = portal.localtest.me
[neg] host in allowlist?     = False
[neg] => the client would use: https://portal.nousresearch.com
```

### What this proves, and the operational rule it forces

The allowlist is REAL and enforced — the negative case is not a no-op — and the
env override is precisely what bypasses it. Both halves matter: had the positive
case passed with the allowlist simply absent, any poisoned `portal_base_url`
persisted to `auth.json` would be trusted.

> **`HERMES_PORTAL_BASE_URL` (or `NOUS_PORTAL_BASE_URL`) is MANDATORY in every
> agent's environment.** If it is unset, misspelled, or dropped by a deployment
> change, the agent does not fail — it silently authenticates against
> `portal.nousresearch.com`, logging one warning and behaving normally otherwise.
> Every dashboard stays green while no agent is talking to Inferno at all.

Treat it as a required deploy-time assertion, not a default: fail agent startup
when it is unset, rather than letting the fallback happen.

## Teardown

```bash
docker rm -f t8-pg t8-redis && docker network rm t8net
rm -rf /tmp/t8-data /tmp/t8-hermes-home /tmp/t8-hermes-home2 /tmp/sub2api-t8
```

---

# Task 6 — billing contract adapter conformance (2026-08-22)

Run against a live Inferno (branch `feat/billing-contract-adapter`, built
`-tags embed`) on `127.0.0.1:18480`, with a REAL device-flow token for
`hermes-cli` carrying `scope=inference:invoke` — exactly what a stock hermes
login holds. Every assertion below was made by the UNMODIFIED client's own
parsers, imported from the fork, not by reading the JSON and judging it.

## Seed data — chosen to exercise the fixes, not the empty case

    groups:              id=2, monthly_limit_usd = 1000
    subscription_plans:  id=1 group=2 "Pro Monthly" $20  / 1 month
                         id=2 group=2 "Pro Annual"  $200 / 12 months
    user_subscriptions:  user=1 group=2 active, monthly_usage_usd = 300

Two plans on ONE group is the R-3.1 duplicate-tierId case, and $200/12mo is the
R-3.2 normalisation case. A first pass with an EMPTY plan table passed every
check while exercising neither — recorded because "the conformance run passed"
is worthless without saying what data it ran against.

## Parsers exercised (all from the read-only fork)

    agent/billing_view.billing_state_from_payload
    agent/subscription_view.subscription_state_from_payload
    hermes_cli/nous_account._subscription_from_payload

## Result — 21 positive + 11 negative, 0 failures

    /api/billing/state          logged_in=True, org+role, balance 999.73253335,
                                spent 0.26746665, card=None, auto_reload=False
    /api/billing/subscription   logged_in=True, context='personal',
                                canChangePlan a real JSON bool, tiers a JSON list,
                                2 plans -> 1 tier, tierId unique,
                                current.tier_id IS in tiers[] (the TUI ===-poll
                                invariant, subscriptionOverlay.tsx:786),
                                current.tierName == tiers[].name,
                                dollarsPerMonthDisplay = "16.67",
                                credits 1000 / remaining 700
    /api/oauth/account          subscription parses, snake_case correct,
                                monthly_charge ABSENT, rollover omitted,
                                tool_access omitted

    TUI would render:  Pro Annual · 16.67/mo · $1,000 credits/mo

    All 7 writes with a stock token   -> 403 {"error":"insufficient_scope"}
    GET /api/billing/auto-top-up      -> 404 (phantom, must not exist)
    GET /.../pending-change           -> 404 (phantom)
    GET /api/analytics/usage          -> 404 (phantom, Task 2 VOID)
    GET /api/billing/state, no token  -> 401

## TWO DEFECTS THIS RUN CAUGHT — both green through every unit test

Fixed in `6299007e`. Both had a VALID SHAPE and a wrong rendered VALUE, which is
precisely what a shape assertion cannot see. (See #1's CORRECTION: only the
second was actually user-visible.)

1. `dollarsPerMonthDisplay` was `16.666666666666668` — the output of a
   DIVISION, unlike every other money field, which is a stored value. New
   `billingDisplayMoney` (2dp) is used for it; `billingMoney` (exact) still
   serves balances.

   **CORRECTED (finding M-3, 2026-08-22.)** This entry originally claimed
   ui-tui subscriptionOverlay.tsx:437 interpolates that string VERBATIM, so
   "the user would have read Pro Annual · 16.666666666666668/mo". That is
   FALSE and the "user-visible" framing was wrong. The value never reaches
   ui-tui as our string: it arrives via `tui_gateway/server.py:9432`
   `format_money(t.dollars_per_month)`, after `agent/subscription_view.py:186`
   has parsed it to a `Decimal`, and `format_money`
   (`agent/billing_view.py:47-61`) quantizes to `Decimal('0.01')`. The Python
   CLI's own row builder, `_format_dollars_grouped`
   (`subscription_view.py:353-365`), quantizes too. Both surfaces would have
   rendered `$16.67`. The 2dp change is still correct — it matches both client
   surfaces and the panel, and a `...Display` field should not carry digits
   nobody sees — but this was a hygiene fix, NOT a user-visible defect, and
   listing it as one overstated what the conformance run caught. The error was
   tracing a TypeScript interpolation and missing that the value passes through
   a Python formatter first.
2. `current.tierName` was `Group.Name` = "Test " — the group's internal admin
   label, trailing space included — while the picker offered the SAME tierId as
   "Pro Annual". One tier under two names.

## A hollow check I wrote and caught

My first envelope check asserted no `code`/`message`/`data` keys and PASSED —
against three `{"error":"invalid_token"}` bodies, because the token had expired.
An assertion over an error body proves nothing. The run now asserts the status
code first and hard-fails on any `error` key before parsing. Recorded because it
is the same defect class this document has been tracking all project.
