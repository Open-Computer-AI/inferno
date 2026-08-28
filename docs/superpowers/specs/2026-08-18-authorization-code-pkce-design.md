# Authorization Code + PKCE — the Hermes Desktop login leg

**Date:** 2026-08-18
**Status:** Draft, pre-approval
**Sub-project:** step 5 — deferred out of the OAuth AS branch, now the blocker

---

## Why this exists

The OAuth AS branch shipped the **device-code** flow, and the real CLI logs in with
it end to end. **Hermes Desktop cannot log in at all**, because it uses a completely
different path.

From `hermes_cli/dashboard_auth/native_flow.py`:

> The desktop cannot be a direct OAuth client of the upstream IDP: the Portal
> `client_id` is per-gateway-instance and the Portal validates that the
> `redirect_uri` ends in `/auth/callback` on the gateway's own public origin — a
> desktop loopback redirect is rejected. So the **gateway brokers** the flow: it
> is the authorization server *to the desktop*, and an OAuth client *to the Portal*.

Three tiers, two nested PKCE exchanges:

```
Desktop ──PKCE──▶ Gateway ──PKCE──▶ Inferno (/oauth/authorize)
                                    ↑ THIS IS THE ONLY MISSING PIECE
```

The desktop half and the gateway half are upstream code that already works. We owe
the Portal half. Since `workspace/` is being retired and the desktop is the product
surface, this blocks the product.

## The contract — read from the client, not invented

`plugins/dashboard_auth/nous/__init__.py` is the gateway's OAuth client. It is the
specification; we do not edit it.

### `GET /oauth/authorize` (browser navigation, query params)

| param | value |
|---|---|
| `response_type` | `code` |
| `client_id` | `agent:{instance_id}` |
| `redirect_uri` | the gateway's own `…/auth/callback` |
| `scope` | **`agent_dashboard:access`** |
| `state` | opaque, 32 random bytes b64url |
| `code_challenge` | S256 of an ~86-char verifier |
| `code_challenge_method` | `S256` |

### `POST /api/oauth/token`

`grant_type=authorization_code`, plus `code`, `redirect_uri`, `client_id`, `code_verifier`.

### Access-token requirements (`__init__.py:432-449`)

- **`algorithms=["RS256"]` — hard-coded.** Verified against `/.well-known/jwks.json`, JWKS cached 5 min.
- `audience` = the **bare** `client_id` (no prefix).
- `issuer` = the Portal base URL.
- `options={"require": ["exp", "iat", "aud", "iss", "sub"]}` — all five mandatory.
- `oauth_contract_version`: absent → warn and proceed; present and `!= 1` → refuse.
- `agent_instance_id`: cross-checked against the `client_id` suffix as defense-in-depth.

### Refresh

A **24h rotating** refresh token with reuse detection and a documented **60-second
grace** on replay. On a dead/expired/reuse-detected token Portal returns **400**,
which the middleware turns into a redirect to `/auth/login`.

## Decisions

### D-1. Move ALL OAuth token signing to RS256 — do not run two algorithms

The gateway plugin hard-codes `algorithms=["RS256"]`. Our AS currently mints ES256.
Three options were considered:

1. **RS256 alongside ES256**, JWKS publishing both, dispatching on `kid`. Correct, but
   permanently doubles the key surface and every "which algorithm is this?" question.
2. **Patch the plugin.** Rejected outright — the client is the specification.
3. **RS256 everywhere.** ✅

**Decision: option 3.** The device-flow consumer is *our own* middleware, which we can
change freely; the dashboard consumer is upstream code we cannot. So the constrained
side wins. The CLI never verifies the access token's signature — it presents it as a
bearer — so nothing on that path breaks.

Cost: RS256 signatures are ~256 bytes against ES256's ~64, so tokens grow by roughly
300 bytes b64. Irrelevant for a bearer header. There are no production tokens to
migrate; the branch is unmerged.

**This also forces the fix for final-review finding I3.** Verification currently uses
`keySvc.Active()` and ignores `kid`, so key rotation is impossible. Switching algorithm
means minting a new key anyway — so verification must dispatch on `kid` over a set of
published keys, which makes the design's documented rotation procedure executable for
the first time.

### D-2. `agent_dashboard:access` joins the scope vocabulary

Not currently in `knownScopes`, so `ValidateScope` would reject the desktop login
outright — the same class of defect as `tool:invoke` and `inference:invoke` before it.
It is a *dashboard-access* scope rather than an inference one; it grants nothing on the
inference path.

### D-3. `redirect_uri` validation becomes load-bearing

