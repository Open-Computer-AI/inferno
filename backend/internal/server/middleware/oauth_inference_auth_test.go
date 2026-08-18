//go:build unit

package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The API-key path's 401, verbatim from api_key_auth.go. Every OAuth-branch
// response has to be distinguishable from it -- that is the whole point of
// keeping the two credential universes apart, so it is spelled out here rather
// than referenced, and a change to either side shows up as a test failure
// rather than as two constants agreeing with each other.
const invalidAPIKeyBody = `{"code":"INVALID_API_KEY","message":"Invalid API key"}`

// oauthInferenceGroupName is the configured oauth_backing_key.group_name for
// these tests, and the name of the active group seeded into the database.
const oauthInferenceGroupName = "oauth-agents"

// oauthInferenceUniqueIndexDDL mirrors migration 910's identity index. Ent's
// schema does not declare it (it is a hand-written migration), so without this
// the backing-row identity rule would simply not exist in the test database.
const oauthInferenceUniqueIndexDDL = `CREATE UNIQUE INDEX api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL AND deleted_at IS NULL`

// capturedAuthContext is everything the /v1 chain downstream reads out of the
// auth middleware. The probe handler records it so a test can assert on the
// contract instead of on a status code alone.
type capturedAuthContext struct {
	ran            bool
	apiKey         *service.APIKey
	apiKeyOK       bool
	subject        AuthSubject
	subjectOK      bool
	role           string
	roleOK         bool
	requestUserID  any
	requestGroup   *service.Group
	opsFallback    *service.APIKey
	opsFallbackOK  bool
	subscriptionOK bool
}

type oauthInferenceFixture struct {
	entClient  *dbent.Client
	keySvc     *service.OAuthKeyService
	clientSvc  *service.OAuthClientService
	backingSvc *service.OAuthBackingKeyService
	apiKeySvc  *service.APIKeyService
	apiKeyRepo *stubApiKeyRepo
	// delegateCalls counts how many times the API-key middleware was entered.
	// The no-fallthrough rule (evidence finding F-B) is asserted on this: a
	// JWT-shaped credential that fails verification must never be retried as an
	// API key, which means this counter must stay at zero.
	delegateCalls *atomic.Int32
	userID        int64
	captured      *capturedAuthContext
	router        *gin.Engine
}

func newOAuthInferenceFixture(t *testing.T) *oauthInferenceFixture {
	t.Helper()
	return newOAuthInferenceFixtureWithGroup(t, oauthInferenceGroupName, oauthInferenceGroupName)
}

