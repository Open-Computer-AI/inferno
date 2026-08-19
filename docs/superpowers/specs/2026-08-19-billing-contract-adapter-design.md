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
GET  /api/billing/state                     the overview screen's data
GET  /api/analytics/usage                   usage history
GET  /api/billing/subscription              plan state
POST /api/billing/subscription/preview      plan-change quote
POST /api/billing/subscription/upgrade      plan change
GET  /api/billing/subscription/pending-change
POST /api/billing/charge  ·  /charge/{id}   top-up
GET/PUT /api/billing/auto-top-up            auto-reload
```

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
   └─ middleware.RequireOAuthScope(billing:read)
        └─ BillingContractService.State(ctx, userID)
             ├─ userSvc.GetByID           balance
             ├─ orgSvc.OrgsForUser        org + role
             ├─ subscriptionSvc.GetActive plan
             ├─ usageSvc.MonthToDate      spent_this_month
             └─ paymentSvc.Limits/Plans   presets, min, max
```

### Where it mounts, and under which scope

These are **bare, Nous-shaped** endpoints at `/api/billing/*` and
`/api/analytics/*` — not under `/api/v1/`, and **not** on the panel's
`{code,message,data}` envelope. The client parses the raw object. This mirrors
the decision already made for `/api/oauth/*`.

Scope enforcement, using the vocabulary #4 already defines:

| Endpoint | Scope |
|---|---|
| `GET /api/billing/state`, `/subscription`, `/pending-change`, `/auto-top-up` | `billing:read` |
| `GET /api/analytics/usage` | `billing:read` |
| `POST /charge`, `/subscription/upgrade`, `PUT /auto-top-up` | `billing:manage` |

`billing:manage` is the scope `oauth_scope_vocabulary.go` documents as *"never
granted at initial login; must be elevated to via a second device flow"* — and
which `/oauth/authorize` now refuses outright. **That rule stays.** The write
endpoints exist and are correctly gated; nothing can reach them until the
step-up flow is built, which is deliberate. A client asking for them today gets
`403 insufficient_scope`, which is the honest answer.

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
- **The `billing:manage` step-up device flow.** The write endpoints are built and
  gated; the elevation flow that lets anyone reach them is its own change.
- **Retiring `/api/v1/payment/*`.** The panel uses it. This adapter sits beside
  it, translating for one client; it does not replace Inferno's own surface.
- **Per-org monthly ceilings.** `limit_usd` stays null until the model exists.

---

## What "done" means

1. `/topup` and `/subscription` in the real CLI stop pointing at
   `portal.nousresearch.com` and show Inferno's numbers.
2. `GET /api/billing/state` returns a `BillingState` the unmodified client parses
   with `logged_in=true`, a correct `balance_usd`, and the right org and role.
3. `GET /api/analytics/usage` returns this user's usage — and **only** this
   user's, asserted with a second user's data present in the same database.
4. A token without `billing:read` gets `403 insufficient_scope`, not data.
5. A token with `billing:read` but not `billing:manage` gets `403` from every
   write endpoint — asserted, because that is the boundary protecting a scope
   nothing can currently grant.
6. Every endpoint is bare-JSON, never the panel envelope — asserted, since
   getting this backwards is a defect the client silently mis-parses.
7. With the usage rollup deliberately failing, `/api/billing/state` still returns
   a balance rather than a 500.
