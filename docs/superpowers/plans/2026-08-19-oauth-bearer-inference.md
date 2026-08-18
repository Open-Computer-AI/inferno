# OAuth-bearer inference on `/v1` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Make Inferno's `/v1` accept the OAuth access token Inferno itself issues, so a hermes agent's inference is authenticated and metered against the user whose token paid for it.

**Architecture:** An OAuth credential branch on the `/v1` chain resolves a verified access token to a **backing `api_keys` row** owned by the token's subject, scoped per `client_id`, created on first use. It then populates exactly the context the existing gateway pipeline already reads. No downstream code changes.

**Tech Stack:** Go 1.26, Gin, ent/Postgres, Redis, `golang-jwt/jwt/v5`.

**Spec:** `docs/superpowers/specs/2026-08-19-oauth-bearer-inference-design.md`
**Evidence:** `.superpowers/sdd/2026-08-18-authorization-code-pkce/v1-oauth-gap-evidence.md`

## Global Constraints

- **RS256 only, `kid`-dispatched.** No second verifier: every task reuses the one extracted in Task 1.
- **The API-key header size cap stays at 256 bytes.** `maxAPIKeyAuthorizationHeaderBytes = MaxAPIKeyCredentialBytes + 128`. The OAuth branch runs *ahead* of it (F-A).
- **A backing row is never hard-deleted.** `usage_logs_api_key_id_fkey` is `ON DELETE CASCADE`; deleting one destroys that agent's usage history (F-C).
- **The backing row's secret is never returned by any endpoint, ever.** Not listable, not in any response body, not derivable.
- **Infrastructure faults surface as 500, never as an auth error.** An outage must not be indistinguishable from a bad credential.
- **API-key auth keeps working on every route it works on today.** Standalone Inferno customers depend on it.
- A JWT that fails verification is **rejected**, never retried as an API key.
- Gates for every task: `cd backend && go build ./... && go vet ./... && go vet -tags unit ./... && go test ./... && go test -tags unit ./...`. The tagged runs are mandatory — 398 of 1132 test files carry `//go:build unit`, so the untagged run is not the suite. Also `bash inferno-frontend/scripts/check-divergence.sh`; declare touched files in BOTH its `DECLARED` list and `GOAL.md`. Never put backticks or `$(` inside `DECLARED` — it is a double-quoted shell string and they execute.
- Every fix needs a test that fails without it, demonstrated by a **compiling** mutation. A mutation that breaks the build proves nothing.

---

### Task 1: Extract the OAuth bearer verifier

**Files:**
- Modify: `backend/internal/server/middleware/oauth_scope.go`
- Test: `backend/internal/server/middleware/oauth_scope_test.go`

**Interfaces:**
- Produces:
  ```go
  type OAuthBearer struct {
      UserID   int64
      ClientID string   // the bare client_id from `aud`
      Scope    string   // whitespace-delimited granted scopes
  }

  // ErrOAuthInvalidToken — the credential is bad. 401 invalid_token.
  // ErrOAuthServerFault — we could not check. 500 server_error.
  var ErrOAuthInvalidToken = errors.New("oauth: invalid token")
  var ErrOAuthServerFault  = errors.New("oauth: cannot verify token")

  func VerifyOAuthBearer(
      ctx context.Context,
      keySvc *service.OAuthKeyService,
      clientSvc *service.OAuthClientService,
      issuer string,
      rawToken string,
  ) (*OAuthBearer, error)
  ```
- Consumes: nothing new.

`RequireOAuthScope` keeps its exact current signature and behaviour, becoming a thin wrapper: extract bearer → `VerifyOAuthBearer` → `scopeSatisfies` → set the three `OAuthContextKey*` values.

**This is a pure refactor.** Every property currently in `RequireOAuthScope` moves into `VerifyOAuthBearer` unchanged: `WithValidMethods(["RS256"])`, `WithExpirationRequired()`, `WithIssuer(issuer)`, the `kid`-dispatch `keyFunc`, the `keyResolutionErr` split that distinguishes an unknown `kid` from an unreachable key store, `sub` parsed with `strconv.ParseInt` and rejected outright if it does not parse, `aud` required as a **single non-empty string** (never an array), and `clientSvc.UsableByClientID` with its `ErrClientNotUsable`/`IsNotFound` vs infrastructure-fault split.

