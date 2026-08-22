package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

// fakeBillingChargeOrderSource stands in for *PaymentService's
// BillingChargeOrderSource half (CreateOrder + GetOrder). GetOrder
// faithfully reproduces the REAL PaymentService.GetOrder's own error
// contract (payment_order.go:918-927: infraerrors.NotFound for an unknown
// id, infraerrors.Forbidden for one owned by someone else) so
// ChargeStatus's "always pending, never distinguishable" behaviour is
// exercised against the actual error shapes production returns, not an
// invented pair of sentinels.
type fakeBillingChargeOrderSource struct {
	createResp   *CreateOrderResponse
	createErr    error
	gotCreateReq CreateOrderRequest
	createCalls  int

	// orders is keyed by order id; UserID on the stored order drives the
	// ownership check, exactly like the real GetOrder.
	orders map[int64]*dbent.PaymentOrder
}

func (f *fakeBillingChargeOrderSource) CreateOrder(_ context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	f.createCalls++
	f.gotCreateReq = req
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResp, nil
}

func (f *fakeBillingChargeOrderSource) GetOrder(_ context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

// billingContractFixture holds the fakes so a test can reach in and break one
// source at a time -- that is what the partial-degradation test needs.
type billingContractFixture struct {
	balance     *fakeBillingBalanceSource
	org         *fakeBillingOrgSource
	usage       *fakeBillingUsageSource
	payment     *fakeBillingPaymentSource
	plan        *fakeBillingPlanSource
	sub         *fakeBillingSubscriptionSource
	charge      *fakeBillingChargeOrderSource
	idempotency *IdempotencyCoordinator
	idemRepo    *inMemoryIdempotencyRepo
	now         time.Time
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
		plan:   &fakeBillingPlanSource{groupInfo: map[int64]PlanGroupInfo{}},
		sub:    &fakeBillingSubscriptionSource{byUser: map[int64][]UserSubscription{}},
		charge: &fakeBillingChargeOrderSource{orders: map[int64]*dbent.PaymentOrder{}},
		now:    time.Date(2026, 8, 19, 13, 45, 0, 0, time.UTC),
	}
	fx.idemRepo = newInMemoryIdempotencyRepo()
	fx.idempotency = NewIdempotencyCoordinator(fx.idemRepo, DefaultIdempotencyConfig())

	svc := NewBillingContractService(fx.balance, fx.org, fx.usage, fx.payment, fx.plan, fx.sub, fx.charge, fx.idempotency, "https://portal.example.com")
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
	require.Equal(t, "20.00", byID["9"].DollarsPerMonthDisplay)
	// tierOrder is a COMPUTED 1-based rank by normalised dollars-per-month
	// (billingAssignTierOrder, ruling C-2), not SubscriptionPlan.SortOrder:
	// group 5 sells at $5/mo and ranks 1, group 9 at $20/mo ranks 2.
	require.Equal(t, 1, byID["5"].TierOrder)
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
// ruling R-3.1 -- one tiers[] entry per GROUP, not per plan.
//
// subscription_plan.group_id is not unique (ent/schema/subscription_plan.go's
// index.Fields("group_id") carries no .Unique()): a group can sell at several
// billing periods. tierId must stay the group id in both `current` and
// `tiers[]` because user_subscription has no plan_id column at all -- only
// group_id -- and the TUI polls current.tier_id against a tiers[] id after an
// upgrade (ui-tui/src/components/subscriptionOverlay.tsx:786).
// ---------------------------------------------------------------------------

// TestBillingSubscriptionCollapsesMultiplePlansPerGroupIntoOneTier is the
// direct reproduction: a group selling both monthly and annual must still
// produce exactly ONE tiers[] row, not two with the same tierId.
func TestBillingSubscriptionCollapsesMultiplePlansPerGroupIntoOneTier(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, SortOrder: 1, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 101, GroupID: 9, Name: "Pro Annual", Price: 180, SortOrder: 2, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	require.Len(t, got.Tiers, 1, "two plans in one group must collapse to one tiers[] row")
	require.Equal(t, "9", got.Tiers[0].TierID)
}

// TestBillingSubscriptionTierIDsAreUniqueAcrossTheWholeArray is the general
// invariant, across several groups, some multi-plan and some single-plan.
func TestBillingSubscriptionTierIDsAreUniqueAcrossTheWholeArray(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 101, GroupID: 9, Name: "Pro Annual", Price: 180, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
		{ID: 200, GroupID: 5, Name: "Starter", Price: 5, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 300, GroupID: 40, Name: "Legacy", Price: 999, ForSale: false, ValidityDays: 1, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, tier := range got.Tiers {
		require.False(t, seen[tier.TierID], "duplicate tierId %q in tiers[]", tier.TierID)
		seen[tier.TierID] = true
	}
	require.Len(t, got.Tiers, 3, "three distinct groups must produce three rows")
}

// TestBillingSubscriptionCurrentTierIDIsFindableInTiers pins the invariant
// subscriptionOverlay.tsx:786's post-upgrade poll depends on:
// current.tier_id === <some tiers[] entry's tier_id>. A future change that
// breaks the shared id space must fail HERE, not silently in the TUI's
// 30-second "still applying" hang.
func TestBillingSubscriptionCurrentTierIDIsFindableInTiers(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 9, Name: "Pro"},
	}}
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 101, GroupID: 9, Name: "Pro Annual", Price: 180, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got.Current)

	found := false
	for _, tier := range got.Tiers {
		if tier.TierID == got.Current.TierID {
			found = true
		}
	}
	require.True(t, found, "current.tierId must be findable in tiers[] -- the TUI's post-upgrade poll depends on this exact match")
}

