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
// auth middleware, plus the ingress-reject reason an operator greps. The probe
// handler records the first group; an outer middleware records the second,
// because a rejected request never reaches the handler.
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
	subscription   *service.UserSubscription
	subscriptionOK bool
	rejectReason   IngressRejectReason
	rejectOK       bool
}

// stubOAuthSubscriptionLoader is a one-method stand-in for
// *service.SubscriptionService. The real service needs two repositories, a
// billing cache, an ent client, a ristretto L1 cache and a background
// maintenance queue; the OAuth branch needs exactly one call, which is why
// OAuthSubscriptionLoader is declared on the consuming side.
type stubOAuthSubscriptionLoader struct {
	calls        atomic.Int32
	lastUserID   atomic.Int64
	lastGroupID  atomic.Int64
	subscription *service.UserSubscription
	err          error
}

func (s *stubOAuthSubscriptionLoader) GetActiveSubscription(_ context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	s.calls.Add(1)
	s.lastUserID.Store(userID)
	s.lastGroupID.Store(groupID)
	return s.subscription, s.err
}

type oauthInferenceOptions struct {
	// configuredGroup is what oauth_backing_key.group_name is set to;
	// seededGroup is the group that actually exists. They differ only in the
	// test that drives ErrNoGroupForOAuthKey.
	configuredGroup  string
	seededGroup      string
	subscriptionType string
	subscriptions    OAuthSubscriptionLoader
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
	// touchedKeyID records the api_keys row TouchLastUsed wrote through to,
	// via the real APIKeyService and its repository seam.
	touchedKeyID *atomic.Int64
	userID       int64
	groupID      int64
	captured     *capturedAuthContext
	router       *gin.Engine
}

func newOAuthInferenceFixture(t *testing.T) *oauthInferenceFixture {
	t.Helper()
	return newOAuthInferenceFixtureWith(t, oauthInferenceOptions{})
}

