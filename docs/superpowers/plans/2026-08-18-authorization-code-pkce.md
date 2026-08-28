# Authorization Code + PKCE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Hermes Desktop log in, by building the Portal half of the gateway-brokered RFC 8252 flow — `GET /oauth/authorize`, the `authorization_code` grant with PKCE, and the RS256 signing the gateway's verifier demands.

**Architecture:** A new `oauth_authorization_code` entity plus authorize/redeem handlers on the existing `/api/oauth` groups. The signing key moves ES256 → RS256 and verification starts dispatching on `kid`, which is what makes key rotation possible for the first time. Everything else — the token service, refresh rotation, scope vocabulary, client registry — is extended, not replaced.

**Tech Stack:** Go 1.26, Gin, ent, `golang-jwt/jwt/v5`, PostgreSQL, Redis. Frontend: Vue 3 in `inferno-frontend/`.

**Spec:** `docs/superpowers/specs/2026-08-18-authorization-code-pkce-design.md`

## Global Constraints

- **Module path `github.com/Wei-Shaw/sub2api`.**
- 🔴 **OAuth-contract endpoints emit bare JSON — never `internal/pkg/response`.** The documented exception is `ApproveDevice`/`DenyDevice`/`PendingDeviceAuthorization`, which use the envelope because the Vue app's axios interceptor needs `code`/`message`. There is a doc comment above `ApproveDevice` explaining it — read it before touching either side.
- 🔴 **Never log an authorization code, `code_verifier`, refresh token, or access token.** Log `client_id` and `kid` only.
- **ent is codegen:** edit `backend/ent/schema/*.go`, then `cd backend && go generate ./ent`. Never hand-edit generated files.
- **Migrations: 901–905 are taken. Start at 906.** Migrations are checksum-locked once applied — never edit an existing one.
- **Ledger row is D5 on this branch** (`GOAL.md` + `inferno-frontend/scripts/check-divergence.sh`), but the parent branch has reassigned D5 to Razorpay. **Declare under D5 as this branch already does; do not renumber** — that is a merge-time reconciliation, not this plan's job.
- **`.golangci.yml` rule `service-no-repository`** forbids `internal/service/**` importing `internal/repository`. Editing `internal/repository` is fine.
- **Test helper:** `newPaymentConfigServiceTestClient(t) *dbent.Client` (`internal/service/payment_config_service_test.go:385`).
- **Caller identity:** `middleware2.GetAuthSubjectFromContext(c)` → `subject.UserID` (int64). `c.Get("user_id")` does not exist.
- **Gates:** `cd backend && go test ./... && golangci-lint run --new-from-rev=b243c545 ./...` (expect 0 new) and, for frontend work, `cd inferno-frontend && node scripts/june-lint.mjs && npx vue-tsc --noEmit -p tsconfig.json && npx vitest run` (0 new june violations; vitest count must not drop).

---

## File Structure

**Created:**

| File | Responsibility |
|---|---|
| `backend/ent/schema/oauth_authorization_code.go` | the in-flight code entity |
| `backend/migrations/906_oauth_authorization_code.sql` | its table |
| `backend/migrations/907_oauth_rs256_key.sql` | retire the ES256 secret row |
| `backend/internal/service/oauth_authorize_service.go` | issue + redeem codes, PKCE verification |
| `backend/internal/service/oauth_redirect_uri.go` | redirect_uri validation, shared by registration and authorize |
| `backend/internal/handler/oauth_authorize_handler.go` | `GET /oauth/authorize` + its consent POST |
| `inferno-frontend/src/views/oauth/AuthorizeConsentView.vue` | consent screen for the non-auto-approve path |

**Modified:**

| File | Change |
|---|---|
| `backend/internal/service/oauth_signing_key.go` | ES256 → RS256; publish a key **set**; look up by `kid` |
| `backend/internal/server/middleware/oauth_scope.go` | verify RS256, dispatch on `kid` |
| `backend/internal/service/oauth_token_service.go` | `authorization_code` grant; RS256 minting; the 60s reuse grace |
| `backend/internal/service/oauth_scope_vocabulary.go` | add `agent_dashboard:access` |
| `backend/internal/service/oauth_client_service.go` | validate `redirect_uri_origin` at registration |
| `backend/internal/handler/oauth_handler.go` | route the new grant |
| `backend/internal/server/routes/oauth.go` | mount authorize + consent |
| `backend/internal/repository/refresh_token_cache.go` | grace-window support in `MarkRotated` |

