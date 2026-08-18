//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/suite"
)

// APIKeyOAuthClientIDSuite exercises the partial unique index added by
// migrations/909_api_key_oauth_client_id.sql:
//
//	CREATE UNIQUE INDEX ... ON api_keys (user_id, oauth_client_id)
//	    WHERE oauth_client_id IS NOT NULL;
//
// This runs against a real Postgres 18 container via IntegrationDBSuite
// (internal/repository/integration_harness_test.go), so the partial-index
// semantics under test are the same ones Postgres enforces in production --
// not an approximation. Each test gets its own transaction that rolls back
// in SetupTest/t.Cleanup, so a constraint violation in one test never
// poisons another.
type APIKeyOAuthClientIDSuite struct {
	IntegrationDBSuite
}

func TestAPIKeyOAuthClientIDSuite(t *testing.T) {
	suite.Run(t, new(APIKeyOAuthClientIDSuite))
}

// TestDuplicateBackingRowRejected proves the identity rule: at most one
// backing row per (user_id, oauth_client_id). The second insert for the
// same pair must fail with a Postgres unique-violation, surfaced by ent as
// a ConstraintError.
func (s *APIKeyOAuthClientIDSuite) TestDuplicateBackingRowRejected() {
	user := mustCreateUser(s.T(), s.client, &service.User{})

	_, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-dup-1").
		SetName("agent:dup backing row").
		SetOauthClientID("agent:duplicate-client").
		Save(s.ctx)
	s.Require().NoError(err, "first backing row for (user, client) should insert")

	_, err = s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-dup-2").
		SetName("agent:dup backing row 2").
		SetOauthClientID("agent:duplicate-client").
		Save(s.ctx)
	s.Require().Error(err, "second backing row for the SAME (user, client) must be rejected")
	s.Require().True(dbent.IsConstraintError(err), "expected a unique-constraint violation, got: %v", err)
}

// TestDifferentOAuthClientIDBothInsert proves the index is keyed on the
// PAIR, not on user_id alone: the same user may have one backing row per
// distinct OAuth client.
func (s *APIKeyOAuthClientIDSuite) TestDifferentOAuthClientIDBothInsert() {
	user := mustCreateUser(s.T(), s.client, &service.User{})

	_, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-client-a").
		SetName("agent:client A").
		SetOauthClientID("agent:client-a").
		Save(s.ctx)
	s.Require().NoError(err, "backing row for client A should insert")

	_, err = s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-client-b").
		SetName("agent:client B").
		SetOauthClientID("agent:client-b").
		Save(s.ctx)
	s.Require().NoError(err, "backing row for client B (same user, different client) should also insert")
}

// TestTwoOrdinaryKeysWithNullOAuthClientIDBothInsert asserts that ordinary
// user-created keys, which all carry oauth_client_id = NULL, are never
// constrained by this index: any number of them coexist per user.
//
// NOTE on what this test does and does not prove about partiality: a
// mutation-prove exercise (Task 2 Step 5 -- see task-2-report.md) applied a
// scratch copy of migrations/909 with the "WHERE oauth_client_id IS NOT
// NULL" clause removed against a real Postgres 18 instance. Two NULL-valued
// keys for the same user STILL both inserted under the non-partial index --
// Postgres unique indexes treat every NULL as distinct from every other
// NULL by default (no NULLS NOT DISTINCT clause is used here), so this
// property does not depend on the WHERE clause. This test therefore does
// NOT, by itself, distinguish "partial" from "non-partial" for correctness.
// The WHERE clause is still the right call: it keeps the index to just the
// (rare) backing rows instead of indexing every ordinary key in the system
// (same rationale as migrations/904 and migrations/908), and it removes the
// silent dependency on Postgres's default NULL-uniqueness behavior rather
// than relying on it implicitly.
func (s *APIKeyOAuthClientIDSuite) TestTwoOrdinaryKeysWithNullOAuthClientIDBothInsert() {
	user := mustCreateUser(s.T(), s.client, &service.User{})

	_, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-ordinary-1").
		SetName("Ordinary key 1").
		Save(s.ctx)
	s.Require().NoError(err, "first ordinary (NULL oauth_client_id) key should insert")

	_, err = s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-ordinary-2").
		SetName("Ordinary key 2").
		Save(s.ctx)
	s.Require().NoError(err, "second ordinary (NULL oauth_client_id) key for the SAME user should also insert")
}

// ---------------------------------------------------------------------------
// Fix round 1 — migration 910 and the PostgreSQL-only credential leak channel.
// ---------------------------------------------------------------------------

