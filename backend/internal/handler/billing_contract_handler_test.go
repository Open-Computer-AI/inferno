package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type billingHandlerBalance struct {
	balance float64
	err     error
	gotUser int64
}

func (b *billingHandlerBalance) GetUserBalance(_ context.Context, userID int64) (float64, error) {
	b.gotUser = userID
	return b.balance, b.err
}

type billingHandlerOrg struct{}

func (billingHandlerOrg) OrgsForUser(context.Context, int64) ([]*dbent.Org, error) {
	return []*dbent.Org{{ID: 1, Slug: "acme-1a2b", Name: "Acme"}}, nil
}
func (billingHandlerOrg) RoleIn(context.Context, int64, int64) (string, error) {
	return service.OrgRoleOwner, nil
}

type billingHandlerUsage struct{}

func (billingHandlerUsage) GetStatsByUser(context.Context, int64, time.Time, time.Time) (*service.UsageStats, error) {
	return &service.UsageStats{TotalActualCost: 1.25}, nil
}

type billingHandlerPayment struct{}

func (billingHandlerPayment) GetAvailableMethodLimits(context.Context) (*service.MethodLimitsResponse, error) {
	return &service.MethodLimitsResponse{GlobalMin: 5, GlobalMax: 500}, nil
}
func (billingHandlerPayment) IsPaymentEnabled(context.Context) bool { return true }

type billingHandlerPlan struct{}

func (billingHandlerPlan) ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return []*dbent.SubscriptionPlan{{ID: 100, GroupID: 9, Name: "Pro", Price: 20, ForSale: true}}, nil
}
func (billingHandlerPlan) GetGroupInfoMap(context.Context, []*dbent.SubscriptionPlan) map[int64]service.PlanGroupInfo {
	return nil
}

// billingHandlerSubscription records the userID it was actually called with
// -- the IDOR guard for GET /api/billing/subscription, same shape as
// billingHandlerBalance's gotUser for GET /api/billing/state.
type billingHandlerSubscription struct {
	gotUser int64
	// subs is what ListActiveUserSubscriptions returns for any user. Zero
	// value is nil -- "no active subscription" -- so every existing caller
	// of &billingHandlerSubscription{} keeps its original behaviour; Task 4's
	// oauth_handler_test.go sets it to exercise the has-a-subscription path.
	subs []service.UserSubscription
}

func (s *billingHandlerSubscription) ListActiveUserSubscriptions(_ context.Context, userID int64) ([]service.UserSubscription, error) {
	s.gotUser = userID
	return s.subs, nil
}

// newBillingContractHandlerUnderTest builds the handler over a real
// BillingContractService whose balance source the caller controls.
func newBillingContractHandlerUnderTest(bal *billingHandlerBalance) *BillingContractHandler {
	return newBillingContractHandlerUnderTestWithSub(bal, &billingHandlerSubscription{})
}

func newBillingContractHandlerUnderTestWithSub(bal *billingHandlerBalance, sub *billingHandlerSubscription) *BillingContractHandler {
	return NewBillingContractHandler(service.NewBillingContractService(
		bal, billingHandlerOrg{}, billingHandlerUsage{}, billingHandlerPayment{}, billingHandlerPlan{}, sub, "https://portal.example.com",
	))
}

