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

// The property, stated directly: whatever an admin path returns for a backing
// row, the secret is not in it.
func TestAdminPathsNeverHandBackTheBackingSecret(t *testing.T) {
	svc := adminSvcWithRow(adminBackingRow())
	ctx := context.Background()
	target := int64(9)

	// Only the paths that return without touching collaborators this stub
	// does not provide. The rebind path has its own test; exercising it here
	// under a disabled guard reaches a nil groupRepo and panics, which would
	// make this test "fail" for a reason that says nothing about disclosure.
	_ = target
	result1, _ := svc.AdminUpdateAPIKeyGroupID(ctx, 42, nil)
	got, _ := svc.AdminResetAPIKeyRateLimitUsage(ctx, 42)

	for i, key := range []*APIKey{
		keyOf(result1), got,
	} {
		if key == nil {
			continue
		}
		require.NotEqual(t, adminBackingSecret, key.Key,
			"admin path %d handed back the backing row's live credential", i)
	}
}

func keyOf(r *AdminUpdateAPIKeyGroupIDResult) *APIKey {
	if r == nil {
		return nil
	}
	return r.APIKey
}
