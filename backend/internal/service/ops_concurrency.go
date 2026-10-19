package service

import (
	"context"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	opsAccountsPageSize          = 100
	opsConcurrencyBatchChunkSize = 200
)

type opsAccountStatsRepository interface {
	ListOpsAccountsForStats(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error)
}

func (s *OpsService) listAllAccountsForOps(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return []Account{}, nil
	}
	if repo, ok := s.accountRepo.(opsAccountStatsRepository); ok {
		return repo.ListOpsAccountsForStats(ctx, platformFilter, groupIDFilter)
	}

	out := make([]Account, 0, 128)
	page := 1
	groupID := int64(0)
	if groupIDFilter != nil {
		groupID = *groupIDFilter
	}
	for {
		accounts, pageInfo, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: opsAccountsPageSize,
		}, platformFilter, "", "", "", groupID, "")
		if err != nil {
			return nil, err
		}
		if len(accounts) == 0 {
			break
		}

		out = append(out, accounts...)
		if pageInfo != nil && int64(len(out)) >= pageInfo.Total {
			break
		}
		if len(accounts) < opsAccountsPageSize {
			break
		}

		page++
		if page > 10_000 {
			log.Printf("[Ops] listAllAccountsForOps: aborting after too many pages (platform=%q)", platformFilter)
			break
		}
	}

	return out, nil
}

func (s *OpsService) getAccountsLoadMapBestEffort(ctx context.Context, accounts []Account) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil {
		return map[int64]*AccountLoadInfo{}
	}
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}
	}

	// De-duplicate IDs (and keep the max concurrency to avoid under-reporting).
	unique := make(map[int64]int, len(accounts))
	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
		}
		lf := acc.EffectiveLoadFactor()
		if prev, ok := unique[acc.ID]; !ok || lf > prev {
			unique[acc.ID] = lf
		}
	}

	batch := make([]AccountWithConcurrency, 0, len(unique))
	for id, maxConc := range unique {
		batch = append(batch, AccountWithConcurrency{
			ID:             id,
			MaxConcurrency: maxConc,
		})
	}

	out := make(map[int64]*AccountLoadInfo, len(batch))
	for i := 0; i < len(batch); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(batch) {
			end = len(batch)
		}
		part, err := s.concurrencyService.GetAccountsLoadBatch(ctx, batch[i:end])
		if err != nil {
			// Best-effort: return zeros rather than failing the ops UI.
			log.Printf("[Ops] GetAccountsLoadBatch failed: %v", err)
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}

	return out
}

