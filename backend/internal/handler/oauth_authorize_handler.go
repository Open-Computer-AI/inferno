package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// Authorize handles GET and POST /oauth/authorize -- the browser-facing leg
// of RFC 6749 §4.1's authorization_code grant, PKCE-bound per RFC 7636.
//
// THIS IS NOT A JSON ENDPOINT. It is reached by a real browser (the hermes
// client opens the system browser here directly), so every response is
// either a real HTTP redirect or a small bare HTML/text page -- never
// internal/pkg/response, never gin.H JSON. The one narrow exception is auth
// failure: this route sits behind middleware.OptionalJWTAuthMiddleware
// (already wired for every other panel endpoint), and a PRESENT-but-invalid
// Authorization header is rejected by that shared middleware with its
// existing JSON 401 body before this handler ever runs. That is deliberately
// NOT reimplemented here: this handler only ever sees "no bearer" (raw
// browser navigation never carries one) or "valid bearer" (an authenticated
// fetch from AuthorizeConsentView.vue always does), and duplicating
// TokenVersion/session-binding revocation logic to preserve a "no JSON, ever"
// purity would trade a real security check for a cosmetic one.
//
// THE ERROR SPLIT IS THE SECURITY PROPERTY (RFC 6749 §4.1.2.1):
//
//   - MUST-NOT-REDIRECT: unknown/unusable client_id, an OOB (device-flow-only)
//     client, a redirect_uri that doesn't match the client's registered
//     origin, and "the session user doesn't own this client" all render a
//     bare error page. redirect_uri is UNVALIDATED at this point (or the
//     client relationship itself is untrustworthy), so redirecting there
//     would be an open redirect: an attacker supplies a bogus client_id
//     plus their own URL and the browser gets bounced there.
//   - Once client_id and redirect_uri are both validated, every other
//     rejection (bad response_type, PKCE, scope) redirects back to
//     redirect_uri with ?error=...&state=..., per §4.1.2.1.
//   - The default arm of every error switch below is the error page, never
//     a redirect -- an unclassified future error is safe by construction.
//
// AUTO-APPROVE is only offered when the requested scope is EXACTLY
// agent_dashboard:access (the desktop-login case) AND the session user owns
// the client; anything else needs the human to look at
// AuthorizeConsentView.vue's consent screen first.
//
// WIRE PROTOCOL (deliberately bare, chosen to fit this codebase's
// header-only JWT model -- there is no session cookie anywhere in Inferno,
// so a raw top-level navigation can never carry proof of login; see this
// file's package doc comment set for the full reasoning):
//
//   - No bearer present -> 302 to {frontend}/login?redirect=<original path
//     and query, verbatim>. Real browser navigations always land here first;
//     AuthorizeConsentView.vue (a requiresAuth Vue route at this same path)
//     is what LoginView returns the browser to afterwards, and it is what
//     issues the authenticated re-request below.
//   - response_type/scope/PKCE shape errors that don't need to know WHO is
//     asking are checked before the bearer check and, when redirect_uri is
//     already validated, delivered as a REAL 302 to redirect_uri -- this
//     works correctly for a raw browser hit too, not just a fetch call.
//   - client_id/redirect_uri/ownership failures -> a real HTTP error status
//     (400/403/500) with a short bare HTML body. Reachable directly by a raw
//     navigation (these checks run before the bearer check), so the body is
//     genuinely human-readable, not just a status code for JS to interpret.
//   - Once a bearer IS present (only ever true for the authenticated fetch
//     AuthorizeConsentView.vue makes back to this same URL) and the request
//     is well-formed: GET reports either "ready" (200, text/plain body =
//     the exact URL to navigate to next -- either redirect_uri?code=...
//     for auto-approve, or redirect_uri?error=...  for a PKCE/scope
//     rejection) or "consent required" (202, empty body). POST carries an
//     explicit decision=approve|deny query parameter (the consent screen's
//     Approve/Deny buttons) and always responds with the 200/target-URL
//     protocol. In every case the frontend's only job is
//     `window.location.assign(bodyText)` -- this endpoint never emits the
//     literal cross-origin 302 itself, because that would only work for a
//     raw navigation, and a raw navigation can never carry the bearer this
//     branch requires (see the package doc above).
func (h *OAuthHandler) Authorize(c *gin.Context) {
	q := c.Request.URL.Query()
	clientID := strings.TrimSpace(q.Get("client_id"))
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	responseType := strings.TrimSpace(q.Get("response_type"))
	scope := q.Get("scope")
	state := q.Get("state")
	codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(q.Get("code_challenge_method"))

	ctx := c.Request.Context()

	// ---- MUST-NOT-REDIRECT class: client_id and redirect_uri -----------
	client, err := h.clientSvc.UsableByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, service.ErrClientNotUsable) || dbent.IsNotFound(err) {
			// Genuinely unregistered, or registered but revoked/not usable.
			// Collapsed into one response on purpose -- see
			// service.ErrClientNotUsable's doc comment: giving a revoked
			// client its own observable response would hand an attacker a
			// free client_id oracle.
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Unknown client", "This application is not registered, or is no longer active.")
			return
		}
		// Any other error is an infrastructure fault (e.g. a DB outage
		// during the client lookup) -- rendering it as "unknown client"
		// would make an outage invisible to alerting.
		slog.Error("oauth authorize: client lookup failed", "client_id", clientID, "error", err)
		renderAuthorizeErrorPage(c, http.StatusInternalServerError, "Server error", "Something went wrong. Please try again.")
		return
	}

	if service.IsOOBOrigin(client.RedirectURIOrigin) {
		renderAuthorizeErrorPage(c, http.StatusBadRequest, "Cannot authorize here", "This application does not support browser sign-in.")
		return
	}
	if err := service.RedirectURIMatchesClient(client.RedirectURIOrigin, redirectURI); err != nil {
		renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "The redirect address for this request does not match what this application registered.")
		return
	}

	// redirect_uri is now trustworthy -- every rejection from here on
	// redirects back to it instead of rendering a page.

	if responseType != "code" {
		c.Redirect(http.StatusFound, authorizeRedirectTarget(redirectURI, "unsupported_response_type", state))
		return
	}
	if err := service.ValidateScope(scope); err != nil {
		c.Redirect(http.StatusFound, authorizeRedirectTarget(redirectURI, "invalid_scope", state))
		return
	}
	if codeChallengeMethod != "S256" || !service.ValidCodeChallengeShape(codeChallenge) {
		c.Redirect(http.StatusFound, authorizeRedirectTarget(redirectURI, "invalid_request", state))
		return
	}

	// ---- Identity: everything past this point needs to know who is
	// asking, which a raw browser navigation can never prove (this
	// codebase has no session cookie -- only a bearer header, which only
	// JS can attach). Send the browser to log in and come straight back
	// to this exact URL, query string intact.
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		loginURL := "/login?redirect=" + url.QueryEscape(c.Request.URL.RequestURI())
		c.Redirect(http.StatusFound, loginURL)
		return
	}

	// ---- MUST-NOT-REDIRECT class: does this user own this client? -----
	//
	// IssueCode does NOT check this, and nothing upstream does either --
	// deliberately left to this handler (see oauth_authorize_service.go's
	// IssueCode doc comment history). Without it any authenticated Inferno
	// user could mint a code, and therefore an access token, against a
	// client they do not own. Treated the same as an unknown client_id:
	// render an error page, never redirect.
	owns, err := h.userOwnsClient(ctx, subject.UserID, client)
	if err != nil {
		slog.Error("oauth authorize: ownership check failed", "client_id", clientID, "error", err)
		renderAuthorizeErrorPage(c, http.StatusInternalServerError, "Server error", "Something went wrong. Please try again.")
		return
	}
	if !owns {
		renderAuthorizeErrorPage(c, http.StatusForbidden, "Not authorized", "Your account is not allowed to authorize this application.")
		return
	}

	approved := false
	switch c.Request.Method {
	case http.MethodPost:
		switch strings.TrimSpace(c.Query("decision")) {
		case "approve":
			approved = true
		case "deny":
			c.Data(http.StatusOK, "text/plain; charset=utf-8",
				[]byte(authorizeRedirectTarget(redirectURI, "access_denied", state)))
			return
		default:
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "No decision was provided.")
			return
		}
	default:
		// GET: auto-approve ONLY for the exact desktop-login scope. Any
		// other request -- a wider scope, or a client the user owns but
		// hasn't reviewed this specific grant for -- needs the consent
		// screen.
		approved = strings.TrimSpace(scope) == service.ScopeAgentDashboardAccess
		if !approved {
			c.Status(http.StatusAccepted) // 202: consent required, no body.
			return
		}
	}

	if !approved || h.authorizeSvc == nil {
		slog.Error("oauth authorize: reached issuance without approval or a wired service")
		renderAuthorizeErrorPage(c, http.StatusInternalServerError, "Server error", "Something went wrong. Please try again.")
		return
	}

	code, err := h.authorizeSvc.IssueCode(ctx, service.IssueCodeInput{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		UserID:              subject.UserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMissingCodeChallenge), errors.Is(err, service.ErrPlainChallengeMethodRejected):
			c.Data(http.StatusOK, "text/plain; charset=utf-8",
				[]byte(authorizeRedirectTarget(redirectURI, "invalid_request", state)))
		case errors.Is(err, service.ErrInvalidScope):
			c.Data(http.StatusOK, "text/plain; charset=utf-8",
				[]byte(authorizeRedirectTarget(redirectURI, "invalid_scope", state)))
		case errors.Is(err, service.ErrUnknownClient), errors.Is(err, service.ErrInvalidRedirectURI):
			// Defense in depth against a TOCTOU between the checks above
			// and this call -- should be unreachable, but the same
			// MUST-NOT-REDIRECT rule applies if it ever isn't.
			slog.Error("oauth authorize: client/redirect_uri rejected at issuance after passing pre-checks", "client_id", clientID, "error", err)
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "This request could not be authorized.")
		default:
			// Code entropy, persistence failure, or anything else
			// unclassified. The default arm is the error page, never a
			// redirect -- an error this handler doesn't recognize must
			// fail closed, not be guessed into an RFC error code and
			// bounced to a third party.
			slog.Error("oauth authorize: code issuance failed", "client_id", clientID, "error", err)
			renderAuthorizeErrorPage(c, http.StatusInternalServerError, "Server error", "Something went wrong. Please try again.")
		}
		return
	}

	c.Data(http.StatusOK, "text/plain; charset=utf-8",
		[]byte(authorizeRedirectTargetWithCode(redirectURI, code, state)))
}