// newOAuthInferenceFixtureWith builds the whole /v1-shaped auth chain over one
// in-memory database: the real OAuth key service (so RS256 verification is
// genuinely exercised), the real client registry (so the audience check runs
// against a real row), the real backing-key service (so a real api_keys row is
// created), the real APIKeyService, and the real API-key middleware as the
// delegate.
func newOAuthInferenceFixtureWith(t *testing.T, opts oauthInferenceOptions) *oauthInferenceFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if opts.configuredGroup == "" {
		opts.configuredGroup = oauthInferenceGroupName
	}
	if opts.seededGroup == "" {
		opts.seededGroup = oauthInferenceGroupName
	}
	if opts.subscriptionType == "" {
		opts.subscriptionType = service.SubscriptionTypeStandard
	}

	entClient := newOAuthScopeTestEntClient(t)
	registerOAuthScopeTestClient(t, entClient, testAudienceClientID, service.ClientActive)

	// Migration 910's identity index, applied through the ent driver's own
	// connection so it lands in the same in-memory database.
	_, err := entClient.ExecContext(context.Background(), oauthInferenceUniqueIndexDDL)
	require.NoError(t, err, "apply migration 910's identity index")

	group, err := entClient.Group.Create().
		SetName(opts.seededGroup).
		SetDescription("group every OAuth backing key binds to").
		SetPlatform(service.PlatformAnthropic).
		SetStatus(domain.StatusActive).
		SetSubscriptionType(opts.subscriptionType).
		SetRateMultiplier(1.0).
		Save(context.Background())
	require.NoError(t, err, "seed the policy group")

	user, err := entClient.User.Create().
		SetEmail("agent-owner@example.com").
		SetUsername("agent-owner").
		SetPasswordHash("hash").
		Save(context.Background())
	require.NoError(t, err, "seed the owning user")

	backingCfg := &config.Config{}
	backingCfg.Default.APIKeyPrefix = "sk-"
	backingCfg.OAuthBackingKey.GroupName = opts.configuredGroup

	apiKeyCfg := &config.Config{RunMode: config.RunModeSimple}
	touched := &atomic.Int64{}
	apiKeyRepo := &stubApiKeyRepo{
		updateLastUsed: func(_ context.Context, id int64, _ time.Time) error {
			touched.Store(id)
			return nil
		},
	}
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, nil, apiKeyCfg)

	f := &oauthInferenceFixture{
		entClient:     entClient,
		keySvc:        service.NewOAuthKeyService(entClient),
		clientSvc:     service.NewOAuthClientService(entClient),
		backingSvc:    service.NewOAuthBackingKeyService(entClient, backingCfg),
		apiKeySvc:     apiKeySvc,
		apiKeyRepo:    apiKeyRepo,
		delegateCalls: &atomic.Int32{},
		touchedKeyID:  touched,
		userID:        user.ID,
		groupID:       group.ID,
		captured:      &capturedAuthContext{},
	}

	delegate := gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeySvc, nil, apiKeyCfg))
	countingDelegate := APIKeyAuthMiddleware(func(c *gin.Context) {
		f.delegateCalls.Add(1)
		delegate(c)
	})

	auth := OAuthOrAPIKeyAuth(
		countingDelegate,
		apiKeySvc,
		opts.subscriptions,
		f.keySvc,
		f.clientSvc,
		f.backingSvc,
		testIssuer,
		&config.Config{},
	)

	// captureReject wraps the auth middleware so the ingress-reject reason can
	// be read AFTER an abort. Nothing downstream of an aborting middleware
	// runs, so this has to sit upstream of it and inspect on the way out.
	captureReject := func(c *gin.Context) {
		c.Next()
		f.captured.rejectReason, f.captured.rejectOK = GetIngressRejectReason(c)
	}

	probe := func(c *gin.Context) {
		cap := f.captured
		cap.ran = true
		cap.apiKey, cap.apiKeyOK = GetAPIKeyFromContext(c)
		cap.subject, cap.subjectOK = GetAuthSubjectFromContext(c)
		cap.role, cap.roleOK = GetUserRoleFromContext(c)
		cap.opsFallback, cap.opsFallbackOK = GetOpsFallbackAPIKey(c)
		cap.subscription, cap.subscriptionOK = GetSubscriptionFromContext(c)
		cap.requestUserID = c.Request.Context().Value(ctxkey.UserID)
		if g, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok {
			cap.requestGroup = g
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}

	r := gin.New()
	r.POST("/v1/responses", captureReject, auth, probe)
	// The two endpoints on the /v1 chain that are NOT inference. They exist in
	// the fixture because requiredOAuthInferenceScope keys off the exact path.
	r.GET("/v1/usage", captureReject, auth, probe)
	r.GET("/v1/sub2api/billing", captureReject, auth, probe)
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
	return f.request(t, http.MethodPost, "/v1/responses", authorization, nil)
}

