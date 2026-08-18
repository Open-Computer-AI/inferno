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
//   - client_id/redirect_uri failures -> a real HTTP error status (400/500)
//     with a short bare HTML body. Reachable directly by a raw navigation
//     (these checks run before the bearer check), so the body is genuinely
//     human-readable, not just a status code for JS to interpret.
//   - Ownership failures -> the same bare HTML body shape, but at 403 --
//     UNLIKE client_id/redirect_uri, this check runs AFTER the bearer
//     check (it needs to know who is asking), so in practice it is only
//     ever reached by the authenticated fetch AuthorizeConsentView.vue
//     makes, never by a raw navigation. The body is real HTML, but axios
//     consumes it as an opaque error object -- extractApiErrorMessage
//     never sees a message field in it, so the person sees a generic
//     "request failed" toast, not this page's copy. Correctness holds
//     either way (never a redirect); only the presentation degrades.
//   - Once a bearer IS present (only ever true for the authenticated fetch
//     AuthorizeConsentView.vue makes back to this same URL) and the request
//     is well-formed: GET reports either "ready" (200, text/plain body =
//     the exact URL to navigate to next -- either redirect_uri?code=...
//     for auto-approve, or redirect_uri?error=...  for a PKCE/scope
//     rejection) or "consent required" (202, empty body). POST carries an
//     explicit decision=approve|deny query parameter (the consent screen's
//     Approve/Deny buttons) and always responds with the 200/target-URL
//     protocol. In every case the frontend's only job is
//     `window.location.replace(bodyText)` -- this endpoint never emits the
//     literal cross-origin 302 itself, because that would only work for a
//     raw navigation, and a raw navigation can never carry the bearer this
//     branch requires (see the package doc above).
//
// ACCEPTED LIMITS -- read before "fixing" either of these; both were raised
// in review and are deliberate trade-offs, not oversights:
//
//   - Server-side "consent" is unverifiable. The POST decision=approve arm
//     reads `decision` from the query string and mints on "approve" with no
//     server-side pending-authorization record, nonce, or binding to any
//     rendered screen -- unlike the device flow's real `user_code` row.
//     Cross-site CSRF is genuinely blocked (OptionalJWTAuthMiddleware reads
//     the bearer from the Authorization header only; no cookie or
//     query-param credential path exists anywhere in this codebase's
//     middleware), but the server cannot distinguish "a human read the
//     scope list and clicked Approve" from "same-origin script issued a
//     POST". Closing that gap needs a real server-side pending-consent
//     record (a second RFC-8628-user_code-shaped table keyed by a
//     server-issued nonce) -- a re-architecture, not a line fix. See the
//     next point for why this doesn't add attacker capability beyond what
//     already exists.
//   - The 200-body-plus-window.location.replace design (chosen over a
//     literal cross-origin 302 -- see the WIRE PROTOCOL section above for
//     why a 302 doesn't work here) means the authorization code is
//     readable by page JS, not only by the browser following a Location
//     header. Any XSS on the panel origin can trigger minting (a same-
//     origin GET/POST the axios interceptor attaches the bearer to) and
//     read the resulting code, for any client the user owns, without the
//     consent screen ever rendering. This is a real, larger blast radius
//     than a pure-302 design. Accepted because panel-origin XSS already
//     means total account takeover via the bearer sitting in
//     localStorage -- an attacker who can run script on this origin does
//     not need this endpoint to do damage -- and the alternative is not
//     "use a real 302" (structurally impossible without a session cookie,
//     which this codebase does not have) but "delete the SPA consent
//     screen and force every scope through the narrow auto-approve path",
//     which is a worse trade.
func (h *OAuthHandler) Authorize(c *gin.Context) {
	q := c.Request.URL.Query()
	clientID := strings.TrimSpace(q.Get("client_id"))
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	responseType := strings.TrimSpace(q.Get("response_type"))
	// Trimmed once, here, and used everywhere below -- including what
	// IssueCode persists. Previously only the auto-approve comparison
	// trimmed the value, so `scope=%20agent_dashboard:access` auto-approved
	// but stored a leading space; the two representations of "the scope"
	// must not diverge by which code path touched them last.
	scope := strings.TrimSpace(q.Get("scope"))
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
	// billing:manage is documented (oauth_scope_vocabulary.go) as never
	// grantable at initial login -- only via a second, explicit device-flow
	// elevation -- but nothing enforced that until now. Without this check,
	// /oauth/authorize was a second, undocumented path to the exact scope
	// that rule exists to gate, reachable via ordinary consent-screen
	// approval rather than the deliberate elevation flow. Same redirect-
	// class treatment as any other unsupported scope: it doesn't need to
	// know who is asking to reject it.
	if scopeContains(scope, service.ScopeBillingManage) {
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
			respondWithTarget(c, authorizeRedirectTarget(redirectURI, "access_denied", state))
			return
		default:
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "No decision was provided.")
			return
		}
	default:
		// GET: auto-approve ONLY for the exact desktop-login scope AND ONLY
		// for the client's actual registrant (client.OwnerUserID), NOT the
		// broader owner-or-org-member test userOwnsClient uses for the
		// MUST-NOT-REDIRECT gate above. That broader test is correct for
		// "may this person even see/deny this request" but is NOT correct
		// for "may a code be minted with zero human interaction": review
		// found that org-member auto-approve, reachable via a bare GET a
		// browser lands on after nothing but a link click, lets any org
		// peer mint a code naming a VICTIM teammate as the grantee for a
		// client the peer controls (register a client with an attacker-
		// controlled redirect_uri, send the victim a same-org link, the
		// victim's browser auto-approves on their behalf with no consent
		// screen ever rendered). Requiring the exact registrant closes
		// that: a non-owner org member always gets the consent screen
		// (POST decision=approve still runs the SAME userOwnsClient check
		// above, so they CAN approve deliberately -- they just cannot be
		// auto-approved without ever seeing what they're granting).
		approved = scope == service.ScopeAgentDashboardAccess && client.OwnerUserID == subject.UserID
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
			respondWithTarget(c, authorizeRedirectTarget(redirectURI, "invalid_request", state))
		case errors.Is(err, service.ErrInvalidScope):
			respondWithTarget(c, authorizeRedirectTarget(redirectURI, "invalid_scope", state))
		case errors.Is(err, service.ErrInvalidRedirectURI):
			// Defense in depth against a TOCTOU between the checks above
			// and this call -- should be unreachable, but the same
			// MUST-NOT-REDIRECT rule applies if it ever isn't.
			slog.Error("oauth authorize: redirect_uri rejected at issuance after passing pre-checks", "client_id", clientID, "error", err)
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "This request could not be authorized.")
		case errors.Is(err, service.ErrUnknownClient) && (errors.Is(err, service.ErrClientNotUsable) || dbent.IsNotFound(err)):
			// Same collapse as the pre-check above (unknown vs revoked
			// reads identically), same defense-in-depth reasoning for the
			// TOCTOU window as ErrInvalidRedirectURI just above.
			slog.Error("oauth authorize: client rejected at issuance after passing pre-checks", "client_id", clientID, "error", err)
			renderAuthorizeErrorPage(c, http.StatusBadRequest, "Invalid request", "This request could not be authorized.")
		default:
			// Everything NOT explicitly matched above -- including an
			// ErrUnknownClient that wraps a genuine infrastructure fault
			// (neither ErrClientNotUsable nor a NotFound; e.g. a DB outage
			// in this exact TOCTOU window), code entropy failures,
			// persistence failures, or anything else unclassified.
			// ErrUnknownClient's own doc comment (oauth_authorize_service.go)
			// is explicit that this case belongs on a logged 500, not a
			// silent "unknown client" render -- collapsing it into the
			// case above would make a database outage invisible to
			// alerting, exactly the mistake ErrClientNotUsable exists to
			// let this handler avoid. The default arm is the error page,
			// never a redirect -- an error this handler doesn't recognize
			// must fail closed, not be guessed into an RFC error code and
			// bounced to a third party.
			slog.Error("oauth authorize: code issuance failed", "client_id", clientID, "error", err)
			renderAuthorizeErrorPage(c, http.StatusInternalServerError, "Server error", "Something went wrong. Please try again.")
		}
		return
	}

	respondWithTarget(c, authorizeRedirectTargetWithCode(redirectURI, code, state))
}