// newOAuthInferenceFixtureWithGroup builds the whole /v1-shaped auth chain over
// one in-memory database: the real OAuth key service (so RS256 verification is
// genuinely exercised), the real client registry (so the audience check runs
// against a real row), the real backing-key service (so a real api_keys row is
// created), and the real API-key middleware as the delegate.
//
// configuredGroup is what oauth_backing_key.group_name is set to; seededGroup is
// the group that actually exists. They differ in exactly one test, the one that
// drives ErrNoGroupForOAuthKey.
func newOAuthInferenceFixtureWithGroup(t *testing.T, configuredGroup, seededGroup string) *oauthInferenceFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	entClient := newOAuthScopeTestEntClient(t)
	registerOAuthScopeTestClient(t, entClient, testAudienceClientID, service.ClientActive)

	// Migration 910's identity index, applied through the ent driver's own
	// connection so it lands in the same in-memory database.
	_, err := entClient.ExecContext(context.Background(), oauthInferenceUniqueIndexDDL)
	require.NoError(t, err, "apply migration 910's identity index")

	if seededGroup != "" {
		_, err := entClient.Group.Create().
			SetName(seededGroup).
			SetDescription("group every OAuth backing key binds to").
			SetPlatform(service.PlatformAnthropic).
			SetStatus(domain.StatusActive).
			SetSubscriptionType(service.SubscriptionTypeStandard).
			SetRateMultiplier(1.0).
			Save(context.Background())
		require.NoError(t, err, "seed the policy group")
	}

	user, err := entClient.User.Create().
		SetEmail("agent-owner@example.com").
		SetUsername("agent-owner").
		SetPasswordHash("hash").
		Save(context.Background())
	require.NoError(t, err, "seed the owning user")

	backingCfg := &config.Config{}
	backingCfg.Default.APIKeyPrefix = "sk-"
	backingCfg.OAuthBackingKey.GroupName = configuredGroup

	apiKeyCfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyRepo := &stubApiKeyRepo{}
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, apiKeyCfg)

	f := &oauthInferenceFixture{
		entClient:     entClient,
		keySvc:        service.NewOAuthKeyService(entClient),
		clientSvc:     service.NewOAuthClientService(entClient),
		backingSvc:    service.NewOAuthBackingKeyService(entClient, backingCfg),
		apiKeySvc:     apiKeySvc,
		apiKeyRepo:    apiKeyRepo,
		delegateCalls: &atomic.Int32{},
		userID:        user.ID,
		captured:      &capturedAuthContext{},
	}

	delegate := gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeySvc, nil, apiKeyCfg))
	countingDelegate := APIKeyAuthMiddleware(func(c *gin.Context) {
		f.delegateCalls.Add(1)
		delegate(c)
	})

	r := gin.New()
	r.POST("/v1/responses",
		OAuthOrAPIKeyAuth(countingDelegate, apiKeySvc, f.keySvc, f.clientSvc, f.backingSvc, testIssuer),
		func(c *gin.Context) {
			cap := f.captured
			cap.ran = true
			cap.apiKey, cap.apiKeyOK = GetAPIKeyFromContext(c)
			cap.subject, cap.subjectOK = GetAuthSubjectFromContext(c)
			cap.role, cap.roleOK = GetUserRoleFromContext(c)
			cap.opsFallback, cap.opsFallbackOK = GetOpsFallbackAPIKey(c)
			_, cap.subscriptionOK = GetSubscriptionFromContext(c)
			cap.requestUserID = c.Request.Context().Value(ctxkey.UserID)
			if g, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok {
				cap.requestGroup = g
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	f.router = r
	return f
}

// inferenceToken mints a real RS256 access token with the given scope, shaped
// exactly as OAuthTokenService.mintAccessToken shapes one.
func (f *oauthInferenceFixture) inferenceToken(t *testing.T, scope string) string {
	t.Helper()
	claims := baseTestClaims(f.userID, testAudienceClientID, scope, time.Now().Add(time.Hour))
	return mintTestRS256Token(t, f.keySvc, claims)
}

func (f *oauthInferenceFixture) do(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	f.router.ServeHTTP(w, req)
	return w
}

// wouldBeSizeCapped reports whether the API-key path's header-size guard --
// the REAL apiKeyHeadersTooLarge, not a restatement of it -- would have killed
// this Authorization header before any lookup.
func wouldBeSizeCapped(authorization string) bool {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Authorization", authorization)
	return apiKeyHeadersTooLarge(c)
}

// TestOAuthTokenReachesTheHandler is the brief's Step 1 test and the whole
// point of Task 4: a valid RS256 token carrying inference:invoke must get PAST
// auth, and must leave behind exactly what the /v1 pipeline reads.
func TestOAuthTokenReachesTheHandler(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusOK, w.Code, "the token must get PAST auth; body: %s", w.Body.String())
	require.True(t, f.captured.ran, "the handler must actually run")
	require.Zero(t, f.delegateCalls.Load(), "the OAuth branch must not enter the API-key middleware")

	// ContextKeyAPIKey: absent, every gateway handler answers 401 itself, and
	// AlphaSearch additionally requires a non-nil Group.
	require.True(t, f.captured.apiKeyOK, "ContextKeyAPIKey must be set")
	require.NotNil(t, f.captured.apiKey)
	require.NotZero(t, f.captured.apiKey.ID, "usage_logs.api_key_id is NOT NULL and FKs to api_keys(id)")
	require.NotNil(t, f.captured.apiKey.Group, "AlphaSearch requires apiKey.Group != nil")
	require.Equal(t, oauthInferenceGroupName, f.captured.apiKey.Group.Name)
	require.NotNil(t, f.captured.apiKey.User)
	require.Equal(t, f.userID, f.captured.apiKey.User.ID)
	require.NotNil(t, f.captured.apiKey.OAuthClientID)
	require.Equal(t, testAudienceClientID, *f.captured.apiKey.OAuthClientID)

	// ContextKeyUser: absent, handlers answer 500 "User context not found".
	require.True(t, f.captured.subjectOK, "ContextKeyUser must be set")
	require.Equal(t, f.userID, f.captured.subject.UserID)
	require.Equal(t, 5, f.captured.subject.Concurrency, "AcquireUserSlotWithWait reads this")

	// ContextKeyUserRole: read by nothing on /v1 today, set for parity.
	require.True(t, f.captured.roleOK, "ContextKeyUserRole must be set")
	require.Equal(t, domain.RoleUser, f.captured.role)

	// ctxkey.UserID on the REQUEST context, which is where profit control and
	// request-body personalisation read it -- not the gin context.
	require.Equal(t, f.userID, f.captured.requestUserID,
		"ctxkey.UserID must be on the request context, not only the gin context")

	// The group context setGroupContext publishes, read by pricing, profit
	// control and scheduling.
	require.NotNil(t, f.captured.requestGroup, "setGroupContext must have run")
	require.Equal(t, oauthInferenceGroupName, f.captured.requestGroup.Name)

	// Ops attribution on early aborts.
	require.True(t, f.captured.opsFallbackOK, "SetOpsFallbackAPIKey must have run")
	require.Equal(t, f.captured.apiKey.ID, f.captured.opsFallback.ID)

	// Documented as optional: a nil subscription bills the wallet.
	require.False(t, f.captured.subscriptionOK, "ContextKeySubscription is deliberately not set")

	// The backing row's secret must never leave the server, and this context is
	// exactly where a stray %v would leak it.
	require.Empty(t, f.captured.apiKey.Key, "the backing key's secret must not reach the gin context")

	// The row is real and is the metering ledger, not a synthetic struct.
	stored, err := f.entClient.APIKey.Get(context.Background(), f.captured.apiKey.ID)
	require.NoError(t, err, "the api_keys row must exist in the database")
	require.NotNil(t, stored.OauthClientID)
	require.Equal(t, testAudienceClientID, *stored.OauthClientID)
}

// TestOAuthTokenIsNotSizeCapped is the regression guard for evidence finding
// F-A. maxAPIKeyAuthorizationHeaderBytes is 256 and apiKeyHeadersTooLarge is
// the FIRST thing the API-key middleware does, so a middleware that ran the
// shape test behind it would 401 every real token -- and would look perfectly
// healthy under a test that used a short fake one.
//
// The fixture is therefore checked against the REAL apiKeyHeadersTooLarge
// before the request is made: this credential genuinely is one the API-key path
// would have killed on size alone.
func TestOAuthTokenIsNotSizeCapped(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)
	authorization := "Bearer " + token

	require.Greater(t, len(authorization), 256,
		"a real RS256 access token header is ~628 bytes; a fixture under 256 would not test F-A at all")
	require.True(t, wouldBeSizeCapped(authorization),
		"the fixture must be a credential the API-key path's size guard would reject, or this test proves nothing")

	w := f.do(t, authorization)

	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"a token-sized credential must not be rejected by the API-key path's 256-byte header cap")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// TestOAuthTokenThatFailsVerificationDoesNotFallThroughToTheAPIKeyPath is
