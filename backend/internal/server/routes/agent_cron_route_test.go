package routes

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// These tests run the REAL chain: the real RS256 key service, the real
// oauth_client registry, the real RequireOAuthScope middleware, the real
// AgentHandler, the real AgentCronService and the real AgentRegistryService,
// all over a real (sqlite) ent client -- mirroring agents_route_test.go,
// which shares this file's harness (agentRouteEnv).
//
// The wire contract these tests pin is
// plugins/cron_providers/chronos/_nas_client.py:96-123's NasCronClient --
// path, snake_case body keys, and the fact that the CALLING AGENT is
// identified by the bearer token's `aud` claim (its own public_id), never a
// request-body field. See agent_handler.go's agentCronHandlerAgentRowID.

// ===========================================================================
// POST /api/agent-cron/provision
// ===========================================================================

// TestAgentCronProvisionRouteArmsAndReturnsScheduleID is the happy path,
// wire-shape pinned to NasCronClient.provision's request
// (_nas_client.py:96-108) and the "e.g. {schedule_id}" response it
// documents.
func TestAgentCronProvisionRouteArmsAndReturnsScheduleID(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-1")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-1", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+token,
		`{"job_id":"job-1","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"job-1:2026-09-01T10:00:00Z"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	raw := rec.Body.String()
	require.NotContains(t, raw, `"code":`, "bare JSON, never the panel envelope")

	var body struct {
		JobID      string `json:"job_id"`
		FireAt     string `json:"fire_at"`
		ScheduleID string `json:"schedule_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	require.Equal(t, "job-1", body.JobID)
	require.NotEmpty(t, body.ScheduleID)
}

