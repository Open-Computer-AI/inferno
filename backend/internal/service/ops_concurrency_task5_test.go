//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type task5OpsSnapshotCache struct {
	stubConcurrencyCacheForTest
	accounts map[int64]ConcurrencySnapshot
	users    map[int64]ConcurrencySnapshot
	err      error
}

func (c *task5OpsSnapshotCache) GetAccountConcurrencySnapshotBatch(context.Context, []int64) (map[int64]ConcurrencySnapshot, error) {
	return c.accounts, c.err
}

func (c *task5OpsSnapshotCache) GetUserConcurrencySnapshotBatch(context.Context, []int64) (map[int64]ConcurrencySnapshot, error) {
	return c.users, c.err
}

func TestTask5OpsSnapshotsExposeClassCountsAndExactAggregate(t *testing.T) {
	cache := &task5OpsSnapshotCache{
		accounts: map[int64]ConcurrencySnapshot{10: {Request: 4, Live: 2, Total: 999}},
	}
	svc := &OpsService{concurrencyService: NewConcurrencyService(cache)}

	snapshots, readError := svc.getAccountSnapshotsForOps(context.Background(), []Account{{ID: 10}})
	require.False(t, readError)
	request, live, total := concurrencySnapshotPointers(snapshots[10])
	require.Equal(t, int64(4), *request)
	require.Equal(t, int64(2), *live)
	require.Equal(t, int64(6), *total)
}

func TestTask5OpsSnapshotsReportReadFailureInsteadOfZero(t *testing.T) {
	cache := &task5OpsSnapshotCache{err: errors.New("redis unavailable")}
	svc := &OpsService{concurrencyService: NewConcurrencyService(cache)}

	snapshots, readError := svc.getAccountSnapshotsForOps(context.Background(), []Account{{ID: 10}})
	require.True(t, readError)
	require.Nil(t, snapshots)
}
