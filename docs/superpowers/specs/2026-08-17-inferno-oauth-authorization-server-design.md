# Inferno as OAuth 2.0 Authorization Server — Design

**Date:** 2026-08-17
**Status:** Approved design, pre-implementation
**Sub-project:** #4 of 4 in the OpenComputer portal swap

---

## Context

OpenComputer is replacing Nous Portal with its own stack. The target architecture
mirrors Nous exactly, because the client — Hermes Desktop and the raw `hermes-agent`
gateway — is upstream code we do not want to fork:

```
INFERNO  = OC Portal.  OAuth 2.0 authorization server + billing + quota + /v1 inference
    │
    ├── provisions ──▶  oc-platform   (compute driver: Hetzner, tunnels, snapshots)
    ├── registers  ──▶  agent:{id}    OAuth client_id, public, baked on the VM
    └── pushes     ──▶  {tunnel}/api/cron/fire   signed JWT, verified via Inferno JWKS

AGENT (raw hermes on the VM) = OAuth client to Inferno, OAuth server to the desktop
DESKTOP = native OAuth client of the *agent*, never of Inferno directly
```

This document specifies **only** the authorization-server half: the OAuth endpoints,
signing keys, client registry, and scopes that everything else depends on. It is the
first sub-project because nothing else can be built without it.

### Why this shape

The decisive constraint comes from `hermes_cli/dashboard_auth/native_flow.py`:

> The desktop cannot be a direct OAuth client of the upstream IDP: the Portal
> `client_id` is per-gateway-instance (`agent:{instance_id}`) and the Portal validates
> that the `redirect_uri` ends in `/auth/callback` on the gateway's own public origin.
> So the gateway brokers the flow: it is the authorization server *to the desktop*, and
> an OAuth client *to the Portal*.

We inherit that topology. Inferno's job is to be a correct authorization server for
gateway clients; the gateway's brokering behaviour already exists upstream and needs no
changes.

The second decisive fact, from `hermes_cli/dashboard_register.py` and
`hermes_cli/config.py:2244`: **the agent's on-disk credential is a public `client_id`,
not a secret.** There is no `client_secret` anywhere in the hermes codebase. User
credentials arrive via OAuth and are refreshed in-process by
`hermes_cli/nous_auth_keepalive.py` (6-hour interval, runs inside the agent). This is
why the current OpenComputer practice of baking a shared inference API key into VM
cloud-init is being retired rather than repaired: per-user attribution falls out of the
OAuth model for free, because the credential that pays *is* the user's token.

## Goals

1. Inferno issues and validates OAuth 2.0 tokens for third-party clients (gateways).
2. `hermes` and Hermes Desktop authenticate against Inferno with **zero code changes**
   beyond configuration — they already speak this contract against Nous Portal.
3. Tokens are verifiable **offline** by the agent and by oc-platform, via JWKS.
4. Reuse Inferno's existing session/refresh machinery rather than duplicating it.

## Non-goals (explicitly deferred to later sub-projects)

- The `agents` registry table and `GET /api/agents` — **sub-project #2**.
- VM provisioning, cloud-init changes, oc-platform's service-secret middleware — **#1**.
- The `/api/billing/*` contract adapter — later, tracked separately.
- Cron-fire signing (Chronos) — depends on this spec's JWKS but ships with #2.
- De-hardcoding `inference-api.nousresearch.com` in the agent — **#3**.
- Migrating oc-platform's Postgres off Supabase. Supabase **Auth** dies as a
  consequence of this work; Supabase **Postgres** stays for now and is replaced later
  as its own change. Bundling the two would make a failure impossible to attribute.

## What already exists in Inferno (reuse, do not rebuild)

Verified in `backend/internal/service/auth_service.go`:

| Capability | Where | Notes |
|---|---|---|
| Refresh-token **families** | `GenerateTokenPair(familyID)` | rotation already grouped |
| **Reuse detection** | `ErrRefreshTokenReused` (line 38) | the hard part is done |
| Session IDs for revocation | `JWTClaims.SessionID` (`sid`) | per-session revoke exists |
| Token invalidation on password change | `JWTClaims.TokenVersion` | |
| Session binding (IP+UA) | `JWTClaims.BindingHash` (`bnd`) | optional, already wired |
| Self-serve API keys | `POST/GET/PUT/DELETE /keys` | key issuance needs no work |
| Per-key usage + quota | `usage_log`, `user_platform_quota` | metering needs no work |

The OAuth AS is therefore mostly **new endpoints over existing machinery**, not a new
subsystem.

