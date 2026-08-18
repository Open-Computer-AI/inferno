package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterOAuthWellKnown mounts unauthenticated discovery endpoints at the
// server root (NOT under /api) — JWKS must live at the well-known path.
func RegisterOAuthWellKnown(r gin.IRouter, h *handler.OAuthHandler) {
	r.GET("/.well-known/jwks.json", h.JWKS)
}

// RegisterOAuthAPIRoutes mounts the authenticated OAuth self-service surface
// under /api/oauth — deliberately NOT /api/v1, since this path is shared
// with the unauthenticated OAuth endpoints (device/code, token, etc. — later
// tasks in this plan) and is consumed by the hermes CLI / dashboard, not the
// panel's versioned API.
func RegisterOAuthAPIRoutes(r gin.IRouter, h *handler.OAuthHandler, jwtAuth middleware.JWTAuthMiddleware) {
	authenticated := r.Group("/api/oauth")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.POST("/self-hosted-client", h.RegisterSelfHostedClient)
		// Device approval screen (Task 7): a logged-in human confirming or
		// rejecting a device-flow login in inferno-frontend's browser UI.
		// Session-authenticated like self-hosted-client above, unlike
		// device/code and token below (RegisterOAuthDeviceRoutes) which have
		// no session by design.
		// What the human is being asked to approve. Session-gated for the same
		// reason approve/deny are, and additionally because an unauthenticated
		// version would let anyone holding a phished user_code confirm it is
		// live and read back the scopes it carries.
		authenticated.GET("/device/pending", h.PendingDeviceAuthorization)
		authenticated.POST("/device/approve", h.ApproveDevice)
		authenticated.POST("/device/deny", h.DenyDevice)
		// AuthorizeConsentView.vue's display-name lookup for the
		// /oauth/authorize consent screen (Task 4). See
		// OAuthHandler.PendingAuthorization's doc comment for why this is
		// on the envelope while /oauth/authorize itself is bare.
		authenticated.GET("/authorize/pending", h.PendingAuthorization)
	}
}

// RegisterOAuthDeviceRoutes mounts the UNAUTHENTICATED /api/oauth device-flow
// endpoints. Deliberately a separate group from RegisterOAuthAPIRoutes (which
// requires jwtAuth): a headless CLI calling POST /api/oauth/device/code has
// no session yet — client_id is the identity for this request, not a bearer
// token. Both groups share the /api/oauth prefix; gin allows two distinct
// *gin.RouterGroup values rooted at the same path as long as their route
// patterns don't collide.
//
// POST /api/oauth/token belongs here for the same reason: it is the
// credential-issuance step itself (device_code and refresh_token grants) —
// the caller by definition has no session/bearer token yet, so it cannot be
// gated behind jwtAuth.
func RegisterOAuthDeviceRoutes(r gin.IRouter, h *handler.OAuthHandler) {
	unauthenticated := r.Group("/api/oauth")
	{
		unauthenticated.POST("/device/code", h.DeviceCode)
		unauthenticated.POST("/token", h.Token)
	}
}

// RegisterOAuthAuthorizeRoute mounts GET/POST /oauth/authorize (Task 4) at
// the server ROOT, not under /api -- it is a browser endpoint, reached by
// the hermes client's system browser via a real top-level navigation, not
// an XHR call, so it cannot live behind the header-only jwtAuth group that
// RegisterOAuthAPIRoutes uses (a raw navigation never carries an
// Authorization header at all; jwtAuth would 401 every unauthenticated hit
// with a JSON body no browser can act on).
//
// optionalJWTAuth (middleware.OptionalJWTAuthMiddleware, already wired for
// every other panel endpoint) is used instead: no header at all passes
// through anonymously (the handler then does its own 302-to-login), a
// header on a still-valid session resolves the AuthSubject the handler
// needs for the ownership check, and a header on an EXPIRED/invalid session
// is rejected by that shared middleware before the handler runs -- see
// OAuthHandler.Authorize's doc comment for why that one case is this
// otherwise-bare endpoint's sole JSON exception.
func RegisterOAuthAuthorizeRoute(r gin.IRouter, h *handler.OAuthHandler, optionalJWTAuth middleware.OptionalJWTAuthMiddleware) {
	r.GET("/oauth/authorize", gin.HandlerFunc(optionalJWTAuth), h.Authorize)
	r.POST("/oauth/authorize", gin.HandlerFunc(optionalJWTAuth), h.Authorize)
}

// RegisterOAuthAccountRoutes mounts GET /api/oauth/account behind Task 6's
// middleware.RequireOAuthScope — a THIRD distinct /api/oauth group,
// deliberately not merged into RegisterOAuthAPIRoutes (gated by jwtAuth,
// Inferno's HMAC panel-session middleware) or RegisterOAuthDeviceRoutes
// (unauthenticated). A hermes agent calling this endpoint presents an
// OAuth-issued ES256 bearer token, not a panel session and not a bare
// client_id — a third auth shape needs its own group. required scope is ""
// (any validly-signed, unexpired OAuth token) since this endpoint only
// resolves which org(s) the token's holder belongs to; it grants no
// elevated capability itself.
func RegisterOAuthAccountRoutes(r gin.IRouter, h *handler.OAuthHandler) {
	scoped := r.Group("/api/oauth")
	{
		scoped.GET("/account", middleware.RequireOAuthScope(h.KeyService(), ""), h.Account)
	}
}
