# Inferno as agent registry + Chronos cron signer — design

**Date:** 2026-08-22
**Status:** Design.
**Depends on:** `feat/oauth-authorization-server` (RS256/JWKS, orgs, the OAuth
token this whole surface authenticates with) and `feat/oauth-bearer-inference`.
**Follows:** the billing contract adapter, which is complete.

---

## Why this exists

Two capabilities the client already has and the portal does not answer:

1. **Hermes Desktop's cloud agent list.** `apps/desktop/electron/main.ts:7811`
   calls `GET {portal}/api/agents` to discover which agents a signed-in user
   owns. Today that 404s, so the Cloud tab has nothing to show.
2. **Unattended work.** The agent's Chronos cron provider
   (`plugins/cron_providers/chronos/`) pushes one-shot schedules to the portal
   and expects the portal to call back at fire time. Today no schedule can be
   armed, so nothing runs unattended.

The second is the load-bearing one. The whole credential model exists so that
**one user login yields an autonomous agent** — the portal pushes signed cron
JWTs and the agent refreshes its own tokens. Without Chronos, "autonomous
forever" is not true.

---

## The contract, read from the client

Every endpoint, field and status below is a real call site. Nothing here is
inferred; the citation is the reason it is in the spec.

### Agent registry

```
GET {portal}/api/agents[?org=<slug>]          main.ts:7811
  200 -> { agents: [ {id, name, status, dashboardUrl, dashboardGatewayState} ],
           org:    {id, slug, name, isPersonal} }
  401 -> desktop attempts ONE silent renewal, then forces re-login (:7827)
  409 -> org_selection_required; body carries the org list for a picker (:7849)
```

`trimCloudAgents` (`main.ts:7918`) **drops any agent whose `id` is not a
string**, and defaults `name`→`id`, `status`→`"unknown"`,
`dashboardGatewayState`→`"unknown"`, `dashboardUrl`→`null`. A wrong type does
not error; it silently removes the agent from the user's list.

`status` and `dashboardGatewayState` are **free-form display strings**, not
enums: `i18n/en.ts:812` renders `cloudStatusLabel: status => "Status: ${status}"`
verbatim. They are the same class of field as `dollarsPerMonthDisplay` — whatever
we emit is what the user reads, so it must be human-readable prose.

### Chronos

Agent -> portal, authenticated with **the agent's existing OAuth access token**
(`_nas_client.py:43-54`) — no new credential type:

```
POST /api/agent-cron/provision   {job_id, fire_at (ISO 8601),
                                  agent_callback_url, dedup_key} -> {schedule_id}
POST /api/agent-cron/cancel      {job_id}
GET  /api/agent-cron/list        -> [ {job_id, fire_at, schedule_id} ]
```

`dedup_key` is `{job_id}:{fire_at}` (`_nas_client.py:96-109`) and exists so
re-arming the same fire is idempotent portal-side. `list` is best-effort,
used by the agent's reconcile after a cold process; on error the agent falls
back to re-arming everything, which the dedup key must absorb.

Portal -> agent at `fire_at` (`web_routers/cron.py:136`):

```
POST {agent_callback_url}/api/cron/fire
Authorization: Bearer <portal-minted JWT>
body: {"job_id": "..."}
```

The agent verifies that JWT against `nas_jwks_url`, `expected_audience` and
`issuer = portal_url` (`cron.py:169-174`). **That is the JWKS Inferno already
serves.** No new key material, no new algorithm — sub-project #4's
`OAuthKeyService` mints it and `/.well-known/jwks.json` already publishes the
verification key.

Two response rules, both from `cron.py`'s own docstring, and both easy to get
backwards:

- **Non-2xx is RETRYABLE.** The agent returns 503 when its gateway is still
  booting specifically so the portal retries.
- **A job that no longer exists returns 200**, so the portal does NOT retry a
  fire that is intentionally absent.

---

## Architecture