// TestBillingRepresentativePlanPrefersForSaleOverNonForSale: a plan nobody
// can buy must never set the displayed price.
func TestBillingRepresentativePlanPrefersForSaleOverNonForSale(t *testing.T) {
	plans := []*dbent.SubscriptionPlan{
		{ID: 1, Name: "Cheap But Discontinued", Price: 1, ForSale: false, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 2, Name: "Current Price", Price: 50, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
	}
	rep, anyForSale := billingRepresentativePlan(plans)
	require.Equal(t, int64(2), rep.ID)
	require.True(t, anyForSale)
}

// TestBillingRepresentativePlanPicksLowestNormalizedPrice: among for-sale
// candidates, the lowest PER-MONTH price wins, not the lowest raw price --
// an annual plan's raw price can be numerically larger while its per-month
// rate is cheaper.
func TestBillingRepresentativePlanPicksLowestNormalizedPrice(t *testing.T) {
	plans := []*dbent.SubscriptionPlan{
		{ID: 1, Name: "Monthly", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},  // $20/mo
		{ID: 2, Name: "Annual", Price: 180, ForSale: true, ValidityDays: 12, ValidityUnit: "months"}, // $15/mo
	}
	rep, _ := billingRepresentativePlan(plans)
	require.Equal(t, int64(2), rep.ID, "the annual plan's normalised $15/mo beats the monthly plan's $20/mo")
}

// TestBillingRepresentativePlanTieBreaksOnLowestPlanID: equal normalised
// price must still pick a stable, deterministic representative.
func TestBillingRepresentativePlanTieBreaksOnLowestPlanID(t *testing.T) {
	plans := []*dbent.SubscriptionPlan{
		{ID: 200, Name: "B", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 100, Name: "A", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
	}
	rep, _ := billingRepresentativePlan(plans)
	require.Equal(t, int64(100), rep.ID)
}

// TestBillingRepresentativePlanReportsIsEnabledIndependentlyOfTheChosenRow:
// isEnabled is "any plan for sale", even when the chosen representative
// itself happens to be a non-for-sale row (an all-grandfathered group).
func TestBillingRepresentativePlanReportsIsEnabledIndependentlyOfTheChosenRow(t *testing.T) {
	plans := []*dbent.SubscriptionPlan{
		{ID: 1, Name: "Legacy Cheap", Price: 5, ForSale: false, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 2, Name: "Legacy Pricier", Price: 10, ForSale: false, ValidityDays: 1, ValidityUnit: "months"},
	}
	rep, anyForSale := billingRepresentativePlan(plans)
	require.Equal(t, int64(1), rep.ID, "with no for-sale candidates, falls back to the lowest normalised price among all plans")
	require.False(t, anyForSale)
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
	require.Equal(t, "100.00", got.Tiers[0].DollarsPerMonthDisplay,
		"a $1200/year plan must display as $100/month, not the raw annual price")
}

// ---------------------------------------------------------------------------
// AccountSubscription -- the snake_case `subscription` object embedded in
// GET /api/oauth/account (Task 4). A DIFFERENT wire contract from
// Subscription()/BillingSubscriptionView above (Task 3, camelCase) even
// though both read the same UserSubscription row.
// ---------------------------------------------------------------------------

// TestAccountSubscriptionReportsPlanTierAndCredits pins the happy path
// against hermes_cli/nous_account.py's _subscription_from_payload (:705-716):
// plan, tier, monthly_credits, credits_remaining, current_period_end all
// present with the right Go types, monthly_charge and rollover_credits never
// present (the Go struct has no field at all for the latter).
func TestAccountSubscriptionReportsPlanTierAndCredits(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
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
	fx.plan.groupInfo = map[int64]PlanGroupInfo{9: {MonthlyLimitUSD: &limit}}

	got := svc.AccountSubscription(context.Background(), 7)

	require.NotNil(t, got, "an active subscription must produce a non-nil subscription object")
	require.Equal(t, "Pro", got.Plan)
	require.NotNil(t, got.Tier)
	// The same int Task 3's tiers[].tierOrder uses -- read straight off the
	// tiers[] row with IsCurrent, so the two endpoints cannot disagree. It is
	// the computed rank (billingAssignTierOrder), not SortOrder: group 5 at
	// $5/mo ranks 1, group 9 at $20/mo ranks 2, and the caller is on group 9.
	require.Equal(t, 2, *got.Tier)
	require.NotNil(t, got.MonthlyCredits)
	require.Equal(t, 100.0, *got.MonthlyCredits)
	require.NotNil(t, got.CreditsRemaining)
	require.Equal(t, 70.0, *got.CreditsRemaining)
	require.Equal(t, "2026-09-19T00:00:00Z", got.CurrentPeriodEnd)
	require.Nil(t, got.MonthlyCharge, "monthly_charge must never be set -- 0 would misclassify a paying user as free tier")
}

// TestAccountSubscriptionReturnsNilWithNoActiveSubscription is Task 1's
// precedent applied here: no active subscription means the caller omits the
// WHOLE `subscription` key (nil, not a zero-valued object), which
// _subscription_from_payload's non-dict branch (:706-707) reads identically
// to any other absent/invalid value.
func TestAccountSubscriptionReturnsNilWithNoActiveSubscription(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = nil

	got := svc.AccountSubscription(context.Background(), 7)

	require.Nil(t, got)
}

// TestAccountSubscriptionReturnsNilWhenTheSubscriptionLookupFails mirrors
// Subscription()'s degrade-not-500 philosophy: a lookup error is not
// distinguishable, from the client's point of view, from "no subscription".
func TestAccountSubscriptionReturnsNilWhenTheSubscriptionLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.err = errors.New("db exploded")

	got := svc.AccountSubscription(context.Background(), 7)

	require.Nil(t, got)
}

// TestAccountSubscriptionOmitsCreditsWhenTheGroupHasNoCap: an uncapped group
// has no credits figure to report, same as Task 3's Current.MonthlyCredits.
func TestAccountSubscriptionOmitsCreditsWhenTheGroupHasNoCap(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 9, Name: "Unlimited", MonthlyLimitUSD: nil},
	}}

	got := svc.AccountSubscription(context.Background(), 7)

	require.NotNil(t, got)
	require.Nil(t, got.MonthlyCredits)
	require.Nil(t, got.CreditsRemaining)
}