// evidence finding F-B. A JWT-shaped credential that does not verify is
// rejected outright. Falling through would let one request probe both
// credential universes, and would feed the IP-keyed invalid-auth abuse limiter
// on the API-key path -- the limiter that the reproduction drove to an IP-wide
// 429 that also blocks every legitimately-keyed client behind the same egress
// IP.
func TestOAuthTokenThatFailsVerificationDoesNotFallThroughToTheAPIKeyPath(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	// Signed with an independently generated ES256 key -- JWT-shaped, three
	// non-empty base64url segments, every claim otherwise valid, but not
	// signed by this server.
	claims := baseTestClaims(f.userID, testAudienceClientID, service.ScopeInferenceInvoke, time.Now().Add(time.Hour))
	token := mintTestES256Token(t, claims)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String(),
		"the OAuth 401 must be distinguishable from the API-key path's 401")
	require.NotEqual(t, invalidAPIKeyBody, strings.TrimSpace(w.Body.String()))
	require.Equal(t, `Bearer error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
	require.Zero(t, f.delegateCalls.Load(),
		"a JWT-shaped credential that fails verification must never be retried as an API key")
	require.False(t, f.captured.ran, "the handler must not run")
}

// TestOAuthNonJWTCredentialStillReachesTheAPIKeyPath is the other half of the
// routing decision: anything that is not JWT-shaped is delegated untouched, so
// an ordinary key is authenticated exactly as it was before this middleware
// existed.
func TestOrdinaryAPIKeyStillSucceedsOnTheSameRoute(t *testing.T) {
	group := &service.Group{
		ID:       9,
		Name:     "ordinary",
		Status:   service.StatusActive,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
	}
	user := &service.User{
		ID:          4242,
		Role:        service.RoleAdmin,
		Status:      service.StatusActive,
		Balance:     10,
		Concurrency: 3,
	}
	stored := &service.APIKey{
		ID:      777,
		UserID:  user.ID,
		Key:     "sk-ordinary-key",
		Name:    "ordinary",
		Status:  service.StatusActive,
		GroupID: &group.ID,
		User:    user,
		Group:   group,
	}

	f := newOAuthInferenceFixture(t)
	f.apiKeyRepo.getByKey = func(context.Context, string) (*service.APIKey, error) { return stored, nil }

	w := f.do(t, "Bearer sk-ordinary-key")

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Equal(t, int32(1), f.delegateCalls.Load(), "an ordinary key must go through the API-key middleware")
	require.True(t, f.captured.apiKeyOK)
	require.Equal(t, int64(777), f.captured.apiKey.ID)
	require.Equal(t, "sk-ordinary-key", f.captured.apiKey.Key)
	require.Equal(t, user.ID, f.captured.subject.UserID)
	require.Equal(t, service.RoleAdmin, f.captured.role)
	require.Equal(t, user.ID, f.captured.requestUserID)
	require.NotNil(t, f.captured.requestGroup)

	// "Byte-identically to today": the same request through the bare API-key
	// middleware must produce the same status, the same body, and the same
	// context. Compared against the real middleware rather than against a
	// restatement of what it is believed to do.
	baselineCaptured := &capturedAuthContext{}
	baseline := gin.New()
	baseline.POST("/v1/responses",
		gin.HandlerFunc(NewAPIKeyAuthMiddleware(f.apiKeySvc, nil, &config.Config{RunMode: config.RunModeSimple})),
		func(c *gin.Context) {
			baselineCaptured.apiKey, baselineCaptured.apiKeyOK = GetAPIKeyFromContext(c)
			baselineCaptured.subject, baselineCaptured.subjectOK = GetAuthSubjectFromContext(c)
			baselineCaptured.role, baselineCaptured.roleOK = GetUserRoleFromContext(c)
			baselineCaptured.requestUserID = c.Request.Context().Value(ctxkey.UserID)
			if g, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok {
				baselineCaptured.requestGroup = g
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	bw := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi"}`))
	breq.Header.Set("Content-Type", "application/json")
	breq.Header.Set("Authorization", "Bearer sk-ordinary-key")
	baseline.ServeHTTP(bw, breq)

	require.Equal(t, bw.Code, w.Code)
	require.Equal(t, bw.Body.String(), w.Body.String())
	require.Equal(t, baselineCaptured.apiKey, f.captured.apiKey)
	require.Equal(t, baselineCaptured.subject, f.captured.subject)
	require.Equal(t, baselineCaptured.role, f.captured.role)
	require.Equal(t, baselineCaptured.requestUserID, f.captured.requestUserID)
	require.Equal(t, baselineCaptured.requestGroup, f.captured.requestGroup)
}

