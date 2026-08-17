package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthHandler serves the OAuth 2.0 authorization-server surface.
//
// NOTE: these handlers deliberately do NOT use internal/pkg/response. That
// package wraps bodies in {code,message,data}; the hermes client parses
// RFC-shaped JSON at the top level and hard-fails on a wrapped body.
type OAuthHandler struct {
	keySvc    *service.OAuthKeyService
	clientSvc *service.OAuthClientService
	orgSvc    *service.OrgService
	deviceSvc *service.OAuthDeviceService
	tokenSvc  *service.OAuthTokenService
}

func NewOAuthHandler(keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, orgSvc *service.OrgService, deviceSvc *service.OAuthDeviceService, tokenSvc *service.OAuthTokenService) *OAuthHandler {
	return &OAuthHandler{keySvc: keySvc, clientSvc: clientSvc, orgSvc: orgSvc, deviceSvc: deviceSvc, tokenSvc: tokenSvc}
}

// KeyService exposes the OAuth signing key service so
// middleware.RequireOAuthScope (the resource-server bearer-verification
// middleware guarding GET /api/oauth/account) can verify token signatures
// without a separate wire-provided dependency — routes.go already has this
// handler in hand when it builds that middleware.
func (h *OAuthHandler) KeyService() *service.OAuthKeyService {
	return h.keySvc
}

