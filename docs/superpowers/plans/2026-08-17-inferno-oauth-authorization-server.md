# Inferno OAuth 2.0 Authorization Server — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Inferno an OAuth 2.0 authorization server so `hermes-agent` gateways and Hermes Desktop can authenticate against it with zero client code changes, replacing Nous Portal.

**Architecture:** New Gin routes under `/api/oauth/*` backed by an `OAuthService`, three new ent entities (`org`/`org_member`, `oauth_client`, `oauth_device_authorization`), and a second ES256 signing key published via JWKS. Inferno's existing HMAC session tokens and refresh-token families are reused, not replaced.

**Tech Stack:** Go 1.26, Gin, ent (codegen ORM), `golang-jwt/jwt/v5`, PostgreSQL. Frontend: Vue 3 + Vite in `inferno-frontend/`.

**Spec:** `docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md`

## Global Constraints

Every task's requirements implicitly include this section.

- **Go module path is `github.com/Wei-Shaw/sub2api`.** All imports use it, including our new files. Do not rename it.
- **🔴 OAuth endpoints MUST NOT use `internal/pkg/response`.** That package wraps every body in `{code, message, data}`. The hermes client parses `response.json()["device_code"]` directly (`hermes_cli/auth.py:4959`) and will hard-fail on a wrapped body. OAuth handlers emit **bare RFC-shaped JSON via `c.JSON(status, gin.H{...})`**. This applies to every endpoint in this plan except `GET /api/oauth/account`, which is ours to shape.
- **Endpoint paths are fixed by the upstream client contract.** Do not "improve" them.
- **ent is codegen.** Edit `ent/schema/*.go`, then run `cd backend && go generate ./ent`. Never hand-edit generated files under `ent/`.
- **Migrations use the 9xx fork series** (upstream is at 223; 900 is taken by `900_add_user_avatar_seed.sql`). Use 901+.
- **Every new/changed file under `backend/`, `frontend/`, `deploy/`, `docs/` must be declared** in `DECLARED` inside `inferno-frontend/scripts/check-divergence.sh` AND in `GOAL.md`'s ledger under entry **D4**, or gate 5 fails. Add to both in the same commit that creates the file.
- **Backend gates:** `cd backend && go test ./...` and `golangci-lint run ./...` must pass before every commit.
- **Frontend work goes in `inferno-frontend/` only.** `frontend/` is a pristine upstream mirror — never edit it. New files are always in june-lint scope, so build to June rules from the start. Gate: `cd inferno-frontend && node scripts/june-lint.mjs && npx vue-tsc --noEmit -p tsconfig.json && npx vitest run`.
- **Never log a token.** Log `kid` and `client_id` only.
- **Money is not involved in this sub-project.** If you find yourself formatting a currency, you are out of scope.

---

## File Structure

**Created:**

| File | Responsibility |
|---|---|
| `backend/ent/schema/org.go` | `org` entity |
| `backend/ent/schema/org_member.go` | `org_member` entity (org↔user, role) |
| `backend/ent/schema/oauth_client.go` | registered OAuth clients (`agent:{id}`) |
| `backend/ent/schema/oauth_device_authorization.go` | device-flow state rows |
| `backend/migrations/901_org_and_members.sql` | org tables |
| `backend/migrations/902_oauth_client.sql` | oauth_client table |
| `backend/migrations/903_oauth_device_authorization.sql` | device auth table |
| `backend/internal/service/oauth_signing_key.go` | ES256 keypair load/generate/rotate, JWKS projection |
| `backend/internal/service/oauth_service.go` | device flow, code flow, token minting |
| `backend/internal/service/org_service.go` | personal-org creation, membership lookup |
| `backend/internal/handler/oauth_handler.go` | all `/api/oauth/*` + `/oauth/authorize` handlers |
| `backend/internal/handler/dto/oauth.go` | request/response shapes |
| `backend/internal/server/middleware/oauth_scope.go` | bearer + scope enforcement |
| `backend/internal/server/routes/oauth.go` | route registration |
| `inferno-frontend/src/views/oauth/DeviceApprovalView.vue` | the human approval screen |

**Modified:**

| File | Change |
|---|---|
| `backend/internal/server/routes/common.go` | mount the oauth route group |
| `backend/internal/service/auth_service.go` | call `OrgService.EnsurePersonalOrg` on registration |
| `inferno-frontend/src/router/index.ts` | route for the approval view |
| `inferno-frontend/scripts/check-divergence.sh` | D4 declarations |
| `GOAL.md` | D4 ledger rows |

---

### Task 1: Org tables + personal org on signup

Nothing else can reference `org_id` until this exists.

**Files:**
- Create: `backend/ent/schema/org.go`, `backend/ent/schema/org_member.go`, `backend/migrations/901_org_and_members.sql`, `backend/internal/service/org_service.go`
- Modify: `backend/internal/service/auth_service.go`
- Test: `backend/internal/service/org_service_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `service.OrgService` with `NewOrgService(entClient *dbent.Client) *OrgService`
  - `(*OrgService) EnsurePersonalOrg(ctx context.Context, userID int64, username string) (*dbent.Org, error)` — idempotent
  - `(*OrgService) OrgsForUser(ctx context.Context, userID int64) ([]*dbent.Org, error)`
  - `(*OrgService) RoleIn(ctx context.Context, orgID, userID int64) (string, error)` — returns `""` when not a member
  - Role constants `service.RoleOwner = "OWNER"`, `RoleAdmin = "ADMIN"`, `RoleMember = "MEMBER"`

- [ ] **Step 1: Write the failing test**

`backend/internal/service/org_service_test.go`:

```go
package service

import (
	"context"
	"testing"
)

func TestEnsurePersonalOrgIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOrgService(client)

	first, err := svc.EnsurePersonalOrg(ctx, 42, "saksham")
	if err != nil {
		t.Fatalf("first EnsurePersonalOrg: %v", err)
	}
	if !first.IsPersonal {
		t.Fatalf("expected IsPersonal=true, got false")
	}

	second, err := svc.EnsurePersonalOrg(ctx, 42, "saksham")
	if err != nil {
		t.Fatalf("second EnsurePersonalOrg: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("not idempotent: created org %d then %d", first.ID, second.ID)
	}
}

func TestEnsurePersonalOrgMakesUserOwner(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOrgService(client)

	org, err := svc.EnsurePersonalOrg(ctx, 7, "archit")
	if err != nil {
		t.Fatalf("EnsurePersonalOrg: %v", err)
	}

	role, err := svc.RoleIn(ctx, org.ID, 7)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != RoleOwner {
		t.Fatalf("expected %q, got %q", RoleOwner, role)
	}
}

func TestRoleInReturnsEmptyForNonMember(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOrgService(client)

	org, err := svc.EnsurePersonalOrg(ctx, 7, "archit")
	if err != nil {
		t.Fatalf("EnsurePersonalOrg: %v", err)
	}

	role, err := svc.RoleIn(ctx, org.ID, 999)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != "" {
		t.Fatalf("expected empty role for non-member, got %q", role)
	}
}
```

If `newTestEntClient` does not already exist in the `service` package, find the existing in-package test helper that builds an ent client (grep `enttest` under `backend/internal/service`) and use that name instead. Do not invent a second helper.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/service/ -run TestEnsurePersonalOrg -v`
Expected: FAIL — `undefined: NewOrgService`

- [ ] **Step 3: Write the ent schemas**

`backend/ent/schema/org.go`:

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Org is a tenant. One personal org is auto-created per user at signup.
type Org struct {
	ent.Schema
}

func (Org) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "orgs"},
	}
}

func (Org) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Org) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("name").
			MaxLen(128).
			NotEmpty(),
		field.Bool("is_personal").
			Default(false),
	}
}
```

`backend/ent/schema/org_member.go`:

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrgMember binds a user to an org with a role.
type OrgMember struct {
	ent.Schema
}

func (OrgMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "org_members"},
	}
}

func (OrgMember) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OrgMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("org_id"),
		field.Int64("user_id"),
		// OWNER | ADMIN | MEMBER. These exact strings are read by the desktop
		// (apps/desktop/electron/main.ts trimCloudOrg) — do not change casing.
		field.String("role").
			MaxLen(16).
			Default("MEMBER"),
	}
}

func (OrgMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
```

