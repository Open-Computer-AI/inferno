//go:build unit

package middleware

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// The zero-balance divergence, pinned as a DECISION rather than left as an
// observation.
//
// The conformance run found that `GET /v1/models` answers 200 for an OAuth
// token and 403 INSUFFICIENT_BALANCE for an ordinary API key, same user, same
// zero balance. Two independent whole-branch reviews adjudicated it the same
// way: document it; add no auth-time balance gate to either path.
//
//   - Not to the OAuth branch: an auth-time `balance <= 0` gate blocks
//     NON-BILLABLE endpoints too, so an agent that has run to zero would lose
//     the ability to discover that it has -- and model listing is on the hermes
//     client's startup path.
//   - Not removed from the API-key path: it is pre-existing, load-bearing, and
//     "API-key auth keeps working everywhere it works today" is this branch's
//     binding constraint.
//
// WHY THIS TEST EXISTS, WHICH IS THE WHOLE POINT. Adding the gate to the OAuth
// branch today would break NOTHING. So a future edit made in good faith ("the
// two credential paths should obviously agree") could silently reverse a
// decision that took two whole-branch reviews to reach, and every gate would
// stay green. This test is what makes that reversal loud.
//
// Read the direction correctly: the OAuth branch is not MISSING a check. The
// API-key path gates a non-billable endpoint at auth time -- a pre-existing
// over-restriction -- and the OAuth branch declined to copy it. No billable
// work is reachable at zero balance on either path; POST /v1/images/batches is
// the one that looked like a bypass and is not, because
// reserveBatchImageBalanceHold enforces balance at the database.
//
// The membership half of the property -- "no NEW /v1 route joins this class
// unnoticed" -- is pinned separately and structurally in
// internal/server/routes/gateway_billing_divergence_test.go, because that is
// where the route table lives.

// nonBillableClassRoutes is the class, AS INDEPENDENTLY VERIFIED for this fix
// wave rather than copied from either review's enumeration. Both reviews
// over-counted it; see nearMissRoutes below for the four groups that are not
// members and why.
//
// Membership test: the route is on the OAuth-mounted chain, is NOT in the
// API-key path's skipBilling set (/v1/usage, /v1/sub2api/billing,
// GET /v1/images/tasks/:task_id), and no handler it can reach calls
// CheckBillingEligibility. Traced per route:
//
//   - GET /v1/models, GET /models -> modelsHandler -> GatewayHandler.Models
//     (gateway_handler.go:1073) or OpenAIGatewayHandler.CodexModels
//     (openai_codex_models_handler.go). gateway_handler.go's three
//     CheckBillingEligibility calls are at :242 and :968 (both inside Messages)
//     and :2040 (CountTokens); the codex models file has zero.
//   - the batch-image job surface -> BatchImageHandler
//     (batch_image_handler.go), which contains ZERO CheckBillingEligibility in
//     the whole file. POST /v1/images/batches is excluded deliberately: it is
//     the one that does billable work, and it is NOT divergent because
//     reserveBatchImageBalanceHold enforces balance at the database
//     (`... AND balance >= $1`), which both reviews independently confirmed and
//     which is the fact that makes documenting this class acceptable.
//   - GET /v1/live/:call_id -> OpenAIGatewayHandler.LiveSideband
//     (openai_live.go:188). The file's only CheckBillingEligibility is at :71,
//     inside Live (the POST).
//
// The middleware keys off the exact path only for scope selection, so
// registering these paths on the real auth chain reproduces the auth-time
// behaviour exactly, without standing up a single handler. Auth-time is the
// only place the divergence lives.
//
// Keep in step with the table in backend/scripts/oauth-conformance.md section 10
// and with the route-set table in
// internal/server/routes/gateway_billing_divergence_test.go.
var nonBillableClassRoutes = []extraRoute{
	{http.MethodGet, "/v1/models"},
	{http.MethodGet, "/models"},
	{http.MethodGet, "/v1/images/batches"},
	{http.MethodGet, "/v1/images/batches/models"},
	{http.MethodGet, "/v1/images/batches/:id"},
	{http.MethodGet, "/v1/images/batches/:id/items"},
	{http.MethodGet, "/v1/images/batches/:id/items/:custom_id/content"},
	{http.MethodGet, "/v1/images/batches/:id/download"},
	{http.MethodPost, "/v1/images/batches/:id/cancel"},
	{http.MethodDelete, "/v1/images/batches/:id"},
	{http.MethodDelete, "/v1/images/batches/:id/outputs"},
	{http.MethodGet, "/v1/live/:call_id"},
}

