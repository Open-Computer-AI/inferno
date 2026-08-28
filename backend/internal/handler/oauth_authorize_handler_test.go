package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthauthorizationcode"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// authorizeTestFixture wires a minimal, real (sqlite-backed) OAuthHandler
// for GET/POST /oauth/authorize, plus a router that lets tests simulate
// "already authenticated" (or not) without going through a real JWT --
// Authorize only ever reads middleware.GetAuthSubjectFromContext, so a test
// middleware that sets AuthSubject from a header is a faithful stand-in for
// the real middleware.OptionalJWTAuthMiddleware this route runs behind in
// production (see RegisterOAuthAuthorizeRoute).
type authorizeTestFixture struct {
	h      *OAuthHandler
	router *gin.Engine
	ent    *dbent.Client
}

const authorizeTestUserHeader = "X-Test-User-ID"

func newAuthorizeTestFixture(t *testing.T) *authorizeTestFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	entClient := newOAuthHandlerTestEntClient(t)
	clientSvc := service.NewOAuthClientService(entClient)
	orgSvc := service.NewOrgService(entClient)
	authorizeSvc := service.NewOAuthAuthorizeService(entClient)

	h := NewOAuthHandler(nil, clientSvc, orgSvc, nil, nil, nil)
	h.SetAuthorizeService(authorizeSvc)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if raw := c.GetHeader(authorizeTestUserHeader); raw != "" {
			uid, err := strconv.ParseInt(raw, 10, 64)
			require.NoError(t, err)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: uid})
		}
		c.Next()
	})
	router.GET("/oauth/authorize", h.Authorize)
	router.POST("/oauth/authorize", h.Authorize)
	router.GET("/api/oauth/authorize/pending", h.PendingAuthorization)

	return &authorizeTestFixture{h: h, router: router, ent: entClient}
}

// authorizationCodes returns every OAuthAuthorizationCode row for clientID,
// ordered oldest-first -- F13's fix: "issues no code" / "issues exactly one
// code" must be asserted against the actual persisted rows, not against
// substrings of the response body (a handler that minted a code and simply
// forgot to put it in the body would otherwise pass).
func (f *authorizeTestFixture) authorizationCodes(t *testing.T, clientID string) []*dbent.OAuthAuthorizationCode {
	t.Helper()
	rows, err := f.ent.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.ClientID(clientID)).
		Order(dbent.Asc(oauthauthorizationcode.FieldID)).
		All(context.Background())
	require.NoError(t, err)
	return rows
}

// pendingAuthorizationCodes narrows authorizationCodes to status="pending"
// -- the count that matters for F9 (repeat issuance must not leave more
// than one live, redeemable code outstanding per client+user).
func (f *authorizeTestFixture) pendingAuthorizationCodes(t *testing.T, clientID string) []*dbent.OAuthAuthorizationCode {
	t.Helper()
	rows, err := f.ent.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.ClientID(clientID), oauthauthorizationcode.Status("pending")).
		Order(dbent.Asc(oauthauthorizationcode.FieldID)).
		All(context.Background())
	require.NoError(t, err)
	return rows
}

// registerClient creates a self-hosted client owned by ownerUserID in orgID,
// with the given registered redirect origin, and returns its client_id.
func (f *authorizeTestFixture) registerClient(t *testing.T, orgID, ownerUserID int64, origin string) string {
	t.Helper()
	created, err := f.h.clientSvc.RegisterSelfHosted(context.Background(), orgID, ownerUserID, origin, "test-agent")
	require.NoError(t, err)
	return created.ClientID
}

// addOrgMember inserts a raw org_members row so a SECOND user (not the
// client's owner_user_id) can be tested against the org-membership half of
// the ownership rule.
func (f *authorizeTestFixture) addOrgMember(t *testing.T, orgID, userID int64, role string) {
	t.Helper()
	_, err := f.ent.OrgMember.Create().
		SetOrgID(orgID).
		SetUserID(userID).
		SetRole(role).
		Save(context.Background())
	require.NoError(t, err)
}