## The one significant blocker

`auth_service.go:1300-1309` restricts token validation to `HS256/384/512` and rejects
any non-HMAC signing method. Inferno's session tokens are signed with a **symmetric**
secret.

**A symmetric key cannot be published as a JWKS.** Both the agent (verifying cron-fire
JWTs) and oc-platform (verifying caller identity) must validate signatures without
holding the signing secret. Sharing an HMAC secret with every VM would mean any single
compromised VM can mint tokens for every user.

**Decision: introduce a second, asymmetric signing key exclusively for OAuth-issued
tokens.** Inferno's existing HMAC session tokens are untouched — they stay internal to
the Vue frontend. The two token types are distinguished by issuer and audience and
never validated by the same code path.

- Algorithm: **ES256** (P-256). Smaller tokens than RS256, and Go's stdlib support is
  first-class. `golang-jwt/jwt/v5` is already a dependency.
- Keys are stored in the existing `security_secret` table, versioned, with `kid`.
- Rotation: publish the new key in JWKS, wait one max-access-token-TTL, then start
  signing with it. Never remove a `kid` from JWKS while unexpired tokens reference it.

## Data model

Two new tables. Both are generic OAuth concepts, not OpenComputer-specific.

### `oauth_client`

| Column | Type | Notes |
|---|---|---|
| `id` | int64 PK | |
| `client_id` | text unique | server-applies the `agent:` prefix — clients never construct it |
| `kind` | enum | `SELF_HOSTED` \| `HOSTED` |
| `name` | text | docker-style `adjective_noun`; **no uniqueness constraint** (matches upstream: the row id is the key, collisions are harmless) |
| `owner_user_id` | FK users | |
| `org_id` | FK org | billing subject; see Open Questions |
| `redirect_uri_origin` | text | the gateway's public origin |
| `created_at`, `revoked_at` | timestamp | |

No `client_secret` column. These are public clients; PKCE is the protection.

### `oauth_device_authorization`

| Column | Type | Notes |
|---|---|---|
| `device_code` | text unique | opaque, high-entropy |
| `user_code` | text unique | short, human-typable, unambiguous alphabet |
| `client_id` | FK oauth_client | |
| `scope` | text | requested scopes |
| `status` | enum | `pending` \| `approved` \| `denied` \| `expired` |
| `approved_user_id` | FK users nullable | set on approval |
| `expires_at`, `last_polled_at` | timestamp | `last_polled_at` backs `slow_down` |

Rows are deleted on terminal status by the existing cleanup-task pattern
(`usage_cleanup_task`).

## Endpoints

All paths are fixed by the client contract in `hermes_cli/auth.py` and
`apps/desktop/electron/main.ts`. **They are not ours to name.**

### `POST /api/oauth/device/code`

Request: `client_id`, optional `scope` (form-encoded).

Response **must** contain every one of these — `auth.py:4959` hard-fails on any missing
field:

```json
{ "device_code": "...", "user_code": "...", "verification_uri": "...",
  "verification_uri_complete": "...", "expires_in": 900, "interval": 5 }
```

### `POST /api/oauth/token`

Grants: `urn:ietf:params:oauth:grant-type:device_code`, `refresh_token`,
`authorization_code` (PKCE, for the gateway broker).

Device-flow errors follow RFC 8628 exactly — `auth.py:5000` branches on these strings:
`authorization_pending`, `slow_down`, `access_denied`, `expired_token`.

Success returns `access_token` (required — client raises without it), `refresh_token`,
`expires_in`, `scope`.

### `GET /api/oauth/account`

Account/entitlement summary for the authenticated bearer. Consumed by
`hermes_cli/nous_account.py`.

### `POST /api/oauth/self-hosted-client`

Bearer-authenticated. Creates a `SELF_HOSTED` client owned by the caller's org, returns
the fully-formed `agent:{id}`. This is what `oc dashboard register` calls and what the
"Local Dashboards" portal page drives.

### `GET /.well-known/jwks.json`

Public. Current + previous signing keys by `kid`.

### `GET /oauth/authorize` (browser)

The gateway's upstream leg. **Auto-approves any current member of the client's org when
a live portal session exists** — this is what makes desktop cloud sign-in silent
(`main.ts:6224`). It removes the human click, never a security check: the gateway still
completes its own PKCE exchange.

## Scopes

| Scope | Grants |
|---|---|
| `inference` | `/v1/*` |
| `billing:read` | read billing state |
| `billing:manage` | charge, auto-top-up, subscription changes |
| `agents:read` | `GET /api/agents` |
| `agents:manage` | register/revoke clients |