// TestAccountSubscriptionOmitsTierWhenTheActiveGroupIsNotInTheCatalog: a
// data-integrity edge case (an active subscription pointing at a group
// ListPlans no longer returns any plan for) must omit `tier` rather than
// fabricate an order.
func TestAccountSubscriptionOmitsTierWhenTheActiveGroupIsNotInTheCatalog(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 999,
		ExpiresAt: time.Now().Add(time.Hour),
		Group:     &Group{ID: 999, Name: "Orphaned"},
	}}
	fx.plan.plans = nil // catalog has nothing for group 999

	got := svc.AccountSubscription(context.Background(), 7)

	require.NotNil(t, got)
	require.Nil(t, got.Tier)
}

// TestAccountSubscriptionCreditsRemainingCanBeZero: same non-negotiable as
// Task 3 -- a real zero is a different fact from "unknown" and must round
// trip as 0.0, not nil.
func TestAccountSubscriptionCreditsRemainingCanBeZero(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	limit := 50.0
	fx.sub.byUser[7] = []UserSubscription{{
		ID: 1, UserID: 7, GroupID: 9,
		ExpiresAt:       time.Now().Add(time.Hour),
		MonthlyUsageUSD: 75, // over the cap
		Group:           &Group{ID: 9, Name: "Pro", MonthlyLimitUSD: &limit},
	}}

	got := svc.AccountSubscription(context.Background(), 7)

	require.NotNil(t, got)
	require.NotNil(t, got.CreditsRemaining)
	require.Equal(t, 0.0, *got.CreditsRemaining)
}