// authorizeQuery builds the standard, valid authorize query string for
// clientID/redirectURI/scope, with a fixed, recognisable state and a
// syntactically valid (43-char base64url) code_challenge -- its exact value
// never matters here, since these tests never exercise token redemption.
func authorizeQuery(clientID, redirectURI, scope, state string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {scope},
		"state":                 {state},
		"code_challenge":        {"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		"code_challenge_method": {"S256"},
	}
}

func (f *authorizeTestFixture) get(t *testing.T, q url.Values, userID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	if userID != 0 {
		req.Header.Set(authorizeTestUserHeader, strconv.FormatInt(userID, 10))
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *authorizeTestFixture) post(t *testing.T, q url.Values, decision string, userID int64) *httptest.ResponseRecorder {
	t.Helper()
	q = cloneValues(q)
	q.Set("decision", decision)
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize?"+q.Encode(), nil)
	if userID != 0 {
		req.Header.Set(authorizeTestUserHeader, strconv.FormatInt(userID, 10))
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *authorizeTestFixture) pending(t *testing.T, clientID string, userID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/authorize/pending?client_id="+url.QueryEscape(clientID), nil)
	if userID != 0 {
		req.Header.Set(authorizeTestUserHeader, strconv.FormatInt(userID, 10))
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

// ---- MUST-NOT-REDIRECT class: never a 3xx, ever -----------------------

func TestAuthorizeUnknownClientRendersErrorPageNotRedirect(t *testing.T) {
	f := newAuthorizeTestFixture(t)

	q := authorizeQuery("agent:does-not-exist", "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestAuthorizeBadRedirectURIRendersErrorPageNotRedirect(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	// Registered origin is agent.example.com; this redirect_uri points
	// somewhere else entirely -- the exact open-redirect shape this split
	// exists to prevent.
	q := authorizeQuery(clientID, "https://evil.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
}

func TestAuthorizeOOBClientRendersErrorPageNotRedirect(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	// hermes-cli-style device-flow-only client: seed it directly with the
	// OOB placeholder origin, exactly as migrations/905 does.
	created, err := f.ent.OAuthClient.Create().
		SetClientID("agent:oob-client").
		SetKind("FIRST_PARTY").
		SetName("oob").
		SetOwnerUserID(42).
		SetOrgID(1).
		SetStatus(service.ClientActive).
		SetRedirectURIOrigin("urn:ietf:wg:oauth:2.0:oob").
		Save(context.Background())
	require.NoError(t, err)

	q := authorizeQuery(created.ClientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
}

func TestAuthorizeNonOwnerRendersErrorPageNotRedirect(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	// Client owned by user 42 in org 1; user 99 is neither the owner nor a
	// member of org 1.
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 99)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
}

// Review finding F2: the org-member-may-auto-approve test this replaces
// (TestAuthorizeOrgMemberMayAuthorizeClientTheyDidNotRegister) enshrined a
// one-click privilege escalation as intended behaviour -- a bare GET, with
// no consent screen ever rendered, minted a code naming user 7 as the
// grantee for a client user 7 never registered and does not control. Any
// org peer could register a client with an attacker-controlled
// redirect_uri and send a same-org teammate a link that silently
// auto-approved on their behalf. Auto-approve now requires
// client.OwnerUserID == the session user exactly; everyone else -- even a
// legitimate org peer -- gets routed to the consent screen instead.
func TestAuthorizeOrgMemberDoesNotAutoApproveClientTheyDidNotRegister(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	// user 7 didn't register the client, but IS a member of its org.
	f.addOrgMember(t, 1, 7, service.OrgRoleMember)

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 7)

	require.Equal(t, http.StatusAccepted, rec.Code, "a non-owner org member must see the consent screen, never auto-approve; body=%s", rec.Body.String())
	require.Empty(t, f.pendingAuthorizationCodes(t, clientID), "no code may be minted without an explicit decision")
}

// The narrowing above must not have gone too far: an org member who did NOT
// register the client is still someone who may SEE and DECIDE ON the
// request (userOwnsClient's broader owner-or-org-member test, kept
// deliberately for this gate) -- they just cannot be auto-approved with
// zero interaction. A deliberate POST decision=approve must still succeed
// and must still pass the ownership gate rather than hitting the 403 page.
func TestAuthorizeOrgMemberPassesOwnershipGateAndCanExplicitlyApprove(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	f.addOrgMember(t, 1, 7, service.OrgRoleMember)

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.post(t, q, "approve", 7)

	require.NotEqual(t, http.StatusForbidden, rec.Code, "org member must pass the ownership gate, not be treated as a non-owner")
	require.Equal(t, http.StatusOK, rec.Code, "an explicit approve from an org member must mint a code; body=%s", rec.Body.String())
	rows := f.authorizationCodes(t, clientID)
	require.Len(t, rows, 1)
	require.EqualValues(t, 7, rows[0].UserID, "the code must be minted for the approving org member, not the client's registrant")
}

// ---- Login round trip ---------------------------------------------------

func TestAuthorizeUnauthenticatedRedirectsToLoginWithFullQueryPreserved(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "csrf-state-1")
	rec := f.get(t, q, 0 /* no header -- unauthenticated */)

	require.Equal(t, http.StatusFound, rec.Code)
	loc := rec.Header().Get("Location")
	require.Contains(t, loc, "/login?redirect=")

	parsed, err := url.Parse(loc)
	require.NoError(t, err)
	// The "redirect" query value is the ORIGINAL request's raw
	// (still-percent-encoded) path+query, exactly as
	// c.Request.URL.RequestURI() returned it -- parsed.Query().Get already
	// undoes the one layer of percent-encoding url.QueryEscape added when
	// building the /login?redirect=... URL, so what's left is that raw
	// RequestURI string, colon and all still escaped as %3A.
	redirectParam := parsed.Query().Get("redirect")
	require.Contains(t, redirectParam, "/oauth/authorize?")
	require.Contains(t, redirectParam, "client_id="+url.QueryEscape(clientID))
	require.Contains(t, redirectParam, "state=csrf-state-1")
	require.Contains(t, redirectParam, "code_challenge=")
}

// ---- Redirect-with-error class: reached without needing auth ------------

func TestAuthorizeUnsupportedResponseTypeRedirectsWithError(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	q.Set("response_type", "token")
	// No auth header at all -- this must still redirect (not force a login
	// round trip) since the rejection doesn't depend on who is asking.
	rec := f.get(t, q, 0)

	require.Equal(t, http.StatusFound, rec.Code)
	loc := rec.Header().Get("Location")
	require.Contains(t, loc, "https://agent.example.com/auth/callback?")
	require.Contains(t, loc, "error=unsupported_response_type")
	require.Contains(t, loc, "state=s1")
}

func TestAuthorizeInvalidScopeRedirectsWithError(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", "not:a:real:scope", "s1")
	rec := f.get(t, q, 0)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "error=invalid_scope")
}

func TestAuthorizeBadCodeChallengeRedirectsWithError(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	q.Set("code_challenge_method", "plain")
	rec := f.get(t, q, 0)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "error=invalid_request")
}

// ---- Auto-approve vs consent --------------------------------------------

func TestAuthorizeAutoApprovesExactDashboardScopeForOwner(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	// F5: RFC 6749 §5.1 requires no-store/no-cache on any response carrying
	// a credential -- this body carries the authorization code itself.
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	body := rec.Body.String()
	require.Contains(t, body, "https://agent.example.com/auth/callback?")
	require.Contains(t, body, "code=")
	require.Contains(t, body, "state=s1")
	require.NotContains(t, body, "error=")

	// F13: "issues a code" must mean a row actually exists, not just that
	// the body contains the substring "code=".
	rows := f.authorizationCodes(t, clientID)
	require.Len(t, rows, 1)
	require.Equal(t, "pending", rows[0].Status)
	require.EqualValues(t, 42, rows[0].UserID)
	require.Equal(t, service.ScopeAgentDashboardAccess, rows[0].Scope)
}

func TestAuthorizeWiderScopeRequiresConsentEvenForOwner(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess+" "+service.ScopeAgentsManage, "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestAuthorizePostApproveIssuesCodeAfterConsent(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentsRead, "s2")
	// GET first reports consent_required...
	getRec := f.get(t, q, 42)
	require.Equal(t, http.StatusAccepted, getRec.Code)

	// ...then explicit approval mints the code.
	rec := f.post(t, q, "approve", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	// F5 (review NEW-10): POST approve was one of respondWithTarget's arms
	// left unasserted for Cache-Control/Pragma -- this response carries an
	// authorization code exactly like the GET auto-approve success body
	// already covered, so it needs the same RFC 6749 §5.1 headers.
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	body := rec.Body.String()
	require.Contains(t, body, "code=")
	require.Contains(t, body, "state=s2")
}

func TestAuthorizePostDenyRedirectsWithAccessDeniedAndIssuesNoCode(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentsRead, "s3")
	rec := f.post(t, q, "deny", 42)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	body := rec.Body.String()
	require.Contains(t, body, "error=access_denied")
	require.Contains(t, body, "state=s3")
	require.NotContains(t, body, "code=")

	// F13: "issues no code" must mean zero rows, not merely that the body
	// happens not to contain the substring "code=" -- a handler that
	// minted a code and forgot to include it in the body would otherwise
	// pass this assertion.
	require.Empty(t, f.authorizationCodes(t, clientID), "deny must not persist any authorization code row")
}

// ---- New scope rule ------------------------------------------------------

func TestAuthorizeBillingManageScopeRedirectsWithInvalidScope(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	// oauth_scope_vocabulary.go documents billing:manage as never grantable
	// at initial login -- only via a second, explicit device-flow
	// elevation. /oauth/authorize must not open a second path to it, even
	// though billing:manage is otherwise a member of the valid vocabulary
	// (so plain ValidateScope alone would let it through).
	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeBillingManage, "s1")
	rec := f.get(t, q, 0) // no auth needed to reject this -- redirect-class, pre-bearer-check
	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "error=invalid_scope")

	// Also rejected when bundled with an otherwise-fine scope, and even for
	// the client's own owner attempting an explicit POST approve -- the
	// billing:manage check runs before the method/decision branch, same as
	// every other redirect-class rejection that doesn't need to know who
	// is asking, so this is a real 302 too, not the 200/target-body
	// protocol the POST decision arms otherwise use.
	q2 := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentsRead+" "+service.ScopeBillingManage, "s2")
	rec2 := f.post(t, q2, "approve", 42)
	require.Equal(t, http.StatusFound, rec2.Code)
	require.Contains(t, rec2.Header().Get("Location"), "error=invalid_scope")
	require.Empty(t, f.authorizationCodes(t, clientID))
}