// serveBillingState invokes State with whatever the middleware would have (or
// would not have) put on the context. No middleware runs here on purpose:
// these tests are about what the handler does with the identity, and the
// middleware's own behaviour is covered in server/routes and server/middleware.
func serveBillingState(h *BillingContractHandler, setContext func(*gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/billing/state?user_id=999", nil)
	if setContext != nil {
		setContext(c)
	}
	h.State(c)
	return rec
}

// TestBillingStateHandlerReadsTheUserFromTheVerifiedBearerOnly is the IDOR
// guard. The request carries ?user_id=999; the response must be about user 7,
// the id RequireOAuthScope put on the context after verifying the token's
// signature, issuer, audience and expiry.
func TestBillingStateHandlerReadsTheUserFromTheVerifiedBearerOnly(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingState(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, int64(7), bal.gotUser,
		"the composed state must be about the bearer's user, never a request-supplied id")
	require.NotEqual(t, int64(999), bal.gotUser)
}

// TestBillingStateHandlerRejectsAMissingOAuthIdentity: reachable only if the
// route is ever registered without RequireOAuthScope. It must not compose a
// response for user 0.
func TestBillingStateHandlerRejectsAMissingOAuthIdentity(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingState(h, nil)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
	require.Zero(t, bal.gotUser, "no source may be consulted without a verified identity")
}

// TestBillingStateHandlerRejectsAWrongTypedOAuthIdentity covers the type
// assertion: a string "7" on the context is not an int64 and must fail closed
// rather than resolve to user 0.
func TestBillingStateHandlerRejectsAWrongTypedOAuthIdentity(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingState(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, "7")
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
	require.Zero(t, bal.gotUser)
}

// TestBillingStateHandlerReports500WithoutLeakingTheCause: the client fails
// open on a 500 and shows a clean message, so the real cause belongs in the
// server log (asserted by inspection of the handler) and NOT in the body.
// Leaking it would hand an unauthenticated-adjacent caller our internals.
func TestBillingStateHandlerReports500WithoutLeakingTheCause(t *testing.T) {
	bal := &billingHandlerBalance{err: errors.New("redis down: dial tcp 10.0.0.5:6379: connect: connection refused")}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingState(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"server_error"}`, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "10.0.0.5")
	require.NotContains(t, rec.Body.String(), "redis")
}

// TestBillingStateHandlerReports500WhenUnwired: a Handlers literal built by a
// route test can leave the service nil. Fail closed rather than panic — a
// panic in a handler is a 500 too, but it takes the whole request goroutine's
// stack into the log on every poll.
func TestBillingStateHandlerReports500WhenUnwired(t *testing.T) {
	h := NewBillingContractHandler(nil)

	rec := serveBillingState(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"server_error"}`, rec.Body.String())
}

// serveBillingSubscription mirrors serveBillingState for GET
// /api/billing/subscription.
func serveBillingSubscription(h *BillingContractHandler, setContext func(*gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/billing/subscription?user_id=999", nil)
	if setContext != nil {
		setContext(c)
	}
	h.Subscription(c)
	return rec
}

// TestBillingSubscriptionHandlerReadsTheUserFromTheVerifiedBearerOnly is the
// IDOR guard for GET /api/billing/subscription, same shape as State's.
func TestBillingSubscriptionHandlerReadsTheUserFromTheVerifiedBearerOnly(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	sub := &billingHandlerSubscription{}
	h := newBillingContractHandlerUnderTestWithSub(bal, sub)

	rec := serveBillingSubscription(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, int64(7), sub.gotUser,
		"the composed subscription must be about the bearer's user, never a request-supplied id")
	require.NotEqual(t, int64(999), sub.gotUser)
}

// TestBillingSubscriptionHandlerRejectsAMissingOAuthIdentity mirrors State's.
func TestBillingSubscriptionHandlerRejectsAMissingOAuthIdentity(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingSubscription(h, nil)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestBillingSubscriptionHandlerRejectsAWrongTypedOAuthIdentity mirrors State's.
func TestBillingSubscriptionHandlerRejectsAWrongTypedOAuthIdentity(t *testing.T) {
	bal := &billingHandlerBalance{balance: 42.50}
	h := newBillingContractHandlerUnderTest(bal)

	rec := serveBillingSubscription(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, "7")
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestBillingSubscriptionHandlerReports500WhenUnwired mirrors State's.
func TestBillingSubscriptionHandlerReports500WhenUnwired(t *testing.T) {
	h := NewBillingContractHandler(nil)

	rec := serveBillingSubscription(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"server_error"}`, rec.Body.String())
}
