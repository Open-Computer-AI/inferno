package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// agentRegistrySeq gives every seeded agent a distinct public_id across a
// test's seed calls -- public_id is globally unique (ent/schema/agent.go),
// so two agents sharing a name across two different users still need
// different public_ids.
var agentRegistrySeq int

// agentRegistryFixture holds the real ent client AgentRegistryService reads
// and writes -- unlike BillingContractService's narrow-interface fakes,
// Register/ListForUser talk to dbent.Agent directly (there is no repository
// layer for agents yet), so the fixture needs an actual database, not a fake.
// now is a plain field, not a func, so a test can reassign it directly
// (fx.now = time.Date(...)) the same way billingContractFixture.now works;
// AgentRegistryService.now closes over &fx via fx's pointer identity.
type agentRegistryFixture struct {
	client *dbent.Client
	now    time.Time
}

// newAgentRegistryTestClient opens an isolated in-memory sqlite database per
// test, modeled on oauth_backing_key_test.go's newBackingKeyTestClient:
// MaxOpenConns(1) serialises sqlite writes so the unique index on public_id
// is enforced deterministically rather than racing.
func newAgentRegistryTestClient(t *testing.T) *dbent.Client {
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

func newAgentRegistryFixture(t *testing.T) (*AgentRegistryService, *agentRegistryFixture) {
	t.Helper()

	fx := &agentRegistryFixture{
		client: newAgentRegistryTestClient(t),
		now:    time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
	}
	svc := NewAgentRegistryService(fx.client)
	svc.now = func() time.Time { return fx.now }
	return svc, fx
}

// seedAgent creates an agent row seen "now" (irrelevant to status -- these
// tests only care about isolation), with a public_id unique across the
// whole fixture.
func (fx *agentRegistryFixture) seedAgent(userID, orgID int64, name string) *dbent.Agent {
	return fx.seedAgentSeenAt(userID, orgID, name, fx.now)
}

// seedAgentSeenAt creates an agent row with an explicit last_seen_at. A zero
// time.Time means "never connected" -- last_seen_at is left nil, exactly
// what an agent that has never heartbeated looks like in the database.
func (fx *agentRegistryFixture) seedAgentSeenAt(userID, orgID int64, name string, seenAt time.Time) *dbent.Agent {
	agentRegistrySeq++
	create := fx.client.Agent.Create().
		SetPublicID(fmt.Sprintf("agent:test:%d", agentRegistrySeq)).
		SetUserID(userID).
		SetOrgID(orgID).
		SetName(name)
	if !seenAt.IsZero() {
		create = create.SetLastSeenAt(seenAt)
	}
	row, err := create.Save(context.Background())
	if err != nil {
		panic(err) // fixture helper, not the test body -- a seed failure is a fixture bug
	}
	return row
}

// ---------------------------------------------------------------------------

// TestListForUserReturnsOnlyTheCallersAgents is the isolation test -- the
// important one. A second user's agent is genuinely present in the database,
// so a ListForUser with no WHERE user_id filter at all would still pass this
// vacuously if the fixture only ever seeded one user's data. It does not:
// see fx.seedAgent(8, 2, ...) below.
func TestListForUserReturnsOnlyTheCallersAgents(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.seedAgent(7, 1, "ours")
	fx.seedAgent(8, 2, "someone else's") // a SECOND real user, same database

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ours", got[0].Name)
}

// TestListForUserExcludesAnotherUsersAgentInTheSameOrg isolates the user_id
// predicate specifically (ruling T2-1): both agents share org 1, so this can
// only pass if user_id is actually enforced. Combined with
// TestListForUserExcludesTheCallersOwnAgentInAnotherOrg below, dropping
// EITHER of ListForUser's two predicates fails exactly one of these two
// tests -- see the mutation-proof section of task-2-report.md.
func TestListForUserExcludesAnotherUsersAgentInTheSameOrg(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.seedAgent(7, 1, "ours")
	fx.seedAgent(8, 1, "someone else's, same org")

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ours", got[0].Name)
}

// TestListForUserExcludesTheCallersOwnAgentInAnotherOrg isolates the org_id
// predicate specifically (ruling T2-1): both agents belong to user 7, so
// this can only pass if org_id is actually enforced. A multi-org user who
// has picked org A must never see org B's agents just because both orgs
// belong to them -- Task 3's org picker (apps/desktop/electron/main.ts:7849)
// is built directly on this call.
func TestListForUserExcludesTheCallersOwnAgentInAnotherOrg(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.seedAgent(7, 1, "org 1 agent")
	fx.seedAgent(7, 2, "org 2 agent")

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "org 1 agent", got[0].Name)
}