// ---- F9: repeat issuance must not leave more than one live code ---------

func TestAuthorizeRepeatAutoApproveInvalidatesPriorPendingCode(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")

	first := f.get(t, q, 42)
	require.Equal(t, http.StatusOK, first.Code)
	firstCode := codeFromTarget(t, first.Body.String())

	// Simulates AuthorizeConsentView.vue remounting (Back-navigation, a
	// refresh, a duplicated onMounted) and re-issuing the same request --
	// review found this previously left BOTH codes independently
	// redeemable, N calls meaning N live credentials for one authorization.
	second := f.get(t, q, 42)
	require.Equal(t, http.StatusOK, second.Code)
	secondCode := codeFromTarget(t, second.Body.String())
	require.NotEqual(t, firstCode, secondCode, "each issuance still mints a fresh code")

	rows := f.authorizationCodes(t, clientID)
	require.Len(t, rows, 2, "both attempts persist a row")
	pending := f.pendingAuthorizationCodes(t, clientID)
	require.Len(t, pending, 1, "only the most recent issuance may still be pending/redeemable")
	require.Equal(t, secondCode, pending[0].Code)

	// The superseded code is not silently dropped -- it's marked consumed
	// with no issued_token_family, the schema's existing "invalidated
	// without ever being redeemed" state (see
	// ent/schema/oauth_authorization_code.go), not deleted.
	for _, row := range rows {
		if row.Code == firstCode {
			require.Equal(t, "consumed", row.Status)
			require.Nil(t, row.IssuedTokenFamily)
		}
	}
}

