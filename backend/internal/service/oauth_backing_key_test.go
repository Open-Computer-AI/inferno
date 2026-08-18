package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const backingKeyTestGroupName = "oauth-agents"

// requireNoBackingRow fails if a row came back, reporting only its id.
// require.Nil would dump the whole *dbent.APIKey, api_keys.key included, and a
// backing key's secret must never reach any output -- a test log included.
func requireNoBackingRow(t *testing.T, row *dbent.APIKey, msg string) {
	t.Helper()
	if row != nil {
		t.Fatalf("%s: got backing row id %d", msg, row.ID)
	}
}

// backingKeyUniqueIndexDDL mirrors migrations/909_api_key_oauth_client_id.sql.
// Ent's schema does not declare this index (it is a hand-written migration), so
// enttest's auto-migration would not create it and every "lost the insert race"
// assertion below would pass vacuously against a table with no constraint. The
// index is what makes the race a race, so the test harness installs it.
const backingKeyUniqueIndexDDL = `CREATE UNIQUE INDEX api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL`

func backingKeyTestConfig(groupName string) *config.Config {
	cfg := &config.Config{}
	cfg.Default.APIKeyPrefix = "sk-"
	cfg.OAuthBackingKey.GroupName = groupName
	return cfg
}

// newBackingKeyTestClient opens a per-test in-memory SQLite database, migrates
// the ent schema into it, and then applies migration 909's partial unique index.
//
// MaxOpenConns(1) is deliberate: it serialises SQLite writes so a concurrent
// test never fails on SQLITE_BUSY, while still letting two goroutines both pass
// the lookup and then both attempt the INSERT — which is exactly the race the
// unique index has to arbitrate.
func newBackingKeyTestClient(t *testing.T) *dbent.Client {
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

	_, err = db.Exec(backingKeyUniqueIndexDDL)
	require.NoError(t, err, "apply migration 909's partial unique index")

	return client
}

func seedBackingKeyGroup(t *testing.T, client *dbent.Client, name string) *dbent.Group {
	t.Helper()
	grp, err := client.Group.Create().
		SetName(name).
		SetDescription("group every OAuth backing key binds to").
		SetPlatform(PlatformAnthropic).
		SetStatus(domain.StatusActive).
		SetSubscriptionType(SubscriptionTypeStandard).
		SetRateMultiplier(1.0).
		Save(context.Background())
	require.NoError(t, err, "seed policy group")
	return grp
}

// seedBackingKeyUser creates the owning user. Ent gives api_keys.user_id a real
// FK, and the harness turns foreign keys on, so the user has to exist before a
// backing row can reference it. Ent's int64 ids are auto-assigned (there is no
// generated SetID on UserCreate), so the caller uses the returned id rather than
// a literal.
func seedBackingKeyUser(t *testing.T, client *dbent.Client) int64 {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(fmt.Sprintf("backing-key-%d@example.com", client.User.Query().CountX(context.Background()))).
		SetPasswordHash("hash").
		SetUsername(fmt.Sprintf("backing-key-user-%d", client.User.Query().CountX(context.Background()))).
		Save(context.Background())
	require.NoError(t, err, "seed user")
	return u.ID
}

// newBackingKeyTestService is the healthy case: a group policy that is both
// configured and present in the database.
func newBackingKeyTestService(t *testing.T) (*OAuthBackingKeyService, *dbent.Client) {
	t.Helper()
	client := newBackingKeyTestClient(t)
	seedBackingKeyGroup(t, client, backingKeyTestGroupName)
	return NewOAuthBackingKeyService(client, backingKeyTestConfig(backingKeyTestGroupName)), client
}

// TestResolveCreatesOnFirstUseAndReusesAfter is the brief's Step 1 test.
//
// Two deviations from the brief's literal text, both forced by the generated
// code rather than chosen:
//   - ent puts eager-loaded relations on `.Edges`, so the assertions read
//     `first.Edges.User` / `first.Edges.Group`, not `first.User` / `first.Group`.
//     `dbent.APIKey` has no `User` or `Group` field; the brief's version does not
//     compile.
//   - the brief hardcodes user id 7. Ent assigns api_keys.user_id's referent
//     automatically and generates no SetID for it, so the test uses the seeded
//     user's real id.
func TestResolveCreatesOnFirstUseAndReusesAfter(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	first, err := svc.Resolve(ctx, userID, "agent:aaa")
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.NotNil(t, first.Edges.User)
	require.NotNil(t, first.Edges.Group, "the gateway pipeline reads apiKey.Group for routing")

	second, err := svc.Resolve(ctx, userID, "agent:aaa")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "the same agent must reuse its row, not accumulate rows")

	other, err := svc.Resolve(ctx, userID, "agent:bbb")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID, "per-agent attribution requires one row per client_id")

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, total)
}

