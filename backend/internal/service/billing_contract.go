package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// BillingContractService answers the Nous-shaped billing questions the hermes
// client asks its portal. It is a CONTRACT ADAPTER, not billing functionality:
// every field below is a translation of something Inferno already stores, and
// the three fields Inferno has no model for are reported as absent rather than
// invented (see BillingCardView, BillingAutoReloadView and ChargePresets).
//
// It composes CACHED services, not repositories. The balance in particular
// comes from BillingCacheService.GetUserBalance (Redis, async write workers,
// singleflight on the miss path) and never from a UserRepository read: this
// endpoint is POLLED by every running agent, and reaching past the cache turns
// one cached read into a query storm.
//
// Dependencies are narrow interfaces rather than the concrete services so that
// (a) this adapter cannot grow into mutating anything, and (b) a test can fail
// exactly one source and observe the degradation. The real services satisfy
// them structurally; there is no adapter type and no second code path.
type BillingContractService struct {
	balanceSvc BillingBalanceSource
	orgSvc     BillingOrgSource
	usageSvc   BillingUsageSource
	paymentSvc BillingPaymentSource

	// portalBaseURL is cfg.Server.FrontendURL -- the browser-facing base URL
	// this deployment is reachable at. See BillingStateView.PortalURL.
	portalBaseURL string

	// now is timezone.Now in production. Injected so the month-to-date window
	// is assertable; unexported, so the seam exists only inside this package.
	now func() time.Time
}

// BillingBalanceSource is the wallet balance, read through the cache.
// Satisfied by *BillingCacheService.
type BillingBalanceSource interface {
	GetUserBalance(ctx context.Context, userID int64) (float64, error)
}

// BillingOrgSource is org membership and role. Satisfied by *OrgService --
// the same two calls OAuthHandler.Account already makes.
type BillingOrgSource interface {
	OrgsForUser(ctx context.Context, userID int64) ([]*dbent.Org, error)
	RoleIn(ctx context.Context, orgID, userID int64) (string, error)
}

// BillingUsageSource is the month-to-date usage rollup and the per-row usage
// history. Satisfied by *UsageService.
//
// ListByUser's isolation is NOT re-implemented here: the WHERE user_id = $1
// predicate lives in usageLogRepository.ListByUser
// (usage_log_repo_query.go), already covered by its own repository
// integration tests. What THIS package is responsible for is narrower and
// easy to get wrong in a different way -- passing the CALLER's verified
// userID through untouched, rather than some other value -- which is exactly
// what TestUsageReturnsOnlyTheCallersRows below pins.
type BillingUsageSource interface {
	GetStatsByUser(ctx context.Context, userID int64, startTime, endTime time.Time) (*UsageStats, error)
	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
}

// BillingPaymentSource is the top-up bounds and the payment kill switch.
// Satisfied by *PaymentConfigService.
type BillingPaymentSource interface {
	GetAvailableMethodLimits(ctx context.Context) (*MethodLimitsResponse, error)
	IsPaymentEnabled(ctx context.Context) bool
}

func NewBillingContractService(
	balanceSvc BillingBalanceSource,
	orgSvc BillingOrgSource,
	usageSvc BillingUsageSource,
	paymentSvc BillingPaymentSource,
	portalBaseURL string,
) *BillingContractService {
	return &BillingContractService{
		balanceSvc:    balanceSvc,
		orgSvc:        orgSvc,
		usageSvc:      usageSvc,
		paymentSvc:    paymentSvc,
		portalBaseURL: portalBaseURL,
		now:           timezone.Now,
	}
}

// ---------------------------------------------------------------------------
// The wire shape.
//
// THESE JSON KEYS ARE NOT OURS TO CHOOSE. The only consumer is
// agent/billing_view.py's billing_state_from_payload, which reads the raw
// object with no key transformation on the way in
// (hermes_cli/nous_billing.py:479-481). Every tag below is the exact key that
// parser reads, verified against that function -- including the two groupings
// that are easy to get backwards:
//
//   - `bounds` carries minUsd/maxUsd, the TOP-UP bounds.
//   - `monthlyCap` carries limitUsd/spentThisMonthUsd/isDefaultCeiling, the
//     monthly SPEND ceiling. It is NOT `bounds`.
//
// A wrong key here does not error. Every .get() misses, the client parses a
// well-formed BillingState full of None, and the CLI shows the same degraded
// screen it shows with no endpoint at all -- the failure OAuthHandler.Account's
// doc comment records as worse than a loud one.
// ---------------------------------------------------------------------------