// codeFromTarget extracts the `code` query parameter from a
// respondWithTarget body (redirect_uri?code=...&state=...).
func codeFromTarget(t *testing.T, target string) string {
	t.Helper()
	parsed, err := url.Parse(target)
	require.NoError(t, err)
	code := parsed.Query().Get("code")
	require.NotEmpty(t, code, "target has no code param: %s", target)
	return code
}

// ---- F14: scope is normalised once, not just at the comparison site -----

func TestAuthorizeScopeIsTrimmedBeforeBothComparingAndPersisting(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", " "+service.ScopeAgentDashboardAccess+" ", "s1")
	rec := f.get(t, q, 42)

	require.Equal(t, http.StatusOK, rec.Code, "a scope that trims down to the exact auto-approve scope must still auto-approve")
	rows := f.authorizationCodes(t, clientID)
	require.Len(t, rows, 1)
	require.Equal(t, service.ScopeAgentDashboardAccess, rows[0].Scope, "the stored scope must be trimmed, not the untrimmed raw query value")
}

// ---- F11: GET /api/oauth/authorize/pending -------------------------------

func TestPendingAuthorizationRequiresAuth(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	rec := f.pending(t, clientID, 0)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPendingAuthorizationReturnsDisplayNameForOwner(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	rec := f.pending(t, clientID, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			ClientName string `json:"client_name"`
			ClientID   string `json:"client_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "test-agent", body.Data.ClientName)
	require.Equal(t, clientID, body.Data.ClientID)
}

func TestPendingAuthorizationReturnsDisplayNameForOrgMember(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	f.addOrgMember(t, 1, 7, service.OrgRoleMember)

	rec := f.pending(t, clientID, 7)
	require.Equal(t, http.StatusOK, rec.Code)
}

// F11: an authenticated user with no relationship to the client must not
// be able to learn its display name -- the exact disclosure review found:
// any authenticated Inferno user who obtained a client_id could previously
// learn another org's client's owner-chosen name.
func TestPendingAuthorizationNonOwnerGetsNotFoundNotTheName(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")

	rec := f.pending(t, clientID, 99)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "test-agent")
}

func TestPendingAuthorizationUnknownClientReturnsNotFound(t *testing.T) {
	f := newAuthorizeTestFixture(t)

	rec := f.pending(t, "agent:does-not-exist", 42)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// F11: a revoked client must read identically to an unknown one -- the
// unfiltered ByClientID lookup this replaced un-collapsed exactly the
// unknown-vs-revoked distinction service.ErrClientNotUsable's doc comment
// says must stay collapsed.
func TestPendingAuthorizationRevokedClientReturnsNotFoundNotTheName(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	_, err := f.ent.OAuthClient.Update().
		Where(oauthclient.ClientID(clientID)).
		SetStatus(service.ClientRevoked).
		Save(context.Background())
	require.NoError(t, err)

	rec := f.pending(t, clientID, 42)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "test-agent")
}

// ---- F5 (review NEW-10): respondWithTarget's headers, asserted directly --

// TestRespondWithTargetSetsNoStoreHeaders covers respondWithTarget itself
// rather than one specific call site. Auto-approve success and POST deny
// already assert this end to end; this closes the gap for the arms that
// are NOT independently reachable through a normal request (IssueCode's
// own invalid_request/invalid_scope redirect-class bodies -- the handler's
// own pre-checks already reject those inputs before ever calling
// IssueCode, so those two branches are defense-in-depth against a TOCTOU,
// not a path a test can drive end to end). Every arm that calls
// respondWithTarget shares this one function, so asserting it here is a
// direct, not a structural, guarantee for all of them.
func TestRespondWithTargetSetsNoStoreHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)

	respondWithTarget(c, "https://agent.example.com/auth/callback?code=abc&state=s1")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "https://agent.example.com/auth/callback?code=abc&state=s1", rec.Body.String())
}