// scopeContains reports whether scope's whitespace-delimited tokens include
// target exactly -- the same tokenisation service.ValidateScope /
// middleware.RequireOAuthScope use, so this agrees with how scope is
// interpreted everywhere else.
func scopeContains(scope, target string) bool {
	for _, s := range strings.Fields(scope) {
		if s == target {
			return true
		}
	}
	return false
}

// respondWithTarget writes the "go here next" wire response every non-error
// 200 arm of Authorize uses: the exact URL (success, an RFC redirect-class
// error, or access_denied) for AuthorizeConsentView.vue to hand to
// window.location.replace. RFC 6749 §5.1 requires Cache-Control: no-store /
// Pragma: no-cache on any response carrying a credential -- this body can
// carry an authorization code, exactly the kind of credential that rule
// exists for, and unlike a 302's Location header (not a cacheable entity
// body) this IS one.
func respondWithTarget(c *gin.Context, target string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(target))
}

// userOwnsClient decides whether userID may SEE and DECIDE ON a grant
// request against client: either they registered it directly
// (owner_user_id), or they are a member (any role) of the org it belongs
// to. Self-hosted clients are created with OwnerUserID set to the
// registering user and OrgID set to that user's org at the time
// (OAuthClientService.RegisterSelfHosted), so org membership is the
// broader, correct test for THIS gate -- it also covers a teammate added
// to the org after the client was registered, which a strict
// OwnerUserID-only check would wrongly refuse. RoleIn returns "" (no
// error) for a non-member, so this reduces to a single membership lookup
// whenever the direct-owner check already misses.
//
// NOT the test for auto-approve. This function answers "may this person
// interact with this request at all" (the MUST-NOT-REDIRECT ownership
// gate below, and the POST decision=approve/deny arms); it deliberately
// does NOT gate whether a code can be minted with zero human interaction
// on a bare GET -- that needs the strictly narrower `client.OwnerUserID ==
// subject.UserID` test the GET branch applies directly, or an org peer who
// registered a client with an attacker-controlled redirect_uri could send
// a same-org teammate a link that auto-mints a code with no consent screen
// ever rendered.
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
// -- a display-name lookup for AuthorizeConsentView.vue's consent screen
// ("who is asking"), not a full re-run of Authorize's redirect_uri/PKCE/
// scope validation (those stay Authorize's job; the screen only renders
// after Authorize's GET already responded 202, so they're already
// satisfied). It is a panel endpoint (session-authenticated, on the
// {code,message,data} envelope), unlike Authorize itself: nothing outside
// inferno-frontend ever calls it, mirroring PendingDeviceAuthorization
// above for exactly the same reason (see that handler's doc comment for
// the envelope-vs-bare split this file's package doc references).
//
// It DOES re-check client usability and ownership, though -- review found
// the first version skipped both: using the unfiltered ByClientID (rather
// than UsableByClientID) meant a revoked client still returned 200,
// un-collapsing exactly the unknown-vs-revoked distinction
// service.ErrClientNotUsable's doc comment says must stay collapsed
// ("giving a revoked client its own observable code would hand an
// attacker a free client_id oracle"); and skipping ownership meant any
// authenticated Inferno user who obtained a client_id (a shared URL, a
// screenshot, a log) could learn another org's client's owner-chosen
// display name. Both failure modes now respond identically to "unknown
// client" -- see the ownership check below for why that's a 404, not a
// distinct 403.
func (h *OAuthHandler) PendingAuthorization(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	clientID := strings.TrimSpace(c.Query("client_id"))
	if clientID == "" {
		response.BadRequest(c, "invalid_request")
		return
	}

	ctx := c.Request.Context()
	client, err := h.clientSvc.UsableByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, service.ErrClientNotUsable) || dbent.IsNotFound(err) {
			response.NotFound(c, "client not found")
			return
		}
		slog.Error("oauth authorize: pending client lookup failed", "client_id", clientID, "error", err)
		response.InternalError(c, "server_error")
		return
	}

	owns, err := h.userOwnsClient(ctx, subject.UserID, client)
	if err != nil {
		slog.Error("oauth authorize: pending ownership check failed", "client_id", clientID, "error", err)
		response.InternalError(c, "server_error")
		return
	}
	if !owns {
		// Same response as "not found", deliberately -- not a distinct
		// 403. This endpoint exists to answer a display-name question for
		// a client the caller is about to decide on; a caller with no
		// relationship to it has no legitimate reason to be asking, and a
		// distinguishable "exists but isn't yours" response would itself
		// be a new org-boundary existence oracle, the same class of leak
		// the UsableByClientID switch above just closed.
		response.NotFound(c, "client not found")
		return
	}

	response.Success(c, gin.H{
		"client_name": client.Name,
		"client_id":   client.ClientID,
	})
}