- [ ] **Step 4: Run codegen**

Run: `cd backend && go generate ./ent`
Expected: new files under `backend/ent/org*` and `backend/ent/orgmember*`; no errors.

- [ ] **Step 5: Write the migration**

`backend/migrations/901_org_and_members.sql`:

```sql
CREATE TABLE IF NOT EXISTS orgs (
    id          BIGSERIAL PRIMARY KEY,
    slug        VARCHAR(64)  NOT NULL UNIQUE,
    name        VARCHAR(128) NOT NULL,
    is_personal BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS org_members (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    role       VARCHAR(16) NOT NULL DEFAULT 'MEMBER',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS org_members_org_id_user_id_key
    ON org_members (org_id, user_id);
CREATE INDEX IF NOT EXISTS org_members_user_id_idx
    ON org_members (user_id);
```

- [ ] **Step 6: Write the service**

`backend/internal/service/org_service.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/orgmember"
)

const (
	RoleOwner  = "OWNER"
	RoleAdmin  = "ADMIN"
	RoleMember = "MEMBER"
)

// OrgService owns tenancy: personal-org creation and membership lookup.
type OrgService struct {
	entClient *dbent.Client
}

func NewOrgService(entClient *dbent.Client) *OrgService {
	return &OrgService{entClient: entClient}
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// slugFor derives a URL-safe slug from a username, with a random suffix so two
// users named "admin" cannot collide on the unique index.
func slugFor(username string) (string, error) {
	base := slugUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(username)), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "org"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("slug suffix: %w", err)
	}
	return base + "-" + hex.EncodeToString(buf), nil
}

// EnsurePersonalOrg returns the user's personal org, creating it on first call.
// Idempotent: safe to call on every login.
func (s *OrgService) EnsurePersonalOrg(ctx context.Context, userID int64, username string) (*dbent.Org, error) {
	members, err := s.entClient.OrgMember.Query().
		Where(orgmember.UserID(userID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query memberships: %w", err)
	}
	for _, m := range members {
		org, err := s.entClient.Org.Get(ctx, m.OrgID)
		if err != nil {
			return nil, fmt.Errorf("load org %d: %w", m.OrgID, err)
		}
		if org.IsPersonal {
			return org, nil
		}
	}

	slug, err := slugFor(username)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(username)
	if name == "" {
		name = slug
	}

	org, err := s.entClient.Org.Create().
		SetSlug(slug).
		SetName(name).
		SetIsPersonal(true).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create personal org: %w", err)
	}

	if _, err := s.entClient.OrgMember.Create().
		SetOrgID(org.ID).
		SetUserID(userID).
		SetRole(RoleOwner).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("create owner membership: %w", err)
	}

	return org, nil
}

// OrgsForUser lists every org the user belongs to.
func (s *OrgService) OrgsForUser(ctx context.Context, userID int64) ([]*dbent.Org, error) {
	members, err := s.entClient.OrgMember.Query().
		Where(orgmember.UserID(userID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query memberships: %w", err)
	}
	orgs := make([]*dbent.Org, 0, len(members))
	for _, m := range members {
		org, err := s.entClient.Org.Get(ctx, m.OrgID)
		if err != nil {
			return nil, fmt.Errorf("load org %d: %w", m.OrgID, err)
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// RoleIn returns the user's role in an org, or "" when they are not a member.
func (s *OrgService) RoleIn(ctx context.Context, orgID, userID int64) (string, error) {
	m, err := s.entClient.OrgMember.Query().
		Where(orgmember.OrgID(orgID), orgmember.UserID(userID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query membership: %w", err)
	}
	return m.Role, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ -run 'TestEnsurePersonalOrg|TestRoleIn' -v`
Expected: PASS (3 tests)

- [ ] **Step 8: Wire personal-org creation into registration**

In `backend/internal/service/auth_service.go`, add an `orgService *OrgService` field to `AuthService`, accept it in the constructor, and call it immediately after a user row is created in `loginOrRegisterOAuthWithTokenPair` and in the email/password registration path.

Failure to create the org must **not** fail the signup — log and continue. A user without an org can still use the API; blocking registration on a tenancy row is a worse outcome than a missing org, and `EnsurePersonalOrg` is idempotent so the next login repairs it:

```go
if _, err := s.orgService.EnsurePersonalOrg(ctx, user.ID, user.Username); err != nil {
	slog.Warn("oauth: personal org creation failed, will retry on next login",
		"user_id", user.ID, "error", err)
}
```

Update every construction site of `NewAuthService` (grep `NewAuthService(` across `backend/`) to pass the new dependency.

- [ ] **Step 9: Run the full backend gate**

Run: `cd backend && go test ./... && golangci-lint run ./...`
Expected: PASS, no lint findings.

- [ ] **Step 10: Declare divergence and commit**

Add to `DECLARED` in `inferno-frontend/scripts/check-divergence.sh` under the `# D4` heading:

```
backend/ent/schema/org.go
backend/ent/schema/org_member.go
backend/migrations/901_org_and_members.sql
backend/internal/service/org_service.go
backend/internal/service/org_service_test.go
backend/internal/service/auth_service.go
```

Plus every file `go generate ./ent` created — get the list with `git status --porcelain backend/ent | awk '{print $2}'`.

Add a matching row to `GOAL.md`'s ledger under D4.

```bash
cd /Users/saksham/OpenComputerV2/inferno
git add backend/ent backend/migrations/901_org_and_members.sql \
        backend/internal/service/org_service.go \
        backend/internal/service/org_service_test.go \
        backend/internal/service/auth_service.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): org + org_member tenancy, personal org on signup"
```

---

### Task 2: ES256 signing key + JWKS endpoint

**Files:**
- Create: `backend/internal/service/oauth_signing_key.go`, `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`
- Modify: `backend/internal/server/routes/common.go`
- Test: `backend/internal/service/oauth_signing_key_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces:
  - `service.OAuthKeyService` with `NewOAuthKeyService(entClient *dbent.Client) *OAuthKeyService`
  - `(*OAuthKeyService) Active(ctx context.Context) (*SigningKey, error)` — generates and persists on first call
  - `(*OAuthKeyService) JWKS(ctx context.Context) (map[string]any, error)`
  - `type SigningKey struct { Kid string; Private *ecdsa.PrivateKey }`
  - `handler.OAuthHandler` + `NewOAuthHandler(...)`
  - `routes.RegisterOAuthRoutes(rg *gin.RouterGroup, h *handler.OAuthHandler)`

The keypair is persisted in the existing `security_secrets` table under key `oauth_es256_active`, PEM-encoded. `security_secrets` already exists — no schema change.

- [ ] **Step 1: Write the failing test**

`backend/internal/service/oauth_signing_key_test.go`:

```go
package service

import (
	"context"
	"crypto/ecdsa"
	"testing"
)

func TestActiveGeneratesAndPersistsKey(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthKeyService(client)

	first, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("first Active: %v", err)
	}
	if first.Kid == "" {
		t.Fatal("expected non-empty kid")
	}
	if _, ok := any(first.Private).(*ecdsa.PrivateKey); !ok {
		t.Fatal("expected an ECDSA private key")
	}

	second, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("second Active: %v", err)
	}
	if first.Kid != second.Kid {
		t.Fatalf("key not persisted: kid %q then %q", first.Kid, second.Kid)
	}
}