// BillingStateView is GET /api/billing/state's body.
type BillingStateView struct {
	// LoggedIn is always true here: the bearer verified or this method was
	// never reached. Emitted for symmetry with the client's own field name;
	// note the client does not actually read it (billing_state_from_payload
	// hardcodes logged_in=True on any parsed payload).
	LoggedIn bool `json:"loggedIn"`

	// Org is nil when the org lookup failed. nil means "could not resolve",
	// never "zero" -- the client reads a missing org as unknown.
	Org *BillingOrgView `json:"org,omitempty"`

	// CanChangePlan is the server-granted capability the client prefers over
	// its own deprecated OWNER/ADMIN role check. Omitted (not false) when the
	// role could not be resolved, so the client falls back to that check
	// rather than being told "no" by a lookup failure.
	CanChangePlan *bool `json:"canChangePlan,omitempty"`

	// BalanceUSD is a decimal STRING, as the whole money contract is: the
	// client parses with Decimal(str(value)) and keeps it exact end-to-end.
	BalanceUSD string `json:"balanceUsd"`

	// CLIBillingEnabled is this deployment's payment kill switch. It gates the
	// client's charge/auto-reload UI (BillingState.can_charge).
	CLIBillingEnabled bool `json:"cliBillingEnabled"`

	// ChargePresets is always empty, and that is DELIBERATE, not a stub left
	// behind. There is NO server-side source for a top-up preset ladder in
	// Inferno: the only ladder in the whole product is a hardcoded Vue default
	// (frontend/src/components/payment/AmountInput.vue's `amounts` prop
	// default, [10, 20, 50, 100, 200, 500, 1000, 2000, 5000]), a frontend
	// literal rather than data any service owns. Copying it here would create a
	// second source of truth that drifts the first time either side is edited.
	// The client degrades cleanly -- with no presets it offers "Custom
	// amount…", still bounded by the real Bounds below. Giving this a real
	// source means adding a settings key both the panel and this adapter read,
	// which is a product decision, not a translation.
	ChargePresets []string `json:"chargePresets"`

	// Bounds is the top-up min/max. nil when the payment-config lookup failed.
	Bounds *BillingBoundsView `json:"bounds,omitempty"`

	Card       BillingCardView       `json:"card"`
	AutoReload BillingAutoReloadView `json:"autoReload"`

	// MonthlyCap is nil when the usage rollup failed -- deliberately nil and
	// not a zero, because "we could not add it up" and "you have spent nothing"
	// are different answers and only one of them is safe to show.
	MonthlyCap *BillingMonthlyCapView `json:"monthlyCap,omitempty"`

	// PortalURL is where the CLI sends a user who needs to top up or add a
	// card, and it is THE FIELD THAT CLOSES THE LIVE BUG this whole adapter
	// exists for -- which is why it is here despite not appearing in the task
	// brief's field list.
	//
	// When the server omits it, agent/billing_view.py:333-335 builds
	// `{portal_base}/billing?topup=open` instead. Both halves of that are wrong
	// for us: the default portal_base is portal.nousresearch.com (so our users
	// are sent to Nous's billing page to top up an Inferno wallet -- the
	// observed 2026-08-19 defect), and even pointed at the right host, /billing
	// is not a route in inferno-frontend/src/router/index.ts at all. Inferno's
	// recharge page is /purchase, which is what billingPortalURL builds.
	//
	// Omitted when this deployment has no frontend_url configured: the client
	// then falls back as before, which is better than being handed a relative
	// or half-formed absolute URL it will resolve against the Nous default.
	PortalURL string `json:"portalUrl,omitempty"`
}

