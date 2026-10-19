package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func apiKeyDTOsWithConcurrency(ctx context.Context, keys []service.APIKey, concurrencyService *service.ConcurrencyService) []dto.APIKey {
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	if concurrencyService == nil || len(keys) == 0 {
		return out
	}

	keyIDs := make([]int64, len(keys))
	for i := range keys {
		keyIDs[i] = keys[i].ID
	}
	snapshots, snapshotErr := concurrencyService.GetAPIKeyConcurrencySnapshotBatch(ctx, keyIDs)
	for i := range out {
		if snapshotErr != nil {
			markAPIKeyConcurrencyUnavailable(&out[i])
			continue
		}
		snapshot, ok := snapshots[keyIDs[i]]
		if !ok {
			markAPIKeyConcurrencyUnavailable(&out[i])
			continue
		}
		request := snapshot.Request
		live := snapshot.Live
		total := request + live
		out[i].CurrentConcurrency = &total
		out[i].CurrentLiveConcurrency = &live
		out[i].CurrentRequestConcurrency = &request
		out[i].ConcurrencyReadError = ""
	}
	return out
}

func markAPIKeyConcurrencyUnavailable(key *dto.APIKey) {
	if key == nil {
		return
	}
	key.CurrentConcurrency = nil
	key.CurrentLiveConcurrency = nil
	key.CurrentRequestConcurrency = nil
	key.ConcurrencyReadError = "unavailable"
}
