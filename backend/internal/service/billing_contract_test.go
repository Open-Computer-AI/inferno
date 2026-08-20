package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fakes. Every source BillingContractService consumes is a narrow interface
// (see billing_contract.go) satisfied structurally by the real cached service,
// so these fakes stand in for exactly what production calls -- no adapter, no
// second code path.
// ---------------------------------------------------------------------------

type fakeBillingBalanceSource struct {
	balance float64
	err     error
	calls   int
}

func (f *fakeBillingBalanceSource) GetUserBalance(_ context.Context, _ int64) (float64, error) {
	f.calls++
	return f.balance, f.err
}

type fakeBillingOrgSource struct {
	orgs    []*dbent.Org
	role    string
	orgsErr error
	roleErr error
}

func (f *fakeBillingOrgSource) OrgsForUser(_ context.Context, _ int64) ([]*dbent.Org, error) {
	return f.orgs, f.orgsErr
}

func (f *fakeBillingOrgSource) RoleIn(_ context.Context, _, _ int64) (string, error) {
	return f.role, f.roleErr
}

type fakeBillingUsageSource struct {
	actualCost float64
	err        error
	gotStart   time.Time
	gotEnd     time.Time
	calls      int
}

func (f *fakeBillingUsageSource) GetStatsByUser(_ context.Context, _ int64, start, end time.Time) (*UsageStats, error) {
	f.calls++
	f.gotStart, f.gotEnd = start, end
	if f.err != nil {
		return nil, f.err
	}
	return &UsageStats{TotalActualCost: f.actualCost}, nil
}

type fakeBillingPaymentSource struct {
	limits  *MethodLimitsResponse
	enabled bool
	err     error
}

func (f *fakeBillingPaymentSource) GetAvailableMethodLimits(_ context.Context) (*MethodLimitsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.limits, nil
}

func (f *fakeBillingPaymentSource) IsPaymentEnabled(_ context.Context) bool { return f.enabled }

// fakeBillingPlanSource stands in for PaymentConfigService's BillingPlanSource
// half: ListPlans + GetGroupInfoMap.
type fakeBillingPlanSource struct {
	plans     []*dbent.SubscriptionPlan
	groupInfo map[int64]PlanGroupInfo
	err       error
}

func (f *fakeBillingPlanSource) ListPlans(_ context.Context) ([]*dbent.SubscriptionPlan, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.plans, nil
}

func (f *fakeBillingPlanSource) GetGroupInfoMap(_ context.Context, _ []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	return f.groupInfo
}

// fakeBillingSubscriptionSource stands in for SubscriptionService's
// ListActiveUserSubscriptions, keyed by user id so a test can seed TWO
// different users' rows in the same fake store and prove one user's
// response never reflects the other's data -- the isolation guarantee the
// task-3 brief calls for.
type fakeBillingSubscriptionSource struct {
	byUser map[int64][]UserSubscription
	err    error
}

func (f *fakeBillingSubscriptionSource) ListActiveUserSubscriptions(_ context.Context, userID int64) ([]UserSubscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byUser[userID], nil
}

// billingContractFixture holds the fakes so a test can reach in and break one
// source at a time -- that is what the partial-degradation test needs.
type billingContractFixture struct {
	balance *fakeBillingBalanceSource
	org     *fakeBillingOrgSource
	usage   *fakeBillingUsageSource
	payment *fakeBillingPaymentSource
	plan    *fakeBillingPlanSource
	sub     *fakeBillingSubscriptionSource
	now     time.Time
}

// recordUsage sets the month-to-date ACTUAL cost -- actual_cost, not
// total_cost, because actual_cost is the column UsageService.Create deducts
// from the wallet (usage_service.go:124-130). "Spent" must mean the same thing
// the balance means or the two numbers on one screen contradict each other.
func (f *billingContractFixture) recordUsage(amount float64) { f.usage.actualCost = amount }

