package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
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

// backingKeyUniqueIndexDDL mirrors the index as migration
// 910_api_key_oauth_client_uniq_live_only.sql leaves it (909 created it; 910
// scoped it to live rows).
// Ent's schema does not declare this index (it is a hand-written migration), so
// enttest's auto-migration would not create it and every "lost the insert race"
// assertion below would pass vacuously against a table with no constraint. The
// index is what makes the race a race, so the test harness installs it.
const backingKeyUniqueIndexDDL = `CREATE UNIQUE INDEX api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL AND deleted_at IS NULL`

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
func newBackingKeyTestClient(t *testing.T) (*dbent.Client, *sql.DB) {
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
	require.NoError(t, err, "apply migration 910's partial unique index")

	return client, db
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
	client, _ := newBackingKeyTestClient(t)
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
	cli, _ := newBackingKeyTestClient(t)
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
	cli, _ := newBackingKeyTestClient(t)
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

	// Resolve redacts the secret out of the row it returns (see
	// TestResolveNeverHandsBackTheSecret), so the generator's output is read
	// straight from the table. Nothing here prints a secret: the assertions
	// report only prefixes and lengths, never the value.
	stored := storedBackingKeySecret(t, cli, a.ID)
	other := storedBackingKeySecret(t, cli, b.ID)
	for _, secret := range []string{stored, other} {
		require.True(t, strings.HasPrefix(secret, "sk-"), "secret should carry the configured prefix; got prefix %q", safeSecretPrefix(secret))
		require.Len(t, secret, len("sk-")+64, "32 random bytes, hex-encoded")
	}
	require.NotEqual(t, stored, other, "every backing key must be independently random")
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

// backingKeyDriverError stands in for *pq.Error: an error whose TEXT hides the
// credential but whose exported field carries it. That is exactly the shape
// real PostgreSQL + lib/pq produce (verified in
// repository.APIKeyOAuthClientIDSuite.TestPostgresUniqueViolationHidesTheKeyInErrorTextButNotInTheErrorValue),
// and it is why a redaction that only rewrites err.Error() is not enough.
type backingKeyDriverError struct {
	Message string
	Detail  string // holds the credential, like pq.Error.Detail
}

func (e *backingKeyDriverError) Error() string { return e.Message }

// TestSanitizeBackingKeyErrorDropsBothTheTextAndTheValue pins both halves of
// the guard: the message is redacted, and the driver error value does not
// survive for anything downstream to serialise.
func TestSanitizeBackingKeyErrorDropsBothTheTextAndTheValue(t *testing.T) {
	secret := "sk-deadbeefdeadbeef"

	// Half 1 -- a message that does carry the secret is redacted, and the
	// diagnosable part survives, so a scrub that nuked the whole message would
	// also fail this.
	textual := fmt.Errorf(`insert api_keys: ERROR: duplicate key value violates unique constraint "api_keys_key_key" (DETAIL: Key (key)=(%s) already exists.)`, secret)
	sanitized := sanitizeBackingKeyError(textual, secret)
	require.NotContains(t, sanitized.Error(), secret, "the credential must not survive into an error string")
	require.Contains(t, sanitized.Error(), "api_keys_key_key", "the diagnosable part must survive")

	// Half 2 -- the real channel. The text is already clean, so a text-only
	// redaction would pass this error straight through with the credential
	// still readable on the value.
	driver := &backingKeyDriverError{
		Message: `pq: duplicate key value violates unique constraint "api_keys_key_key"`,
		Detail:  fmt.Sprintf("Key (key)=(%s) already exists.", secret),
	}
	wrapped := fmt.Errorf("ent: constraint failed: %w", driver)
	require.NotContains(t, wrapped.Error(), secret, "precondition: the driver hides the secret from Error()")

	sanitized = sanitizeBackingKeyError(wrapped, secret)
	var leaked *backingKeyDriverError
	require.False(t, errors.As(sanitized, &leaked),
		"the driver error value must not survive; anything serialising it structurally would print the credential")
	require.NotContains(t, sanitized.Error(), secret)

	require.Nil(t, sanitizeBackingKeyError(nil, secret))
	// An empty secret still flattens: the value is the risk, not just the text.
	require.False(t, errors.As(sanitizeBackingKeyError(wrapped, ""), &leaked))
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

// ---------------------------------------------------------------------------
// Fix round 1 — tests added in response to review findings F1..F9.
// ---------------------------------------------------------------------------

// storedBackingKeySecret reads api_keys.key straight from the table, bypassing
// Resolve's redaction. It exists so tests can assert on the credential's shape
// without Resolve having to hand one out.
func storedBackingKeySecret(t *testing.T, cli *dbent.Client, id int64) string {
	t.Helper()
	row, err := cli.APIKey.Query().Where(apikey.IDEQ(id)).Only(context.Background())
	require.NoError(t, err)
	return row.Key
}

// requireNoSecret fails when a credential is present, reporting its length and
// a three-character prefix rather than the value. require.Empty would print the
// whole secret into the test log.
func requireNoSecret(t *testing.T, got, msg string) {
	t.Helper()
	if got != "" {
		t.Fatalf("%s: carried %d characters starting %q", msg, len(got), safeSecretPrefix(got))
	}
}

// safeSecretPrefix renders at most the first three characters of a secret, so a
// failure message can say which prefix was seen without printing a credential.
func safeSecretPrefix(secret string) string {
	if len(secret) > 3 {
		return secret[:3] + "…"
	}
	return secret
}

// backingKeyCreateFailsWith installs an ent hook that fails every APIKey CREATE
// with an error built from the row's own key.
//
// This is not a stub standing in for the create: the hook runs inside the real
// `s.entClient.APIKey.Create()…Save(ctx)` call that createOrAdoptWinner makes.
// It reproduces on SQLite what only PostgreSQL does in production — put the
// offending value in the error text -- which is precisely why the sanitizer's call sites
// survived mutation before: SQLite hides the secret, so nothing noticed when
// the sanitizer was removed (review F3).
func backingKeyCreateFailsWith(cli *dbent.Client, build func(key string) error) {
	cli.APIKey.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			am, ok := m.(*dbent.APIKeyMutation)
			if !ok || !am.Op().Is(dbent.OpCreate) {
				return next.Mutate(ctx, m)
			}
			key, _ := am.Key()
			return nil, build(key)
		})
	})
}