// TestStatusIsHumanReadableProseDerivedFromHeartbeat pins the exact three
// display strings apps/desktop/src/i18n/en.ts:812 renders VERBATIM into the
// UI (cloudStatusLabel: status => "Status: ${status}") -- status is prose,
// not an enum token.
func TestStatusIsHumanReadableProseDerivedFromHeartbeat(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.now = time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	fx.seedAgentSeenAt(7, 1, "fresh", fx.now.Add(-30*time.Second))
	fx.seedAgentSeenAt(7, 1, "stale", fx.now.Add(-3*time.Hour))
	fx.seedAgentSeenAt(7, 1, "never", time.Time{})

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	byName := map[string]AgentView{}
	for _, a := range got {
		byName[a.Name] = a
	}
	require.Equal(t, "online", byName["fresh"].Status)
	require.Equal(t, "last seen 3h ago", byName["stale"].Status)
	require.Equal(t, "never connected", byName["never"].Status)
}

// TestListForUserExcludesRevokedAgents: revoked_at IS NOT NULL rows must
// never appear in the caller's list.
func TestListForUserExcludesRevokedAgents(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	live := fx.seedAgent(7, 1, "live")
	revoked := fx.seedAgent(7, 1, "revoked")
	_, err := fx.client.Agent.UpdateOne(revoked).SetRevokedAt(fx.now).Save(context.Background())
	require.NoError(t, err)

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, live.Name, got[0].Name)
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

// TestRegisterCreatesANewAgent covers the first-boot path: no existing row,
// Register creates one and stamps last_seen_at with the injected clock.
func TestRegisterCreatesANewAgent(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)

	got, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID:     "agent:abc",
		Name:         "My Agent",
		DashboardURL: "https://dash.example.com/abc",
	})
	require.NoError(t, err)
	require.Equal(t, "agent:abc", got.ID)
	require.Equal(t, "My Agent", got.Name)
	require.Equal(t, "https://dash.example.com/abc", got.DashboardURL)
	require.Equal(t, "online", got.Status)

	row, err := fx.client.Agent.Query().Only(context.Background())
	require.NoError(t, err)
	require.NotNil(t, row.LastSeenAt)
	require.True(t, row.LastSeenAt.Equal(fx.now))
}

// TestRegisterRejectsHijackingAnotherUsersPublicID is ruling T2-2's proof.
// public_id IS the agent's OAuth client_id and this credential model treats
// client_ids as PUBLIC by design, so ANY authenticated user could learn
// another agent's public_id. Without an ownership check, Register's
// OnConflictColumns(public_id)+UpdateNewValues() would silently overwrite
// user_id/org_id on conflict -- user 9 "registering" user 7's public_id
// would flip the row to user 9 and the agent would vanish from user 7's
// ListForUser.
func TestRegisterRejectsHijackingAnotherUsersPublicID(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)

	_, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID: "agent:shared", Name: "User 7's Agent",
	})
	require.NoError(t, err)

	got, err := svc.Register(context.Background(), 9, 2, RegisterAgentInput{
		PublicID: "agent:shared", Name: "User 9's Takeover Attempt",
	})
	require.Error(t, err, "registering a public_id already owned by a different user must fail")
	require.Nil(t, got)

	row, err := fx.client.Agent.Query().Where(agent.PublicIDEQ("agent:shared")).Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(7), row.UserID, "the row must still belong to its original owner")
	require.Equal(t, "User 7's Agent", row.Name, "the failed hijack attempt must not have mutated the row at all")

	stillOurs, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, stillOurs, 1)
	require.Equal(t, "User 7's Agent", stillOurs[0].Name, "user 7 must still see their own agent")

	hijackerView, err := svc.ListForUser(context.Background(), 9, 2)
	require.NoError(t, err)
	require.Empty(t, hijackerView, "user 9 must not see an agent they failed to register")
}