func newBillingContractFixture(t *testing.T) (*BillingContractService, *billingContractFixture) {
	t.Helper()

	fx := &billingContractFixture{
		balance: &fakeBillingBalanceSource{balance: 42.50},
		org: &fakeBillingOrgSource{
			orgs: []*dbent.Org{{ID: 1, Slug: "acme-1a2b", Name: "Acme"}},
			role: OrgRoleOwner,
		},
		usage: &fakeBillingUsageSource{},
		payment: &fakeBillingPaymentSource{
			limits:  &MethodLimitsResponse{GlobalMin: 5, GlobalMax: 500},
			enabled: true,
		},
		plan: &fakeBillingPlanSource{groupInfo: map[int64]PlanGroupInfo{}},
		sub:  &fakeBillingSubscriptionSource{byUser: map[int64][]UserSubscription{}},
		now:  time.Date(2026, 8, 19, 13, 45, 0, 0, time.UTC),
	}

	svc := NewBillingContractService(fx.balance, fx.org, fx.usage, fx.payment, fx.plan, fx.sub, "https://portal.example.com")
	svc.now = func() time.Time { return fx.now }
	return svc, fx
}

// ---------------------------------------------------------------------------

func TestBillingStateReportsBalanceOrgAndSpend(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(1.25)

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.True(t, got.LoggedIn)
	require.Equal(t, "42.5", got.BalanceUSD)
	require.NotNil(t, got.Org)
	require.Equal(t, "OWNER", got.Org.Role)
	require.Equal(t, "1", got.Org.ID)
	require.Equal(t, "acme-1a2b", got.Org.Slug)
	require.NotNil(t, got.MonthlyCap)
	require.Equal(t, "1.25", got.MonthlyCap.SpentThisMonthUSD)
	require.Equal(t, "none", got.Card.Kind,
		"Inferno has no card vault; reporting anything else makes the CLI offer a one-click charge that cannot work")
	require.False(t, got.AutoReload.Enabled)
}

// TestBillingStateDegradesWhenTheUsageRollupFails is the partial-degradation
// guarantee. The client fails open on our behalf, so our obligation is the
// inverse: return everything that resolved rather than nothing.
//
// MUTATION CHECK: replacing resolveMonthlyCap's `return nil` with a propagated
// error (so a usage failure aborts State) fails this test on its FIRST
// assertion -- require.NoError -- which is a require, so it FailNow()s before
// any field is dereferenced. It fails on the assertion, not on a nil panic.
func TestBillingStateDegradesWhenTheUsageRollupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.usage.err = errors.New("usage_logs aggregate timed out")

	got, err := svc.State(context.Background(), 7)

	require.NoError(t, err, "a failed usage rollup must not fail the whole response")
	require.NotNil(t, got)
	require.Equal(t, "42.5", got.BalanceUSD, "the balance is still useful when the rollup is down")
	require.NotNil(t, got.Org)
	require.Equal(t, "OWNER", got.Org.Role)
	require.Nil(t, got.MonthlyCap,
		"nil means could-not-resolve; a zero here would claim the user has spent nothing this month")
	require.Equal(t, 1, fx.usage.calls, "the rollup was actually attempted")
}

// TestBillingStateFailsWhenTheBalanceCannotBeResolved is the other half of the
// same rule: the balance is the one section whose failure IS fatal.
func TestBillingStateFailsWhenTheBalanceCannotBeResolved(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.balance.err = errors.New("redis down and the database refused too")

	got, err := svc.State(context.Background(), 7)

	require.Error(t, err)
	require.Nil(t, got)
	require.ErrorContains(t, err, "resolve balance")
}

// TestBillingStateDegradesWhenTheOrgLookupFails proves the org section
// degrades independently of the balance and the rollup -- and, specifically,
// that canChangePlan is OMITTED rather than sent as false. A false there would
// tell the client "this user may not change their plan", which is a claim a
// failed lookup has not earned; omitting it returns the client to its own
// role fallback.
func TestBillingStateDegradesWhenTheOrgLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.orgsErr = errors.New("org query failed")
	fx.recordUsage(1.25)

	got, err := svc.State(context.Background(), 7)

	require.NoError(t, err)
	require.Nil(t, got.Org)
	require.Nil(t, got.CanChangePlan, "a lookup failure must not be reported as a denied capability")
	require.Equal(t, "42.5", got.BalanceUSD)
	require.NotNil(t, got.MonthlyCap)
	require.Equal(t, "1.25", got.MonthlyCap.SpentThisMonthUSD)
}

