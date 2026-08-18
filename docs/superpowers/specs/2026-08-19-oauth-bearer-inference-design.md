# OAuth-bearer inference on `/v1` — design

**Date:** 2026-08-19
**Status:** Design. Implements sub-project #0, which no earlier spec named.
**Depends on:** `feat/oauth-authorization-server` (`d82d9ad4`) — the RS256 AS, the
scope vocabulary, and `RequireOAuthScope`'s `kid`-dispatched verifier.
**Branch:** `feat/oauth-bearer-inference`

---

## Why this exists

The OAuth authorization server is complete, conformance-proven against the real
unmodified hermes client, and **issues a credential that Inferno's own `/v1`
refuses.**

That is the whole gap between "Inferno is an authorization server" and "hermes runs
on Inferno", and it is the original motivation for the entire portal swap: stop
baking a shared inference API key into VM cloud-init, and attribute inference to
the user whose OAuth token paid for it.

It was named in no plan and in no spec's non-goals. It was found by tracing the
credential end to end, and then **reproduced empirically** before this design was
written. Evidence:
`.superpowers/sdd/2026-08-18-authorization-code-pkce/v1-oauth-gap-evidence.md`.

### The reproduction

A real RS256 token from a real RFC 8628 device grant, driven by the unmodified
`hermes_cli.auth._nous_device_code_login`:

```
claims  {"aud":"hermes-cli","exp":...,"iat":...,"iss":"http://127.0.0.1:18480",
         "oauth_contract_version":1,"scope":"inference:invoke","sub":"1"}
client  agent_key == access_token: True
```

| Request (`Authorization: Bearer <that token>`) | Status | Body |
|---|---|---|
| `POST /v1/messages` | 401 | `{"code":"INVALID_API_KEY","message":"Invalid API key"}` |
| `POST /v1/chat/completions` | 401 | same |
| `GET /v1/models` | 401 | same |
| `POST /v1/responses`, `POST /responses` | 401 | same |
| `POST /v1/alpha/search`, `POST /alpha/search` | 401 | same |
| `GET /api/oauth/account` | **200** | the account payload |

The mirror image holds: a real API key gets `403 INSUFFICIENT_BALANCE` from `/v1`
(i.e. past auth) and `401 invalid_token` from `/api/oauth/account`. **Two fully
disjoint credential universes.**

---

## Three findings from the reproduction that shape the design

These are the reason this spec was written from evidence rather than from reading.
Each one invalidates an obvious implementation.

### F-A. The request never reaches the key lookup

`maxAPIKeyAuthorizationHeaderBytes = MaxAPIKeyCredentialBytes + 128 = 256`. The
token's header is 628 bytes, so `apiKeyHeadersTooLarge` (`api_key_auth.go:42`)
aborts **before** `GetByKey` — which is why the access log shows `latency_ms: 0`.

Any RS256 JWT exceeds this; 621 bytes is typical, not an outlier.

> **Consequence:** teaching `GetByKey` about OAuth tokens would still 401 every one
> of them. The OAuth branch MUST run ahead of the size cap, and the cap must not
> be raised for the API-key path (it is a cheap DoS guard and should stay tight).

### F-B. The retry loop ends in an IP-wide block, not a spin

The abort path calls `recordInvalidAuthFailure`. Driving the real adapter's
401→force-refresh loop reached `attempt 114 -> HTTP 429
{"code":"INVALID_AUTH_RATE_LIMITED"}`. Defaults: threshold 120, 60 s window, 60 s
block, keyed on **client IP**.

> **Consequence:** one OAuth-only agent can 429 *every legitimately-keyed client
> behind the same egress IP* — a NAT'd office, or several agents on one VM — with
> no failed-auth attribution to any user, key, or client. This is a live
> availability fault today, independent of the fix, and the fix must not leave a
> variant of it behind.

### F-C. A real `api_keys` row is structurally required

Not a preference — the schema enforces it:

- `usage_logs.user_id`, `.api_key_id`, `.account_id` are all `NOT NULL`, with
  `usage_logs_api_key_id_fkey → api_keys(id) ON DELETE CASCADE`.
- `usage_billing_dedup` has `api_key_id NOT NULL`, and it is half of
  `UNIQUE (request_id, api_key_id)`.
- The quota and rate-limit ledger **is** the key row — `api_keys.quota_used`,
  `usage_5h|1d|7d`, `window_*_start`, all `NOT NULL`, updated by
  `UPDATE api_keys WHERE id=$2`. There is no separate per-key quota table.
- `apiKey.Group`/`GroupID` is the **only** input to platform routing, channel pool
  selection, model mapping, and pricing.

> **Consequence:** there is no "keyless" inference path to build. An OAuth request
> must be backed by a real `api_keys` row, or the entire metering, billing, quota
> and routing stack has to be rewritten. The earlier spec's claim that "metering
> needs no work" survives *only* via a backing row.
>
> `ON DELETE CASCADE` additionally means such a row must **never be hard-deleted**:
> doing so silently destroys that agent's whole usage history.

---

## Design

### The shape