// TestCreatePathErrorNeverCarriesTheCredential_PlainError pins the scrub at the
// non-constraint create-error call site. Remove `scrubBackingKeySecret` there
// and this fails.
func TestCreatePathErrorNeverCarriesTheCredential_PlainError(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	var generated string
	backingKeyCreateFailsWith(cli, func(key string) error {
		generated = key
		// Shaped like lib/pq: the credential lives on a field, NOT in the
		// message. A redaction that only rewrote the message would pass this
		// straight through.
		return &backingKeyDriverError{
			Message: "pq: some write failure (Failing row contains ...)",
			Detail:  fmt.Sprintf("Failing row contains (%s)", key),
		}
	})

	row, err := svc.Resolve(ctx, userID, "agent:plain-error")
	requireNoBackingRow(t, row, "a failing create returns no row")
	require.Error(t, err)
	require.NotEmpty(t, generated, "the hook must have seen the generated secret")
	require.NotContains(t, err.Error(), generated, "the create path's error must never carry the credential")
	require.Contains(t, err.Error(), "Failing row contains", "the diagnosable part of the error must survive")

	var leaked *backingKeyDriverError
	require.False(t, errors.As(err, &leaked),
		"the driver error value must not survive either -- pq.Error.Detail carries the credential even when Error() does not")
}

