package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestRequireOAuthScopeRejectsMissingBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(nil, nil, testIssuer, "billing:manage"), func(c *gin.Context) {
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
	r.GET("/x", RequireOAuthScope(nil, nil, testIssuer, "billing:manage"), func(c *gin.Context) {
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

// newOAuthScopeTestEntClient is this package's equivalent of
// internal/handler/oauth_handler_test.go's newOAuthHandlerTestEntClient —
// that helper isn't reusable here (unexported, different package), so this
// mirrors its shape exactly: an in-memory sqlite ent client, one per test.
func newOAuthScopeTestEntClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// testIssuer is the issuer these tests mint with and verify against. It is
// the same string baseTestClaims stamps into `iss`, so a middleware that
// stopped checking the issuer could not hide behind a mismatch.
const testIssuer = "https://portal.example.com"

// testAudienceClientID is the client_id every token below is minted for.
// Registered as an ACTIVE oauth_client by newOAuthScopeTestEnv, because the
// middleware now resolves a token's audience against the registry.
const testAudienceClientID = "agent:abc123"

// oauthScopeTestEnv is the resource server's dependencies over ONE sqlite
// ent client: the real key service (so these tests exercise actual RS256
// verification, not a stub) and the real client registry (so the audience
// check runs against real rows).
type oauthScopeTestEnv struct {
	entClient *dbent.Client
	keySvc    *service.OAuthKeyService
	clientSvc *service.OAuthClientService
}

func newOAuthScopeTestEnv(t *testing.T) *oauthScopeTestEnv {
	t.Helper()
	entClient := newOAuthScopeTestEntClient(t)
	registerOAuthScopeTestClient(t, entClient, testAudienceClientID, service.ClientActive)
	return &oauthScopeTestEnv{
		entClient: entClient,
		keySvc:    service.NewOAuthKeyService(entClient),
		clientSvc: service.NewOAuthClientService(entClient),
	}
}

// registerOAuthScopeTestClient inserts an oauth_client row directly rather
// than through RegisterSelfHosted, which mints a random client_id — these
// tests need a KNOWN client_id so the minted `aud` and the registry agree.
func registerOAuthScopeTestClient(t *testing.T, entClient *dbent.Client, clientID, status string) {
	t.Helper()
	_, err := entClient.OAuthClient.Create().
		SetClientID(clientID).
		SetKind("SELF_HOSTED").
		SetName("test client").
		SetOwnerUserID(42).
		SetOrgID(1).
		SetRedirectURIOrigin("https://agent.example.com").
		SetStatus(status).
		Save(context.Background())
	require.NoError(t, err)
}

// mintTestRS256Token signs claims with keySvc's active RS256 key, exactly as
// OAuthTokenService.mintAccessToken does (same header/claims shape), so
// these tests are a faithful resource-server-side check against the actual
// token minting, not a hand-rolled approximation of it.
func mintTestRS256Token(t *testing.T, keySvc *service.OAuthKeyService, claims jwt.MapClaims) string {
	t.Helper()
	key, err := keySvc.Active(context.Background())
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)
	return signed
}

// mintTestES256Token signs claims with an independently generated ES256
// keypair (never keySvc's own key — ES256/RS256 use unrelated key shapes so
// there's no shared key to reuse), with header/claims shaped exactly like a
// real access token. This is stand-in for a stale, pre-migration token: the
// algorithm this server used to sign with, and exactly what
// TestRequireOAuthScopeRejectsES256SignedToken exercises.
func mintTestES256Token(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = "stale-es256-kid"
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

func baseTestClaims(userID int64, clientID, scope string, exp time.Time) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":   testIssuer,
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   clientID,
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   exp.Unix(),
	}
}

func newOAuthScopeTestRouter(env *oauthScopeTestEnv, required string) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(env.keySvc, env.clientSvc, testIssuer, required), func(c *gin.Context) {
		uid, _ := c.Get(OAuthContextKeyUserID)
		clientID, _ := c.Get(OAuthContextKeyClientID)
		scope, _ := c.Get(OAuthContextKeyScope)
		c.JSON(http.StatusOK, gin.H{
			"oauth_user_id":   uid,
			"oauth_client_id": clientID,
			"oauth_scope":     scope,
		})
	})
	return r, httptest.NewRecorder()
}

func TestRequireOAuthScopeAcceptsValidTokenWithSufficientScope(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference billing:read", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "billing:read")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Contains(t, w.Body.String(), `"oauth_user_id":42`)
	require.Contains(t, w.Body.String(), `"oauth_client_id":"agent:abc123"`)
	require.Contains(t, w.Body.String(), `"oauth_scope":"inference billing:read"`)
}

