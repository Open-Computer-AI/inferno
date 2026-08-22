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

// cronArmer is the narrow slice of *AgentCronFirer that Provision needs: arm
// a freshly-(re)provisioned row into the timing wheel for THIS process,
// without waiting for the next boot. RehydrateOnBoot only covers process
// START (ent/schema/agent_cron_fire.go's doc comment: "a restart drops
// every pending timer... rows are rehydrated into the wheel on boot") -- it
// says nothing about a row provisioned while a process is already running,
// and until ruling T5-1 nothing armed that case either: Provision upserted
// the row and returned, and it sat "armed" in the database with no timer
// anywhere until the next restart. This interface is how Provision closes
// that gap.
//
// Deliberately the ONLY thing AgentCronService knows about AgentCronFirer --
// never a *AgentCronFirer field. That keeps the dependency one-directional
// (AgentCronService -> AgentCronFirer): AgentCronFirer's own methods are
// inherently cross-agent (RehydrateOnBoot must see every agent's armed rows
// at once), so it talks to dbent.AgentCronFire directly rather than holding
// a *AgentCronService whose surface is scoped to one caller -- see
// AgentCronFirer's doc comment. Two structs each holding a pointer to the
// other would force an awkward two-phase construction; routing the
// dependency through this one narrow interface avoids that entirely, and
// lets Provision's own tests substitute a fake with no real timing wheel or
// HTTP client involved.
type cronArmer interface {
	ArmNow(ctx context.Context, fireID int64, fireAt time.Time)
}

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

	// armer arms a freshly-(re)provisioned row into the CURRENT process's
	// timing wheel -- see cronArmer's doc comment. Required, not an optional
	// nil-checked dependency: a nil-tolerant "skip arming silently" design
	// would reintroduce the exact failure mode ruling T5-1 exists to close
	// (a fire that sits armed in the database with no timer anywhere until
	// the next restart) -- so a caller that forgets to wire one panics
	// immediately on the first Provision call, loudly, in tests, rather than
	// shipping a server that silently never arms anything.
	armer cronArmer

	// newScheduleID mints this service's own schedule_id on a fresh arm.
	// Injected so a test can pin it, mirroring AgentRegistryService.now's
	// seam for the same reason.
	newScheduleID func() string
}

