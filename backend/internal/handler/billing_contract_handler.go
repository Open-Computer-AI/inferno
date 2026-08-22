package handler

import (
	"log/slog"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

// ---------------------------------------------------------------------------
// Task 5 -- the 7 write endpoints, all gated on billing:manage
// (server/routes/billing_contract.go). Same bare-JSON, same identity-from-
// context rule as State/Subscription above; see that doc comment.
// ---------------------------------------------------------------------------

// billingContractUserID re-does State/Subscription's own identity extraction
// for the 7 write handlers below, factored out here (not backported onto
// State/Subscription) to keep this task's diff to its own methods.
func billingContractUserID(c *gin.Context) (int64, bool) {
	uidVal, ok := c.Get(middleware2.OAuthContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return 0, false
	}
	userID, ok := uidVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return 0, false
	}
	return userID, true
}

// billingContractWriteError maps a service error to a bare-JSON body via the
// SAME infraerrors.Code/Reason/Message this codebase already uses to drive
// its HTTP status codes elsewhere (internal/pkg/errors/http.go's ToHTTP) --
// just without ToHTTP's enveloped Status shape, since this surface's
// contract is bare JSON (see this file's top doc comment).
func billingContractWriteError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.JSON(status, gin.H{
		"error":   infraerrors.Reason(err),
		"message": infraerrors.Message(err),
	})
}

// billingChargeRequest is POST /api/billing/charge's body
// (hermes_cli/nous_billing.py:525): amountUsd is a JSON NUMBER (the client's
// own encoding for what it sends), unlike this adapter's own money OUTPUT
// fields, which are decimal strings (see BillingChargeStatusView.AmountUSD's
// doc comment).
//
// It is still parsed, and a malformed body is still a 400, even though the
// service refuses every charge: a caller sending garbage should be told its
// request was garbage, not handed a substantive answer about payment methods
// it never validly asked for.
type billingChargeRequest struct {
	AmountUSD float64 `json:"amountUsd"`
}

// Charge handles POST /api/billing/charge (nous_billing.py:504-528).
//
// 403 + BillingRefusalView, never 202 (ruling D-2). The status is not
// arbitrary: it is the one hermes_cli/nous_billing.py's _raise_for_error maps
// to a plain BillingError carrying `error`, which
// cli_billing_mixin.py:1239 renders as "No card on file". See
// BillingContractService.Charge for the full status-to-exception reading, and
// for why creating an order here locked users out of their own top-up flow.
func (h *BillingContractHandler) Charge(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req billingChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "amountUsd is required"})
		return
	}

	refusal := h.svc.Charge(c.Request.Context(), userID, req.AmountUSD, c.GetHeader("Idempotency-Key"))
	c.JSON(http.StatusForbidden, refusal)
}

// ChargeStatus handles GET /api/billing/charge/{id} (nous_billing.py:530-545).
//
// No error branch reaches this handler: BillingContractService.ChargeStatus
// never returns an error (unknown/foreign/malformed ids all resolve to
// {status:"pending"} inside the service -- see its doc comment for why that
// collapse happens THERE, not here).
func (h *BillingContractHandler) ChargeStatus(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	chargeID := c.Param("id")
	status := h.svc.ChargeStatus(c.Request.Context(), userID, chargeID)
	c.JSON(http.StatusOK, status)
}

// SubscriptionPreview handles POST /api/billing/subscription/preview
// (nous_billing.py:573-591). The body is accepted but not required to be
// well-formed: the answer is the same "blocked" regardless (see the service
// method's doc comment).
func (h *BillingContractHandler) SubscriptionPreview(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req struct {
		SubscriptionTypeID string `json:"subscriptionTypeId"`
	}
	_ = c.ShouldBindJSON(&req) // best-effort; the response does not depend on it

	preview := h.svc.SubscriptionPreview(c.Request.Context(), userID, req.SubscriptionTypeID)
	c.JSON(http.StatusOK, preview)
}

// SubscriptionUpgrade handles POST /api/billing/subscription/upgrade
// (nous_billing.py:648-675). 501, never a fake success -- see the service
// method's doc comment for why.
func (h *BillingContractHandler) SubscriptionUpgrade(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req struct {
		SubscriptionTypeID string `json:"subscriptionTypeId"`
	}
	_ = c.ShouldBindJSON(&req)

	refusal := h.svc.SubscriptionUpgrade(c.Request.Context(), userID, req.SubscriptionTypeID, c.GetHeader("Idempotency-Key"))
	c.JSON(http.StatusNotImplemented, refusal)
}

// PendingChangeSet handles PUT /api/billing/subscription/pending-change
// (nous_billing.py:594-627). 501 -- Inferno has no end-of-period-intent
// model (ruling R-5.1); the body is accepted but unused, same reasoning as
// SubscriptionPreview.
func (h *BillingContractHandler) PendingChangeSet(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	refusal := h.svc.PendingChangeSet(c.Request.Context(), userID)
	c.JSON(http.StatusNotImplemented, refusal)
}

// PendingChangeClear handles DELETE /api/billing/subscription/pending-change
// (nous_billing.py:631-644). 501, same reasoning as PendingChangeSet.
func (h *BillingContractHandler) PendingChangeClear(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	refusal := h.svc.PendingChangeClear(c.Request.Context(), userID)
	c.JSON(http.StatusNotImplemented, refusal)
}

// AutoTopUp handles PATCH /api/billing/auto-top-up (nous_billing.py:480-499).
// 501 -- already ruled in the spec (ruling R-5.1); the body is accepted but
// unused, same reasoning as SubscriptionPreview.
func (h *BillingContractHandler) AutoTopUp(c *gin.Context) {
	userID, ok := billingContractUserID(c)
	if !ok {
		return
	}
	if h.svc == nil {
		slog.Error("billing: contract service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	refusal := h.svc.AutoTopUp(c.Request.Context(), userID)
	c.JSON(http.StatusNotImplemented, refusal)
}