// BillingOrgView is the client's org block: id, slug, name, role.
type BillingOrgView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Role is "" when the per-org role lookup failed. Never defaulted to a
	// role -- OAuthHandler.Account defaults to MEMBER for the same situation
	// because its payload has no way to say "unknown"; this one does.
	Role string `json:"role,omitempty"`
}

// BillingBoundsView is the top-up amount range. Both fields are pointers so a
// missing bound is null rather than "0": MethodLimitsResponse uses 0 to mean
// "no minimum"/"no maximum", and "0" would read to the client as a real
// zero-dollar bound.
type BillingBoundsView struct {
	MinUSD *string `json:"minUsd"`
	MaxUSD *string `json:"maxUsd"`
}

// BillingCardView is always {"kind":"none"}.
//
// Inferno's payment model is order-based -- create an order, pay it through a
// provider, the order settles. There is no stored-card vault and no
// payment_method_id anywhere in this codebase. "none" is the honest answer and
// the client already handles it: with no card, cli_billing_mixin.py routes the
// user to the portal/order flow instead of offering a one-click charge that
// would have nothing to charge.
type BillingCardView struct {
	Kind string `json:"kind"`
}

// BillingAutoReloadView is always {"enabled":false}. Auto top-up does not exist
// in Inferno, and it would need a stored payment method to mean anything, so it
// inherits BillingCardView's gap.
type BillingAutoReloadView struct {
	Enabled bool `json:"enabled"`
}

// BillingMonthlyCapView is the monthly spend picture.
//
// LimitUSD is always null: a per-org monthly ceiling is not modelled in
// Inferno. Null reads to the client as "no limit configured", which is true;
// IsDefaultCeiling is false for the same reason -- there is no default ceiling
// to have fallen back to.
//
// SpentThisMonthUSD IS real, aggregated from usage_logs.
type BillingMonthlyCapView struct {
	LimitUSD          *string `json:"limitUsd"`
	SpentThisMonthUSD string  `json:"spentThisMonthUsd"`
	IsDefaultCeiling  bool    `json:"isDefaultCeiling"`
}

// ---------------------------------------------------------------------------
// GET /api/analytics/usage's body.
//
// UNLIKE BillingStateView's keys, these are NOT pinned against a live client
// parser. hermes_cli/nous_billing.py -- the module that owns every real
// /api/billing/* call the client makes -- has no function that requests
// /api/analytics/usage, and the only occurrence of that literal path
// anywhere in the checked-out hermes-agent client
// (hermes_cli/web_server.py's own local FastAPI app) is the CLI's OWN
// desktop-dashboard route, served from its local sqlite session database --
// an entirely different, non-networked surface, not a call to this server.
// Searched: hermes_cli/{nous_billing,nous_account,nous_subscription,
// cli_billing_mixin}.py and agent/{billing_view,billing_usage,
// account_usage}.py. See task-2-report.md for the full trace.
//
// So these camelCase keys follow this package's own established convention
// (BillingStateView) rather than a verified consumer, on the working theory
// that the design doc's contract table anticipates a client revision that
// hasn't shipped in this checkout. If a real parser surfaces later, treat
// this shape as provisional and reconcile it the same way F1 caught
// BillingStateView's wrong snake_case keys.
// ---------------------------------------------------------------------------