func (f *oauthInferenceFixture) request(t *testing.T, method, path, authorization string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(`{"model":"m","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	f.router.ServeHTTP(w, req)
	return w
}

// provision drives one successful request so the backing row exists, and
// returns its id.
func (f *oauthInferenceFixture) provision(t *testing.T, token string) int64 {
	t.Helper()
	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code, "provisioning request must succeed")
	require.True(t, f.captured.apiKeyOK)
	id := f.captured.apiKey.ID
	*f.captured = capturedAuthContext{}
	return id
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

// ---------------------------------------------------------------------------
// The properties the task exists to establish
// ---------------------------------------------------------------------------

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

	// A standard-type group has no subscription to load.
	require.False(t, f.captured.subscriptionOK, "a standard group must not publish a subscription")

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

// TestOrdinaryAPIKeyStillSucceedsOnTheSameRoute is the other half of the
// routing decision: anything that is not JWT-shaped is delegated untouched, so
// an ordinary key is authenticated exactly as it was before this middleware
// existed.
//
// "Byte-identically to today" is established by running the same request
// through the bare API-key middleware and comparing, rather than by restating
// what that middleware is believed to do -- and it is done in BOTH run modes,
// because simple mode takes an early return that skips the subscription load
// and the whole billing block.
func TestOrdinaryAPIKeyStillSucceedsOnTheSameRoute(t *testing.T) {
	for _, runMode := range []string{config.RunModeSimple, config.RunModeStandard} {
		t.Run(runMode, func(t *testing.T) {
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
			getByKey := func(context.Context, string) (*service.APIKey, error) { return stored, nil }

			f := newOAuthInferenceFixture(t)
			f.apiKeyRepo.getByKey = getByKey

			w := f.do(t, "Bearer sk-ordinary-key")

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			require.Equal(t, int32(1), f.delegateCalls.Load(), "an ordinary key must go through the API-key middleware")
			require.True(t, f.captured.apiKeyOK)
			require.Equal(t, int64(777), f.captured.apiKey.ID)
			require.Equal(t, "sk-ordinary-key", f.captured.apiKey.Key)

			// The same request through the bare API-key middleware, in the run
			// mode under test.
			baselineCfg := &config.Config{RunMode: runMode}
			baselineCaptured := &capturedAuthContext{}
			baselineRepo := &stubApiKeyRepo{getByKey: getByKey}
			baseline := gin.New()
			baseline.POST("/v1/responses",
				gin.HandlerFunc(NewAPIKeyAuthMiddleware(
					service.NewAPIKeyService(baselineRepo, nil, nil, nil, nil, nil, baselineCfg), nil, baselineCfg)),
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
		})
	}
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

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

// TestBillingEndpointsRequireBillingReadNotInferenceInvoke: /v1/usage and
// /v1/sub2api/billing ride the same group-level middleware as inference but are
// not inference -- they return the backing key's usage history and the group's
// billing multipliers. The scope vocabulary defines billing:read to gate
// exactly that, and an inference-only token must not reach them by virtue of
// sharing a mount.
func TestBillingEndpointsRequireBillingReadNotInferenceInvoke(t *testing.T) {
	for _, path := range []string{"/v1/usage", "/v1/sub2api/billing"} {
		t.Run(path, func(t *testing.T) {
			f := newOAuthInferenceFixture(t)

			inferenceOnly := f.inferenceToken(t, service.ScopeInferenceInvoke)
			w := f.request(t, http.MethodGet, path, "Bearer "+inferenceOnly, nil)
			require.Equal(t, http.StatusForbidden, w.Code,
				"inference:invoke alone must not reach a billing endpoint; body: %s", w.Body.String())
			require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
			require.False(t, f.captured.ran)

			billingReader := f.inferenceToken(t, service.ScopeBillingRead)
			w = f.request(t, http.MethodGet, path, "Bearer "+billingReader, nil)
			require.Equal(t, http.StatusOK, w.Code,
				"billing:read must reach the billing endpoints; body: %s", w.Body.String())
			require.True(t, f.captured.ran)
		})
	}
}

// TestBillingReadAloneCannotInfer is the other direction of the same line: the
// billing scope must not become an inference credential.
func TestBillingReadAloneCannotInfer(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeBillingRead)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"insufficient_scope"}`, w.Body.String())
}

// ---------------------------------------------------------------------------
// The kill switches: the checks CheckBillingEligibility does not make
// ---------------------------------------------------------------------------

