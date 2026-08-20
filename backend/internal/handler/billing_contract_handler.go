package handler

import (
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BillingContractHandler serves the Nous-shaped /api/billing/* surface the
// hermes client asks its portal for.
//
// NOTE: like OAuthHandler, these handlers deliberately do NOT use
// internal/pkg/response. That package wraps every body in
// {code,message,data}; the client parses the raw object at the top level.
// An envelope here does not fail loudly -- json.loads succeeds, every
// payload.get() misses, and the client renders a well-formed "logged out"
// screen. That is why the route test asserts the ABSENCE of those three keys
// rather than merely asserting the fields it wants are present.
type BillingContractHandler struct {
	svc *service.BillingContractService
}

func NewBillingContractHandler(svc *service.BillingContractService) *BillingContractHandler {
	return &BillingContractHandler{svc: svc}
}

// State handles GET /api/billing/state -- the CLI's billing overview.
//
// Authenticated by middleware.RequireOAuthScope (RS256 OAuth bearer), not
// jwtAuth: the caller is a headless agent holding an OAuth access token, not
// an Inferno panel session. See server/routes/billing_contract.go.
//
// The identity comes from the gin context key the middleware sets, never from
// a query parameter or body field -- this response is entirely about the
// caller's own money, and a user-supplied id here would be an IDOR.
func (h *BillingContractHandler) State(c *gin.Context) {
	uidVal, ok := c.Get(middleware2.OAuthContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	userID, ok := uidVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	state, err := h.svc.State(c.Request.Context(), userID)
	if err != nil {
		// Only an unresolvable balance reaches here; every optional section
		// degrades inside the service. Loud on our side, quiet on theirs --
		// the client fails open on a 500 and shows a clean message, so this
		// log is the ONLY place the real cause is ever recorded.
		slog.Error("billing: state composition failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, state)
}

// Usage handles GET /api/analytics/usage -- the caller's own usage history.
//
// Same identity rule as State: the userID comes only from the verified
// bearer the middleware put on the context, never from a query parameter, and
// the same unwired/unauthenticated failure handling applies. See State's doc
// comment for why.
//
// response.ParsePagination is used for INPUT parsing only -- it reads
// page/page_size (and limit) off the query string and returns plain ints. It
// does not touch the response body, so reusing it here does not pull in the
// panel's {code,message,data} envelope; c.JSON below still writes the bare
// object this package's doc comment requires.
func (h *BillingContractHandler) Usage(c *gin.Context) {
	uidVal, ok := c.Get(middleware2.OAuthContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	userID, ok := uidVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}

	usage, err := h.svc.Usage(c.Request.Context(), userID, params)
	if err != nil {
		// Unlike State, there is no partial result to fall back to: the
		// usage list IS the response. Loud on our side, quiet on theirs.
		slog.Error("billing: usage composition failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, usage)
}
