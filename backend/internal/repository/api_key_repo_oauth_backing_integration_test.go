//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// APIKeyRepoOAuthBackingSuite pins the persistence half of Task 5 against real
// PostgreSQL 18: the `oauth_client_id IS NULL` predicate on the shared key
// listing query, and the secret blanking on the usage-log api_key hydration.
//
// Real PostgreSQL rather than SQLite on purpose. Both behaviours under test are
// SQL-shaped -- a partial WHERE clause and a NULL-vs-non-NULL column read
// through the raw usage-log query path -- and this project has already shipped
// three tests that passed on an engine where the behaviour under test does not
// exist, including a secret-leak test in Task 3 that only PostgreSQL could fail.
type APIKeyRepoOAuthBackingSuite struct {
	suite.Suite

	ctx      context.Context
	tx       *dbent.Tx
	client   *dbent.Client
	keys     *apiKeyRepository
	usage    *usageLogRepository
	user     *service.User
	group    *service.Group
	account  *service.Account
	ordinary *service.APIKey
	backing  *service.APIKey
}

func TestAPIKeyRepoOAuthBackingSuite(t *testing.T) {
	suite.Run(t, new(APIKeyRepoOAuthBackingSuite))
}

const (
	backingSweepClientID     = "agent:repo-suite"
	backingSweepSecret       = "sk-backing-repo-suite-secret"
	backingSweepOrdinaryKey  = "sk-ordinary-repo-suite-secret"
	backingSweepOrdinaryName = "the user's own key"
)

func (s *APIKeyRepoOAuthBackingSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.client = tx.Client()
	s.keys = newAPIKeyRepositoryWithSQL(s.client, tx)
	s.usage = newUsageLogRepositoryWithSQL(s.client, tx)

	s.user = mustCreateUser(s.T(), s.client, &service.User{})
	s.group = mustCreateGroup(s.T(), s.client, &service.Group{Name: "oauth-backing-suite-" + uuid.NewString()})
	s.account = mustCreateAccount(s.T(), s.client, &service.Account{Name: "acct-" + uuid.NewString()})

	clientID := backingSweepClientID
	s.ordinary = mustCreateApiKey(s.T(), s.client, &service.APIKey{
		UserID:  s.user.ID,
		Key:     backingSweepOrdinaryKey,
		Name:    backingSweepOrdinaryName,
		GroupID: &s.group.ID,
	})
	s.backing = mustCreateApiKey(s.T(), s.client, &service.APIKey{
		UserID:        s.user.ID,
		Key:           backingSweepSecret,
		Name:          "OAuth agent " + clientID,
		GroupID:       &s.group.ID,
		OAuthClientID: &clientID,
	})
}

func (s *APIKeyRepoOAuthBackingSuite) listedIDs(keys []service.APIKey) []int64 {
	ids := make([]int64, 0, len(keys))
	for i := range keys {
		ids = append(ids, keys[i].ID)
	}
	return ids
}

func (s *APIKeyRepoOAuthBackingSuite) params() pagination.PaginationParams {
	return pagination.PaginationParams{Page: 1, PageSize: 50, SortBy: "id", SortOrder: pagination.SortOrderAsc}
}