// TestOAuthDisabledBackingKeyIsForbidden: disabling the backing row is the ONLY
// per-agent kill switch an operator has -- an access token cannot be revoked
// inside its window, and the downstream CheckBillingEligibility does not look at
// api_keys.status. If this middleware ignored the status, the kill switch would
// not work.
func TestOAuthDisabledBackingKeyIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)
	rowID := f.provision(t, token)

	require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
		SetStatus(service.StatusAPIKeyDisabled).
		Exec(context.Background()))

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"backing_key_disabled"}`, w.Body.String())
	require.False(t, f.captured.ran, "a disabled backing row must stop the agent")
	require.True(t, f.captured.rejectOK)
	require.Equal(t, IngressRejectAPIKeyDisabled, f.captured.rejectReason)
}

// TestOAuthExpiredBackingKeyIsForbidden. api_keys.expires_at is settable on a
// backing row through the ordinary key-management surface, and
// CheckBillingEligibility never calls IsExpired -- so without this check an
// expired backing row keeps serving inference forever.
func TestOAuthExpiredBackingKeyIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)
	rowID := f.provision(t, token)

	require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
		SetExpiresAt(time.Now().Add(-time.Hour)).
		Exec(context.Background()))

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"backing_key_expired"}`, w.Body.String())
	require.False(t, f.captured.ran)
	require.Equal(t, IngressRejectOAuthBackingKeyExpired, f.captured.rejectReason,
		"expiry must be greppable apart from a disabled key")
}

// TestOAuthQuotaExhaustedBackingKeyIsForbidden. The per-key quota ledger IS the
// backing row (api_keys.quota / quota_used, written by the billing settlement),
// and CheckBillingEligibility checks rate limits but never IsQuotaExhausted --
// so a quota set on a backing row would be collected and never enforced.
func TestOAuthQuotaExhaustedBackingKeyIsForbidden(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)
	rowID := f.provision(t, token)

	require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
		SetQuota(10).
		SetQuotaUsed(10).
		Exec(context.Background()))

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"backing_key_quota_exhausted"}`, w.Body.String())
	require.False(t, f.captured.ran)
	require.Equal(t, IngressRejectOAuthBackingKeyQuotaExhausted, f.captured.rejectReason,
		"quota exhaustion must be greppable apart from a disabled key")
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
	require.Equal(t, IngressRejectUserInactive, f.captured.rejectReason)
}

// TestOAuthDeletedOwnerIsForbiddenAndProvisionsNothing is C-1's middleware half.
//
// It is NOT a duplicate of TestOAuthInactiveOwnerIsForbidden. That one covers a
// status flag on a user who still exists; this one covers a user who has been
// DELETED, which before the fix travelled a completely different route: past
// the status check entirely, into OAuthBackingKeyService.Resolve, which
// INSERTed a fresh live backing row and only then failed -- 500 server_error,
// on every subsequent request, forever, for what is a deliberate
// administrative revocation.
//
// Three assertions, three distinct claims:
//
//   - 403 account_inactive, not 500: the classification. Removing the
//     ErrBackingKeyOwnerGone arm from the middleware's switch drops this to the
//     default 500 arm and this fails.
//   - IngressRejectUserInactive: the request is marked as an ingress rejection,
//     so a revoked user's agent does not fill ops_error_logs with what looks
//     like an outage.
//   - no live api_keys row afterwards: the resurrection is stopped. This is the
//     one that fails if the pre-INSERT liveness check is removed while the
//     sentinel stays (reload returns the same sentinel, so the status code
//     alone would not notice).
func TestOAuthDeletedOwnerIsForbiddenAndProvisionsNothing(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)
	ctx := context.Background()

	rowID := f.provision(t, token)

	// What AdminService.DeleteUser leaves behind: the backing row tombstoned
	// (it lists with IncludeOAuthBacking: true) and the user tombstoned.
	deletedAt := time.Now()
	require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).SetDeletedAt(deletedAt).Exec(ctx))
	require.NoError(t, f.entClient.User.UpdateOneID(f.userID).SetDeletedAt(deletedAt).Exec(ctx))

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"account_inactive"}`, w.Body.String(),
		"a deleted owner is an authorization outcome, not an infrastructure fault")
	require.False(t, f.captured.ran)
	require.Equal(t, IngressRejectUserInactive, f.captured.rejectReason)

	live, err := f.entClient.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, live,
		"the request must not have provisioned a fresh live backing row -- with a fresh secret -- for a deleted user")
}