// userOwnsClient decides whether userID may authorize a grant against
// client: either they registered it directly (owner_user_id), or they are a
// member (any role) of the org it belongs to. Self-hosted clients are
// created with OwnerUserID set to the registering user and OrgID set to
// that user's org at the time (OAuthClientService.RegisterSelfHosted), so
// org membership is the broader, correct test -- it also covers a
// teammate added to the org after the client was registered, which a
// strict OwnerUserID-only check would wrongly refuse. RoleIn returns "" (no
// error) for a non-member, so this reduces to a single membership lookup
// whenever the direct-owner check already misses.
func (h *OAuthHandler) userOwnsClient(ctx context.Context, userID int64, client *dbent.OAuthClient) (bool, error) {
	if client.OwnerUserID == userID {
		return true, nil
	}
	if h.orgSvc == nil {
		return false, nil
	}
	role, err := h.orgSvc.RoleIn(ctx, client.OrgID, userID)
	if err != nil {
		return false, err
	}
	return role != "", nil
}

// renderAuthorizeErrorPage writes a small, static, HTML error page for the
// MUST-NOT-REDIRECT class of failure. Deliberately does not interpolate any
// caller-supplied value (client_id, redirect_uri, ...) into the body --
// title and message are always one of a fixed, safe set of strings, so
// there is nothing here for an attacker-chosen client_id/redirect_uri to
// inject into.
func renderAuthorizeErrorPage(c *gin.Context, status int, title, message string) {
	body := "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + title +
		"</title></head><body><h1>" + title + "</h1><p>" + message + "</p></body></html>"
	c.Data(status, "text/html; charset=utf-8", []byte(body))
}

