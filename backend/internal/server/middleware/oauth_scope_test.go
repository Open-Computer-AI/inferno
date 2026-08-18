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

// newOAuthScopeTestKeyService returns a real OAuthKeyService (backed by a
// throwaway sqlite ent client) so the tests below exercise actual RS256
// signature verification against the same code path RequireOAuthScope uses
// in production, rather than a stub.
func newOAuthScopeTestKeyService(t *testing.T) *service.OAuthKeyService {
	t.Helper()
	return service.NewOAuthKeyService(newOAuthScopeTestEntClient(t))
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
		"iss":   "https://portal.example.com",
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   clientID,
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   exp.Unix(),
	}
}

func newOAuthScopeTestRouter(keySvc *service.OAuthKeyService, required string) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireOAuthScope(keySvc, required), func(c *gin.Context) {
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
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference billing:read", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "billing:read")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Contains(t, w.Body.String(), `"oauth_user_id":42`)
	require.Contains(t, w.Body.String(), `"oauth_client_id":"agent:abc123"`)
	require.Contains(t, w.Body.String(), `"oauth_scope":"inference billing:read"`)
}

func TestRequireOAuthScopeRejectsInsufficientScope(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	// Granted "billing:manage_nothing" and bare "billing" — neither must
	// satisfy a "billing:manage" requirement. A strings.Contains-based
	// implementation would wrongly accept the first of these.
	claims := baseTestClaims(42, "agent:abc123", "inference billing:manage_nothing billing", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "billing:manage")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsNoScopeClaimAgainstNonEmptyRequirement(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := jwt.MapClaims{
		"iss": "https://portal.example.com",
		"sub": "42",
		"aud": "agent:abc123",
		// no "scope" claim at all.
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(time.Hour))

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("some-hmac-secret-that-is-not-the-rs256-key"))
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(time.Hour))

	signed := mintTestES256Token(t, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	keySvc := newOAuthScopeTestKeyService(t)
	key, err := keySvc.Active(context.Background())
	require.NoError(t, err)

	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// Deliberately no tok.Header["kid"] set.
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	keySvc := newOAuthScopeTestKeyService(t)
	key, err := keySvc.Active(context.Background())
	require.NoError(t, err)

	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "not-a-real-kid"
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	entClient := newOAuthScopeTestEntClient(t)
	keySvc := service.NewOAuthKeyService(entClient)

	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(time.Hour))
	tok := mintTestRS256Token(t, keySvc, claims)

	require.NoError(t, entClient.Close())

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
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
	keySvc := newOAuthScopeTestKeyService(t)
	claims := jwt.MapClaims{
		"iss":   "https://portal.example.com",
		"sub":   "42",
		"aud":   "agent:abc123",
		"scope": "inference:invoke",
		"iat":   time.Now().Unix(),
		// no "exp" claim at all.
	}
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsExpiredToken(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference:invoke", time.Now().Add(-time.Hour))
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsUnparsableSubject(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := jwt.MapClaims{
		"iss":   "https://portal.example.com",
		"sub":   "not-a-number",
		"aud":   "agent:abc123",
		"scope": "inference:invoke",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	tok := mintTestRS256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference:invoke")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}