- [ ] **Step 1: Confirm the existing tests pass unchanged**

Run: `cd backend && go test -tags unit ./internal/server/middleware/ -run TestRequireOAuthScope -v`
Expected: PASS. Record the test names — they are the refactor's safety net.

- [ ] **Step 2: Extract `VerifyOAuthBearer`, keep `RequireOAuthScope` as a wrapper**

Move the verification body verbatim. `RequireOAuthScope` maps the two sentinels to its existing responses: `ErrOAuthInvalidToken` → `401 {"error":"invalid_token"}`, `ErrOAuthServerFault` → `500 {"error":"server_error"}`, and logs the fault exactly as it does today (never the credential; only `kid` and `client_id`).

- [ ] **Step 3: Re-run the existing tests — they must still pass with zero edits**

Run: `cd backend && go test -tags unit ./internal/server/middleware/ -run TestRequireOAuthScope -v`
Expected: PASS, unmodified. If any test needed editing, the refactor changed behaviour — stop and report.

- [ ] **Step 4: Add direct tests for `VerifyOAuthBearer`**

Cover, asserting the returned sentinel: valid token → `OAuthBearer` with the right `UserID`/`ClientID`/`Scope`; `alg: none` and HS256 → `ErrOAuthInvalidToken`; missing `kid` → `ErrOAuthInvalidToken`; unknown `kid` → `ErrOAuthInvalidToken`; **key store unreachable → `ErrOAuthServerFault`**; missing `exp` → `ErrOAuthInvalidToken`; wrong `iss` → `ErrOAuthInvalidToken`; `aud` as an array → `ErrOAuthInvalidToken`; `sub` non-numeric → `ErrOAuthInvalidToken`; client revoked → `ErrOAuthInvalidToken`; **client lookup DB fault → `ErrOAuthServerFault`**.

- [ ] **Step 5: Mutation-prove the two fault splits**

Fold `ErrOAuthServerFault` into `ErrOAuthInvalidToken` for the key-store case (compiling change), run the tests, paste the failure. Restore. Repeat for the client-lookup case. These two splits are the ones that keep an outage distinguishable from a credential failure.

- [ ] **Step 6: Gates and commit**

---

### Task 2: The backing-row column and its uniqueness

**Files:**
- Modify: `backend/ent/schema/api_key.go`
- Create: `backend/migrations/909_api_key_oauth_client_id.sql`
- Regenerate: `backend/ent/` (`go generate ./ent`)
- Test: `backend/internal/repository/api_key_repo_test.go` (or the nearest existing repo test file)

**Interfaces:**
- Produces: `api_keys.oauth_client_id` — nullable string. `NULL` for every ordinary key; the OAuth `client_id` for a backing row.
- Consumes: nothing.

- [ ] **Step 1: Add the field to the ent schema**

```go
field.String("oauth_client_id").
    Optional().
    Nillable().
    Comment("OAuth client_id this key backs. NULL for ordinary user-created keys. " +
        "Non-NULL marks an internal backing row: never listed, never returned, never hard-deleted."),
```

- [ ] **Step 2: Write the migration**

```sql
-- 909: api_keys.oauth_client_id — the backing row for OAuth-bearer inference.
--
-- NULL for every ordinary key. Non-NULL means this row exists only so an
-- OAuth access token has something to meter against (see spec 2026-08-19,
-- finding F-C: usage_logs.api_key_id is NOT NULL with an FK, and the quota
-- ledger IS the key row).
--
-- The partial unique index is the identity rule: at most ONE backing row per
-- (user, client). It is partial so ordinary keys, which all carry NULL, are
-- unaffected — a plain UNIQUE would collapse every user's keys to one.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS oauth_client_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL;
```

- [ ] **Step 3: Regenerate ent and build**

