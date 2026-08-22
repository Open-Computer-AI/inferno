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

// ListForUser returns the caller's own agents, excluding revoked ones
// (revoked_at IS NOT NULL). Filtered on user_id only -- orgID is accepted for
// signature symmetry with Register (the brief's Interfaces block specifies
// it), but agents.org_id is not part of this query's WHERE clause: the
// brief's own mutation-proof (Step 6) drops "the user_id predicate" and
// expects a SECOND user's agent (seeded in a different org, 8/2 vs 7/1) to
// leak into the result. If org_id were also filtered, dropping only user_id
// would not leak it (org_id=1 would still exclude org_id=2) -- so user_id is
// the sole isolation boundary this method enforces today.
func (s *AgentRegistryService) ListForUser(ctx context.Context, userID, orgID int64) ([]AgentView, error) {
	_ = orgID

	rows, err := s.client.Agent.Query().
		Where(
			agent.UserIDEQ(userID),
			agent.RevokedAtIsNil(),
		).
		Order(dbent.Asc(agent.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent registry: list agents for user %d: %w", userID, err)
	}

	out := make([]AgentView, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.toView(row))
	}
	return out, nil
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
