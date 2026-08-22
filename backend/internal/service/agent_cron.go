package service

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agentcronfire"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/google/uuid"
)

// AgentCronService is the service layer over the `agent_cron_fires` table
// (Task 1's dbent.AgentCronFire schema): provision/cancel/list the one-shot
// fires behind /api/agent-cron/{provision,cancel,list}. Modeled on
// AgentRegistryService -- it talks to dbent.AgentCronFire directly (there is
// no repository layer for this table either), and Provision's idempotent
// re-arm needs ent's own OnConflict builder the same way Register's UPSERT
// does.
//
// CHRONOS IS NOT A CRON ENGINE. The agent arms a SINGLE fire_at and re-arms
// after each fire -- there is no recurrence rule, no cron expression, no
// timezone arithmetic here, by design (task-4-brief.md).
//
// Every request/response key this service's callers use is cited to
// plugins/cron_providers/chronos/_nas_client.py:96-123 (NasCronClient), the
// agent-side client this table's rows exist to serve.
type AgentCronService struct {
	client *dbent.Client

	// newScheduleID mints this service's own schedule_id on a fresh arm.
	// Injected so a test can pin it, mirroring AgentRegistryService.now's
	// seam for the same reason.
	newScheduleID func() string
}

func NewAgentCronService(client *dbent.Client) *AgentCronService {
	return &AgentCronService{
		client:        client,
		newScheduleID: newAgentCronScheduleID,
	}
}

func newAgentCronScheduleID() string {
	return "cron_" + uuid.NewString()
}

// ProvisionInput is what an agent asks NAS to arm, cited verbatim to
// NasCronClient.provision's body (_nas_client.py:96-108):
// {job_id, fire_at, agent_callback_url, dedup_key}.
type ProvisionInput struct {
	JobID  string
	FireAt string // RFC3339/ISO 8601, e.g. "2026-09-01T10:00:00Z"

	// AgentCallbackURL is where this fire calls back into the agent.
	AgentCallbackURL string

	// DedupKey is "{job_id}:{fire_at}" (_nas_client.py:100-101), UNIQUE at
	// the database (ent/schema/agent_cron_fire.go). This is the entire
	// idempotency mechanism -- see Provision's doc comment.
	DedupKey string
}

// ArmedFireView is the service's read model of one armed fire, shared by
// Provision's and ListArmed's responses. Mirrors the shape
// NasCronClient.list_armed documents each item as carrying:
// {job_id, fire_at, schedule_id} (_nas_client.py:116-119).
type ArmedFireView struct {
	JobID      string
	FireAt     string
	ScheduleID string
}

// Provision arms (or re-arms) one one-shot fire. The UNIQUE dedup_key
// constraint is the entire idempotency mechanism: the agent's cold-start
// reconcile re-arms every job it still wants (_nas_client.py:114-120), so
// calling this twice with the SAME dedup_key is normal reconcile traffic,
// never an error -- it must return the SAME schedule_id both times, and
// must never create a second row.
//
// This is deliberately NOT a read-then-write existence check -- that races
// with itself under two concurrent re-arms. It is a single
// INSERT ... ON CONFLICT (dedup_key) DO UPDATE SET updated_at = now():
// OnConflictColumns(dedup_key) + a conflict clause that touches ONLY
// updated_at (never UpdateNewValues, which would also overwrite
// schedule_id on every re-arm). Touching updated_at is what makes the
// upsert a real UPDATE, so RETURNING id still resolves to the EXISTING
// row's id on conflict -- "same fire, same schedule_id" -- while a fresh
// dedup_key still inserts a brand new row with the schedule_id minted for
// this call.
func (s *AgentCronService) Provision(ctx context.Context, agentRowID int64, in ProvisionInput) (*ArmedFireView, error) {
	if in.JobID == "" {
		return nil, infraerrors.BadRequest("AGENT_CRON_JOB_ID_REQUIRED", "job_id is required")
	}
	if in.DedupKey == "" {
		return nil, infraerrors.BadRequest("AGENT_CRON_DEDUP_KEY_REQUIRED", "dedup_key is required")
	}
	if in.AgentCallbackURL == "" {
		return nil, infraerrors.BadRequest("AGENT_CRON_CALLBACK_URL_REQUIRED", "agent_callback_url is required")
	}
	fireAt, err := time.Parse(time.RFC3339, in.FireAt)
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_CRON_FIRE_AT_INVALID", "fire_at must be RFC3339/ISO 8601")
	}

	id, err := s.client.AgentCronFire.Create().
		SetAgentRowID(agentRowID).
		SetJobID(in.JobID).
		SetFireAt(fireAt).
		SetCallbackURL(in.AgentCallbackURL).
		SetDedupKey(in.DedupKey).
		SetScheduleID(s.newScheduleID()).
		OnConflictColumns(agentcronfire.FieldDedupKey).
		UpdateUpdatedAt().
		ID(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent cron: provision job %q: %w", in.JobID, err)
	}

	row, err := s.client.AgentCronFire.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agent cron: reload fire %d after provision: %w", id, err)
	}

	view := agentCronFireView(row)
	return &view, nil
}

// Cancel marks job_id's armed fire for this agent as cancelled. Cancelling
// an unknown job_id -- already fired, already cancelled, or never armed at
// all -- is a NO-OP RETURNING NIL, not an error: the agent cancels
// optimistically and has no way to know server-side state in advance
// (task-4-brief.md Step 6). Scoped to agentRowID so one agent can never
// cancel another agent's fire by guessing its job_id.
func (s *AgentCronService) Cancel(ctx context.Context, agentRowID int64, jobID string) error {
	if jobID == "" {
		return infraerrors.BadRequest("AGENT_CRON_JOB_ID_REQUIRED", "job_id is required")
	}

	_, err := s.client.AgentCronFire.Update().
		Where(
			agentcronfire.AgentRowIDEQ(agentRowID),
			agentcronfire.JobIDEQ(jobID),
			agentcronfire.StateEQ(agentcronfire.StateArmed),
		).
		SetState(agentcronfire.StateCancelled).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("agent cron: cancel job %q: %w", jobID, err)
	}
	return nil
}

// ListArmed returns this agent's currently ARMED fires. Best-effort, used
// by the agent's cold-start reconcile (_nas_client.py:114-120): on error
// the agent's own caller falls back to idempotently re-arming everything it
// wants via Provision, so this need not be transactionally perfect, only
// correctly scoped to the caller's own agent.
func (s *AgentCronService) ListArmed(ctx context.Context, agentRowID int64) ([]ArmedFireView, error) {
	rows, err := s.client.AgentCronFire.Query().
		Where(
			agentcronfire.AgentRowIDEQ(agentRowID),
			agentcronfire.StateEQ(agentcronfire.StateArmed),
		).
		Order(dbent.Asc(agentcronfire.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent cron: list armed fires for agent %d: %w", agentRowID, err)
	}

	out := make([]ArmedFireView, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentCronFireView(row))
	}
	return out, nil
}

// agentCronFireView maps a dbent.AgentCronFire row to the service's read
// model. FireAt is re-serialized through RFC3339 rather than passed through
// the driver's raw formatting, so Provision's and ListArmed's FireAt read
// identically regardless of which one produced the row.
func agentCronFireView(row *dbent.AgentCronFire) ArmedFireView {
	return ArmedFireView{
		JobID:      row.JobID,
		FireAt:     row.FireAt.UTC().Format(time.RFC3339),
		ScheduleID: row.ScheduleID,
	}
}