func TestJWKSExposesPublicKeyOnly(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthKeyService(client)

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}

	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	k := keys[0]
	for _, want := range []string{"kty", "crv", "x", "y", "kid", "use", "alg"} {
		if _, present := k[want]; !present {
			t.Errorf("JWKS entry missing %q", want)
		}
	}
	if _, leaked := k["d"]; leaked {
		t.Fatal("JWKS leaked the private scalar 'd'")
	}
	if k["alg"] != "ES256" {
		t.Errorf("expected alg ES256, got %v", k["alg"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/service/ -run 'TestActiveGenerates|TestJWKSExposes' -v`
Expected: FAIL — `undefined: NewOAuthKeyService`

- [ ] **Step 3: Write the key service**

`backend/internal/service/oauth_signing_key.go`:

```go
package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
)

// activeKeySecretName is the security_secrets row holding the PEM-encoded
// ES256 private key used to sign OAuth-issued tokens. Deliberately separate
// from Inferno's HMAC session-token secret: a symmetric key cannot be
// published as a JWKS, and agents must verify signatures offline.
const activeKeySecretName = "oauth_es256_active"

type SigningKey struct {
	Kid     string
	Private *ecdsa.PrivateKey
}

type OAuthKeyService struct {
	entClient *dbent.Client
}

func NewOAuthKeyService(entClient *dbent.Client) *OAuthKeyService {
	return &OAuthKeyService{entClient: entClient}
}

// kidFor derives a stable key id from the public key bytes, so the same key
// always produces the same kid without storing it separately.
func kidFor(pub *ecdsa.PublicKey) string {
	raw := elliptic.Marshal(pub.Curve, pub.X, pub.Y) //nolint:staticcheck // JWK thumbprint input
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

// Active returns the current signing key, generating and persisting one on
// first use.
func (s *OAuthKeyService) Active(ctx context.Context) (*SigningKey, error) {
	row, err := s.entClient.SecuritySecret.Query().
		Where(securitysecret.Key(activeKeySecretName)).
		Only(ctx)
	if err == nil {
		block, _ := pem.Decode([]byte(row.Value))
		if block == nil {
			return nil, fmt.Errorf("oauth signing key: stored PEM is undecodable")
		}
		priv, perr := x509.ParseECPrivateKey(block.Bytes)
		if perr != nil {
			return nil, fmt.Errorf("oauth signing key: parse: %w", perr)
		}
		return &SigningKey{Kid: kidFor(&priv.PublicKey), Private: priv}, nil
	}
	if !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("oauth signing key: query: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: generate: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: marshal: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	if _, err := s.entClient.SecuritySecret.Create().
		SetKey(activeKeySecretName).
		SetValue(string(encoded)).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("oauth signing key: persist: %w", err)
	}

	return &SigningKey{Kid: kidFor(&priv.PublicKey), Private: priv}, nil
}

func b64uint(i *big.Int, size int) string {
	b := i.Bytes()
	if len(b) < size {
		padded := make([]byte, size)
		copy(padded[size-len(b):], b)
		b = padded
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// JWKS projects the active key to its public JWK form. Never includes "d".
func (s *OAuthKeyService) JWKS(ctx context.Context) (map[string]any, error) {
	key, err := s.Active(ctx)
	if err != nil {
		return nil, err
	}
	pub := key.Private.PublicKey
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "EC",
			"crv": "P-256",
			"x":   b64uint(pub.X, 32),
			"y":   b64uint(pub.Y, 32),
			"kid": key.Kid,
			"use": "sig",
			"alg": "ES256",
		}},
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ -run 'TestActiveGenerates|TestJWKSExposes' -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Write the handler and route**

`backend/internal/handler/oauth_handler.go`:

```go
package handler

import (
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthHandler serves the OAuth 2.0 authorization-server surface.
//
// NOTE: these handlers deliberately do NOT use internal/pkg/response. That
// package wraps bodies in {code,message,data}; the hermes client parses
// RFC-shaped JSON at the top level and hard-fails on a wrapped body.
type OAuthHandler struct {
	keySvc *service.OAuthKeyService
}

func NewOAuthHandler(keySvc *service.OAuthKeyService) *OAuthHandler {
	return &OAuthHandler{keySvc: keySvc}
}

// JWKS handles GET /.well-known/jwks.json
func (h *OAuthHandler) JWKS(c *gin.Context) {
	jwks, err := h.keySvc.JWKS(c.Request.Context())
	if err != nil {
		slog.Error("oauth: jwks projection failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, jwks)
}
```

`backend/internal/server/routes/oauth.go`:

```go
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterOAuthWellKnown mounts unauthenticated discovery endpoints at the
// server root (NOT under /api) — JWKS must live at the well-known path.
func RegisterOAuthWellKnown(r gin.IRouter, h *handler.OAuthHandler) {
	r.GET("/.well-known/jwks.json", h.JWKS)
}
```

- [ ] **Step 6: Mount the route**

In `backend/internal/server/routes/common.go`, follow the existing registration pattern and call `RegisterOAuthWellKnown` on the root router (not the `/api` group). Construct `NewOAuthKeyService` and `NewOAuthHandler` wherever the other handlers are constructed.

- [ ] **Step 7: Verify the endpoint end to end**

Run the server locally, then:

```bash
curl -s localhost:8080/.well-known/jwks.json | python3 -m json.tool
```

Expected: a `keys` array with one entry containing `kty`, `crv`, `x`, `y`, `kid`, `use`, `alg: ES256`, and **no** `d`.

- [ ] **Step 8: Run the full backend gate**

Run: `cd backend && go test ./... && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 9: Declare divergence and commit**

Add the four new/modified backend files to `DECLARED` and `GOAL.md` D4, then:

```bash
git add backend/internal/service/oauth_signing_key.go \
        backend/internal/service/oauth_signing_key_test.go \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        backend/internal/server/routes/common.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): ES256 signing key + JWKS endpoint"
```

---

### Task 3: oauth_client table + self-hosted client registration

**Files:**
- Create: `backend/ent/schema/oauth_client.go`, `backend/migrations/902_oauth_client.sql`, `backend/internal/service/oauth_client_service.go`, `backend/internal/handler/dto/oauth.go`
- Modify: `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`
- Test: `backend/internal/service/oauth_client_service_test.go`

**Interfaces:**
- Consumes: `service.OrgService` (Task 1).
- Produces:
  - `service.OAuthClientService` with `NewOAuthClientService(entClient *dbent.Client) *OAuthClientService`
  - `(*OAuthClientService) RegisterSelfHosted(ctx context.Context, orgID, userID int64, redirectOrigin string) (*dbent.OAuthClient, error)`
  - `(*OAuthClientService) ByClientID(ctx context.Context, clientID string) (*dbent.OAuthClient, error)`
  - `(*OAuthClientService) GenerateName() string`
  - Status constants `service.ClientPending = "pending"`, `ClientActive = "active"`, `ClientRevoked = "revoked"`

- [ ] **Step 1: Write the failing test**

`backend/internal/service/oauth_client_service_test.go`:

```go
package service

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterSelfHostedAppliesAgentPrefix(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if !strings.HasPrefix(got.ClientID, "agent:") {
		t.Fatalf("expected an agent: prefix, got %q", got.ClientID)
	}
	if got.Status != ClientPending {
		t.Fatalf("expected status %q, got %q", ClientPending, got.Status)
	}
}

func TestRegisterSelfHostedNamesAreNotRequiredToBeUnique(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthClientService(client)

	// Two registrations must both succeed even if names collide. Upstream
	// treats the row id as the key; a name collision is harmless.
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://a.example.com"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://b.example.com"); err != nil {
		t.Fatalf("second register: %v", err)
	}
}

func TestByClientIDRoundTrips(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthClientService(client)

	created, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	found, err := svc.ByClientID(ctx, created.ClientID)
	if err != nil {
		t.Fatalf("ByClientID: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("round trip mismatch: %d vs %d", found.ID, created.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/service/ -run 'TestRegisterSelfHosted|TestByClientID' -v`
Expected: FAIL — `undefined: NewOAuthClientService`

- [ ] **Step 3: Write the ent schema**

`backend/ent/schema/oauth_client.go`:

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthClient is a registered OAuth client — one per gateway instance.
// These are PUBLIC clients: there is deliberately no client_secret column.
// PKCE is the protection.
type OAuthClient struct {
	ent.Schema
}

func (OAuthClient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_clients"},
	}
}

func (OAuthClient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		field.String("client_id").
			MaxLen(128).
			NotEmpty().
			Unique(),
		// SELF_HOSTED | HOSTED
		field.String("kind").
			MaxLen(16).
			Default("SELF_HOSTED"),
		// Docker-style adjective_noun. NOT unique — the row id is the key.
		field.String("name").
			MaxLen(64).
			NotEmpty(),
		field.Int64("owner_user_id"),
		field.Int64("org_id"),
		// Set by oc-platform once the VM exists. Registration is idempotent
		// on this value so a retried provision reuses the row.
		field.String("instance_id").
			MaxLen(128).
			Optional().
			Nillable().
			Unique(),
		// pending | active | revoked
		field.String("status").
			MaxLen(16).
			Default("pending"),
		field.String("redirect_uri_origin").
			MaxLen(255).
			NotEmpty(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
	}
}

func (OAuthClient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("owner_user_id"),
		index.Fields("status"),
	}
}
```

- [ ] **Step 4: Run codegen**

Run: `cd backend && go generate ./ent`
Expected: `backend/ent/oauthclient*` generated; no errors.

- [ ] **Step 5: Write the migration**

`backend/migrations/902_oauth_client.sql`:

```sql
CREATE TABLE IF NOT EXISTS oauth_clients (
    id                  BIGSERIAL PRIMARY KEY,
    client_id           VARCHAR(128) NOT NULL UNIQUE,
    kind                VARCHAR(16)  NOT NULL DEFAULT 'SELF_HOSTED',
    name                VARCHAR(64)  NOT NULL,
    owner_user_id       BIGINT       NOT NULL,
    org_id              BIGINT       NOT NULL,
    instance_id         VARCHAR(128),
    status              VARCHAR(16)  NOT NULL DEFAULT 'pending',
    redirect_uri_origin VARCHAR(255) NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS oauth_clients_instance_id_key
    ON oauth_clients (instance_id) WHERE instance_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS oauth_clients_org_id_idx        ON oauth_clients (org_id);
CREATE INDEX IF NOT EXISTS oauth_clients_owner_user_id_idx ON oauth_clients (owner_user_id);
CREATE INDEX IF NOT EXISTS oauth_clients_status_idx        ON oauth_clients (status);
```

- [ ] **Step 6: Write the service**

`backend/internal/service/oauth_client_service.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
)

const (
	ClientPending = "pending"
	ClientActive  = "active"
	ClientRevoked = "revoked"
)

// Docker-style name parts. Mirrors hermes_cli/dashboard_register.py so
// registered agents read the same on both surfaces.
var (
	nameAdjectives = []string{
		"amber", "bold", "brave", "bright", "calm", "clever", "cosmic", "crisp",
		"dreamy", "eager", "electric", "fancy", "gentle", "golden", "happy",
		"hidden", "jolly", "keen", "lively", "lucid", "lunar", "mellow", "merry",
		"mighty", "nimble", "noble", "polished", "quiet", "quirky", "rapid",
		"serene", "sharp", "shiny", "silent", "snappy", "solar", "spry", "stellar",
		"sunny", "swift", "tidy", "vivid", "vibrant", "witty", "zesty",
	}
	nameNouns = []string{
		"albatross", "antelope", "badger", "beacon", "comet", "condor", "cypress",
		"dolphin", "ember", "falcon", "ferret", "galaxy", "glacier", "harbor",
		"heron", "ibex", "jaguar", "kestrel", "lantern", "lynx", "meadow", "nebula",
		"ocelot", "orchid", "otter", "panther", "petrel", "quasar", "raven", "reef",
		"sparrow", "summit", "tundra", "vortex", "walrus", "willow", "yarrow",
		"kepler", "tesla", "curie", "hopper", "turing", "lovelace",
	}
)

type OAuthClientService struct {
	entClient *dbent.Client
}

func NewOAuthClientService(entClient *dbent.Client) *OAuthClientService {
	return &OAuthClientService{entClient: entClient}
}

func pick(list []string) string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(list))))
	if err != nil {
		return list[0]
	}
	return list[n.Int64()]
}

