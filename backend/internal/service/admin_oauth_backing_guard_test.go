//go:build unit

// The admin half of Task 5's "a backing row is invisible" rule.
//
// APIKeyService's user-facing paths were guarded first, and it is easy to stop
// there — but /api/v1/admin/api-keys/:id reaches the same rows through a
// different service, and its handler serialises the result through dto.APIKey,
// whose Key field is `json:"key"`. Both methods below return the row.
//
// AdminUpdateAPIKeyGroupID is the sharper of the two: with groupID nil it
// returns BEFORE making any change, so an admin PUTting a backing row's id with
// an empty body was handed that row's live api_keys.key. No mutation, no
// listing, no enumeration of anything else — one request.

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// adminBackingStubRepo returns one row for any id. The row is a backing row
// when clientID is non-empty.
type adminBackingStubRepo struct {
	APIKeyRepository
	row *APIKey
}

func (r *adminBackingStubRepo) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	return r.row, nil
}

func (r *adminBackingStubRepo) Update(_ context.Context, _ *APIKey, _ APIKeyUpdateFields) error {
	return nil
}

func adminSvcWithRow(row *APIKey) *adminServiceImpl {
	return &adminServiceImpl{apiKeyRepo: &adminBackingStubRepo{row: row}}
}

const adminBackingSecret = "sk-admin-reachable-backing-secret"

func adminBackingRow() *APIKey {
	cid := "agent:deadbeef"
	return &APIKey{ID: 42, UserID: 7, Key: adminBackingSecret, OAuthClientID: &cid}
}

func adminOrdinaryRow() *APIKey {
	return &APIKey{ID: 43, UserID: 7, Key: "sk-ordinary-key"}
}

// The nil-groupID early return is the one that leaked without mutating.
func TestAdminUpdateAPIKeyGroupID_RefusesBackingRowOnTheNoOpPath(t *testing.T) {
	svc := adminSvcWithRow(adminBackingRow())

	result, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 42, nil)

	require.Error(t, err, "a nil groupID returns the row before changing anything; "+
		"without the guard that is a one-request disclosure of a live credential")
	require.ErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable)
	require.Nil(t, result, "no result means nothing for the handler to serialise")
}

func TestAdminUpdateAPIKeyGroupID_RefusesRebindingABackingRow(t *testing.T) {
	svc := adminSvcWithRow(adminBackingRow())
	target := int64(9)

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 42, &target)

	require.ErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable,
		"apiKey.Group is the only routing input; rebinding it silently re-points "+
			"a running agent's inference at a different upstream")
}

func TestAdminResetAPIKeyRateLimitUsage_RefusesBackingRow(t *testing.T) {
	svc := adminSvcWithRow(adminBackingRow())

	got, err := svc.AdminResetAPIKeyRateLimitUsage(context.Background(), 42)

	require.ErrorIs(t, err, ErrOAuthBackingKeyUnmodifiable)
	require.Nil(t, got)
}

// The guards must not fire on ordinary keys, or they have broken admin key
// management for every standalone Inferno customer.
func TestAdminPathsStillReachOrdinaryKeys(t *testing.T) {
	svc := adminSvcWithRow(adminOrdinaryRow())

	result, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 43, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(43), result.APIKey.ID)

	got, err := svc.AdminResetAPIKeyRateLimitUsage(context.Background(), 43)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Zero(t, got.Usage5h, "the reset must still do its job")
}

// TestAdminPathsNeverHandBackTheBackingSecret states the property over the SET
// of admin paths, rather than one path at a time.
//
// It was vacuous, and that is worth recording rather than quietly fixing (m-13):
// while the guards hold, every call returns a nil row, so the old version's loop
// body never executed and the test asserted NOTHING. It would have failed if a
// guard were removed -- the row would come back and require.NotEqual would fire
// -- but a test that asserts nothing in the passing case is a test whose green
// means nothing, and this project has twelve recorded instances of that class.
//
// It now asserts in BOTH states, and the two assertions are different claims:
//
//   - Today's behaviour: every admin path refuses a backing row outright, with
//     ErrOAuthBackingKeyUnmodifiable and no row at all. Remove a guard and this
//     fails immediately, on the assertion, naming disclosure.
//   - Future-proofing: IF a path ever returns a row alongside its error, that
//     row must not carry the secret. Vacuous today by construction, and labelled
//     as such instead of being presented as the point.
//
// It is table-driven so that adding a third admin read path means adding a row
// here, which is the thing the four single-path tests above cannot make anyone
// do.
func TestAdminPathsNeverHandBackTheBackingSecret(t *testing.T) {
	svc := adminSvcWithRow(adminBackingRow())
	ctx := context.Background()

	// Only paths that return without touching collaborators this stub does not
	// provide. AdminUpdateAPIKeyGroupID with a non-nil target has its own test;
	// exercising it here under a disabled guard reaches a nil groupRepo and
	// panics, which would make this test "fail" for a reason that says nothing
	// about disclosure -- and a mutation that kills a test by panicking has
	// proved nothing.
	updateResult, updateErr := svc.AdminUpdateAPIKeyGroupID(ctx, 42, nil)
	resetResult, resetErr := svc.AdminResetAPIKeyRateLimitUsage(ctx, 42)

	paths := []struct {
		name string
		key  *APIKey
		err  error
	}{
		{"AdminUpdateAPIKeyGroupID(nil groupID)", keyOf(updateResult), updateErr},
		{"AdminResetAPIKeyRateLimitUsage", resetResult, resetErr},
	}

	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			require.ErrorIs(t, p.err, ErrOAuthBackingKeyUnmodifiable,
				"an admin path must refuse a backing row outright; without the guard it returns the "+
					"row, and the handler serialises it through dto.APIKey whose Key field is json:\"key\"")
			require.Nil(t, p.key,
				"no row means nothing for the handler to serialise")
			if p.key != nil {
				// Unreachable while the assertion above holds. Kept as the
				// backstop for a future path that returns a row WITH an error.
				require.NotEqual(t, adminBackingSecret, p.key.Key,
					"admin path handed back the backing row's live credential")
			}
		})
	}
}

func keyOf(r *AdminUpdateAPIKeyGroupIDResult) *APIKey {
	if r == nil {
		return nil
	}
	return r.APIKey
}
