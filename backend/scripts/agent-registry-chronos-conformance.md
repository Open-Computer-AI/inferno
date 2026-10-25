# Sub-project #2 conformance — agent registry + Chronos (2026-08-22)

Run against a live Inferno built from `feat/agent-registry-chronos` on
`127.0.0.1:18481`, against a FRESH `agentsdb` so the migrations ran for real.
Assertions are made by the CLIENT'S OWN code where one exists — the desktop's
`trimCloudAgents` projection and the agent's own Chronos `verify.py` — not by
reading JSON and judging it.

## Migrations applied to a real Postgres

    agents, agent_cron_fires                     created
    unique index on agent_cron_fires             agentcronfire_agent_row_id_dedup_key

That index name is ruling T4-1 verified at the database: dedup_key uniqueness is
COMPOSITE with agent_row_id, not global. The single-column version let one agent's
provision collide into another agent's row.

## Agent registry

    POST /api/agents/register  -> 200
    GET  /api/agents           -> 200
    {"agents":[{"id":"agent:conformance","name":"Conformance Box",
                "status":"online","dashboardUrl":"https://agent.localtest.me",
                "dashboardGatewayState":""}],
     "org":{"id":"1","slug":"org-058a6a16","name":"org-058a6a16","isPersonal":true}}

Run through `trimCloudAgents` / `trimCloudOrg` ported VERBATIM from
`apps/desktop/electron/main.ts:7918-7929` and `:7866-7878`:

    agents surviving the desktop filter: 1
    org: {"id":"1","slug":"org-058a6a16","name":"org-058a6a16"}

`id` survives as a string, so the agent is not silently dropped.