// GenerateName returns a docker-style adjective_noun label. There is no
// uniqueness constraint — collisions are harmless.
func (s *OAuthClientService) GenerateName() string {
	return pick(nameAdjectives) + "_" + pick(nameNouns)
}

// RegisterSelfHosted creates a SELF_HOSTED client owned by the given org.
// The "agent:" prefix is applied SERVER-side; callers never construct it.
func (s *OAuthClientService) RegisterSelfHosted(ctx context.Context, orgID, userID int64, redirectOrigin string) (*dbent.OAuthClient, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("client id entropy: %w", err)
	}
	clientID := "agent:" + hex.EncodeToString(buf)

	created, err := s.entClient.OAuthClient.Create().
		SetClientID(clientID).
		SetKind("SELF_HOSTED").
		SetName(s.GenerateName()).
		SetOwnerUserID(userID).
		SetOrgID(orgID).
		SetStatus(ClientPending).
		SetRedirectURIOrigin(redirectOrigin).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}
	return created, nil
}

// ByClientID loads a client by its public client_id.
func (s *OAuthClientService) ByClientID(ctx context.Context, clientID string) (*dbent.OAuthClient, error) {
	return s.entClient.OAuthClient.Query().
		Where(oauthclient.ClientID(clientID)).
		Only(ctx)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ -run 'TestRegisterSelfHosted|TestByClientID' -v`
Expected: PASS (3 tests)

- [ ] **Step 8: Add the registration endpoint**

`backend/internal/handler/dto/oauth.go`:

```go
package dto

// SelfHostedClientRequest is the body of POST /api/oauth/self-hosted-client.
type SelfHostedClientRequest struct {
	Name           string `json:"name"`
	RedirectOrigin string `json:"redirect_origin" binding:"required"`
}

// SelfHostedClientResponse is returned to `oc dashboard register`.
type SelfHostedClientResponse struct {
	ClientID string `json:"client_id"`
	Name     string `json:"name"`
}
```

Add to `backend/internal/handler/oauth_handler.go` (extend the struct and constructor with `clientSvc *service.OAuthClientService` and `orgSvc *service.OrgService`):

```go
// RegisterSelfHostedClient handles POST /api/oauth/self-hosted-client.
// Bearer-authenticated: the caller must be a logged-in user.
func (h *OAuthHandler) RegisterSelfHostedClient(c *gin.Context) {
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	uid, ok := userID.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	var req dto.SelfHostedClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, uid)
	if err != nil || len(orgs) == 0 {
		slog.Error("oauth: no org for user", "user_id", uid, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	created, err := h.clientSvc.RegisterSelfHosted(ctx, orgs[0].ID, uid, req.RedirectOrigin)
	if err != nil {
		slog.Error("oauth: self-hosted client registration failed", "user_id", uid, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	slog.Info("oauth: registered self-hosted client",
		"client_id", created.ClientID, "org_id", orgs[0].ID)

	c.JSON(http.StatusOK, dto.SelfHostedClientResponse{
		ClientID: created.ClientID,
		Name:     created.Name,
	})
}
```

Register the route in `backend/internal/server/routes/oauth.go` under the authenticated `/api` group, using the same auth middleware the other authenticated user routes use (grep `authenticated :=` in `routes/user.go` for the exact name).

- [ ] **Step 9: Run the full backend gate**

Run: `cd backend && go test ./... && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 10: Declare divergence and commit**

```bash
git add backend/ent backend/migrations/902_oauth_client.sql \
        backend/internal/service/oauth_client_service.go \
        backend/internal/service/oauth_client_service_test.go \
        backend/internal/handler/dto/oauth.go \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): oauth_client registry + self-hosted client registration"
```

---

### Task 4: Device authorization request

**Files:**
- Create: `backend/ent/schema/oauth_device_authorization.go`, `backend/migrations/903_oauth_device_authorization.sql`, `backend/internal/service/oauth_device_service.go`
- Modify: `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`
- Test: `backend/internal/service/oauth_device_service_test.go`

**Interfaces:**
- Consumes: `service.OAuthClientService.ByClientID` (Task 3).
- Produces:
  - `service.OAuthDeviceService` with `NewOAuthDeviceService(entClient *dbent.Client, portalBaseURL string) *OAuthDeviceService`
  - `(*OAuthDeviceService) RequestCode(ctx context.Context, clientID, scope string) (*DeviceCodeGrant, error)`
  - `type DeviceCodeGrant struct { DeviceCode, UserCode, VerificationURI, VerificationURIComplete string; ExpiresIn, Interval int }`
  - `(*OAuthDeviceService) Approve(ctx context.Context, userCode string, userID int64) error`
  - `(*OAuthDeviceService) Deny(ctx context.Context, userCode string) error`
  - Errors `service.ErrDeviceCodeNotFound`, `ErrDeviceCodeExpired`
  - `service.UserCodeAlphabet` (exported for the frontend test to assert against)

- [ ] **Step 1: Write the failing test**

`backend/internal/service/oauth_device_service_test.go`:

```go
package service

import (
	"context"
	"strings"
	"testing"
)

func TestRequestCodeReturnsEveryRequiredField(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	// hermes_cli/auth.py:4959 hard-fails if ANY of these is missing.
	if grant.DeviceCode == "" {
		t.Error("missing device_code")
	}
	if grant.UserCode == "" {
		t.Error("missing user_code")
	}
	if grant.VerificationURI == "" {
		t.Error("missing verification_uri")
	}
	if grant.VerificationURIComplete == "" {
		t.Error("missing verification_uri_complete")
	}
	if grant.ExpiresIn <= 0 {
		t.Error("missing/invalid expires_in")
	}
	if grant.Interval <= 0 {
		t.Error("missing/invalid interval")
	}
	if !strings.Contains(grant.VerificationURIComplete, grant.UserCode) {
		t.Errorf("verification_uri_complete %q should embed user_code %q",
			grant.VerificationURIComplete, grant.UserCode)
	}
}

func TestUserCodeAvoidsAmbiguousCharacters(t *testing.T) {
	for _, bad := range []string{"0", "O", "1", "I", "L"} {
		if strings.Contains(UserCodeAlphabet, bad) {
			t.Errorf("user code alphabet must not contain %q — it is read aloud and typed", bad)
		}
	}
}

func TestApproveMarksTheRowApproved(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	if err := svc.Approve(ctx, grant.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	row, err := svc.byDeviceCode(ctx, grant.DeviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("expected approved, got %q", row.Status)
	}
	if row.ApprovedUserID == nil || *row.ApprovedUserID != 42 {
		t.Fatalf("expected approved_user_id=42, got %v", row.ApprovedUserID)
	}
}

func TestRequestCodeRejectsUnknownClient(t *testing.T) {
	ctx := context.Background()
	client := newTestEntClient(t)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	if _, err := svc.RequestCode(ctx, "agent:does-not-exist", "inference"); err == nil {
		t.Fatal("expected an error for an unregistered client_id")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/service/ -run 'TestRequestCode|TestUserCode|TestApproveMarks' -v`
Expected: FAIL — `undefined: NewOAuthDeviceService`

- [ ] **Step 3: Write the ent schema**

`backend/ent/schema/oauth_device_authorization.go`:

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthDeviceAuthorization is one in-flight RFC 8628 device-flow request.
type OAuthDeviceAuthorization struct {
	ent.Schema
}

func (OAuthDeviceAuthorization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_device_authorizations"},
	}
}

func (OAuthDeviceAuthorization) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OAuthDeviceAuthorization) Fields() []ent.Field {
	return []ent.Field{
		field.String("device_code").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("user_code").
			MaxLen(16).
			NotEmpty().
			Unique(),
		field.String("client_id").
			MaxLen(128).
			NotEmpty(),
		field.String("scope").
			MaxLen(255).
			Default(""),
		// pending | approved | denied | expired
		field.String("status").
			MaxLen(16).
			Default("pending"),
		field.Int64("approved_user_id").
			Optional().
			Nillable(),
		field.Time("expires_at"),
		field.Time("last_polled_at").
			Optional().
			Nillable(),
	}
}

func (OAuthDeviceAuthorization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
```

- [ ] **Step 4: Run codegen**

Run: `cd backend && go generate ./ent`
Expected: `backend/ent/oauthdeviceauthorization*` generated.

- [ ] **Step 5: Write the migration**

`backend/migrations/903_oauth_device_authorization.sql`:

```sql
CREATE TABLE IF NOT EXISTS oauth_device_authorizations (
    id               BIGSERIAL PRIMARY KEY,
    device_code      VARCHAR(128) NOT NULL UNIQUE,
    user_code        VARCHAR(16)  NOT NULL UNIQUE,
    client_id        VARCHAR(128) NOT NULL,
    scope            VARCHAR(255) NOT NULL DEFAULT '',
    status           VARCHAR(16)  NOT NULL DEFAULT 'pending',
    approved_user_id BIGINT,
    expires_at       TIMESTAMPTZ  NOT NULL,
    last_polled_at   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS oauth_device_authorizations_status_idx
    ON oauth_device_authorizations (status);
CREATE INDEX IF NOT EXISTS oauth_device_authorizations_expires_at_idx
    ON oauth_device_authorizations (expires_at);
```

- [ ] **Step 6: Write the service**

`backend/internal/service/oauth_device_service.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
	"github.com/Wei-Shaw/sub2api/ent/oauthdeviceauthorization"
)

// UserCodeAlphabet deliberately omits 0/O and 1/I/L: the code is read aloud
// and typed by a human.
const UserCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const (
	deviceCodeTTL      = 15 * time.Minute
	devicePollInterval = 5
)

var (
	ErrDeviceCodeNotFound = errors.New("device code not found")
	ErrDeviceCodeExpired  = errors.New("device code expired")
)

type DeviceCodeGrant struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

type OAuthDeviceService struct {
	entClient     *dbent.Client
	portalBaseURL string
}

func NewOAuthDeviceService(entClient *dbent.Client, portalBaseURL string) *OAuthDeviceService {
	return &OAuthDeviceService{
		entClient:     entClient,
		portalBaseURL: strings.TrimRight(portalBaseURL, "/"),
	}
}

func randomUserCode() (string, error) {
	out := make([]byte, 0, 9)
	for i := 0; i < 8; i++ {
		if i == 4 {
			out = append(out, '-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(UserCodeAlphabet))))
		if err != nil {
			return "", err
		}
		out = append(out, UserCodeAlphabet[n.Int64()])
	}
	return string(out), nil
}

// RequestCode starts a device-flow authorization for a registered client.
func (s *OAuthDeviceService) RequestCode(ctx context.Context, clientID, scope string) (*DeviceCodeGrant, error) {
	if _, err := s.entClient.OAuthClient.Query().
		Where(oauthclient.ClientID(clientID)).
		Only(ctx); err != nil {
		return nil, fmt.Errorf("unknown client_id %q: %w", clientID, err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("device code entropy: %w", err)
	}
	deviceCode := hex.EncodeToString(raw)

	userCode, err := randomUserCode()
	if err != nil {
		return nil, fmt.Errorf("user code: %w", err)
	}

	if _, err := s.entClient.OAuthDeviceAuthorization.Create().
		SetDeviceCode(deviceCode).
		SetUserCode(userCode).
		SetClientID(clientID).
		SetScope(scope).
		SetStatus("pending").
		SetExpiresAt(time.Now().Add(deviceCodeTTL)).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("persist device authorization: %w", err)
	}

	verifyURI := s.portalBaseURL + "/device"
	complete := verifyURI + "?user_code=" + url.QueryEscape(userCode)

	return &DeviceCodeGrant{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verifyURI,
		VerificationURIComplete: complete,
		ExpiresIn:               int(deviceCodeTTL.Seconds()),
		Interval:                devicePollInterval,
	}, nil
}

func (s *OAuthDeviceService) byDeviceCode(ctx context.Context, deviceCode string) (*dbent.OAuthDeviceAuthorization, error) {
	row, err := s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.DeviceCode(deviceCode)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, ErrDeviceCodeNotFound
	}
	return row, err
}

func (s *OAuthDeviceService) setStatusByUserCode(ctx context.Context, userCode, status string, userID *int64) error {
	row, err := s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.UserCode(userCode)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return ErrDeviceCodeNotFound
	}
	if err != nil {
		return fmt.Errorf("query device authorization: %w", err)
	}
	if time.Now().After(row.ExpiresAt) {
		return ErrDeviceCodeExpired
	}

	upd := row.Update().SetStatus(status)
	if userID != nil {
		upd = upd.SetApprovedUserID(*userID)
	}
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("update device authorization: %w", err)
	}
	return nil
}

// Approve marks a device authorization approved by the given user.
func (s *OAuthDeviceService) Approve(ctx context.Context, userCode string, userID int64) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "approved", &userID)
}

