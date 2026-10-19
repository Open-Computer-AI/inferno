package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown
// 验证 compact 调度时显式支持 (tier=2) 优先于未探测 (tier=1)。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91001)
	accounts := []Account{
		{
			ID:          71001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{}, // unknown
		},
		{
			ID:          71002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": true}, // tier=2
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71002), selection.Account.ID, "compact-supported account should win over unknown")
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported
// 验证 force_off / 已探测不支持 (tier=0) 的账号不会被 compact 请求选中。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91002)
	accounts := []Account{
		{
			ID:          71010,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_mode": OpenAICompactModeForceOff},
		},
		{
			ID:          71011,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": false},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableCompactAccounts), "compact-only accounts should rejected explicitly unsupported and return compact error")
	require.Nil(t, selection)
}

func newOpenAICompactionSchedulerTestService(accounts []Account, advanced bool) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	if advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	}
	return svc
}

func TestOpenAIGatewayService_SelectAccountWithSchedulerForLiveNeverUsesOrdinarySlotOrWaitPlan(t *testing.T) {
	for _, tc := range []struct {
		name      string
		advanced  bool
		loadBatch bool
		sticky    bool
		forceFull bool
	}{
		{name: "legacy_direct", loadBatch: false},
		{name: "legacy_sticky", loadBatch: false, sticky: true},
		{name: "legacy_load_batch", loadBatch: true, forceFull: true},
		{name: "advanced_direct", advanced: true},
		{name: "advanced_sticky", advanced: true, sticky: true},
		{name: "advanced_load_batch", advanced: true, loadBatch: true, forceFull: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

			const sessionHash = "live-scheduler-sticky"
			groupID := int64(91099)
			accountID := int64(71999)
			backupID := int64(71998)
			acquiredIDs := make([]int64, 0)
			concurrencyCache := &schedulerTestConcurrencyCache{
				acquiredIDs: &acquiredIDs,
			}
			if tc.forceFull {
				concurrencyCache.loadMap = map[int64]*AccountLoadInfo{
					accountID: {AccountID: accountID, LoadRate: 100, CurrentConcurrency: 1},
					backupID:  {AccountID: backupID, LoadRate: 0, CurrentConcurrency: 0},
				}
				concurrencyCache.acquireResults = map[int64]bool{accountID: false}
			}
			gatewayCache := &schedulerTestGatewayCache{}
			if tc.sticky {
				gatewayCache.sessionBindings = map[string]int64{sessionHash: accountID}
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = tc.loadBatch
			accounts := []Account{{
				ID:          accountID,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{"access_token": "live-test-token"},
			}}
			expectedAccountID := accountID
			if tc.forceFull {
				accounts = append(accounts, Account{
					ID:          backupID,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeOAuth,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Credentials: map[string]any{"access_token": "live-backup-token"},
				})
				expectedAccountID = backupID
			}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:              gatewayCache,
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(concurrencyCache),
			}
			if tc.advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}

			selection, _, err := svc.SelectAccountWithSchedulerForLive(
				context.Background(), &groupID, "", sessionHashIf(tc.sticky, sessionHash), "gpt-live", nil,
				OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityLive, false, false, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.True(t, selection.Acquired)
			require.Nil(t, selection.WaitPlan, "Live selection must never return an ordinary wait plan")
			require.Equal(t, expectedAccountID, selection.Account.ID)
			require.Empty(t, acquiredIDs, "Live selection must not call ordinary account-slot admission")
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func sessionHashIf(enabled bool, value string) string {
	if enabled {
		return value
	}
	return ""
}

type schedulerClassAwareConcurrencyCache struct {
	schedulerTestConcurrencyCache
	loadRequests *[]AccountWithConcurrency
	acquireMaxes *[]int
}

func (c *schedulerClassAwareConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if c.loadRequests != nil {
		*c.loadRequests = append(*c.loadRequests, accounts...)
	}
	return c.schedulerTestConcurrencyCache.GetAccountsLoadBatch(ctx, accounts)
}

func (c *schedulerClassAwareConcurrencyCache) AcquireAccountSlot(_ context.Context, _ int64, max int, _ string) (bool, error) {
	if c.acquireMaxes != nil {
		*c.acquireMaxes = append(*c.acquireMaxes, max)
	}
	return true, nil
}

func TestOpenAISchedulerOrdinarySplitUsesRequestLimitAcrossLegacyAndAdvanced(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

			live, requests := 4, 2
			loadRequests := make([]AccountWithConcurrency, 0)
			acquireMaxes := make([]int, 0)
			cache := &schedulerClassAwareConcurrencyCache{
				schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{},
				loadRequests:                  &loadRequests,
				acquireMaxes:                  &acquireMaxes,
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = false
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{{
					ID:                 72001,
					Platform:           PlatformOpenAI,
					Type:               AccountTypeOAuth,
					Status:             StatusActive,
					Schedulable:        true,
					Concurrency:        1,
					LiveConcurrency:    &live,
					RequestConcurrency: &requests,
					Credentials:        map[string]any{"access_token": "request-test-token"},
				}}},
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(cache),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}

			groupID := int64(91098)
			selection, _, err := svc.SelectAccountWithScheduler(
				context.Background(), &groupID, "", "", "gpt-request", nil, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.True(t, selection.Acquired)
			if advanced {
				require.NotEmpty(t, loadRequests)
				require.Equal(t, ConcurrencyClassRequest, loadRequests[0].AdmissionClass)
				require.True(t, loadRequests[0].SplitPolicy)
				require.Equal(t, requests, loadRequests[0].MaxConcurrency)
			}
			// The legacy path does not use a load batch, but both scheduler modes
			// must pass the independent request cap to the ordinary slot acquire.
			if !advanced {
				require.Equal(t, []int{requests}, acquireMaxes)
			}
		})
	}
}