// TestAgentCronProvisionRouteIsIdempotentOnSameDedupKey: a re-arm (the
// agent's cold-start reconcile) is normal traffic, never an error, and
// returns the SAME schedule_id both times (AgentCronService.Provision's
// doc comment).
func TestAgentCronProvisionRouteIsIdempotentOnSameDedupKey(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-2")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-2", service.ScopeInferenceInvoke)
	payload := `{"job_id":"job-1","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"job-1:2026-09-01T10:00:00Z"}`

	first := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+token, payload)
	require.Equal(t, http.StatusOK, first.Code, "body: %s", first.Body.String())
	second := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+token, payload)
	require.Equal(t, http.StatusOK, second.Code, "a re-arm must succeed, not error; body: %s", second.Body.String())

	var firstBody, secondBody struct {
		ScheduleID string `json:"schedule_id"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondBody))
	require.Equal(t, firstBody.ScheduleID, secondBody.ScheduleID, "same fire, same schedule_id")
}

// TestAgentCronProvisionRouteRejectsATokenForAnUnregisteredAgent: a
// validly-signed token whose aud names no agent at all must not silently
// resolve to "no agent" and proceed -- it must be refused.
func TestAgentCronProvisionRouteRejectsATokenForAnUnregisteredAgent(t *testing.T) {
	env := newAgentRouteEnv(t)
	// No registerAgentClient call: the client and token are minted for a
	// public_id that names no Agent row.
	token := env.mintForClient(t, agentRouteUserID, "agent:never-registered", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+token,
		`{"job_id":"job-1","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"job-1:2026-09-01T10:00:00Z"}`)

	require.NotEqual(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ===========================================================================
// GET /api/agent-cron/list
// ===========================================================================

// TestAgentCronListRouteReturnsOnlyThisAgentsArmedFires is the cross-tenant
// test at the wire layer: TWO distinct agents (distinct public_ids, hence
// distinct OAuth clients and distinct agent_row_ids) each provision a fire;
// the caller's own token must only ever see its own agent's fire.
func TestAgentCronListRouteReturnsOnlyThisAgentsArmedFires(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:mine")
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:another") // a SECOND agent, same user, same database
	mine := env.mintForClient(t, agentRouteUserID, "agent:mine", service.ScopeInferenceInvoke)
	another := env.mintForClient(t, agentRouteUserID, "agent:another", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+mine,
		`{"job_id":"mine-job","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"mine-job:2026-09-01T10:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+another,
		`{"job_id":"another-job","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"another-job:2026-09-01T10:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.get(t, "/api/agent-cron/list", "Bearer "+mine)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Armed []map[string]any `json:"armed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Armed, 1, "?list must return ONLY this agent's armed fires")
	require.Equal(t, "mine-job", body.Armed[0]["job_id"])
}

// TestAgentCronListRouteIsolatesTwoGenuinelyDifferentUsers is the minor
// finding's fix: TestAgentCronListRouteReturnsOnlyThisAgentsArmedFires above
// only ever uses agentRouteUserID for both agents, so it cannot tell
// user_id isolation apart from agent_row_id isolation. This seeds a SECOND
// real user (9) with their own registered agent and confirms each user's
// list call sees only their own agent's fire.
func TestAgentCronListRouteIsolatesTwoGenuinelyDifferentUsers(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:user7")
	env.registerAgentClient(t, 9, 1, "agent:user9") // a SECOND, genuinely different user
	user7Token := env.mintForClient(t, agentRouteUserID, "agent:user7", service.ScopeInferenceInvoke)
	user9Token := env.mintForClient(t, 9, "agent:user9", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+user7Token,
		`{"job_id":"user7-job","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"user7-job:2026-09-01T10:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+user9Token,
		`{"job_id":"user9-job","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"user9-job:2026-09-01T10:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.get(t, "/api/agent-cron/list", "Bearer "+user7Token)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var body struct {
		Armed []map[string]any `json:"armed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Armed, 1, "user 7 must see only their own fire, never user 9's")
	require.Equal(t, "user7-job", body.Armed[0]["job_id"])
}

// TestAgentCronProvisionRouteRejectsATokenWhoseSubDoesNotOwnTheAgentNamedByAud
// forges the other half of the ownership check: a validly-signed token
// whose `aud` names an agent registered to a DIFFERENT user_id than the
// token's own `sub`. agentCronHandlerAgentRowID must refuse this via
// ResolveOwnedAgentRowID's ownership check (ruling T2-2-shaped), not
// silently resolve to the agent the aud names.
func TestAgentCronProvisionRouteRejectsATokenWhoseSubDoesNotOwnTheAgentNamedByAud(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:owned-by-user7")
	// A token whose sub is a DIFFERENT, genuine user (9) but whose aud names
	// user 7's agent.
	forged := env.mintForClient(t, 9, "agent:owned-by-user7", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+forged,
		`{"job_id":"job-1","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"job-1:2026-09-01T10:00:00Z"}`)

	require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
}

// ===========================================================================
// POST /api/agent-cron/cancel
// ===========================================================================

// TestAgentCronCancelRouteMarksCancelledAndRemovesFromList is the round
// trip: provision, cancel, then confirm list no longer carries it.
func TestAgentCronCancelRouteMarksCancelledAndRemovesFromList(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-cancel")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-cancel", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "Bearer "+token,
		`{"job_id":"job-1","fire_at":"2026-09-01T10:00:00Z","agent_callback_url":"https://agent.example/","dedup_key":"job-1:2026-09-01T10:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.do(t, http.MethodPost, "/api/agent-cron/cancel", "Bearer "+token, `{"job_id":"job-1"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = env.get(t, "/api/agent-cron/list", "Bearer "+token)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Armed []map[string]any `json:"armed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Empty(t, body.Armed, "a cancelled fire must be absent from list")
}

// TestAgentCronCancelRouteOnUnknownJobIDIsANoOp: the agent cancels
// optimistically -- an unrecognized job_id must 200, never error
// (AgentCronService.Cancel's doc comment).
func TestAgentCronCancelRouteOnUnknownJobIDIsANoOp(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-noop")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-noop", service.ScopeInferenceInvoke)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/cancel", "Bearer "+token, `{"job_id":"no-such-job"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ===========================================================================
// Cross-cutting
// ===========================================================================

// TestAgentCronRoutesRejectAnUnauthenticatedCall: no token -> 401, on all
// three routes.
func TestAgentCronRoutesRejectAnUnauthenticatedCall(t *testing.T) {
	env := newAgentRouteEnv(t)

	rec := env.do(t, http.MethodPost, "/api/agent-cron/provision", "", `{"job_id":"job-1"}`)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = env.do(t, http.MethodPost, "/api/agent-cron/cancel", "", `{"job_id":"job-1"}`)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = env.get(t, "/api/agent-cron/list", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAgentCronRoutesAdmitAnyValidScope proves the endpoints do NOT gate on
// agents:read/agents:manage (Global Constraints) -- the exact defect three
// billing:read endpoints shipped with on the previous branch.
func TestAgentCronRoutesAdmitAnyValidScope(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-scope")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-scope", service.ScopeInferenceInvoke)

	rec := env.get(t, "/api/agent-cron/list", "Bearer "+token)
	require.Equal(t, http.StatusOK, rec.Code,
		"a token carrying only the scope a real client requests must be admitted; body: %s", rec.Body.String())
}

// TestAgentCronRoutesAreNotMountedUnderAPIV1 mirrors
// TestAgentsRouteIsNotMountedUnderAPIV1: the chronos client hardcodes
// /api/agent-cron/*, not the panel's versioned prefix.
func TestAgentCronRoutesAreNotMountedUnderAPIV1(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.registerAgentClient(t, agentRouteUserID, 1, "agent:cron-v1")
	token := env.mintForClient(t, agentRouteUserID, "agent:cron-v1", service.ScopeInferenceInvoke)

	rec := env.get(t, "/api/v1/agent-cron/list", "Bearer "+token)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