// Deny marks a device authorization denied.
func (s *OAuthDeviceService) Deny(ctx context.Context, userCode string) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "denied", nil)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ -run 'TestRequestCode|TestUserCode|TestApproveMarks' -v`
Expected: PASS (4 tests)

- [ ] **Step 8: Add the endpoint**

Add to `backend/internal/handler/oauth_handler.go` (extend struct/constructor with `deviceSvc *service.OAuthDeviceService`):

```go
// DeviceCode handles POST /api/oauth/device/code.
// Form-encoded per RFC 8628. Unauthenticated — the client_id is the identity.
func (h *OAuthHandler) DeviceCode(c *gin.Context) {
	clientID := c.PostForm("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	scope := c.PostForm("scope")

	grant, err := h.deviceSvc.RequestCode(c.Request.Context(), clientID, scope)
	if err != nil {
		slog.Warn("oauth: device code request rejected", "client_id", clientID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client"})
		return
	}

	// Bare RFC 8628 body — NOT wrapped by internal/pkg/response.
	c.JSON(http.StatusOK, gin.H{
		"device_code":               grant.DeviceCode,
		"user_code":                 grant.UserCode,
		"verification_uri":          grant.VerificationURI,
		"verification_uri_complete": grant.VerificationURIComplete,
		"expires_in":                grant.ExpiresIn,
		"interval":                  grant.Interval,
	})
}
```

Register `POST /api/oauth/device/code` in `routes/oauth.go` on the **unauthenticated** `/api` group.

- [ ] **Step 9: Verify the wire shape by hand**

```bash
curl -s -X POST localhost:8080/api/oauth/device/code \
  -d "client_id=agent:<a real registered id>" -d "scope=inference" | python3 -m json.tool
```

Expected: a flat object with exactly the six RFC fields at the **top level**. If you see `{"code":0,"message":"success","data":{...}}`, a `response.Success` call has crept in — fix it, the client will not parse it.

- [ ] **Step 10: Run the full backend gate and commit**

Run: `cd backend && go test ./... && golangci-lint run ./...`

Declare the new files, then:

```bash
git add backend/ent backend/migrations/903_oauth_device_authorization.sql \
        backend/internal/service/oauth_device_service.go \
        backend/internal/service/oauth_device_service_test.go \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): RFC 8628 device authorization request"
```

---

### Task 5: Token endpoint — device_code and refresh_token grants

This is the task most likely to silently break the client. The error strings are a contract.

**Files:**
- Create: `backend/internal/service/oauth_token_service.go`
- Modify: `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`
- Test: `backend/internal/service/oauth_token_service_test.go`

**Interfaces:**
- Consumes: `OAuthKeyService.Active` (Task 2), `OAuthDeviceService` (Task 4).
- Produces:
  - `service.OAuthTokenService` with `NewOAuthTokenService(entClient *dbent.Client, keySvc *OAuthKeyService, deviceSvc *OAuthDeviceService, issuer string) *OAuthTokenService`
  - `(*OAuthTokenService) ExchangeDeviceCode(ctx context.Context, clientID, deviceCode string) (*OAuthTokens, error)`
  - `type OAuthTokens struct { AccessToken, RefreshToken, Scope string; ExpiresIn int }`
  - Sentinel errors `service.ErrAuthorizationPending`, `ErrSlowDown`, `ErrAccessDenied`, `ErrExpiredToken` — each maps to the RFC error string of the same name in snake_case.

- [ ] **Step 1: Write the failing test**

`backend/internal/service/oauth_token_service_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func newDeviceFlowFixture(t *testing.T) (context.Context, *OAuthTokenService, *OAuthDeviceService, string, string) {
	t.Helper()
	ctx := context.Background()
	client := newTestEntClient(t)
	clients := NewOAuthClientService(client)
	keys := NewOAuthKeyService(client)
	devices := NewOAuthDeviceService(client, "https://portal.example.com")
	tokens := NewOAuthTokenService(client, keys, devices, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := devices.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	return ctx, tokens, devices, oc.ClientID, grant.DeviceCode
}

func TestExchangeReturnsAuthorizationPendingBeforeApproval(t *testing.T) {
	ctx, tokens, _, clientID, deviceCode := newDeviceFlowFixture(t)

	_, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("expected ErrAuthorizationPending, got %v", err)
	}
}

func TestExchangeReturnsSignedES256TokenAfterApproval(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}

	parsed, _, err := jwt.NewParser().ParseUnverified(got.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if parsed.Method.Alg() != "ES256" {
		t.Fatalf("expected ES256, got %s", parsed.Method.Alg())
	}
	if parsed.Header["kid"] == nil || parsed.Header["kid"] == "" {
		t.Fatal("access token header must carry a kid")
	}
}

func TestDeviceCodeIsSingleUse(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); err == nil {
		t.Fatal("a device code must not be redeemable twice")
	}
}