// UsageView is GET /api/analytics/usage's body: one page of the caller's own
// usage history, newest first (UsageService.ListByUser's default order).
type UsageView struct {
	// Items is never nil -- JSON null is not an empty list. Each item is
	// this caller's own row; see TestUsageReturnsOnlyTheCallersRows.
	Items []UsageItemView `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	// PageSize echoes pagination.PaginationParams.Limit()'s normalized
	// value, not whatever the caller asked for -- so a client that requested
	// page_size=0 or 100000 sees what it actually got.
	PageSize int `json:"pageSize"`
}

// UsageItemView is one usage_logs row, translated to the money-as-string
// convention the rest of this adapter uses (see BillingStateView.BalanceUSD).
type UsageItemView struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"userId"`
	Model  string `json:"model"`

	InputTokens         int `json:"inputTokens"`
	OutputTokens        int `json:"outputTokens"`
	CacheCreationTokens int `json:"cacheCreationTokens"`
	CacheReadTokens     int `json:"cacheReadTokens"`

	// TotalCostUSD is the pre-multiplier list price; ActualCostUSD is what
	// was actually debited from the wallet (actual_cost -- see
	// billing_contract.go's resolveMonthlyCap doc comment for why the two
	// are not interchangeable).
	TotalCostUSD  string `json:"totalCostUsd"`
	ActualCostUSD string `json:"actualCostUsd"`

	CreatedAt time.Time `json:"createdAt"`
}

// Usage composes GET /api/analytics/usage. Unlike State, there is no partial
// result to degrade to: the entire response IS the usage list, so a failure
// here is fatal and answers 500, logged loud -- same rule as State's balance
// section, for the same reason (see State's doc comment).
func (s *BillingContractService) Usage(ctx context.Context, userID int64, p pagination.PaginationParams) (*UsageView, error) {
	logs, page, err := s.usageSvc.ListByUser(ctx, userID, p)
	if err != nil {
		return nil, fmt.Errorf("billing contract: list usage for user %d: %w", userID, err)
	}

	items := make([]UsageItemView, 0, len(logs))
	for _, l := range logs {
		items = append(items, UsageItemView{
			ID:                  l.ID,
			UserID:              l.UserID,
			Model:               l.Model,
			InputTokens:         l.InputTokens,
			OutputTokens:        l.OutputTokens,
			CacheCreationTokens: l.CacheCreationTokens,
			CacheReadTokens:     l.CacheReadTokens,
			TotalCostUSD:        billingMoney(l.TotalCost),
			ActualCostUSD:       billingMoney(l.ActualCost),
			CreatedAt:           l.CreatedAt,
		})
	}

	out := &UsageView{
		Items:    items,
		Page:     p.Page,
		PageSize: p.Limit(),
	}
	if page != nil {
		out.Total = page.Total
		if page.Page > 0 {
			out.Page = page.Page
		}
		if page.PageSize > 0 {
			out.PageSize = page.PageSize
		}
	}
	return out, nil
}

// State composes the client's overview screen.
//
// PARTIAL RESULTS ARE THE NORMAL CASE. Each optional section resolves
// independently: a failure logs the real error and leaves that section nil, and
// nil means "could not resolve", never "zero". Only the balance is fatal --
// without it there is nothing useful to say, and every other field is decoration
// around a number the user cannot see.
//
// That asymmetry is deliberate and it is the inverse of the client's. The client
// FAILS OPEN: agent/billing_view.py::build_billing_state turns any error into
// BillingState(logged_in=False) plus a clean message rather than a crash. Because
// it will swallow whatever we send, we must be the loud half -- log the real
// error server-side, return every field that did resolve. A balance is useful
// even when the usage rollup is down; 500ing the whole response because one
// aggregate failed converts a degraded screen into a blank one.
func (s *BillingContractService) State(ctx context.Context, userID int64) (*BillingStateView, error) {
	// FATAL. Not "log and continue": see the doc comment above.
	balance, err := s.balanceSvc.GetUserBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("billing contract: resolve balance for user %d: %w", userID, err)
	}

	out := &BillingStateView{
		LoggedIn:      true,
		BalanceUSD:    billingMoney(balance),
		ChargePresets: []string{}, // never nil -- JSON null is not an empty list
		Card:          BillingCardView{Kind: "none"},
		AutoReload:    BillingAutoReloadView{Enabled: false},
		PortalURL:     billingPortalURL(s.portalBaseURL),
	}

	out.Org, out.CanChangePlan = s.resolveOrg(ctx, userID)
	out.MonthlyCap = s.resolveMonthlyCap(ctx, userID)
	out.Bounds, out.CLIBillingEnabled = s.resolvePayment(ctx)

	return out, nil
}

