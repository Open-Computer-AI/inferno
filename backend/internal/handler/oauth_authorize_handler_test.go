package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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

	return &authorizeTestFixture{h: h, router: router, ent: entClient}
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

func TestAuthorizeOrgMemberMayAuthorizeClientTheyDidNotRegister(t *testing.T) {
	f := newAuthorizeTestFixture(t)
	clientID := f.registerClient(t, 1, 42, "https://agent.example.com")
	// user 7 didn't register the client, but IS a member of its org.
	f.addOrgMember(t, 1, 7, service.OrgRoleMember)

	q := authorizeQuery(clientID, "https://agent.example.com/auth/callback", service.ScopeAgentDashboardAccess, "s1")
	rec := f.get(t, q, 7)

	require.Equal(t, http.StatusOK, rec.Code, "org member must be able to auto-approve, body=%s", rec.Body.String())
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
	body := rec.Body.String()
	require.Contains(t, body, "https://agent.example.com/auth/callback?")
	require.Contains(t, body, "code=")
	require.Contains(t, body, "state=s1")
	require.NotContains(t, body, "error=")
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
	body := rec.Body.String()
	require.Contains(t, body, "error=access_denied")
	require.Contains(t, body, "state=s3")
	require.NotContains(t, body, "code=")
}