func TestExchangeRejectsMismatchedClient(t *testing.T) {
	ctx, tokens, devices, _, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if _, err := tokens.ExchangeDeviceCode(ctx, "agent:someone-else", deviceCode); err == nil {
		t.Fatal("a device code must only be redeemable by the client that requested it")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/service/ -run 'TestExchange|TestDeviceCodeIsSingleUse' -v`
Expected: FAIL — `undefined: NewOAuthTokenService`

- [ ] **Step 3: Write the token service**

`backend/internal/service/oauth_token_service.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"

	"github.com/golang-jwt/jwt/v5"
)

// RFC 8628 §3.5 error codes. The hermes client branches on these exact
// strings (hermes_cli/auth.py:5000) — do not reword them.
var (
	ErrAuthorizationPending = errors.New("authorization_pending")
	ErrSlowDown             = errors.New("slow_down")
	ErrAccessDenied         = errors.New("access_denied")
	ErrExpiredToken         = errors.New("expired_token")
)

const oauthAccessTokenTTL = 15 * time.Minute

type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	ExpiresIn    int
}

type OAuthTokenService struct {
	entClient *dbent.Client
	keySvc    *OAuthKeyService
	deviceSvc *OAuthDeviceService
	issuer    string
}

func NewOAuthTokenService(entClient *dbent.Client, keySvc *OAuthKeyService, deviceSvc *OAuthDeviceService, issuer string) *OAuthTokenService {
	return &OAuthTokenService{
		entClient: entClient,
		keySvc:    keySvc,
		deviceSvc: deviceSvc,
		issuer:    issuer,
	}
}

// mintAccessToken signs an ES256 JWT whose audience is the client_id, so an
// agent can verify a token was minted for it and no other instance.
func (s *OAuthTokenService) mintAccessToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	key, err := s.keySvc.Active(ctx)
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   s.issuer,
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   clientID,
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   now.Add(oauthAccessTokenTTL).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = key.Kid

	signed, err := tok.SignedString(key.Private)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func newOpaqueToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// ExchangeDeviceCode implements the device_code grant.