// nonBillableClassRequestPaths are the concrete paths used to drive the routes
// above, in the same order: gin route patterns need real segments to match.
var nonBillableClassRequestPaths = []string{
	"/v1/models",
	"/models",
	"/v1/images/batches",
	"/v1/images/batches/models",
	"/v1/images/batches/7",
	"/v1/images/batches/7/items",
	"/v1/images/batches/7/items/c-1/content",
	"/v1/images/batches/7/download",
	"/v1/images/batches/7/cancel",
	"/v1/images/batches/7",
	"/v1/images/batches/7/outputs",
	"/v1/live/call-1",
}

// nearMissRoutes are the routes BOTH whole-branch reviews listed as members of
// the non-billable class and which are NOT members. Recorded here, with a test,
// because the reviews warned that "the handler has no CheckBillingEligibility"
// is not a sufficient membership test -- and then both made the mirror-image
// error on these four groups by reading the ROUTE's immediate handler rather
// than following it through.
//
//   - GET /v1/videos/** (status + content) -> videoStatusHandler /
//     videoContentHandler -> OpenAIGatewayHandler.GrokVideoStatus /
//     GrokVideoContent -> handleGrokMedia (grok_media.go:54), which calls
//     CheckBillingEligibility at :153.
//   - GET /v1/custom-voices, /:voice_id, /:voice_id/audio -> voiceHandler /
//     customVoicePathHandler -> OpenAIGatewayHandler.GrokVoice
//     (grok_audio.go:132), which calls CheckBillingEligibility at :142.
//   - GET /v1/realtime -> OpenAIGatewayHandler.GrokRealtime (grok_audio.go:24),
//     which calls CheckBillingEligibility at :38.
//   - GET /v1/responses -> OpenAIGatewayHandler.ResponsesWebSocket
//     (openai_gateway_handler.go:1605), which calls CheckBillingEligibility at
//     :1824.
//
// They ARE still auth-time divergent -- an API key is refused before the
// handler and an OAuth token reaches it -- but the OUTCOME is not divergent,
// because the handler refuses a zero-balance caller itself. (For a non-Grok
// group the four gate on platform and answer 404 "not supported for this
// platform" before billing; a 404 is not billable work either.) The distinction
// matters: this is the group where "documented divergence" is about the error
// SHAPE, not about access.
var nearMissRoutes = []extraRoute{
	{http.MethodGet, "/v1/videos/:request_id"},
	{http.MethodGet, "/v1/videos/:request_id/content"},
	{http.MethodGet, "/v1/custom-voices"},
	{http.MethodGet, "/v1/custom-voices/:voice_id"},
	{http.MethodGet, "/v1/realtime"},
	{http.MethodGet, "/v1/responses"},
}

var nearMissRequestPaths = []string{
	"/v1/videos/abc",
	"/v1/videos/abc/content",
	"/v1/custom-voices",
	"/v1/custom-voices/v-1",
	"/v1/realtime",
	"/v1/responses",
}

// zeroBalanceDelegateKey is what the real API-key middleware is handed: a valid,
// active key whose owner has a zero balance. Everything about it passes auth
// except the balance.
func zeroBalanceDelegateKey() *service.APIKey {
	groupID := int64(1)
	return &service.APIKey{
		ID:      99,
		UserID:  1,
		Key:     "sk-an-ordinary-key-with-no-money-behind-it",
		Name:    "ordinary",
		Status:  service.StatusActive,
		GroupID: &groupID,
		Group: &service.Group{
			ID:               groupID,
			Name:             "oauth-agents",
			Platform:         service.PlatformAnthropic,
			Status:           service.StatusActive,
			SubscriptionType: service.SubscriptionTypeStandard,
			RateMultiplier:   1,
		},
		User: &service.User{
			ID:      1,
			Status:  service.StatusActive,
			Role:    service.RoleUser,
			Balance: 0,
		},
	}
}