// TestBillingStateOmitsCanChangePlanWhenTheRoleLookupFails is the narrower
// case: the org resolved but its role did not.
func TestBillingStateOmitsCanChangePlanWhenTheRoleLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.roleErr = errors.New("membership query failed")

	got, err := svc.State(context.Background(), 7)

	require.NoError(t, err)
	require.NotNil(t, got.Org, "membership was already proven by OrgsForUser")
	require.Equal(t, "1", got.Org.ID)
	require.Empty(t, got.Org.Role)
	require.Nil(t, got.CanChangePlan)
}

// TestBillingStateGrantsPlanChangeOnlyToOwnerAndAdmin pins the capability
// mapping. A MEMBER getting true here would make the CLI offer a plan change
// the server will refuse.
func TestBillingStateGrantsPlanChangeOnlyToOwnerAndAdmin(t *testing.T) {
	for _, tc := range []struct {
		role string
		want bool
	}{
		{OrgRoleOwner, true},
		{OrgRoleAdmin, true},
		{OrgRoleMember, false},
	} {
		t.Run(tc.role, func(t *testing.T) {
			svc, fx := newBillingContractFixture(t)
			fx.org.role = tc.role

			got, err := svc.State(context.Background(), 7)

			require.NoError(t, err)
			require.NotNil(t, got.CanChangePlan)
			require.Equal(t, tc.want, *got.CanChangePlan)
		})
	}
}

// TestBillingStateAggregatesTheCalendarMonthToDate pins the window itself.
// Without this, "spent this month" could quietly become "spent ever" or a
// rolling 30 days and every assertion above would still pass.
func TestBillingStateAggregatesTheCalendarMonthToDate(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.now = time.Date(2026, 8, 19, 13, 45, 0, 0, time.UTC)

	_, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, timezone.StartOfMonth(fx.now), fx.usage.gotStart,
		"the window must start at the first instant of the current month")
	require.Equal(t, fx.now, fx.usage.gotEnd)
	require.Equal(t, 2026, fx.usage.gotStart.Year())
	require.Equal(t, time.August, fx.usage.gotStart.Month())
	require.Equal(t, 1, fx.usage.gotStart.Day())
}

// TestBillingStateReportsTopUpBoundsAndTheKillSwitch covers the payment half.
func TestBillingStateReportsTopUpBoundsAndTheKillSwitch(t *testing.T) {
	svc, _ := newBillingContractFixture(t)

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.True(t, got.CLIBillingEnabled)
	require.NotNil(t, got.Bounds)
	require.NotNil(t, got.Bounds.MinUSD)
	require.Equal(t, "5", *got.Bounds.MinUSD)
	require.NotNil(t, got.Bounds.MaxUSD)
	require.Equal(t, "500", *got.Bounds.MaxUSD)
}

// TestBillingStateReportsAnUnsetBoundAsNull: MethodLimitsResponse uses 0 to
// mean "no minimum"/"no maximum". Emitting "0" would tell the client the
// maximum top-up is zero dollars.
func TestBillingStateReportsAnUnsetBoundAsNull(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.payment.limits = &MethodLimitsResponse{GlobalMin: 0, GlobalMax: 0}

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.NotNil(t, got.Bounds)
	require.Nil(t, got.Bounds.MinUSD)
	require.Nil(t, got.Bounds.MaxUSD)
}

// TestBillingStateDegradesWhenPaymentLimitsFail: the bounds go, the kill
// switch stays -- they are two different reads.
func TestBillingStateDegradesWhenPaymentLimitsFail(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.payment.err = errors.New("provider instance query failed")
	fx.payment.enabled = true

	got, err := svc.State(context.Background(), 7)

	require.NoError(t, err)
	require.Nil(t, got.Bounds)
	require.True(t, got.CLIBillingEnabled)
	require.Equal(t, "42.5", got.BalanceUSD)
}

