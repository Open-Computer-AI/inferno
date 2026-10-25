//go:build unit

// Copied from internal/handler/concurrency_test_helpers_test.go (stage split concurrency)
// so upstream route tests get an authoritative, always-admitting concurrency fixture.
package routes

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type routesFakeConcurrencyCache struct{}

func newRoutesTestConcurrencyService() *service.ConcurrencyService {
	return service.NewConcurrencyService(&routesFakeConcurrencyCache{})
}

func (f *routesFakeConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (f *routesFakeConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error { return nil }
func (f *routesFakeConcurrencyCache) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *routesFakeConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (f *routesFakeConcurrencyCache) DecrementAccountWaitCount(context.Context, int64) error { return nil }
func (f *routesFakeConcurrencyCache) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *routesFakeConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (f *routesFakeConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error   { return nil }
func (f *routesFakeConcurrencyCache) GetUserConcurrency(context.Context, int64) (int, error) { return 0, nil }
func (f *routesFakeConcurrencyCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (f *routesFakeConcurrencyCache) DecrementWaitCount(context.Context, int64) error { return nil }
func (f *routesFakeConcurrencyCache) GetAccountsLoadBatch(context.Context, []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	return map[int64]*service.AccountLoadInfo{}, nil
}
func (f *routesFakeConcurrencyCache) GetUsersLoadBatch(context.Context, []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	return map[int64]*service.UserLoadInfo{}, nil
}
func (f *routesFakeConcurrencyCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = 0
	}
	return result, nil
}
func (f *routesFakeConcurrencyCache) CleanupExpiredAccountSlots(context.Context, int64) error { return nil }
func (f *routesFakeConcurrencyCache) CleanupExpiredAccountSlotKeys(context.Context) error     { return nil }
func (f *routesFakeConcurrencyCache) CleanupStaleProcessSlots(context.Context, string) error  { return nil }
