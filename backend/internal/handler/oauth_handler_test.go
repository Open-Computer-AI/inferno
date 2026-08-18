package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	oauthmiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// This file asserts the WIRE CONTRACT of POST /api/oauth/token: exactly the
// RFC-shaped {"error": "..."} / token body at the top level — never
// internal/pkg/response's {code,message,data} wrapper — across every
// grant/error branch. The security logic itself (reuse detection, the
// device-code CAS, scope preservation, ...) is covered at the service layer
// in internal/service/oauth_token_service_test.go; this file exists so a
// future refactor that reintroduces response.Success/response.Error here,
// or rewords one of the RFC error strings, fails a test instead of only
// showing up in a curl transcript in a report.

func newOAuthHandlerTestEntClient(t *testing.T) *dbent.Client {
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

// oauthHandlerTestRefreshCache is a minimal in-memory service.RefreshTokenCache.
// Only what the token endpoint's happy/error paths actually exercise; family
// reuse semantics are tested at the service layer, not re-verified here.
type oauthHandlerTestRefreshCache struct {
	mu       sync.Mutex
	tokens   map[string]*service.RefreshTokenData
	replays  map[string][]byte
	families map[string]map[string]struct{}
	users    map[int64]map[string]struct{}
}

func newOAuthHandlerTestRefreshCache() *oauthHandlerTestRefreshCache {
	return &oauthHandlerTestRefreshCache{
		tokens:   map[string]*service.RefreshTokenData{},
		replays:  map[string][]byte{},
		families: map[string]map[string]struct{}{},
		users:    map[int64]map[string]struct{}{},
	}
}

func (c *oauthHandlerTestRefreshCache) StoreRefreshToken(_ context.Context, hash string, data *service.RefreshTokenData, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cloned := *data
	c.tokens[hash] = &cloned
	return nil
}

// PersistRefreshToken mirrors the atomic record+both-memberships write the
// Redis implementation performs in one EVAL.
func (c *oauthHandlerTestRefreshCache) PersistRefreshToken(_ context.Context, hash string, data *service.RefreshTokenData, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cloned := *data
	c.tokens[hash] = &cloned
	if c.users[data.UserID] == nil {
		c.users[data.UserID] = map[string]struct{}{}
	}
	c.users[data.UserID][hash] = struct{}{}
	if c.families[data.FamilyID] == nil {
		c.families[data.FamilyID] = map[string]struct{}{}
	}
	c.families[data.FamilyID][hash] = struct{}{}
	return nil
}

func (c *oauthHandlerTestRefreshCache) GetRefreshToken(_ context.Context, hash string) (*service.RefreshTokenData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.tokens[hash]
	if !ok {
		return nil, service.ErrRefreshTokenNotFound
	}
	cloned := *data
	return &cloned, nil
}

func (c *oauthHandlerTestRefreshCache) DeleteRefreshToken(_ context.Context, hash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, hash)
	return nil
}

func (c *oauthHandlerTestRefreshCache) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash := range c.users[userID] {
		delete(c.tokens, hash)
	}
	delete(c.users, userID)
	return nil
}

func (c *oauthHandlerTestRefreshCache) DeleteTokenFamily(_ context.Context, familyID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash := range c.families[familyID] {
		delete(c.tokens, hash)
	}
	delete(c.families, familyID)
	return nil
}

func (c *oauthHandlerTestRefreshCache) AddToUserTokenSet(_ context.Context, userID int64, hash string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users[userID] == nil {
		c.users[userID] = map[string]struct{}{}
	}
	c.users[userID][hash] = struct{}{}
	return nil
}

func (c *oauthHandlerTestRefreshCache) AddToFamilyTokenSet(_ context.Context, familyID, hash string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.families[familyID] == nil {
		c.families[familyID] = map[string]struct{}{}
	}
	c.families[familyID][hash] = struct{}{}
	return nil
}

func (c *oauthHandlerTestRefreshCache) GetUserTokenHashes(_ context.Context, userID int64) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.users[userID]))
	for h := range c.users[userID] {
		out = append(out, h)
	}
	return out, nil
}

func (c *oauthHandlerTestRefreshCache) GetFamilyTokenHashes(_ context.Context, familyID string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.families[familyID]))
	for h := range c.families[familyID] {
		out = append(out, h)
	}
	return out, nil
}