// TestAccountSubscriptionIsolatesBetweenTwoUsers: same isolation guarantee
// Task 3 pins, on the new snake_case path.
func TestAccountSubscriptionIsolatesBetweenTwoUsers(t *testing.T) {
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

	got := svc.AccountSubscription(context.Background(), 7)

	require.NotNil(t, got)
	require.Equal(t, "User7Plan", got.Plan)
}

// ===========================================================================
// Task 5 -- the 7 write endpoints. Ruling R-5.1: POST /charge and
// GET /charge/{id} are IMPLEMENTED over PaymentService's real CreateOrder /
// GetOrder contract; the other 5 are honest refusals with no data
// dependency. Scope-gate and wire-byte assertions live in
// server/routes/billing_contract_route_test.go; these are the
// service-level behaviour + mapping-table tests.
// ===========================================================================

// TestChargeCreatesABalanceOrderAndReturnsItsIDAsChargeID pins the request
// CreateOrder actually receives (nous_billing.py:525's amountUsd, a JSON
// number, becomes the order Amount 1:1) and the 202 body's shape
// (nous_billing.py:513-514, cli_billing_mixin.py:1155's `chargeId`).
func TestChargeCreatesABalanceOrderAndReturnsItsIDAsChargeID(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.charge.createResp = &CreateOrderResponse{OrderID: 501}

	got, err := svc.Charge(context.Background(), 7, 25.5, "idem-key-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "501", got.ChargeID)
	require.Equal(t, 1, fx.charge.createCalls)
	require.Equal(t, int64(7), fx.charge.gotCreateReq.UserID)
	require.Equal(t, 25.5, fx.charge.gotCreateReq.Amount)
	require.Equal(t, payment.OrderTypeBalance, fx.charge.gotCreateReq.OrderType)
}

// TestChargeMissingIdempotencyKeyIs400 pins the brief's exact requirement:
// "a missing header is a 400, not a default" (nous_billing.py:511-521). The
// order source must never be touched -- a missing key is refused before any
// side effect, not defaulted into one.
func TestChargeMissingIdempotencyKeyIs400(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.charge.createResp = &CreateOrderResponse{OrderID: 501}

	_, err := svc.Charge(context.Background(), 7, 25, "")

	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err))
	require.Equal(t, 0, fx.charge.createCalls, "a missing Idempotency-Key must never reach CreateOrder")
}

// TestChargeMissingIdempotencyKeyIs400EvenWhitespaceOnly: the client's own
// pre-flight check trims before checking truthiness (nous_billing.py:518-521:
// `idempotency_key.strip()` in the truthiness test) -- a whitespace-only
// header must be treated the same as an absent one, not accepted as "present".
func TestChargeMissingIdempotencyKeyIs400EvenWhitespaceOnly(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	_, err := svc.Charge(context.Background(), 7, 25, "   ")
	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err))
	require.Equal(t, 0, fx.charge.createCalls)
}

// TestChargeReusesTheSameKeyOnRetryWithoutCreatingASecondOrder is the brief's
// other Step-6 requirement, proved against the REAL *IdempotencyCoordinator
// (backed by the in-memory repo idempotency_test.go already defines in this
// package) -- not a re-implementation of dedup logic in a fake, so this
// actually exercises the production code path.
func TestChargeReusesTheSameKeyOnRetryWithoutCreatingASecondOrder(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.charge.createResp = &CreateOrderResponse{OrderID: 777}

	first, err := svc.Charge(context.Background(), 7, 25, "same-key")
	require.NoError(t, err)
	require.Equal(t, "777", first.ChargeID)

	second, err := svc.Charge(context.Background(), 7, 25, "same-key")
	require.NoError(t, err)
	require.Equal(t, "777", second.ChargeID, "a retried charge must resolve to the SAME chargeId")
	require.Equal(t, 1, fx.charge.createCalls, "the retry must never create a second order")
}