// TestListByUserIDHidesTheBackingRow is the listing half of the rule. The user
// owns both rows; only the ordinary one is theirs to see.
//
// The pagination total is asserted too, because a filter applied after the
// count would still report "2 keys" and leak the row's existence.
func (s *APIKeyRepoOAuthBackingSuite) TestListByUserIDHidesTheBackingRow() {
	keys, page, err := s.keys.ListByUserID(s.ctx, s.user.ID, s.params(), service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Equal([]int64{s.ordinary.ID}, s.listedIDs(keys))
	s.Require().Equal(int64(1), page.Total, "the count must be filtered too, not just the page")

	for i := range keys {
		s.Require().NotEqual(backingSweepSecret, keys[i].Key)
	}
}

// TestListAllByUserIDHidesTheBackingRow covers the OTHER listing path.
// sort_by=current_concurrency loads every key unpaginated through
// ListAllByUserID; both paths share apiKeyListByUserIDQuery, and this is what
// proves they do.
func (s *APIKeyRepoOAuthBackingSuite) TestListAllByUserIDHidesTheBackingRow() {
	keys, err := s.keys.ListAllByUserID(s.ctx, s.user.ID, service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Equal([]int64{s.ordinary.ID}, s.listedIDs(keys))
}

// TestSearchCannotReachTheBackingRowByItsSecret closes an enumeration channel
// that is easy to miss: the list `search` filter matches api_keys.key with
// ILIKE, so without the predicate a user could confirm a guessed secret --
// or a support tool could echo one back -- through GET /keys?search=.
func (s *APIKeyRepoOAuthBackingSuite) TestSearchCannotReachTheBackingRowByItsSecret() {
	keys, page, err := s.keys.ListByUserID(s.ctx, s.user.ID, s.params(), service.APIKeyListFilters{
		Search: backingSweepSecret,
	})
	s.Require().NoError(err)
	s.Require().Empty(keys, "a backing row must not be reachable by searching for its own secret")
	s.Require().Equal(int64(0), page.Total)
}

// TestSearchCannotReachTheBackingRowByItsName is the same channel through the
// other ILIKE column. backingKeyName() labels the row "OAuth agent <client_id>",
// which is a guessable string.
func (s *APIKeyRepoOAuthBackingSuite) TestSearchCannotReachTheBackingRowByItsName() {
	keys, _, err := s.keys.ListByUserID(s.ctx, s.user.ID, s.params(), service.APIKeyListFilters{
		Search: "OAuth agent",
	})
	s.Require().NoError(err)
	s.Require().Empty(keys)
}

// TestIncludeOAuthBackingIsTheOnlyWayToSeeIt proves the opt-out actually opts
// in -- i.e. that the predicate is conditional rather than unconditional.
//
// This is not decoration: admin user deletion is the one caller that sets it,
// and it MUST still see backing rows, or deleting a user would leave live,
// never-expiring credentials behind owned by a user who no longer exists.
func (s *APIKeyRepoOAuthBackingSuite) TestIncludeOAuthBackingIsTheOnlyWayToSeeIt() {
	keys, page, err := s.keys.ListByUserID(s.ctx, s.user.ID, s.params(), service.APIKeyListFilters{
		IncludeOAuthBacking: true,
	})
	s.Require().NoError(err)
	s.Require().ElementsMatch([]int64{s.ordinary.ID, s.backing.ID}, s.listedIDs(keys))
	s.Require().Equal(int64(2), page.Total)
}

// TestGetByIDStillFindsTheBackingRow guards the boundary of where the predicate
// was put. apiKeyRepository.GetByID must NOT filter: it is the billing hot
// path's read (APIKeyService.UpdateQuotaUsed re-reads the row to decide quota
// exhaustion), and the backing row is exactly the row that path must find.
// The user-facing refusal lives one layer up, in APIKeyService.managedAPIKey.
func (s *APIKeyRepoOAuthBackingSuite) TestGetByIDStillFindsTheBackingRow() {
	row, err := s.keys.GetByID(s.ctx, s.backing.ID)
	s.Require().NoError(err)
	s.Require().NotNil(row.OAuthClientID)
	s.Require().Equal(backingSweepClientID, *row.OAuthClientID)
}

func (s *APIKeyRepoOAuthBackingSuite) insertUsageLog(apiKeyID int64) {
	s.T().Helper()
	_, err := s.tx.ExecContext(s.ctx,
		`INSERT INTO usage_logs (user_id, api_key_id, account_id, request_id, model, total_cost, actual_cost)
		 VALUES ($1, $2, $3, $4, $5, 0, 0)`,
		s.user.ID, apiKeyID, s.account.ID, uuid.NewString(), "claude-3")
	s.Require().NoError(err, "insert usage log")
}

// TestUsageLogHydrationBlanksOnlyTheBackingSecret is the other user-facing
// surface, and the one the brief's "hand-picked two" would have missed.
//
// GET /api/v1/usage embeds the whole dto.APIKey in every usage row
// (dto.usageLogFromServiceUser), `key` included, and an OAuth agent's inference
// writes its usage against the BACKING row -- so before this the owner could
// read the backing secret out of their own usage list without ever touching a
// /keys endpoint. Filtering the row out is not an option here: that would hide
// the agent's usage from the user it is billed to. Only the credential goes,
// and the attribution (name) stays.
func (s *APIKeyRepoOAuthBackingSuite) TestUsageLogHydrationBlanksOnlyTheBackingSecret() {
	s.insertUsageLog(s.ordinary.ID)
	s.insertUsageLog(s.backing.ID)

	logs, _, err := s.usage.ListWithFilters(s.ctx, s.params(), UsageLogFilters{UserID: s.user.ID})
	s.Require().NoError(err)
	s.Require().Len(logs, 2)

	seen := map[int64]*service.APIKey{}
	for i := range logs {
		s.Require().NotNil(logs[i].APIKey, "the api_key edge must still be hydrated")
		seen[logs[i].APIKeyID] = logs[i].APIKey
	}

	backing := seen[s.backing.ID]
	s.Require().NotNil(backing)
	s.Require().Empty(backing.Key, "the backing row's secret must never leave the server")
	s.Require().Equal("OAuth agent "+backingSweepClientID, backing.Name,
		"the attribution must survive -- the user still has to see whose usage this is")

	ordinary := seen[s.ordinary.ID]
	s.Require().NotNil(ordinary)
	s.Require().Equal(backingSweepOrdinaryKey, ordinary.Key,
		"an ordinary key's own usage rows are unchanged")
}