// TestOAuthTokenWithoutInferenceInvokeScopeIsForbidden: a token whose identity
// is genuine but whose scope does not cover inference gets 403, never 401.
// 401 would tell the real client the bearer is stale, and it would force-refresh
// and retry forever for a reason a refresh can never fix.
//
// The granted scopes here include "inference:invoke_nothing" and bare
// "inference", neither of which may satisfy the requirement -- a
// strings.Contains implementation would wrongly accept the first.
func TestOAuthTokenWithoutInferenceInvokeScopeIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, "inference inference:invoke_nothing billing:read")

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
	require.Zero(t, f.delegateCalls.Load())
	require.False(t, f.captured.ran)

	// No backing row may be created for a token that is not allowed to infer.
	count, err := f.entClient.APIKey.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count, "scope is checked before any row is provisioned")
}

// TestOAuthNoGroupPolicyIsForbiddenNotUnauthorized: an operator configuration
// problem. A nil Group reaches platform routing and panics, so Resolve refuses
// rather than returning such a row -- and the refusal must not read to the
// client as a stale credential.
func TestOAuthNoGroupPolicyIsForbiddenNotUnauthorized(t *testing.T) {
	// Configured group name matches no group in the database.
	f := newOAuthInferenceFixtureWithGroup(t, "no-such-group", oauthInferenceGroupName)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "oauth_backing_group_unavailable", body["error"])
	require.Contains(t, body["error_description"], "oauth_backing_key.group_name",
		"the message has to name the setting an operator must fix")
	require.Zero(t, f.delegateCalls.Load())
	require.False(t, f.captured.ran)
}