// TestResolveSeparatesUsersWithTheSameClientID: the identity is the PAIR. Two
// users running the same agent build must not share one quota ledger.
func TestResolveSeparatesUsersWithTheSameClientID(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	alice := seedBackingKeyUser(t, cli)
	bob := seedBackingKeyUser(t, cli)

	aliceRow, err := svc.Resolve(ctx, alice, "agent:shared")
	require.NoError(t, err)
	bobRow, err := svc.Resolve(ctx, bob, "agent:shared")
	require.NoError(t, err)

	require.NotEqual(t, aliceRow.ID, bobRow.ID)
	require.Equal(t, alice, aliceRow.UserID)
	require.Equal(t, bob, bobRow.UserID)
}

// TestResolveRecoversTheWinnerAfterLosingTheInsertRace drives the recovery path
// directly and deterministically.
//
// createOrAdoptWinner is what Resolve calls once its lookup has missed. Calling
// it when the row already exists reproduces exactly the state a concurrent
// caller is in: its lookup missed, the other caller's INSERT has since landed,
// and its own INSERT is about to be rejected by the (user_id, oauth_client_id)
// unique index. The contract is that it re-reads and returns the winner rather
// than failing the request.
//
// Delete the constraint-error branch and this fails with the raw unique
// violation; delete the index from the harness and it fails on a second row id.
func TestResolveRecoversTheWinnerAfterLosingTheInsertRace(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	winner, err := svc.Resolve(ctx, userID, "agent:racer")
	require.NoError(t, err)

	grp := seedBackingKeyGroupID(t, cli)
	loser, err := svc.createOrAdoptWinner(ctx, userID, "agent:racer", grp)
	require.NoError(t, err, "losing the insert race must not fail the request")
	require.Equal(t, winner.ID, loser.ID, "the loser must adopt the winner's row")
	require.NotNil(t, loser.Edges.Group, "the adopted row still has to carry a group")
	require.NotNil(t, loser.Edges.User)

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, total, "the losing insert must leave no second row behind")
}

func seedBackingKeyGroupID(t *testing.T, cli *dbent.Client) int64 {
	t.Helper()
	grp, err := cli.Group.Query().Only(context.Background())
	require.NoError(t, err)
	return grp.ID
}

// TestResolveConcurrentFirstUseCreatesExactlyOneRow is the brief's Step 4.
// Run with -race -count=5.
func TestResolveConcurrentFirstUseCreatesExactlyOneRow(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	const callers = 2
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
	)
	start.Add(1)
	ids := make([]int64, 0, callers)
	errs := make([]error, 0, callers)

	for i := 0; i < callers; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			row, err := svc.Resolve(ctx, userID, "agent:concurrent")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids = append(ids, row.ID)
		}()
	}
	start.Done()
	done.Wait()

	require.Empty(t, errs, "an agent's first two concurrent inference calls must not produce a 500")
	require.Len(t, ids, callers)
	require.Equal(t, ids[0], ids[1], "both callers must land on the same backing row")

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, total, "exactly one backing row may exist for (user, client)")
}

// TestResolveWithoutAConfiguredGroupIsATypedError: no policy configured is an
// operator problem, and it must arrive as ErrNoGroupForOAuthKey so Task 4 can
// answer 403 with something readable — never as a row whose nil Group panics in
// routing, and never as a silently-picked arbitrary group.
func TestResolveWithoutAConfiguredGroupIsATypedError(t *testing.T) {
	cli := newBackingKeyTestClient(t)
	seedBackingKeyGroup(t, cli, backingKeyTestGroupName)
	svc := NewOAuthBackingKeyService(cli, backingKeyTestConfig(""))
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	row, err := svc.Resolve(ctx, userID, "agent:ungrouped")
	// Assert on the id rather than the row: testify prints the whole struct on
	// failure, and api_keys.key is a credential that must not reach a log.
	requireNoBackingRow(t, row, "no row may be returned when no group resolves")
	require.ErrorIs(t, err, ErrNoGroupForOAuthKey)

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, total, "a failed group policy must not leave a half-built backing row")
}

// TestResolveWithAMissingConfiguredGroupIsATypedError: configured but absent (or
// soft-deleted, or inactive) is the same operator-visible failure.
func TestResolveWithAMissingConfiguredGroupIsATypedError(t *testing.T) {
	cli := newBackingKeyTestClient(t)
	seedBackingKeyGroup(t, cli, backingKeyTestGroupName)
	svc := NewOAuthBackingKeyService(cli, backingKeyTestConfig("no-such-group"))
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	row, err := svc.Resolve(ctx, userID, "agent:ungrouped")
	requireNoBackingRow(t, row, "no row may be returned when the configured group is absent")
	require.ErrorIs(t, err, ErrNoGroupForOAuthKey)
	require.Contains(t, err.Error(), "no-such-group", "the operator has to be able to see which name failed")
}

// TestResolveRebindsARowThatLostItsGroup: an existing backing row whose group
// was cleared (its group was deleted, say) must not come back with a nil Group.
// Routing dereferences it.
func TestResolveRebindsARowThatLostItsGroup(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	first, err := svc.Resolve(ctx, userID, "agent:orphan")
	require.NoError(t, err)
	require.NoError(t, cli.APIKey.UpdateOneID(first.ID).ClearGroupID().Exec(ctx))

	again, err := svc.Resolve(ctx, userID, "agent:orphan")
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID, "rebinding must not create a second row")
	require.NotNil(t, again.Edges.Group, "Resolve must never hand back a row with a nil Group")

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, total)
}

