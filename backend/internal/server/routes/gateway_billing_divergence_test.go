package routes

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The membership half of the zero-balance divergence, pinned structurally.
//
// The BEHAVIOUR -- an OAuth token at zero balance passes on the non-billable
// class while an API key is refused at auth -- is pinned in
// internal/server/middleware/oauth_billing_divergence_test.go, against the real
// middlewares. It cannot also live here, because this package's fixture stubs
// the API-key middleware.
//
// What lives here is the question that package cannot answer: **has a new /v1
// route joined the class without anyone noticing?** The /v1 mount is a
// group-level Use, so every route added to the group inherits the OAuth branch
// automatically -- there is no per-route decision to forget, which is exactly
// why a new route can join a documented divergence class silently.
//
// The mechanical assertion is set equality over the /v1 route table. The
// classification beside each route is the human judgement the assertion FORCES:
// a new route fails this test until someone puts it in a bucket, and the bucket
// names the question they have to answer. No test can verify the judgement
// itself without static analysis of every handler; this one makes sure the
// judgement is made.

// billingClass is what a /v1 route's relationship to balance enforcement is.
type billingClass int

const (
	// classSkipBilling: in the API-key path's skipBilling set. Both credential
	// paths behave identically; no divergence.
	classSkipBilling billingClass = iota
	// classEnforced: the route reaches a handler that refuses a zero-balance
	// caller -- CheckBillingEligibility, or (POST /v1/images/batches) the
	// database-level hold in reserveBatchImageBalanceHold. Divergence is limited
	// to the error shape and WHERE the refusal happens.
	classEnforced
	// classNonBillableDivergent: the adjudicated class. No handler it reaches
	// refuses a zero-balance caller, so an OAuth token proceeds where an API key
	// is refused at auth. Documented, deliberately not gated. Adding a route
	// here is a decision, not an accident.
	classNonBillableDivergent
)

type classifiedRoute struct {
	method string
	path   string
	class  billingClass
}

// v1RouteBillingClassification is every route the /v1 group registers, i.e.
// every route the OAuth branch covers through the group-level Use.
//
// Root-level aliases are excluded: they are registered individually, and only
// six of them carry oauthOrAPIKeyAuth (gateway_oauth_mount_test.go's
// oauthMountedRoutes is the authority on which). GET /models is the only root
// alias in the divergent class and it is covered by the behavioural test.
var v1RouteBillingClassification = []classifiedRoute{
	// --- skipBilling: identical on both credential paths -------------------
	{http.MethodGet, "/v1/usage", classSkipBilling},
	{http.MethodGet, "/v1/sub2api/billing", classSkipBilling},
	{http.MethodGet, "/v1/images/tasks/:task_id", classSkipBilling},

	// --- the adjudicated non-billable divergent class ----------------------
	{http.MethodGet, "/v1/models", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches/models", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches/:id", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches/:id/items", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches/:id/items/:custom_id/content", classNonBillableDivergent},
	{http.MethodGet, "/v1/images/batches/:id/download", classNonBillableDivergent},
	{http.MethodPost, "/v1/images/batches/:id/cancel", classNonBillableDivergent},
	{http.MethodDelete, "/v1/images/batches/:id", classNonBillableDivergent},
	{http.MethodDelete, "/v1/images/batches/:id/outputs", classNonBillableDivergent},
	{http.MethodGet, "/v1/live/:call_id", classNonBillableDivergent},

	// --- balance enforced somewhere the caller cannot get past -------------
	{http.MethodPost, "/v1/messages", classEnforced},
	{http.MethodPost, "/v1/messages/count_tokens", classEnforced},
	{http.MethodPost, "/v1/chat/completions", classEnforced},
	{http.MethodPost, "/v1/embeddings", classEnforced},
	{http.MethodPost, "/v1/responses", classEnforced},
	{http.MethodPost, "/v1/responses/*subpath", classEnforced},
	{http.MethodGet, "/v1/responses", classEnforced},
	{http.MethodPost, "/v1/alpha/search", classEnforced},
	{http.MethodPost, "/v1/live", classEnforced},
	{http.MethodPost, "/v1/images/generations", classEnforced},
	{http.MethodPost, "/v1/images/edits", classEnforced},
	{http.MethodPost, "/v1/images/generations/async", classEnforced},
	{http.MethodPost, "/v1/images/edits/async", classEnforced},
	{http.MethodPost, "/v1/images/batches", classEnforced},
	{http.MethodPost, "/v1/videos", classEnforced},
	{http.MethodPost, "/v1/videos/generations", classEnforced},
	{http.MethodPost, "/v1/videos/edits", classEnforced},
	{http.MethodPost, "/v1/videos/extensions", classEnforced},
	{http.MethodGet, "/v1/videos/:request_id", classEnforced},
	{http.MethodGet, "/v1/videos/:request_id/content", classEnforced},
	{http.MethodGet, "/v1/videos/generations/:request_id", classEnforced},
	{http.MethodGet, "/v1/videos/generations/:request_id/content", classEnforced},
	{http.MethodGet, "/v1/videos/edits/:request_id", classEnforced},
	{http.MethodGet, "/v1/videos/edits/:request_id/content", classEnforced},
	{http.MethodGet, "/v1/videos/extensions/:request_id", classEnforced},
	{http.MethodGet, "/v1/videos/extensions/:request_id/content", classEnforced},
	{http.MethodPost, "/v1/tts", classEnforced},
	{http.MethodPost, "/v1/stt", classEnforced},
	{http.MethodPost, "/v1/custom-voices", classEnforced},
	{http.MethodGet, "/v1/custom-voices", classEnforced},
	{http.MethodGet, "/v1/custom-voices/:voice_id", classEnforced},
	{http.MethodGet, "/v1/custom-voices/:voice_id/audio", classEnforced},
	{http.MethodPatch, "/v1/custom-voices/:voice_id", classEnforced},
	{http.MethodDelete, "/v1/custom-voices/:voice_id", classEnforced},
	{http.MethodGet, "/v1/realtime", classEnforced},
	{http.MethodPost, "/v1/web_search", classEnforced},
	{http.MethodPost, "/v1/x_search", classEnforced},
}

