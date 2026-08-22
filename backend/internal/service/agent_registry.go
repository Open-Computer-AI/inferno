package service

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// agentOnlineThreshold is how recently an agent must have heartbeated to
// read as "online" rather than "last seen X ago". Chosen against no fixed
// heartbeat interval spec; wide enough to absorb ordinary network/scheduler
// jitter between heartbeats without flapping an agent to "offline" between
// two consecutive, healthy calls.
const agentOnlineThreshold = 2 * time.Minute

// AgentRegistryService is the service layer over the `agents` table (Task 1's
// dbent.Agent schema): register/heartbeat and list. It talks to dbent.Agent
// directly rather than through a narrow interface like BillingContractService
// -- there is no repository layer for agents yet, and Register's UPSERT needs
// ent's own OnConflict builder, which a repository interface would have to
// re-expose anyway.
type AgentRegistryService struct {
	client *dbent.Client

	// now is timezone.Now in production. Injected so the status test can pin
	// "now" and assert the exact prose the mapper derives from it -- see
	// billing_contract.go's identical seam for the same reason.
	now func() time.Time
}

func NewAgentRegistryService(client *dbent.Client) *AgentRegistryService {
	return &AgentRegistryService{
		client: client,
		now:    timezone.Now,
	}
}

// RegisterAgentInput is what an agent tells Inferno about itself at boot
// (controller ruling P-1, task-2-brief.md). Fields are what the agent can
// actually know about itself.
type RegisterAgentInput struct {
	PublicID         string // the agent's OAuth client_id, e.g. "agent:abc"
	Name             string
	DashboardURL     string
	OCPlatformUserID string // optional; "" when unknown
}

// AgentView is the service-layer read model of one agent, shared by
// Register's response and ListForUser's rows.
type AgentView struct {
	// ID is the agent's public_id, not the int64 row id -- GET /api/agents
	// emits public_id as `id` (ent/schema/agent.go's doc comment), and the
	// desktop drops any agent whose id is not a string
	// (apps/desktop/electron/main.ts:7922).
	ID           string
	Name         string
	Status       string
	DashboardURL string

	// DashboardGatewayState has no backing column on the agents schema
	// (Task 1) and no data source anywhere in this task's brief -- it is
	// always "" rather than an invented value, the same "not invented"
	// discipline billing_contract.go documents for its own absent fields
	// (e.g. BillingStateView.MonthlyCap.LimitUSD). A later task can wire a
	// real source in without touching this type's shape.
	DashboardGatewayState string
}

// Register is an UPSERT keyed on public_id (agent/schema/agent.go declares
// it globally .Unique(), so a public_id can only ever belong to one agent --
// "keyed on (user_id, public_id)" per the brief collapses to public_id alone)
// so a rebooting agent heartbeats instead of duplicating itself. It sets
// last_seen_at = now() on every call, including when the row already existed.
//
// OWNERSHIP CHECK (ruling T2-2). public_id IS the agent's OAuth client_id
// (ent/schema/agent.go:32-34), and this credential model treats client_ids as
// PUBLIC by design -- the agent holds a public client_id, never a secret. So
// OnConflictColumns(agent.FieldPublicID) alone is not safe: it targets
// public_id ALONE and, with UpdateNewValues(), would overwrite EVERY column
// on conflict -- including user_id and org_id -- with no check that the
// existing row belongs to the caller. Any authenticated user who learned
// another agent's public_id could silently steal it.
//
// A composite conflict target (user_id, public_id) does NOT fix this:
// public_id is globally .Unique() in the schema (correct -- a client_id must
// be globally unique), so there is no composite unique index for a
// (user_id, public_id) ON CONFLICT clause to reference; a second user
// registering the same public_id would hit the plain unique constraint
// instead of reaching that ON CONFLICT path at all.
//
// So this looks the row up by public_id FIRST. If it exists and belongs to a
// different user, this returns a typed error before ever reaching the
// upsert -- closing the hijack while keeping public_id globally unique. One
// extra SELECT on a boot-time path is an acceptable cost.
func (s *AgentRegistryService) Register(ctx context.Context, userID, orgID int64, in RegisterAgentInput) (*AgentView, error) {
	if in.PublicID == "" {
		return nil, infraerrors.BadRequest("AGENT_PUBLIC_ID_REQUIRED", "public_id is required")
	}

	existing, err := s.client.Agent.Query().Where(agent.PublicIDEQ(in.PublicID)).Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("agent registry: look up existing owner of public_id %q: %w", in.PublicID, err)
	}
	if err == nil && existing.UserID != userID {
		return nil, infraerrors.Forbidden("AGENT_PUBLIC_ID_OWNED_BY_ANOTHER_USER",
			"this agent's public_id is already registered to a different user")
	}

	name := in.Name
	if name == "" {
		// Mirrors the desktop's own fallback:
		// `name: typeof a.name === 'string' ? a.name : a.id`
		// (apps/desktop/electron/main.ts:7924).
		name = in.PublicID
	}

	create := s.client.Agent.Create().
		SetPublicID(in.PublicID).
		SetUserID(userID).
		SetOrgID(orgID).
		SetName(name).
		SetDashboardURL(in.DashboardURL).
		SetLastSeenAt(s.now())
	if in.OCPlatformUserID != "" {
		create = create.SetOcPlatformUserID(in.OCPlatformUserID)
	}

	id, err := create.
		OnConflictColumns(agent.FieldPublicID).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent registry: register public_id %q: %w", in.PublicID, err)
	}

	row, err := s.client.Agent.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agent registry: reload agent %d after register: %w", id, err)
	}

	view := s.toView(row)
	return &view, nil
}