func (s *OAuthTokenService) ExchangeDeviceCode(ctx context.Context, clientID, deviceCode string) (*OAuthTokens, error) {
	row, err := s.deviceSvc.byDeviceCode(ctx, deviceCode)
	if err != nil {
		return nil, err
	}

	// Bind the code to the client that requested it.
	if row.ClientID != clientID {
		return nil, ErrAccessDenied
	}

	if time.Now().After(row.ExpiresAt) {
		return nil, ErrExpiredToken
	}

	switch row.Status {
	case "pending":
		// Rate-limit the poll loop: a client polling faster than the advertised
		// interval gets slow_down rather than a free retry.
		if row.LastPolledAt != nil && time.Since(*row.LastPolledAt) < time.Duration(devicePollInterval)*time.Second {
			return nil, ErrSlowDown
		}
		if _, uerr := row.Update().SetLastPolledAt(time.Now()).Save(ctx); uerr != nil {
			return nil, fmt.Errorf("update poll timestamp: %w", uerr)
		}
		return nil, ErrAuthorizationPending
	case "denied":
		return nil, ErrAccessDenied
	case "approved":
		// fall through
	default:
		return nil, ErrExpiredToken
	}

	if row.ApprovedUserID == nil {
		return nil, ErrAccessDenied
	}
	userID := *row.ApprovedUserID

	access, err := s.mintAccessToken(ctx, userID, clientID, row.Scope)
	if err != nil {
		return nil, err
	}
	refresh, err := newOpaqueToken()
	if err != nil {
		return nil, fmt.Errorf("refresh token entropy: %w", err)
	}

	// Single use: consume the row so a replayed device_code cannot mint a
	// second token family.
	if _, err := row.Update().SetStatus("expired").Save(ctx); err != nil {
		return nil, fmt.Errorf("consume device code: %w", err)
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		Scope:        row.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}
```

> **Refresh-token persistence:** this task mints an opaque refresh token but does not yet store it. Wiring it into Inferno's existing refresh-token family store (`RefreshTokenCache`, `ErrRefreshTokenReused`) is Step 6 below — do not invent a parallel store.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/service/ -run 'TestExchange|TestDeviceCodeIsSingleUse' -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Add the token endpoint**

Add to `backend/internal/handler/oauth_handler.go` (extend struct/constructor with `tokenSvc *service.OAuthTokenService`):

```go
// Token handles POST /api/oauth/token.
func (h *OAuthHandler) Token(c *gin.Context) {
	grantType := c.PostForm("grant_type")
	clientID := c.PostForm("client_id")

	switch grantType {
	case "urn:ietf:params:oauth:grant-type:device_code":
		tokens, err := h.tokenSvc.ExchangeDeviceCode(c.Request.Context(), clientID, c.PostForm("device_code"))
		if err != nil {
			// RFC 8628 §3.5: pending/slow_down are 400s with an error code the
			// client polls on. Anything else is terminal.
			switch {
			case errors.Is(err, service.ErrAuthorizationPending):
				c.JSON(http.StatusBadRequest, gin.H{"error": "authorization_pending"})
			case errors.Is(err, service.ErrSlowDown):
				c.JSON(http.StatusBadRequest, gin.H{"error": "slow_down"})
			case errors.Is(err, service.ErrAccessDenied):
				c.JSON(http.StatusBadRequest, gin.H{"error": "access_denied"})
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "expired_token"})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"token_type":    "Bearer",
			"expires_in":    tokens.ExpiresIn,
			"scope":         tokens.Scope,
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
	}
}
```

Register `POST /api/oauth/token` on the unauthenticated `/api` group.

- [ ] **Step 6: Persist refresh tokens through the existing family store**

Read `backend/internal/service/auth_service.go` around `GenerateTokenPair` and `RefreshToken` to learn the family-store interface, then replace `newOpaqueToken()` in `ExchangeDeviceCode` with a call that registers the token in that same store under a new family. Add the `refresh_token` grant to `Token`, routing through the existing rotation path so `ErrRefreshTokenReused` still fires.

Add a test asserting that replaying a rotated refresh token fails:

```go
func TestRefreshTokenReuseIsRejected(t *testing.T) {
	// Mint a pair, rotate it once, then replay the ORIGINAL refresh token.
	// Expect an error — reuse must invalidate the family, not just the token.
	t.Skip("implement against the family store interface found in auth_service.go")
}
```

Remove the `t.Skip` once the store interface is wired; a skipped test is not a passing test.

- [ ] **Step 7: Run the full backend gate and commit**

Run: `cd backend && go test ./... && golangci-lint run ./...`

```bash
git add backend/internal/service/oauth_token_service.go \
        backend/internal/service/oauth_token_service_test.go \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): token endpoint with device_code and refresh_token grants"
```

---

### Task 6: Scope enforcement middleware + /api/oauth/account

**Files:**
- Create: `backend/internal/server/middleware/oauth_scope.go`
- Modify: `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`
- Test: `backend/internal/server/middleware/oauth_scope_test.go`

**Interfaces:**
- Consumes: `OAuthKeyService.Active` (Task 2).
- Produces:
  - `middleware.RequireOAuthScope(keySvc *service.OAuthKeyService, scope string) gin.HandlerFunc`
  - Sets `oauth_user_id` (int64), `oauth_client_id` (string), `oauth_scope` (string) on the Gin context.

Scopes: `inference`, `billing:read`, `billing:manage`, `agents:read`, `agents:manage`. `billing:manage` is **not** granted at initial login — the desktop re-runs the device flow to elevate.

- [ ] **Step 1: Write the failing test**

`backend/internal/server/middleware/oauth_scope_test.go`:

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireOAuthScopeRejectsMissingBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(nil, "billing:manage"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a bearer, got %d", w.Code)
	}
}

func TestRequireOAuthScopeRejectsUnsignedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(nil, "billing:manage"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	// alg=none — must never be accepted.
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unsigned token, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/server/middleware/ -run TestRequireOAuthScope -v`
Expected: FAIL — `undefined: RequireOAuthScope`

- [ ] **Step 3: Write the middleware**

`backend/internal/server/middleware/oauth_scope.go`:

```go
package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// RequireOAuthScope validates an OAuth-issued ES256 bearer and enforces a scope.
//
// It accepts ES256 ONLY. Inferno's HMAC session tokens are a different token
// type on a different code path and must never validate here — accepting both
// would let a session cookie's secret mint API credentials.
func RequireOAuthScope(keySvc *service.OAuthKeyService, required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if raw == "" || keySvc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		key, err := keySvc.Active(c.Request.Context())
		if err != nil {
			slog.Error("oauth: cannot load signing key", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		claims := jwt.MapClaims{}
		parser := jwt.NewParser(jwt.WithValidMethods([]string{"ES256"}))
		if _, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			return &key.Private.PublicKey, nil
		}); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		granted, _ := claims["scope"].(string)
		if !scopeSatisfies(granted, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_scope"})
			return
		}

		sub, _ := claims["sub"].(string)
		uid, err := strconv.ParseInt(sub, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		aud, _ := claims["aud"].(string)

		c.Set("oauth_user_id", uid)
		c.Set("oauth_client_id", aud)
		c.Set("oauth_scope", granted)
		c.Next()
	}
}

func scopeSatisfies(granted, required string) bool {
	if required == "" {
		return true
	}
	for _, s := range strings.Fields(granted) {
		if s == required {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/server/middleware/ -run TestRequireOAuthScope -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Add the account endpoint**

Add to `backend/internal/handler/oauth_handler.go`:

```go
// Account handles GET /api/oauth/account — consumed by hermes_cli/nous_account.py.
func (h *OAuthHandler) Account(c *gin.Context) {
	uid, ok := c.Get("oauth_user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	userID, ok := uid.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("oauth: account org lookup failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	out := make([]gin.H, 0, len(orgs))
	for _, o := range orgs {
		role, rerr := h.orgSvc.RoleIn(ctx, o.ID, userID)
		if rerr != nil {
			role = service.RoleMember
		}
		out = append(out, gin.H{
			"id":         strconv.FormatInt(o.ID, 10),
			"slug":       o.Slug,
			"name":       o.Name,
			"isPersonal": o.IsPersonal,
			"role":       role,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": strconv.FormatInt(userID, 10),
		"orgs":    out,
	})
}
```

Register `GET /api/oauth/account` behind `RequireOAuthScope(keySvc, "")`.

- [ ] **Step 6: Run the full backend gate and commit**

Run: `cd backend && go test ./... && golangci-lint run ./...`

```bash
git add backend/internal/server/middleware/oauth_scope.go \
        backend/internal/server/middleware/oauth_scope_test.go \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): scope enforcement middleware + account endpoint"
```

---

### Task 7: Device approval screen

**Files:**
- Create: `inferno-frontend/src/views/oauth/DeviceApprovalView.vue`, `inferno-frontend/src/views/oauth/__tests__/DeviceApprovalView.spec.ts`
- Modify: `inferno-frontend/src/router/index.ts`, `backend/internal/handler/oauth_handler.go`, `backend/internal/server/routes/oauth.go`

**Interfaces:**
- Consumes: `OAuthDeviceService.Approve/Deny` (Task 4).
- Produces: `POST /api/oauth/device/approve` and `POST /api/oauth/device/deny`, both authenticated by Inferno's normal session middleware (this is a browser screen, not an agent call), body `{ "user_code": "ABCD-EFGH" }`.

This is a **new** file, so june-lint holds it to all ten ground rules from the start. Read `inferno-frontend/README.md` for the rules and copy the structure of an existing converted view before writing it.

- [ ] **Step 1: Add the approve/deny endpoints**

In `oauth_handler.go`:

```go
type deviceDecisionRequest struct {
	UserCode string `json:"user_code" binding:"required"`
}

// ApproveDevice handles POST /api/oauth/device/approve (browser session auth).
func (h *OAuthHandler) ApproveDevice(c *gin.Context) {
	uid, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := uid.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req deviceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	switch err := h.deviceSvc.Approve(c.Request.Context(), req.UserCode, userID); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"status": "approved"})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
	case errors.Is(err, service.ErrDeviceCodeExpired):
		c.JSON(http.StatusGone, gin.H{"error": "expired"})
	default:
		slog.Error("oauth: device approval failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
	}
}
```

Write `DenyDevice` identically, calling `h.deviceSvc.Deny(ctx, req.UserCode)` and returning `{"status":"denied"}`. Register both on the session-authenticated `/api` group.

- [ ] **Step 2: Write the failing component test**

`inferno-frontend/src/views/oauth/__tests__/DeviceApprovalView.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import DeviceApprovalView from '../DeviceApprovalView.vue'