// TestBillingStateReadsTheBalanceThroughTheCache is a composition check, not a
// value check: this endpoint is polled by every running agent, and the one
// mistake that matters here is reaching past BillingCacheService to a
// repository. The interface this service declares (BillingBalanceSource) has
// exactly one method, GetUserBalance, which *BillingCacheService provides and
// UserRepository does not -- so a regression to userRepo.GetByID cannot
// compile. This asserts the call is made exactly once per response.
func TestBillingStateReadsTheBalanceThroughTheCacheExactlyOnce(t *testing.T) {
	svc, fx := newBillingContractFixture(t)

	_, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, 1, fx.balance.calls)
}

// TestBillingCacheServiceSatisfiesBillingBalanceSource is the other half of
// the above: the narrow interface is only meaningful if the real cached
// service actually implements it. A compile-time assertion, so a signature
// change on BillingCacheService.GetUserBalance breaks the build here rather
// than silently leaving wire to bind something else.
func TestBillingCacheServiceSatisfiesBillingBalanceSource(t *testing.T) {
	var _ BillingBalanceSource = (*BillingCacheService)(nil)
	var _ BillingOrgSource = (*OrgService)(nil)
	var _ BillingUsageSource = (*UsageService)(nil)
	var _ BillingPaymentSource = (*PaymentConfigService)(nil)
	var _ BillingPlanSource = (*PaymentConfigService)(nil)
	var _ BillingSubscriptionSource = (*SubscriptionService)(nil)
}

// TestBillingStatePortalURLPointsAtThisDeployment covers the live bug this
// endpoint exists to fix: without a server-supplied portalUrl the client
// builds `{portal_base}/billing?topup=open`, whose default base is
// portal.nousresearch.com and whose path does not exist on Inferno at all.
func TestBillingStatePortalURLPointsAtThisDeployment(t *testing.T) {
	svc, _ := newBillingContractFixture(t)

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, "https://portal.example.com/purchase", got.PortalURL)
	require.NotContains(t, got.PortalURL, "nousresearch.com")
}

// TestBillingStateOmitsPortalURLWhenUnconfigured: an empty frontend_url must
// not produce "/purchase", a relative path the client would absolutise against
// the Nous default.
func TestBillingStateOmitsPortalURLWhenUnconfigured(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	svc.portalBaseURL = ""

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)
	require.Empty(t, got.PortalURL)
	_ = fx
}

// ---------------------------------------------------------------------------
// GET /api/billing/subscription -- Subscription()
// ---------------------------------------------------------------------------

// TestBillingSubscriptionReportsCurrentPlanTiersAndContext is the happy path:
// an active subscription surfaces as `current` with a real tierId, and the
// plan catalog lists it as `isCurrent`.
func TestBillingSubscriptionReportsCurrentPlanTiersAndContext(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.orgs = []*dbent.Org{{ID: 1, Slug: "acme-1a2b", Name: "Acme", IsPersonal: false}}
	fx.org.role = OrgRoleOwner

	limit := 100.0
	fx.sub.byUser[7] = []UserSubscription{{
		ID:              42,
		UserID:          7,
		GroupID:         9,
		ExpiresAt:       time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC),
		MonthlyUsageUSD: 30,
		Group:           &Group{ID: 9, Name: "Pro", MonthlyLimitUSD: &limit},
	}}
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro", Price: 20, SortOrder: 2, ForSale: true},
		{ID: 101, GroupID: 5, Name: "Starter", Price: 5, SortOrder: 1, ForSale: true},
	}
	fx.plan.groupInfo = map[int64]PlanGroupInfo{
		9: {MonthlyLimitUSD: &limit},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, "team", got.Context, "IsPersonal:false must map to team")
	require.NotNil(t, got.Org)
	require.Equal(t, "OWNER", got.Org.Role)
	require.NotNil(t, got.CanChangePlan)
	require.True(t, *got.CanChangePlan)

	require.NotNil(t, got.Current, "an active subscription must produce a non-nil current")
	require.Equal(t, "9", got.Current.TierID)
	require.Equal(t, "Pro", got.Current.TierName)
	require.NotNil(t, got.Current.MonthlyCredits)
	require.Equal(t, "100", *got.Current.MonthlyCredits)
	require.NotNil(t, got.Current.CreditsRemaining)
	require.Equal(t, "70", *got.Current.CreditsRemaining)
	require.Equal(t, "2026-09-19T00:00:00Z", got.Current.CycleEndsAt)
	require.False(t, got.Current.CancelAtPeriodEnd)

	require.Len(t, got.Tiers, 2)
	byID := map[string]BillingTierView{}
	for _, tier := range got.Tiers {
		byID[tier.TierID] = tier
	}
	require.True(t, byID["9"].IsCurrent)
	require.False(t, byID["5"].IsCurrent)
	require.Equal(t, "20", byID["9"].DollarsPerMonthDisplay)
	require.Equal(t, 2, byID["9"].TierOrder)
	require.True(t, byID["9"].IsEnabled)
}