func routeKey(method, path string) string { return method + " " + path }

// TestEveryV1RouteIsClassifiedAgainstTheBalanceDivergence is the control.
//
// It fails in both directions: a route added to /v1 without a classification,
// and a classification left behind for a route that no longer exists. The
// failure message tells the next author what decision they are being asked to
// make, because "add it to the list to make the test pass" is exactly the wrong
// response and the message has to say so.
func TestEveryV1RouteIsClassifiedAgainstTheBalanceDivergence(t *testing.T) {
	router, _ := newOAuthMountTestRouter(t)

	actual := make([]string, 0, len(v1RouteBillingClassification))
	for _, r := range router.Routes() {
		if r.Path == "/v1" || strings.HasPrefix(r.Path, "/v1/") {
			actual = append(actual, routeKey(r.Method, r.Path))
		}
	}
	declared := make([]string, 0, len(v1RouteBillingClassification))
	for _, r := range v1RouteBillingClassification {
		declared = append(declared, routeKey(r.method, r.path))
	}
	sort.Strings(actual)
	sort.Strings(declared)

	require.ElementsMatch(t, declared, actual,
		"every /v1 route inherits the OAuth branch through the group-level Use, so a new one joins "+
			"the documented zero-balance divergence class automatically. Before adding it below, decide "+
			"which bucket it is in: does anything it reaches refuse a zero-balance caller? If nothing "+
			"does, it is classNonBillableDivergent and must also be added to "+
			"middleware/oauth_billing_divergence_test.go and to the table in "+
			"backend/scripts/oauth-conformance.md section 10.")
}

// TestTheDivergentClassIsExactlyWhatIsDocumented pins the class membership
// itself, separately from the total, so that reclassifying an existing route
// out of the divergent bucket is also a visible edit.
func TestTheDivergentClassIsExactlyWhatIsDocumented(t *testing.T) {
	got := make([]string, 0, 11)
	for _, r := range v1RouteBillingClassification {
		if r.class == classNonBillableDivergent {
			got = append(got, routeKey(r.method, r.path))
		}
	}
	sort.Strings(got)

	// Spelled out rather than derived, so this is a second, independent
	// statement of the class and not a restatement of the table above.
	want := []string{
		"DELETE /v1/images/batches/:id",
		"DELETE /v1/images/batches/:id/outputs",
		"GET /v1/images/batches",
		"GET /v1/images/batches/:id",
		"GET /v1/images/batches/:id/download",
		"GET /v1/images/batches/:id/items",
		"GET /v1/images/batches/:id/items/:custom_id/content",
		"GET /v1/images/batches/models",
		"GET /v1/live/:call_id",
		"GET /v1/models",
		"POST /v1/images/batches/:id/cancel",
	}
	require.Equal(t, want, got,
		"the divergent class is a decision two whole-branch reviews took; changing its membership "+
			"is a decision, not a refactor")
}