// authorizeRedirectTarget builds redirect_uri?error=...&state=... per RFC
// 6749 §4.1.2.1. state is echoed back verbatim, exactly as received -- it is
// the client's CSRF defence, never interpreted here -- and omitted from the
// query entirely when the request didn't carry one (state is OPTIONAL per
// §4.1.1), rather than appending an empty state= the client never sent.
func authorizeRedirectTarget(redirectURI, errCode, state string) string {
	v := url.Values{}
	v.Set("error", errCode)
	if state != "" {
		v.Set("state", state)
	}
	return redirectURI + "?" + v.Encode()
}

// authorizeRedirectTargetWithCode builds redirect_uri?code=...&state=... --
// the RFC 6749 §4.1.2 success response. Never logs code: it is a bearer
// credential for the token endpoint exactly like the ones this package's
// other handlers already refuse to log.
func authorizeRedirectTargetWithCode(redirectURI, code, state string) string {
	v := url.Values{}
	v.Set("code", code)
	if state != "" {
		v.Set("state", state)
	}
	return redirectURI + "?" + v.Encode()
}

// PendingAuthorization handles GET /api/oauth/authorize/pending?client_id=...
// -- purely a display-name lookup for AuthorizeConsentView.vue's consent
// screen ("who is asking"), NOT a second copy of Authorize's security
// checks. It is a panel endpoint (session-authenticated, on the
// {code,message,data} envelope), unlike Authorize itself: nothing outside
// inferno-frontend ever calls it, mirroring PendingDeviceAuthorization
// above for exactly the same reason (see that handler's doc comment for
// the envelope-vs-bare split this file's package doc references).
//
// It intentionally does NOT re-check client/redirect_uri/ownership --
// those are Authorize's job, already satisfied before the consent screen
// can be showing at all (the screen only renders after Authorize's GET
// responded 202). This endpoint answers one question only: "what is this
// client called", so a revoked-between-calls client degrades to its raw
// client_id rather than a hard failure -- the consent screen can still
// show *something*, and the actual grant remains gated by Authorize's own
// checks regardless of what this displays.
func (h *OAuthHandler) PendingAuthorization(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	clientID := strings.TrimSpace(c.Query("client_id"))
	if clientID == "" {
		response.BadRequest(c, "invalid_request")
		return
	}

	client, err := h.clientSvc.ByClientID(c.Request.Context(), clientID)
	if err != nil {
		if dbent.IsNotFound(err) {
			response.NotFound(c, "client not found")
			return
		}
		slog.Error("oauth authorize: pending client lookup failed", "client_id", clientID, "error", err)
		response.InternalError(c, "server_error")
		return
	}

	response.Success(c, gin.H{
		"client_name": client.Name,
		"client_id":   client.ClientID,
	})
}