// ListForUser returns the caller's agents within the given org, excluding
// revoked ones (revoked_at IS NOT NULL). Filtered on BOTH user_id AND org_id
// (ruling T2-1): Task 3 builds the 409 org_selection_required org picker
// directly on this call (apps/desktop/electron/main.ts:7849), so a
// multi-org user who has picked org A must never see org B's agents just
// because both orgs belong to them -- an org-blind list would make that
// picker decorative. user_id alone is not enough either: it is the OTHER
// half of the isolation boundary, proven independently below by
// TestListForUserExcludesAnotherUsersAgentInTheSameOrg.
func (s *AgentRegistryService) ListForUser(ctx context.Context, userID, orgID int64) ([]AgentView, error) {
	rows, err := s.client.Agent.Query().
		Where(
			agent.UserIDEQ(userID),
			agent.OrgIDEQ(orgID),
			agent.RevokedAtIsNil(),
		).
		Order(dbent.Asc(agent.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent registry: list agents for user %d org %d: %w", userID, orgID, err)
	}

	out := make([]AgentView, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.toView(row))
	}
	return out, nil
}

// ResolveOwnedAgentRowID resolves an agent's public_id (its OAuth
// client_id -- see RegisterAgentInput.PublicID's doc comment) to its row
// id, scoped to the caller. Chronos's /api/agent-cron/* surface
// (task-4-brief.md) works in row ids, but the agent identifies itself by
// its OWN OAuth token, whose verified `aud` claim IS its public_id
// (RequireOAuthScope publishes it as the caller's identity,
// middleware/oauth_scope.go) -- there is no separate agent-id field in the
// wire contract (plugins/cron_providers/chronos/_nas_client.py has none),
// so this is the one place that translation happens.
//
// Mirrors Register's own ownership check (ruling T2-2) rather than
// inventing a second pattern: an agent id the caller does not own must not
// be addressable, so a public_id that exists but belongs to a different
// user_id is rejected with Forbidden, never silently resolved to someone
// else's row.
//
// Also excludes revoked agents (ruling T4-2), mirroring ListForUser's own
// agent.RevokedAtIsNil() filter: revocation is the intended kill switch for
// an agent, and /api/agent-cron/* is precisely the capability that keeps
// running UNATTENDED after revocation if this did not check it -- a kill
// switch that does not stop cron is not a kill switch. Treated the same as
// "not found" rather than a distinct status: a revoked agent is not
// addressable, the same as one that never existed.
func (s *AgentRegistryService) ResolveOwnedAgentRowID(ctx context.Context, userID int64, publicID string) (int64, error) {
	if publicID == "" {
		return 0, infraerrors.BadRequest("AGENT_CRON_CALLER_UNIDENTIFIED", "the calling agent could not be identified")
	}

	row, err := s.client.Agent.Query().
		Where(agent.PublicIDEQ(publicID), agent.RevokedAtIsNil()).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return 0, infraerrors.NotFound("AGENT_NOT_FOUND", "agent not found")
		}
		return 0, fmt.Errorf("agent registry: resolve owned agent row id for public_id %q: %w", publicID, err)
	}
	if row.UserID != userID {
		return 0, infraerrors.Forbidden("AGENT_NOT_OWNED_BY_CALLER", "this agent does not belong to you")
	}

	return row.ID, nil
}

// toView maps a dbent.Agent row to the service's read model, deriving Status
// as human-readable prose from last_seen_at -- apps/desktop/src/i18n/en.ts:812
// renders it VERBATIM (`cloudStatusLabel: status => "Status: ${status}"`), so
// it must read as prose, never an enum token.
func (s *AgentRegistryService) toView(row *dbent.Agent) AgentView {
	return AgentView{
		ID:           row.PublicID,
		Name:         row.Name,
		Status:       agentStatusFromLastSeen(row.LastSeenAt, s.now()),
		DashboardURL: row.DashboardURL,
	}
}

// agentStatusFromLastSeen derives the display status. lastSeenAt nil means
// the agent has never heartbeated at all (never Registered, which is the
// only writer of last_seen_at).
func agentStatusFromLastSeen(lastSeenAt *time.Time, now time.Time) string {
	if lastSeenAt == nil {
		return "never connected"
	}

	elapsed := now.Sub(*lastSeenAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed <= agentOnlineThreshold {
		return "online"
	}
	return "last seen " + agentHumanElapsed(elapsed) + " ago"
}

// agentHumanElapsed renders a duration as coarse, human-scaled prose:
// minutes under an hour, hours under a day, days beyond that. Never
// sub-minute -- anything that close to "now" already reads as "online"
// above, so this function is only ever reached with elapsed > 2 minutes.
func agentHumanElapsed(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}