// TestRegisterIsAnUpsertKeyedOnPublicID: a rebooting agent heartbeats
// instead of duplicating itself -- registering the same public_id twice
// must leave exactly one row, with last_seen_at refreshed to the second
// call's clock value.
func TestRegisterIsAnUpsertKeyedOnPublicID(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)

	_, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID: "agent:reboot", Name: "Reboot Agent",
	})
	require.NoError(t, err)

	fx.now = fx.now.Add(time.Hour)
	got, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID: "agent:reboot", Name: "Reboot Agent Renamed",
	})
	require.NoError(t, err)
	require.Equal(t, "Reboot Agent Renamed", got.Name)

	count, err := fx.client.Agent.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count, "a re-registered public_id must not duplicate the row")

	row, err := fx.client.Agent.Query().Only(context.Background())
	require.NoError(t, err)
	require.NotNil(t, row.LastSeenAt)
	require.True(t, row.LastSeenAt.Equal(fx.now), "last_seen_at must be refreshed on every Register call")
}

// TestRegisterRejectsABlankPublicID: a blank public_id is a 400, never a
// generated placeholder (controller ruling P-1).
func TestRegisterRejectsABlankPublicID(t *testing.T) {
	svc, _ := newAgentRegistryFixture(t)

	got, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID: "",
		Name:     "No Public ID",
	})
	require.Error(t, err)
	require.Nil(t, got)
}

// ---------------------------------------------------------------------------
// ResolveOwnedAgentRowID
// ---------------------------------------------------------------------------

// TestResolveOwnedAgentRowIDReturnsTheRowIDForTheCallersOwnAgent is the
// happy path Task 4's /api/agent-cron/* handlers depend on: the caller's
// verified OAuth aud claim (their agent's public_id) resolves to that
// agent's row id.
func TestResolveOwnedAgentRowIDReturnsTheRowIDForTheCallersOwnAgent(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	_, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{PublicID: "agent:mine"})
	require.NoError(t, err)

	row, err := fx.client.Agent.Query().Where(agent.PublicIDEQ("agent:mine")).Only(context.Background())
	require.NoError(t, err)

	id, err := svc.ResolveOwnedAgentRowID(context.Background(), 7, "agent:mine")
	require.NoError(t, err)
	require.Equal(t, row.ID, id)
}

// TestResolveOwnedAgentRowIDRejectsAnAgentTheCallerDoesNotOwn: an agent id
// the caller does not own must not be addressable (task-4-brief.md) --
// user 9 must not be able to arm/cancel/list fires under user 7's agent
// just by presenting a token whose aud names it.
func TestResolveOwnedAgentRowIDRejectsAnAgentTheCallerDoesNotOwn(t *testing.T) {
	svc, _ := newAgentRegistryFixture(t)
	_, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{PublicID: "agent:not-yours"})
	require.NoError(t, err)

	_, err = svc.ResolveOwnedAgentRowID(context.Background(), 9, "agent:not-yours")
	require.Error(t, err)
}

// TestResolveOwnedAgentRowIDRejectsAnUnknownPublicID: a public_id that
// belongs to no agent at all must 404, not silently resolve to row 0.
func TestResolveOwnedAgentRowIDRejectsAnUnknownPublicID(t *testing.T) {
	svc, _ := newAgentRegistryFixture(t)

	_, err := svc.ResolveOwnedAgentRowID(context.Background(), 7, "agent:no-such-agent")
	require.Error(t, err)
}

// TestRegisterNameFallsBackToPublicIDWhenBlank mirrors the desktop's own
// `name: typeof a.name === 'string' ? a.name : a.id`
// (apps/desktop/electron/main.ts:7924).
func TestRegisterNameFallsBackToPublicIDWhenBlank(t *testing.T) {
	svc, _ := newAgentRegistryFixture(t)

	got, err := svc.Register(context.Background(), 7, 1, RegisterAgentInput{
		PublicID: "agent:no-name",
		Name:     "",
	})
	require.NoError(t, err)
	require.Equal(t, "agent:no-name", got.Name)
}
