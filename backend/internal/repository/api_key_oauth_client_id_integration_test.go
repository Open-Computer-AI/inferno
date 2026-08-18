//go:build integration

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