`billing:manage` is deliberately **not** granted at initial login. The desktop re-runs
the device flow to elevate — `apps/desktop/src/app/settings/billing/use-step-up.tsx`
already implements this and its tests assert the behaviour.

## Security requirements

1. **PKCE required on `authorization_code`.** Verify `SHA256(verifier) == challenge`.
   Codes are single-use; a second redemption invalidates the session.
2. **Refresh-token reuse invalidates the family**, not just the presented token. Inferno
   already implements this — the OAuth path must route through it, not around it.
3. **`redirect_uri` must match the client's registered origin and end in
   `/auth/callback`.** Loopback redirects are rejected. This is precisely why the
   gateway must broker for the desktop; relaxing it collapses the security model.
4. **Never unsigned-decode.** Copy the upstream posture: an empty JWKS URL refuses all
   tokens rather than degrading to an unverified decode
   (`config.py:3040`, `nas_jwks_url`).
5. **`user_code` alphabet excludes ambiguous characters** (`0/O`, `1/I/l`). It is read
   aloud and typed.
6. **Rate-limit the token endpoint per `device_code`**, and honour `slow_down` — the
   client polls on a fixed interval and a hung endpoint freezes its UI.
7. **Never log tokens.** Follow the existing redaction denylist
   (`hermes_cli/auth.py:5383` is the client-side equivalent); log `kid` and
   `client_id` only.

## Testing

- **State machine:** table tests for every device-flow transition, including
  `slow_down` on over-polling and correct expiry.
- **PKCE binding:** a code minted for challenge A is not redeemable with verifier B.
- **Reuse detection:** replaying a rotated refresh token kills the whole family.
- **JWKS rotation:** tokens signed with the previous `kid` still verify during overlap.
- **Scope enforcement:** a token without `billing:manage` is rejected by billing writes.
- **Conformance (highest value):** the desktop's existing test fixtures in
  `apps/desktop/src/app/settings/billing/*.test.ts` and
  `apps/desktop/src/app/settings/toolset-config-panel.test.tsx` encode the Nous contract
  precisely. Replaying them against Inferno is a free, exact conformance suite. Any
  divergence they catch is a real incompatibility.
- **Live:** `oc setup` against a local Inferno completes a device login end to end.

## Open questions

1. **Org vs user as the billing subject.** Nous Portal is org-scoped throughout —
   routes are `/orgs/{slug}/api-keys`, `/api/agents` returns an `org`, and multi-org
   users get a `409 org_selection_required` the desktop renders as a picker
   (`main.ts:6165`). Inferno has `group`, which is an entitlement bucket, not a tenant.
   Adopting orgs now is more work; skipping them means the `409` path is dead code and
   retrofitting tenancy later is expensive. **Recommendation: model a minimal org now**
   — one personal org auto-created per user — so the contract is satisfied and the
   desktop's org handling is exercised rather than bypassed.
2. **`dashboardToken` disposition.** oc-platform stores it plaintext with
   `TODO(security, before public signups)` for KMS encryption, and rotation is
   unimplemented. This design removes its primary job. Confirm it can be deleted rather
   than inherited.
3. **Provisioning atomicity.** Inferno mints a `client_id`, then oc-platform may fail to
   provision. Needs a reconcile job or orphaned clients accumulate. Belongs to
   sub-project #1 but is caused here.

## Implementation order within this sub-project

1. ES256 keypair + `security_secret` storage + `GET /.well-known/jwks.json`
2. `oauth_client` table + `POST /api/oauth/self-hosted-client`
3. Device flow: `POST /api/oauth/device/code` + `POST /api/oauth/token` + approval page
4. `authorization_code` + PKCE + `GET /oauth/authorize` with org auto-approve
5. Scope enforcement middleware + `GET /api/oauth/account`
6. Conformance run against the desktop's fixtures

## Divergence ledger

This work adds new tables and handlers under `backend/`. Per `GOAL.md`, every file that
differs from upstream `Wei-Shaw/sub2api` must be declared in the ledger **and** in
`DECLARED` inside `scripts/check-divergence.sh` before commit. This is a deliberate,
permanent divergence — Inferno becoming an authorization server is the product, not
drift. Add a single ledger entry `D4 — OAuth authorization server` covering the new
`ent/schema/oauth_*.go`, their codegen output, the new handlers, and routes.

**Do not hand-merge `ent/` codegen after a rebase** — re-run `go generate ./ent`.