func (c *oauthHandlerTestRefreshCache) IsTokenInFamily(_ context.Context, familyID, hash string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.families[familyID][hash]
	return ok, nil
}

// MarkRotated mirrors the Redis Lua script closely enough for the handler
// tests: the whole decision happens under c.mu, and a replay inside
// service.RefreshReuseGracePeriod is answered with the stored pair rather
// than treated as reuse. The grace semantics themselves are asserted at the
// service layer (internal/service/oauth_token_service_test.go); this stub
// only has to avoid contradicting them.
func (c *oauthHandlerTestRefreshCache) MarkRotated(_ context.Context, hash string, tombstoned *service.RefreshTokenData, sealedReplay []byte, now time.Time) (*service.RefreshRotationResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.tokens[hash]
	if !ok {
		return nil, service.ErrRefreshTokenNotFound
	}
	cloned := *data
	if data.Rotated {
		stored, inGrace := c.replays[hash]
		superseded := inGrace && string(stored) == service.RefreshReplaySupersededMarker
		if inGrace && !superseded && data.RotatedAtUnix > 0 && now.Unix()-data.RotatedAtUnix <= int64(service.RefreshReuseGracePeriod/time.Second) {
			return &service.RefreshRotationResult{Data: &cloned, Outcome: service.RefreshRotationGraceReplay, SealedReplay: stored}, nil
		}
		return &service.RefreshRotationResult{Data: &cloned, Outcome: service.RefreshRotationReuse}, nil
	}
	if len(sealedReplay) == 0 {
		return nil, fmt.Errorf("mark refresh token rotated: refused to claim an un-rotated record without a replay pair")
	}
	tomb := *tombstoned
	c.tokens[hash] = &tomb
	if c.replays == nil {
		c.replays = make(map[string][]byte)
	}
	c.replays[hash] = sealedReplay
	if tombstoned.PredecessorHash != "" {
		c.replays[tombstoned.PredecessorHash] = []byte(service.RefreshReplaySupersededMarker)
	}
	return &service.RefreshRotationResult{Data: &cloned, Outcome: service.RefreshRotationClaimed}, nil
}

// oauthHandlerTestUserLookup always reports every user active — account
// status re-validation is exercised at the service layer, not here.
type oauthHandlerTestUserLookup struct{}

func (oauthHandlerTestUserLookup) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{
		ID:     id,
		Email:  fmt.Sprintf("user%d@example.com", id),
		Status: service.StatusActive,
	}, nil
}

func newOAuthHandlerTestHandler(t *testing.T) *OAuthHandler {
	t.Helper()
	client := newOAuthHandlerTestEntClient(t)
	keySvc := service.NewOAuthKeyService(client)
	clientSvc := service.NewOAuthClientService(client)
	deviceSvc := service.NewOAuthDeviceService(client, "https://portal.example.com")
	tokenSvc := service.NewOAuthTokenService(client, keySvc, deviceSvc, newOAuthHandlerTestRefreshCache(), oauthHandlerTestUserLookup{}, "https://portal.example.com")
	return NewOAuthHandler(keySvc, clientSvc, nil, deviceSvc, tokenSvc, nil)
}

func newOAuthHandlerTestRouter(t *testing.T) (*gin.Engine, *OAuthHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newOAuthHandlerTestHandler(t)
	router := gin.New()
	router.POST("/api/oauth/token", h.Token)
	return router, h
}

func postOAuthToken(t *testing.T, router *gin.Engine, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// requireBareErrorBody asserts the body is EXACTLY {"error": want} at the
// top level — no "code", "message", or "data" keys, which is what
// internal/pkg/response's wrapper would add. The hermes client hard-fails on
// a wrapped body, per this package's NOTE at the top of oauth_handler.go.
func requireBareErrorBody(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body, 1, "body must have exactly one top-level key, got %v", body)
	require.Equal(t, want, body["error"])
	for _, forbidden := range []string{"code", "message", "data"} {
		_, present := body[forbidden]
		require.False(t, present, "response must not carry internal/pkg/response's %q wrapper key", forbidden)
	}
}