const approve = vi.fn()
vi.mock('@/api/oauth', () => ({
  approveDevice: (code: string) => approve(code),
  denyDevice: vi.fn(),
}))

describe('DeviceApprovalView', () => {
  beforeEach(() => approve.mockReset())

  it('prefills the code from the user_code query parameter', () => {
    const wrapper = mount(DeviceApprovalView, {
      global: { mocks: { $route: { query: { user_code: 'ABCD-EFGH' } } } },
    })
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('ABCD-EFGH')
  })

  it('submits the entered code on approve', async () => {
    const wrapper = mount(DeviceApprovalView, {
      global: { mocks: { $route: { query: {} } } },
    })
    await wrapper.find('input').setValue('WXYZ-2345')
    await wrapper.find('[data-test="approve"]').trigger('click')
    expect(approve).toHaveBeenCalledWith('WXYZ-2345')
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd inferno-frontend && npx vitest run src/views/oauth`
Expected: FAIL — cannot resolve `../DeviceApprovalView.vue`

- [ ] **Step 4: Write the view and the API client**

Create `inferno-frontend/src/api/oauth.ts` exporting `approveDevice(userCode: string)` and `denyDevice(userCode: string)`, POSTing to `/api/oauth/device/approve` and `/api/oauth/device/deny` through the existing API client (copy the import and call style from a neighbouring file in `src/api/`).

Create `DeviceApprovalView.vue` with: a heading, an input prefilled from `route.query.user_code`, an Approve button (`data-test="approve"`), a Deny button (`data-test="deny"`), and success/error states. **Scoped CSS on June tokens only — no `bg-gray-*` utilities**, or june-lint will reject the file.

- [ ] **Step 5: Add the route**

In `inferno-frontend/src/router/index.ts`, add a `/device` route rendering `DeviceApprovalView`, requiring auth (an unauthenticated visitor must be sent to login and returned here afterwards — the device flow sends users to this URL before they may have a session).

The path must be `/device` — `OAuthDeviceService.RequestCode` builds `verification_uri` as `{portal}/device`.

- [ ] **Step 6: Run the frontend gate**

Run:
```bash
cd inferno-frontend
node scripts/june-lint.mjs
npx vue-tsc --noEmit -p tsconfig.json
npx vitest run
```
Expected: june-lint `clean across N files`; typecheck silent; all tests pass and the count has not dropped.

- [ ] **Step 7: Commit**

```bash
git add inferno-frontend/src/views/oauth inferno-frontend/src/api/oauth.ts \
        inferno-frontend/src/router/index.ts \
        backend/internal/handler/oauth_handler.go \
        backend/internal/server/routes/oauth.go \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "feat(oauth): device approval screen"
```

---

### Task 8: End-to-end conformance against the real hermes client

The highest-value verification in the plan: the client is the spec.

**Files:**
- Create: `backend/scripts/oauth-conformance.md` (the runbook)

- [ ] **Step 1: Register a client and start a device login**

With Inferno running locally on `:8080`:

```bash
export HERMES_PORTAL_BASE_URL=http://localhost:8080
cd /Users/saksham/OpenComputerV2/OpenComputerV2
uv run oc setup
```

Expected: the CLI prints a `user_code` and a verification URL, then polls. It must **not** raise `Device code response missing fields`.

- [ ] **Step 2: Approve in the browser**

Open the printed `verification_uri_complete`, confirm the code is prefilled, click Approve.

Expected: the CLI's poll loop returns and writes `~/.hermes/auth.json` with an `access_token`.

- [ ] **Step 3: Verify the stored token**

```bash
python3 -c "
import json,base64,pathlib
s=json.load(open(pathlib.Path.home()/'.hermes'/'auth.json'))
tok=s['providers']['nous']['access_token']
h=json.loads(base64.urlsafe_b64decode(tok.split('.')[0]+'=='))
print(h)
assert h['alg']=='ES256', h
assert h.get('kid'), 'missing kid'
print('OK')
"
```

Expected: `OK`. (If the provider key is not `nous`, print `s['providers'].keys()` and use the right one — sub-project #3 renames this.)

- [ ] **Step 4: Verify JWKS verification works offline**

Fetch `http://localhost:8080/.well-known/jwks.json` and verify the token signature against it with any JWT library. Expected: signature valid, `kid` matches.

- [ ] **Step 5: Verify the poll loop honours slow_down**

Re-run `oc setup`, and poll the token endpoint manually twice within one second:

```bash
curl -s -X POST localhost:8080/api/oauth/token \
  -d "grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  -d "client_id=<id>" -d "device_code=<code>"
```

Expected: first call `{"error":"authorization_pending"}`, immediate second call `{"error":"slow_down"}`.

- [ ] **Step 6: Write the runbook and commit**

Record every command above and its expected output in `backend/scripts/oauth-conformance.md` so the next person re-runs it without rediscovering it.

```bash
git add backend/scripts/oauth-conformance.md \
        inferno-frontend/scripts/check-divergence.sh GOAL.md
git commit -m "docs(oauth): end-to-end conformance runbook"
```

---

## Deferred to later sub-projects

Not in this plan, by design:

- `authorization_code` + PKCE + `GET /oauth/authorize` with org auto-approve — needed only for the desktop's gateway-brokered login, which needs a registered gateway first (**#1/#2**).
- The `agents` registry and `GET /api/agents` — **#2**.
- Cron-fire JWT signing — **#2** (depends on this plan's JWKS).
- `/api/billing/*` contract adapter — tracked separately.
- `instance_id` linkage and the pending-client reconcile sweep — **#1** (columns exist from Task 3).

## Self-review notes

- Spec coverage: D-1 → Task 1. ES256/JWKS → Task 2. `oauth_client` + self-hosted registration → Task 3. Device flow → Tasks 4–5. Scopes + account → Task 6. Approval page → Task 7. Conformance → Task 8. **Not covered here, deliberately:** `authorization_code`/`/oauth/authorize`, which the spec lists at implementation-order step 5 — moved to #1/#2 because it cannot be tested without a registered gateway.
- Type consistency: `newTestEntClient` is assumed to exist in the `service` package; Task 1 Step 1 tells the implementer to find the real helper name rather than create one.
- Known gap by design: Task 5 Step 6 requires reading `auth_service.go` to discover the family-store interface. It cannot be written blind, and inventing a second refresh store would be worse than one guided step.