// GetConcurrencyStats returns real-time concurrency usage aggregated by platform/group/account.
//
// Optional filters:
// - platformFilter: only include accounts in that platform (best-effort reduces DB load)
// - groupIDFilter: only include accounts that belong to that group
func (s *OpsService) GetConcurrencyStats(
	ctx context.Context,
	platformFilter string,
	groupIDFilter *int64,
) (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, nil, nil, err
	}

	accounts, err := s.listAllAccountsForOps(ctx, platformFilter, groupIDFilter)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	collectedAt := time.Now()
	loadMap := s.getAccountsLoadMapBestEffort(ctx, accounts)
	snapshots, snapshotReadError := s.getAccountSnapshotsForOps(ctx, accounts)

	platform := make(map[string]*PlatformConcurrencyInfo)
	group := make(map[int64]*GroupConcurrencyInfo)
	account := make(map[int64]*AccountConcurrencyInfo)
	platformSnapshots := make(map[string]ConcurrencySnapshot)
	platformSnapshotKnown := make(map[string]bool)
	groupSnapshots := make(map[int64]ConcurrencySnapshot)
	groupSnapshotKnown := make(map[int64]bool)

	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
		}

		var matchedGroup *Group
		if groupIDFilter != nil && *groupIDFilter > 0 {
			for _, grp := range acc.Groups {
				if grp == nil || grp.ID <= 0 {
					continue
				}
				if grp.ID == *groupIDFilter {
					matchedGroup = grp
					break
				}
			}
			// Group filter provided: skip accounts not in that group.
			if matchedGroup == nil {
				continue
			}
		}

		load := loadMap[acc.ID]
		currentInUse := int64(0)
		waiting := int64(0)
		if load != nil {
			currentInUse = int64(load.CurrentConcurrency)
			waiting = int64(load.WaitingCount)
		}
		snapshot, snapshotKnown := snapshots[acc.ID]
		if snapshotKnown {
			currentInUse = int64(snapshot.Request + snapshot.Live)
		}

		// Account-level view picks one display group (the first group).
		displayGroupID := int64(0)
		displayGroupName := ""
		if matchedGroup != nil {
			displayGroupID = matchedGroup.ID
			displayGroupName = matchedGroup.Name
		} else if len(acc.Groups) > 0 && acc.Groups[0] != nil {
			displayGroupID = acc.Groups[0].ID
			displayGroupName = acc.Groups[0].Name
		}

		if _, ok := account[acc.ID]; !ok {
			info := &AccountConcurrencyInfo{
				AccountID:      acc.ID,
				AccountName:    acc.Name,
				Platform:       acc.Platform,
				GroupID:        displayGroupID,
				GroupName:      displayGroupName,
				CurrentInUse:   currentInUse,
				MaxCapacity:    int64(acc.Concurrency),
				WaitingInQueue: waiting,
			}
			if info.MaxCapacity > 0 {
				info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
			}
			if snapshotKnown {
				info.CurrentRequestConcurrency, info.CurrentLiveConcurrency, info.CurrentConcurrency = concurrencySnapshotPointers(snapshot)
			} else if snapshotReadError || s.concurrencyService != nil {
				info.ConcurrencyReadError = "unavailable"
			}
			account[acc.ID] = info
		}

		// Platform aggregation.
		if acc.Platform != "" {
			if _, ok := platform[acc.Platform]; !ok {
				platform[acc.Platform] = &PlatformConcurrencyInfo{
					Platform: acc.Platform,
				}
			}
			p := platform[acc.Platform]
			p.MaxCapacity += int64(acc.Concurrency)
			p.CurrentInUse += currentInUse
			p.WaitingInQueue += waiting
			if s.concurrencyService != nil {
				if _, seen := platformSnapshotKnown[acc.Platform]; !seen {
					platformSnapshotKnown[acc.Platform] = true
				}
				if snapshotKnown && !snapshotReadError && platformSnapshotKnown[acc.Platform] {
					platformSnapshots[acc.Platform] = addConcurrencySnapshots(platformSnapshots[acc.Platform], snapshot)
				} else {
					platformSnapshotKnown[acc.Platform] = false
				}
			}
		}

		// Group aggregation (one account may contribute to multiple groups).
		if matchedGroup != nil {
			grp := matchedGroup
			if _, ok := group[grp.ID]; !ok {
				group[grp.ID] = &GroupConcurrencyInfo{
					GroupID:   grp.ID,
					GroupName: grp.Name,
					Platform:  grp.Platform,
				}
			}
			g := group[grp.ID]
			if g.GroupName == "" && grp.Name != "" {
				g.GroupName = grp.Name
			}
			if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
				// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
				g.Platform = ""
			}
			g.MaxCapacity += int64(acc.Concurrency)
			g.CurrentInUse += currentInUse
			g.WaitingInQueue += waiting
			if s.concurrencyService != nil {
				if _, seen := groupSnapshotKnown[grp.ID]; !seen {
					groupSnapshotKnown[grp.ID] = true
				}
				if snapshotKnown && !snapshotReadError && groupSnapshotKnown[grp.ID] {
					groupSnapshots[grp.ID] = addConcurrencySnapshots(groupSnapshots[grp.ID], snapshot)
				} else {
					groupSnapshotKnown[grp.ID] = false
				}
			}
		} else {
			for _, grp := range acc.Groups {
				if grp == nil || grp.ID <= 0 {
					continue
				}
				if _, ok := group[grp.ID]; !ok {
					group[grp.ID] = &GroupConcurrencyInfo{
						GroupID:   grp.ID,
						GroupName: grp.Name,
						Platform:  grp.Platform,
					}
				}
				g := group[grp.ID]
				if g.GroupName == "" && grp.Name != "" {
					g.GroupName = grp.Name
				}
				if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
					// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
					g.Platform = ""
				}
				g.MaxCapacity += int64(acc.Concurrency)
				g.CurrentInUse += currentInUse
				g.WaitingInQueue += waiting
				if s.concurrencyService != nil {
					if _, seen := groupSnapshotKnown[grp.ID]; !seen {
						groupSnapshotKnown[grp.ID] = true
					}
					if snapshotKnown && !snapshotReadError && groupSnapshotKnown[grp.ID] {
						groupSnapshots[grp.ID] = addConcurrencySnapshots(groupSnapshots[grp.ID], snapshot)
					} else {
						groupSnapshotKnown[grp.ID] = false
					}
				}
			}
		}
	}

	if s.concurrencyService != nil {
		for platformName, info := range platform {
			if !snapshotReadError && platformSnapshotKnown[platformName] {
				info.CurrentRequestConcurrency, info.CurrentLiveConcurrency, info.CurrentConcurrency = concurrencySnapshotPointers(platformSnapshots[platformName])
			} else {
				info.ConcurrencyReadError = "unavailable"
			}
		}
		for groupID, info := range group {
			if !snapshotReadError && groupSnapshotKnown[groupID] {
				info.CurrentRequestConcurrency, info.CurrentLiveConcurrency, info.CurrentConcurrency = concurrencySnapshotPointers(groupSnapshots[groupID])
			} else {
				info.ConcurrencyReadError = "unavailable"
			}
		}
	}

	for _, info := range platform {
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
		}
	}
	for _, info := range group {
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
		}
	}

	return platform, group, account, &collectedAt, nil
}