// JWKS handles GET /.well-known/jwks.json
func (h *OAuthHandler) JWKS(c *gin.Context) {
	jwks, err := h.keySvc.JWKS(c.Request.Context())
	if err != nil {
		slog.Error("oauth: jwks projection failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, jwks)
}

// RegisterSelfHostedClient handles POST /api/oauth/self-hosted-client.
// Bearer-authenticated: the caller must be a logged-in user.
func (h *OAuthHandler) RegisterSelfHostedClient(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	uid := subject.UserID

	var req dto.SelfHostedClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, uid)
	if err != nil {
		slog.Error("oauth: failed to look up orgs for user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if len(orgs) == 0 {
		slog.Error("oauth: no org for user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	created, err := h.clientSvc.RegisterSelfHosted(ctx, orgs[0].ID, uid, req.RedirectOrigin, req.Name)
	if err != nil {
		if errors.Is(err, service.ErrClientNameTooLong) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		slog.Error("oauth: self-hosted client registration failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	slog.Info("oauth: registered self-hosted client",
		"client_id", created.ClientID, "org_id", orgs[0].ID)

	c.JSON(http.StatusOK, dto.SelfHostedClientResponse{
		ClientID: created.ClientID,
		Name:     created.Name,
	})
}

// DeviceCode handles POST /api/oauth/device/code.
// Form-encoded per RFC 8628. Unauthenticated — the client_id is the caller's
// identity and it has no session yet at this point in the flow.
func (h *OAuthHandler) DeviceCode(c *gin.Context) {
	clientID := c.PostForm("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	scope := c.PostForm("scope")

	grant, err := h.deviceSvc.RequestCode(c.Request.Context(), clientID, scope)
	if err != nil {
		if errors.Is(err, service.ErrPortalNotConfigured) {
			slog.Error("oauth: device code request failed, server misconfigured", "client_id", clientID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		// err is logged (it's an internal wrapped error — "unknown client_id"
		// or a persistence failure — never device_code/user_code, which are
		// never logged anywhere) so an operator can tell a bad client_id
		// apart from a real backend fault. The response body deliberately
		// stays the single generic "invalid_client" for both cases — this is
		// an anti-probing choice on the wire, not a logging one.
		slog.Warn("oauth: device code request rejected", "client_id", clientID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client"})
		return
	}

	// Bare RFC 8628 body — NOT wrapped by internal/pkg/response.
	c.JSON(http.StatusOK, gin.H{
		"device_code":               grant.DeviceCode,
		"user_code":                 grant.UserCode,
		"verification_uri":          grant.VerificationURI,
		"verification_uri_complete": grant.VerificationURIComplete,
		"expires_in":                grant.ExpiresIn,
		"interval":                  grant.Interval,
	})
}

// tokenResponse writes a bare RFC 6749 §5.1 token response — NOT wrapped by
// internal/pkg/response. §5.1 also requires Cache-Control: no-store and
// Pragma: no-cache on every token response: the body carries a bearer
// credential, and an intermediary caching it would hand that credential to
// the next requester on the same cache path.
func tokenResponse(c *gin.Context, tokens *service.OAuthTokens) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    tokens.ExpiresIn,
		"scope":         tokens.Scope,
	})
}

// nousRefreshTokenHeader is the header the real hermes client sends the
// refresh_token grant's credential in, INSTEAD of the RFC 6749 §6 form
// field: hermes_cli/auth.py's `_refresh_access_token` posts
// `headers={"x-nous-refresh-token": refresh_token}` with only `grant_type`
// and `client_id` in the body (confirmed by reading that function — never
// edited, that repo is read-only). Task 8 conformance-tested the
// device_code grant only and missed this: the refresh grant looked correct
// against RFC 6749 and the wire-contract tests, but against the actual
// client it silently read an always-empty body field and rejected every
// refresh. The client is the specification, so this is what the server
// reads.
const nousRefreshTokenHeader = "X-Nous-Refresh-Token"

// refreshTokenFromRequest resolves the refresh_token grant's credential:
// the header the real client sends, first; the RFC 6749 §6 form field,
// second. Preferring the header (rather than requiring both, or requiring
// the header) keeps this endpoint compatible with any other RFC-conformant
// client that only knows the body field, while still working against the
// one real client this plan exists to serve. An empty-after-trim header is
// treated as absent, not as a present-but-empty credential, so a client
// that sends an empty header alongside a real body value (or omits the
// header entirely — Go's http.Header.Get returns "" either way) still
// falls through to the body. Never log the return value — it's a bearer
// credential exactly like the body field it may have come from (see the
// no-credential-logging rule stated on Token below).
func refreshTokenFromRequest(c *gin.Context) string {
	if header := strings.TrimSpace(c.GetHeader(nousRefreshTokenHeader)); header != "" {
		return header
	}
	return c.PostForm("refresh_token")
}

// Token handles POST /api/oauth/token. Unauthenticated — this endpoint IS
// the credential-issuance step; a caller with a valid session wouldn't need
// it. Form-encoded per RFC 6749/8628. Never logs device_code, refresh_token,
// or access_token — client_id only.
//
// Every branch below matches known sentinels explicitly and falls through to
// a logged 500 server_error for anything else (a Redis outage, a signing-key
// read failure, ...). Folding an unmatched internal error into an OAuth
// error code — e.g. treating a transient backend fault as expired_token or
// invalid_grant — would tell every agent hitting it "your credential is
// dead, re-run the device flow", turning a blip into a fleet-wide forced
// re-auth, silently: nothing about that response distinguishes it from a
// real expired/invalid credential, and nothing gets logged for an operator
// to notice. See DeviceCode above for the same pattern already in use here.
func (h *OAuthHandler) Token(c *gin.Context) {
	grantType := c.PostForm("grant_type")
	clientID := c.PostForm("client_id")

	switch grantType {
	case "urn:ietf:params:oauth:grant-type:device_code":
		tokens, err := h.tokenSvc.ExchangeDeviceCode(c.Request.Context(), clientID, c.PostForm("device_code"))
		if err != nil {
			// RFC 8628 §3.5: authorization_pending/slow_down are the normal
			// poll-loop responses, not failures — the hermes client branches
			// on these exact strings. All matched cases are 400s; the OAuth
			// error body, not the HTTP status, carries the meaning.
			switch {
			case errors.Is(err, service.ErrAuthorizationPending):
				c.JSON(http.StatusBadRequest, gin.H{"error": "authorization_pending"})
			case errors.Is(err, service.ErrSlowDown):
				c.JSON(http.StatusBadRequest, gin.H{"error": "slow_down"})
			case errors.Is(err, service.ErrAccessDenied):
				c.JSON(http.StatusBadRequest, gin.H{"error": "access_denied"})
			case errors.Is(err, service.ErrExpiredToken),
				errors.Is(err, service.ErrDeviceCodeNotFound),
				errors.Is(err, service.ErrDeviceCodeExpired):
				c.JSON(http.StatusBadRequest, gin.H{"error": "expired_token"})
			default:
				slog.Error("oauth: device_code token exchange failed", "client_id", clientID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			}
			return
		}
		tokenResponse(c, tokens)

	case "refresh_token":
		tokens, err := h.tokenSvc.ExchangeRefreshToken(c.Request.Context(), clientID, refreshTokenFromRequest(c))
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidGrant), errors.Is(err, service.ErrRefreshTokenReused):
				// RFC 6749 §5.2: invalid_grant covers unknown/expired/wrong-client
				// tokens AND detected reuse — the wire response deliberately
				// does not distinguish reuse from an ordinary invalid token
				// (that would tell a prober which case it hit); the reuse
				// case is logged server-side instead (see ExchangeRefreshToken).
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
			default:
				slog.Error("oauth: refresh_token token exchange failed", "client_id", clientID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			}
			return
		}
		tokenResponse(c, tokens)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
	}
}

// Account handles GET /api/oauth/account — consumed by the hermes CLI to
// resolve which org(s) the bearer's holder belongs to. Authenticated by
// middleware.RequireOAuthScope (Task 6, ES256 OAuth bearer), NOT jwtAuth: a
// headless agent presents an OAuth-issued access token here, not an Inferno
// panel session, so it is registered on its own route group in
// server/routes/oauth.go rather than under RegisterOAuthAPIRoutes.
func (h *OAuthHandler) Account(c *gin.Context) {
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

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("oauth: account org lookup failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	out := make([]gin.H, 0, len(orgs))
	for _, o := range orgs {
		role, rerr := h.orgSvc.RoleIn(ctx, o.ID, userID)
		if rerr != nil {
			// A per-org role lookup failure should not fail the whole
			// account response (the caller still legitimately belongs to
			// this org — OrgsForUser just proved it via org_members) — but
			// it must never silently claim an elevated role. MEMBER is
			// this codebase's least-privileged org role.
			slog.Error("oauth: account role lookup failed", "org_id", o.ID, "error", rerr)
			role = service.OrgRoleMember
		}
		out = append(out, gin.H{
			"id":         strconv.FormatInt(o.ID, 10),
			"slug":       o.Slug,
			"name":       o.Name,
			"isPersonal": o.IsPersonal,
			"role":       role,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": strconv.FormatInt(userID, 10),
		"orgs":    out,
	})
}

// deviceDecisionRequest is the body both ApproveDevice and DenyDevice bind:
// the user_code the human typed or that verification_uri_complete prefilled.
type deviceDecisionRequest struct {
	UserCode string `json:"user_code" binding:"required"`
}

// ApproveDevice handles POST /api/oauth/device/approve and DenyDevice handles
// POST /api/oauth/device/deny — the device-flow approval screen's two
// actions, session-authenticated (jwtAuth): a logged-in human is confirming
// or rejecting a device-flow login in inferno-frontend's browser UI, not an
// agent or CLI presenting a bearer token or a bare client_id.
//
// DELIBERATE ASYMMETRY, read before "fixing" this: every other handler in
// this file (JWKS, DeviceCode, Token, Account, RegisterSelfHostedClient)
// emits bare JSON and is barred from internal/pkg/response, because the
// hermes client parses RFC-shaped JSON at the top level. These two handlers
// are the opposite — they MUST use internal/pkg/response — because they are
// NOT part of that wire contract at all. Nothing outside inferno-frontend
// ever calls them; they exist solely for its device approval screen.
// inferno-frontend/src/api/client.ts's response interceptor (its error path)
// reads apiData.code and apiData.message to build the message it surfaces to
// the user. A bare {"error":"not_found"} body has neither field, so every
// failure here would otherwise reach the user as a generic, non-actionable
// axios error instead of "that code doesn't exist" / "that code expired,
// run the command again". Keep this pair on response.*; keep the rest of
// this file bare.
//
// Neither handler ever logs user_code — it is a credential until redeemed,
// same rule as DeviceCode/Token above.
func (h *OAuthHandler) ApproveDevice(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req deviceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request")
		return
	}

	switch err := h.deviceSvc.Approve(c.Request.Context(), req.UserCode, subject.UserID); {
	case err == nil:
		response.Success(c, gin.H{"status": "approved"})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		response.NotFound(c, "device code not found")
	case errors.Is(err, service.ErrDeviceCodeExpired):
		response.Error(c, http.StatusGone, "device code expired")
	default:
		slog.Error("oauth: device approval failed", "user_id", subject.UserID, "error", err)
		response.InternalError(c, "server_error")
	}
}

func (h *OAuthHandler) DenyDevice(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req deviceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request")
		return
	}

	switch err := h.deviceSvc.Deny(c.Request.Context(), req.UserCode); {
	case err == nil:
		response.Success(c, gin.H{"status": "denied"})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		response.NotFound(c, "device code not found")
	case errors.Is(err, service.ErrDeviceCodeExpired):
		response.Error(c, http.StatusGone, "device code expired")
	default:
		slog.Error("oauth: device denial failed", "user_id", subject.UserID, "error", err)
		response.InternalError(c, "server_error")
	}
}
