package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agentcronfire"
	"github.com/Wei-Shaw/sub2api/ent/enttest"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// agentCronSeq gives every seeded fire a distinct dedup_key across a test's
// seed calls -- dedup_key is UNIQUE (ent/schema/agent_cron_fire.go), so two
// fires sharing a job_id across two different agents still need distinct
// keys unless a test deliberately wants a collision.
var agentCronSeq int

// agentCronFixture holds the real ent client AgentCronService reads and
// writes -- like AgentRegistryService, Provision/Cancel/ListArmed talk to
// dbent.AgentCronFire directly, so the fixture needs an actual database.
type agentCronFixture struct {
	client *dbent.Client
}

// newAgentCronTestClient opens an isolated in-memory sqlite database per
// test, modeled on agent_registry_test.go's newAgentRegistryTestClient:
// MaxOpenConns(1) serialises sqlite writes so the unique index on
// dedup_key is enforced deterministically rather than racing.
func newAgentCronTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	dbName := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err, "open sqlite")
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err, "enable foreign keys")

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func newAgentCronFixture(t *testing.T) (*AgentCronService, *agentCronFixture) {
	t.Helper()

	fx := &agentCronFixture{client: newAgentCronTestClient(t)}
	svc := NewAgentCronService(fx.client)
	return svc, fx
}

// countFires returns the total number of agent_cron_fires rows across every
// agent -- the idempotency test's proof that a duplicate arm never
// duplicates the row.
func (fx *agentCronFixture) countFires() int {
	n, err := fx.client.AgentCronFire.Query().Count(context.Background())
	if err != nil {
		panic(err) // fixture helper, not the test body
	}
	return n
}

// seedFire creates one ARMED fire directly for agentRowID/jobID, with a
// dedup_key unique across the whole fixture.
func (fx *agentCronFixture) seedFire(agentRowID int64, jobID string) *dbent.AgentCronFire {
	agentCronSeq++
	fireAt, err := time.Parse(time.RFC3339, "2026-09-01T10:00:00Z")
	if err != nil {
		panic(err) // fixture helper, not the test body -- a fixed literal can never fail to parse
	}
	row, err := fx.client.AgentCronFire.Create().
		SetAgentRowID(agentRowID).
		SetJobID(jobID).
		SetFireAt(fireAt).
		SetCallbackURL("https://agent.example/").
		SetDedupKey(fmt.Sprintf("%s:seed:%d", jobID, agentCronSeq)).
		SetScheduleID(fmt.Sprintf("cron_seed_%d", agentCronSeq)).
		Save(context.Background())
	if err != nil {
		panic(err) // fixture helper, not the test body
	}
	return row
}

// ---------------------------------------------------------------------------
// Provision
// ---------------------------------------------------------------------------

// TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire is task-4-brief.md
// Step 1, verbatim. The agent's cold-start reconcile re-arms everything it
// wants, so a duplicate arm is normal traffic, not an error path.
func TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	first, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	second, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err, "a re-arm is normal reconcile traffic, never an error")

	require.Equal(t, first.ScheduleID, second.ScheduleID, "same fire, same schedule_id")
	require.Equal(t, 1, fx.countFires(), "the UNIQUE dedup_key is what enforces this")
}

// TestProvisionRejectsMissingRequiredFields pins the 400s: job_id,
// agent_callback_url and dedup_key are all NotEmpty at the schema
// (ent/schema/agent_cron_fire.go), so an empty one must never reach the
// database.
func TestProvisionRejectsMissingRequiredFields(t *testing.T) {
	svc, _ := newAgentCronFixture(t)
	base := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	missingJobID := base
	missingJobID.JobID = ""
	_, err := svc.Provision(context.Background(), 1, missingJobID)
	require.Error(t, err)

	missingCallback := base
	missingCallback.AgentCallbackURL = ""
	_, err = svc.Provision(context.Background(), 1, missingCallback)
	require.Error(t, err)

	missingDedup := base
	missingDedup.DedupKey = ""
	_, err = svc.Provision(context.Background(), 1, missingDedup)
	require.Error(t, err)
}

// TestProvisionRejectsAnUnparseableFireAt: fire_at is a single ISO 8601
// instant, never a cron expression -- Chronos is not a cron engine.
func TestProvisionRejectsAnUnparseableFireAt(t *testing.T) {
	svc, _ := newAgentCronFixture(t)

	_, err := svc.Provision(context.Background(), 1, ProvisionInput{
		JobID:            "job-1",
		FireAt:           "*/5 * * * *",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:*/5 * * * *",
	})
	require.Error(t, err)
}

// TestProvisionOfADifferentDedupKeyArmsASecondFire: a genuinely new fire
// (different job or different fire_at) must not collide with an earlier
// arm.
func TestProvisionOfADifferentDedupKeyArmsASecondFire(t *testing.T) {
	svc, fx := newAgentCronFixture(t)

	first, err := svc.Provision(context.Background(), 1, ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	})
	require.NoError(t, err)

	second, err := svc.Provision(context.Background(), 1, ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-02T10:00:00Z", // the NEXT re-arm after the first fires
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-02T10:00:00Z",
	})
	require.NoError(t, err)

	require.NotEqual(t, first.ScheduleID, second.ScheduleID)
	require.Equal(t, 2, fx.countFires())
}

