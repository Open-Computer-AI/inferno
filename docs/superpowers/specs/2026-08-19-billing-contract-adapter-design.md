# The `/api/billing/*` contract adapter — design

**Date:** 2026-08-19
**Status:** Design. Implements the adapter that #4's spec listed under
"Non-goals (explicitly deferred to later sub-projects)": *"The `/api/billing/*`
contract adapter — later, tracked separately."*
**Depends on:** `feat/oauth-authorization-server` (the AS, orgs, `/api/oauth/account`)
and `feat/oauth-bearer-inference` (`/v1` on an OAuth token).

---

## Why this exists

Inferno is OC Portal. It replaces Nous Portal for a client that is upstream
`hermes-agent`, and that client asks its portal a set of questions Inferno does
not currently answer:

```
GET    /api/billing/state                        nous_billing.py:477   overview data
GET    /api/billing/subscription                 nous_billing.py:555   plan state
POST   /api/billing/subscription/preview         nous_billing.py:586   chargeless quote
POST   /api/billing/subscription/upgrade         nous_billing.py:669   immediate upgrade
PUT    /api/billing/subscription/pending-change  nous_billing.py:623   set end-of-period intent
DELETE /api/billing/subscription/pending-change  nous_billing.py:641   clear it (resume/undo)
POST   /api/billing/charge                       nous_billing.py:522   buy credits
GET    /api/billing/charge/{id}                  nous_billing.py:545   poll a charge
PATCH  /api/billing/auto-top-up                  nous_billing.py:492   configure auto-reload
```

**Every row above is a real call site in the client, cited by file and line.**
Nothing goes in this table that is not one. An earlier version of this table was
built by grepping the hermes repo for path strings, and shipped three errors that
survived into the plan (F-10):

- `GET /api/analytics/usage` was listed as usage history. It is **not a portal
  endpoint at all** — `hermes_cli/web_server.py:15361` is the AGENT'S OWN
  dashboard route, served by the local hermes web server, called by
  `web/src/lib/api.ts:512` and `apps/desktop/src/hermes.ts:1841` against
  localhost, backed by local sqlite. `nous_billing.py` and `auth.py` contain the
  string "analytics" zero times. It was built (b7670aa9) and reverted (3cae8f63).
- auto-top-up was listed `GET/PUT`. The client sends **PATCH**, and never GETs it
  — the current auto-reload state arrives inside `/api/billing/state`.
- pending-change was listed `GET`. The client sends **PUT** and **DELETE**; there
  is no GET, because the pending change is also read from `/api/billing/state`.

The last two would each have answered 405 to the real client, silently: the
client fails open, so a 405 shows as a degraded screen, not an error.

**A path string carries no direction.** Finding `"/api/x"` in a client repo does
not mean the client CALLS it — it may be a route the client SERVES. Read the call
site, not the string.

All of them 404 today. Two consequences observed live on 2026-08-19:

1. `/topup` and `/subscription` fall back to `portal.nousresearch.com/billing` —
   **our users get sent to Nous's billing page.** That is a live bug today,
   independent of this adapter.
2. The CLI shows degraded/unknown states for balance and entitlement even when
   Inferno knows the answer.

**Inferno already holds the data for nearly all of it.** This is a contract
adapter — a translation layer over endpoints that exist — not new billing
functionality. That is the single most important fact shaping the design.

---

## What Inferno already serves (user-facing only)

Admin surface is deliberately excluded: the client is a *user* agent holding a
user's OAuth token, and nothing it asks for should require an admin route.

| Inferno endpoint (user-facing) | Holds |
|---|---|
| `GET /api/v1/user/profile` | balance, user identity |
| `GET /api/oauth/account` | user, orgs, role, `paid_service_access` |
| `GET /api/v1/subscriptions/active` | the active subscription |
| `GET /api/v1/subscriptions/summary` | plan summary |
| `GET /api/v1/subscriptions/progress` | period progress |
| `GET /api/v1/subscriptions` | subscription list |
| `GET /api/v1/usage` · `/usage/stats` | usage rows, aggregates |
| `GET /api/v1/usage/dashboard/*` | trend, models, snapshot |
| `GET /api/v1/user/api-keys/:id/usage/daily` | per-key daily usage |
| `GET /api/v1/user/platform-quotas` | per-platform quota |
| `GET /api/v1/payment/plans` | purchasable plans |
| `GET /api/v1/payment/limits` | min/max top-up bounds |
| `GET /api/v1/payment/config` · `/checkout-info` | provider config |
| `POST /api/v1/payment/orders` | create a top-up order |
| `GET /api/v1/payment/orders/my` · `/:id` | order history |

