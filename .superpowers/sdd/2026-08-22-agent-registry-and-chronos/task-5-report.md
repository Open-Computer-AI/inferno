STATUS: in progress

# Task 5: The firer -- mint, POST, retry, rehydrate

Branch: feat/agent-registry-chronos, starting HEAD 28679038.

## Plan

1. Explore Task 4 interfaces (AgentCronService, ent schema, OAuthKeyService,
   TimingWheelService), oauth_token_service.go's mintAccessToken as the model.
2. Write agent_cron_firer_test.go with the brief's tests verbatim (Steps 1, 6, 7).
3. Implement agent_cron_firer.go to pass them, one test at a time.
4. Mutation-prove Steps 5 and 8.
5. Wire RehydrateOnBoot into cmd/server boot (wire.go + wire_gen.go).
6. Run full gate, fix divergence, commit each green step.


## Step 1-4: JWT claims test + mintFireToken

Wrote `backend/internal/service/agent_cron_firer_test.go` with the brief's
Step 1 test verbatim (`TestFireTokenCarriesEveryClaimTheAgentVerifierRequires`),
plus a fixture (`newFirerFixture`/`firerFixture`) modeled on
`agent_cron_test.go`'s in-memory sqlite pattern, a fake counting timing wheel
(`fakeCronWheel`, structurally satisfying `cronTimingWheel{Schedule,Cancel}`),
a real `*OAuthKeyService`, and `fx.parseWithJWKS` which verifies a minted
token against `OAuthKeyService.JWKS()`'s OWN output (reconstructing the RSA
public key from its n/e), never against the raw private key -- per the
brief's explicit instruction.

First run (before `agent_cron_firer.go` existed):

```
vet: internal/service/agent_cron_firer_test.go:112:38: undefined: AgentCronFirer
```

Implemented `backend/internal/service/agent_cron_firer.go`:
`NewAgentCronFirer(client, keySvc, wheel, issuer)`, `mintFireToken(ctx, aud,
jobID)` modeled on `OAuthTokenService.mintAccessToken`
(`keySvc.Active(ctx)` -> `jwt.NewWithClaims(RS256, ...)` ->
`tok.Header["kid"]` -> `tok.SignedString`), claims `{iss, aud, purpose:
"cron_fire", job_id, iat, exp}`. Refuses to mint on a blank issuer by
reusing `ErrIssuerNotConfigured` (the existing sentinel from
`oauth_token_service.go` -- same failure, same remedy, so no second sentinel).

Also added extra coverage beyond the brief's verbatim test:
`TestMintFireTokenRefusesBlankIssuer`.

Run after implementation:

```
=== RUN   TestFireTokenCarriesEveryClaimTheAgentVerifierRequires
--- PASS: TestFireTokenCarriesEveryClaimTheAgentVerifierRequires (0.25s)
=== RUN   TestMintFireTokenRefusesBlankIssuer
--- PASS: TestMintFireTokenRefusesBlankIssuer (0.01s)
```