// TestProvisionByTwoDifferentAgentsWithTheSameDedupKeyGivesEachItsOwnRow is
// ruling T4-1's proof. dedup_key is "{job_id}:{fire_at}" and job_id lives
// in the CALLING AGENT's own namespace, not a global one -- two different
// agents legitimately arm the identically-named job on the same instant
// (e.g. both run a job literally named "daily-report"). Before T4-1, a
// field-level-unique dedup_key let agent 2's Provision collide into agent
// 1's row: agent 2 got back agent 1's job_id/schedule_id, no row of its
// own, and agent 1's row was silently touched. The composite
// (agent_row_id, dedup_key) index fixes this structurally.
func TestProvisionByTwoDifferentAgentsWithTheSameDedupKeyGivesEachItsOwnRow(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "daily-report",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "daily-report:2026-09-01T10:00:00Z",
	}

	forAgent1, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	forAgent2, err := svc.Provision(context.Background(), 2, in)
	require.NoError(t, err, "a second agent's identical dedup_key must not collide into the first agent's row")

	require.NotEqual(t, forAgent1.ScheduleID, forAgent2.ScheduleID, "each agent must get its OWN schedule_id, not the other's")
	require.Equal(t, 2, fx.countFires(), "each agent must get its OWN row -- the composite index scopes uniqueness per agent")

	agent1Fires, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, agent1Fires, 1)
	require.Equal(t, forAgent1.ScheduleID, agent1Fires[0].ScheduleID, "agent 1's own fire must still carry agent 1's schedule_id")

	agent2Fires, err := svc.ListArmed(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, agent2Fires, 1)
	require.Equal(t, forAgent2.ScheduleID, agent2Fires[0].ScheduleID, "agent 2's own fire must still carry agent 2's schedule_id")
}

// ---------------------------------------------------------------------------
// ListArmed
// ---------------------------------------------------------------------------

// TestListArmedReturnsOnlyThisAgentsFires is task-4-brief.md Step 5,
// verbatim -- the cross-tenant test. A SECOND agent's fire is genuinely
// present in the database, so this can only pass if ListArmed actually
// filters on agent_row_id.
func TestListArmedReturnsOnlyThisAgentsFires(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	fx.seedFire(1, "ours")
	fx.seedFire(2, "another agent's") // a SECOND agent, same database

	got, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ours", got[0].JobID)
}

// TestListArmedExcludesCancelledAndFiredFires: only state=armed rows are
// "currently armed" -- a fired or cancelled row is not something the
// agent's reconcile should treat as still outstanding.
func TestListArmedExcludesCancelledAndFiredFires(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	armed := fx.seedFire(1, "still-armed")
	fired := fx.seedFire(1, "already-fired")
	cancelled := fx.seedFire(1, "already-cancelled")

	_, err := fx.client.AgentCronFire.UpdateOne(fired).SetState(agentcronfire.StateFired).Save(context.Background())
	require.NoError(t, err)
	_, err = fx.client.AgentCronFire.UpdateOne(cancelled).SetState(agentcronfire.StateCancelled).Save(context.Background())
	require.NoError(t, err)

	got, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, armed.JobID, got[0].JobID)
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// TestCancelMarksStateCancelledAndRemovesFromListArmed is task-4-brief.md
// Step 6's main assertion.
func TestCancelMarksStateCancelledAndRemovesFromListArmed(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	row := fx.seedFire(1, "job-1")

	err := svc.Cancel(context.Background(), 1, "job-1")
	require.NoError(t, err)

	reloaded, err := fx.client.AgentCronFire.Get(context.Background(), row.ID)
	require.NoError(t, err)
	require.Equal(t, agentcronfire.StateCancelled, reloaded.State)

	got, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, got, "a cancelled fire must be absent from ListArmed")
}

// TestCancelOnAnUnknownJobIDIsANoOp: the agent cancels optimistically, so an
// unrecognized job_id must return nil, never an error.
func TestCancelOnAnUnknownJobIDIsANoOp(t *testing.T) {
	svc, _ := newAgentCronFixture(t)

	err := svc.Cancel(context.Background(), 1, "no-such-job")
	require.NoError(t, err, "cancelling an unknown job_id is a no-op, not an error")
}

// TestCancelDoesNotAffectAnotherAgentsFireWithTheSameJobID: job_id is not
// globally unique across agents, so Cancel must be scoped to agentRowID --
// otherwise one agent could cancel another agent's fire just by guessing
// its job_id.
func TestCancelDoesNotAffectAnotherAgentsFireWithTheSameJobID(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	other := fx.seedFire(2, "job-1") // a DIFFERENT agent, same job_id

	err := svc.Cancel(context.Background(), 1, "job-1")
	require.NoError(t, err)

	reloaded, err := fx.client.AgentCronFire.Get(context.Background(), other.ID)
	require.NoError(t, err)
	require.Equal(t, agentcronfire.StateArmed, reloaded.State, "agent 1 cancelling job-1 must not touch agent 2's own job-1")
}
