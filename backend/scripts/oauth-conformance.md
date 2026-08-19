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

## KNOWN DIVERGENCE found by this run — not yet fixed

`GET /v1/models` answers **200 for an OAuth token** and **403
INSUFFICIENT_BALANCE for an ordinary API key**, for the same user with the
same zero balance.

Cause: `api_key_auth.go:264` gates on balance at AUTH time
(`apiKeyBalanceBelowAuthThreshold`, `balance <= 0`). The OAuth branch does not
replicate that gate; it relies on `CheckBillingEligibility`, which runs
downstream in the billing-bearing handlers and which `/v1/models` never calls.

Not a security hole — listing models is not billable, and every billable
endpoint is correctly refused. But the two credential paths are meant to reach
identical outcomes on identical routes, and here they do not. Deliberately
left for the whole-branch review rather than changed unreviewed: altering an
auth-time gate is exactly the kind of edit that wants a second pair of eyes.

## Teardown

```bash
docker rm -f t8-pg t8-redis && docker network rm t8net
rm -rf /tmp/t8-data /tmp/t8-hermes-home /tmp/t8-hermes-home2 /tmp/sub2api-t8
```
