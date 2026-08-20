package handler

import (
	"log/slog"
	"net/http"

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

// Subscription handles GET /api/billing/subscription -- the CLI's plan
// picker. Same identity and error-shape rules as State; see its doc comment.
func (h *BillingContractHandler) Subscription(c *gin.Context) {
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

	sub, err := h.svc.Subscription(c.Request.Context(), userID)
	if err != nil {
		// Nothing in BillingContractService.Subscription is fatal today (see
		// its doc comment); this branch exists for signature symmetry with
		// State and as a seam for a future genuinely-fatal source.
		slog.Error("billing: subscription composition failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, sub)
}