### Ownership: Inferno owns identity, oc-platform owns the VM

oc-platform already models the compute plane — `users` (with
`agent_type: "oc" | "hermes"`), `vm_tunnels` (user_id, tunnel_id, hostname,
dns records, revoked_at), `instance_events`, `snapshots`, one VM per user,
RLS tenant isolation. An `agents` table that duplicated VM lifecycle would give
two systems a way to disagree about the same fact.

So Inferno stores **identity and linkage only**: which Inferno user and org own
an agent, its display name, its dashboard URL, and the oc-platform user UUID it
corresponds to. That mapping does not exist anywhere today — oc-platform keys
users by UUID and Inferno by int64 — and creating it is a deliverable in its own
right, independent of anything else here.

**Liveness is deferred, deliberately.** The obvious design calls oc-platform over
the service secret to answer `status`. But `status` turned out to be a printed
string, and the agent already contacts Inferno to register — so a `last_seen_at`
heartbeat answers it with no cross-service dependency on an endpoint the desktop
polls. The oc-platform call would only distinguish *why* an absent agent is
absent (VM stopped vs agent crashed). `agents.oc_platform_user_id` is stored so
that call becomes possible the moment something needs it; it is not made now.

### Registration: the agent registers itself

VM provisioning is sub-project #1 and is scheduled last, so this design cannot
depend on it. The agent self-registers at boot with its `client_id` and tunnel
hostname, bound to the user whose OAuth token it holds. This works for VMs that
exist today, and when #1 lands it automates the same call rather than replacing
it — there is no throwaway path.

### Firing: the table is the truth, the wheel is a cache

Inferno ships `TimingWheelService` (`Schedule(name, delay, fn)` / `Cancel`),
already wired and running. Chronos uses it for timing only.

**The wheel is in-memory.** A restart drops every pending timer, so armed fires
live in `agent_cron_fires` and the wheel is rehydrated from that table on boot.
Getting this wrong loses every scheduled job silently — nothing errors, work
just never happens — so rehydration is a first-class requirement with its own
test, not an implementation detail.

---

## Data model

```
agents
  id                   TEXT PRIMARY KEY   -- public, opaque; the desktop reads a STRING
  user_id              BIGINT NOT NULL    -- Inferno user
  org_id               BIGINT NOT NULL
  name                 TEXT NOT NULL
  dashboard_url        TEXT               -- the agent's tunnel hostname
  oc_platform_user_id  UUID               -- the int64 <-> UUID link; nullable
  last_seen_at         TIMESTAMPTZ        -- heartbeat; drives `status`
  created_at           TIMESTAMPTZ NOT NULL
  revoked_at           TIMESTAMPTZ

agent_cron_fires
  id            BIGSERIAL PRIMARY KEY
  agent_id      TEXT NOT NULL REFERENCES agents(id)
  job_id        TEXT NOT NULL
  fire_at       TIMESTAMPTZ NOT NULL
  callback_url  TEXT NOT NULL
  dedup_key     TEXT NOT NULL UNIQUE      -- {job_id}:{fire_at}; idempotent re-arm
  schedule_id   TEXT NOT NULL             -- what we return to the agent
  state         TEXT NOT NULL             -- armed | fired | cancelled
  attempts      INT NOT NULL DEFAULT 0
  last_error    TEXT
  created_at    TIMESTAMPTZ NOT NULL
```

`dedup_key UNIQUE` is what makes re-arming idempotent, enforced at the database
rather than by a read-then-write that races with itself.

---

## Scope and enforcement

All six endpoints take the agent's or user's OAuth token.

`oauth_scope_vocabulary.go:26-27` defines `agents:read` and `agents:manage`.
**Neither is requested by any client** — checked, not assumed: the strings appear
nowhere in `hermes_cli/auth.py` or `plugins/cron_providers/chronos/`. The Chronos
client authenticates with `resolve_nous_access_token()`, the ordinary token whose
scope is `inference:invoke`; the desktop's discovery call uses the same portal
session.

