package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
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
	families map[string]map[string]struct{}
	users    map[int64]map[string]struct{}
}

func newOAuthHandlerTestRefreshCache() *oauthHandlerTestRefreshCache {
	return &oauthHandlerTestRefreshCache{
		tokens:   map[string]*service.RefreshTokenData{},
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

func (c *oauthHandlerTestRefreshCache) MarkRotated(_ context.Context, hash string, tombstoned *service.RefreshTokenData) (*service.RefreshTokenData, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.tokens[hash]
	if !ok {
		return nil, false, service.ErrRefreshTokenNotFound
	}
	cloned := *data
	if data.Rotated {
		return &cloned, true, nil
	}
	tomb := *tombstoned
	c.tokens[hash] = &tomb
	return &cloned, false, nil
}

// oauthHandlerTestUserLookup always reports every user active — account
// status re-validation is exercised at the service layer, not here.
type oauthHandlerTestUserLookup struct{}

func (oauthHandlerTestUserLookup) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Status: service.StatusActive}, nil
}

func newOAuthHandlerTestHandler(t *testing.T) *OAuthHandler {
	t.Helper()
	client := newOAuthHandlerTestEntClient(t)
	keySvc := service.NewOAuthKeyService(client)
	clientSvc := service.NewOAuthClientService(client)
	deviceSvc := service.NewOAuthDeviceService(client, "https://portal.example.com")
	tokenSvc := service.NewOAuthTokenService(client, keySvc, deviceSvc, newOAuthHandlerTestRefreshCache(), oauthHandlerTestUserLookup{}, "https://portal.example.com")
	return NewOAuthHandler(keySvc, clientSvc, nil, deviceSvc, tokenSvc)
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
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference")
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
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference")
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
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference")
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
		grant, err := h.deviceSvc.RequestCode(ctx, clientID, "inference")
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