func TestRequireOAuthScopeRejectsInsufficientScope(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	// Granted "billing:manage_nothing" and bare "billing" — neither must
	// satisfy a "billing:manage" requirement. A strings.Contains-based
	// implementation would wrongly accept the first of these.
	claims := baseTestClaims(42, testAudienceClientID, "inference billing:manage_nothing billing", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "billing:manage")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsNoScopeClaimAgainstNonEmptyRequirement(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := jwt.MapClaims{
		"iss": testIssuer,
		"sub": "42",
		"aud": testAudienceClientID,
		// no "scope" claim at all.
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsHMACSignedToken is the security property that
// matters most here: Inferno's panel-session tokens are HMAC-signed
// (jwt_auth.go / AuthService.ValidateToken) with a symmetric secret shared
// by every server process. If RequireOAuthScope accepted HMAC in addition
// to RS256, anyone holding that shared secret — which is far more widely
// held than the OAuth RS256 private key — could mint their own "OAuth"
// bearer tokens. This must fail even though every claim in the token is
// otherwise perfectly valid (correct sub, aud, scope, and a future exp).
func TestRequireOAuthScopeRejectsHMACSignedToken(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("some-hmac-secret-that-is-not-the-rs256-key"))
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsES256SignedToken is the migration-specific
// companion to the HMAC test above: ES256 is the algorithm this server used
// to sign OAuth access tokens with before the RS256 migration (Task 1 of the
// authorization_code + PKCE plan), so a stale, pre-migration token is
// exactly ES256-shaped — a real key, a real signature, a plausible kid,
// every other claim valid. Nothing else in this suite would catch a
// middleware that still accepted it: the HMAC test only proves symmetric
// signatures are rejected, not that the specific algorithm being retired is.
func TestRequireOAuthScopeRejectsES256SignedToken(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))

	signed := mintTestES256Token(t, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsTokenWithNoKidHeader is the regression test
// for ByKid-only dispatch: a token with no kid header must be rejected
// outright, never silently resolved to whatever the current active key
// happens to be. The signature itself is entirely genuine — signed with the
// real active RS256 key and otherwise-valid claims — so this fails only
// because the kid header is missing, not for any other reason.
func TestRequireOAuthScopeRejectsTokenWithNoKidHeader(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	key, err := env.keySvc.Active(context.Background())
	require.NoError(t, err)

	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// Deliberately no tok.Header["kid"] set.
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsTokenWithUnknownKid is ByKid-dispatch's other
// half: a syntactically well-formed kid that just doesn't match any signing
// key this server knows about (service.ErrUnknownKid) must be rejected the
// same as a missing one, not treated as some other kind of error.
func TestRequireOAuthScopeRejectsTokenWithUnknownKid(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	key, err := env.keySvc.Active(context.Background())
	require.NoError(t, err)

	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "not-a-real-kid"
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeReturns500OnKeyStoreFailure is the regression test
// for the review finding that folding key-resolution failures into the same
// 401 as a genuinely bad token would make a Postgres outage indistinguishable,
// in logs and metrics, from "every client's credential is dead". A real,
// validly-signed token (minted while the store was healthy) is presented
// AFTER the underlying ent client is closed — ByKid does not cache, so it
// re-queries on every call and this closed connection is exactly the shape
// of failure a real DB outage would produce, not a hand-rolled stub. This
// must come back as 500 server_error, not 401 invalid_token.
func TestRequireOAuthScopeReturns500OnKeyStoreFailure(t *testing.T) {
	env := newOAuthScopeTestEnv(t)

	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	require.NoError(t, env.entClient.Close())

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.JSONEq(t, `{"error":"server_error"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsTokenWithNoExpClaim is the fix for the review
// finding: golang-jwt/v5 only validates exp when the claim is PRESENT
// (validator.go's verifyExpiresAt defaults required=false), so without
// jwt.WithExpirationRequired() a validly RS256-signed token that simply
// omits exp would be accepted as non-expiring forever. Signed with the real
// active RS256 key and otherwise-valid claims — only exp is missing — so
// this fails for the right reason (the missing-exp check) and not because
// the signature or any other claim is wrong.
func TestRequireOAuthScopeRejectsTokenWithNoExpClaim(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := jwt.MapClaims{
		"iss":   testIssuer,
		"sub":   "42",
		"aud":   testAudienceClientID,
		"scope": "inference:invoke",
		"iat":   time.Now().Unix(),
		// no "exp" claim at all.
	}
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsExpiredToken(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(-time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsUnparsableSubject(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := jwt.MapClaims{
		"iss":   testIssuer,
		"sub":   "not-a-number",
		"aud":   testAudienceClientID,
		"scope": "inference:invoke",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// ---------------------------------------------------------------------------
// M-4 (issuer + audience verification) and M-3 (case-insensitive scheme).
// ---------------------------------------------------------------------------

// TestRequireOAuthScopeRejectsForeignIssuer is M-4's issuer half.
//
// Task 6 spent a whole task getting `iss` right on the minting side, but
// nothing on the server ever read it back: the claim was asserted by the
// client and checked by nobody. Not exploitable today — the kid binds a
// token to this deployment's own signing key — but it is a property this
// branch introduced, and it must be enforced before a second key or a
// second issuer exists rather than after.
func TestRequireOAuthScopeRejectsForeignIssuer(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	claims["iss"] = "https://someone-elses-portal.example.com"
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsMissingIssuer proves the claim is REQUIRED,
// not merely matched-when-present. jwt.WithIssuer passes required=true, so a
// token minted before Task 6 with iss: "" (or with no iss at all) is
// rejected rather than accepted as unattributed.
func TestRequireOAuthScopeRejectsMissingIssuer(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	delete(claims, "iss")
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestRequireOAuthScopeRejectsMissingAudience is M-4's audience half.
//
// `aud` is not decoration here: this middleware publishes it as
// OAuthContextKeyClientID — the CALLER'S IDENTITY, which handlers act on.
// The bare type assertion it used to be read with turned a missing or
// array-valued claim into "", and every handler downstream was handed an
// empty client identity as though that were a fact.
func TestRequireOAuthScopeRejectsMissingAudience(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	delete(claims, "aud")
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsUnregisteredAudience: an audience naming a
// client this server never registered is not an identity, and must not be
// published as one.
func TestRequireOAuthScopeRejectsUnregisteredAudience(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, "agent:never-registered", "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsRevokedAudience is what makes the audience
// check load-bearing rather than cosmetic: revoking a client used to leave
// its already-minted access tokens good for the rest of their 15 minutes,
// because nothing on the resource-server side consulted the registry. Now
// the kill switch takes effect at the next request.
func TestRequireOAuthScopeRejectsRevokedAudience(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	_, err := env.entClient.OAuthClient.Update().
		Where(oauthclient.ClientID(testAudienceClientID)).
		SetStatus(service.ClientRevoked).
		Save(context.Background())
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(env, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRefusesEverythingWithoutAnIssuer: an unset issuer is
// a server misconfiguration, and the check must not silently vanish when the
// configuration it depends on is missing. Fails closed and 500s, mirroring
// ErrIssuerNotConfigured on the minting side.
func TestRequireOAuthScopeRefusesEverythingWithoutAnIssuer(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(env.keySvc, env.clientSvc, "", "inference:invoke"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.JSONEq(t, `{"error":"server_error"}`, w.Body.String())
}

// TestRequireOAuthScopeAcceptsLowercaseBearerScheme is M-3. RFC 6750 §2.1 /
// RFC 7235 §2.1 make the auth-scheme token case-insensitive, so `bearer
// <tok>` is a well-formed credential. The real client sends "Bearer", so
// this is interop hygiene — but a resource server that rejects a
// spec-conformant header is wrong regardless of who is calling it today.
func TestRequireOAuthScopeAcceptsLowercaseBearerScheme(t *testing.T) {
	for _, scheme := range []string{"bearer", "BEARER", "BeArEr"} {
		t.Run(scheme, func(t *testing.T) {
			env := newOAuthScopeTestEnv(t)
			claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
			tok := mintTestRS256Token(t, env.keySvc, claims)

			r, w := newOAuthScopeTestRouter(env, "inference:invoke")
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", scheme+" "+tok)
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		})
	}
}

// TestRequireOAuthScopeRejectsANonBearerScheme is the other half of M-3: a
// case-insensitive comparison must not become a prefix-blind one.
func TestRequireOAuthScopeRejectsANonBearerScheme(t *testing.T) {
	env := newOAuthScopeTestEnv(t)
	claims := baseTestClaims(42, testAudienceClientID, "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, env.keySvc, claims)

	for _, header := range []string{"Basic " + tok, "Bearer" + tok, tok} {
		r, w := newOAuthScopeTestRouter(env, "inference:invoke")
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", header)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code, "header=%q", header[:min(12, len(header))])
	}
}
