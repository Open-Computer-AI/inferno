package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"

	"github.com/stretchr/testify/require"
)

// firstPage is the pagination request TestUsageReturnsOnlyTheCallersRows and
// its siblings use -- page 1, the service's own default page size.
var firstPage = pagination.PaginationParams{Page: 1, PageSize: 20}

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

	// records is the fake's "database" for ListByUser -- every recorded row,
	// across every user, exactly as a real usage_logs table would hold them.
	// A second user's rows belong HERE, genuinely present, not asserted away
	// by construction; ListByUser's own WHERE-equivalent filter is what
	// TestUsageReturnsOnlyTheCallersRows is actually pinning (see the
	// mutation note on that test).
	records []usageRecord
	listErr error

	gotListUserID int64
	gotListParams pagination.PaginationParams
	listCalls     int
}

// usageRecord is one row in the fake's in-memory table.
type usageRecord struct {
	userID     int64
	actualCost float64
}

func (f *fakeBillingUsageSource) GetStatsByUser(_ context.Context, _ int64, start, end time.Time) (*UsageStats, error) {
	f.calls++
	f.gotStart, f.gotEnd = start, end
	if f.err != nil {
		return nil, f.err
	}
	return &UsageStats{TotalActualCost: f.actualCost}, nil
}

// ListByUser is the isolation boundary: it must return only rows whose
// userID matches the requested one. See TestUsageReturnsOnlyTheCallersRows'
// mutation note for why this predicate, specifically, is the one that has to
// be provably load-bearing.
func (f *fakeBillingUsageSource) ListByUser(_ context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	f.listCalls++
	f.gotListUserID = userID
	f.gotListParams = params
	if f.listErr != nil {
		return nil, nil, f.listErr
	}

	var out []UsageLog
	for _, r := range f.records {
		if r.userID != userID {
			continue
		}
		out = append(out, UsageLog{UserID: r.userID, ActualCost: r.actualCost, TotalCost: r.actualCost})
	}
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.Limit(),
	}, nil
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

// billingContractFixture holds the fakes so a test can reach in and break one
// source at a time -- that is what the partial-degradation test needs.
type billingContractFixture struct {
	balance *fakeBillingBalanceSource
	org     *fakeBillingOrgSource
	usage   *fakeBillingUsageSource
	payment *fakeBillingPaymentSource
	now     time.Time
}

// recordUsage records one usage row for userID, and (for backward
// compatibility with the State/MonthlyCap tests, which only ever exercise
// one user) also sets the month-to-date ACTUAL cost GetStatsByUser reports --
// actual_cost, not total_cost, because actual_cost is the column
// UsageService.Create deducts from the wallet (usage_service.go:124-130).
// "Spent" must mean the same thing the balance means or the two numbers on
// one screen contradict each other.
func (f *billingContractFixture) recordUsage(userID int64, amount float64) {
	f.usage.actualCost = amount
	f.usage.records = append(f.usage.records, usageRecord{userID: userID, actualCost: amount})
}

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
		now: time.Date(2026, 8, 19, 13, 45, 0, 0, time.UTC),
	}

	svc := NewBillingContractService(fx.balance, fx.org, fx.usage, fx.payment, "https://portal.example.com")
	svc.now = func() time.Time { return fx.now }
	return svc, fx
}

// ---------------------------------------------------------------------------

func TestBillingStateReportsBalanceOrgAndSpend(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 1.25)

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
	fx.recordUsage(7, 1.25)

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
// GET /api/analytics/usage
// ---------------------------------------------------------------------------

// TestUsageReturnsOnlyTheCallersRows is the important test in this file. A
// second real user's usage row is recorded in the fake's "database" alongside
// the caller's own -- without it, this test would pass against an
// implementation with no filter at all, because there would be nothing else
// in the store for a leak to surface.
//
// MUTATION CHECK (task-2-report.md has the exact diff and failing output):
// deleting the `if r.userID != userID { continue }` line from
// fakeBillingUsageSource.ListByUser -- so the fake returns every recorded row
// regardless of who asked -- still compiles, and fails this test's
// require.Len(t, got.Items, 1) with len(got.Items) == 2, one of them user 8's
// $99 row. That is what proves the isolation this test asserts is real and
// not a tautology of the fixture only ever holding one user's data.
func TestUsageReturnsOnlyTheCallersRows(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 1.00)  // ours
	fx.recordUsage(8, 99.00) // a second real user, in the same database

	got, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, int64(7), got.Items[0].UserID)
}

// TestUsageThreadsTheCallersUserIDToListByUser is the direct version of the
// same guarantee: the userID BillingContractService.Usage was called with is
// exactly the userID that reaches the source, never a request-supplied or
// otherwise substituted one.
func TestUsageThreadsTheCallersUserIDToListByUser(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 1.00)

	_, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)

	require.Equal(t, 1, fx.usage.listCalls)
	require.Equal(t, int64(7), fx.usage.gotListUserID)
}

// TestUsageMapsRowFieldsToTheContractShape pins the per-item translation:
// tokens copied straight across, both cost fields rendered as the decimal
// STRING convention this whole adapter uses (see BillingStateView.BalanceUSD),
// and TotalCostUSD/ActualCostUSD kept distinct rather than collapsed into one
// field, because they answer different questions (list price vs. what was
// actually debited -- see resolveMonthlyCap's doc comment).
func TestUsageMapsRowFieldsToTheContractShape(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 3.5)

	got, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)

	item := got.Items[0]
	require.Equal(t, int64(7), item.UserID)
	require.Equal(t, "3.5", item.TotalCostUSD)
	require.Equal(t, "3.5", item.ActualCostUSD)
}

// TestUsageReportsPaginationMetadata: total/page/pageSize must reflect what
// the source actually returned, not just echo the request -- a client using
// total to decide whether to fetch another page needs the real count.
func TestUsageReportsPaginationMetadata(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 1.00)
	fx.recordUsage(7, 2.00)

	got, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)

	require.Equal(t, int64(2), got.Total)
	require.Equal(t, 1, got.Page)
	require.Equal(t, 20, got.PageSize)
}

// TestUsageReturnsAnEmptyNotNilListWhenThereIsNoUsage: JSON null and JSON []
// are different wire values, and a client that does `for item in items`
// crashes on null. Mirrors BillingStateView.ChargePresets' same rule.
func TestUsageReturnsAnEmptyNotNilListWhenThereIsNoUsage(t *testing.T) {
	svc, _ := newBillingContractFixture(t)

	got, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)
	require.NotNil(t, got.Items)
	require.Empty(t, got.Items)
}

// TestUsageFailsWhenTheListLookupFails: unlike State, there is no partial
// result to fall back to here -- the usage list IS the response, so a failed
// lookup is fatal and must be reported as an error the handler turns into
// 500, not swallowed into an empty-looking 200.
func TestUsageFailsWhenTheListLookupFails(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.usage.listErr = errors.New("usage_logs query timed out")

	got, err := svc.Usage(context.Background(), 7, firstPage)

	require.Error(t, err)
	require.Nil(t, got)
	require.ErrorContains(t, err, "list usage")
}