`oauth_client.redirect_uri_origin` is currently accepted from the caller and stored
**unvalidated** (recorded in the OAuth AS spec's "What was NOT built"). The security
requirement from the original design now has to be real:

- the `redirect_uri` presented at `/oauth/authorize` **must** match the client's
  registered origin,
- **must** end in `/auth/callback`,
- loopback (`127.0.0.1`, `localhost`) **must** be rejected — this is precisely why the
  desktop cannot talk to us directly and must broker through its gateway.

Validate at **registration** as well as at authorize time: storing an unvalidated
origin and only checking later means a bad row sits in the table looking legitimate.

### D-4. Implement the 60-second reuse grace

Our refresh rotation revokes the family the instant a rotated token is replayed. The
Portal contract documents a 60s grace, and the reason is legitimate races: a desktop
that fires a retry, or two windows refreshing together, would otherwise nuke a healthy
session. Within the grace, a replay of the *immediately previous* token returns the
current pair rather than revoking.

⚠️ This deliberately weakens reuse detection, so it must be bounded precisely: grace
applies only to the single most-recently-rotated token, only within 60s, and any replay
outside that window revokes the family as it does today. Outside the grace nothing
changes.

### D-5. Authorization codes are single-use, short-lived, and PKCE-bound

10-minute TTL, redeemable exactly once, bound to `client_id` + `redirect_uri` +
`code_challenge`. A second redemption **revokes** any token already issued from that
code — RFC 6749 §4.1.2's required posture, and the same discipline the device grant
already applies via its CAS.

### D-6. Org auto-approve, and what the human sees

The original design says `/oauth/authorize` "auto-approves any current member of the
client's org when a live portal session exists" — that is what makes the desktop's
silent cloud sign-in silent (`main.ts:6224`).

**Decision: auto-approve only when the requested scope is exactly `agent_dashboard:access`
and the user already owns the client.** Anything else renders a consent screen. Silent
approval is acceptable for a scope that grants dashboard access to an agent the user
already owns; it is not acceptable as a general mechanism, and the device flow's own
consent screen exists because of exactly that reasoning.

## Non-goals

- Refresh-token *family* semantics beyond the 60s grace — already built.
- `/api/agents`, cron signing, VM provisioning — sub-projects #1 and #2.
- The billing contract.
- Retiring ES256 from anything outside OAuth (Inferno's HMAC panel sessions are untouched).

## Security requirements

1. **PKCE mandatory.** No `code_challenge` → reject. `plain` method → reject; S256 only.
2. **Verify `SHA256(verifier) == challenge`** at redemption; mismatch is a hard failure.
3. **Codes single-use**, and a replay revokes tokens minted from the original.
4. **`redirect_uri` at redemption must byte-match the one at authorize.** A mismatch is
   the classic code-interception vector.
5. **Never log a code, verifier, refresh token, or access token.**
6. **`state` is the client's CSRF defence** — echo it back unmodified, never interpret it.
7. **An empty JWKS must refuse**, never degrade to unverified decode.

## Testing

- PKCE binding: a code minted for challenge A is not redeemable with verifier B.
- Single use: second redemption fails **and** kills the first token pair.
- `redirect_uri` mismatch between authorize and token is rejected.
- Loopback and non-`/auth/callback` redirect URIs are rejected at both registration and authorize.
- `alg: none` and ES256-signed tokens are rejected by anything expecting RS256.
- Grace window: replay at t+30s returns the current pair; at t+90s revokes the family.
- **Conformance:** drive the real `plugins/dashboard_auth/nous` provider against a local
  Inferno — `start_login` → approve → `complete_login` → `verify_session` → `refresh_session`.
  That plugin verifies our token with PyJWT against our JWKS, so a green run proves the
  RS256 switch, the claim set, the audience and the issuer all at once. This is the
  single highest-value test in the plan; the device-grant equivalent caught two defects
  the unit tests could not.

## Resolved by inspection (2026-08-18) — no open questions

**Nothing depends on our ES256 tokens.** Verified rather than assumed:
`oauth_signing_key.go` and `oauth_token_service.go` do not exist on `inferno-redesign`
at all — the OAuth AS lives only in this worktree. The running container is
`weishaw/sub2api:latest`, upstream's *published image*, which contains none of this
code. Both Dockerfiles build `frontend/` + `backend/` from the parent branch, which
has no OAuth AS. So zero tokens have ever been issued outside throwaway test
containers, and **D-1 is a clean switch with no migration**.

**`agent_dashboard:access` is write-only. Nothing consumes it.** The string occurs in
exactly four places in the entire hermes repo: a docstring, the `_SCOPE` constant that
puts it on the authorize request, a test fixture default, and a test asserting it is
sent. No route enforces it, and the plugin never reads a `scope` claim back out of the
verified token — it does not survive into `Session` or `AuthContext`.

The consequence is worth stating precisely, because it is easy to draw the wrong
conclusion: its **security value is zero**, but its **compatibility requirement is
absolute**. We must add it to the vocabulary not because it protects anything, but
because `ValidateScope` would otherwise reject the desktop login outright. And we must
echo it back into the token faithfully rather than dropping it — the vocabulary rule
against silently narrowing a scope request applies here as everywhere else, even though
in this instance nothing would notice.

It gates nothing on our side, now or later, unless we deliberately build something that
reads it.