// TestBillingSubscriptionReportsNoPlanAsNilCurrent pins the exact shape
// agent/subscription_view.py:142-144 documents: "no plan" is current:null,
// never an object of nulls -- the all-null-object shape is gone. A caller
// with zero active subscriptions must get a nil *BillingCurrentSubscriptionView
// (which marshals to JSON null; the wire-byte assertion lives in the route
// test), not a zero-valued struct.
func TestBillingSubscriptionReportsNoPlanAsNilCurrent(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = nil // no active subscriptions

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Nil(t, got.Current, "no active subscription must produce a nil current, not a zero-valued object")
}

// TestBillingSubscriptionTiersIsNeverNilEvenWhenTheCatalogIsEmpty: `tiers`
// must be a JSON array even with zero plans -- a nil Go slice marshals to
// `null`, which subscription_view.py:224-229 silently turns into an empty
// picker with no error, but the Go-level contract is that Tiers is always a
// non-nil (possibly zero-length) slice.
func TestBillingSubscriptionTiersIsNeverNilEvenWhenTheCatalogIsEmpty(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = nil

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.NotNil(t, got.Tiers)
	require.Empty(t, got.Tiers)
}

// TestBillingSubscriptionMarksAGrandfatheredPlanDisabledButStillCurrent: a
// plan the caller is on but that is no longer ForSale must still appear in
// `tiers` (isEnabled:false) so the picker can render the caller's own row --
// this is why resolveTiers uses ListPlans, not ListPlansForSale.
func TestBillingSubscriptionMarksAGrandfatheredPlanDisabledButStillCurrent(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Group:     &Group{ID: 9, Name: "Legacy Pro"},
	}}
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Legacy Pro", Price: 20, SortOrder: 1, ForSale: false},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Len(t, got.Tiers, 1)
	require.True(t, got.Tiers[0].IsCurrent)
	require.False(t, got.Tiers[0].IsEnabled, "a no-longer-sold plan must still show, but disabled")
}

// TestBillingSubscriptionCreditsRemainingCanBeZero: a real zero remaining is
// a different fact than "unknown" and must NOT be nulled out the way
// billingOptionalMoney treats an unset bound.
func TestBillingSubscriptionCreditsRemainingCanBeZero(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	limit := 50.0
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt:       time.Now().Add(time.Hour),
		MonthlyUsageUSD: 75, // over the cap
		Group:           &Group{ID: 9, Name: "Pro", MonthlyLimitUSD: &limit},
	}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.NotNil(t, got.Current)
	require.NotNil(t, got.Current.CreditsRemaining)
	require.Equal(t, "0", *got.Current.CreditsRemaining, "over-cap usage must clamp to 0, not go negative")
}

// TestBillingSubscriptionOmitsMonthlyCreditsWhenTheGroupHasNoCap: an
// uncapped group has no credits figure to report -- same not-invented
// pattern as BillingStateView.MonthlyCap.LimitUSD.
func TestBillingSubscriptionOmitsMonthlyCreditsWhenTheGroupHasNoCap(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 9, Name: "Unlimited", MonthlyLimitUSD: nil},
	}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.NotNil(t, got.Current)
	require.Nil(t, got.Current.MonthlyCredits)
	require.Nil(t, got.Current.CreditsRemaining)
}