func TestTokenUnsupportedGrantType(t *testing.T) {
	router, _ := newOAuthHandlerTestRouter(t)

	rec := postOAuthToken(t, router, url.Values{
		"grant_type": {"password"},
		"client_id":  {"agent:whatever"},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	requireBareErrorBody(t, rec, "unsupported_grant_type")
}

func TestTokenDeviceCodeGrantErrorBranches(t *testing.T) {
	router, h := newOAuthHandlerTestRouter(t)
	ctx := context.Background()

	clientOC, err := h.clientSvc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	require.NoError(t, err)
	clientID := clientOC.ClientID

	t.Run("authorization_pending before approval", func(t *testing.T) {
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference:invoke")
		require.NoError(t, err)

		rec := postOAuthToken(t, router, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {clientID},
			"device_code": {grant.DeviceCode},
		})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		requireBareErrorBody(t, rec, "authorization_pending")
	})

	t.Run("slow_down on immediate repoll", func(t *testing.T) {
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference:invoke")
		require.NoError(t, err)
		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {clientID},
			"device_code": {grant.DeviceCode},
		}
		first := postOAuthToken(t, router, form)
		requireBareErrorBody(t, first, "authorization_pending")

		second := postOAuthToken(t, router, form)
		require.Equal(t, http.StatusBadRequest, second.Code)
		requireBareErrorBody(t, second, "slow_down")
	})

	t.Run("access_denied for mismatched client", func(t *testing.T) {
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference:invoke")
		require.NoError(t, err)
		require.NoError(t, h.deviceSvc.Approve(ctx, grant.UserCode, 42))

		rec := postOAuthToken(t, router, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {"agent:someone-else"},
			"device_code": {grant.DeviceCode},
		})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		requireBareErrorBody(t, rec, "access_denied")
	})

	t.Run("successful exchange returns a bare token body with no-store headers", func(t *testing.T) {
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference:invoke")
		require.NoError(t, err)
		require.NoError(t, h.deviceSvc.Approve(ctx, grant.UserCode, 42))

		rec := postOAuthToken(t, router, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {clientID},
			"device_code": {grant.DeviceCode},
		})

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		require.Equal(t, "no-cache", rec.Header().Get("Pragma"))

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		wantKeys := []string{"access_token", "refresh_token", "token_type", "expires_in", "scope"}
		require.Len(t, body, len(wantKeys), "unexpected top-level keys: %v", body)
		for _, k := range wantKeys {
			require.Contains(t, body, k)
		}
		require.NotContains(t, body, "code")
		require.NotContains(t, body, "message")
		require.NotContains(t, body, "data")
		require.Equal(t, "Bearer", body["token_type"])
	})
}

func TestTokenRefreshGrantInvalidGrant(t *testing.T) {
	router, h := newOAuthHandlerTestRouter(t)
	ctx := context.Background()
	clientOC, err := h.clientSvc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	require.NoError(t, err)

	rec := postOAuthToken(t, router, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientOC.ClientID},
		"refresh_token": {"art_does-not-exist"},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	requireBareErrorBody(t, rec, "invalid_grant")
}

// mintOAuthTestTokenPair runs a full device_code grant (request → approve →
// exchange) and returns the resulting refresh_token, so refresh-grant tests
// below don't have to duplicate that setup. Each call mints a FRESH token —
// refresh tokens are single-use/rotate-on-use (Task 5), so a subtest that
// calls this must not reuse another subtest's value.
func mintOAuthTestTokenPair(t *testing.T, router *gin.Engine, h *OAuthHandler, ctx context.Context, clientID string) string {
	t.Helper()
	grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference:invoke")
	require.NoError(t, err)
	require.NoError(t, h.deviceSvc.Approve(ctx, grant.UserCode, 42))

	rec := postOAuthToken(t, router, url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {clientID},
		"device_code": {grant.DeviceCode},
	})
	require.Equal(t, http.StatusOK, rec.Code, "device_code exchange must succeed to set up a refresh test: %s", rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	refreshToken, _ := body["refresh_token"].(string)
	require.NotEmpty(t, refreshToken)
	return refreshToken
}