This is the `billing:read` trap exactly. That branch gated three endpoints on a
scope nothing requests, shipped them permanently 403, and only found out when an
implementer read the client. **Do not repeat it here.**

Ruling, decided now rather than during implementation: these endpoints require a
**valid token and no particular scope**, matching the billing reads (R-1.2). The
data is the caller's own agents and their own schedules; a token minted for that
user already means the holder acts as that user. `agents:read` / `agents:manage`
stay in the vocabulary — removing them would 400 any client that legitimately
asks — but nothing is gated on them until something can grant them.

Every row returned is scoped to the calling user, asserted with a second user's
agents present in the same database.

---

## Silent per-agent sign-in — the known difference

After discovery, the desktop opens `{dashboardUrl}/login` in the same OAuth
partition and expects the agent's `/oauth/authorize` to auto-approve **for an
org member** and 302 back with no prompt (`main.ts:cloudAgentSilentSignIn`).

Inferno deliberately does not do that. Sub-project #4's review found that
org-member auto-approve lets an org peer register a client with an
attacker-controlled redirect URI, send a same-org teammate a link, and have the
victim's browser mint a code naming the victim — with no consent screen ever
rendered. Auto-approve is therefore narrowed to the client's actual registrant
(`oauth_authorize_handler.go:304`).

**Decision: keep the narrowing.** The consequence, stated plainly so it is not
rediscovered as a bug: silent sign-in works on an agent you registered; a
teammate signing into someone else's agent sees one consent screen and approves
deliberately. That is the intended behaviour of this system, not a gap in it.

---

## Failure behaviour

- **A fire's HTTP call fails** -> increment `attempts`, record `last_error`,
  re-arm with backoff. Non-2xx is retryable by the client's own contract.
- **The agent reports the job is gone (200)** -> mark `fired`, never retry.
- **Restart** -> rehydrate `state = 'armed' AND fire_at > now()` into the wheel.
  Rows whose `fire_at` has passed while we were down fire immediately, once.
- **A `fired` row must never re-fire** on rehydration, even if it is replayed.
- **oc-platform unreachable** -> not on any path in this design (liveness
  deferred), so it cannot degrade `/api/agents`.
- **Agent unreachable at fire time** -> retries, then the row ages out; the
  agent's own reconcile re-arms what it still wants.

---

## Explicit non-goals

- **Recurring schedules.** Chronos is a ONE-SHOT scheduler: the agent arms a
  single `fire_at` and re-arms after each fire. No cron expressions, no
  recurrence, no timezone arithmetic.
- **VM provisioning.** Sub-project #1, deliberately last.
- **Calling oc-platform for liveness.** Deferred, with the column in place.
- **Widening auto-approve back to org members.** Decided against, above.
- **A cron UI in the Inferno panel.** The agent owns job definitions; the portal
  only arms and fires.

---

## What "done" means

1. Hermes Desktop's Cloud tab lists the signed-in user's agents, with a name and
   a status, and opening one signs in (silently for the registrant).
2. A multi-org user with no org selected gets `409` carrying the org list, and
   the desktop renders a picker.
3. `GET /api/agents` returns only the caller's agents — asserted with a second
   user's agents present in the same database.
4. An agent arms a one-shot; at `fire_at` it receives
   `POST /api/cron/fire` with a JWT it verifies against Inferno's published
   JWKS; the job runs. Demonstrated end to end against a real agent, not
   asserted from unit tests.
5. Re-arming the same `{job_id}:{fire_at}` twice produces one armed fire.
6. Inferno is restarted with fires armed; every one still fires. This is the
   requirement most likely to be quietly missed, because nothing errors when it
   is broken.
7. A job the agent no longer has answers 200 and is never retried.
8. Every response is bare JSON, never the panel's `{code,message,data}` envelope.