func NewAgentCronService(client *dbent.Client, armer cronArmer) *AgentCronService {
	return &AgentCronService{
		client:        client,
		armer:         armer,
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

// Provision arms (or re-arms) one one-shot fire. The composite
// (agent_row_id, dedup_key) UNIQUE index (ent/schema/agent_cron_fire.go,
// ruling T4-1) is the entire idempotency mechanism, scoped to ONE agent:
// the agent's cold-start reconcile re-arms every job it still wants
// (_nas_client.py:114-120), so calling this twice with the SAME
// agentRowID+dedup_key is normal reconcile traffic, never an error -- it
// must return the SAME schedule_id both times, and must never create a
// second row.
//
// dedup_key ("{job_id}:{fire_at}") is NOT globally unique -- job_id lives
// in the calling agent's own namespace, so two different agents legitimately
// hold the identical dedup_key for their own, unrelated jobs. Ruling T4-1
// is why this targets the COMPOSITE index rather than dedup_key alone: a
// field-level-unique dedup_key let one agent's re-arm collide into a
// DIFFERENT agent's row (agent 2 provisioning agent 1's dedup_key got back
// agent 1's job_id/schedule_id, agent 1's row silently touched). Targeting
// the composite index makes a foreign agent's row structurally unreachable
// as a conflict target at all -- there is no INSERT this agent can issue
// that conflicts on any row it does not itself own.
//
// This is deliberately NOT a read-then-write existence check -- that races
// with itself under two concurrent re-arms. It is a single
// INSERT ... ON CONFLICT (agent_row_id, dedup_key) DO UPDATE:
// OnConflictColumns(agentRowID, dedupKey) + a conflict clause naming its
// columns one by one (never UpdateNewValues, which would also overwrite
// schedule_id on every re-arm). Touching updated_at is what
// makes the upsert a real UPDATE, so RETURNING id still resolves to the
// EXISTING row's id on conflict -- "same fire, same schedule_id" -- while a
// fresh dedup_key (or a fresh agent) still inserts a brand new row with the
// schedule_id minted for this call.
//
// RULING W-5: AN EXPLICIT PROVISION ON A TERMINAL ROW RE-ARMS IT. The
// conflict clause therefore resets state -> armed, attempts -> 0 and
// last_error -> "" alongside updated_at, but still never touches
// schedule_id (which is why this is a field-by-field conflict clause and
// not UpdateNewValues). Before this, a conflict onto a row already `fired`
// or `cancelled` touched updated_at ONLY: the row stayed terminal,
// AgentCronFirer.FireNow returns silently for any row whose state is not
// `armed`, and the call answered 200 with the original schedule_id having
// armed NOTHING -- forever, because ListArmed also hides a terminal row, so
// the agent's reconcile never learns the job is dead and keeps re-arming
// into the same no-op.
//
// That is reachable on the agent's own documented recovery path. Its
// reconcile (plugins/cron_providers/chronos/__init__.py:194-215) compares
// what it wants against _list_armed(), which returns ARMED fires only, and
// re-arms anything missing. Normally the re-arm carries the NEXT
// occurrence -- new fire_at, new dedup_key, a fresh row -- but it replays
// the IDENTICAL key in two real windows: (a) the fire lands and the agent
// crashes before persisting next_run_at, so on restart it still wants the
// old instant; (b) _list_armed itself errors, whose own log line at :189
// reads "will re-arm idempotently", so `observed` is empty and it re-arms
// everything it has. Rejecting a terminal row instead would convert both of
// those recoverable crash windows into a permanent hard failure.
//
// This does NOT weaken the schema's "a `fired` row must never re-fire".
// That invariant governs REHYDRATION (AgentCronFirer.RehydrateOnBoot, whose
// query filters to state=armed, and FireNow's own state guard) -- a boot
// nobody asked for, replaying rows nobody re-requested. Provision is
// someone asking. The two paths stay distinct and are pinned in both
// directions by TestProvisionOnAFiredRowReArmsIt and
// TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes.
//
// The post-upsert agentRowID equality check below is DEFENSE IN DEPTH, not
// the primary guard -- the composite index already makes a foreign row
// unreachable as a conflict target, so this should be unreachable. It is
// kept anyway: an assertion a future refactor could see fail loudly beats
// one that can only be reasoned about from the index definition.
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
		OnConflictColumns(agentcronfire.FieldAgentRowID, agentcronfire.FieldDedupKey).
		UpdateUpdatedAt().
		SetState(agentcronfire.StateArmed).
		SetAttempts(0).
		SetLastError("").
		ID(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent cron: provision job %q: %w", in.JobID, err)
	}

	row, err := s.client.AgentCronFire.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agent cron: reload fire %d after provision: %w", id, err)
	}
	if row.AgentRowID != agentRowID {
		// Unreachable given the composite index above -- see this
		// function's doc comment. Fails loudly rather than silently
		// returning another agent's fire.
		return nil, fmt.Errorf("agent cron: provision job %q resolved to agent_row_id %d, want %d", in.JobID, row.AgentRowID, agentRowID)
	}

	// Ruling T5-1: arm this row into the CURRENT process's timing wheel right
	// now, rather than leaving it to the next restart's RehydrateOnBoot. Runs
	// AFTER the upsert has committed -- an arm racing ahead of the row it
	// names existing would have nothing to load when its timer fires -- and
	// unconditionally on every Provision call, fresh arm or idempotent
	// re-arm alike: fireWheelKey is keyed on row.ID, which the composite
	// (agent_row_id, dedup_key) upsert above resolves to the SAME id both
	// times (ruling T4-1), so a re-arm MOVES this process's existing timer
	// to the (possibly unchanged) fire_at rather than adding a second one --
	// see ArmNow's doc comment.
	s.armer.ArmNow(ctx, row.ID, row.FireAt)

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