---

### Task 1: RS256 signing key with a published key set

Everything downstream signs or verifies with this, so it lands first. Doing it before the code flow also means only one algorithm ever exists in the tree — no interim state where both are live.

**Files:**
- Modify: `backend/internal/service/oauth_signing_key.go`, `backend/internal/server/middleware/oauth_scope.go`, `backend/internal/service/oauth_token_service.go`
- Create: `backend/migrations/907_oauth_rs256_key.sql`
- Test: `backend/internal/service/oauth_signing_key_test.go`, `backend/internal/server/middleware/oauth_scope_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `type SigningKey struct { Kid string; Private *rsa.PrivateKey }` — **`Private` changes type**
  - `(*OAuthKeyService) Active(ctx) (*SigningKey, error)` — unchanged signature
  - `(*OAuthKeyService) ByKid(ctx context.Context, kid string) (*SigningKey, error)` — **new**; returns `ErrUnknownKid` when absent
  - `(*OAuthKeyService) JWKS(ctx) (map[string]any, error)` — now emits an RSA JWK (`kty: RSA`, `n`, `e`) and may emit more than one entry

- [ ] **Step 1: Write the failing tests**

Replace the ES256 assertions in `oauth_signing_key_test.go` and add key-set coverage:

```go
func TestActiveIsRSA(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	key, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if key.Private == nil || key.Private.N == nil {
		t.Fatal("expected an RSA private key")
	}
	if bits := key.Private.N.BitLen(); bits < 2048 {
		t.Fatalf("RSA key must be >= 2048 bits, got %d", bits)
	}
}

func TestJWKSEmitsRSAPublicKeyOnly(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	k := keys[0]
	for _, want := range []string{"kty", "n", "e", "kid", "use", "alg"} {
		if _, present := k[want]; !present {
			t.Errorf("JWKS entry missing %q", want)
		}
	}
	if k["kty"] != "RSA" || k["alg"] != "RS256" {
		t.Errorf("expected RSA/RS256, got %v/%v", k["kty"], k["alg"])
	}
	// The private half must never appear. For RSA that is more fields than ES256's single "d".
	for _, secret := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		if _, leaked := k[secret]; leaked {
			t.Fatalf("JWKS leaked private RSA parameter %q", secret)
		}
	}
}

