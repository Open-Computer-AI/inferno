package middleware

import (
	"context"
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
// throwaway sqlite ent client) so the tests below exercise actual ES256
// signature verification against the same code path RequireOAuthScope uses
// in production, rather than a stub.
func newOAuthScopeTestKeyService(t *testing.T) *service.OAuthKeyService {
	t.Helper()
	return service.NewOAuthKeyService(newOAuthScopeTestEntClient(t))
}

// mintTestES256Token signs claims with keySvc's active ES256 key, exactly as
// OAuthTokenService.mintAccessToken does (same header/claims shape), so
// these tests are a faithful resource-server-side check against Task 5's
// actual token minting, not a hand-rolled approximation of it.
func mintTestES256Token(t *testing.T, keySvc *service.OAuthKeyService, claims jwt.MapClaims) string {
	t.Helper()
	key, err := keySvc.Active(context.Background())
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
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
	tok := mintTestES256Token(t, keySvc, claims)

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
	tok := mintTestES256Token(t, keySvc, claims)

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
	tok := mintTestES256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference")
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
// to ES256, anyone holding that shared secret — which is far more widely
// held than the OAuth ES256 private key — could mint their own "OAuth"
// bearer tokens. This must fail even though every claim in the token is
// otherwise perfectly valid (correct sub, aud, scope, and a future exp).
func TestRequireOAuthScopeRejectsHMACSignedToken(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference", time.Now().Add(time.Hour))

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("some-hmac-secret-that-is-not-the-es256-key"))
	require.NoError(t, err)

	r, w := newOAuthScopeTestRouter(keySvc, "inference")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

// TestRequireOAuthScopeRejectsTokenWithNoExpClaim is the fix for the review
// finding: golang-jwt/v5 only validates exp when the claim is PRESENT
// (validator.go's verifyExpiresAt defaults required=false), so without
// jwt.WithExpirationRequired() a validly ES256-signed token that simply
// omits exp would be accepted as non-expiring forever. Signed with the real
// active ES256 key and otherwise-valid claims — only exp is missing — so
// this fails for the right reason (the missing-exp check) and not because
// the signature or any other claim is wrong.
func TestRequireOAuthScopeRejectsTokenWithNoExpClaim(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := jwt.MapClaims{
		"iss":   "https://portal.example.com",
		"sub":   "42",
		"aud":   "agent:abc123",
		"scope": "inference",
		"iat":   time.Now().Unix(),
		// no "exp" claim at all.
	}
	tok := mintTestES256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}

func TestRequireOAuthScopeRejectsExpiredToken(t *testing.T) {
	keySvc := newOAuthScopeTestKeyService(t)
	claims := baseTestClaims(42, "agent:abc123", "inference", time.Now().Add(-time.Hour))
	tok := mintTestES256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference")
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
		"scope": "inference",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	tok := mintTestES256Token(t, keySvc, claims)

	r, w := newOAuthScopeTestRouter(keySvc, "inference")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String())
}
