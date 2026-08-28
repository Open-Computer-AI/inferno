//go:build unit

// Unit tests for the user-facing key-management refusals that keep an OAuth
// backing row untouchable. The listing half of Task 5 is a SQL predicate and is
// tested against real PostgreSQL in
// internal/repository/api_key_repo_oauth_backing_integration_test.go; a stub
// repository cannot observe a WHERE clause, and asserting one here would be a
// test mirroring its own fixture.

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func backingRowStub(id, userID int64, clientID string) *APIKey {
	return &APIKey{ID: id, UserID: userID, Key: "sk-backing-secret", OAuthClientID: &clientID}
}

// TestAPIKeyService_GetByID_ReportsOAuthBackingRowAsNotFound pins the read half.
//
// The row IS owned by the caller, so ownership cannot be what refuses this. It
// is refused because a backing row is invisible on every user-facing surface --
// and because APIKeyService.GetByID's own response body carries api_keys.key,
// which for a backing row is a live, non-expiring credential the server
// promised never to return.
func TestAPIKeyService_GetByID_ReportsOAuthBackingRowAsNotFound(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: backingRowStub(42, 7, "agent:owned-by-this-user")}
	svc := &APIKeyService{apiKeyRepo: repo}

	key, err := svc.GetByID(context.Background(), 42)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Nil(t, key, "no row may be returned, because the row carries the secret")
}

// TestAPIKeyService_GetByID_StillReturnsOrdinaryKeys is the other half: the
// refusal keys on oauth_client_id being non-NULL, so it must leave alone the
// ordinary keys that are every other row in the table.
func TestAPIKeyService_GetByID_StillReturnsOrdinaryKeys(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: &APIKey{ID: 43, UserID: 7, Key: "sk-ordinary", OAuthClientID: nil}}
	svc := &APIKeyService{apiKeyRepo: repo}

	key, err := svc.GetByID(context.Background(), 43)
	require.NoError(t, err)
	require.Equal(t, int64(43), key.ID)
	require.Equal(t, "sk-ordinary", key.Key, "an ordinary key's owner still gets its secret")
}

// TestAPIKeyService_Update_RefusesOAuthBackingRow closes what the Task 4 review
// left open: "APIKeyService.Update still accepts IP-ACL edits on backing rows".
//
// The caller is the OWNER -- that is the point, ownership is not a sufficient
// authorization check for this row -- and the refusal must happen before
// anything is written.
func TestAPIKeyService_Update_RefusesOAuthBackingRow(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: backingRowStub(42, 7, "agent:owned-by-this-user")}
	svc := &APIKeyService{apiKeyRepo: repo}

	name := "renamed by its owner"
	_, err := svc.Update(context.Background(), 42, 7, UpdateAPIKeyRequest{Name: &name})
	require.ErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable)
	require.Empty(t, repo.updatedKeys, "no column may be written on a backing row")
}

// TestAPIKeyService_Update_RefusesEveryEditableColumnOnABackingRow enumerates
// the edits the refusal has to cover, one request per column family.
//
// group_id is the one that matters most and the reason this is a blanket
// refusal rather than a per-column one: apiKey.Group is the only routing input
// in the gateway pipeline, and OAuthBackingKeyService.Resolve rebinds a backing
// row's group only when that group is missing or inactive -- so pointing a
// backing row at a different ACTIVE group silently re-routes and re-prices that
// agent's inference forever. The others (expires_at, status, the IP ACL) are
// user-reachable bricks; Task 4's F1 fix made the IP ACL apply to OAuth-backed
// requests, which is exactly what turns a whitelist edit into a lockout.
func TestAPIKeyService_Update_RefusesEveryEditableColumnOnABackingRow(t *testing.T) {
	groupID := int64(99)
	status := StatusDisabled
	quota := 0.0
	reset := true
	whitelist := []string{"203.0.113.7"}
	blacklist := []string{"203.0.113.8"}
	limit := 1.0

	cases := map[string]UpdateAPIKeyRequest{
		"group_id — silent permanent re-route of the agent's inference": {GroupID: &groupID},
		"status — disables the agent":                                   {Status: &status},
		"ip_whitelist — locks the agent out (Task 4 F1 made it apply)":  {IPWhitelist: &whitelist},
		"ip_blacklist — locks the agent out":                            {IPBlacklist: &blacklist},
		"quota — starves the agent":                                     {Quota: &quota},
		"reset_quota — erases the agent's billing ledger":               {ResetQuota: &reset},
		"clear_expiration":                                              {ClearExpiration: true},
		"rate_limit_5h":                                                 {RateLimit5h: &limit},
		"rate_limit_1d":                                                 {RateLimit1d: &limit},
		"rate_limit_7d":                                                 {RateLimit7d: &limit},
		"reset_rate_limit_usage":                                        {ResetRateLimitUsage: &reset},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &apiKeyRepoStub{apiKey: backingRowStub(42, 7, "agent:owned-by-this-user")}
			svc := &APIKeyService{apiKeyRepo: repo}

			_, err := svc.Update(context.Background(), 42, 7, req)
			require.ErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable)
			require.Empty(t, repo.updatedKeys)
		})
	}
}

