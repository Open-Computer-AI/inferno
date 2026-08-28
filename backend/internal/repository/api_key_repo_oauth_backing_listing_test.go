package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// This file exists because of final review F-5 (the second review's F-4): the
// single most important disclosure control on this branch had coverage in
// NEITHER mandated gate.
//
// The control is one predicate in apiKeyListByUserIDQuery:
//
//	if !filters.IncludeOAuthBacking {
//	    q = q.Where(apikey.OauthClientIDIsNil())
//	}
//
// It is what keeps an OAuth backing row's live, never-expiring api_keys.key out
// of every user-facing key listing — and dto.APIKey marshals `key` verbatim, so
// losing it is a credential leak, not a cosmetic regression.
//
// Everything that could observe it lived in
// api_key_repo_oauth_backing_integration_test.go, whose build tag is
// `//go:build integration`. Mutation M1 in the whole-branch review deleted the
// predicate outright and BOTH mandated gates stayed green. Worse, the
// integration harness `os.Exit(0)`s when Docker is absent and `CI` is unset — so
// a developer machine without Docker, or a CI job that forgets `CI=1`, sees
// green with zero coverage of the property the whole design rests on.
//
// The integration suite is not replaced. It stays, and it stays the authority:
// both behaviours it pins are SQL-shaped, and this project has already shipped
// three tests that passed on an engine where the behaviour under test does not
// exist. What this file adds is a floor — the predicate cannot be deleted
// without SOMETHING failing in a gate that always runs, on every machine,
// with or without Docker.
//
// This file is deliberately UNTAGGED, which means it runs in `go test ./...`
// AND in `go test -tags unit ./...`. That is the whole point; do not add a build
// tag to it.

func newOAuthBackingListingRepo(t *testing.T) (*apiKeyRepository, *dbent.Client) {
	t.Helper()

	// A per-test database name. `cache=shared` in-memory sqlite is keyed by
	// name, so a shared literal would leak rows between tests in this package.
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &apiKeyRepository{client: client, sql: db}, client
}

const (
	listingBackingClientID = "agent:listing-floor"
	listingBackingSecret   = "sk-backing-row-secret-that-must-never-be-listed"
	listingOrdinarySecret  = "sk-an-ordinary-key-the-user-made"
)

type oauthBackingListingFixture struct {
	repo        *apiKeyRepository
	userID      int64
	ordinaryID  int64
	backingID   int64
	paginateAll pagination.PaginationParams
}