// listAllActiveUsersForOps returns all active users with their concurrency settings.
func (s *OpsService) listAllActiveUsersForOps(ctx context.Context) ([]User, error) {
	if s == nil || s.userRepo == nil {
		return []User{}, nil
	}

	out := make([]User, 0, 128)
	page := 1
	for {
		users, pageInfo, err := s.userRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: opsAccountsPageSize,
		}, UserListFilters{
			Status: StatusActive,
		})
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			break
		}

		out = append(out, users...)
		if pageInfo != nil && int64(len(out)) >= pageInfo.Total {
			break
		}
		if len(users) < opsAccountsPageSize {
			break
		}

		page++
		if page > 10_000 {
			log.Printf("[Ops] listAllActiveUsersForOps: aborting after too many pages")
			break
		}
	}

	return out, nil
}

// getUsersLoadMapBestEffort returns user load info for the given users.
func (s *OpsService) getUsersLoadMapBestEffort(ctx context.Context, users []User) map[int64]*UserLoadInfo {
	if s == nil || s.concurrencyService == nil {
		return map[int64]*UserLoadInfo{}
	}
	if len(users) == 0 {
		return map[int64]*UserLoadInfo{}
	}

	// De-duplicate IDs (and keep the max concurrency to avoid under-reporting).
	unique := make(map[int64]int, len(users))
	for _, u := range users {
		if u.ID <= 0 {
			continue
		}
		if prev, ok := unique[u.ID]; !ok || u.Concurrency > prev {
			unique[u.ID] = u.Concurrency
		}
	}

	batch := make([]UserWithConcurrency, 0, len(unique))
	for id, maxConc := range unique {
		batch = append(batch, UserWithConcurrency{
			ID:             id,
			MaxConcurrency: maxConc,
		})
	}

	out := make(map[int64]*UserLoadInfo, len(batch))
	for i := 0; i < len(batch); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(batch) {
			end = len(batch)
		}
		part, err := s.concurrencyService.GetUsersLoadBatch(ctx, batch[i:end])
		if err != nil {
			// Best-effort: return zeros rather than failing the ops UI.
			log.Printf("[Ops] GetUsersLoadBatch failed: %v", err)
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}

	return out
}