// TestOAuthBackingKeyIPRestrictionIsEnforced is the fix for the review's
// highest finding. api_keys.ip_whitelist / ip_blacklist are editable on a
// backing row through the ordinary key-management surface (only Delete refuses
// backing rows, not Update) and the panel displays them, but nothing on the
// OAuth path compiled or consulted them -- so the control was displayed and
// inert.
//
// Both directions are asserted, because a check that denies everything would
// pass the deny half alone.
func TestOAuthBackingKeyIPRestrictionIsEnforced(t *testing.T) {
	// httptest.NewRequest's RemoteAddr, which is what GetSecurityClientIP
	// resolves to with forwarded-header trust off.
	const requestIP = "192.0.2.1"

	t.Run("denies an IP outside the whitelist", func(t *testing.T) {
		f := newOAuthInferenceFixture(t)
		token := f.inferenceToken(t, service.ScopeInferenceInvoke)
		rowID := f.provision(t, token)

		require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
			SetIPWhitelist([]string{"10.0.0.1"}).
			Exec(context.Background()))

		w := f.do(t, "Bearer "+token)

		require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
		require.JSONEq(t, `{"error":"ip_not_allowed"}`, w.Body.String())
		require.False(t, f.captured.ran, "the IP restriction must actually stop the request")
		require.Equal(t, IngressRejectIPRestricted, f.captured.rejectReason)
	})

	t.Run("admits an IP inside the whitelist", func(t *testing.T) {
		f := newOAuthInferenceFixture(t)
		token := f.inferenceToken(t, service.ScopeInferenceInvoke)
		rowID := f.provision(t, token)

		require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
			SetIPWhitelist([]string{requestIP}).
			Exec(context.Background()))

		w := f.do(t, "Bearer "+token)

		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		require.True(t, f.captured.ran)
	})

	t.Run("denies an IP on the blacklist", func(t *testing.T) {
		f := newOAuthInferenceFixture(t)
		token := f.inferenceToken(t, service.ScopeInferenceInvoke)
		rowID := f.provision(t, token)

		require.NoError(t, f.entClient.APIKey.UpdateOneID(rowID).
			SetIPBlacklist([]string{requestIP}).
			Exec(context.Background()))

		w := f.do(t, "Bearer "+token)

		require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
		require.JSONEq(t, `{"error":"ip_not_allowed"}`, w.Body.String())
		require.False(t, f.captured.ran)
	})
}

// ---------------------------------------------------------------------------
// Subscription
// ---------------------------------------------------------------------------

// TestOAuthSubscriptionGroupPublishesItsSubscription is the fix for the
// review's finding 3. CheckBillingEligibility computes
// `isSubscriptionMode := group.IsSubscriptionType() && subscription != nil`, so
// a nil subscription on a subscription-type group silently reroutes billing to
// the wallet -- and a subscription customer with a zero wallet is then
// hard-403'd INSUFFICIENT_BALANCE while holding a valid subscription.
func TestOAuthSubscriptionGroupPublishesItsSubscription(t *testing.T) {
	sub := &service.UserSubscription{ID: 31337, Status: service.StatusActive}
	loader := &stubOAuthSubscriptionLoader{subscription: sub}
	f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		subscriptions:    loader,
	})
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.True(t, f.captured.subscriptionOK, "ContextKeySubscription must be published for a subscription group")
	require.Equal(t, sub.ID, f.captured.subscription.ID)
	require.Equal(t, int32(1), loader.calls.Load())
	require.Equal(t, f.userID, loader.lastUserID.Load(), "the subscription is keyed on (user, group)")
	require.Equal(t, f.groupID, loader.lastGroupID.Load())
}

// TestOAuthStandardGroupDoesNotLoadASubscription: the loader must not be
// consulted at all for a standard-type group, which is the common case and the
// one that must stay at one round trip.
func TestOAuthStandardGroupDoesNotLoadASubscription(t *testing.T) {
	loader := &stubOAuthSubscriptionLoader{}
	f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{subscriptions: loader})
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code)
	require.Zero(t, loader.calls.Load(), "a standard group must not query the subscription service")
	require.False(t, f.captured.subscriptionOK)
}

