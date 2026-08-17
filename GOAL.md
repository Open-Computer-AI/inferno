# GOAL — finish the June conversion

Single entrypoint. Read this first, work top-down, stop when a gate fails.
Build history and design rationale live in `INFERNO-BUILD.md`; this file is the
backlog and the definition of done.

Baseline taken 2026-08-13, immediately after the dashboard chart conversion:

```
conversion: 118/326 files (36.2%), 14674 legacy utilities across 113384 lines
224 test files / 1588 tests green · june-lint clean across 129 files
```

---

## The one number

```bash
cd inferno-frontend && node scripts/conversion-status.mjs
```

`filesRemaining` and `legacyUtilitiesRemaining` must **only ever go down**.
`--json` for machine use. This reports; it does not gate. The gate is below.

---

## GATES — every one must pass before any commit

Run from `inferno-frontend/`. A red gate means stop and fix, never commit past it.

| # | command | pass condition |
|---|---------|----------------|
| 1 | `npm run typecheck` | zero output after the banner |
| 2 | `node scripts/june-lint.mjs` | `clean across N files` |
| 3 | `npm run test:run` | all green, **and the test count never drops** |
| 4 | `npm run build` | `built in …`, no new errors |
| 5 | `./scripts/check-divergence.sh` | every changed file appears in the divergence ledger below |
| 6 | `node scripts/conversion-status.mjs` | remaining count strictly lower than the line above |

Gate 5 changed meaning on 2026-08-15. It used to require the diff be **empty**.
That was right while Inferno was a pure restyle and wrong the moment it started
becoming its own product: upstream is not going to grow the features we want,
so backend divergence is the goal, not a defect. Requiring zero forbade the
product from existing.

It now requires that divergence be **declared**. The failure it still catches is
the one that actually happened: `84a3c4ac`, a commit whose subject said
`feat(ui):`, quietly regenerated ent and added a migration. Accidental drift is
still fatal. Deliberate drift is legal, listed, and survives reconciles.

**Measure against the base HEAD sits on, never against a freshly fetched
`upstream/main`.** Against a stale ref it under-reports; against a fresh one it
counts upstream's own movement as our drift. On 2026-08-15 the same tree read
19 files against the base and 246 against fresh upstream, of which 227 were
upstream moving on. `check-divergence.sh` uses `git merge-base`, which is the
only ref that answers "what have *we* changed". The daily reconcile gets this
right by rebasing (step 3) *before* asserting (step 4).

### The divergence ledger

Anything under `backend/`, `frontend/`, `deploy/` or `docs/` must appear here
**and** in `DECLARED` inside `scripts/check-divergence.sh`. A file that differs
and is not listed is a bug, not a feature.