// TestChargeDifferentUsersSameLiteralKeyConflictsRatherThanCrossReplays
// documents a real property of the SHARED IdempotencyCoordinator (not
// something Task 5 introduces: internal/handler/idempotency_helper.go's
// executeUserIdempotentJSON has the identical shape for every other
// idempotent write in this app). The storage key is (scope, sha256(literal
// key)) -- NOT actor-scoped -- so two different users choosing the exact
// same literal key string under this scope hit the SAME stored record.
// ActorScope only enters the FINGERPRINT, so the collision is caught as a
// fingerprint mismatch -> 409 IDEMPOTENCY_KEY_CONFLICT, never a silent
// cross-user replay of user 7's chargeId to user 8. That 409 is the safe
// outcome: no data crosses the tenant boundary, and in production this is
// academic anyway (the client's new_idempotency_key() mints a UUID per
// purchase, so a real cross-user literal collision is not a live risk).
func TestChargeDifferentUsersSameLiteralKeyConflictsRatherThanCrossReplays(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.charge.createResp = &CreateOrderResponse{OrderID: 1}

	gotUser7, err := svc.Charge(context.Background(), 7, 25, "shared-key")
	require.NoError(t, err)
	require.Equal(t, "1", gotUser7.ChargeID)

	_, err = svc.Charge(context.Background(), 8, 25, "shared-key")
	require.Error(t, err, "a different user's identical literal key must never replay user 7's chargeId")
	require.Equal(t, 409, infraerrors.Code(err))
	require.Equal(t, 1, fx.charge.createCalls, "the conflicting request must never create a second order either")
}

// TestChargeStatusMapsEveryInfernoOrderStatusToTheClientsThreeStates is the
// mapping table the report documents: Pending/Paid/Recharging -> "pending"
// (money not yet landed in the balance); Completed -> "settled" (the only
// status the money has actually landed, payment_fulfillment.go:371-373);
// Expired/Cancelled/Failed -> "failed" (terminal negatives).
func TestChargeStatusMapsEveryInfernoOrderStatusToTheClientsThreeStates(t *testing.T) {
	cases := []struct {
		name       string
		orderStat  string
		wantStatus string
		wantAmount bool
		wantReason bool
	}{
		{"pending", OrderStatusPending, "pending", false, false},
		{"paid_but_not_yet_credited", OrderStatusPaid, "pending", false, false},
		{"recharging", OrderStatusRecharging, "pending", false, false},
		{"completed", OrderStatusCompleted, "settled", true, false},
		{"expired", OrderStatusExpired, "failed", false, true},
		{"cancelled", OrderStatusCancelled, "failed", false, true},
		{"failed", OrderStatusFailed, "failed", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, fx := newBillingContractFixture(t)
			fx.charge.orders[42] = &dbent.PaymentOrder{ID: 42, UserID: 7, Status: tc.orderStat, Amount: 19.99}

			got := svc.ChargeStatus(context.Background(), 7, "42")

			require.Equal(t, tc.wantStatus, got.Status)
			if tc.wantAmount {
				require.NotNil(t, got.AmountUSD)
				require.Equal(t, "19.99", *got.AmountUSD)
			} else {
				require.Nil(t, got.AmountUSD)
			}
			if tc.wantReason {
				require.NotNil(t, got.Reason)
				require.NotEmpty(t, *got.Reason)
				// The three RESERVED values name Stripe 3DS/card outcomes
				// Inferno cannot produce (no card on file) -- must never be
				// faked here (brief, cli_billing_mixin.py:1203-1208).
				require.NotContains(t, []string{"authentication_required", "payment_method_expired", "card_declined"}, *got.Reason)
			} else {
				require.Nil(t, got.Reason)
			}
		})
	}
}

// TestChargeStatusUnknownIDIsPendingNeverAnError is the SECURITY-CRITICAL
// case (brief, citing nous_billing.py:536-538): an id with no matching order
// must answer exactly like a real pending charge, never a 404-shaped signal.
func TestChargeStatusUnknownIDIsPendingNeverAnError(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.ChargeStatus(context.Background(), 7, "999999")
	require.Equal(t, "pending", got.Status)
	require.Nil(t, got.AmountUSD)
	require.Nil(t, got.Reason)
}