// TestBillingSubscriptionContextIsPersonalForAPersonalOrg is the other half
// of the context mapping.
func TestBillingSubscriptionContextIsPersonalForAPersonalOrg(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.orgs = []*dbent.Org{{ID: 1, Slug: "me-1a2b", Name: "Personal", IsPersonal: true}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, "personal", got.Context)
}

// TestBillingSubscriptionDefaultsContextToPersonalWhenOrgLookupFails: the
// client silently defaults an unrecognized context to "personal"
// (subscription_view.py:221-222); the server side matches that default
// rather than emitting an empty string on a lookup failure.
func TestBillingSubscriptionDefaultsContextToPersonalWhenOrgLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.orgsErr = errors.New("org query failed")

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Equal(t, "personal", got.Context)
	require.Nil(t, got.Org)
	require.Nil(t, got.CanChangePlan)
}

// TestBillingSubscriptionOmitsCanChangePlanWhenTheRoleLookupFails mirrors
// State's identical guarantee: a lookup failure must not be reported as a
// denied capability.
func TestBillingSubscriptionOmitsCanChangePlanWhenTheRoleLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.org.roleErr = errors.New("membership query failed")

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.NotNil(t, got.Org)
	require.Nil(t, got.CanChangePlan)
}

// TestBillingSubscriptionDegradesWhenTheSubscriptionLookupFails: Subscription
// never fails the whole response -- there is no field here as load-bearing as
// State's balance. Current degrades to nil and everything else still resolves.
//
// MUTATION CHECK: propagating the subscription lookup's error out of
// Subscription (so it returns (nil, err) instead of degrading) fails this
// test on require.NoError, the same FailNow()-before-dereference shape as
// TestBillingStateDegradesWhenTheUsageRollupFails.
func TestBillingSubscriptionDegradesWhenTheSubscriptionLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.err = errors.New("user_subscriptions query timed out")
	fx.plan.plans = []*dbent.SubscriptionPlan{{ID: 100, GroupID: 9, Name: "Pro", Price: 20, ForSale: true}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, got.Current)
	require.NotEmpty(t, got.Tiers, "the plan catalog is independent of the subscription lookup and must still resolve")
}

// TestBillingSubscriptionDegradesWhenThePlanCatalogFails: the inverse -- a
// broken plan catalog must not take `current` down with it.
func TestBillingSubscriptionDegradesWhenThePlanCatalogFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.err = errors.New("subscription_plans query failed")
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 9, Name: "Pro"},
	}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got.Current)
	require.Equal(t, "9", got.Current.TierID)
	require.NotNil(t, got.Tiers)
	require.Empty(t, got.Tiers)
}

// TestBillingSubscriptionDegradesWhenTheGroupIsNotEagerLoaded: a defensive
// case -- ListActiveUserSubscriptions is documented to eager-load Group, but
// if a row ever arrives without one, Subscription must degrade to nil rather
// than emit a current object with an empty tierId (which the client would
// treat as no-plan on the id check anyway, but a name-only leak with a blank
// id is worse than an honest nil).
func TestBillingSubscriptionDegradesWhenTheGroupIsNotEagerLoaded(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     nil,
	}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, got.Current)
}