| # | area | files | why | re-apply after rebase |
|---|------|-------|-----|-----------------------|
| D1 | `avatar_seed` on `users` | `ent/schema/user.go` (3 lines) + `ent/` regeneration (8 files, 286 lines) + `dto/{types,mappers}.go`, `user_handler.go`, `api_key_repo.go`, `user_repo.go`, `service/user.go`, `user_service.go` (15 lines) + `migrations/900_add_user_avatar_seed.sql` | Persists a regenerated identicon server-side so it survives reload and syncs across devices. Upstream stores avatars in a separate table and has no seed concept. | **Do not hand-merge the `ent/` files.** Re-run `go generate ./ent` and let codegen rebuild from the 3-line schema. Only `ent/mutation.go` realistically conflicts — it is shared across every entity, so any upstream schema change touches it. |
| D2 | English legal-document defaults | `service/setting_public.go` (4 strings) + `server/api_contract_test.go` (golden fixture) | Chinese defaults on the legal documents of a rebranded English product. Applies only when no `login_agreement_documents` settings row exists — i.e. a fresh install, which is exactly when it matters. | Trivial. Zero upstream commits to this file in the 124 we were behind. |
| D3 | `.gitignore` | appended negations | Lets `inferno-frontend/` and `docs/superpowers/` be tracked (upstream allowlists `docs/*`). Outside the four gated dirs, so the script does not see it; listed for completeness. | Trivial. |
| D4 | OpenComputer portal design specs | `docs/superpowers/specs/*.md` | Design docs for making Inferno the OC Portal (OAuth authorization server, agent registry, billing contract) — replacing Nous Portal for the hermes-agent client. Docs only, no code. Upstream has no equivalent and never will. | Trivial — additive files, cannot conflict. |
| D5 | OAuth authorization server (org tenancy + ES256 signing key/JWKS + oauth_client registry + device authorization request) | `ent/schema/{org,org_member,oauth_client,oauth_device_authorization}.go` + `ent/` regeneration (44 files, incl. incidental `ent/group.go` comment-only drift picked up by the same `go generate ./ent` run, and Task 4's 7 new `ent/oauthdeviceauthorization*` files plus 9 further-modified shared codegen files) + `migrations/901_org_and_members.sql` + `migrations/904_org_personal_user_id.sql` + `migrations/902_oauth_client.sql` + `migrations/903_oauth_device_authorization.sql` + `service/{org_service,org_service_test}.go` + `service/{auth_service,wire}.go` + `cmd/server/wire_gen.go` + `cmd/jwtgen/main.go` + 16 test files threading the new `NewAuthService` `orgService` param + `service/{oauth_signing_key,oauth_signing_key_test}.go` + `service/{oauth_client_service,oauth_client_service_test}.go` + `service/{oauth_device_service,oauth_device_service_test}.go` + `handler/oauth_handler.go` + `handler/{handler,wire}.go` + `handler/dto/oauth.go` + `server/routes/{oauth,common}.go` + `server/router.go` + `service/{oauth_token_service,oauth_token_service_test}.go` + `service/refresh_token_cache.go` + `repository/refresh_token_cache.go` + `repository/refresh_token_cache_test.go` + `handler/oauth_handler_test.go` + `server/routes/oauth_token_route_test.go` + `server/middleware/{oauth_scope,oauth_scope_test}.go` + `server/routes/oauth_account_route_test.go` + `handler/oauth_device_decision_test.go` + `inferno-frontend/src/{api/oauth.ts,router/index.ts,views/oauth/DeviceApprovalView.vue,views/oauth/__tests__/DeviceApprovalView.spec.ts,i18n/locales/en/misc.ts,i18n/locales/zh/misc.ts}` + `migrations/905_hermes_cli_first_party_client.sql` + `scripts/oauth-conformance.md` | OAuth authorization server plan Task 1: minimal org tenancy so every later task can reference `org_id`. Every user gets an idempotent personal org on signup (email or OAuth), created best-effort — failure logs a warning and does not block signup, since `EnsurePersonalOrg` repairs on next login. Migration numbering: 900 already taken (D1), so this is 901, first of the fork's 9xx series; 902/903 are reserved by this same plan's later tasks (oauth_client, oauth_device_authorization), so the concurrency-safety follow-up below is 904. Ledger numbering: D4 already taken (design specs, committed same day), so this is D5. Org-role constants are named `OrgRoleOwner`/`OrgRoleAdmin`/`OrgRoleMember` (not `RoleOwner`/`RoleAdmin`/`RoleMember`) because `service.RoleAdmin` already exists as the platform-wide user role (`domain.RoleAdmin == "admin"`); the wire-format string values themselves (`"OWNER"`/`"ADMIN"`/`"MEMBER"`, read verbatim by the desktop client) are unchanged. `migrations/904_org_personal_user_id.sql` adds `orgs.personal_user_id` (nullable, unique where non-null) — a Task 1 review found `EnsurePersonalOrg`'s original read-then-create had no DB-level invariant, so two near-simultaneous calls for a new user (double-fired OAuth callback, two tabs) could each create a distinct personal org; `EnsurePersonalOrg` now looks up and inserts by `personal_user_id` and treats a unique-constraint violation on create as "lost the race, re-read the winner's row" rather than an error. Upstream has no tenancy concept. **Task 2:** ES256 signing keypair + public JWKS endpoint at `/.well-known/jwks.json` (server root, not `/api`), so agents/services can verify OAuth-issued tokens offline. The keypair is generated on first `Active()` call and persisted PEM-encoded in the existing `security_secrets` table under key `oauth_es256_active` — no schema change, no new migration. `Active()` handles the same concurrent-first-call race as `EnsurePersonalOrg`: a lost race on the `security_secrets.key` unique constraint (detected via `dbent.IsConstraintError`, not string matching, since prod is Postgres and tests are SQLite) re-reads and returns the winner's key rather than erroring or minting a second, orphaned key. JWKS projection never includes the private scalar `d`; `x`/`y` are left-padded to a fixed 32 bytes via `b64uint` since P-256 coordinates are fixed-width. OAuth handlers deliberately bypass `internal/pkg/response`'s `{code,message,data}` wrapper — external clients parse RFC-shaped JSON at the top level. **Task 3:** `oauth_client` registry (`oauth_clients` table) + `POST /api/oauth/self-hosted-client`, bearer-authenticated, mounted at the server root under `/api/oauth` (a new `RegisterOAuthAPIRoutes`, sibling to `RegisterOAuthWellKnown`) rather than under the versioned `/api/v1` panel API, because this surface is consumed by the hermes CLI / dashboard and shares its path prefix with later unauthenticated OAuth endpoints (device/code, token) in this same plan. `oauth_clients` are PUBLIC OAuth clients — no `client_secret` column; PKCE is the protection. `client_id` is always server-generated as `agent:{32 hex chars}`; callers never choose it, since a client that could pick its own id could impersonate another agent. `name` (docker-style `adjective_noun`) is deliberately NOT unique — the row id is the key, so collisions are harmless and never block registration. `instance_id` is nullable and unique (partial index, `WHERE instance_id IS NOT NULL`, mirroring `orgs_personal_user_id_key`) but nothing sets it yet — the column exists now so a later oc-platform sub-project can make VM provisioning idempotent on it without a further migration. The handler resolves the caller's org via `OrgService.OrgsForUser` and uses `orgs[0]` (every user has exactly one personal org per Task 1); an empty slice returns a clean 500 rather than indexing out of bounds. Logging is restricted to `client_id` (public by design) and `org_id` — never `user_id`, tokens, or secrets. Verified live: built the binary, ran it against a throwaway Postgres+Redis, logged in as the auto-setup admin, seeded that user a personal org directly (auto-setup's admin bypasses the normal signup path that calls `EnsurePersonalOrg`, so it has none by default), and confirmed `POST /api/oauth/self-hosted-client` with a bearer token returns a bare `{"client_id":"agent:...","name":"..."}` (not `{code,message,data}`-wrapped) while the same call with no `Authorization` header returns 401; two registrations with colliding-possible names both succeeded; server logs contained `client_id`/`org_id` only, never the bearer token or admin password. **Task 4 (this addition):** RFC 8628 device authorization request, `POST /api/oauth/device/code` — a new `oauth_device_authorizations` table plus `service.OAuthDeviceService.RequestCode`. Deliberately UNAUTHENTICATED (a new `RegisterOAuthDeviceRoutes`, a second `/api/oauth` group registered alongside the bearer-gated `RegisterOAuthAPIRoutes`, not merged into it): a headless CLI calling this endpoint has no session yet, `client_id` is the request's only identity, and it is validated against the Task 3 `oauth_client` registry (`OAuthDeviceService` composes an internal `*OAuthClientService` and calls its `ByClientID`, rather than re-querying the ent client directly, so client validation stays in one place). `user_code` is an 8-character, human-typed code drawn from `service.UserCodeAlphabet` (`ABCDEFGHJKMNPQRSTUVWXYZ23456789` — no `0/O`, no `1/I/L`); `device_code` is 32 bytes of `crypto/rand` hex-encoded, and its `Read` error is checked (an unchecked failure would silently degrade to a predictable code, letting an attacker redeem someone else's pending login). Both columns are schema-`Unique()`; a `device_code` collision cannot practically happen at 256 bits of entropy, but a `user_code` collision is handled by regenerating and retrying the insert (bounded at 5 attempts, detected via `dbent.IsConstraintError`) rather than surfacing a raw constraint-violation 500 to an innocent caller. `verification_uri` is built from `cfg.Server.FrontendURL` (`{frontend_url}/device`) and `verification_uri_complete` embeds the URL-escaped `user_code`; `FrontendURL` defaults to `""` for deployments that never set it, and rather than silently emitting an unusable relative URL, `RequestCode` refuses with `service.ErrPortalNotConfigured` (mapped to a 500 `server_error`, logged at Error level with no request-derived fields) when it is empty — a fail-loud choice over failing the whole server at startup, since not every deployment necessarily uses the device flow. `expires_in` is 900s (15 min), `interval` is 5s. Neither `device_code` (a bearer credential until redeemed) nor `user_code` (also unwise) is ever logged; handler error paths log `client_id` only, and the "unknown client_id" and "portal not configured" paths are logged at different levels (`Warn` vs `Error`) but return generically-worded bodies so a probing caller can't distinguish response shapes. `Approve`/`Deny` (consumed by Task 5, the approval endpoint) look up by `user_code`, normalize it (`TrimSpace`+`ToUpper`, so a human typing lowercase or with stray whitespace still matches), and reject with `ErrDeviceCodeNotFound`/`ErrDeviceCodeExpired` as unwrapped sentinels so handler code can `errors.Is` them. All OAuth handlers in this row continue to bypass `internal/pkg/response`'s wrapper — verified live (see report). **Task 5 (this addition):** `POST /api/oauth/token`, the device_code and refresh_token grants — the task that actually mints credentials, on the same unauthenticated `/api/oauth` group as `device/code` (the caller has no session, that is the point of a token endpoint). `service.OAuthTokenService.mintAccessToken` signs ES256 JWTs itself (`jwt.NewWithClaims(jwt.SigningMethodES256, claims)`, `Header["kid"] = key.Kid`, keyed via Task 2's `OAuthKeyService.Active`) and deliberately does NOT call `AuthService.GenerateTokenPair` — that mints Inferno's separate HMAC-signed panel-session token, which Task 6's resource-server middleware (ES256-only, by design) would silently reject two tasks later. Refresh tokens ARE persisted through the existing Redis-backed `RefreshTokenCache` (`internal/service/refresh_token_cache.go`) rather than a parallel store: `RefreshTokenData` gained three `omitempty` fields — `Scope`, `ClientID` (absent from the panel-session shape, needed here so a refresh can validate both), and `Rotated` (a tombstone marker, added beyond the two the plan anticipated). Rotation does NOT hard-delete the presented token on success; it re-stores the same hash with `Rotated=true` for the remainder of its original TTL. That tombstone is what makes reuse detection possible at all: a replay of that exact raw token is looked up successfully, sees `Rotated=true`, and calls `DeleteTokenFamily(data.FamilyID)` — which deletes every hash the Redis-backed cache ever added to that family set, including the token the legitimate client received from the rotation. A hard-delete-on-rotation design (which is what `AuthService.RefreshTokenPair`, the panel-session equivalent, actually does — `ErrRefreshTokenReused` is declared there but never returned; nothing in that path currently re-derives the family from a deleted hash) would have made this indistinguishable from "never existed" and downgraded reuse detection to a silent no-op; `oauth_token_service_test.go`'s `TestRefreshTokenReuseIsRejected` asserts both that the replay itself fails AND that the token from the legitimate rotation is also dead afterward, specifically to catch a single-token-delete regression. Device-code redemption is single-use via a status-guarded `UPDATE ... WHERE status='approved'` (`OAuthDeviceAuthorization.Update().Where(ID(row.ID), Status("approved")).SetStatus("expired"))`, not the plan's unconditional update, so two concurrent redemptions of one human approval can't both mint a token pair. The RFC 8628 §3.5 error strings (`authorization_pending`, `slow_down`, `access_denied`, `expired_token`) are unwrapped sentinel errors the hermes client branches on verbatim; `slow_down` fires by comparing `time.Since(*row.LastPolledAt)` against the poll `interval` and — critically — only advances `LastPolledAt` on a poll that is NOT itself rejected as `slow_down`, so a client can't reset the backoff by polling continuously. `refresh_token` grant failures (unknown token, wrong `client_id`, expired, and reuse) all collapse to RFC 6749 §5.2's single `invalid_grant` on the wire — distinguishing them would let a caller enumerate token/client existence — with the reuse case additionally logged server-side (`client_id` only, per the no-credential-logging rule that holds throughout this row). `issuer` is wired from `cfg.Server.FrontendURL`, the same public origin Task 2's JWKS is served from, via a new `ProvideOAuthTokenService` (`service/wire.go`) threaded through `cmd/server/wire_gen.go` by hand (no `wire` binary in this environment, consistent with how Tasks 3-4 updated it). No ent schema change was needed — Task 4 had already added `oauth_device_authorizations.last_polled_at` in anticipation of this task. Verified live: built the binary against a throwaway Postgres+Redis, registered a client, requested a device code, polled once (`authorization_pending`), polled again immediately (`slow_down`), approved the row directly in Postgres, polled again (200 with `access_token`/`refresh_token`/`scope`), decoded the access token header (`alg: ES256`, non-empty `kid`), redeemed the same `device_code` a second time (`expired_token`), rotated the refresh token once, then replayed the original refresh token and confirmed both the replay and the previously-rotated token now fail — see report for the full transcript. **Task 5 review fixes (this addition):** an independent review of Task 5 (run on a stronger model, since this is where credentials are actually minted) found `ExchangeRefreshToken`'s "tombstone" design sound but its Go-side implementation racy: reading `RefreshTokenData` then separately re-storing it as rotated was two Redis round trips, so two concurrent presentations of the SAME refresh token (an attacker replaying a stolen token at the same instant the legitimate client refreshes on schedule) could both observe `Rotated=false` and both mint a token pair — forking the family into two live branches that never again trip the reuse detector, since neither side ever re-presents an already-rotated token to the other. Fixed by adding `RefreshTokenCache.MarkRotated`, a single atomic GET-decide-SET Redis operation (`internal/repository/refresh_token_cache.go`'s `refreshTokenMarkRotatedScript`, a Lua `EVAL` using `cjson.decode` to check the `rotated` field and Redis 6's `SET ... KEEPTTL` to flip it while preserving the record's exact remaining TTL — no separate PTTL-read-then-SET-PX window to race on either); `ExchangeRefreshToken` now calls it *before* minting anything, so a mid-flight crash costs at most the one token being rotated, never a forked family. `TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins` (service layer, in-memory fake mirroring the same atomicity under its own mutex) and `TestRefreshTokenCache_MarkRotatedConcurrentOnlyOneWins` (repository layer, real Lua `EVAL` against miniredis) both fire N=20/50 goroutines at one token under `-race` and assert exactly one wins; mutation-tested by temporarily swapping `ExchangeRefreshToken`'s body back to the pre-fix read-then-write shape and confirming the SAME test then fails (typically all N presentations "succeed" — see task-5-report.md for the exact before/after runs). The review's second finding, `ExchangeRefreshToken` never re-validating the account, is also closed: `OAuthTokenService` now takes a minimal `OAuthUserLookup` (just `GetByID` — deliberately narrower than the full `UserRepository` so any real implementation satisfies it structurally, per Go's implicit interface satisfaction, with no adapter) and revokes the whole token family via `DeleteTokenFamily` if the user is missing or inactive, mirroring `AuthService.RefreshTokenPair`'s equivalent check — closing the gap where `UserService.UpdateStatus`/`Delete` invalidate an auth cache but never call `RevokeAllUserSessions`, so a banned user's OAuth refresh token would otherwise keep minting fresh access tokens for the rest of its 30-day life. Third: the `Token` handler previously folded every unmatched internal error (a Redis outage, a signing-key read failure) into an OAuth error code (`expired_token`/`invalid_grant`) with no server-side log line — indistinguishable from a real expired/invalid credential and silent to an operator; it now matches known sentinels explicitly (including `service.ErrDeviceCodeNotFound`/`ErrDeviceCodeExpired`, missed in the original mapping) and falls through to a logged `500 server_error`, mirroring the pattern `DeviceCode` already used. Fourth: `tokenResponse` now sets `Cache-Control: no-store` and `Pragma: no-cache` per RFC 6749 §5.1, since the body carries a bearer credential an intermediary must never cache. Fifth: `issueRefreshToken`'s `AddToUserTokenSet` failure is now fatal (was logged-and-ignored) — a token silently missing from the user set would survive a password-change/logout-all-devices revocation for the rest of its life. Sixth: `oauth_token_service_test.go`'s reuse test now asserts the exact sentinel (`errors.Is(err, ErrInvalidGrant)`) for the legitimately-rotated token's post-revocation state rather than "any error", so an unrelated failure can't masquerade as successful family revocation. New wire-contract test coverage closes the gap the review flagged (handler-level behavior was previously verified only by a curl transcript, nothing in `go test ./...`): `internal/handler/oauth_handler_test.go` table-tests every `Token` grant/error branch against a real `httptest` request, asserting the exact `{"error":"..."}` body with no `code`/`message`/`data` keys and the no-store headers on success; `internal/server/routes/oauth_token_route_test.go` asserts `POST /api/oauth/token` resolves with no `Authorization` header when only `RegisterOAuthDeviceRoutes` is registered, and (inversely) that `jwtAuth` never runs for it even when `RegisterOAuthAPIRoutes` is also registered on the same engine. Deliberately NOT fixed, per the review's explicit scoping: a narrower residual race between the winner's own `AddToFamilyTokenSet` call and a concurrent loser's `DeleteTokenFamily` call (only reachable when a token's legitimate rotation and its replay are presented at truly the same instant, not the realistic sequential-replay case this feature defends against — noted in task-5-report.md, not exercised by the final concurrency test's assertions); unbounded token-family-set growth; the device-code-consumed-before-minting ordering; and `AuthService.RefreshTokenPair`'s own dead `ErrRefreshTokenReused` (confirmed real, pre-existing, out of scope — the panel-session path is untouched by this row). **Task 6 (this addition):** the consumption side of Task 5's credentials — a new resource-server middleware, `middleware.RequireOAuthScope(keySvc *service.OAuthKeyService, required string) gin.HandlerFunc`, plus `GET /api/oauth/account`. The middleware parses with `jwt.NewParser(jwt.WithValidMethods([]string{"ES256"}))` and additionally asserts `token.Method.(*jwt.SigningMethodECDSA)` inside its keyfunc — two independent checks against algorithm confusion, mirroring `AuthService.ValidateToken`'s existing pattern for the HMAC panel-session side (`auth_service.go:1322`) — specifically so a token signed with Inferno's HMAC panel-session secret (held by every server process) can never be accepted here: that is the entire reason Task 2 introduced a separate ES256 keypair, and accepting both algorithms on this middleware would have quietly defeated it. `scopeSatisfies` is exact, whitespace-split equality (`strings.Fields` + `==`), not `strings.Contains` — a substring match would let a token carrying `billing:manage_nothing` or bare `billing` satisfy a `billing:manage` requirement, a privilege-escalation bug; `billing:manage` is deliberately never granted at initial device-flow login (the desktop re-runs the flow to elevate) and this middleware does not special-case it. `exp` enforcement is `golang-jwt/v5`'s default claims validation (no extra code needed) since `mintAccessToken` always sets it. The verified `sub` claim (a decimal string, per `strconv.FormatInt` in `mintAccessToken`) is parsed with `strconv.ParseInt` and the request rejected outright if it doesn't parse, rather than defaulting `oauth_user_id` to `0` — a zero-valued id reaching a handler would silently act on the wrong account instead of failing closed. Three context keys are set on success — `oauth_user_id` (`int64`), `oauth_client_id` (`string`, from `aud`), `oauth_scope` (`string`) — as exported string constants (`OAuthContextKeyUserID` etc.) in the middleware package rather than the brief's bare string literals, so the handler and the middleware can't drift on the key name independently. `OAuthHandler` gained one accessor, `KeyService() *service.OAuthKeyService`, so `routes.RegisterOAuthAccountRoutes` (a third, new `/api/oauth` route group — distinct from both the jwtAuth-gated `RegisterOAuthAPIRoutes` and the fully-unauthenticated `RegisterOAuthDeviceRoutes`, because an OAuth bearer is a third, different auth shape) can build the middleware without a new wire-provided dependency; no `wire.go`/`wire_gen.go` changes were needed since `OAuthKeyService` and `OAuthHandler` were already wired by Tasks 2 and 3. `GET /api/oauth/account` is registered with `required=""` (any validly-signed, unexpired OAuth token — it grants no elevated capability, it just resolves the caller's own org membership) and returns `{"user_id": "<decimal>", "orgs": [{"id","slug","name","isPersonal","role"}, ...]}`, reusing Task 1's `OrgService.OrgsForUser`/`RoleIn`; a per-org `RoleIn` failure logs and falls back to `service.OrgRoleMember` (this codebase's least-privileged org role) rather than ever inventing an elevated one — the brief's own draft used the nonexistent `service.RoleMember` (that identifier is the *platform* role constant; the org-scoped ones are `OrgRole{Owner,Admin,Member}`, renamed in Task 1 specifically to avoid this collision) and would not have compiled as intended. Test fixtures reuse this package's already-established `newPaymentConfigServiceTestClient`-equivalent pattern (the brief's named helper, `newTestEntClient`, does not exist; `internal/handler/oauth_handler_test.go`'s `newOAuthHandlerTestEntClient` is the same shape one package over and was mirrored, not reused directly, since it's unexported across package boundaries) — real ES256 signature verification against a throwaway sqlite-backed `OAuthKeyService`, not a stub. Beyond the brief's two smoke tests (`nil` keySvc, missing bearer / `alg:none`), `oauth_scope_test.go` adds: valid ES256 token + sufficient scope → 200 with correct context values; valid token + insufficient scope → 403 `insufficient_scope`; no `scope` claim against a non-empty requirement → 403; an HMAC-signed token with otherwise-fully-valid claims (correct `sub`/`aud`/`scope`, future `exp`) → 401 `invalid_token` (the security property that matters most here); an expired ES256 token → 401; an unparsable `sub` → 401. `oauth_handler_test.go` adds the `Account` endpoint's own wire-contract tests reusing `requireBareErrorBody` per this row's standing rule against `internal/pkg/response`: missing bearer, garbage token, and a full round trip (real `EnsurePersonalOrg` row, real minted token, asserts the exact 2-key top-level body and the nested org fields, no `code`/`message`/`data`). Verified live: full `docker-compose`-free flow against a throwaway Postgres+Redis — registered a client, ran the device flow to approval, exchanged for a real access token, then `curl -H "Authorization: Bearer <token>" /api/oauth/account` returned the bare account JSON; the same call with no header and with a garbage token both returned bare `401 {"error":"invalid_token"}`; server logs contained no token material — see task-6-report.md for the full transcript. **Task 7 (this addition):** the device approval screen — the only human-facing step of the whole flow. Go half: `OAuthHandler.ApproveDevice`/`DenyDevice`, `POST /api/oauth/device/approve` and `/device/deny`, registered on the existing jwtAuth-gated `RegisterOAuthAPIRoutes` group (a logged-in human approves in a browser, not a bearer-token agent). These two are a DELIBERATE, commented exception to this row's own bare-JSON rule: every other OAuth handler in this file bypasses `internal/pkg/response` because the hermes client parses RFC-shaped JSON at the top level, but these two are panel-only — never called by hermes — and inferno-frontend's `api/client.ts` response interceptor reads `apiData.code`/`apiData.message` to build the error it shows the user, so a bare `{"error":"..."}` body would surface as a generic, non-actionable axios error on every failure. `ErrDeviceCodeNotFound`/`ErrDeviceCodeExpired` map to `response.NotFound` (404) and `response.Error(c, http.StatusGone, ...)` (410); `user_code`/`device_code` are never logged, same rule as Tasks 4-6. New `internal/handler/oauth_device_decision_test.go` (kept separate from `oauth_handler_test.go`, whose own header comment asserts the opposite, bare-body contract for `/token`) covers missing auth, missing/unknown/expired `user_code`, and a real approve/deny round trip against a seeded `oauth_device_authorizations` row, asserting the envelope shape and the persisted `status`/`approved_user_id`. Vue half, entirely new to this plan (Tasks 1-6 never touched `inferno-frontend/`): `views/oauth/DeviceApprovalView.vue` at route `/device` (exact path required — `OAuthDeviceService.RequestCode` hardcodes `verification_uri` as `{frontend_url}/device`), `requiresAuth: true` so an unauthenticated visitor is bounced to `/login?redirect=/device?user_code=...` and returned with the query intact by the router's existing `redirect` handling (no new code needed there beyond the route entry itself). The `user_code` input is prefilled from `route.query.user_code`, live-uppercased and re-hyphenated at `XXXX-XXXX` on every keystroke (tolerating a pasted lowercase or hyphen-less code) via a computed `v-model` setter; the backend independently trims/uppercases too. Three outcomes past the form: approve/deny (200) are terminal, rendered through the existing `InterstitialState` component (archetype F, already shared by 9 OAuth-callback-shaped routes); an expired code (410) is ALSO terminal, its own screen telling the human to re-run the CLI command, since retyping cannot fix it; an unknown code (404) is deliberately NOT terminal — it stays on the form as an inline `Input` error, since it usually means a mistyped character and the person should be able to fix and resubmit immediately, the same shape as a wrong password on the login screen. New `api/oauth.ts` calls through the shared `apiClient` (so the auth token, `Accept-Language`, and the response-envelope/error interceptor all still apply) but targets `buildGatewayUrl('/api/oauth/device/...')` rather than a bare relative path, since `/api/oauth` lives outside `apiClient`'s `/api/v1` `baseURL` and a relative path would double-prefix. A new top-level `device` i18n namespace was added to `misc.ts` (en + zh) rather than editing `router/index.ts`/`i18n/` files under a swarm-conversion agent's "don't touch" convention — that convention (`inferno-frontend/CONVENTIONS.md`) governs the parallel June-redesign workflow for existing views, not a new backend-plan feature with its own brief explicitly assigning those files. `inferno-frontend/scripts/check-divergence.sh`'s `CHANGED` computation is scoped to `backend frontend deploy docs` from the repo root (`frontend/` there is the pristine upstream mirror, not `inferno-frontend/`), so none of the `inferno-frontend/` files above are in that gate's enforcement scope or its `DECLARED` list — they are listed here in the files column for the record, and in `DECLARED` only the one new backend file (`oauth_device_decision_test.go`) actually falls inside the gate's diff scope. Verified live: threw away Postgres+Redis (`t7-pg`/`t7-redis`, distinct from any other worktree's containers), built the binary, ran `npx vite` against it with `VITE_DEV_PROXY_TARGET` pointed at the throwaway backend, registered a client, requested a device code (`verification_uri_complete` resolved to `http://127.0.0.1:15173/device?user_code=...`), and drove the actual browser through the real UI: unauthenticated visit redirected to `/login?redirect=/device?user_code=...` and landed back on `/device` with the code prefilled after login; clicked Approve and confirmed both the UI's "Device approved" screen and `oauth_device_authorizations.status='approved'`/`approved_user_id` in Postgres; redeemed the same `device_code` at `/api/oauth/token` for a real ES256 access token, closing the loop end to end through the browser rather than a direct-SQL stand-in (Tasks 4-6's necessary workaround before this screen existed). Also verified live: a wrong code stays on the form with an inline error, an expired code shows the terminal "run the command again" screen, and Deny shows its own terminal screen — see task-7-report.md for the full transcript and screenshots. **Task 8 (this addition, final task of this plan):** end-to-end conformance — drove the REAL hermes Python CLI (`hermes_cli/auth.py`, a separate read-only repo, `/Users/saksham/OpenComputerV2/OpenComputerV2` — never edited, never committed to) against this server. Found two plan defects by inspecting the real client, both fixed here rather than in the client (the client is the specification): (1) the real CLI hardcodes `client_id="hermes-cli"` (`DEFAULT_NOUS_CLIENT_ID`, `auth.py:77`) — a fixed, well-known, first-party public client, not an `agent:{id}` Task 3's `RegisterSelfHosted` can mint, so `POST /api/oauth/device/code` 400s `invalid_client` on the very first call without a matching `oauth_clients` row. Fixed by `migrations/905_hermes_cli_first_party_client.sql`, which seeds `client_id='hermes-cli', kind='FIRST_PARTY', status='active', owner_user_id=0, org_id=0, redirect_uri_origin='urn:ietf:wg:oauth:2.0:oob'`. `kind='FIRST_PARTY'` is a new third value — neither `SELF_HOSTED` nor `HOSTED` fits, since both existing values denote *a gateway instance* (the schema's own doc comment) and hermes-cli is the CLI binary itself, one client_id shared by every install; introducing the value is safe because `kind` has no CHECK constraint (`902_oauth_client.sql`) and nothing in Go switches on it (verified by grep — only ent-generated boilerplate touches the field). `owner_user_id`/`org_id` are `0` as an explicit "no owner" sentinel, not a fabricated user: both columns are `NOT NULL` with no `FK` (so no schema change was forced), and `users`/`orgs` are `BIGSERIAL PRIMARY KEY` starting at 1, so `0` can never collide with a real row — see the migration's own header comment for the full reasoning the brief asked for. (2) The real CLI sends and client-side-asserts `scope=inference:invoke` (`NOUS_INFERENCE_INVOKE_SCOPE`, `auth.py:78-80`; the assertion is `_nous_invoke_jwt_status`, exact-equality, mirroring this repo's own `scopeSatisfies`), but the design doc's scope table and several tests named it `inference`. `RequestCode`/`mintAccessToken` are scope-pass-through (they echo back whatever the client sent — verified by reading `oauth_device_service.go`/`oauth_token_service.go`; no code path hardcodes a default), so this did not actually block the wire protocol in this run, but was corrected everywhere the vocabulary is defined or asserted anyway, per the brief: the design doc's scope table (`docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md`, with a note explaining the correction) and the scope literals in `oauth_handler_test.go`, `oauth_device_service_test.go`, `oauth_token_service_test.go`, `oauth_scope_test.go`, `refresh_token_cache_test.go` — so a future scope-enforcement feature (e.g. gating `/v1/*` on `inference:invoke`) is built against the vocabulary the real client actually uses, not a name nobody sends. Conformance itself: built the binary against throwaway Postgres+Redis (`t8-pg`/`t8-redis`), confirmed the seeded `hermes-cli` row and the RFC 8628 device-code response (all six required fields, unwrapped). Rather than the full interactive `oc setup` wizard (which exits immediately under this environment's non-TTY stdin, before touching the network — `hermes_cli/setup.py`'s own guard) or automating an unrelated curated-model-picker menu behind a forced pty, drove the real, unedited `hermes_cli.auth._nous_device_code_login` (plus the same `_save_provider_state`/`_save_auth_store` persistence `_login_nous` uses) directly from a small driver script — 100% real client code making real HTTP requests, scoped to exactly the OAuth mechanics under test; documented as a deliberate deviation in the runbook and report. `HERMES_HOME` was set to a scratch directory for the whole run, never the real `~/.hermes`. Approved via the API directly (bearer session from the seeded auto-setup admin, itself seeded a personal org first, same as Tasks 3/5/6/7) rather than a real browser, since Task 7 already verified the `/device` screen visually with screenshots — this run's focus was the CLI side. Verified: the driver's printed instructions never raised `Device code response missing fields`; the poll loop returned and wrote `auth.json` to the scratch `HERMES_HOME`; the persisted access token's header decodes to `alg: ES256` with a non-empty `kid`; the token's signature verifies against `GET /.well-known/jwks.json` (PyJWT, real signature check, not a shape check) with claims `aud: hermes-cli`, `scope: inference:invoke`, `sub` matching the approving user; a second, independent device authorization polled twice within one second returned `authorization_pending` then `slow_down`. Full transcript (redacted — no raw device_code/refresh_token/access_token, JWT headers only) in `task-8-report.md`; the reproducible runbook is `scripts/oauth-conformance.md`. Gate: `go test ./...` all green, `golangci-lint run --new-from-rev=36ea7ab8 ./...` 0 issues. | **Do not hand-merge the `ent/` files.** Re-run `go generate ./ent` and let codegen rebuild from the four schema files; then re-run `cd backend/cmd/server && go generate ./...` to regenerate `wire_gen.go` from `wire.go`. Only `ent/mutation.go` and `ent/runtime/runtime.go` realistically conflict, for the same reason as D1. Tasks 2, 3, and 4's non-ent files are ordinary Go source and merge normally. |

**Better mechanism for D2 when someone has time:** seed the four titles as a
`login_agreement_documents` settings row instead. Admin-editable, and it touches
no Go at all, which retires this ledger entry.

**Before adding an entry:** prefer frontend-only, then an existing backend field,
then upstreaming it as a PR to `Wei-Shaw/sub2api`, and only then a new entry
here. Each entry is a permanent tax on every future reconcile. `avatar_seed` was
checked against riding on the existing `avatar_url` field first — it cannot,
because `SetAvatar` runs `normalizeUserAvatarInput`, which rejects a bare seed.

### Known-good exceptions, do not "fix" these
- `npm run lint:check` reports one pre-existing eslint error in
  `scripts/june-lint.mjs` (`no-misleading-character-class`, the emoji range).
  It reproduces on a clean tree. Leave it or fix it deliberately, but it is not
  a regression you caused.
- `/admin/audit-logs` has one nested scrollbar: `DataTable`'s `.table-wrapper`
  inside `TablePageLayout`, which scrolls internally **by design**.

---

## Verification that is not a gate, but is required

Type-checking proves nothing about whether a screen looks right. For any visual
change:

1. **Render it.** Dev server on `:5173`, log in, navigate to the actual route.
2. **Measure, do not eyeball.** `getBoundingClientRect`, `getComputedStyle`,
   `scrollHeight` vs `clientHeight`. Numbers in the commit message.
3. **Both themes.** `localStorage.setItem('theme','dark')` then reload.
4. **Two widths minimum**, one of them below 1024 where the rail goes off-canvas.
5. **Seed first if the screen is empty.** A component cannot be verified against
   no data — see "Seeds" below.

### The shell audit — rerun after any layout change
Walks every in-shell route asserting: document must not scroll · card frame must
not drift while scrolled · outer gaps 7/7/7 · no reserved scrollbar gutter · no
nested scrollbar · no new console errors. Last run: **48/49 clean**, the one
being the by-design table-wrapper above. The script is in the session log for
`384cd7d0`; rebuild it from that commit message if needed.

---

## BACKLOG — ordered. Do them in this order.

### 1. `TablePageLayout` height coupling — SMALL, DO FIRST
`src/components/layout/TablePageLayout.vue` hardcodes
`height: calc(100vh - 46px / 62px / 78px)` at three breakpoints. Its own comment
explains why: *"shell__card is a plain block … so this element cannot inherit a
height through it"*. **That stopped being true in `31a75e07`** — the shell is a
flex chain pinned to `100dvh` now.

- Replace the three `calc()` rules with `height: 100%` (or `flex: 1; min-height: 0`).
- **Done when:** the three `calc(100vh` occurrences are gone AND all 15 table
  views still render with `.tpl` height == the card's client height, measured.
- **Risk:** touches every admin table page. Measure `.tpl` on at least
  `/admin/users`, `/admin/accounts`, `/admin/audit-logs` before and after.

### 2. Finish ops — the modal-only surfaces — ✅ COMPLETE — every mounted file under `src/views/admin/ops/` is converted.

| file | legacy utils | commit |
|------|--------------|--------|
| `OpsSettingsDialog.vue` | 93 | `4e716ec0` |
| `OpsErrorDetailModal.vue` | 95 | `2bff989f` |
| `OpsRequestDetailsModal.vue` | 87 | `755828bc` |
| `OpsAlertRulesCard.vue` | 86 | `6fcea767` |
| `OpsErrorLogTable.vue` | 66 | `263ff0b5` |
| `OpsOpenAITokenStatsCard.vue` | 50 | `11d9fc7f` |
| `OpsErrorDetailsModal.vue` | 21 | `11d9fc7f` |
| `OpsDashboard.vue` | 6 | `11d9fc7f` |

`conversion-status.mjs` now reports 129/324 converted files; the two dead ops
cards below have been removed rather than counted as converted. Every mounted
ops surface listed above was opened in a browser against seeded data and
measured, not just typechecked.

**To see the OpenAI token stats card at all:** it is hidden by default. Ops
page → Settings → Advanced settings → "Display OpenAI token request stats".
The flag lives in `getAdvancedSettings`, not in the `settings` table, so there
is no row to flip directly.

#### ✅ Two dead ops cards removed; controls consolidated

`OpsRuntimeSettingsCard.vue` (118 utils, 537L) and
`OpsEmailNotificationCard.vue` (121 utils, 442L) have **zero references
anywhere in `src/`** — no import, no `defineAsyncComponent`, no `<component
:is>`. They are also unmounted in `upstream/main`; they arrived dead in
`d464c0f0` when the upstream frontend was vendored wholesale.

They are superseded by `OpsSettingsDialog.vue`, which calls the same two API
pairs (`get/updateAlertRuntimeSettings`, `get/updateEmailNotificationConfig`)
and renders the same fields.

**Resolution:** the alert-silencing and distributed-lock controls from the dead
runtime card are now editable in `OpsSettingsDialog.vue`, and both dead cards
are deleted. The dialog still sends the complete runtime object to the existing
single PUT endpoint, so fields outside the form continue to round-trip intact.

Silencing is validated before save using the existing RFC3339, positive rule ID,
`P0..P3` severity, and lock-key/TTL rules. Focused coverage lives in
`OpsSettingsDialog.spec.ts`.

### 3. `SettingsView` — the elephant, 12,923 lines / 1,742 utilities
### ⚠️ DEFERRED — needs an owner decision before it can start. Item 4 went first.

**Correction:** this file previously said "seven routes". `INFERNO-BUILD.md`
line 1434 says **nine**. That line is the source of truth.

**Why it is blocked and not merely large.** The same line states: *"part 14
wants a redirect from the old URL and a release note, since admin bookmarks
break."* Splitting `/admin/settings` into nine routes is a user-facing
navigation change that invalidates every bookmark and deep link an operator
has. That is a product decision, not a styling one, and it is not something to
land silently overnight.

Owner picks one:
1. **Nine real routes** (`/admin/settings/general`, `/security`, …) with a
   redirect from `/admin/settings` to the first tab, plus a release note.
   Matches archetype C. Bookmarks to the bare URL survive via the redirect;
   nothing else does, because nothing else exists today.
2. **Nine components, one route**, selected by a tab or a `?tab=` query. Zero
   URL breakage, the 12,923-line file still becomes nine reviewable files, and
   the review problem is solved. Loses the clean per-section URL.
3. **Restyle in place** without splitting. Cheapest, but a 12,923-line SFC
   cannot be meaningfully reviewed, which is the reason the split was proposed.

Recommendation: **2**. It captures the entire reviewability win, which is the
actual blocker, at zero cost to anyone's bookmarks, and 1 stays available later
as a pure routing change once the components already exist.

Once unblocked, either way:
- Split **before** restyling. One section per commit, all six gates each.
- It is on the june-lint `TOUCHED_NOT_CONVERTED` waiver — **remove that entry**
  when the last section lands, and confirm lint still passes with it gone.

### 4. Remaining Chart.js — ✅ COMPLETE

Commit `6db5fef1` removes the final runtime Chart.js consumers:

- `DailyRevenueChart.vue` uses independent Dither strips per currency and for
  order count; it was completed in `b594a5dd` with payment seed data.
- `MonitorTrendChart.vue` uses three Dither strips while preserving zoom,
  localization, gap handling, and the existing metric semantics.
- The live admin `AccountStatsModal.vue` usage trend uses Dither strips. The
  duplicate unmounted `components/account/AccountStatsModal.vue` was reconciled
  and removed in `b594a5dd` before conversion.
- `chart.js` and `vue-chartjs` are removed from `package.json` and the lockfile.

The remaining textual references are migration comments/tests only; no runtime
imports remain. The account modal stays on the `TOUCHED_NOT_CONVERTED` waiver
because its other legacy sections still need the later account-modal pass.

### 5. The rest, by weight
`node scripts/conversion-status.mjs --top 30`. Largest first, except that the
three account modals (`CreateAccountModal` 965, `EditAccountModal` 620,
`BulkEditAccountModal` 293) are one job, not three — they share structure and
are all on the waiver list.

---

## Seeds — a screen with no data cannot be verified

All seeds are `now()`-relative, so re-running re-anchors them. The DB clock
drifts ahead of the seed; if a page reads Idle or "No data", reseed before
concluding anything is broken.

```bash
cd inferno-frontend/scripts/seed
for f in seed-dashboard seed-year seed-models seed-ops seed-ops-dense seed-alert-events seed-users; do
  docker cp $f.sql sub2api-postgres:/tmp/
  docker exec sub2api-postgres psql -U sub2api -d sub2api -q -f /tmp/$f.sql
done
```

See `inferno-frontend/scripts/seed/README.md` for what each one covers.

Seeded so far: a year of dashboard daily/hourly, per-model usage, dense ops
traffic + error logs + hourly metrics, 24 alert events across the full P0–P3
ramp and all three statuses, and 11 users with 30 days of usage.

Local dev login: `admin@sub2api.local` / `36232e0cd5be929e4004e4ca025b100e`.

---

## Rules that are not negotiable

1. **Gate 5.** The backend is read-only. If `upstream/main..HEAD` shows anything
   under `backend/`, `frontend/`, `deploy/` or `docs/`, revert it.
2. **Never weaken a test to make it green.** If a spec fails, decide whether the
   test or the code is wrong and fix that one. `INFERNO-BUILD.md` has a worked
   example of this going right.
3. **Never delete a row, column or control to make a layout tidy.** Reducing
   scope is the owner's call. If something must go, say so explicitly.
4. **Measure before claiming.** "Looks fine" is not evidence. Every completion
   claim in the log so far carries numbers; keep that.
5. **A BEM block may not be named after a bare Tailwind utility** while Tailwind
   is still in the build. `june-lint` enforces this now — it was found the hard
   way when `.ring` inherited Tailwind's blue focus ring for three commits.

---

## Status log — append one line per landed commit

| date | commit | what | files converted |
|------|--------|------|-----------------|
| 2026-08-13 | `0ae68c16` | ops skeleton matched to the real page | 119 |
| 2026-08-13 | `31a75e07` | shell: card scrolls, not the document | 119 |
| 2026-08-13 | `384cd7d0` | card scrollbar hidden, padding squared | 119 |
| 2026-08-13 | `35aab418` | dashboard user-trend chart off Chart.js | 118 |
| 2026-08-13 | `caeb742f` | TablePageLayout derives its height (item 1) | 118 |
| 2026-08-13 | `4e716ec0` | ops settings dialog + NumberField primitive | 120 |
| 2026-08-13 | `2bff989f` | ops error detail modal; 12 cards to a `<dl>` | 121 |
| 2026-08-13 | `755828bc` | ops request details; colour off the default state | 122 |
| 2026-08-13 | `6fcea767` | ops alert rules; list stops showing enum names | 123 |
| 2026-08-13 | `263ff0b5` | ops error log table; 6 hues for a category to 0 | 124 |
| 2026-08-13 | `11d9fc7f` | last 3 ops surfaces — **item 2 complete** | 127 |
| 2026-08-13 | `b594a5dd` | payment revenue chart off Chart.js; duplicate account modal reconciled | 127 |
| 2026-08-13 | `6db5fef1` | remaining Chart.js consumers removed; item 4 complete | 129 |
| 2026-08-15 | `bdba323b` | landed PR #3's upstream reconcile by cherry-pick; sync PR queue emptied | 208 |
| 2026-08-15 | `8bc560dc` | restored the site logo + its sanitiser, dropped by the sidebar rewrite | 208 |
| 2026-08-15 | `3e6888ab` | confirm-dialog helper retargeted at the Button primitive | 208 |

### Open, needs an owner decision — read before the next sync run

1. **Gate 5 is breached locally.** `84a3c4ac` put 19 files under `backend/`: an
   `avatar_seed` user field with full ent regeneration, migration
   `161_add_user_avatar_seed.sql` (a duplicate of the existing 161), and CN→EN
   default legal-doc titles in `setting_public.go`. Not pushed, which is the
   only reason the reconcile routine has not tripped. Rule 1 says revert; the
   uncommitted profile-avatar work depends on the field, so the two decisions
   are one decision.
2. **`inferno-redesign` on the remote is 61 commits behind local.** Every
   reconcile the routine has produced was computed against that stale base,
   which is why all three PRs read as massively conflicting. Pushing fixes the
   routine's view and trips Gate 5 in the same move — see 1.
3. **Local is 124 commits behind `upstream/main`.** Gate 5 only means what it
   claims *after* a rebase; against a stale ref it under-reports (19 files),
   against a fresh one it over-reports (246, of which 227 are just upstream
   moving on). Run it post-rebase or not at all.
4. **june-lint: 845 violations across 273 files**, none from the commits above.
   The denominator grew when the restore commits pulled ~140 half-migrated
   files into scope; a file counts as converted the moment it has a scoped
   `<style>` block. `dead-teal-palette` 476 and `ground-rule-3-two-weights` 271
   are the bulk, concentrated in `UsageTable.vue` / `UsageFilters.vue`.
5. **`AppSidebar.vue:114`** binds `user.avatar_url` to `:src` through a bare
   `.trim()` with no sanitiser — same hole as the site logo, different field.
6. Two gaps PR #3 flagged and left open: `backup.ts` multi-part downloads will
   404 until `BackupView.vue` converts, and `accountUsageRefresh.ts` is ported
   but unwired, so Grok quota cells lack the refresh OpenAI/Codex cells have.