Add an OAuth credential path to the `/v1` chain that **resolves an OAuth access
token to a backing API-key row owned by the token's subject**, then populates
exactly the context the existing pipeline already expects. Downstream code does not
change at all.

```
Authorization: Bearer <credential>
        │
        ├── looks like a JWT (three base64url segments)? ──► OAuth branch
        │        verify RS256 via kid dispatch  (reuse RequireOAuthScope's verifier)
        │        require scope inference:invoke
        │        require iss == our issuer, aud == a registered client_id
        │        resolve sub → user
        │        resolve (user, client_id) → backing api_keys row  [get-or-create]
        │        populate ContextKeyAPIKey / ContextKeyUser / group context
        │        └─► unchanged gateway pipeline
        │
        └── otherwise ────────────────────────────────────► existing API-key path
                 size cap, GetByKey, all current behaviour, untouched
```

The JWT-shape test is a **routing** decision, not a security one: a value that
looks like a JWT and fails verification is rejected, never retried as an API key.
Falling through would let an attacker probe both universes with one request.

### Why get-or-create a backing row, and not the alternatives

| Option | Verdict |
|---|---|
| Keyless inference (`api_key_id` nullable) | Rejected. Three `NOT NULL`s, an FK, a unique constraint, and the entire quota ledger. This is a schema rewrite of the metering core to serve one caller. |
| Reuse the user's existing default API key | Rejected. Users may have none; it conflates agent usage with the user's own key; and revoking one revokes the other. |
| **Backing row per (user, client_id), created on first use** | **Chosen.** Preserves every downstream invariant, gives per-agent attribution for free (one row per agent instance), and makes quota, billing and routing work with no new code. |

### The security property that makes the backing row safe

A backing row has a `key` column, and a real API key is a bearer credential that
does not expire. That is the one genuine risk in this design, and it is closed by a
single rule:

> **The backing key's secret value is never returned to anyone, ever.** It is not
> listable through `/keys`, not shown in the panel, not included in any API
> response, and not derivable from anything the client holds. It exists only as a
> row the server resolves *to*, never a credential the server hands *out*.

Consequences that must hold, and be tested:

- The row is marked as OAuth-backed and excluded from every user-facing key listing
  and key-management endpoint.
- Revoking the OAuth client, or the user, disables the row (soft state change, never
  a delete — see `ON DELETE CASCADE` above).
- The row's secret is generated with the same entropy as a normal key so that a
  hypothetical leak is not *worse* than a normal key leak, but nothing in the
  system ever transmits it.

### Failed-auth accounting (F-B)

The OAuth branch must not inherit the IP-keyed block that can take out a whole
egress IP.

- A token that **verifies** but is unauthorized (missing scope, unknown client,
  disabled user) is a clean `401`/`403` with an RFC-shaped body, and is accounted
  against the **token subject**, not the IP.
- A token that **fails verification** is accounted as it is today — it carries no
  trustworthy identity, so IP is the only key available — but the OAuth branch must
  return promptly rather than looping, and the response must be distinguishable
  from `INVALID_API_KEY` so a client can tell "your token is wrong" from "your key
  is wrong".
- F-B's existing IP-wide block is a **pre-existing fault** and is *not* fixed here.
  It is recorded as a residual with its own follow-up; this design's obligation is
  to not add a second path into it.

### Group assignment

`apiKey.Group` is the only routing input, so the backing row needs one. The group
for an OAuth-backed row is resolved by an explicit, configured policy — not by
picking an arbitrary group — and a user with no resolvable group gets a clear
`403` naming the misconfiguration rather than a nil-group panic downstream.

---

## Explicit non-goals

- **Fixing F-B's IP-wide invalid-auth block.** Pre-existing, its own change.
- **The `/api/billing/*` adapter.** An agent whose user has no balance will get
  `403 INSUFFICIENT_BALANCE` from `/v1` *after* this lands — which is correct
  behaviour, and is the billing sub-project's problem, not this one's.
- **`/v1beta` (Gemini).** It uses `APIKeyAuthWithSubscriptionGoogle`, a different
  middleware. Out of scope unless the agent path needs it; it does not today.
- **Raising or removing the API-key header size cap.** It stays exactly as it is.
- **Retiring API-key auth.** Standalone Inferno customers depend on it. Both
  credential types must work on the same routes, forever.

---

## What "done" means

1. The reproduction above is re-run and `/v1/messages`, `/v1/chat/completions`,
   `/v1/models`, `/v1/responses`, `/responses`, `/v1/alpha/search` and
   `/alpha/search` all get **past auth** with the OAuth token — reaching the same
   outcome a real API key reaches, including `403 INSUFFICIENT_BALANCE` where that
   is what a funded-less user should see.
2. A `usage_logs` row exists for an OAuth-served request, attributed to the right
   `user_id`, and its `api_key_id` names the backing row.
3. A second agent instance for the same user resolves to a **different** backing
   row, so per-agent usage is separable.
4. An API key still works on every one of those routes, unchanged.
5. A JWT that fails verification is rejected without being retried as an API key,
   and without a response that is indistinguishable from `INVALID_API_KEY`.
6. The backing key's secret appears in **no** response body from any endpoint —
   asserted, not assumed.