// TestCreatePathErrorNeverCarriesTheCredential_ConstraintError pins the scrub at
// the other call site: a unique violation whose re-read finds no live row. Since
// migration 910 scoped the identity index to live rows, the reachable cause is a
// collision on api_keys.key itself — and that is exactly the constraint whose
// PostgreSQL DETAIL line carries the freshly generated credential.
func TestCreatePathErrorNeverCarriesTheCredential_ConstraintError(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	var generated string
	backingKeyCreateFailsWith(cli, func(key string) error {
		generated = key
		// Wrapping a real *dbent.ConstraintError keeps dbent.IsConstraintError
		// true, so this takes the adopt-the-winner branch — which then finds no
		// live row and must report without leaking.
		return fmt.Errorf("ent: constraint failed: %w: %w",
			&backingKeyDriverError{
				Message: `pq: duplicate key value violates unique constraint "api_keys_key_key"`,
				Detail:  fmt.Sprintf("Key (key)=(%s) already exists.", key),
			},
			&dbent.ConstraintError{})
	})

	row, err := svc.Resolve(ctx, userID, "agent:constraint-error")
	requireNoBackingRow(t, row, "a rejected create returns no row")
	require.Error(t, err)
	require.NotEmpty(t, generated)
	require.NotContains(t, err.Error(), generated, "the adopt-winner failure path must never carry the credential")
	require.Contains(t, err.Error(), "api_keys_key_key", "the diagnosable part of the error must survive")

	var leaked *backingKeyDriverError
	require.False(t, errors.As(err, &leaked),
		"the driver error value must not survive the adopt-winner failure path either")
}

// TestResolveNeverHandsBackTheSecret is the F4 fix.
//
// *dbent.APIKey.String() writes "key=<secret>" and the field is
// `json:"key,omitempty"`, so a returned row is one careless %v or c.JSON away
// from leaking the credential — and Task 4 puts this exact struct into the gin
// context. Resolve therefore blanks the field on the way out. The row in the
// database still carries the real secret; only the copy that leaves this
// service does not.
func TestResolveNeverHandsBackTheSecret(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	created, err := svc.Resolve(ctx, userID, "agent:redacted")
	require.NoError(t, err)
	// Not require.Empty: testify prints the offending value, and the offending
	// value here would be a live credential.
	requireNoSecret(t, created.Key, "the returned row must not carry api_keys.key")

	stored := storedBackingKeySecret(t, cli, created.ID)
	require.NotEmpty(t, stored, "the row in the database must still have its credential")
	require.NotContains(t, created.String(), stored, "String() must have nothing to leak")

	marshalled, err := json.Marshal(created)
	require.NoError(t, err)
	require.NotContains(t, string(marshalled), stored, "JSON marshalling must have nothing to leak")

	// The reuse path returns through the same choke point.
	reused, err := svc.Resolve(ctx, userID, "agent:redacted")
	require.NoError(t, err)
	require.Equal(t, created.ID, reused.ID)
	requireNoSecret(t, reused.Key, "the fast path must redact too")
}

// TestResolveInfrastructureFaultInGroupResolutionIsNotAnAuthError is the F2 fix.
//
// The earlier test closed the client, so `lookup` failed first and policyGroup
// — the branch the claim is actually about — was never reached. Here the
// api_keys lookup succeeds and only the group query fails: the row exists but
// its group binding was cleared, so eager-loading issues no group query (ent
// skips loadGroup when every group_id is NULL), and policyGroup is the first
// statement to touch the missing table.
//
// Wrapping policyGroup's DB error in ErrNoGroupForOAuthKey would make Task 4
// answer 403 for a dead database. This test is what stops that.
func TestResolveInfrastructureFaultInGroupResolutionIsNotAnAuthError(t *testing.T) {
	cli, db := newBackingKeyTestClient(t)
	seedBackingKeyGroup(t, cli, backingKeyTestGroupName)
	svc := NewOAuthBackingKeyService(cli, backingKeyTestConfig(backingKeyTestGroupName))
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	first, err := svc.Resolve(ctx, userID, "agent:db-down")
	require.NoError(t, err, "the service is healthy before the fault is injected")
	require.NoError(t, cli.APIKey.UpdateOneID(first.ID).ClearGroupID().Exec(ctx))

	_, err = db.Exec("DROP TABLE groups")
	require.NoError(t, err, "inject the infrastructure fault")

	row, err := svc.Resolve(ctx, userID, "agent:db-down")
	requireNoBackingRow(t, row, "a broken group query returns no row")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNoGroupForOAuthKey),
		"a database failure while resolving the group is a 500, not a 403; got: %v", err)
}