Run: `cd backend && go generate ./ent && go build ./...`
Expected: clean. Declare every regenerated file in `check-divergence.sh` and `GOAL.md`.

- [ ] **Step 4: Test the partial-unique constraint**

Two backing rows for the same `(user_id, oauth_client_id)` → constraint violation. Two backing rows for the same user with **different** `oauth_client_id` → both insert. Two ordinary keys (`NULL`) for the same user → both insert, proving the index is genuinely partial.

- [ ] **Step 5: Mutation-prove the partiality**

Drop the `WHERE oauth_client_id IS NOT NULL` clause in a scratch copy of the migration, apply, and show the two-ordinary-keys test failing. This is the assertion that stops the index from breaking every existing user.

- [ ] **Step 6: Gates and commit**

---

### Task 3: The backing-key service

**Files:**
- Create: `backend/internal/service/oauth_backing_key.go`
- Create: `backend/internal/service/oauth_backing_key_test.go`
- Modify: `backend/internal/service/wire.go`, `backend/cmd/server/wire.go` (+ `wire_gen.go` via `go run github.com/google/wire/cmd/wire ./cmd/server`)
- Modify: `backend/internal/config/config.go` (group policy setting)

**Interfaces:**
- Consumes: `OAuthBearer` (Task 1), `api_keys.oauth_client_id` (Task 2).
- Produces:
  ```go
  type OAuthBackingKeyService struct{ /* ent client, config */ }

  // ErrNoGroupForOAuthKey — no group could be resolved. 403 with an operator-
  // readable message; never a nil-group panic downstream.
  var ErrNoGroupForOAuthKey = errors.New("oauth backing key: no group configured")

  // Resolve returns the backing api_keys row for (userID, clientID),
  // creating it on first use. The returned row has User and Group loaded,
  // because the gateway pipeline reads both.
  func (s *OAuthBackingKeyService) Resolve(ctx context.Context, userID int64, clientID string) (*dbent.APIKey, error)
  ```

- [ ] **Step 1: Write the failing test for get-or-create**

```go
func TestResolveCreatesOnFirstUseAndReusesAfter(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()

	first, err := svc.Resolve(ctx, 7, "agent:aaa")
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.NotNil(t, first.User)
	require.NotNil(t, first.Group, "the gateway pipeline reads apiKey.Group for routing")

	second, err := svc.Resolve(ctx, 7, "agent:aaa")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "the same agent must reuse its row, not accumulate rows")

	other, err := svc.Resolve(ctx, 7, "agent:bbb")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID, "per-agent attribution requires one row per client_id")

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, total)
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd backend && go test -tags unit ./internal/service/ -run TestResolveCreatesOnFirstUse -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `Resolve`**

Look up by `(user_id, oauth_client_id)`. On miss, create with: a secret generated by the **same generator ordinary keys use** (a leak must not be worse than an ordinary key leak), `oauth_client_id` set, a name identifying it as agent-backed, active status, and the group from the configured policy. Handle the create racing itself — two concurrent first requests must not both insert; on a unique-violation, re-read and return the winner rather than failing the request.

- [ ] **Step 4: Test the concurrent-first-use race**

Two goroutines calling `Resolve` for the same `(user, client)` concurrently must both succeed and return the **same** row id, with exactly one row created. Run with `-race -count=5`.

- [ ] **Step 5: Group policy and its failure mode**

Read the group from config. If none resolves, return `ErrNoGroupForOAuthKey`. Test that this is a clean typed error and never a nil `Group` on a returned row — a nil group reaches routing and panics.

- [ ] **Step 6: Wire it and gate**

Add to `service.ProviderSet` and `cmd/server/wire.go`; regenerate wire. Gates and commit.

---

### Task 4: The `/v1` OAuth credential branch

**Files:**
- Create: `backend/internal/server/middleware/oauth_inference_auth.go`
- Create: `backend/internal/server/middleware/oauth_inference_auth_test.go`
- Modify: `backend/internal/server/routes/gateway.go`, `backend/internal/server/router.go`

**Interfaces:**
- Consumes: `VerifyOAuthBearer` (Task 1), `OAuthBackingKeyService.Resolve` (Task 3).
- Produces:
  ```go
  // OAuthOrAPIKeyAuth runs the OAuth branch when the credential looks like a
  // JWT, and otherwise delegates to the existing API-key middleware unchanged.
  func OAuthOrAPIKeyAuth(
      apiKeyAuth APIKeyAuthMiddleware,
      keySvc *service.OAuthKeyService,
      clientSvc *service.OAuthClientService,
      backingSvc *service.OAuthBackingKeyService,
      issuer string,
  ) gin.HandlerFunc
  ```

**The ordering that matters (F-A):** this middleware replaces `gin.HandlerFunc(apiKeyAuth)` in the `/v1` chain and runs the shape test **before** any size check, because the API-key path's 256-byte cap aborts a 628-byte JWT before it is ever looked up.

- [ ] **Step 1: Write the failing test — an OAuth token reaches the handler**

```go
func TestOAuthTokenReachesTheHandler(t *testing.T) {
	// A valid RS256 token with scope inference:invoke must pass this
	// middleware and set exactly what the gateway pipeline reads.
	rec := doRequest(t, fixture, "Bearer "+validInferenceToken)
	require.Equal(t, http.StatusOK, rec.Code, "the token must get PAST auth")
}

