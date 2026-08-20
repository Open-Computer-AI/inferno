package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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

func (billingHandlerUsage) ListByUser(_ context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return []service.UsageLog{{UserID: userID, ActualCost: 1.25, TotalCost: 1.25}},
		&pagination.PaginationResult{Total: 1, Page: params.Page, PageSize: params.Limit()}, nil
}

type billingHandlerPayment struct{}

func (billingHandlerPayment) GetAvailableMethodLimits(context.Context) (*service.MethodLimitsResponse, error) {
	return &service.MethodLimitsResponse{GlobalMin: 5, GlobalMax: 500}, nil
}
func (billingHandlerPayment) IsPaymentEnabled(context.Context) bool { return true }

// newBillingContractHandlerUnderTest builds the handler over a real
// BillingContractService whose balance source the caller controls.
func newBillingContractHandlerUnderTest(bal *billingHandlerBalance) *BillingContractHandler {
	return NewBillingContractHandler(service.NewBillingContractService(
		bal, billingHandlerOrg{}, billingHandlerUsage{}, billingHandlerPayment{}, "https://portal.example.com",
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

// ---------------------------------------------------------------------------
// GET /api/analytics/usage
// ---------------------------------------------------------------------------

// billingHandlerUsageTracking records the userID ListByUser was actually
// called with, for the IDOR guard below. A pointer type (unlike the
// stateless billingHandlerUsage above) because the test needs to read that
// value back out after the request.
type billingHandlerUsageTracking struct {
	gotUser int64
}

func (u *billingHandlerUsageTracking) GetStatsByUser(context.Context, int64, time.Time, time.Time) (*service.UsageStats, error) {
	return &service.UsageStats{TotalActualCost: 1.25}, nil
}

func (u *billingHandlerUsageTracking) ListByUser(_ context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	u.gotUser = userID
	return []service.UsageLog{{UserID: userID, ActualCost: 1.25, TotalCost: 1.25}},
		&pagination.PaginationResult{Total: 1, Page: params.Page, PageSize: params.Limit()}, nil
}

func newBillingContractHandlerUnderTestWithUsage(usage *billingHandlerUsageTracking) *BillingContractHandler {
	return NewBillingContractHandler(service.NewBillingContractService(
		&billingHandlerBalance{balance: 42.50}, billingHandlerOrg{}, usage, billingHandlerPayment{}, "https://portal.example.com",
	))
}

// serveBillingUsage mirrors serveBillingState: no middleware runs here on
// purpose, the request carries ?user_id=999, and the tests below are about
// what the handler does with whatever identity (or lack of one) is on the
// context.
func serveBillingUsage(h *BillingContractHandler, setContext func(*gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/analytics/usage?user_id=999", nil)
	if setContext != nil {
		setContext(c)
	}
	h.Usage(c)
	return rec
}

// TestUsageHandlerReadsTheUserFromTheVerifiedBearerOnly is the IDOR guard for
// GET /api/analytics/usage, mirroring TestBillingStateHandlerReadsTheUserFromTheVerifiedBearerOnly.
func TestUsageHandlerReadsTheUserFromTheVerifiedBearerOnly(t *testing.T) {
	usage := &billingHandlerUsageTracking{}
	h := newBillingContractHandlerUnderTestWithUsage(usage)

	rec := serveBillingUsage(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, int64(7), usage.gotUser,
		"the usage list must be about the bearer's user, never a request-supplied id")
	require.NotEqual(t, int64(999), usage.gotUser)
}

// TestUsageHandlerRejectsAMissingOAuthIdentity mirrors the billing/state case.
func TestUsageHandlerRejectsAMissingOAuthIdentity(t *testing.T) {
	usage := &billingHandlerUsageTracking{}
	h := newBillingContractHandlerUnderTestWithUsage(usage)

	rec := serveBillingUsage(h, nil)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
	require.Zero(t, usage.gotUser, "no source may be consulted without a verified identity")
}

// TestUsageHandlerRejectsAWrongTypedOAuthIdentity mirrors the billing/state
// case: a string "7" on the context must fail closed, not resolve to user 0.
func TestUsageHandlerRejectsAWrongTypedOAuthIdentity(t *testing.T) {
	usage := &billingHandlerUsageTracking{}
	h := newBillingContractHandlerUnderTestWithUsage(usage)

	rec := serveBillingUsage(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, "7")
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
	require.Zero(t, usage.gotUser)
}

// TestUsageHandlerReports500WhenUnwired mirrors the billing/state case.
func TestUsageHandlerReports500WhenUnwired(t *testing.T) {
	h := NewBillingContractHandler(nil)

	rec := serveBillingUsage(h, func(c *gin.Context) {
		c.Set(middleware2.OAuthContextKeyUserID, int64(7))
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"server_error"}`, rec.Body.String())
}