// TestBillingSubscriptionIsolatesBetweenTwoUsers is the isolation proof the
// task-3 brief requires: a second user's rows are present in the SAME fake
// store (byUser is keyed by user id, exactly like the real repository's
// WHERE user_id = ? predicate), and user 7's response must never reflect
// user 8's plan.
//
// MUTATION CHECK: hardcoding resolveCurrentSubscription's lookup to always
// read fx.sub.byUser[8] (or any id other than the caller's) makes user 7's
// assertion below fail -- TierID flips from "9" to "40", and IsCurrent on
// tier 9 flips from true to false. See task-3-report.md for the literal diff
// and failing output.
func TestBillingSubscriptionIsolatesBetweenTwoUsers(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 9, Name: "User7Plan"},
	}}
	fx.sub.byUser[8] = []UserSubscription{{
		ID: 2, UserID: 8, GroupID: 40,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 40, Name: "User8Plan"},
	}}
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "User7Plan", Price: 20, ForSale: true},
		{ID: 200, GroupID: 40, Name: "User8Plan", Price: 40, ForSale: true},
	}

	got7, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got7.Current)
	require.Equal(t, "9", got7.Current.TierID)
	require.Equal(t, "User7Plan", got7.Current.TierName)

	got8, err := svc.Subscription(context.Background(), 8)
	require.NoError(t, err)
	require.NotNil(t, got8.Current)
	require.Equal(t, "40", got8.Current.TierID)
	require.Equal(t, "User8Plan", got8.Current.TierName)

	for _, tier := range got7.Tiers {
		if tier.TierID == "9" {
			require.True(t, tier.IsCurrent)
		}
		if tier.TierID == "40" {
			require.False(t, tier.IsCurrent, "user 7's response must never mark user 8's plan current")
		}
	}
}

// TestBillingSubscriptionPortalURLMatchesState: same deep link, same
// deployment, for the same reason State's does.
func TestBillingSubscriptionPortalURLMatchesState(t *testing.T) {
	svc, _ := newBillingContractFixture(t)

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "https://portal.example.com/purchase", got.PortalURL)
	require.NotContains(t, got.PortalURL, "nousresearch.com")
}

// ---------------------------------------------------------------------------
// ruling R-3.2 -- dollarsPerMonthDisplay normalised by validity, matching
// inferno-frontend/src/components/payment/validity.ts's unit semantics.
// ---------------------------------------------------------------------------

func TestBillingDollarsPerMonthNormalizesByValidityUnit(t *testing.T) {
	for _, tc := range []struct {
		name         string
		price        float64
		validityDays int
		validityUnit string
		want         float64
	}{
		{"monthly plan, unit=months, days=1 month -> unchanged", 20, 1, "months", 20},
		{"annual plan, unit=months, days=12 months -> divided by 12", 1200, 12, "months", 100},
		{"annual plan, unit=month singular -> same as plural", 1200, 12, "month", 100},
		{"weekly-billed plan, unit=weeks, days=4 weeks (28 real days)", 70, 4, "weeks", 75},
		{"unit=week singular -> same as plural", 70, 4, "week", 75},
		{"day-unit plan, days=30 -> already ~monthly", 300, 30, "day", 300},
		{"unrecognized unit falls back to literal days, same as 'day'", 300, 30, "fortnight", 300},
		{"mixed case and whitespace normalise the same as lowercase trimmed", 1200, 12, "  Months  ", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := billingDollarsPerMonth(tc.price, tc.validityDays, tc.validityUnit)
			require.InDelta(t, tc.want, got, 0.0001)
		})
	}
}

// TestBillingDollarsPerMonthHandlesZeroValidityWithoutDividingByZero: a
// plan with no usable validity period (only reachable via malformed/legacy
// data -- CreatePlan/UpdatePlan validate ValidityDays > 0) must not panic or
// produce Inf/NaN; it falls back to the raw, unnormalised price.
func TestBillingDollarsPerMonthHandlesZeroValidityWithoutDividingByZero(t *testing.T) {
	got := billingDollarsPerMonth(1200, 0, "months")
	require.Equal(t, 1200.0, got)

	got = billingDollarsPerMonth(1200, -5, "months")
	require.Equal(t, 1200.0, got, "a negative validity is equally malformed and must not divide by (or produce) a negative number")
}

// TestBillingSubscriptionRepresentativeTierUsesNormalizedDollarsPerMonth is
// the end-to-end wiring check: Subscription() actually calls the normaliser,
// not the raw Price, for the group it picks.
func TestBillingSubscriptionRepresentativeTierUsesNormalizedDollarsPerMonth(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Annual", Price: 1200, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Len(t, got.Tiers, 1)
	require.Equal(t, "100", got.Tiers[0].DollarsPerMonthDisplay,
		"a $1200/year plan must display as $100/month, not the raw annual price")
}