func TestByKidResolvesTheActiveKeyAndRejectsUnknown(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	active, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}

	got, err := svc.ByKid(ctx, active.Kid)
	if err != nil {
		t.Fatalf("ByKid(active): %v", err)
	}
	if got.Kid != active.Kid {
		t.Fatalf("ByKid returned %q, want %q", got.Kid, active.Kid)
	}

	if _, err := svc.ByKid(ctx, "not-a-real-kid"); !errors.Is(err, ErrUnknownKid) {
		t.Fatalf("expected ErrUnknownKid for an unknown kid, got %v", err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `cd backend && go test ./internal/service/ -run 'TestActiveIsRSA|TestJWKSEmitsRSA|TestByKid' -v`
Expected: FAIL — compile errors on `key.Private.N` and `undefined: ErrUnknownKid`.

- [ ] **Step 3: Convert the key service to RSA**

In `backend/internal/service/oauth_signing_key.go`:
- Change the secret row name from `oauth_es256_active` to `oauth_rs256_active`, keeping the old constant as `legacyES256SecretName` with a comment saying it exists only so `907` can delete its row.
- `SigningKey.Private` becomes `*rsa.PrivateKey`.
- Generate with `rsa.GenerateKey(rand.Reader, 2048)`; marshal with `x509.MarshalPKCS1PrivateKey`, PEM type `RSA PRIVATE KEY`; parse with `x509.ParsePKCS1PrivateKey`.
- `kidFor` hashes the DER of the **public** key (`x509.MarshalPKIXPublicKey`) rather than the EC point; keep the same `base64.RawURLEncoding` of the first 16 SHA-256 bytes so the shape is unchanged.
- `JWKS` emits `{"kty":"RSA","n":<b64url(N)>,"e":<b64url(E)>,"kid":…,"use":"sig","alg":"RS256"}`. Encode `E` as a big-endian byte slice with leading zeros stripped (for the usual 65537 that is `AQAB`).
- Add `ErrUnknownKid` and `ByKid`. For now `ByKid` resolves only the active key, but it must be the **only** lookup verification uses, so adding a second key later is a change in one place.

Keep the concurrency-safe create path exactly as it is: the lost-race branch must still re-read and return the winner, using `dbent.IsConstraintError`. Do not restructure it while changing the algorithm.

- [ ] **Step 4: Run the key tests to green**

Run: `cd backend && go test ./internal/service/ -run 'TestActiveIsRSA|TestJWKSEmitsRSA|TestByKid|TestActiveIsConcurrencySafe' -v`
Expected: PASS, including the pre-existing concurrency test.

- [ ] **Step 5: Switch minting and verification**

In `oauth_token_service.go`'s `mintAccessToken`: `jwt.SigningMethodRS256`, and add the two claims the gateway's verifier reads:

```go
claims := jwt.MapClaims{
	"iss":                    s.issuer,
	"sub":                    strconv.FormatInt(userID, 10),
	"aud":                    clientID,
	"scope":                  scope,
	"iat":                    now.Unix(),
	"exp":                    now.Add(oauthAccessTokenTTL).Unix(),
	"oauth_contract_version": 1,
}
// agent_instance_id is the client_id suffix, which the gateway cross-checks
// against its own configured client_id as defense-in-depth
// (plugins/dashboard_auth/nous/__init__.py:33-35).
if rest, ok := strings.CutPrefix(clientID, "agent:"); ok {
	claims["agent_instance_id"] = rest
}
```

In `middleware/oauth_scope.go`: allow `RS256` only, and resolve the key **by `kid`**:

```go
parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired())
_, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("token has no kid")
	}
	key, err := keySvc.ByKid(c.Request.Context(), kid)
	if err != nil {
		return nil, err
	}
	return &key.Private.PublicKey, nil
})
```

Update the middleware's HMAC-rejection test to also assert an **ES256**-signed token is rejected — that is the algorithm we just left, and it is exactly what a stale token would carry.

- [ ] **Step 6: Write the migration**

`backend/migrations/907_oauth_rs256_key.sql`:

```sql
-- The OAuth AS moved from ES256 to RS256: the gateway's own verifier
-- (plugins/dashboard_auth/nous) hard-codes algorithms=["RS256"], so the
-- constrained consumer decides. No token minted with the old key can be
-- verified any more, and none was ever issued outside test containers --
-- the AS has never been deployed. Deleting the row makes the service
-- generate a fresh RSA key on first use.
DELETE FROM security_secrets WHERE key = 'oauth_es256_active';
```

- [ ] **Step 7: Full gate, then commit**

Run: `cd backend && go test ./... && golangci-lint run --new-from-rev=b243c545 ./...`
Expected: 0 FAIL, 0 new issues. Any test still asserting `ES256` is a real failure — fix the assertion, do not weaken it.

Declare the new file under **D5** in `GOAL.md` and `check-divergence.sh`, then:

```bash
git add backend/ inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): RS256 signing with kid-dispatched verification"
```

---

### Task 2: redirect_uri validation

Small, self-contained, and Task 3 depends on it. Doing it separately keeps the authorize handler's diff about the flow rather than about parsing URLs.

**Files:**
- Create: `backend/internal/service/oauth_redirect_uri.go`
- Modify: `backend/internal/service/oauth_client_service.go`
- Test: `backend/internal/service/oauth_redirect_uri_test.go`

**Interfaces:**
- Produces:
  - `service.ValidateRedirectURI(raw string) error`
  - `service.RedirectURIMatchesClient(registeredOrigin, redirectURI string) error`
  - `service.ErrInvalidRedirectURI`

- [ ] **Step 1: Write the failing test**

```go
func TestValidateRedirectURI(t *testing.T) {
	good := []string{
		"https://agent-abc.tryopencomputer.com/auth/callback",
		"https://gw.example.com/some/prefix/auth/callback",
	}
	for _, u := range good {
		if err := ValidateRedirectURI(u); err != nil {
			t.Errorf("%q must be accepted, got %v", u, err)
		}
	}

	bad := map[string]string{
		"http://agent.example.com/auth/callback":  "plaintext http",
		"https://127.0.0.1:8765/auth/callback":    "loopback — this is exactly why the desktop must broker through its gateway",
		"https://localhost:8765/auth/callback":    "loopback by name",
		"https://[::1]:8765/auth/callback":        "loopback v6",
		"https://agent.example.com/callback":      "does not end in /auth/callback",
		"https://agent.example.com/auth/callback?x=1": "query string",
		"https://agent.example.com/auth/callback#f":   "fragment",
		"https://user:pw@agent.example.com/auth/callback": "userinfo",
		"":                                        "empty",
		"not-a-url":                               "unparseable",
	}
	for u, why := range bad {
		if err := ValidateRedirectURI(u); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must be rejected (%s), got %v", u, why, err)
		}
	}
}

func TestRedirectURIMatchesClient(t *testing.T) {
	const origin = "https://agent-abc.tryopencomputer.com"

	if err := RedirectURIMatchesClient(origin, origin+"/auth/callback"); err != nil {
		t.Errorf("matching origin must be accepted, got %v", err)
	}
	// A different host that merely *contains* the registered origin as a
	// substring must not pass — prefix matching is the classic hole here.
	for _, bad := range []string{
		"https://evil.com/auth/callback",
		"https://agent-abc.tryopencomputer.com.evil.com/auth/callback",
		"https://agent-abc.tryopencomputer.com:8443/auth/callback",
	} {
		if err := RedirectURIMatchesClient(origin, bad); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must not match origin %q, got %v", bad, origin, err)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd backend && go test ./internal/service/ -run 'TestValidateRedirectURI|TestRedirectURIMatchesClient' -v`
Expected: FAIL — `undefined: ValidateRedirectURI`.

- [ ] **Step 3: Implement it**

Parse with `net/url`. Require `https`, a non-empty host, no userinfo, no query, no fragment, and a path ending in `/auth/callback`. Reject loopback by resolving the hostname literal: `127.0.0.0/8`, `::1`, and the literal name `localhost`.

Compare origins by **parsed scheme + host + port equality**, never `strings.HasPrefix` — the test above exists because prefix matching accepts `…tryopencomputer.com.evil.com`.

- [ ] **Step 4: Green, then enforce at registration**

Run the same test — expect PASS.

Then call `ValidateRedirectURI` in `RegisterSelfHosted` before the insert, returning a distinct error the handler maps to `400 invalid_request`. Add a test that registration rejects a loopback origin. **Existing registration tests use `https://agent.example.com` style origins without the `/auth/callback` suffix** — decide deliberately whether registration takes an *origin* (no path) or a full redirect URI, make the validator match, and fix the tests to whichever you chose. Say which in your report.

- [ ] **Step 5: Gate and commit**

Run: `cd backend && go test ./... && golangci-lint run --new-from-rev=<task-1-sha> ./...`

```bash
git commit -m "feat(oauth): validate redirect_uri at registration and authorize"
```

---

### Task 3: authorization codes — issue and redeem

**Files:**
- Create: `backend/ent/schema/oauth_authorization_code.go`, `backend/migrations/906_oauth_authorization_code.sql`, `backend/internal/service/oauth_authorize_service.go`
- Modify: `backend/internal/service/oauth_token_service.go`, `backend/internal/service/oauth_scope_vocabulary.go`
- Test: `backend/internal/service/oauth_authorize_service_test.go`

**Interfaces:**
- Consumes: `ValidateRedirectURI` / `RedirectURIMatchesClient` (Task 2), `OAuthKeyService` (Task 1).
- Produces:
  - `(*OAuthAuthorizeService) IssueCode(ctx, in IssueCodeInput) (string, error)`
  - `type IssueCodeInput struct { ClientID, RedirectURI, Scope, CodeChallenge, CodeChallengeMethod string; UserID int64 }`
  - `(*OAuthTokenService) ExchangeAuthorizationCode(ctx, clientID, code, redirectURI, codeVerifier string) (*OAuthTokens, error)`
  - `service.ErrInvalidGrant` (already exists) for every redemption failure

**Schema** (`oauth_authorization_codes`): `code` (unique), `client_id`, `user_id`, `redirect_uri`, `scope`, `code_challenge`, `code_challenge_method`, `status` (`pending`|`consumed`), `issued_token_family` (nullable — set on redemption so a replay can revoke it), `expires_at`, timestamps. Index `status`, `expires_at`.

- [ ] **Step 1: Write the failing tests**

```go
func TestAuthorizationCodeRoundTrip(t *testing.T) {
	// happy path: issue with a challenge, redeem with the matching verifier
}

func TestAuthorizationCodeIsSingleUseAndReplayRevokes(t *testing.T) {
	// Redeem once -> tokens. Redeem the SAME code again -> ErrInvalidGrant,
	// AND the refresh token from the first redemption must now be dead.
	// RFC 6749 4.1.2: a replayed code revokes what it already minted.
	// The second assertion is the one that matters -- a test checking only
	// that the replay fails passes against an implementation that merely
	// marks the code consumed and leaves the stolen tokens live.
}

func TestAuthorizationCodeRejectsWrongVerifier(t *testing.T) {
	// challenge from verifier A, redeem with verifier B -> ErrInvalidGrant
}

func TestAuthorizationCodeRejectsRedirectURIMismatch(t *testing.T) {
	// redeem with a different redirect_uri than the one bound at issue
}

func TestAuthorizationCodeRejectsPlainChallengeMethod(t *testing.T) {
	// code_challenge_method=plain must be refused at issue; S256 only
}

func TestAuthorizationCodeRejectsForeignClient(t *testing.T) {
	// a code issued for client A is not redeemable by client B
}

func TestAuthorizationCodeExpires(t *testing.T) {
	// past expires_at -> ErrInvalidGrant, and no tokens minted
}
```

Write each body in full following the fixture style of `oauth_token_service_test.go`.

- [ ] **Step 2: Run them and watch them fail**

Run: `cd backend && go test ./internal/service/ -run TestAuthorizationCode -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Schema, codegen, migration**

Write the ent schema, run `cd backend && go generate ./ent`, and write `906_oauth_authorization_code.sql` matching the generated column set exactly — a prior task's review caught column-order drift between the two, so cross-check against `ent/migrate/schema.go` rather than writing the SQL from memory.

- [ ] **Step 4: Implement issue + redeem**

`IssueCode`: validate the method is `S256`, validate the redirect URI against the client's registered origin, `crypto/rand` the code (32 bytes hex), 10-minute TTL, persist `pending`.

`ExchangeAuthorizationCode`: **consume atomically first** — a status-predicated `UPDATE … WHERE code = ? AND status = 'pending'` returning `affected == 0` on a replay, exactly as the device grant's CAS does. Then verify `SHA256(verifier) == challenge` with `subtle.ConstantTimeCompare`, verify `client_id` and `redirect_uri` match what was bound, verify expiry, and only then mint. Record the issued family on the row so a replay can revoke it.

On a replay (`affected == 0` with the row already `consumed`), call `DeleteTokenFamily` on the recorded family before returning `ErrInvalidGrant`.

Add `ScopeAgentDashboardAccess = "agent_dashboard:access"` to the vocabulary and the `knownScopes` map.

- [ ] **Step 5: Wire the grant into the token endpoint**

Add the `authorization_code` case to `Token`'s switch in `oauth_handler.go`, reading `code`, `redirect_uri`, `client_id`, `code_verifier` from the form. Bare JSON, and the same `ErrInvalidGrant` → `{"error":"invalid_grant"}` mapping the refresh branch uses. Unmatched internal errors log and return `500 server_error` — do not fold them into an OAuth error code.

- [ ] **Step 6: Green, gate, commit**

Run: `cd backend && go test ./... && golangci-lint run --new-from-rev=<task-2-sha> ./...`

```bash
git commit -m "feat(oauth): authorization_code grant with PKCE"
```

---

### Task 4: `GET /oauth/authorize` and the consent screen

**Files:**
- Create: `backend/internal/handler/oauth_authorize_handler.go`, `inferno-frontend/src/views/oauth/AuthorizeConsentView.vue` (+ spec)
- Modify: `backend/internal/server/routes/oauth.go`, `inferno-frontend/src/router/index.ts`

- [ ] **Step 1: Implement the authorize endpoint**

`GET /oauth/authorize` is a **browser** endpoint on the session-authenticated group. An unauthenticated visitor must be redirected to login and returned here **with the full query string intact** — losing it drops the PKCE challenge and the desktop's `state`. `DeviceApprovalView`'s route already solves this; copy how it preserves `redirect`.

Validate: `response_type=code`, a known and usable `client_id`, `redirect_uri` matching the client's registered origin, `code_challenge` present with `code_challenge_method=S256`, and the scope inside the vocabulary.

**Errors split two ways, and the split is a security requirement.** A bad `redirect_uri` or unknown `client_id` must render an error page — redirecting an unvalidated URI is an open redirect. Every *other* error redirects back to the validated `redirect_uri` with `?error=…&state=…`, per RFC 6749 §4.1.2.1.

**Auto-approve** when the requested scope is exactly `agent_dashboard:access` **and** the session user owns the client. Otherwise render the consent screen. Auto-approval issues the code and 302s to `redirect_uri?code=…&state=…`.

- [ ] **Step 2: Build the consent screen**

Reuse `DeviceApprovalView`'s two-step pattern: show the client name and a human-readable permission list, and do not render Approve until they are on screen. New `.vue` files are held to all ten June rules from birth — copy a converted view's structure.

- [ ] **Step 3: Both gates, then commit**

Backend gate plus `cd inferno-frontend && node scripts/june-lint.mjs && npx vue-tsc --noEmit -p tsconfig.json && npx vitest run`.

```bash
git commit -m "feat(oauth): /oauth/authorize with consent and org auto-approve"
```

---

### Task 5: the 60-second refresh reuse grace

**Files:** modify `backend/internal/repository/refresh_token_cache.go`, `backend/internal/service/refresh_token_cache.go`, `backend/internal/service/oauth_token_service.go`

The Portal contract documents a 60s grace on refresh reuse (`plugins/dashboard_auth/nous/__init__.py:57-60`); we revoke instantly, which would kill healthy desktop sessions on a benign retry or two windows refreshing together.

- [ ] **Step 1: Extend the Lua script**

`MarkRotated` currently returns `alreadyRotated`. It must also return **when** the rotation happened, so the caller can tell a replay inside the grace from one outside it. Keep the whole read-decide-write inside the single `EVAL` — the atomicity is what closes the rotation race, and splitting it reopens a Critical.

- [ ] **Step 2: Apply the grace**

Within 60s of rotation, a replay of the **immediately previous** token returns the current pair instead of revoking. Outside it, revoke the family exactly as now.

⚠️ Scope this precisely: the grace applies to one token, once, for 60 seconds. Any broader reading weakens reuse detection, which is the property the whole rotation design exists to provide.

- [ ] **Step 3: Test both sides**

Replay at t+30s returns the current pair and the family survives; replay at t+90s revokes. Mutation-test it: an implementation without the grace must fail the first test, and one that always grants must fail the second.

- [ ] **Step 4: Gate and commit**

---

### Task 6: Conformance against the real gateway plugin

The highest-value task. The device-grant equivalent caught two defects no unit test could.

- [ ] **Step 1: Drive the real provider**

From the read-only client repo, instantiate `plugins/dashboard_auth/nous`'s provider against a local Inferno and run the full protocol: `start_login` → approve in the browser → `complete_login` → `verify_session` → `refresh_session`.

That provider verifies our token with PyJWT against our JWKS, so a green run proves the RS256 switch, the claim set, the audience, the issuer and `kid` resolution **all at once**.

**Do not edit the client repo.** If something fails, the fix goes in our server.

- [ ] **Step 2: Assert the claim contract explicitly**

Decode the minted token and confirm `aud` is the **bare** `client_id`, `iss` is the portal base URL, `exp`/`iat`/`sub` are present, `oauth_contract_version == 1`, and `agent_instance_id` equals the `client_id` suffix.

- [ ] **Step 3: Extend the runbook and commit**

Add the dashboard leg to `backend/scripts/oauth-conformance.md` beside the device and refresh legs, including the `HERMES_HOME` warning.

---

## Self-review notes

- Spec coverage: D-1 → Task 1. D-2 → Task 3 step 4. D-3 → Task 2. D-4 → Task 5. D-5 → Task 3. D-6 → Task 4.
- Task 1 changes `SigningKey.Private`'s type, which breaks every existing caller — that is deliberate and is why it is first; the compiler enumerates the work.
- Task 3's replay test asserts **revocation**, not just failure, for the same reason the refresh-reuse test had to: the weaker assertion passes against the bug.
- Deliberately deferred: retiring the ES256 constant entirely (kept so migration 907 documents itself), and multi-key rotation beyond `ByKid` existing (the seam is what Task 1 owes; populating a second key is a later operational change).