// postOAuthTokenWithHeader is postOAuthToken plus an optional
// x-nous-refresh-token header — used only by the refresh-grant
// credential-source tests below. header == "" omits the header entirely
// (distinct from sending it empty, which the tests exercise separately).
func postOAuthTokenWithHeader(t *testing.T, router *gin.Engine, form url.Values, header string, headerSet bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if headerSet {
		req.Header.Set(nousRefreshTokenHeader, header)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestTokenRefreshGrantCredentialSource locks down refreshTokenFromRequest's
// contract: the real hermes client (hermes_cli/auth.py's
// _refresh_access_token) sends the refresh token in the x-nous-refresh-token
// HEADER, with only grant_type/client_id in the body — confirmed by reading
// that function directly (that repo is read-only; never edited). Task 8's
// conformance run tested the device_code grant only and missed this: the
// refresh grant read RFC 6749 §5.2-correctly from the body, passed every
// existing test, and was still completely broken against the one real
// client this plan exists to serve, because that client never populates the
// body field at all. These four cases are the ones that would have caught
// it, plus the precedence/fallback rules the fix's doc comment promises.
func TestTokenRefreshGrantCredentialSource(t *testing.T) {
	router, h := newOAuthHandlerTestRouter(t)
	ctx := context.Background()
	clientOC, err := h.clientSvc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	require.NoError(t, err)
	clientID := clientOC.ClientID

	requireSuccessfulRefresh := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.NotEmpty(t, body["access_token"])
		require.NotEmpty(t, body["refresh_token"])
	}

	t.Run("succeeds via header alone (the real client's path)", func(t *testing.T) {
		refreshToken := mintOAuthTestTokenPair(t, router, h, ctx, clientID)
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}}
		rec := postOAuthTokenWithHeader(t, router, form, refreshToken, true)
		requireSuccessfulRefresh(t, rec)
	})

	t.Run("still succeeds via the RFC 6749 body field", func(t *testing.T) {
		refreshToken := mintOAuthTestTokenPair(t, router, h, ctx, clientID)
		form := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {clientID},
			"refresh_token": {refreshToken},
		}
		rec := postOAuthTokenWithHeader(t, router, form, "", false)
		requireSuccessfulRefresh(t, rec)
	})

	t.Run("header takes precedence when both are present", func(t *testing.T) {
		validToken := mintOAuthTestTokenPair(t, router, h, ctx, clientID)
		form := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {clientID},
			"refresh_token": {"art_wrong-value-from-body-must-be-ignored"},
		}
		rec := postOAuthTokenWithHeader(t, router, form, validToken, true)
		requireSuccessfulRefresh(t, rec)
	})

	t.Run("empty/whitespace header falls through to a valid body value", func(t *testing.T) {
		refreshToken := mintOAuthTestTokenPair(t, router, h, ctx, clientID)
		form := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {clientID},
			"refresh_token": {refreshToken},
		}
		rec := postOAuthTokenWithHeader(t, router, form, "   ", true)
		requireSuccessfulRefresh(t, rec)
	})

	t.Run("both absent keeps the existing invalid_grant contract", func(t *testing.T) {
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}}
		rec := postOAuthTokenWithHeader(t, router, form, "", false)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		requireBareErrorBody(t, rec, "invalid_grant")
	})
}

// The tests below cover GET /api/oauth/account's wire contract, the same way
// the Token tests above cover POST /api/oauth/token's: exactly the bare
// {"error": "..."} (or, on success, the account body) at the top level,
// never internal/pkg/response's {code,message,data} wrapper — see
// requireBareErrorBody's doc comment. Unlike the Token tests, this endpoint
// sits behind middleware.RequireOAuthScope (Task 6), so these routers
// register that middleware for real rather than calling h.Account directly,
// so a regression in either layer (the middleware's context keys, or the
// handler reading them) fails a test.

func newOAuthAccountTestHandler(t *testing.T) *OAuthHandler {
	t.Helper()
	client := newOAuthHandlerTestEntClient(t)
	keySvc := service.NewOAuthKeyService(client)
	clientSvc := service.NewOAuthClientService(client)
	orgSvc := service.NewOrgService(client)
	deviceSvc := service.NewOAuthDeviceService(client, "https://portal.example.com")
	tokenSvc := service.NewOAuthTokenService(client, keySvc, deviceSvc, newOAuthHandlerTestRefreshCache(), oauthHandlerTestUserLookup{}, "https://portal.example.com")
	// The audience of every token these tests mint has to name a real,
	// usable client: RequireOAuthScope now resolves `aud` against the
	// registry rather than trusting it as the caller's identity.
	_, err := client.OAuthClient.Create().
		SetClientID("agent:test-client").
		SetKind("SELF_HOSTED").
		SetName("account test client").
		SetOwnerUserID(1).
		SetOrgID(1).
		SetRedirectURIOrigin("https://agent.example.com").
		SetStatus(service.ClientActive).
		Save(context.Background())
	require.NoError(t, err)
	return NewOAuthHandler(keySvc, clientSvc, orgSvc, deviceSvc, tokenSvc, oauthHandlerTestUserLookup{})
}