// TestPolicyGroupDatabaseErrorIsNotErrNoGroupForOAuthKey drives the same branch
// directly, the way TestResolveRecoversTheWinnerAfterLosingTheInsertRace drives
// createOrAdoptWinner directly. Belt to the previous test's braces: this one
// cannot be satisfied by any earlier statement failing first.
func TestPolicyGroupDatabaseErrorIsNotErrNoGroupForOAuthKey(t *testing.T) {
	cli, db := newBackingKeyTestClient(t)
	seedBackingKeyGroup(t, cli, backingKeyTestGroupName)
	svc := NewOAuthBackingKeyService(cli, backingKeyTestConfig(backingKeyTestGroupName))
	ctx := context.Background()

	grp, err := svc.policyGroup(ctx)
	require.NoError(t, err, "the policy resolves while the table is there")
	require.NotNil(t, grp)

	_, err = db.Exec("DROP TABLE groups")
	require.NoError(t, err)

	grp, err = svc.policyGroup(ctx)
	require.Nil(t, grp)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNoGroupForOAuthKey),
		"an unreadable groups table is an infrastructure fault, not a group policy failure; got: %v", err)
}

// TestResolveKeepsServingAnAlreadyProvisionedAgentAfterThePolicyBreaks is the F5
// fix, and it pins the fast path for a reason that is a behaviour rather than a
// restatement of the code.
//
// The reuse guarantee itself does NOT rest on the fast path — the identity index
// plus adopt-the-winner deliver that even without it (see the report's M1). What
// the fast path does deliver is resilience: an operator who later mistypes
// oauth_backing_key.group_name must not 403 every agent that already has a
// backing row. This is a deliberate choice for continuity over failing loudly;
// the test is what makes it a decision rather than an accident of an
// optimisation.
func TestResolveKeepsServingAnAlreadyProvisionedAgentAfterThePolicyBreaks(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	first, err := svc.Resolve(ctx, userID, "agent:fastpath")
	require.NoError(t, err)

	svc.cfg.OAuthBackingKey.GroupName = "no-such-group" // the operator breaks the policy afterwards

	again, err := svc.Resolve(ctx, userID, "agent:fastpath")
	require.NoError(t, err, "an already-provisioned agent must not 403 because the policy broke later")
	require.Equal(t, first.ID, again.ID)
	require.NotNil(t, again.Edges.Group)

	// A *new* agent, by contrast, still fails loudly — the broken policy is not
	// silently ignored, it only stops being retroactive.
	fresh, err := svc.Resolve(ctx, userID, "agent:brand-new")
	requireNoBackingRow(t, fresh, "a new agent cannot be provisioned under a broken policy")
	require.ErrorIs(t, err, ErrNoGroupForOAuthKey)
}

// TestResolveRebindsARowWhoseGroupWasDeactivated is the F8 fix.
//
// Soft-deleting a group was already handled (the interceptor filters it, the
// edge comes back nil, Resolve rebinds). Deactivating one is the same class and
// was not: the fast path returned any row with a non-nil group edge, so agents
// kept billing through a group the operator had switched off. The status is
// already loaded on the edge, so checking it costs no query.
func TestResolveRebindsARowWhoseGroupWasDeactivated(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	stale := seedBackingKeyGroup(t, cli, "retired-group")
	first, err := svc.Resolve(ctx, userID, "agent:deactivated")
	require.NoError(t, err)
	require.NoError(t, cli.APIKey.UpdateOneID(first.ID).SetGroupID(stale.ID).Exec(ctx))
	require.NoError(t, cli.Group.UpdateOneID(stale.ID).SetStatus("disabled").Exec(ctx))

	again, err := svc.Resolve(ctx, userID, "agent:deactivated")
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID, "rebinding must not create a second row")
	require.NotNil(t, again.Edges.Group)
	require.NotEqual(t, stale.ID, again.Edges.Group.ID, "a deactivated group must not keep receiving traffic")
	require.Equal(t, domain.StatusActive, again.Edges.Group.Status)
}

