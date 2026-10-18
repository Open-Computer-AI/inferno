//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyPolicyFence_RepositoryWritersPublishTriggerGeneration(t *testing.T) {
	ctx := context.Background()
	publisher := &concurrencyCache{rdb: integrationRedis}

	testUser := mustCreateUser(t, integrationEntClient, &service.User{
		Email:       "policy-fence-" + t.Name() + "@example.com",
		Concurrency: 6,
	})
	testAccount := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:        "policy-fence-" + t.Name(),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Concurrency: 3,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM concurrency_policy_outbox WHERE (entity_type = 'user' AND entity_id = $1) OR (entity_type = 'account' AND entity_id = $2)", testUser.ID, testAccount.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", testUser.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", testAccount.ID)
		_ = integrationRedis.Del(ctx, userConcurrencyPolicyKey(testUser.ID), accountConcurrencyPolicyKey(testAccount.ID)).Err()
	})

	users := newUserRepositoryWithSQL(integrationEntClient, integrationDB, publisher)
	loadedUser, err := users.GetByID(ctx, testUser.ID)
	require.NoError(t, err)
	loadedUser.Concurrency = 8
	require.NoError(t, users.Update(ctx, loadedUser, service.UserUpdateFields{Concurrency: true}))
	require.Equal(t, int64(1), loadedUser.ConcurrencyPolicyGeneration)
	require.Equal(t, "1", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "generation").Val())
	require.Equal(t, "8", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "shared").Val())

	live, requests := 4, 2
	loadedUser.LiveConcurrency = &live
	loadedUser.RequestConcurrency = &requests
	require.NoError(t, users.Update(ctx, loadedUser, service.UserUpdateFields{ConcurrencyPolicy: true}))
	require.Equal(t, int64(2), loadedUser.ConcurrencyPolicyGeneration)
	require.Equal(t, "split", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "mode").Val())
	require.Equal(t, "2", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "request").Val())

	// A legacy batch writer must still advance the DB generation and preserve
	// the nullable pair; the split projection deliberately ignores shared.
	shared := 11
	_, err = users.BatchSetConcurrency(ctx, []int64{testUser.ID}, shared)
	require.NoError(t, err)
	loadedUser, err = users.GetByID(ctx, testUser.ID)
	require.NoError(t, err)
	require.Equal(t, shared, loadedUser.Concurrency)
	require.Equal(t, int64(3), loadedUser.ConcurrencyPolicyGeneration)
	require.NotNil(t, loadedUser.LiveConcurrency)
	require.NotNil(t, loadedUser.RequestConcurrency)
	require.Equal(t, "3", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "generation").Val())

	// The redeem adjustment is a raw SQL writer. It must be covered by the
	// same trigger and publish path without clearing the split pair.
	require.NoError(t, users.ApplyRedeemConcurrencyAdjustment(ctx, testUser.ID, -2))
	loadedUser, err = users.GetByID(ctx, testUser.ID)
	require.NoError(t, err)
	require.Equal(t, 9, loadedUser.Concurrency)
	require.Equal(t, int64(4), loadedUser.ConcurrencyPolicyGeneration)
	require.NotNil(t, loadedUser.LiveConcurrency)
	require.Equal(t, "4", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(testUser.ID), "generation").Val())

	// The generation is a DB-owned fence, not an application-controlled
	// payload. A generation-only write must be ignored and must not create a
	// second publication event.
	var pendingBefore int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM concurrency_policy_outbox
		WHERE entity_type = 'user' AND entity_id = $1
	`, testUser.ID).Scan(&pendingBefore))
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE users SET concurrency_policy_generation = 999 WHERE id = $1
	`, testUser.ID)
	require.NoError(t, err)
	loadedUser, err = users.GetByID(ctx, testUser.ID)
	require.NoError(t, err)
	require.Equal(t, int64(4), loadedUser.ConcurrencyPolicyGeneration)
	var pendingAfterGeneration int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM concurrency_policy_outbox
		WHERE entity_type = 'user' AND entity_id = $1
	`, testUser.ID).Scan(&pendingAfterGeneration))
	require.Equal(t, pendingBefore, pendingAfterGeneration)

	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil, publisher)
	loadedAccount, err := accounts.GetByID(ctx, testAccount.ID)
	require.NoError(t, err)
	loadedAccount.Concurrency = 7
	require.NoError(t, accounts.Update(ctx, loadedAccount))
	require.Equal(t, int64(1), loadedAccount.ConcurrencyPolicyGeneration)
	require.Equal(t, "7", integrationRedis.HGet(ctx, accountConcurrencyPolicyKey(testAccount.ID), "shared").Val())

	bulkValue := 9
	rows, err := accounts.BulkUpdate(ctx, []int64{testAccount.ID}, service.AccountBulkUpdate{Concurrency: &bulkValue})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	loadedAccount, err = accounts.GetByID(ctx, testAccount.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), loadedAccount.ConcurrencyPolicyGeneration)
	require.Equal(t, "2", integrationRedis.HGet(ctx, accountConcurrencyPolicyKey(testAccount.ID), "generation").Val())

	var pending int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM concurrency_policy_outbox
		WHERE entity_type = 'user' AND entity_id = $1 AND published_at IS NULL
	`, testUser.ID).Scan(&pending))
	require.Greater(t, pending, 0, "synchronous publication must leave durable events for recovery, not delete the audit trail")

	// Exercise the real SQL claim/mark implementation and the retry worker
	// against the same isolated PostgreSQL/Redis pair.
	outbox := NewConcurrencyPolicyOutboxRepository(integrationDB)
	reconciler := service.NewConcurrencyPolicyReconciler(outbox, publisher)
	require.NoError(t, reconciler.ReconcileOnce(ctx))
	var pendingAfter int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM concurrency_policy_outbox
		WHERE entity_type = 'user' AND entity_id = $1 AND published_at IS NULL
	`, testUser.ID).Scan(&pendingAfter))
	require.Equal(t, 0, pendingAfter)
}

func TestConcurrencyPolicyFence_CreateOwnsGenerationAndAccountExternalCommitHook(t *testing.T) {
	ctx := context.Background()
	publisher := &concurrencyCache{rdb: integrationRedis}

	user := &service.User{
		Email:                       "policy-create-owned-" + t.Name() + "@example.com",
		PasswordHash:                "test-password-hash",
		Role:                        service.RoleUser,
		Status:                      service.StatusActive,
		Concurrency:                 5,
		ConcurrencyPolicyGeneration: 77,
	}
	users := newUserRepositoryWithSQL(integrationEntClient, integrationDB, publisher)
	require.NoError(t, users.Create(ctx, user))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM concurrency_policy_outbox WHERE entity_type = 'user' AND entity_id = $1", user.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		_ = integrationRedis.Del(ctx, userConcurrencyPolicyKey(user.ID)).Err()
	})
	createdUser, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), createdUser.ConcurrencyPolicyGeneration)
	require.Equal(t, "0", integrationRedis.HGet(ctx, userConcurrencyPolicyKey(user.ID), "generation").Val())

	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil, publisher)
	account := &service.Account{
		Name:                        "policy-account-external-" + t.Name(),
		Platform:                    service.PlatformOpenAI,
		Type:                        service.AccountTypeOAuth,
		Concurrency:                 3,
		ConcurrencyPolicyGeneration: 88,
		Credentials:                 map[string]any{},
		Extra:                       map[string]any{},
		Status:                      service.StatusActive,
		Schedulable:                 true,
	}
	outerTx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = outerTx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, outerTx)
	require.NoError(t, accounts.CreateWithAccountGroups(txCtx, account, nil))
	// The repository cannot acknowledge Redis while the caller-owned
	// transaction is still open. The explicit hook is the post-commit fence.
	require.NoError(t, outerTx.Commit())
	require.NoError(t, accounts.PublishCurrentConcurrencyPolicy(ctx, account.ID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM concurrency_policy_outbox WHERE entity_type = 'account' AND entity_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", account.ID)
		_ = integrationRedis.Del(ctx, accountConcurrencyPolicyKey(account.ID)).Err()
	})
	createdAccount, err := accounts.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), createdAccount.ConcurrencyPolicyGeneration)
	require.Equal(t, "0", integrationRedis.HGet(ctx, accountConcurrencyPolicyKey(account.ID), "generation").Val())
}

func TestConcurrencyPolicyFence_OutboxContainsTriggerAssignedGeneration(t *testing.T) {
	ctx := context.Background()
	userFixture := mustCreateUser(t, integrationEntClient, &service.User{
		Email:       "policy-trigger-" + t.Name() + "@example.com",
		Concurrency: 3,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM concurrency_policy_outbox WHERE entity_type = 'user' AND entity_id = $1", userFixture.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userFixture.ID)
	})

	var before int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT concurrency_policy_generation FROM users WHERE id = $1", userFixture.ID).Scan(&before))
	_, err := integrationDB.ExecContext(ctx, "UPDATE users SET concurrency = $1 WHERE id = $2", 10, userFixture.ID)
	require.NoError(t, err)

	var generation int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT concurrency_policy_generation FROM users WHERE id = $1", userFixture.ID).Scan(&generation))
	require.Equal(t, before+1, generation)
	var outboxGeneration int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT generation
		FROM concurrency_policy_outbox
		WHERE entity_type = 'user' AND entity_id = $1 AND generation = $2
	`, userFixture.ID, generation).Scan(&outboxGeneration))
	require.Equal(t, generation, outboxGeneration)
}

func TestConcurrencyPolicyFence_InvalidPairsRemainRejectedByDatabase(t *testing.T) {
	ctx := context.Background()
	userFixture := mustCreateUser(t, integrationEntClient, &service.User{
		Email:       "policy-invalid-" + t.Name() + "@example.com",
		Concurrency: 3,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM concurrency_policy_outbox WHERE entity_type = 'user' AND entity_id = $1", userFixture.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userFixture.ID)
	})

	_, err := integrationDB.ExecContext(ctx, "UPDATE users SET live_concurrency = 4, request_concurrency = NULL WHERE id = $1", userFixture.ID)
	require.Error(t, err)
	var live, requests sql.NullInt64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT live_concurrency, request_concurrency FROM users WHERE id = $1", userFixture.ID).Scan(&live, &requests))
	require.False(t, live.Valid, "invalid partial pair was committed")
	require.False(t, requests.Valid, "invalid partial pair was committed")
}