// TestChargeStatusForeignOrderIsPendingNeverLeaksTheOwnersData is the OTHER
// half of the security-critical rule: an id that exists but belongs to a
// DIFFERENT real user (seeded in the same fixture, same as if they shared a
// database) must answer "pending" -- never the real status, never the real
// amount, never a 403/404 that would confirm the id exists at all.
func TestChargeStatusForeignOrderIsPendingNeverLeaksTheOwnersData(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	// User 8's REAL, completed, $500 order.
	fx.charge.orders[55] = &dbent.PaymentOrder{ID: 55, UserID: 8, Status: OrderStatusCompleted, Amount: 500}

	got := svc.ChargeStatus(context.Background(), 7, "55") // user 7 probing user 8's id

	require.Equal(t, "pending", got.Status, "a foreign order must never reveal it settled")
	require.Nil(t, got.AmountUSD, "must never leak the other user's $500 amount")
	require.Nil(t, got.Reason)
}

// TestChargeStatusMalformedIDIsPending: a non-numeric id (never a real
// PaymentOrder id) gets the identical treatment, for the identical
// existence-oracle reason -- responding differently to "malformed" vs
// "well-formed but unknown" would let a prober learn the id SHAPE.
func TestChargeStatusMalformedIDIsPending(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.ChargeStatus(context.Background(), 7, "not-a-real-id")
	require.Equal(t, "pending", got.Status)
}

// ---------------------------------------------------------------------------
// MUTATION PROOF (brief, Step 5): "return 404 for unknown ids" must fail
// this test. TestChargeStatusMutationProofUnknownIDMustNotBe404 does not
// itself mutate the source -- it documents, and would catch, exactly the
// regression a future edit could introduce: this test fails if ChargeStatus
// is ever changed to special-case a NotFound/Forbidden error into a
// different (or absent) status. See task-5-report.md for the actual
// compiling mutation + its failing `go test` output.
// ---------------------------------------------------------------------------

// TestSubscriptionPreviewIsAlwaysBlocked: "blocked" is the client's OWN
// vocabulary for "the commit would be refused" (nous_billing.py's
// post_subscription_preview docstring), so this renders cleanly on the
// client rather than as an error.
func TestSubscriptionPreviewIsAlwaysBlocked(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.SubscriptionPreview(context.Background(), 7, "pro-monthly")
	require.Equal(t, "blocked", got.Effect)
	require.NotEmpty(t, got.Reason)
}

// TestSubscriptionUpgradeNeverFakesA3DSOrDeclinedCardStatus is the brief's
// own named hazard: "DO NOT fake requires_action or payment_failed" --
// those mean 3DS-pending and card-declined, and Inferno has no card at all,
// so claiming either would be a lie the user cannot act on.
func TestSubscriptionUpgradeNeverFakesA3DSOrDeclinedCardStatus(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.SubscriptionUpgrade(context.Background(), 7, "pro-monthly", "idem-key")
	require.NotEqual(t, "requires_action", got.Error)
	require.NotEqual(t, "payment_failed", got.Error)
	require.Equal(t, "no_payment_method", got.Error)
	require.NotEmpty(t, got.Message)
	require.Equal(t, "https://portal.example.com/purchase", got.PortalURL)
}

// TestPendingChangeSetIsAnHonestRefusal / Clear / AutoTopUp: no data
// dependency, no invented success -- ruling R-5.1.
func TestPendingChangeSetIsAnHonestRefusal(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.PendingChangeSet(context.Background(), 7)
	require.Equal(t, "unsupported_operation", got.Error)
	require.NotEmpty(t, got.Message)
}

func TestPendingChangeClearIsAnHonestRefusal(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.PendingChangeClear(context.Background(), 7)
	require.Equal(t, "unsupported_operation", got.Error)
	require.NotEmpty(t, got.Message)
}

func TestAutoTopUpIsAnHonestRefusal(t *testing.T) {
	svc, _ := newBillingContractFixture(t)
	got := svc.AutoTopUp(context.Background(), 7)
	require.Equal(t, "unsupported_operation", got.Error)
	require.NotEmpty(t, got.Message)
}

// ---------------------------------------------------------------------------
// Task 6 conformance findings. Both of these shipped GREEN through every unit
// test in Tasks 3 and 4 and were caught only by running the real client's
// parsers against a live server with real seeded plans. The shape was valid in
// both cases; only the RENDERED VALUE was wrong, which no shape assertion sees.
// ---------------------------------------------------------------------------