// resolveOrg returns the primary org and the plan-change capability, or
// (nil, nil) when the org could not be resolved.
func (s *BillingContractService) resolveOrg(ctx context.Context, userID int64) (*BillingOrgView, *bool) {
	orgs, err := s.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("billing contract: org lookup failed", "user_id", userID, "error", err)
		return nil, nil
	}
	if len(orgs) == 0 {
		// Post-C1 every session provisions a personal org, so this is not
		// expected; a missing org is still "unknown", not an empty one.
		slog.Error("billing contract: user has no org", "user_id", userID)
		return nil, nil
	}

	primary := orgs[0]
	view := &BillingOrgView{
		ID:   strconv.FormatInt(primary.ID, 10),
		Slug: primary.Slug,
		Name: primary.Name,
	}

	role, err := s.orgSvc.RoleIn(ctx, primary.ID, userID)
	if err != nil {
		// The org half is still correct and useful. Leave Role empty and omit
		// canChangePlan entirely, so the client falls back to its own role
		// check instead of being told "no" by a lookup failure.
		slog.Error("billing contract: org role lookup failed", "user_id", userID, "org_id", primary.ID, "error", err)
		return view, nil
	}
	view.Role = role

	canChangePlan := role == OrgRoleOwner || role == OrgRoleAdmin
	return view, &canChangePlan
}

// resolveMonthlyCap aggregates month-to-date spend, or returns nil when the
// rollup failed.
func (s *BillingContractService) resolveMonthlyCap(ctx context.Context, userID int64) *BillingMonthlyCapView {
	now := s.now()
	start := timezone.StartOfMonth(now)

	stats, err := s.usageSvc.GetStatsByUser(ctx, userID, start, now)
	if err != nil {
		slog.Error("billing contract: month-to-date usage rollup failed", "user_id", userID, "error", err)
		return nil
	}
	if stats == nil {
		slog.Error("billing contract: month-to-date usage rollup returned no stats", "user_id", userID)
		return nil
	}

	return &BillingMonthlyCapView{
		LimitUSD: nil,
		// TotalActualCost, not TotalCost: actual_cost is the column
		// UsageService.Create deducts from the wallet, so it is the only one
		// that means the same thing as the balance shown beside it.
		SpentThisMonthUSD: billingMoney(stats.TotalActualCost),
		IsDefaultCeiling:  false,
	}
}

// resolvePayment returns the top-up bounds and the payment kill switch. On a
// limits failure the bounds are nil, but cliBillingEnabled is still answered --
// it is a separate settings read.
func (s *BillingContractService) resolvePayment(ctx context.Context) (*BillingBoundsView, bool) {
	enabled := s.paymentSvc.IsPaymentEnabled(ctx)

	limits, err := s.paymentSvc.GetAvailableMethodLimits(ctx)
	if err != nil {
		slog.Error("billing contract: payment limits lookup failed", "error", err)
		return nil, enabled
	}
	if limits == nil {
		return nil, enabled
	}

	return &BillingBoundsView{
		MinUSD: billingOptionalMoney(limits.GlobalMin),
		MaxUSD: billingOptionalMoney(limits.GlobalMax),
	}, enabled
}

// billingMoney renders a money value as the decimal STRING the contract calls
// for. 'f' with precision -1 is the shortest representation that round-trips,
// so 42.50 renders "42.5" and 1.25 renders "1.25" -- never scientific notation,
// and never a fixed 2dp that would silently drop the sub-cent per-request costs
// this system actually meters. The client quantises for display.
func billingMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// billingOptionalMoney maps MethodLimitsResponse's "0 means unset" convention
// onto a JSON null, which is what the client reads as "no bound".
func billingOptionalMoney(v float64) *string {
	if v <= 0 {
		return nil
	}
	s := billingMoney(v)
	return &s
}

// billingPortalURL builds the top-up deep link from this deployment's
// browser-facing base URL. /purchase is Inferno's recharge route
// (inferno-frontend/src/router/index.ts) -- deliberately not the client's
// /billing fallback, which does not exist here.
func billingPortalURL(base string) string {
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/purchase"
}