// TestZeroBalanceDivergenceOnTheNonBillableClass asserts BOTH sides of the
// adjudicated decision on every route in the class.
//
// Both halves are required. The OAuth half alone would pass if the API-key gate
// were deleted (which is the change the reviews refused); the API-key half alone
// would pass if the OAuth branch grew the gate (the other change they refused).
// Only together do they pin the decision that was actually made.
func TestZeroBalanceDivergenceOnTheNonBillableClass(t *testing.T) {
	require.Equal(t, len(nonBillableClassRoutes), len(nonBillableClassRequestPaths),
		"every declared route needs a concrete path to drive it")

	for i, route := range nonBillableClassRoutes {
		path := nonBillableClassRequestPaths[i]
		t.Run(route.method+" "+path, func(t *testing.T) {
			f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{
				// Standard, NOT SimpleMode: the API-key path's auth-time
				// balance gate lives inside the block SimpleMode returns before,
				// so under SimpleMode the API-key half of this test would pass
				// vacuously.
				apiKeyRunMode:  config.RunModeStandard,
				delegateAPIKey: zeroBalanceDelegateKey(),
				extraRoutes:    nonBillableClassRoutes,
			})

			// The fixture's own user is created with the default zero balance,
			// so no funding step is needed on the OAuth side -- and that is
			// exactly the condition under test.
			token := f.inferenceToken(t, service.ScopeInferenceInvoke)
			oauthResp := f.request(t, route.method, path, "Bearer "+token, nil)

			require.Equal(t, http.StatusOK, oauthResp.Code,
				"an OAuth agent at zero balance must still reach a non-billable endpoint; "+
					"adding an auth-time balance gate here would stop an agent DISCOVERING it is at zero. "+
					"body: %s", oauthResp.Body.String())
			require.True(t, f.captured.ran, "the request must have got past auth")
			require.Zero(t, f.delegateCalls.Load(), "the OAuth branch must not fall through")

			*f.captured = capturedAuthContext{}

			apiKeyResp := f.request(t, route.method, path, "Bearer sk-an-ordinary-key-with-no-money-behind-it", nil)

			require.Equal(t, int32(1), f.delegateCalls.Load(), "the API-key path must have run")
			require.Equal(t, http.StatusForbidden, apiKeyResp.Code,
				"the API-key path's pre-existing auth-time balance gate must keep firing; "+
					"removing it would change behaviour for every existing customer. body: %s",
				apiKeyResp.Body.String())
			require.Contains(t, apiKeyResp.Body.String(), "INSUFFICIENT_BALANCE")
			require.False(t, f.captured.ran, "the API-key request must NOT have got past auth")
		})
	}
}

// TestZeroBalanceAuthDivergenceOnTheNearMissRoutes records the correction to
// both reviews' enumeration as an executable claim.
//
// These routes diverge at AUTH time exactly as the class above does -- which is
// why they are worth pinning -- but their handlers enforce billing themselves,
// so the OUTCOME is not divergent. Only the auth-time half is asserted here,
// because that is the only half this fixture (whose handler is a probe) can
// honestly observe. The handler-side reasoning, with file:line for every one, is
// on nearMissRoutes.
func TestZeroBalanceAuthDivergenceOnTheNearMissRoutes(t *testing.T) {
	require.Equal(t, len(nearMissRoutes), len(nearMissRequestPaths))

	for i, route := range nearMissRoutes {
		path := nearMissRequestPaths[i]
		t.Run(route.method+" "+path, func(t *testing.T) {
			f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{
				apiKeyRunMode:  config.RunModeStandard,
				delegateAPIKey: zeroBalanceDelegateKey(),
				extraRoutes:    nearMissRoutes,
			})
			token := f.inferenceToken(t, service.ScopeInferenceInvoke)

			require.Equal(t, http.StatusOK, f.request(t, route.method, path, "Bearer "+token, nil).Code,
				"auth must not be where the balance refusal happens on the OAuth path")
			require.True(t, f.captured.ran)

			*f.captured = capturedAuthContext{}
			apiKeyResp := f.request(t, route.method, path, "Bearer sk-an-ordinary-key-with-no-money-behind-it", nil)
			require.Equal(t, http.StatusForbidden, apiKeyResp.Code, "body: %s", apiKeyResp.Body.String())
			require.False(t, f.captured.ran)
		})
	}
}

// TestZeroBalanceOAuthStillReachesTheBillableRoutesAuth is the control that
// stops the test above being read as "OAuth skips billing".
//
// A zero-balance OAuth agent gets past AUTH on a billable route too -- and is
// then refused by CheckBillingEligibility inside the handler, which is the
// design (the conformance run recorded exactly that: 403 insufficient balance on
// every billable endpoint). So the divergence is confined to WHERE the refusal
// happens, on routes that never bill, and not to WHETHER unfunded work is
// billable.
func TestZeroBalanceOAuthStillReachesTheBillableRoutesAuth(t *testing.T) {
	f := newOAuthInferenceFixtureWith(t, oauthInferenceOptions{
		apiKeyRunMode:  config.RunModeStandard,
		delegateAPIKey: zeroBalanceDelegateKey(),
	})
	token := f.inferenceToken(t, service.ScopeInferenceInvoke)

	require.Equal(t, http.StatusOK, f.do(t, "Bearer "+token).Code,
		"POST /v1/responses is billable, and auth is not where its balance refusal lives")
	require.True(t, f.captured.ran)
}