func TestOAuthTokenIsNotSizeCapped(t *testing.T) {
	// The regression guard for F-A: a 600+ byte credential must not be
	// rejected by the API-key path's 256-byte header cap.
	require.Greater(t, len("Bearer "+validInferenceToken), 256)
	rec := doRequest(t, fixture, "Bearer "+validInferenceToken)
	require.NotEqual(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run and watch both fail**

Run: `cd backend && go test -tags unit ./internal/server/middleware/ -run TestOAuthToken -v`
Expected: FAIL.

- [ ] **Step 3: Implement the branch**

Shape test: a Bearer value with exactly three non-empty `.`-separated base64url segments takes the OAuth branch; everything else delegates to `apiKeyAuth` untouched. On the OAuth branch: `VerifyOAuthBearer` → require `inference:invoke` via `scopeSatisfies` → `backingSvc.Resolve` → set `ctxkey.UserID` on the request context and `ContextKeyAPIKey`, `ContextKeyUser` (`AuthSubject{UserID, Concurrency}`), `ContextKeyUserRole`, `SetOpsFallbackAPIKey`, and group context via the same `setGroupContext` the API-key path calls.

Error mapping: `ErrOAuthInvalidToken` → `401` with an OAuth-shaped body **distinguishable from `INVALID_API_KEY`**; missing scope → `403 insufficient_scope`; `ErrOAuthServerFault` and `ErrNoGroupForOAuthKey` → `500`/`403` respectively, logged, never an auth-shaped error.

- [ ] **Step 4: Run — both pass**

- [ ] **Step 5: Test the properties that keep the two universes separate**

A JWT-shaped credential that fails verification must **not** fall through to the API-key path (assert the response is the OAuth-shaped 401, not `INVALID_API_KEY`). An ordinary API key must still succeed on the same route, byte-identically to today. A valid token **without** `inference:invoke` gets 403, not 401.

- [ ] **Step 6: Mutation-prove the ordering and the no-fallthrough rule**

Move the shape test to *after* the size cap (compiling), show `TestOAuthTokenIsNotSizeCapped` failing. Restore. Then make a failed OAuth verification fall through to `apiKeyAuth`, show the no-fallthrough test failing. Restore. Paste both.

- [ ] **Step 7: Mount on every affected route**

`/v1` group, `/responses`, `/responses/*subpath`, `/alpha/search`. **Not** `/v1beta` — it uses `APIKeyAuthWithSubscriptionGoogle` and is out of scope. Gates and commit.

---

### Task 5: The backing row is invisible

**Files:**
- Modify: `backend/internal/service/api_key_service.go`, `backend/internal/handler/api_key_handler.go` (`List` at `:106`)
- Test: `backend/internal/handler/api_key_handler_oauth_backing_test.go`

**Interfaces:**
- Consumes: `api_keys.oauth_client_id` (Task 2).

- [ ] **Step 1: Write the failing tests**

A user with one ordinary key and one backing row sees **only** the ordinary key from `GET /keys`. Fetching the backing row by id, updating it, and deleting it through the user-facing key endpoints all fail as if it does not exist. And the decisive one:

```go
func TestBackingKeySecretNeverAppearsInAnyResponse(t *testing.T) {
	// The single rule that makes a non-expiring bearer row safe.
	secret := backingRow.Key
	require.NotEmpty(t, secret)
	for _, resp := range allUserFacingKeyResponses(t, fixture) {
		require.NotContains(t, resp.Body.String(), secret,
			"the backing key's secret must never leave the server")
	}
}
```

- [ ] **Step 2: Run and watch them fail**

- [ ] **Step 3: Filter backing rows out of every user-facing key path**

Add `oauth_client_id IS NULL` to the listing query and to every by-id lookup reachable from a user-facing key endpoint. Do this in the **service layer**, so a future handler cannot forget it.

- [ ] **Step 4: Run — all pass**

- [ ] **Step 5: Mutation-prove the filter**

Remove the `IS NULL` predicate from the listing query (compiling), show the listing test and the secret-leak test failing. Restore, paste.

- [ ] **Step 6: Gates and commit**

---

### Task 6: Conformance — re-run the reproduction

**Files:**
- Modify: `backend/scripts/oauth-conformance.md` (add the inference leg)

This task's deliverable is **evidence**, and it is the one that decides whether the goal is met.

- [ ] **Step 1: Stand up the runbook's `t8` infra and obtain a real token**

Use the existing runbook. Drive the unmodified `hermes_cli.auth._nous_device_code_login` for a token with `inference:invoke`, exactly as the evidence file did. Do not hand-mint a token — the point is the real client's real credential.

- [ ] **Step 2: Re-run every endpoint from the evidence table**

`POST /v1/messages`, `POST /v1/chat/completions`, `GET /v1/models`, `POST /v1/responses`, `POST /responses`, `POST /v1/alpha/search`, `POST /alpha/search`.

Expected: every one gets **past auth**. `403 INSUFFICIENT_BALANCE` is a PASS — it is what a funded-less user should see and is exactly what a real API key gets today. `401 INVALID_API_KEY` is a FAIL. Record verbatim status and body for each, beside the pre-fix values.

- [ ] **Step 3: Prove the attribution**

Fund or subscribe the user so a request completes, then assert in Postgres: a `usage_logs` row exists, its `user_id` is the token's `sub`, and its `api_key_id` is the backing row. Then run a second agent instance (a second `client_id`) and assert its usage lands on a **different** `api_key_id` for the same `user_id`.

- [ ] **Step 4: Prove API keys still work**

A real API key on every route from Step 2, reaching the same outcome it reaches today.

- [ ] **Step 5: Prove the secret never leaves**

`grep` the backing row's secret against every response body captured in this run. Zero hits.

- [ ] **Step 6: Extend the runbook and commit**

Add the inference leg beside the device, refresh and dashboard legs, including the `HERMES_HOME` warning. Tear down every `t8*` container, volume and network; confirm the ports are free.

---

## Self-review notes

- Spec coverage: F-A → Task 4 (ordering, mutation-proven). F-B → not fixed, explicit non-goal; Task 4's obligation is only to not add a second path into it. F-C → Task 2 + Task 3. "Never returned" → Task 5. "Done means" 1-6 → Task 6 steps 2-5.
- Task 1 is first because it is a pure refactor that everything else consumes, and because its safety net (the existing `RequireOAuthScope` tests passing **unedited**) is strongest before any new behaviour exists.
- The partial unique index in Task 2 is the one place a mistake would damage existing users — hence its own mutation step proving the index is genuinely partial.
- Deliberately deferred: `/v1beta`, the billing adapter, F-B's IP-wide block, and retiring API-key auth. All are named non-goals in the spec.
