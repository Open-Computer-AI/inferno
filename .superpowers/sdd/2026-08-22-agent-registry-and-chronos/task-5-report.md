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

---

# Fix round 1 of 5

Coordinator ruling: two of my four reported concerns (#1 ArmNow gap, #3
wireinject-source drift) are real defects to fix; two (#2 no AgentCronFirer
field, #4 invented backoff/body) are correct as built, do not change.

## FIX 1 (ruling T5-1): Provision now arms freshly-provisioned fires immediately

Added `AgentCronFirer.ArmNow(ctx, fireID, fireAt)` (agent_cron_firer.go),
sharing `fireWheelKey` with `RehydrateOnBoot`/`markRetry` -- verified against
go-zero's actual `TimingWheel.setTask` source (`moveTask`, looked up by key)
that a re-arm of the same key MOVES the existing timer rather than adding a
second one, not just assumed.

`AgentCronService` now depends on the firer through a narrow, unexported
`cronArmer` interface (`ArmNow` only) rather than a concrete
`*AgentCronFirer` field -- keeps the dependency direction one-way
(`AgentCronService -> AgentCronFirer`), matching the coordinator's note that
concern #2 (the firer not holding `*AgentCronService`) is exactly what keeps
this acyclic. `NewAgentCronService`'s signature changed to
`(client, armer cronArmer)`; `armer` is REQUIRED, not nil-checked -- a
nil-tolerant "skip arming silently" design would reintroduce the exact
silent-failure class this whole task exists to close.

Provision calls `s.armer.ArmNow(ctx, row.ID, row.FireAt)` unconditionally
right after the upsert commits, before returning the view.

**Extra defect found and fixed while wiring this** (not requested, flagged
here rather than silently bundled): go-zero's `TimingWheel.SetTimer` REJECTS
`delay <= 0` outright (`ErrArgument`), and `TimingWheelService.Schedule` only
LOGS that rejection rather than firing immediately or erroring
(`timing_wheel_service.go`). `RehydrateOnBoot`'s prior "clamp overdue delay to
0" would therefore have silently NEVER scheduled an overdue fire against the
REAL wheel -- exactly the failure class this task exists to close, just one
level deeper. Added `fireMinDelay = time.Millisecond` and a shared
`clampFireDelay` helper, used by both `RehydrateOnBoot` and the new `ArmNow`.

**Call sites updated:**
- `cmd/server/wire_gen.go`: `agentCronFirer` now constructed BEFORE
  `agentCronService` (dependency order), threaded into `NewAgentCronService`.
- `internal/service/agent_cron_test.go`: added `fakeCronArmer` (keyed by
  fireID, mirroring the real wheel-key collapse behavior) and threaded it
  through `newAgentCronFixture`.
- `internal/server/routes/agents_route_test.go`: added `noopCronArmer` (these
  are wire/routing tests, not firer-behavior tests -- already covered
  directly in `internal/service`).

**New tests** (`agent_cron_test.go`):
- `TestProvisionArmsTheNewRowIntoTheTimingWheel`
- `TestProvisionReArmDoesNotLeaveTwoTimersForOneRow`

**Mutation proof** (compiling): removed the `s.armer.ArmNow(...)` call from
`Provision`. Result:

```
=== RUN   TestProvisionArmsTheNewRowIntoTheTimingWheel
    Error: Not equal: expected: 1, actual: 0
    Messages: Provision must arm exactly one row into the current process's timing wheel
--- FAIL: TestProvisionArmsTheNewRowIntoTheTimingWheel (0.01s)
=== RUN   TestProvisionReArmDoesNotLeaveTwoTimersForOneRow
    Error: Not equal: expected: 1, actual: 0
    Messages: a re-arm of the same row must not leave two timers for it
--- FAIL: TestProvisionReArmDoesNotLeaveTwoTimersForOneRow (0.01s)
```

Reverted (byte-identical restore confirmed via `diff` against a pre-mutation
backup); full suite green again.

Commit: `a100e28164`.

## FIX 2 (ruling T5-2): a real wire regen no longer drops the firer

Added `wire.Bind(new(cronArmer), new(*AgentCronFirer))` to
`internal/service/wire.go`'s `ProviderSet` -- has to live there, not in
`cmd/server/wire.go`, because `cronArmer` is unexported and a bind in
another package could not even spell the type name.

**This closes the concern as a direct consequence of FIX 1**: before FIX 1,
`ProvideAgentCronFirer` was a true orphan -- nothing in the requested output
graph (`Application.Server`/`PromptAudit`/`Cleanup`) consumed its result, so
a real regen genuinely would have dropped it, exactly as flagged. After FIX
1, `AgentCronService` has a real, bind-satisfied dependency on it via
`cronArmer`, and `AgentCronService` is transitively required to build
`Application.Server` (through `AgentHandler` -> the handler set -> the
router). So the bind alone -- inside a file already covered by
`cmd/server/wire.go`'s existing `service.ProviderSet` inclusion -- is
sufficient.

**Verified empirically, not just asserted**, two ways:

1. `go run -mod=mod github.com/google/wire/cmd/wire check ./cmd/server/...`
   -- passed with zero output (wire's `check` only prints on failure).
2. Ran the REAL generator against the unmodified `cmd/server/wire.go`:
   `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server/...`. It
   wrote a new `wire_gen.go`; diffed against the hand-maintained file from
   the FIX 1 commit:

```
git diff --stat cmd/server/wire_gen.go
 backend/cmd/server/wire_gen.go | 9 +--------
 1 file changed, 1 insertion(+), 8 deletions(-)
```

   The only changes: `idempotencyCoordinator`'s construction moved a few
   lines later, and my hand-written explanatory comment was dropped (real
   codegen carries no interspersed comments). The
   `agentCronFirer`/`agentCronService` construction lines are IDENTICAL to
   what I had hand-written. `gofmt -l` clean; `go build ./...` and
   `go build -tags wireinject ./cmd/server/...` both pass.

**`cmd/server/wire.go` (the wireinject source) is intentionally left
UNCHANGED**, deviating from the coordinator's literal "add the provider to
cmd/server/wire.go" -- flagged explicitly rather than silently done: the
graph resolves through the ALREADY-present `service.ProviderSet` inclusion
plus the new bind, proven by the regen-diff above. That file is not
currently in the declared-divergence set; adding a textual change there
(even comment-only) would cost a permanent ledger entry per
`check-divergence.sh`'s own stated policy ("Do not add a row just to make
this pass") for a change that empirically makes no functional difference. I
adopted the REAL generator's output for `wire_gen.go` rather than my
hand-maintained approximation of it, which is strictly stronger evidence
that nothing is silently dropped than a hand edit would have been.

Commit: `4456ee2493`.

## Full gate + divergence, both fixes together

```
cd backend && go test -tags unit ./...
```
(`timeout: 600000`.) Result: 52 `ok`, 0 `FAIL`. `go vet -tags unit ./...`
clean. `gofmt -l` clean on every touched file.

`./inferno-frontend/scripts/check-divergence.sh` from repo root: `245
file(s) differ * 256 declared`, exit 1, **same 9 undeclared files as
baseline** (unchanged, unrelated pre-existing divergence) -- no new
DECLARED entries were needed since every touched file in this round
(`agent_cron.go`, `agent_cron_firer.go`, `agent_cron_test.go`,
`agents_route_test.go`, `wire.go`, `wire_gen.go`) was already declared
under D8-D12.

STATUS: complete (fix round 1 of 5 addressed; awaiting further rounds if any)