// TestDollarsPerMonthDisplayIsATwoDecimalDisplayString pins the first.
// ui-tui/src/components/subscriptionOverlay.tsx:437 interpolates this field
// VERBATIM into a user-visible label:
//
//	`${tier.name} · ${tier.dollars_per_month_display}/mo`
//
// dollarsPerMonthDisplay is the result of a DIVISION (price normalised by the
// billing period), so shortest-round-trip float formatting emits
// 16.666666666666668 for a $200/12-month plan and the user reads
// "Pro Annual · 16.666666666666668/mo".
func TestDollarsPerMonthDisplayIsATwoDecimalDisplayString(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	limit := 1000.0
	fx.sub.byUser[7] = []UserSubscription{{
		ID:        42,
		UserID:    7,
		GroupID:   9,
		ExpiresAt: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC),
		Group:     &Group{ID: 9, Name: "Test ", MonthlyLimitUSD: &limit},
	}}
	// $200 over 12 months -> 200*30/360 -> 16.666666666666668 raw.
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Annual", Price: 200, ValidityDays: 12, ValidityUnit: "month", ForSale: true},
	}
	fx.plan.groupInfo = map[int64]PlanGroupInfo{9: {MonthlyLimitUSD: &limit}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, got.Tiers, 1)
	require.Equal(t, "16.67", got.Tiers[0].DollarsPerMonthDisplay,
		"this string is rendered verbatim into a TUI label -- it must never carry float noise")
}

// TestCurrentTierNameAgreesWithItsTiersEntry pins the second. current and its
// tiers[] entry share a tierId (the group id), so they describe ONE tier. When
// current.tierName came from Group.Name it read "Test " -- the group's internal
// admin label, trailing space included -- while the picker offered the same
// tier as "Pro Annual". One tier under two names.
func TestCurrentTierNameAgreesWithItsTiersEntry(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	limit := 1000.0
	fx.sub.byUser[7] = []UserSubscription{{
		ID:        42,
		UserID:    7,
		GroupID:   9,
		ExpiresAt: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC),
		Group:     &Group{ID: 9, Name: "Test ", MonthlyLimitUSD: &limit},
	}}
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, ValidityDays: 1, ValidityUnit: "month", ForSale: true},
		{ID: 101, GroupID: 9, Name: "Pro Annual", Price: 200, ValidityDays: 12, ValidityUnit: "month", ForSale: true},
	}
	fx.plan.groupInfo = map[int64]PlanGroupInfo{9: {MonthlyLimitUSD: &limit}}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got.Current)
	require.Len(t, got.Tiers, 1, "two plans on one group collapse to one tier (R-3.1)")

	require.Equal(t, got.Tiers[0].TierID, got.Current.TierID, "same tier, same id")
	require.Equal(t, got.Tiers[0].Name, got.Current.TierName,
		"one tierId must not carry two different names")
	require.NotEqual(t, "Test ", got.Current.TierName, "must not leak the group's admin label")
}

// ---------------------------------------------------------------------------
// C-2 -- tierOrder must be a 1-based rank, because every client surface drops
// tier_order <= 0.
// ---------------------------------------------------------------------------

// billingClientSelectableTiers is agent/subscription_view.py:373-379's
// selectable_tiers, transliterated:
//
//	t.is_enabled and not t.is_current and (t.tier_order or 0) > 0
//
// sorted by tier_order. ui-tui/src/components/subscriptionOverlay.tsx:381
// (the Free plan catalogue) and :525 (the change-plan picker) apply the
// IDENTICAL filter, so this one helper models all three surfaces.
//
// It exists because asserting "tierOrder is 1" only pins the number; the
// property that actually matters is that the PICKER IS NOT EMPTY, and a
// future change could satisfy the first while breaking the second. This
// re-derives the client's answer from our own emitted array.
func billingClientSelectableTiers(tiers []BillingTierView) []BillingTierView {
	out := []BillingTierView{}
	for _, t := range tiers {
		if t.IsEnabled && !t.IsCurrent && t.TierOrder > 0 {
			out = append(out, t)
		}
	}
	return out
}