func newOAuthAccountTestRouter(t *testing.T) (*gin.Engine, *OAuthHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newOAuthAccountTestHandler(t)
	router := gin.New()
	router.GET("/api/oauth/account", oauthmiddleware.RequireOAuthScope(h.KeyService(), h.ClientService(), h.TokenIssuer(), ""), h.Account)
	return router, h
}

// mintTestAccountToken signs an RS256 access token with h's own active
// signing key, exactly as OAuthTokenService.mintAccessToken does — same
// claim shape — so these tests exercise the real verification path in
// RequireOAuthScope rather than a hand-rolled approximation of it.
func mintTestAccountToken(t *testing.T, h *OAuthHandler, userID int64, scope string) string {
	t.Helper()
	key, err := h.keySvc.Active(context.Background())
	require.NoError(t, err)

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   "https://portal.example.com",
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   "agent:test-client",
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)
	return signed
}

func getOAuthAccount(router *gin.Engine, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/account", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAccountRejectsMissingBearer(t *testing.T) {
	router, _ := newOAuthAccountTestRouter(t)

	rec := getOAuthAccount(router, "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	requireBareErrorBody(t, rec, "invalid_token")
}

func TestAccountRejectsGarbageToken(t *testing.T) {
	router, _ := newOAuthAccountTestRouter(t)

	rec := getOAuthAccount(router, "not-a-jwt-at-all")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	requireBareErrorBody(t, rec, "invalid_token")
}

func TestAccountReturnsOrgsForValidToken(t *testing.T) {
	router, h := newOAuthAccountTestRouter(t)
	ctx := context.Background()

	org, err := h.orgSvc.EnsurePersonalOrg(ctx, 42, "alice")
	require.NoError(t, err)

	tok := mintTestAccountToken(t, h, 42, "inference:invoke")
	rec := getOAuthAccount(router, tok)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	// Still bare — no {code,message,data} envelope.
	require.NotContains(t, body, "code")
	require.NotContains(t, body, "message")
	require.NotContains(t, body, "data")

	// The shape hermes_cli/nous_account.py's _info_from_account_payload
	// actually reads. Every assertion below names a key that function
	// dereferences; the previous {"user_id","orgs"} shape matched NONE of
	// them and failed soft, so the client reported "could not find a Nous
	// Portal account or organisation" on every paid-capability gate.
	user, ok := body["user"].(map[string]any)
	require.True(t, ok, "user must be an object, got %T", body["user"])
	require.Equal(t, "user42@example.com", user["email"])
	// privy_did is deliberately absent: Inferno does not use Privy and a
	// fabricated value would be worse than none.
	require.NotContains(t, user, "privy_did")

	organisation, ok := body["organisation"].(map[string]any)
	require.True(t, ok, "organisation must be an object, got %T", body["organisation"])
	require.Equal(t, strconv.FormatInt(org.ID, 10), organisation["id"])
	require.Equal(t, org.Slug, organisation["slug"])
	require.Equal(t, org.Name, organisation["name"])
	require.Equal(t, service.OrgRoleOwner, organisation["role"])
	require.Equal(t, true, organisation["isPersonal"])

	// Explicit "no paid tier" rather than an absent key: absent makes the
	// client say "could not verify your entitlement", which is a different
	// (and wrong) claim from "you are on the free tier".
	access, ok := body["paid_service_access"].(map[string]any)
	require.True(t, ok, "paid_service_access must be an object, got %T", body["paid_service_access"])
	require.Equal(t, false, access["allowed"])
	require.Equal(t, false, access["paid_access"])
	require.Equal(t, strconv.FormatInt(org.ID, 10), access["organisation_id"])
	// The user HAS an account, so the client's "account_missing" branch —
	// which tells them to re-authenticate — must not fire.
	require.NotContains(t, access, "reason")

	// Retained for the multi-org picker the design anticipates.
	orgs, ok := body["orgs"].([]any)
	require.True(t, ok, "orgs must be an array, got %T", body["orgs"])
	require.Len(t, orgs, 1)
	orgBody, ok := orgs[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, strconv.FormatInt(org.ID, 10), orgBody["id"])
	require.Equal(t, org.Slug, orgBody["slug"])
	require.Equal(t, service.OrgRoleOwner, orgBody["role"])
	require.Equal(t, true, orgBody["isPersonal"])
}

// TestAccountReportsAccountMissingWithNoOrg locks the degraded path. Post-C1
// every session provisions a personal org, so this should be unreachable — but
// if it ever happens the client must get the specific "account_missing" reason
// (which tells the user to re-authenticate) rather than a silently org-less
// payload that reads as a valid free-tier account.
func TestAccountReportsAccountMissingWithNoOrg(t *testing.T) {
	router, h := newOAuthAccountTestRouter(t)

	tok := mintTestAccountToken(t, h, 99, "inference:invoke")
	rec := getOAuthAccount(router, tok)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, body, "organisation")

	access, ok := body["paid_service_access"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "account_missing", access["reason"])
	require.Equal(t, false, access["allowed"])
}