func newOAuthBackingListingFixture(t *testing.T) *oauthBackingListingFixture {
	t.Helper()
	ctx := context.Background()
	repo, client := newOAuthBackingListingRepo(t)

	u, err := client.User.Create().
		SetEmail("backing-listing@example.com").
		SetUsername("backing-listing").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	ordinary, err := client.APIKey.Create().
		SetUserID(u.ID).
		SetKey(listingOrdinarySecret).
		SetName("the user's own key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	// The backing row is created through the ent client directly, exactly as
	// OAuthBackingKeyService.createOrAdoptWinner does: apiKeyRepository.Create
	// has no SetOauthClientID at all, which is itself part of the design —
	// there is no repository path that manufactures a backing row.
	backing, err := client.APIKey.Create().
		SetUserID(u.ID).
		SetKey(listingBackingSecret).
		SetName("OAuth agent " + listingBackingClientID).
		SetOauthClientID(listingBackingClientID).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	return &oauthBackingListingFixture{
		repo:        repo,
		userID:      u.ID,
		ordinaryID:  ordinary.ID,
		backingID:   backing.ID,
		paginateAll: pagination.PaginationParams{Page: 1, PageSize: 50, SortBy: "id", SortOrder: pagination.SortOrderAsc},
	}
}

func listedAPIKeyIDs(keys []service.APIKey) []int64 {
	ids := make([]int64, 0, len(keys))
	for i := range keys {
		ids = append(ids, keys[i].ID)
	}
	return ids
}

// TestListingsHideTheOAuthBackingRow is the floor. It fails if the predicate is
// deleted, weakened to a constant, or moved somewhere a listing can bypass.
//
// Four listing shapes, because the predicate lives on the shared query and all
// four go through it — the paginated list, its COUNT, the unpaginated list, and
// the two ILIKE search columns. The COUNT is asserted separately on purpose: a
// filter applied to the page but not to the total would still report "2 keys"
// and leak the row's existence.
func TestListingsHideTheOAuthBackingRow(t *testing.T) {
	f := newOAuthBackingListingFixture(t)
	ctx := context.Background()

	t.Run("ListByUserID hides it, and so does the count", func(t *testing.T) {
		keys, page, err := f.repo.ListByUserID(ctx, f.userID, f.paginateAll, service.APIKeyListFilters{})
		require.NoError(t, err)
		require.Equal(t, []int64{f.ordinaryID}, listedAPIKeyIDs(keys))
		require.Equal(t, int64(1), page.Total, "the count must be filtered too, not just the page")
		for i := range keys {
			require.NotEqual(t, listingBackingSecret, keys[i].Key)
		}
	})

	t.Run("ListAllByUserID hides it", func(t *testing.T) {
		// The other listing path: sort_by=current_concurrency loads every key
		// unpaginated through here. Both paths share apiKeyListByUserIDQuery,
		// and this is what proves they do.
		keys, err := f.repo.ListAllByUserID(ctx, f.userID, service.APIKeyListFilters{})
		require.NoError(t, err)
		require.Equal(t, []int64{f.ordinaryID}, listedAPIKeyIDs(keys))
	})

	t.Run("search cannot reach it by its own secret", func(t *testing.T) {
		// GET /keys?search= matches api_keys.key with ILIKE, so without the
		// predicate a user could confirm a guessed secret — or a support tool
		// could echo one back.
		keys, page, err := f.repo.ListByUserID(ctx, f.userID, f.paginateAll, service.APIKeyListFilters{
			Search: listingBackingSecret,
		})
		require.NoError(t, err)
		// Assert on the IDS, never on the rows. require.Empty on []service.APIKey
		// dumps every field of every row it got, api_keys.key included, so the
		// failure message of a test about not disclosing a secret would disclose
		// it into the CI log.
		require.Empty(t, listedAPIKeyIDs(keys), "a backing row must not be reachable by searching for its own secret")
		require.Equal(t, int64(0), page.Total)
	})

	t.Run("search cannot reach it by its name", func(t *testing.T) {
		// backingKeyName() labels the row "OAuth agent <client_id>", which is a
		// guessable string.
		keys, _, err := f.repo.ListByUserID(ctx, f.userID, f.paginateAll, service.APIKeyListFilters{
			Search: "OAuth agent",
		})
		require.NoError(t, err)
		require.Empty(t, listedAPIKeyIDs(keys))
	})
}

// TestIncludeOAuthBackingIsTheOnlyWayToSeeTheBackingRow proves the predicate is
// CONDITIONAL, not unconditional — which is not decoration.
//
// Admin user deletion is the one caller that sets IncludeOAuthBacking, and it
// MUST still see backing rows: that is the sweep that stops a deleted user
// leaving live, never-expiring credentials behind. A "fix" that hard-wired the
// predicate would silence every test above and quietly break Task 5's control.
func TestIncludeOAuthBackingIsTheOnlyWayToSeeTheBackingRow(t *testing.T) {
	f := newOAuthBackingListingFixture(t)
	ctx := context.Background()

	keys, page, err := f.repo.ListByUserID(ctx, f.userID, f.paginateAll, service.APIKeyListFilters{
		IncludeOAuthBacking: true,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{f.ordinaryID, f.backingID}, listedAPIKeyIDs(keys))
	require.Equal(t, int64(2), page.Total)
}

// TestGetByIDStillFindsTheBackingRowInEveryGate guards the BOUNDARY of where the
// predicate was put.
//
// apiKeyRepository.GetByID must NOT filter: it is the billing hot path's read
// (APIKeyService.UpdateQuotaUsed re-reads the row to decide quota exhaustion),
// and the backing row is exactly the row that path must find. The user-facing
// refusal lives one layer up, in APIKeyService.managedAPIKey. Pushing the
// predicate down into GetByID would look like a tightening and would silently
// stop OAuth agents being billed.
func TestGetByIDStillFindsTheBackingRowInEveryGate(t *testing.T) {
	f := newOAuthBackingListingFixture(t)

	row, err := f.repo.GetByID(context.Background(), f.backingID)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NotNil(t, row.OAuthClientID)
	require.Equal(t, listingBackingClientID, *row.OAuthClientID)
}