// TestOAuthSubscriptionGroupWithNoActiveSubscriptionIsForbidden mirrors the
// API-key path's SUBSCRIPTION_NOT_FOUND: a subscription group with no active
// subscription must not silently fall through to wallet billing.
func TestOAuthSubscriptionGroupWithNoActiveSubscriptionIsForbidden(t *testing.T) {
	loader := &stubOAuthSubscriptionLoader{err: service.ErrSubscriptionNotFound}
	f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		subscriptions:    loader,
	})
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	w := f.do(t, "Bearer "+token)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	require.JSONEq(t, `{"error":"subscription_not_found"}`, w.Body.String())
	require.False(t, f.captured.ran)
	require.Equal(t, IngressRejectOAuthSubscriptionNotFound, f.captured.rejectReason)
}

// ---------------------------------------------------------------------------
// Operator-configuration failures
// ---------------------------------------------------------------------------

// TestOAuthNoGroupPolicyIsForbiddenNotUnauthorized: an operator configuration
// problem. A nil Group reaches platform routing and panics, so Resolve refuses
// rather than returning such a row -- and the refusal must not read to the
// client as a stale credential.
func TestOAuthNoGroupPolicyIsForbiddenNotUnauthorized(t *testing.T) {
	f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{configuredGroup: "no-such-group"})
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
	require.Equal(t, IngressRejectOAuthBackingGroupUnavailable, f.captured.rejectReason)
}

// ---------------------------------------------------------------------------
// A caller that has already gone away
// ---------------------------------------------------------------------------

// TestOAuthCancelledRequestIsNotAServerError. The brief names this as binding:
// a cancelled client request must not become a 500. It must also not become a
// 200 -- c.Abort() with no status leaves gin to emit its default 200 with an
// empty body whenever the connection is in fact still alive, which a client
// parses as a malformed success and the ops error logger never sees.
//
// Cancellation is detected on the REQUEST context rather than only on the
// returned error, because VerifyOAuthBearer's classifier discards the cause: a
// signing-key lookup cancelled mid-flight arrives with nothing for errors.Is to
// match.
func TestOAuthCancelledRequestIsNotAServerError(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := f.request(t, http.MethodPost, "/v1/responses", "Bearer "+token, ctx)

	require.Equal(t, statusClientClosedRequest, w.Code,
		"a cancelled caller is 499, not 500 and not 200; body: %s", w.Body.String())
	require.NotEqual(t, http.StatusInternalServerError, w.Code)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.False(t, f.captured.ran)
}

// TestOAuthExpiredDeadlineIsAGatewayTimeout: the other half of the same arm.
func TestOAuthExpiredDeadlineIsAGatewayTimeout(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	w := f.request(t, http.MethodPost, "/v1/responses", "Bearer "+token, ctx)

	require.Equal(t, http.StatusGatewayTimeout, w.Code,
		"an expired deadline is 504, not 500 and not 200; body: %s", w.Body.String())
	require.False(t, f.captured.ran)
}

// ---------------------------------------------------------------------------
// Attribution and shape
// ---------------------------------------------------------------------------

// TestOAuthSuccessTouchesTheBackingRowsLastUsed. last_used_at on the backing
// row is the only per-agent activity signal an operator has: the row is never
// presented as a credential, so there is no auth-cache hit to infer it from.
// Asserted through the real APIKeyService down to its repository seam.
func TestOAuthSuccessTouchesTheBackingRowsLastUsed(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code)

	require.NotZero(t, f.touchedKeyID.Load(), "TouchLastUsed must have run for the backing row")
	require.Equal(t, f.captured.apiKey.ID, f.touchedKeyID.Load())
}

// TestOAuthBillingInfoRequestDoesNotTouchLastUsed matches the API-key path,
// which skips the touch on /v1/sub2api/billing: reading your own multipliers is
// not usage.
func TestOAuthBillingInfoRequestDoesNotTouchLastUsed(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeBillingRead)

	require.Equal(t, http.StatusOK, f.request(t, http.MethodGet, "/v1/sub2api/billing", "Bearer "+token, nil).Code)

	require.Zero(t, f.touchedKeyID.Load(), "the billing-info endpoint must not record usage")
}