// TestAPIKeyService_Update_StillEditsOrdinaryKeys guards the blast radius: the
// key-management surface standalone Inferno customers depend on must be
// unchanged for the rows that are not backing rows.
func TestAPIKeyService_Update_StillEditsOrdinaryKeys(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: &APIKey{ID: 43, UserID: 7, Key: "sk-ordinary", Status: StatusActive}}
	svc := &APIKeyService{apiKeyRepo: repo}

	name := "renamed"
	updated, err := svc.Update(context.Background(), 43, 7, UpdateAPIKeyRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.Len(t, repo.updatedKeys, 1)
	require.Equal(t, "renamed", repo.updatedKeys[0].Name)
}

// TestAPIKeyService_Update_OwnershipStillOutranksTheBackingRefusal keeps the
// two refusals distinguishable. A backing row belonging to SOMEONE ELSE must
// answer ErrInsufficientPerms, not ErrOAuthBackingKeyUnmodifiable: the second
// would confirm to a stranger that the id they guessed is an OAuth agent's.
func TestAPIKeyService_Update_OwnershipStillOutranksTheBackingRefusal(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: backingRowStub(42, 7, "agent:someone-elses")}
	svc := &APIKeyService{apiKeyRepo: repo}

	name := "n"
	_, err := svc.Update(context.Background(), 42, 8, UpdateAPIKeyRequest{Name: &name})
	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.NotErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable)
}

// TestAPIKeyService_List_DoesNotOptIntoBackingRows states exactly what it
// proves and no more: the /keys listing never sets
// APIKeyListFilters.IncludeOAuthBacking, so it takes the safe default.
//
// It does NOT prove backing rows are excluded -- that is a WHERE clause, and a
// stub repository cannot observe one; APIKeyRepoOAuthBackingSuite proves it
// against real PostgreSQL. What this catches is a future edit that opts in,
// e.g. by copying the admin user-deletion call site (the one place that must).
func TestAPIKeyService_List_DoesNotOptIntoBackingRows(t *testing.T) {
	repo := &apiKeyRepoStub{allowListByUserID: true}
	svc := &APIKeyService{apiKeyRepo: repo}

	_, _, err := svc.List(context.Background(), 7, pagination.PaginationParams{Page: 1, PageSize: 20}, APIKeyListFilters{})
	require.NoError(t, err)
	require.Len(t, repo.listByUserIDFilters, 1)
	require.False(t, repo.listByUserIDFilters[0].IncludeOAuthBacking,
		"the /keys listing must never ask the repository for backing rows")
}

// TestAPIKeyService_ListByCurrentConcurrency_DoesNotOptIntoBackingRows covers
// the OTHER listing path. sort_by=current_concurrency is a separate branch that
// loads every key unpaginated through ListAllByUserID, and it is exactly the
// kind of second code path a listing filter gets forgotten on -- so it is
// asserted to route through the shared, filtered query builder and to take the
// same safe default.
func TestAPIKeyService_ListByCurrentConcurrency_DoesNotOptIntoBackingRows(t *testing.T) {
	repo := &apiKeyRepoStub{allowListAllByUserID: true}
	svc := &APIKeyService{apiKeyRepo: repo}

	params := pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "current_concurrency"}

	_, _, err := svc.List(context.Background(), 7, params, APIKeyListFilters{})
	require.NoError(t, err)
	require.Empty(t, repo.listByUserIDCalls, "this path must not go through ListByUserID")
	require.Len(t, repo.listAllByUserIDFilters, 1)
	require.False(t, repo.listAllByUserIDFilters[0].IncludeOAuthBacking,
		"the current_concurrency listing must never ask the repository for backing rows")
}