Also implemented `FireNow`, `RehydrateOnBoot`, `markFired`, `markRetry` in
the same pass (Steps 6/7 tests written alongside Step 1's), all green:

```
=== RUN   TestNon2xxIsRetriedAndJobGone200IsNot
--- PASS: TestNon2xxIsRetriedAndJobGone200IsNot (0.11s)
=== RUN   TestFireNowRecordsLastError
--- PASS: TestFireNowRecordsLastError (0.06s)
=== RUN   TestFireNowIsIdempotentOnAlreadyFiredRow
--- PASS: TestFireNowIsIdempotentOnAlreadyFiredRow (0.02s)
=== RUN   TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes
--- PASS: TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes (0.01s)
=== RUN   TestFireNowPostsAuthorizationBearer
--- PASS: TestFireNowPostsAuthorizationBearer (0.06s)
```

`go build ./...` clean.

## Design deviation from the brief worth flagging

The brief's "Consumes" line lists `AgentCronService` as a dependency, but
`AgentCronService`'s surface (`Provision`/`Cancel`/`ListArmed`) is
deliberately scoped to ONE calling agent's rows (ruling T4-1/T4-2).
`RehydrateOnBoot` needs every agent's armed rows in one boot-time query, and
`FireNow`/`markFired`/`markRetry` need row-level mutations (state,
attempts, last_error) that `AgentCronService` does not expose. `AgentCronFirer`
therefore talks to `dbent.AgentCronFire` directly -- the same "no repository
layer for this table" choice `agent_cron.go` itself already documents -- and
does NOT hold a `*AgentCronService` field. Documented in `AgentCronFirer`'s
doc comment.

## Step 5: mutation-prove the purpose claim (compiling)

Removed `"purpose": "cron_fire",` from the claims map in `mintFireToken`
(compiling change). Result:

```
=== RUN   TestFireTokenCarriesEveryClaimTheAgentVerifierRequires
    agent_cron_firer_test.go:267:
        	Error:      	Not equal:
        	            	expected: string("cron_fire")
        	            	actual  : <nil>(<nil>)
        	Messages:   	verify.py:140 rejects any token without purpose==cron_fire --
        	            	without this claim an ordinary agent access token would be
        	            	replayable against /api/cron/fire
--- FAIL: TestFireTokenCarriesEveryClaimTheAgentVerifierRequires (0.07s)
```

Reverted (`diff` against a pre-mutation backup confirms byte-identical
restore); full suite green again.

## Step 8: mutation-prove rehydration (compiling)

Made `RehydrateOnBoot` a no-op returning `(0, nil)` before the real body
(dead code after an early return -- compiles; `go build ./...` clean, only
`go vet` would flag it, which was not run in this window). Result:

```
=== RUN   TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes
    agent_cron_firer_test.go:358:
        	Error:      	Not equal:
        	            	expected: 2
        	            	actual  : 0
        	Messages:   	only armed rows; a fired row must never re-fire on rehydrate
--- FAIL: TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes (0.02s)
```

Reverted (byte-identical restore confirmed via `diff`); full suite green
again, `go build ./...` clean.

## Step 9: wire RehydrateOnBoot into server startup

Added `ProvideAgentCronFirer(entClient, keySvc, timingWheel, cfg) *AgentCronFirer`
to `backend/internal/service/wire.go`: constructs the firer and calls
`RehydrateOnBoot(context.Background())` immediately, logging the result
(count or error) rather than propagating a fatal error -- crashing the whole
server's boot (billing, inference, every other endpoint) over one feature's
in-memory-cache rehydration would be strictly worse than the documented
failure mode.

Hand-edited `backend/cmd/server/wire_gen.go`'s `initializeApplication`:
inserted `service.ProvideAgentCronFirer(client, oAuthKeyService,
timingWheelService, configConfig)` right after `agentCronService :=
service.NewAgentCronService(client)` -- `timingWheelService` (constructed +
started at line ~125) and `oAuthKeyService` (line ~313) are both already in
scope there. Its return value is deliberately discarded as a bare statement:
nothing downstream needs a handle on the firer today, since FireNow is only
ever reached through the timing wheel's own callback closures armed by
RehydrateOnBoot (and, for retries, by FireNow's own re-arm).

Also registered `ProvideAgentCronFirer` in the service package's
`ProviderSet` var next to `NewAgentCronService`, with a comment noting it is
NOT wired into `cmd/server/wire.go`'s (the `wireinject`-tagged source) build
graph -- Task 5's declared file list is `internal/service/wire.go` +
`cmd/server/wire_gen.go` only, not the wireinject source, so a future real
`go generate ./...` wire regen would currently DROP this construction unless
`cmd/server/wire.go` is also updated to request it. Flagged as a residual.

`go build ./...`: clean.

## Full gate

```
cd backend && go test -tags unit ./...
```
(`timeout: 600000` on both runs -- once right after wiring, once as the
final check after the divergence-ledger edits.)

Result: **all packages `ok`, zero `FAIL` lines**, including
`github.com/Wei-Shaw/sub2api/cmd/server` (validates the hand-edited
`wire_gen.go` compiles and its own tests pass) and
`github.com/Wei-Shaw/sub2api/internal/service` (160s -- the package this
task's new tests live in). The two documented flakes
(`TestFilterGrokFreeQuotaAccounts*`, `TestApplyCodexFingerprintClientMetadataRaw*`)
did not fire in either run; no re-run was needed.

## Divergence

Baseline before this task: `./inferno-frontend/scripts/check-divergence.sh`
from repo root -> `243 file(s) differ * 254 declared`, exit 1, 9 undeclared
(pre-existing, unrelated to this task).

Declared `backend/internal/service/agent_cron_firer.go` and
`agent_cron_firer_test.go` as D12 in `DECLARED` (script) and GOAL.md's
divergence ledger table (wire.go/wire_gen.go already covered by D8-D11's
blanket declarations).

After this task: `245 file(s) differ * 256 declared`, exit 1, **same 9
undeclared files** (all pre-existing and unrelated to Task 5):
`auth_email_binding.go`, `auth_oauth_email_flow.go`,
`balance_notify_service.go`, `content_moderation.go`,
`domain_constants.go`, `payment_order_result_test.go`,
`setting_features.go`, `setting_service_update_test.go`,
`totp_service.go`.

## Things the brief did not fully specify, and the calls I made

1. **`AgentCronFirer` does not hold `AgentCronService`** (see the "Design
   deviation" note above) -- talks to `dbent.AgentCronFire` directly.
2. **`mintFireToken`'s exact claim set beyond what the test asserts**: added
   `job_id` and `iat` alongside the required `iss`/`aud`/`purpose`/`exp`.
   Not asserted by the brief's test but harmless and useful for the agent's
   own correlation.
3. **HTTP request body shape** (`{job_id, schedule_id, fire_at}` JSON POST):
   invented, since the brief does not specify cron.py's exact request
   schema and that file is not present in this repo (it lives in the
   read-only Hermes client repo this task only has doc-comment citations
   into, not source access). The Authorization header and the response-code
   contract are the parts the brief actually specifies and tests, and both
   are implemented per the brief.
4. **Retry backoff schedule** (`attempts*30s`, capped 10m): invented: the
   brief says "re-arm with backoff" without specifying the curve. Chosen to
   stay well inside `TimingWheelService`'s ~1 hour real capacity.
5. **KNOWN GAP, explicitly flagged, not fixed in this task**:
   `AgentCronService.Provision` (Task 4) does not itself arm anything into
   the timing wheel. A fire provisioned while the server is already running
   is therefore armed only on the server's NEXT boot (via
   `RehydrateOnBoot`), not immediately. The task-5 brief's own produced
   interfaces are exactly `RehydrateOnBoot` and `FireNow` -- there is no
   `ArmNow`/"arm on provision" step in its checklist, and the files listed
   as in-scope for this task do not include `agent_cron.go` or the route
   handler. This looks like a real product gap (an agent that calls
   `/api/agent-cron/provision` mid-session would see nothing fire until the
   server restarts), but it is outside this task's literal, tested scope --
   documented here and in GOAL.md's D12 row for Task 6 (the real-client
   conformance run) to catch if it matters end-to-end.

STATUS: complete