func TestAccountRejectsInsufficientScopeIsUnreachable(t *testing.T) {
	// GET /api/oauth/account is registered with required scope "" (see
	// RegisterOAuthAccountRoutes) — any validly-signed, unexpired token
	// passes the scope check regardless of what scope it carries. This test
	// documents that choice: an inference-only token (never granted
	// billing:manage at login, per the plan) must still be able to resolve
	// its own account/org info.
	router, h := newOAuthAccountTestRouter(t)
	ctx := context.Background()
	_, err := h.orgSvc.EnsurePersonalOrg(ctx, 7, "bob")
	require.NoError(t, err)

	tok := mintTestAccountToken(t, h, 7, "inference:invoke")
	rec := getOAuthAccount(router, tok)

	require.Equal(t, http.StatusOK, rec.Code)
}

// TestDeviceCodeRejectsUnknownScope pins the wire contract for the scope
// allowlist: `invalid_scope` is RFC 8628 §3.2 / RFC 6749 §4.1.2.1's code, and
// it must stay a BARE body like every other hermes-facing response here.
//
// It is deliberately distinct from invalid_client: an unrecognised scope is a
// caller bug a developer has to be able to see, and the vocabulary is public
// and fixed, so naming it leaks nothing.
func TestDeviceCodeRejectsUnknownScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newOAuthHandlerTestEntClient(t)
	keySvc := service.NewOAuthKeyService(client)
	clientSvc := service.NewOAuthClientService(client)
	deviceSvc := service.NewOAuthDeviceService(client, "https://portal.example.com")
	tokenSvc := service.NewOAuthTokenService(client, keySvc, deviceSvc, newOAuthHandlerTestRefreshCache(), oauthHandlerTestUserLookup{}, "https://portal.example.com")
	h := NewOAuthHandler(keySvc, clientSvc, nil, deviceSvc, tokenSvc, nil)

	router := gin.New()
	router.POST("/api/oauth/device/code", h.DeviceCode)

	oc, err := clientSvc.RegisterSelfHosted(t.Context(), 1, 42, "https://agent.example.com", "")
	require.NoError(t, err)

	post := func(scope string) *httptest.ResponseRecorder {
		form := url.Values{"client_id": {oc.ClientID}, "scope": {scope}}
		req := httptest.NewRequest(http.MethodPost, "/api/oauth/device/code", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// The phishing payload from the finding: real client, real code, scopes
	// the victim would never knowingly grant.
	rec := post("billing:manage agents:manage inference:invoke everything")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	requireBareErrorBody(t, rec, "invalid_scope")

	// A legitimate request still works, and the same scopes minus the bogus
	// entry are perfectly grantable — the allowlist rejects unknown values,
	// it does not blanket-ban privileged ones.
	require.Equal(t, http.StatusOK, post("billing:manage agents:manage inference:invoke").Code)
	require.Equal(t, http.StatusOK, post("").Code)
}