// TestNonBase64URLCredentialIsDelegatedNotOAuthBranched exercises the shape
// test's alphabet, which is the boundary both F-A and F-B hang off. The
// credential below has three non-empty dot-separated segments but characters
// outside base64url, so it is not JWT-shaped and belongs to the API-key path.
//
// An isBase64URLSegment that always returned true would send it to the OAuth
// branch and answer 401 invalid_token with the delegate never entered.
func TestNonBase64URLCredentialIsDelegatedNotOAuthBranched(t *testing.T) {
	f := newOAuthInferenceFixture(t)

	w := f.do(t, "Bearer not+base64.seg!ment.here=")

	require.Equal(t, int32(1), f.delegateCalls.Load(),
		"a credential outside the base64url alphabet is not JWT-shaped and belongs to the API-key path")
	require.NotEqual(t, `{"error":"invalid_token"}`, strings.TrimSpace(w.Body.String()))
}

// TestLooksLikeJWTRoutesOnShapeAlone is the routing table for the shape test.
// Each row is a routing decision, not a restatement of the implementation: the
// true rows go to the OAuth branch and can never be authenticated as API keys,
// and the false rows go to the API-key path and can never be answered as
// tokens.
func TestLooksLikeJWTRoutesOnShapeAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"three base64url segments", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln", true},
		{"segments using - and _", "a-b_c.d-e_f.g-h_i", true},
		{"an ordinary api key has no dots", "sk-0123456789abcdef", false},
		{"two segments", "header.payload", false},
		{"four segments (JWE, not JWS)", "a.b.c.d", false},
		{"empty signature segment (alg=none)", "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.", false},
		{"empty header segment", ".eyJzdWIiOiIxIn0.c2ln", false},
		{"empty middle segment", "eyJhbGciOiJSUzI1NiJ9..c2ln", false},
		{"base64 padding is not base64url", "aGVhZGVy.cGF5bG9hZA==.c2ln", false},
		{"standard-base64 + and /", "a+b.c/d.efg", false},
		{"empty credential", "", false},
		{"dots only", "..", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, looksLikeJWT(tc.raw))
		})
	}
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

// TestOAuthRejectionsAreMarkedAsIngressRejects: an unmarked 4xx is captured
// into ops_error_logs as an operational request error. A client presenting a
// bad or under-scoped token is not an operational error, and the reasons are
// OAuth-specific so an operator can tell a real agent's credential failure
// apart from a script spraying garbage keys -- the exact complaint the gap
// evidence records.
func TestOAuthRejectionsAreMarkedAsIngressRejects(t *testing.T) {
	f := newOAuthInferenceFixture(t)

	claims := baseTestClaims(f.userID, testAudienceClientID, service.ScopeInferenceInvoke, time.Now().Add(time.Hour))
	w := f.do(t, "Bearer "+mintTestES256Token(t, claims))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.True(t, f.captured.rejectOK, "an invalid token must be marked as an ingress reject")
	require.Equal(t, IngressRejectOAuthInvalidToken, f.captured.rejectReason)

	*f.captured = capturedAuthContext{}
	w = f.do(t, "Bearer "+f.inferenceToken(t, service.ScopeBillingRead))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, f.captured.rejectOK)
	require.Equal(t, IngressRejectOAuthInsufficientScope, f.captured.rejectReason)
}

// TestOAuthSuccessIsNotMarkedAsAnIngressReject: the mark must not be
// unconditional, or every successful inference request would be excluded from
// ops error logging.
func TestOAuthSuccessIsNotMarkedAsAnIngressReject(t *testing.T) {
	f := newOAuthInferenceFixture(t)
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code)
	require.False(t, f.captured.rejectOK, "a successful request is not an ingress rejection")
}