### FINDING C-1 (Minor) — `dashboardGatewayState` is "" and renders as a blank label
`i18n/en.ts:812` renders `cloudStatusLabel: status => "Status: ${status}"`
VERBATIM, so the UI shows "Status: " with nothing after it. The desktop's own
fallback is `'unknown'` — but that only applies when the value is NOT a string,
and "" IS a string, so it passes straight through. We should emit "unknown"
(matching the desktop's own intent) rather than an empty string.

## Agent onboarding — the real path, and a gap in it

`POST /api/agent-cron/*` resolves the calling agent from its token's `aud` claim,
so a `hermes-cli` token cannot arm anything: it correctly answered
`404 AGENT_NOT_FOUND`. The real path is:

    POST /api/oauth/self-hosted-client  -> {"client_id":"agent:6db39220..."}
    POST /api/agents/register            (public_id = that client_id)
    device flow with client_id=agent:6db39220...  -> token with aud = that id

Two constraints on the client registration are deliberate and were confirmed live:
`redirect_origin` must be **https** and must NOT be a loopback host — the RFC 8252
brokering rule. `https://agent.localtest.me` satisfies both while still resolving
to 127.0.0.1.

### FINDING C-2 (Important, out of scope for this sub-project) — nothing activates a self-hosted client
A registered self-hosted client lands `status = pending`, and `UsableByClientID`
filters on status, so it cannot run any flow. I activated it with a direct
`UPDATE oauth_clients SET status='active'` to continue. **No API, panel screen or
automated path does this today.** Until one exists, agent self-registration cannot
complete without a manual database edit. Belongs to sub-project #1 (provisioning)
or an admin surface, not here — recorded so it is not rediscovered as a bug.

## Chronos — a real fire, end to end

    POST /api/agent-cron/provision  fire_at = +15s  -> 200 {schedule_id}
    row: conformance-job | armed | attempts 0
    ... 15 seconds later, at the listener on 127.0.0.1:19099 ...
    FIRE RECEIVED
    body: {"job_id":"conformance-job","schedule_id":"cron_963abbb2-...","fire_at":"..."}
    row: conformance-job | fired | attempts 0

JWT header/claims:

    {"alg":"RS256","kid":"XuVpIZsVdwG99CyP3uCy3g","typ":"JWT"}
    {"aud":"agent:6db39220...","exp":...,"iat":...,
     "iss":"http://127.0.0.1:18481","job_id":"conformance-job",
     "purpose":"cron_fire"}

This is ruling T5-1 verified: before that fix `Provision` armed nothing into the
wheel and a fire only went out on the NEXT boot. It also exercises the delay clamp
added in the same round — `TimingWheelService.Schedule` swallows `SetTimer`'s
`delay <= 0` rejection into a log line, so an unclamped overdue delay would have
scheduled nothing, silently.

## Verified by the agent's OWN verifier, not by us

`plugins/cron_providers/chronos/verify.py`, resolving the key through our served
`/.well-known/jwks.json`:

    RESULT: VERIFIED
    claims: {'aud': 'agent:6db39220...', 'purpose': 'cron_fire', ...}

    wrong audience                        -> REJECTED (correct)
    an ordinary agent ACCESS token        -> REJECTED
      "cron fire: token missing/!=cron_fire purpose claim"

That last line is the security boundary of this whole feature, demonstrated with a
REAL token rather than asserted: same `aud`, same issuer, valid signature, and it
is refused for one reason only — no `purpose` claim. Without it, any routine
inference token would trigger arbitrary jobs.

## Restart durability — done-criterion 6

The requirement most likely to ship broken, because nothing errors when it is:

    arm restart-job for +75s
    kill the server
    armed rows surviving in the DB: restart-job | armed
    restart
    [AgentCronFirer] rehydrated 1 armed fire(s) on boot
    ... FIRE ARRIVED, purpose=cron_fire, job_id=restart-job
    row: restart-job | fired

## Result — CORRECTED after the whole-branch review

**7 of 8 done-criteria met, not 8.** The original wording of this section claimed
8/8 and it was wrong.

**Done-criterion #1 ("Hermes Desktop's Cloud tab lists the signed-in user's
agents") is NOT demonstrated.** What is demonstrated is that the response SHAPE
survives `trimCloudAgents` — proven by porting that projection verbatim over a
response fetched with a hand-minted bearer token. That says nothing about the
AUTHENTICATION SCHEME, and the scheme is the part that fails:

  - `GET /api/agents` is bearer-only (`routes/agents.go` -> `RequireOAuthScope`,
    which reads `Authorization: Bearer` and has no cookie path).
  - The desktop's `discoverCloudAgents` gates on `hasLivePortalSession()`, which
    checks for a **`privy-token` cookie on the portal host**
    (`apps/desktop/electron/main.ts:7459-7460`). Inferno never sets Privy
    cookies, so the desktop throws `needsCloudLogin` BEFORE issuing any request.
  - Its fetch path uses `useSessionCookies: true` and attaches no bearer —
    `main.ts:7781` says so outright: "no bearer needed — NAS accepts the cookie".

This is plausibly a program-level premise of "Inferno replaces Nous Portal" (the
desktop needs an Inferno-shaped login either way, and that is not this branch's
job). The defect that IS this branch's is that nothing recorded it, and this
document asserted a criterion it had not tested. Corrected here rather than left
standing; the ruling on what to do about it is in the ledger.

This is the same error the billing branch made and recorded as I-1: asserting
through the client's PARSERS and calling it conformance with the client. Parser-
level evidence proves shape. It cannot prove auth, transport, or anything the
consumer does before it parses.

The other seven criteria stand as recorded above, including the two that matter
most — a real fire verified by the agent's own `verify.py`, and survival of a
server restart.

Findings: C-1 (blank gateway-state label — PROMOTED to blocking by the review as
IM-1, since `dashboardGatewayState` turns out to be the only status text the
desktop renders) and C-2 (no activation path for a pending self-hosted client —
triaged as genuinely out of scope, sub-project #1's).


---

# SUPERSEDED IN PART — the CR-2 fix invalidates this run's reproduction (2026-08-22)

The whole-branch review's **CR-2** (SSRF: `agent_callback_url` was unvalidated and
we POSTed to it carrying a JWT we had signed) is now fixed in `07f1dcc881`:
`Provision` validates the callback (https, absolute, no userinfo, no loopback /
private / link-local / metadata addresses) AND the firer dials through
`newSSRFSafeHTTPClient`.

**That means the run recorded above can no longer be reproduced as written.** It
used `http://127.0.0.1:19099`, which is now rejected twice over — once at
`Provision` for being http and loopback, and again at dial time by
`safeDialContext`. `https://agent.localtest.me` would pass `Provision` but still
be refused when dialled, because it resolves to 127.0.0.1.

This does not retract the evidence. What was proven on 2026-08-22 stands: a real
fire reached a listener, the agent's own `verify.py` returned VERIFIED, an
ordinary access token was REJECTED at the fire endpoint, and a fire survived
killing and restarting the server. It was proven against code that then changed
underneath it, which is the normal life of a conformance record.

**To re-run it after the CR-2 fix**, one of:
  - terminate the callback on a real non-loopback address (a tunnel to a host
    with a public IP), or
  - assert the new contract instead: `Provision` answers 200 for a valid public
    https callback, and the fire is refused at dial time with the SSRF client's
    error recorded in `last_error`.

Do not "fix" this by relaxing the callback validation. The whole point of CR-2 is
that we sign a token and hand it to whatever address the caller names.

# A THIRD FLAKE CLASS, found during the fix wave — a TIME BOMB, not a race

`TestFireTokenCarriesEveryClaimTheAgentVerifierRequires` and
`TestFireNowPostsAuthorizationBearer` began failing at 12:05 UTC on 2026-08-22
with `token has invalid claims: token is expired`. The fixture minted at a FAKE
`2026-08-22T12:00Z` with a 5-minute TTL while `parseWithJWKS` validated `exp`
against the WALL CLOCK. It passed until real time crossed 12:05, then failed
forever. Reproduced on clean `ad1d3386` before any fix-wave change, and fixed in
`d8ac8f8eef` by pinning the parser to the fixture clock.

This one is worse than the two known races, because it DECAYS rather than
flickering: every "52 ok, exit 0" recorded on this branch before 12:05 UTC was
true when written and is not reproducible now. A green gate has a shelf life if
any test compares a minted `exp` against real time. The project's known-flake
list now has three entries, and this is the only deterministic one.