// TestSoftDeletedBackingRowReleasesTheSlot proves migration 910 against real
// PostgreSQL.
//
// Under 909's predicate (`WHERE oauth_client_id IS NOT NULL`) this insert was
// refused: ent's soft-delete interceptor hides a tombstoned row from every read,
// but the index counted it, so a tombstone held the (user_id, oauth_client_id)
// slot forever and the agent could never resolve again. 910 adds
// `AND deleted_at IS NULL`, so the slot is released and the agent self-heals.
//
// This runs against the real migrations (TestMain calls ApplyMigrations), so
// what is under test is the shipped index, not a copy of it.
func (s *APIKeyOAuthClientIDSuite) TestSoftDeletedBackingRowReleasesTheSlot() {
	user := mustCreateUser(s.T(), s.client, &service.User{})

	first, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-tombstone-1").
		SetName("agent:tombstoned backing row").
		SetOauthClientID("agent:tombstoned").
		Save(s.ctx)
	s.Require().NoError(err)

	// Tombstone it the way APIKeyService.Delete's repository call does.
	s.Require().NoError(s.client.APIKey.UpdateOneID(first.ID).SetDeletedAt(time.Now()).Exec(s.ctx))

	replacement, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-tombstone-2").
		SetName("agent:tombstoned backing row (replacement)").
		SetOauthClientID("agent:tombstoned").
		Save(s.ctx)
	s.Require().NoError(err, "a tombstone must not hold the identity slot; migration 910 scopes the index to live rows")
	s.Require().NotEqual(first.ID, replacement.ID)

	// The tombstone still exists, so the usage history hanging off it via
	// usage_logs_api_key_id_fkey (ON DELETE CASCADE) is untouched.
	tombstoned, err := s.client.APIKey.Query().
		Where(apikey.IDEQ(first.ID)).
		Only(mixins.SkipSoftDelete(s.ctx))
	s.Require().NoError(err, "nothing may hard-delete a backing row")
	s.Require().NotNil(tombstoned.DeletedAt)

	// And two LIVE rows for the same pair are still refused: 910 narrowed the
	// index, it did not weaken the identity rule.
	_, err = s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-oauth-tombstone-3").
		SetName("agent:tombstoned backing row (third)").
		SetOauthClientID("agent:tombstoned").
		Save(s.ctx)
	s.Require().Error(err)
	s.Require().True(dbent.IsConstraintError(err))
}

// TestPostgresUniqueViolationHidesTheKeyInErrorTextButNotInTheErrorValue
// records what real PostgreSQL + lib/pq actually do, because the Task 3
// review's premise was half wrong and the difference changes what the guard in
// service.sanitizeBackingKeyError has to be.
//
// A unique violation on api_keys.key gives:
//
//	err.Error() = ent: constraint failed: pq: duplicate key value violates
//	              unique constraint "api_keys_key_key"
//	pqError.Detail = Key (key)=(sk-...) already exists.
//
// So the credential is NOT in the error's TEXT -- lib/pq's Error() renders only
// Severity and Message, never Detail -- but it IS in the error VALUE, sitting on
// an exported *pq.Error.Detail field that travels up every wrapped chain. A
// redaction that only rewrites err.Error() therefore never sees it, while
// anything that serialises the error structurally (zap.Any("err", err),
// %#v, a JSON error reporter) prints it in full.
//
// This test is a canary in both directions:
//   - if lib/pq ever starts rendering Detail in Error(), the first assertion
//     fails and the text-level redaction stops being defence-in-depth;
//   - if PostgreSQL ever stops populating Detail, the second fails and the
//     value-level flattening can be reconsidered.
func (s *APIKeyOAuthClientIDSuite) TestPostgresUniqueViolationHidesTheKeyInErrorTextButNotInTheErrorValue() {
	user := mustCreateUser(s.T(), s.client, &service.User{})
	// A literal test string, not a generated secret: nothing real is printed
	// even when an assertion fails.
	const secret = "sk-detail-line-carries-this-value"

	_, err := s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey(secret).
		SetName("first key").
		Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey(secret).
		SetName("colliding key").
		Save(s.ctx)
	s.Require().Error(err)
	s.Require().True(dbent.IsConstraintError(err))

	s.Require().NotContains(err.Error(), secret,
		"lib/pq renders only Severity+Message, so the credential is not in the error text")

	var pqErr *pq.Error
	s.Require().True(errors.As(err, &pqErr), "the driver error must still be in the chain")
	s.Require().Contains(pqErr.Detail, secret,
		"PostgreSQL puts the offending key in DETAIL, and lib/pq keeps it on an exported field -- this is the real leak channel, and it is why service.sanitizeBackingKeyError flattens the error value instead of only rewriting its text")
}