func TestOpenAISchedulerLiveLoadRequestsUseLiveClassAcrossLegacyAndAdvanced(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "advanced"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

			live, requests := 4, 2
			loadRequests := make([]AccountWithConcurrency, 0)
			cache := &schedulerClassAwareConcurrencyCache{
				schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{},
				loadRequests:                  &loadRequests,
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = true
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{{
					ID: 72002, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1,
					LiveConcurrency: &live, RequestConcurrency: &requests,
					Credentials: map[string]any{"access_token": "live-class-test-token"},
				}}},
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(cache),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}

			groupID := int64(91097)
			selection, _, err := svc.SelectAccountWithSchedulerForLive(
				context.Background(), &groupID, "", "", "gpt-live", nil,
				OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityLive, false, false, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.True(t, selection.Acquired)
			require.Nil(t, selection.WaitPlan)
			require.NotEmpty(t, loadRequests)
			require.Equal(t, ConcurrencyClassLive, loadRequests[0].AdmissionClass)
			require.True(t, loadRequests[0].SplitPolicy)
			require.Equal(t, live, loadRequests[0].MaxConcurrency)
		})
	}
}

func selectOpenAICompactionSchedulerTestAccount(t *testing.T, svc *OpenAIGatewayService, groupID int64, requireCompact bool) (*AccountSelectionResult, error) {
	t.Helper()
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(),
		&groupID,
		"",
		"",
		"gpt-5.6-sol",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		requireCompact,
		false,
		false,
	)
	return selection, err
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_NativeCompactionIgnoresLegacyCompactProbe(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71012,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_supported":   false,
					"openai_responses_supported": true,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91007, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(71012), selection.Account.ID)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_NativeCompactionAllowsForceOff(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71013,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_mode":        OpenAICompactModeForceOff,
					"openai_responses_supported": true,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91008, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(71013), selection.Account.ID)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_NativeCompactionRequiresResponsesCapability(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{{
				ID:          71014,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Extra: map[string]any{
					"openai_compact_mode":        OpenAICompactModeForceOn,
					"openai_responses_supported": false,
				},
			}}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91009, false)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.NotErrorIs(t, err, ErrNoAvailableCompactAccounts)
			require.NotContains(t, err.Error(), "/responses/compact")
			require.Nil(t, selection)
		})
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LegacyCompactionKeepsCompactEligibility(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_scheduler", true: "advanced_scheduler"}[advanced], func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc := newOpenAICompactionSchedulerTestService([]Account{
				{
					ID:          71015,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Extra: map[string]any{
						"openai_compact_supported":   false,
						"openai_responses_supported": true,
					},
				},
				{
					ID:          71016,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Extra: map[string]any{
						"openai_compact_mode":        OpenAICompactModeForceOff,
						"openai_responses_supported": true,
					},
				},
			}, advanced)

			selection, err := selectOpenAICompactionSchedulerTestAccount(t, svc, 91010, true)
			require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
			require.Contains(t, err.Error(), "/responses/compact")
			require.Nil(t, selection)
		})
	}
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRequiresResponsesCapability
// prevents a confirmed Chat Completions-only API key from silently dropping the
// compaction trigger through the Responses-to-Chat fallback.
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRequiresResponsesCapability(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91005)
	accounts := []Account{{
		ID:          71050,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_compact_supported":   true,
			"openai_responses_supported": false,
		},
	}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		true,
		false,
		false,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts))
	require.Nil(t, selection)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactSkipsChatOnlyAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91006)
	accounts := []Account{
		{
			ID:          71060,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    10,
			Extra: map[string]any{
				"openai_compact_supported":   true,
				"openai_responses_supported": false,
			},
		},
		{
			ID:          71061,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra: map[string]any{
				"openai_compact_supported":   true,
				"openai_responses_supported": true,
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.6-sol",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityResponses,
		true,
		false,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71061), selection.Account.ID)
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown
// 验证当没有"已知支持"账号时，compact 请求会回退到"未探测"账号。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91003)
	accounts := []Account{
		{
			ID:          71020,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": false}, // tier=0
		},
		{
			ID:          71021,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{}, // unknown -> tier=1
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71021), selection.Account.ID, "unknown account should be picked when no supported account available")
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactAllowsGrok verifies
// that OpenAI-compatible compact routing does not reject Grok accounts.
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactAllowsGrok(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91004)
	accounts := []Account{
		{
			ID:          71030,
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"grok-4.5": "grok-4.5"},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"grok-4.5",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		true,
		false,
		true,
		PlatformGrok,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71030), selection.Account.ID)
}

// TestOpenAICompactSupportTier 验证 tier 分类逻辑。
func TestOpenAICompactSupportTier(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    int
	}{
		{name: "nil", account: nil, want: 0},
		{name: "non openai", account: &Account{Platform: PlatformAnthropic}, want: 0},
		{name: "grok", account: &Account{Platform: PlatformGrok}, want: 2},
		{name: "openai unknown", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{}}, want: 1},
		{name: "openai supported", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": true}}, want: 2},
		{name: "openai unsupported", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": false}}, want: 0},
		{name: "force on", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOn}}, want: 2},
		{name: "force off overrides probe true", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOff, "openai_compact_supported": true}}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openAICompactSupportTier(tt.account); got != tt.want {
				t.Fatalf("openAICompactSupportTier(...) = %d, want %d", got, tt.want)
			}
		})
	}
}