// TestBillingSubscriptionTierOrderIsOneBasedOnDefaultSortOrder is the C-2
// reproduction and its fix, in one test.
//
// The seed is the DEFAULT case, not a hand-picked one: SortOrder is left at
// Go's zero value, which is also ent/schema/subscription_plan.go:62's
// field.Int("sort_order").Default(0), which is also what Inferno's own plan
// editor posts (inferno-frontend/src/views/admin/orders/PlanEditDialog.vue:126
// initialises sort_order:0, :182 resets to it, :199 posts it) because
// CreatePlanRequest.SortOrder carries no binding:"required"
// (payment_config_service.go:177). An operator who adds two plans through the
// admin panel and never touches the sort-order box produces exactly this.
//
// Before the fix this emitted tierOrder:0 on both rows, giving a well-formed
// two-element tiers[] array that every client surface filtered down to an
// EMPTY plan picker -- no error, no empty response, nothing to notice. Task 6's
// conformance run missed it because it seeded SortOrder 1 and 2 and only
// asserted the rows were PRESENT, never calling selectable_tiers.
func TestBillingSubscriptionTierOrderIsOneBasedOnDefaultSortOrder(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = []*dbent.SubscriptionPlan{
		// SortOrder deliberately OMITTED -- the schema default.
		{ID: 100, GroupID: 9, Name: "Pro", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 101, GroupID: 5, Name: "Starter", Price: 5, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, got.Tiers, 2)

	for _, tier := range got.Tiers {
		require.GreaterOrEqual(t, tier.TierOrder, 1,
			"every emitted tier must rank >= 1; tier %q at %d is invisible to selectable_tiers "+
				"(subscription_view.py:373-379) and to subscriptionOverlay.tsx:381,525",
			tier.Name, tier.TierOrder)
	}

	// The property that actually broke: the picker the user sees.
	require.Len(t, billingClientSelectableTiers(got.Tiers), 2,
		"the client's own filter must keep BOTH tiers; an empty result is the silently-blank picker C-2 describes")

	// Ranks ascend by normalised dollars-per-month and are DISTINCT, so
	// is_upgrade (subscription_view.py:409-411) can order any two rows.
	// $5 Starter ranks below $20 Pro regardless of which came first out of
	// ListPlans.
	byID := map[string]BillingTierView{}
	for _, tier := range got.Tiers {
		byID[tier.TierID] = tier
	}
	require.Equal(t, 1, byID["5"].TierOrder, "the cheaper group must rank first")
	require.Equal(t, 2, byID["9"].TierOrder)

	// Emission order is unchanged (ListPlans' own order); only the rank is
	// re-derived. The client sorts by tier_order itself on every surface.
	require.Equal(t, "9", got.Tiers[0].TierID)
	require.Equal(t, "5", got.Tiers[1].TierID)
}

// TestBillingSubscriptionTierOrderIsDistinctWhenSortOrdersCollide covers the
// SortOrder+1 shortcut the brief rules out. Two groups both left at the
// default would both become 1 under `SortOrder + 1`, and is_upgrade
// (subscription_view.py:409-411, `orders.get(tier_id, 0) > cur_order`) then
// answers False in BOTH directions between them -- the picker offers a change
// it cannot characterise. Distinctness is a contract property, not a tidiness
// one.
//
// The prices here are also deliberately NOT the raw Price column: a $180/year
// plan normalises to $15/month (billingDollarsPerMonth, ruling R-3.2) and so
// must rank BELOW the $20/month plan, even though 180 > 20. Ranking on raw
// Price would order the picker by a number the user never sees.
func TestBillingSubscriptionTierOrderIsDistinctWhenSortOrdersCollide(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.plan.plans = []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 200, GroupID: 5, Name: "Team Annual", Price: 180, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
		{ID: 300, GroupID: 3, Name: "Starter", Price: 5, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
	}

	got, err := svc.Subscription(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, got.Tiers, 3)

	seen := map[int]string{}
	for _, tier := range got.Tiers {
		require.GreaterOrEqual(t, tier.TierOrder, 1)
		prev, dup := seen[tier.TierOrder]
		require.False(t, dup, "tierOrder %d is shared by %q and %q; is_upgrade cannot order them", tier.TierOrder, prev, tier.Name)
		seen[tier.TierOrder] = tier.Name
	}

	byID := map[string]BillingTierView{}
	for _, tier := range got.Tiers {
		byID[tier.TierID] = tier
	}
	require.Equal(t, 1, byID["3"].TierOrder, "$5/mo")
	require.Equal(t, 2, byID["5"].TierOrder, "$180/yr normalises to $15/mo and must outrank neither $20/mo nor be ranked on its raw 180")
	require.Equal(t, 3, byID["9"].TierOrder, "$20/mo")
}
