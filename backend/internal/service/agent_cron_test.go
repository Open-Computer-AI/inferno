package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
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

// fakeCronArmer stands in for AgentCronFirer.ArmNow in AgentCronService
// tests (ruling T5-1), modeling exactly the invariant fireWheelKey buys in
// production: keyed by fireID, so a re-arm of the SAME row overwrites its
// own entry rather than accumulating a second one -- the identical "one
// timer per row" property go-zero's real TimingWheel.SetTimer enforces by
// key (agent_cron_firer.go's fireWheelKey doc comment).
type fakeCronArmer struct {
	mu    sync.Mutex
	calls map[int64]time.Time // fireID -> most recently armed fire_at
}

func newFakeCronArmer() *fakeCronArmer {
	return &fakeCronArmer{calls: make(map[int64]time.Time)}
}

func (a *fakeCronArmer) ArmNow(_ context.Context, fireID int64, fireAt time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[fireID] = fireAt
}

// count returns how many DISTINCT rows have been armed -- not how many times
// ArmNow was called, which is the whole point: a re-arm of an already-armed
// row must not grow this number.
func (a *fakeCronArmer) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

// armed reports whether fireID was armed, and at what fire_at. Distinct from
// count: ruling W-5's re-arm test must prove a timer was set for THAT row
// after the row went terminal, which a total is structurally unable to say.
func (a *fakeCronArmer) armed(fireID int64) (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.calls[fireID]
	return at, ok
}

// forget drops every recorded arm, so a test can prove the NEXT Provision
// armed something rather than reading an arm the setup already made.
func (a *fakeCronArmer) forget() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = make(map[int64]time.Time)
}

// agentCronSeq gives every seeded fire a distinct dedup_key across a test's
// seed calls -- dedup_key is UNIQUE (ent/schema/agent_cron_fire.go), so two
// fires sharing a job_id across two different agents still need distinct
// keys unless a test deliberately wants a collision.
var agentCronSeq int

// agentCronFixture holds the real ent client AgentCronService reads and
// writes -- like AgentRegistryService, Provision/Cancel/ListArmed talk to
// dbent.AgentCronFire directly, so the fixture needs an actual database --
// plus the fake armer standing in for AgentCronFirer (ruling T5-1).
type agentCronFixture struct {
	client *dbent.Client
	armer  *fakeCronArmer
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

	fx := &agentCronFixture{client: newAgentCronTestClient(t), armer: newFakeCronArmer()}
	svc := NewAgentCronService(fx.client, fx.armer)
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

// onlyFire returns the fixture's single fire row, failing the test if there
// is not exactly one -- the ruling W-5 tests provision one row and then need
// its id to drive it terminal.
func (fx *agentCronFixture) onlyFire(t *testing.T) *dbent.AgentCronFire {
	t.Helper()
	rows, err := fx.client.AgentCronFire.Query().All(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1, "expected exactly one fire row")
	return rows[0]
}

func (fx *agentCronFixture) reload(t *testing.T, id int64) *dbent.AgentCronFire {
	t.Helper()
	row, err := fx.client.AgentCronFire.Get(context.Background(), id)
	require.NoError(t, err, "reload fire %d", id)
	return row
}

// driveTerminal puts a row into a terminal state with the failure bookkeeping
// a real fire cycle would have left behind, so a re-arm has something
// observable to reset.
func (fx *agentCronFixture) driveTerminal(t *testing.T, id int64, state agentcronfire.State) {
	t.Helper()
	_, err := fx.client.AgentCronFire.UpdateOneID(id).
		SetState(state).
		SetAttempts(3).
		SetLastError("callback answered 503: not ready").
		Save(context.Background())
	require.NoError(t, err, "drive fire %d to %s", id, state)
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

// TestProvisionArmsTheNewRowIntoTheTimingWheel is ruling T5-1: a fire armed
// while the server is RUNNING must not wait for the next boot's
// RehydrateOnBoot to get a timer -- Provision itself must arm it, via
// AgentCronFirer.ArmNow, the moment the upsert commits.
func TestProvisionArmsTheNewRowIntoTheTimingWheel(t *testing.T) {
	svc, fx := newAgentCronFixture(t)

	_, err := svc.Provision(context.Background(), 1, ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	})
	require.NoError(t, err)

	require.Equal(t, 1, fx.armer.count(), "Provision must arm exactly one row into the current process's timing wheel")
}

// TestProvisionReArmDoesNotLeaveTwoTimersForOneRow is ruling T5-1's other
// half: Provision's own idempotent re-arm (same agent_row_id + dedup_key,
// ruling T4-1) resolves to the SAME row both times, so arming it twice must
// still leave exactly one timer for that row, never two.
func TestProvisionReArmDoesNotLeaveTwoTimersForOneRow(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	_, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	_, err = svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)

	require.Equal(t, 1, fx.armer.count(), "a re-arm of the same row must not leave two timers for it")
}

