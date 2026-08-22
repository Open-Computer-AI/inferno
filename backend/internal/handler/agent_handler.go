package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AgentHandler serves the two surfaces built on top of Task 2's
// AgentRegistryService:
//
//   - GET /api/agents -- the desktop's Hermes Cloud discovery call
//     (apps/desktop/electron/main.ts's discoverCloudAgents /
//     trimCloudAgents / trimCloudOrg). Like BillingContractHandler and
//     OAuthHandler.Account, this does NOT use internal/pkg/response's
//     {code,message,data} envelope -- the desktop parses the raw object at
//     the top level, and an envelope here is silently mis-parsed (every
//     agent dropped, not an error) rather than rejected.
//   - POST /api/agents/register -- an agent's own boot-time
//     register/heartbeat call. This is OUR OWN endpoint (no shipping
//     client hardcodes its body shape the way the desktop hardcodes
//     GET /api/agents), so its REQUEST body follows Inferno's own
//     snake_case convention -- see agentRegisterRequest's field tags --
//     while its response still matches GET's camelCase agent shape, for
//     consistency between the two.
type AgentHandler struct {
	svc    *service.AgentRegistryService
	orgSvc *service.OrgService
}

func NewAgentHandler(svc *service.AgentRegistryService, orgSvc *service.OrgService) *AgentHandler {
	return &AgentHandler{svc: svc, orgSvc: orgSvc}
}