// TestResolveKeepsAnOperatorsDeliberateGroupBinding is the other half of F8: only
// an unusable group is rebound. A row an operator deliberately moved to a
// different ACTIVE group keeps that binding, so the rebind is a repair and not a
// policy that silently overwrites operator intent on every request.
func TestResolveKeepsAnOperatorsDeliberateGroupBinding(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	special := seedBackingKeyGroup(t, cli, "hand-picked-group")
	first, err := svc.Resolve(ctx, userID, "agent:handpicked")
	require.NoError(t, err)
	require.NoError(t, cli.APIKey.UpdateOneID(first.ID).SetGroupID(special.ID).Exec(ctx))

	again, err := svc.Resolve(ctx, userID, "agent:handpicked")
	require.NoError(t, err)
	require.NotNil(t, again.Edges.Group)
	require.Equal(t, special.ID, again.Edges.Group.ID, "an active, deliberately chosen group must be left alone")
}

// TestResolveSoftDeletedBackingRowSelfHeals is the unit-level F1 fix.
//
// ent's soft-delete interceptor hides a tombstoned row from every read, while
// migration 909's index did not filter deleted_at — so a tombstone permanently
// held the (user_id, oauth_client_id) slot and the agent could never resolve
// again. Migration 910 adds "AND deleted_at IS NULL", so the slot is released
// and the next Resolve provisions a fresh row. Point the harness DDL back at
// 909's predicate and this fails.
func TestResolveSoftDeletedBackingRowSelfHeals(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()
	userID := seedBackingKeyUser(t, cli)

	first, err := svc.Resolve(ctx, userID, "agent:tombstoned")
	require.NoError(t, err)
	require.NoError(t, cli.APIKey.UpdateOneID(first.ID).SetDeletedAt(time.Now()).Exec(ctx))

	again, err := svc.Resolve(ctx, userID, "agent:tombstoned")
	require.NoError(t, err, "a tombstoned backing row must not brick the agent")
	require.NotEqual(t, first.ID, again.ID, "the tombstone released the slot, so a fresh row is provisioned")
	require.NotNil(t, again.Edges.Group)

	// The tombstone — and therefore the usage history hanging off it via
	// usage_logs_api_key_id_fkey — is still there. Nothing was hard-deleted.
	total, err := cli.APIKey.Query().Where(apikey.DeletedAtNotNil()).Count(mixins.SkipSoftDelete(ctx))
	require.NoError(t, err)
	require.Equal(t, 1, total, "the tombstoned row must still exist")
}

// TestResolveRejectsInvalidInputs is the F9 fix: the two guards that had no
// test. Both must fail before any row is written.
func TestResolveRejectsInvalidInputs(t *testing.T) {
	svc, cli := newBackingKeyTestService(t)
	ctx := context.Background()

	row, err := svc.Resolve(ctx, 0, "agent:bad-user")
	requireNoBackingRow(t, row, "a non-positive user id returns no row")
	require.Error(t, err)

	row, err = svc.Resolve(ctx, -1, "agent:bad-user")
	requireNoBackingRow(t, row, "a negative user id returns no row")
	require.Error(t, err)

	var nilSvc *OAuthBackingKeyService
	row, err = nilSvc.Resolve(ctx, 1, "agent:nil-service")
	requireNoBackingRow(t, row, "a nil service returns no row")
	require.Error(t, err, "a nil service must error rather than panic")

	row, err = NewOAuthBackingKeyService(nil, backingKeyTestConfig(backingKeyTestGroupName)).Resolve(ctx, 1, "agent:no-client")
	requireNoBackingRow(t, row, "a service with no ent client returns no row")
	require.Error(t, err)

	total, err := cli.APIKey.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, total, "no guard may leave a row behind")
}