// TestResolveMarksTheRowAsAgentBackedAndActive pins the row shape Task 4 and
// Task 5 depend on: oauth_client_id set (Task 5 filters listings on it), active
// status, and a name that says what the row is.
func TestResolveMarksTheRowAsAgentBackedAndActive(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	row, err := svc.Resolve(ctx, userID, "agent:marked")
	require.NoError(t, err)
	require.NotNil(t, row.OauthClientID)
	require.Equal(t, "agent:marked", *row.OauthClientID)
	require.Equal(t, domain.StatusActive, row.Status)
	require.NotEmpty(t, row.Name)
	require.LessOrEqual(t, len(row.Name), 100, "api_keys.name is MaxLen(100)")

	stored, err := cli.APIKey.Query().Where(apikey.IDEQ(row.ID)).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, stored.OauthClientID)
	require.Equal(t, "agent:marked", *stored.OauthClientID)
}

// TestResolveUsesTheOrdinaryKeyGenerator: the secret must come from the same
// generator ordinary keys use, so a hypothetical leak is no worse than an
// ordinary key leak. GenerateAPIKeySecret is that generator — APIKeyService.
// GenerateKey now delegates to it — and its output is the configured prefix
// followed by 32 random bytes as hex.
func TestResolveUsesTheOrdinaryKeyGenerator(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	a, err := svc.Resolve(ctx, userID, "agent:secret-a")
	require.NoError(t, err)
	b, err := svc.Resolve(ctx, userID, "agent:secret-b")
	require.NoError(t, err)

	for _, row := range []*dbent.APIKey{a, b} {
		require.True(t, strings.HasPrefix(row.Key, "sk-"), "got %q", row.Key)
		require.Len(t, row.Key, len("sk-")+64, "32 random bytes, hex-encoded")
	}
	require.NotEqual(t, a.Key, b.Key, "every backing key must be independently random")
}

// TestGenerateKeyDelegatesToGenerateAPIKeySecret keeps "the same generator" from
// silently becoming two generators that merely look alike: it asserts the
// ordinary-key path produces the same shape from the same configured prefix.
func TestGenerateKeyDelegatesToGenerateAPIKeySecret(t *testing.T) {
	svc := &APIKeyService{cfg: backingKeyTestConfig(backingKeyTestGroupName)}
	svc.cfg.Default.APIKeyPrefix = "ok-"

	ordinary, err := svc.GenerateKey()
	require.NoError(t, err)
	backing, err := GenerateAPIKeySecret("ok-")
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(ordinary, "ok-"))
	require.True(t, strings.HasPrefix(backing, "ok-"))
	require.Equal(t, len(ordinary), len(backing))
	require.NotEqual(t, ordinary, backing)
}

// TestScrubBackingKeySecretRedactsTheCredential: a Postgres unique violation
// puts the offending value in its DETAIL line, so an error from the INSERT can
// carry the freshly generated secret. Nothing may transmit or log it, error
// strings included.
func TestScrubBackingKeySecretRedactsTheCredential(t *testing.T) {
	secret := "sk-deadbeefdeadbeef"
	raw := fmt.Errorf(`insert api_keys: ERROR: duplicate key value violates unique constraint "api_keys_key_key" (DETAIL: Key (key)=(%s) already exists.)`, secret)

	scrubbed := scrubBackingKeySecret(raw, secret)
	require.NotContains(t, scrubbed.Error(), secret, "the credential must not survive into an error string")
	require.Contains(t, scrubbed.Error(), "api_keys_key_key", "the diagnosable part must survive")

	require.Equal(t, raw, scrubBackingKeySecret(raw, ""), "an empty secret is a no-op")
	require.Nil(t, scrubBackingKeySecret(nil, secret))
}

// TestResolveSurfacesInfrastructureFaultsAsPlainErrors: a dead database is a
// 500, not ErrNoGroupForOAuthKey (which Task 4 turns into a 403). Conflating
// them would tell an operator their group policy is broken when their database
// is down.
func TestResolveSurfacesInfrastructureFaultsAsPlainErrors(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)
	require.NoError(t, cli.Close())

	row, err := svc.Resolve(ctx, userID, "agent:down")
	requireNoBackingRow(t, row, "an infrastructure fault returns no row")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNoGroupForOAuthKey), "an infrastructure fault must not read as an auth/config failure, got: %v", err)
}

// TestResolveRejectsAnEmptyClientID: an empty client_id would collapse every
// agent of a user onto one row (and NULL, which the partial index ignores).
func TestResolveRejectsAnEmptyClientID(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	row, err := svc.Resolve(ctx, userID, "   ")
	requireNoBackingRow(t, row, "an empty client_id returns no row")
	require.Error(t, err)

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, total)
}