// TestOAuthDisabledBackingKeyIsForbidden: disabling the backing row is the ONLY
// per-agent kill switch an operator has -- an access token cannot be revoked
// inside its window, and the downstream CheckBillingEligibility does not look at
// api_keys.status. If this middleware ignored the status, the kill switch would
// not work.
func TestOAuthDisabledBackingKeyIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code, "provision the backing row first")

	// The operator pulls the switch.
	rowID := f.captured.apiKey.ID
	require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
		SetStatus(service.StatusAPIKeyDisabled).
		Exec(context.Background()))

	f.captured.ran = false
	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"backing_key_disabled"}`, w.Body.String())
	require.False(t, f.captured.ran, "a disabled backing row must stop the agent")
}

// TestOAuthInactiveOwnerIsForbidden: a deactivated account must not keep
// serving inference for the remaining lifetime of an already-minted token.
func TestOAuthInactiveOwnerIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.NoError(t, f.entClient.User.UpdateOneID(f.userID).
		SetStatus(service.StatusDisabled).
		Exec(context.Background()))

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"account_inactive"}`, w.Body.String())
	require.False(t, f.captured.ran)
}

// TestOAuthBranchIsNotEnteredWithoutABearerHeader: no Authorization header at
// all is an API-key-path concern (it owns x-api-key and x-goog-api-key too), so
// the request must be delegated rather than answered here.
func TestOAuthBranchIsNotEnteredWithoutABearerHeader(t *testing.T) {
	f := newOAuthInferenceFixture(t)

	w := f.do(t, "")

	require.Equal(t, int32(1), f.delegateCalls.Load(), "a credential-less request belongs to the API-key path")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "API_KEY_REQUIRED")
}