// getAccountSnapshotsForOps reads class-aware occupancy in bounded batches.
// The second return value means a strict read was attempted and failed; the
// caller uses it to expose unknown diagnostics instead of zeroes.
func (s *OpsService) getAccountSnapshotsForOps(ctx context.Context, accounts []Account) (map[int64]ConcurrencySnapshot, bool) {
	if s == nil || s.concurrencyService == nil || len(accounts) == 0 {
		return map[int64]ConcurrencySnapshot{}, false
	}
	ids := make([]int64, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if account.ID <= 0 {
			continue
		}
		if _, ok := seen[account.ID]; ok {
			continue
		}
		seen[account.ID] = struct{}{}
		ids = append(ids, account.ID)
	}
	result := make(map[int64]ConcurrencySnapshot, len(ids))
	for i := 0; i < len(ids); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		part, err := s.concurrencyService.GetAccountConcurrencySnapshotBatch(ctx, ids[i:end])
		if err != nil {
			return nil, true
		}
		for id, snapshot := range part {
			result[id] = snapshot
		}
	}
	return result, false
}

func (s *OpsService) getUserSnapshotsForOps(ctx context.Context, users []User) (map[int64]ConcurrencySnapshot, bool) {
	if s == nil || s.concurrencyService == nil || len(users) == 0 {
		return map[int64]ConcurrencySnapshot{}, false
	}
	ids := make([]int64, 0, len(users))
	seen := make(map[int64]struct{}, len(users))
	for _, user := range users {
		if user.ID <= 0 {
			continue
		}
		if _, ok := seen[user.ID]; ok {
			continue
		}
		seen[user.ID] = struct{}{}
		ids = append(ids, user.ID)
	}
	result := make(map[int64]ConcurrencySnapshot, len(ids))
	for i := 0; i < len(ids); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		part, err := s.concurrencyService.GetUserConcurrencySnapshotBatch(ctx, ids[i:end])
		if err != nil {
			return nil, true
		}
		for id, snapshot := range part {
			result[id] = snapshot
		}
	}
	return result, false
}

func concurrencySnapshotPointers(snapshot ConcurrencySnapshot) (request, live, total *int64) {
	requestValue := int64(snapshot.Request)
	liveValue := int64(snapshot.Live)
	totalValue := requestValue + liveValue
	return &requestValue, &liveValue, &totalValue
}

func addConcurrencySnapshots(left, right ConcurrencySnapshot) ConcurrencySnapshot {
	left.Request += right.Request
	left.Live += right.Live
	left.Total = left.Request + left.Live
	return left
}

// GetUserConcurrencyStats returns real-time concurrency usage for all active users.
func (s *OpsService) GetUserConcurrencyStats(ctx context.Context) (map[int64]*UserConcurrencyInfo, *time.Time, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, err
	}

	users, err := s.listAllActiveUsersForOps(ctx)
	if err != nil {
		return nil, nil, err
	}

	collectedAt := time.Now()
	loadMap := s.getUsersLoadMapBestEffort(ctx, users)
	snapshots, snapshotReadError := s.getUserSnapshotsForOps(ctx, users)

	result := make(map[int64]*UserConcurrencyInfo)

	for _, u := range users {
		if u.ID <= 0 {
			continue
		}

		load := loadMap[u.ID]
		currentInUse := int64(0)
		waiting := int64(0)
		if load != nil {
			currentInUse = int64(load.CurrentConcurrency)
			waiting = int64(load.WaitingCount)
		}
		snapshot, snapshotKnown := snapshots[u.ID]
		if snapshotKnown {
			currentInUse = int64(snapshot.Request + snapshot.Live)
		}

		// Skip users with no concurrency activity
		if currentInUse == 0 && waiting == 0 && !snapshotReadError && !snapshotKnown {
			continue
		}

		info := &UserConcurrencyInfo{
			UserID:         u.ID,
			UserEmail:      u.Email,
			Username:       u.Username,
			CurrentInUse:   currentInUse,
			MaxCapacity:    int64(u.Concurrency),
			WaitingInQueue: waiting,
		}
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
		}
		if snapshotKnown && !snapshotReadError {
			info.CurrentRequestConcurrency, info.CurrentLiveConcurrency, info.CurrentConcurrency = concurrencySnapshotPointers(snapshot)
		} else if snapshotReadError || s.concurrencyService != nil {
			info.ConcurrencyReadError = "unavailable"
		}
		result[u.ID] = info
	}

	return result, &collectedAt, nil
}
