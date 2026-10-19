package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type fakeConcurrencyCache struct{}

func newTestConcurrencyService() *service.ConcurrencyService {
	return service.NewConcurrencyService(&fakeConcurrencyCache{})
}

func (f *fakeConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (f *fakeConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error { return nil }
func (f *fakeConcurrencyCache) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *fakeConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (f *fakeConcurrencyCache) DecrementAccountWaitCount(context.Context, int64) error { return nil }
func (f *fakeConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *fakeConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (f *fakeConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error   { return nil }
func (f *fakeConcurrencyCache) GetUserConcurrency(context.Context, int64) (int, error) { return 0, nil }
func (f *fakeConcurrencyCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (f *fakeConcurrencyCache) DecrementWaitCount(context.Context, int64) error { return nil }
func (f *fakeConcurrencyCache) GetAccountsLoadBatch(context.Context, []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	return map[int64]*service.AccountLoadInfo{}, nil
}
func (f *fakeConcurrencyCache) GetUsersLoadBatch(context.Context, []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	return map[int64]*service.UserLoadInfo{}, nil
}
func (f *fakeConcurrencyCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = 0
	}
	return result, nil
}
func (f *fakeConcurrencyCache) CleanupExpiredAccountSlots(context.Context, int64) error { return nil }
func (f *fakeConcurrencyCache) CleanupExpiredAccountSlotKeys(context.Context) error     { return nil }
func (f *fakeConcurrencyCache) CleanupStaleProcessSlots(context.Context, string) error  { return nil }