// TestProvisionOnAFiredRowReArmsIt is ruling W-5, the direction that was
// broken: an explicit Provision naming a dedup_key whose row already FIRED
// must re-arm that row -- state back to armed, attempts and last_error
// cleared, a timer actually set -- while still answering with the SAME
// schedule_id (ruling T4-1).
//
// Before this, the conflict clause touched updated_at only: the row stayed
// `fired`, FireNow returns silently for any non-armed row, ListArmed hides
// it, and the call answered 200 having armed nothing, forever. The agent's
// reconcile replays the identical dedup_key in two real windows (it crashes
// after a fire but before persisting next_run_at; or its own _list_armed
// errors and it re-arms everything), so this is a recovery path, not a
// theoretical edge -- see Provision's doc comment.
//
// MUTATION that kills this test: drop the SetState/SetAttempts/SetLastError
// from Provision's conflict clause, leaving UpdateUpdatedAt() alone.
func TestProvisionOnAFiredRowReArmsIt(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	first, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	row := fx.onlyFire(t)
	fx.driveTerminal(t, row.ID, agentcronfire.StateFired)
	// Forget the arm the first Provision made, so what follows can only be
	// satisfied by a NEW arm.
	fx.armer.forget()

	second, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err, "re-provisioning a fired row is a recovery request, never an error")

	require.Equal(t, first.ScheduleID, second.ScheduleID, "same fire, same schedule_id (ruling T4-1)")
	require.Equal(t, 1, fx.countFires(), "a re-arm must never create a second row")

	reloaded := fx.reload(t, row.ID)
	require.Equal(t, agentcronfire.StateArmed, reloaded.State, "a re-provisioned fired row must be ARMED again")
	require.Equal(t, 0, reloaded.Attempts, "a re-arm starts a fresh retry budget")
	require.Equal(t, "", reloaded.LastError, "a re-arm must not inherit the previous cycle's failure")

	armedAt, ok := fx.armer.armed(row.ID)
	require.True(t, ok, "a re-armed row must get a timer in THIS process, not wait for the next boot")
	require.Equal(t, reloaded.FireAt.UTC(), armedAt.UTC())

	listed, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, listed, 1, "the agent's reconcile must be able to see the fire again")
}

// TestProvisionOnACancelledRowReArmsIt is the same ruling W-5 direction for
// the other terminal state: the agent cancels optimistically (Cancel is a
// no-op on an unknown job), so re-arming a job it cancelled and then wanted
// back must work identically to re-arming a fired one.
func TestProvisionOnACancelledRowReArmsIt(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	_, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	row := fx.onlyFire(t)
	require.NoError(t, svc.Cancel(context.Background(), 1, "job-1"))
	require.Equal(t, agentcronfire.StateCancelled, fx.reload(t, row.ID).State)
	fx.armer.forget()

	_, err = svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)

	require.Equal(t, agentcronfire.StateArmed, fx.reload(t, row.ID).State)
	_, ok := fx.armer.armed(row.ID)
	require.True(t, ok, "a re-armed cancelled row must get a timer in THIS process")
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