// agentHandlerUserID mirrors billingContractUserID: the caller's identity
// comes from the gin context key middleware.RequireOAuthScope sets, never
// from a query parameter or body field.
func agentHandlerUserID(c *gin.Context) (int64, bool) {
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

// orgJSON renders one org into the exact shape both GET /api/agents' `org`
// key and the 409 org_selection_required `orgs` list use.
// apps/desktop/electron/main.ts's trimCloudOrg and parseOrgSelectionError
// both read {id, slug, name, isPersonal} and both gate on
// `typeof o.id === 'string'` before trusting the row at all -- id is a JSON
// STRING, exactly the discipline AgentView.ID's doc comment (agent_registry.go)
// documents for the agent id, and the same projection
// OAuthHandler.Account already uses for GET /api/oauth/account's org list.
func orgJSON(o *dbent.Org) gin.H {
	return gin.H{
		"id":         strconv.FormatInt(o.ID, 10),
		"slug":       o.Slug,
		"name":       o.Name,
		"isPersonal": o.IsPersonal,
	}
}

// agentJSON renders one AgentView into GET /api/agents' / the register
// response's agent shape. id is the service's AgentView.ID -- already the
// agent's public_id string, never the int64 row id (see AgentView's doc
// comment) -- so no conversion is needed here, only the key rename to
// camelCase.
func agentJSON(v service.AgentView) gin.H {
	return gin.H{
		"id":                    v.ID,
		"name":                  v.Name,
		"status":                v.Status,
		"dashboardUrl":          v.DashboardURL,
		"dashboardGatewayState": v.DashboardGatewayState,
	}
}

// resolveOrg picks the org that scopes this request's agent visibility, and
// is what makes GET /api/agents' 409 org picker (main.ts:parseOrgSelectionError)
// and its ?org= re-call (main.ts's discoverCloudAgents, gateway-settings.tsx's
// `org.slug ?? org.id`) actually work:
//
//   - ?org=<slug> query param present: must name an org the caller is a
//     member of. An unrecognized slug must NOT silently fall back to another
//     org -- that would leak agents from an org the caller picked away from
//     -- so this 403s instead of guessing.
//   - no ?org param, exactly one membership: used implicitly. This is the
//     common case -- most users belong only to their personal org, and the
//     desktop must not force a picker on every single-org user.
//   - no ?org param, two or more memberships: 409
//     {"error":"org_selection_required","orgs":[...]}, so the desktop can
//     render a picker and re-call with ?org=<chosen slug>.
//   - zero memberships: every user gets a personal org at login
//     (OrgService.EnsurePersonalOrg), so this is a data-integrity fault, not
//     a legitimate empty state -- 500, not a silently empty picker.
//
// Writes the HTTP response itself and returns ok=false on every non-success
// path, mirroring agentHandlerUserID's pattern -- callers just `return` on
// !ok.
func (h *AgentHandler) resolveOrg(c *gin.Context, userID int64) (*dbent.Org, bool) {
	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("agents: org lookup failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return nil, false
	}

	if slug := c.Query("org"); slug != "" {
		for _, o := range orgs {
			if o.Slug == slug {
				return o, true
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "org_not_found"})
		return nil, false
	}

	switch len(orgs) {
	case 0:
		slog.Error("agents: user has no org membership", "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return nil, false
	case 1:
		return orgs[0], true
	default:
		list := make([]gin.H, 0, len(orgs))
		for _, o := range orgs {
			list = append(list, orgJSON(o))
		}
		c.JSON(http.StatusConflict, gin.H{"error": "org_selection_required", "orgs": list})
		return nil, false
	}
}

// List handles GET /api/agents. See this type's doc comment for the bare-JSON
// / camelCase rules and resolveOrg's doc comment for the org-selection logic.
func (h *AgentHandler) List(c *gin.Context) {
	userID, ok := agentHandlerUserID(c)
	if !ok {
		return
	}
	if h.svc == nil || h.orgSvc == nil {
		slog.Error("agents: registry service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	org, ok := h.resolveOrg(c, userID)
	if !ok {
		return
	}

	views, err := h.svc.ListForUser(c.Request.Context(), userID, org.ID)
	if err != nil {
		slog.Error("agents: list failed", "user_id", userID, "org_id", org.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	agents := make([]gin.H, 0, len(views))
	for _, v := range views {
		agents = append(agents, agentJSON(v))
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"org":    orgJSON(org),
	})
}

// agentRegisterRequest is POST /api/agents/register's body -- snake_case,
// this codebase's own convention (see this file's top doc comment for why
// that differs from GET's camelCase response).
type agentRegisterRequest struct {
	PublicID         string `json:"public_id"`
	Name             string `json:"name"`
	DashboardURL     string `json:"dashboard_url"`
	OCPlatformUserID string `json:"oc_platform_user_id"`
}

// Register handles POST /api/agents/register -- an agent's own boot-time
// register/heartbeat call (service.AgentRegistryService.Register's doc
// comment: an UPSERT keyed on public_id).
//
// Errors from the service are mapped through infraerrors.Code/Reason/Message
// -- the same pattern billingContractWriteError uses -- so
// infraerrors.Forbidden("AGENT_PUBLIC_ID_OWNED_BY_ANOTHER_USER", ...)
// (ruling T2-2: another user already owns this public_id) surfaces as 403,
// not 500, and infraerrors.BadRequest("AGENT_PUBLIC_ID_REQUIRED", ...)
// surfaces as 400. Any error that is NOT an *infraerrors.ApplicationError
// (a real infrastructure failure) is logged and reported as a bare 500,
// never echoed to the caller.
func (h *AgentHandler) Register(c *gin.Context) {
	userID, ok := agentHandlerUserID(c)
	if !ok {
		return
	}
	if h.svc == nil || h.orgSvc == nil {
		slog.Error("agents: registry service is not wired")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req agentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "public_id is required"})
		return
	}

	org, ok := h.resolveOrg(c, userID)
	if !ok {
		return
	}

	view, err := h.svc.Register(c.Request.Context(), userID, org.ID, service.RegisterAgentInput{
		PublicID:         req.PublicID,
		Name:             req.Name,
		DashboardURL:     req.DashboardURL,
		OCPlatformUserID: req.OCPlatformUserID,
	})
	if err != nil {
		status := infraerrors.Code(err)
		if status == 0 {
			status = http.StatusInternalServerError
		}
		if status == http.StatusInternalServerError {
			slog.Error("agents: register failed", "user_id", userID, "org_id", org.ID, "error", err)
		}
		c.JSON(status, gin.H{"error": infraerrors.Reason(err), "message": infraerrors.Message(err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agent": agentJSON(*view),
		"org":   orgJSON(org),
	})
}
