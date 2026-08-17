package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
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
}

func NewOAuthHandler(keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, orgSvc *service.OrgService, deviceSvc *service.OAuthDeviceService) *OAuthHandler {
	return &OAuthHandler{keySvc: keySvc, clientSvc: clientSvc, orgSvc: orgSvc, deviceSvc: deviceSvc}
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