---

## The contract, mapped

`GET /api/billing/state` is the load-bearing one — it is what the CLI's overview,
low-balance warnings and `/usage` all read. The client parses it into
`agent/billing_view.py`'s `BillingState`, and **fails open**: any failure yields
`logged_in=False` and a clean message rather than a crash. That fail-open
behaviour is what makes a partial implementation safe to ship.

| `BillingState` field | Source in Inferno | Status |
|---|---|---|
| `logged_in` | the bearer verified | ✅ free |
| `org_id`, `org_slug`, `org_name`, `role` | orgs + org_members (built in #4) | ✅ have it |
| `balance_usd` | `users.balance` | ✅ have it |
| `can_change_plan_raw` | org role | ✅ derivable |
| `charge_presets`, `min_usd`, `max_usd` | `/api/v1/payment/limits`, `/payment/plans` | ✅ have it |
| `cli_billing_enabled` | a setting | ✅ trivial |
| `bounds.limit_usd`, `spent_this_month_usd` | `user_platform_quota`, `usage_logs` | ⚠️ partial |
| `card` (kind, brand, payment_method_id) | — | ❌ **gap** |
| `auto_reload` (enabled, threshold, reload_to) | — | ❌ **gap** |

### The two genuine gaps, and what to do about them

**`card` — stored payment methods.** Inferno's payment model is *order-based*:
you create an order, pay it through a provider (Razorpay), the order settles.
There is no stored-card vault, no `payment_method_id`. Nous's shape assumes one.

> **Decision:** report `card.kind = "none"`. That is the honest answer and the
> client already handles it — the CLI then routes top-up through the order flow
> instead of offering a one-click charge. Do NOT invent a card object.

**`auto_reload` — auto top-up.** Does not exist in Inferno at all. It needs a
stored payment method to be meaningful, so it inherits the gap above.

> **Decision:** report `auto_reload.enabled = false`, and answer
> `GET /api/billing/auto-top-up` with the same. `PUT` returns `501` with a
> message naming the reason, rather than `404` — a client that can distinguish
> "not supported here" from "endpoint missing" can say something useful.

**`bounds` — monthly spend ceiling.** Inferno has per-platform quota and full
`usage_logs`, so `spent_this_month_usd` is a straight aggregate. A per-org
monthly *ceiling* is not modelled.

> **Decision:** populate `spent_this_month_usd` from `usage_logs`; report
> `limit_usd = null` with `is_default_ceiling = false`. A null ceiling reads as
> "no limit configured", which is true.

---

## Design

### Shape

A thin adapter package that composes existing **services** — not one that
proxies Inferno's own HTTP endpoints. Calling ourselves over HTTP to answer one
request would double the latency and hide errors behind a second status code.

```
GET /api/billing/state
   └─ middleware.RequireOAuthBearer()        // valid token, no scope -- see R-1.2
        └─ BillingContractService.State(ctx, userID)
             ├─ userSvc.GetByID           balance
             ├─ orgSvc.OrgsForUser        org + role
             ├─ subscriptionSvc.GetActive plan
             ├─ usageSvc.MonthToDate      spent_this_month
             └─ paymentSvc.Limits/Plans   presets, min, max
```

### Where it mounts, and under which scope

These are **bare, Nous-shaped** endpoints at `/api/billing/*` and
not under `/api/v1/`, and **not** on the panel's
`{code,message,data}` envelope. The client parses the raw object. This mirrors
the decision already made for `/api/oauth/*`.

Scope enforcement, using the vocabulary #4 already defines:

| Endpoint | Scope |
|---|---|
| `GET /api/billing/state`, `GET /subscription` | valid token, no scope |
| `POST /charge`, `GET /charge/{id}` | `billing:manage` |
| `POST /subscription/preview`, `POST /subscription/upgrade` | `billing:manage` |
| `PUT` and `DELETE /subscription/pending-change` | `billing:manage` |
| `PATCH /auto-top-up` | `billing:manage` |

**Reads take a valid token and no particular scope** (ruling R-1.2). `billing:read`
is in the vocabulary but no client ever requests it — hermes's default scope is
`inference:invoke` and its only step-up asks `billing:manage` — so gating a read on
it ships a dead endpoint. The data is the caller's OWN balance, plan and spend, and
a token minted for that user already means the holder acts as that user. The same
ruling was applied to `/v1/usage` and `/v1/sub2api/billing` in 569c7e3e, so one rule
now covers every "read your own billing data" surface. Residual exposure is parked
as F-9: revisit if third-party clients ever get real users.

`billing:manage` is the scope `oauth_scope_vocabulary.go` documents as *"never
granted at initial login; must be elevated to via a second device flow"* — and
which `/oauth/authorize` refuses outright. **That rule stays.**

**CORRECTED (whole-branch review, finding C-1).** An earlier version of this
section said "nothing can reach them until the step-up flow is built". That was
false and was never tested. The step-up flow **already exists on both sides**:
`OAuthDeviceService.RequestCode` validates only against `knownScopes`, which
contains `billing:manage`, and `hermes_cli/auth.py`'s `step_up_nous_billing_scope`
runs exactly that device flow. Verified live — the device grant mints a token
whose `scope` claim carries it, and the write endpoints accept it.

The asymmetry between the two grants is deliberate, not an oversight: RFC 8628's
device flow puts a human approval step in front of the elevation, which the
redirect flow's silent re-consent does not.

So a **stock** login token (`DEFAULT_NOUS_SCOPE = "inference:invoke"`) gets
`403 insufficient_scope`, which is the honest answer for that token. A token that
has been through the step-up reaches the handlers — and there the refusal is the
response **body**, not the gate.

### Fail-open is the client's, not ours

`build_billing_state` fails open on our behalf. That means our failure mode
should be *loud on our side, quiet on theirs*: log the real error, return a
well-formed object with the fields we could resolve. Never 500 the whole
response because one optional aggregate failed — the balance is useful even when
the usage rollup is down.

### `subscription` and `tool_access` in `/api/oauth/account`

Two fields the client reads that #4 deliberately omitted. Now resolvable:

- **`subscription`** — map from `subscriptionSvc.GetActive`. Absent (not null)
  when there is none, which the client reads as "no plan".
- **`tool_access`** — Inferno has no per-tool entitlement model. Omit it, as #4
  does today. Omission reads as "unknown" and the client degrades; inventing a
  permissive object would grant capabilities nobody modelled.

---

## Explicit non-goals

- **Stored payment methods / card vault.** Order-based checkout stays.
- **Auto top-up.** Depends on the above.
- ~~**The `billing:manage` step-up device flow.**~~ **NOT a non-goal — it already
  works.** Listed here originally on the false premise that it was unbuilt (C-1).
  Both halves ship today: the device grant issues the scope and the client asks
  for it. Nothing in this branch needs to build it.
- **Retiring `/api/v1/payment/*`.** The panel uses it. This adapter sits beside
  it, translating for one client; it does not replace Inferno's own surface.
- **Per-org monthly ceilings.** `limit_usd` stays null until the model exists.

---

## What "done" means

1. `/topup` and `/subscription` in the real CLI stop pointing at
   `portal.nousresearch.com` and show Inferno's numbers.
2. `GET /api/billing/state` returns a `BillingState` the unmodified client parses
   with `logged_in=true`, a correct `balance_usd`, and the right org and role.
3. Every response is asserted to contain **only the calling user's** data, with a
   second user's rows present in the same database — the assertion is worthless
   without them.
4. Every path in the table above is served at the **exact method the client sends**.
   A wrong method answers 405, which the fail-open client shows as a degraded
   screen rather than an error, so this is asserted per row, not assumed.
5. A token without `billing:manage` gets `403` from every
   write endpoint — asserted, because that is the boundary protecting a scope
   nothing can currently grant.
6. Every endpoint is bare-JSON, never the panel envelope — asserted, since
   getting this backwards is a defect the client silently mis-parses.
7. With the usage rollup deliberately failing, `/api/billing/state` still returns
   a balance rather than a 500.
