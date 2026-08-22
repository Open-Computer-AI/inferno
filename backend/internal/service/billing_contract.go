package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
	planSvc    BillingPlanSource
	subSvc     BillingSubscriptionSource

	// chargeSvc and idempotency back the two IMPLEMENTED writes (Task 5,
	// ruling R-5.1): POST /api/billing/charge and GET /api/billing/charge/{id}.
	// See the "The 2 implementable writes" section below.
	chargeSvc   BillingChargeOrderSource
	idempotency BillingIdempotencyExecutor

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

// BillingUsageSource is the month-to-date usage rollup. Satisfied by
// *UsageService.
type BillingUsageSource interface {
	GetStatsByUser(ctx context.Context, userID int64, startTime, endTime time.Time) (*UsageStats, error)
}

// BillingPaymentSource is the top-up bounds and the payment kill switch.
// Satisfied by *PaymentConfigService.
type BillingPaymentSource interface {
	GetAvailableMethodLimits(ctx context.Context) (*MethodLimitsResponse, error)
	IsPaymentEnabled(ctx context.Context) bool
}

// BillingPlanSource is the purchasable-plan catalog used to build the
// GET /api/billing/subscription tier picker. Satisfied by
// *PaymentConfigService -- the SAME concrete service as BillingPaymentSource,
// but kept as its own narrow interface (not folded into BillingPaymentSource)
// so a test can break the plan catalog without also breaking the top-up
// bounds, matching the rest of this file's one-interface-per-concern rule.
type BillingPlanSource interface {
	// ListPlans returns EVERY plan, not just ForSale ones: a grandfathered
	// plan the caller is still on but that can no longer be purchased must
	// still appear in `tiers` (isEnabled:false) or the picker cannot render
	// the caller's own current row. See resolveTiers.
	ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error)
	GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo
}

// BillingSubscriptionSource is the caller's own active subscriptions.
// Satisfied by *SubscriptionService.
type BillingSubscriptionSource interface {
	ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error)
}

// BillingChargeOrderSource creates and reads the order-based charges that
// stand in for Nous's Stripe "charge" concept (ruling R-5.1: Inferno is
// order-based, not card-based, across ALL SIX of its payment providers --
// EasyPay, Alipay, Wxpay, Stripe, Airwallex, Razorpay
// (payment_config_providers.go:182-183). Stripe IS integrated here, but as a
// CHECKOUT integration, not a stored-card one: payment_order.go's provider
// snapshot (buildPaymentOrderProviderSnapshot) only records currency for it,
// same as the others, and customer_id/payment_method_id/setup_intent/
// off_session appear NOWHERE in ent/schema or the payment services --
// confirmed by grep, not assumed. That absence, on every provider including
// Stripe, is what makes R-5.1's refusal correct: SubscriptionUpgrade needs an
// off-session charge against a card already on file, and no provider here
// has one). Satisfied structurally by *PaymentService --
// both methods already exist there with this exact signature
// (payment_order.go's CreateOrder, :25, and GetOrder, :918).
//
// GetOrder's own contract is why GET /api/billing/charge/{id} can answer the
// "never 404, never another org's data" rule with no extra work here: it
// already returns infraerrors.NotFound for an unknown id and
// infraerrors.Forbidden for one owned by a different user (:918-927) --
// ChargeStatus below collapses BOTH into the client's "pending", never
// distinguishing them on the wire.
type BillingChargeOrderSource interface {
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error)
	GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error)
}

// BillingIdempotencyExecutor is the "reuse the same key on retry -> the same
// chargeId, never a second order" guarantee
// (hermes_cli/nous_billing.py:511-521,529). Satisfied by
// *IdempotencyCoordinator, this codebase's OWN generic idempotency mechanism
// (internal/service/idempotency.go) -- already migrated, already used
// elsewhere for exactly this "don't double-execute a POST" problem, so this
// is reuse, not a bespoke second implementation.
//
// A narrow interface (not the concrete *IdempotencyCoordinator) for the same
// reason every other dependency on this struct is one: a test can fail the
// idempotency store in isolation without touching the other five sources.
type BillingIdempotencyExecutor interface {
	Execute(ctx context.Context, opts IdempotencyExecuteOptions, execute func(context.Context) (any, error)) (*IdempotencyExecuteResult, error)
}

func NewBillingContractService(
	balanceSvc BillingBalanceSource,
	orgSvc BillingOrgSource,
	usageSvc BillingUsageSource,
	paymentSvc BillingPaymentSource,
	planSvc BillingPlanSource,
	subSvc BillingSubscriptionSource,
	chargeSvc BillingChargeOrderSource,
	idempotency BillingIdempotencyExecutor,
	portalBaseURL string,
) *BillingContractService {
	return &BillingContractService{
		balanceSvc:    balanceSvc,
		orgSvc:        orgSvc,
		usageSvc:      usageSvc,
		paymentSvc:    paymentSvc,
		planSvc:       planSvc,
		subSvc:        subSvc,
		chargeSvc:     chargeSvc,
		idempotency:   idempotency,
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

	_, out.Org, out.CanChangePlan = s.resolvePrimaryOrg(ctx, userID)
	out.MonthlyCap = s.resolveMonthlyCap(ctx, userID)
	out.Bounds, out.CLIBillingEnabled = s.resolvePayment(ctx)

	return out, nil
}

// resolvePrimaryOrg returns the caller's primary org record, its wire view,
// and the plan-change capability. All three are nil/empty together only when
// the org could not be resolved at all; the raw record is returned alongside
// the view so a caller that needs a field the view does not carry (Subscription
// needs IsPersonal, for `context`) does not have to query a second time.
func (s *BillingContractService) resolvePrimaryOrg(ctx context.Context, userID int64) (*dbent.Org, *BillingOrgView, *bool) {
	orgs, err := s.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("billing contract: org lookup failed", "user_id", userID, "error", err)
		return nil, nil, nil
	}
	if len(orgs) == 0 {
		// Post-C1 every session provisions a personal org, so this is not
		// expected; a missing org is still "unknown", not an empty one.
		slog.Error("billing contract: user has no org", "user_id", userID)
		return nil, nil, nil
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
		return primary, view, nil
	}
	view.Role = role

	canChangePlan := role == OrgRoleOwner || role == OrgRoleAdmin
	return primary, view, &canChangePlan
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

// billingDisplayMoney formats a value the client renders VERBATIM into a label,
// as opposed to one it parses for arithmetic. Two decimal places, always.
//
// billingMoney uses shortest-round-trip precision, which is right for a stored
// balance (the client parses it with Decimal and wants it exact) and WRONG for
// dollarsPerMonthDisplay, which is the result of a DIVISION: a $200/12-month
// plan normalises to 16.666666666666668, and ui-tui's subscriptionOverlay.tsx:437
// interpolates that string straight into
//
//	`${tier.name} · ${tier.dollars_per_month_display}/mo`
//
// so the user would read "Pro Annual · 16.666666666666668/mo". Caught by the
// Task 6 conformance run against real seeded plans; every unit test passed with
// the raw value because the SHAPE was correct and only the rendering was wrong.
func billingDisplayMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
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

// ---------------------------------------------------------------------------
// GET /api/billing/subscription -- the CLI's /subscription screen.
//
// THESE JSON KEYS ARE NOT OURS TO CHOOSE EITHER. The only consumer is
// agent/subscription_view.py's subscription_state_from_payload (:214), plus
// its helpers _parse_current (:141) and _parse_tier (:176). Every field below
// is the exact camelCase key that parser reads. Two of them fail SILENTLY on
// a type mistake, same as billing/state:
//
//   - `tiers` must be a JSON array, even when empty ([], never null) -- a
//     non-list silently becomes () (subscription_view.py:224-229), an empty
//     plan picker with no error anywhere.
//   - `canChangePlan` must be a real JSON bool -- a string "true" parses to
//     None, not true (:236-240).
//
// `current` is the other sharp edge: "no plan" is wire-represented as
// `current: null`, NOT an object of nulls (the old all-null-object shape is
// gone per the comment at subscription_view.py:142-144). A present `current`
// is discarded entirely unless it carries a truthy tierId/id (:147-149).
// ---------------------------------------------------------------------------

// BillingSubscriptionView is GET /api/billing/subscription's body.
type BillingSubscriptionView struct {
	// Org mirrors BillingStateView.Org -- same struct, same resolution. The
	// subscription parser only reads .name/.id/.role from it
	// (subscription_view.py:218-220); .slug is unread here but still a real,
	// cited field (billing_view.py, Task 1), not an invented one.
	Org *BillingOrgView `json:"org,omitempty"`

	// Context is EXACTLY "personal" or "team" -- any other string silently
	// becomes "personal" on the client (subscription_view.py:221-222), so an
	// unresolved org still emits a valid value rather than an empty string.
	Context string `json:"context"`

	// Tiers is the selectable plan catalog. Never nil: an empty Go slice
	// marshals to `[]`, which is what a `list` guard downstream requires.
	Tiers []BillingTierView `json:"tiers"`

	// CanChangePlan mirrors BillingStateView.CanChangePlan: omitted (not
	// false) when the role lookup failed, so the client falls back to its own
	// role check instead of being told "no" by a lookup failure.
	CanChangePlan *bool `json:"canChangePlan,omitempty"`

	// Current is the caller's active subscription, or nil (-> JSON `null`)
	// when there is none. Deliberately NOT omitempty: the client's "no plan"
	// state is a present `current: null` key, and an omitted key is only
	// coincidentally equivalent (payload.get("current") on a missing key also
	// returns None) -- being explicit is what the type-fidelity test pins.
	Current *BillingCurrentSubscriptionView `json:"current"`

	// PortalURL reuses BillingStateView's exact deep link (billingPortalURL):
	// read at agent/subscription_view.py:292 by the CALLER of
	// subscription_state_from_payload (build_subscription_state), not by the
	// parser itself, but it is a real read site all the same. Downstream,
	// subscription_manage_url (subscription_view.py:302-345) only keeps this
	// URL's scheme+host and replaces the path with /manage-subscription, so
	// the exact path we send here (/purchase, same as billing/state) does not
	// matter -- only that host is never nousresearch.com.
	PortalURL string `json:"portalUrl,omitempty"`
}

// BillingTierView is one row of the plan picker
// (subscription_view.py's _parse_tier, :176-189).
type BillingTierView struct {
	// TierID is the owning GROUP's id, not a SubscriptionPlan id. Inferno has
	// no subscription-to-plan link -- a UserSubscription is keyed to
	// (userID, groupID) -- so the only identity BillingCurrentSubscriptionView
	// can ever report is the group. For the client's `tier.tier_id ===
	// current.tier_id` picker-highlight match to work AT ALL, every tier row's
	// id must be that same group id, not the underlying plan's own id.
	TierID string `json:"tierId"`
	Name   string `json:"name"`
	// TierOrder is a 1-BASED RANK this adapter computes (billingAssignTierOrder),
	// NOT SubscriptionPlan.SortOrder.
	//
	// It carried SortOrder until C-2. SortOrder's schema default is 0
	// (ent/schema/subscription_plan.go:62), CreatePlanRequest.SortOrder has no
	// binding:"required" (payment_config_service.go:177), and Inferno's own plan
	// editor posts 0 (inferno-frontend/src/views/admin/orders/PlanEditDialog.vue:126,
	// :182, :199) -- so plans created through the panel without an admin typing a
	// number all arrive here as 0. Every client surface then DROPS them:
	// agent/subscription_view.py:373-379 selectable_tiers keeps only
	// `is_enabled and not is_current and (t.tier_order or 0) > 0`, and
	// ui-tui/src/components/subscriptionOverlay.tsx:381 and :525 apply the same
	// `tier.tier_order > 0` filter. The result was a well-formed, non-empty
	// tiers[] array that rendered as an EMPTY plan picker, with no error anywhere.
	//
	// tierOrder is a RANK on the client, not a display hint: subscription_view.py's
	// is_upgrade (:409-411) compares it to decide whether a pick is an upgrade, and
	// both surfaces SORT by it. 0 is the client's encoding for "free / no
	// subscription", which on Inferno is the ABSENCE of a subscription
	// (subscriptionOverlay.tsx:370, `isFree = !c?.tier_id`), never a catalogue row
	// -- so every row this adapter emits is a paid tier and must rank >= 1.
	TierOrder int `json:"tierOrder"`
	// DollarsPerMonthDisplay is the plan's Price NORMALISED to a 30-day rate by
	// billingDollarsPerMonth (ruling R-3.2) and rendered to exactly 2 decimal
	// places by billingDisplayMoney -- an annual plan priced 1200 reports
	// "100.00", not "1200". The field's NAME asserts a unit, so it has to mean it;
	// the panel's own /api/v1/payment/plans (payment_handler.go's GetPlans) shows
	// the raw Price instead, but it shows it beside a validity label and asserts no
	// unit, which this key does.
	DollarsPerMonthDisplay string `json:"dollarsPerMonthDisplay"`
	// MonthlyCredits is absent when the plan's group has no MonthlyLimitUSD
	// configured (an uncapped group has no "credits" concept to report) --
	// same not-invented pattern as BillingStateView.MonthlyCap.LimitUSD.
	MonthlyCredits *string `json:"monthlyCredits,omitempty"`
	IsCurrent      bool    `json:"isCurrent"`
	// IsEnabled is the plan's ForSale flag. False marks a grandfathered plan
	// the caller may still be on but that can no longer be newly selected --
	// exactly the semantics _parse_tier's isEnabled documents.
	IsEnabled bool `json:"isEnabled"`
}

// BillingCurrentSubscriptionView is the caller's active plan
// (subscription_view.py's _parse_current, :141-159).
//
// pendingDowngradeTierName, pendingDowngradeAt and cancellationEffectiveAt are
// DELIBERATELY ABSENT FIELDS, not zero values: Inferno's UserSubscription has
// no scheduled-downgrade or cancellation model at all (service/user_subscription.go
// has no such column), so there is nothing to report and inventing "never
// pending" would be indistinguishable from "we don't know". CancelAtPeriodEnd
// is the one exception -- see its own comment.
type BillingCurrentSubscriptionView struct {
	// TierID is the GROUP id -- see BillingTierView.TierID's comment. Always
	// non-empty when Current is non-nil: strconv.FormatInt on a real group id
	// can never produce the empty string the client's truthy-tierId guard
	// (subscription_view.py:147-149) would discard.
	TierID   string `json:"tierId"`
	TierName string `json:"tierName,omitempty"`

	// MonthlyCredits / CreditsRemaining map Group.MonthlyLimitUSD (the plan's
	// monthly spend ceiling) and MonthlyLimitUSD-minus-MonthlyUsageUSD. This is
	// a judgment call, not a literal reading: Inferno has no "credits" unit
	// separate from its dollar balance, so the group's own monthly USD cap is
	// the closest honest analog -- and, like MonthlyCredits, both are absent
	// (nil) when the group has no cap, because "uncapped" has no credits
	// figure to report. CreditsRemaining is clamped at 0 rather than going
	// negative, and is NEVER nil merely because it IS zero -- a real 0
	// remaining is a different fact than "unknown", so this does not reuse
	// billingOptionalMoney (which treats 0 as "no bound").
	MonthlyCredits   *string `json:"monthlyCredits,omitempty"`
	CreditsRemaining *string `json:"creditsRemaining,omitempty"`

	// CycleEndsAt is the subscription's ExpiresAt, RFC 3339. The parser stores
	// it as an opaque string with no format validation (subscription_view.py:150),
	// so any ISO 8601 rendering is safe; RFC3339 matches every other timestamp
	// this codebase emits over JSON.
	CycleEndsAt string `json:"cycleEndsAt,omitempty"`

	// CancelAtPeriodEnd is always false. Unlike the three omitted fields
	// above, this one IS always knowable: Inferno has no cancellation flow at
	// all, so "is a cancellation scheduled" is always, definitely, no --
	// not "unknown". bool(raw.get(...)) on a missing key also defaults to
	// False, so this is emitted explicitly rather than relying on that.
	CancelAtPeriodEnd bool `json:"cancelAtPeriodEnd"`
}

// Subscription composes the client's /subscription screen.
//
// Unlike State, nothing here is fatal: there is no single field this endpoint
// cannot proceed without (no analogue of "the balance"). Every section
// degrades independently -- log the real error, leave that section at its
// zero value (nil org, "personal" context, empty tiers, null current) -- so
// this never 500s. The error return exists for signature symmetry with State
// and as a seam for a future genuinely-fatal source; nothing today triggers it.
func (s *BillingContractService) Subscription(ctx context.Context, userID int64) (*BillingSubscriptionView, error) {
	out := &BillingSubscriptionView{
		Context:   "personal",
		Tiers:     []BillingTierView{},
		PortalURL: billingPortalURL(s.portalBaseURL),
	}

	primaryOrg, orgView, canChangePlan := s.resolvePrimaryOrg(ctx, userID)
	out.Org = orgView
	out.CanChangePlan = canChangePlan
	if primaryOrg != nil && !primaryOrg.IsPersonal {
		out.Context = "team"
	}

	current, activeGroupID, hasActive := s.resolveCurrentSubscription(ctx, userID)
	out.Current = current
	out.Tiers = s.resolveTiers(ctx, activeGroupID, hasActive)

	return out, nil
}

// resolveCurrentSubscription returns the caller's current plan (nil when the
// caller has none or the lookup failed), the group id it belongs to, and
// whether one was actually found -- the group id alone cannot distinguish
// "no subscription" from "subscription in group 0", though group ids in
// practice start above 0.
func (s *BillingContractService) resolveCurrentSubscription(ctx context.Context, userID int64) (*BillingCurrentSubscriptionView, int64, bool) {
	subs, err := s.subSvc.ListActiveUserSubscriptions(ctx, userID)
	if err != nil {
		slog.Error("billing contract: active subscription lookup failed", "user_id", userID, "error", err)
		return nil, 0, false
	}
	if len(subs) == 0 {
		return nil, 0, false
	}

	sub := subs[0]
	if sub.Group == nil {
		// ListActiveUserSubscriptions eager-loads Group; a nil Group here
		// means the row is unusable for identity purposes -- degrade rather
		// than report a subscription with no name and no group id.
		slog.Error("billing contract: active subscription has no group loaded", "user_id", userID, "subscription_id", sub.ID)
		return nil, 0, false
	}

	// TierName comes from the same representative plan resolveTiers picks for
	// this group, NOT from Group.Name. They share a tierId, so a user whose
	// tiers[] entry reads "Pro Annual" must not see their CURRENT plan named
	// "Test " (the group's internal admin label). Falls back to the group name
	// only when the group sells no plans. Caught by Task 6 conformance.
	tierName := sub.Group.Name
	if rep, ok := s.representativePlanName(ctx, sub.GroupID); ok {
		tierName = rep
	}

	view := &BillingCurrentSubscriptionView{
		TierID:            strconv.FormatInt(sub.GroupID, 10),
		TierName:          tierName,
		CycleEndsAt:       sub.ExpiresAt.UTC().Format(time.RFC3339),
		CancelAtPeriodEnd: false,
	}

	if sub.Group.MonthlyLimitUSD != nil {
		limit := *sub.Group.MonthlyLimitUSD
		limitStr := billingMoney(limit)
		view.MonthlyCredits = &limitStr

		remaining := limit - sub.MonthlyUsageUSD
		if remaining < 0 {
			remaining = 0
		}
		remainingStr := billingMoney(remaining)
		view.CreditsRemaining = &remainingStr
	}

	return view, sub.GroupID, true
}

// resolveTiers builds the plan picker from EVERY plan (not just ForSale ones
// -- see BillingPlanSource.ListPlans), COLLAPSED TO ONE ENTRY PER GROUP
// (ruling R-3.1).
//
// subscription_plan.group_id is NOT unique (ent/schema/subscription_plan.go's
// index.Fields("group_id") carries no .Unique()): one group can sell at
// several billing periods (inferno-frontend/src/components/payment/validity.ts
// exists precisely to render that). Emitting one tiers[] row per PLAN would
// therefore emit duplicate tierIds the moment an admin adds a second billing
// period to an existing group -- not hypothetical, the schema and the panel
// both already support it.
//
// tierId MUST stay the group id in both `current` and `tiers[]` -- NOT the
// plan's own id. user_subscription stores group_id and has no plan_id column
// at all (ent/schema/user_subscription.go), so a plan id could never appear
// in `current` and current.tierId would never match any tiers[] entry. That
// match is load-bearing on the client: after an upgrade, the TUI polls
// `fresh?.current?.tier_id === pendingTierId` for up to 30s
// (ui-tui/src/components/subscriptionOverlay.tsx:786), where pendingTierId
// came from a tiers[] entry the caller picked -- a mismatched id space hangs
// that poll forever ("still applying").
//
// So each group is represented by ONE plan, chosen by billingRepresentativePlan:
// prefer a for-sale plan (a plan nobody can buy must never set the displayed
// price), then the lowest normalised dollars-per-month (the "from" price a
// picker conventionally shows), then the lowest plan id as a deterministic
// tie-break. isEnabled is independent of which plan was chosen: true if ANY
// plan in the group is for sale.
// representativePlanName returns the name of the SAME plan resolveTiers would
// choose to represent this group, so current.tierName and the matching tiers[]
// entry's name agree. Both carry the same tierId (the group id), so disagreeing
// names describe one tier under two labels.
func (s *BillingContractService) representativePlanName(ctx context.Context, groupID int64) (string, bool) {
	plans, err := s.planSvc.ListPlans(ctx)
	if err != nil {
		slog.Error("billing contract: plan catalog lookup failed resolving current tier name", "error", err)
		return "", false
	}
	inGroup := make([]*dbent.SubscriptionPlan, 0, 2)
	for _, pl := range plans {
		if pl.GroupID == groupID {
			inGroup = append(inGroup, pl)
		}
	}
	if len(inGroup) == 0 {
		return "", false
	}
	rep, _ := billingRepresentativePlan(inGroup)
	if rep == nil {
		return "", false
	}
	return rep.Name, true
}

func (s *BillingContractService) resolveTiers(ctx context.Context, activeGroupID int64, hasActive bool) []BillingTierView {
	plans, err := s.planSvc.ListPlans(ctx)
	if err != nil {
		slog.Error("billing contract: plan catalog lookup failed", "error", err)
		return []BillingTierView{}
	}

	groupInfo := s.planSvc.GetGroupInfoMap(ctx, plans)

	// Group while preserving first-seen order (ListPlans is already
	// BySortOrder-ordered, so this keeps the same display order the old
	// one-row-per-plan version had).
	byGroup := make(map[int64][]*dbent.SubscriptionPlan, len(plans))
	groupOrder := make([]int64, 0, len(plans))
	for _, p := range plans {
		if _, seen := byGroup[p.GroupID]; !seen {
			groupOrder = append(groupOrder, p.GroupID)
		}
		byGroup[p.GroupID] = append(byGroup[p.GroupID], p)
	}

	tiers := make([]BillingTierView, 0, len(groupOrder))
	perMonth := make([]float64, 0, len(groupOrder))
	for _, groupID := range groupOrder {
		rep, anyForSale := billingRepresentativePlan(byGroup[groupID])
		dollars := billingDollarsPerMonth(rep.Price, rep.ValidityDays, rep.ValidityUnit)
		view := BillingTierView{
			TierID:                 strconv.FormatInt(groupID, 10),
			Name:                   rep.Name,
			DollarsPerMonthDisplay: billingDisplayMoney(dollars),
			IsCurrent:              hasActive && groupID == activeGroupID,
			IsEnabled:              anyForSale,
		}
		if gi, ok := groupInfo[groupID]; ok && gi.MonthlyLimitUSD != nil {
			mc := billingMoney(*gi.MonthlyLimitUSD)
			view.MonthlyCredits = &mc
		}
		tiers = append(tiers, view)
		perMonth = append(perMonth, dollars)
	}
	billingAssignTierOrder(tiers, perMonth)
	return tiers
}

// billingAssignTierOrder stamps every tier row with a 1-based rank, in place
// (ruling C-2). perMonth[i] is tiers[i]'s normalised dollars-per-month.
//
// WHY A COMPUTED RANK AND NOT SubscriptionPlan.SortOrder: see
// BillingTierView.TierOrder's doc comment. The short version is that SortOrder
// defaults to 0, the panel's own plan editor posts 0, and every client surface
// filters `tier_order > 0` -- so carrying SortOrder emitted an invisible plan
// picker on default data.
//
// WHY NOT SortOrder+1: SortOrder is not unique and is not required to be
// meaningful. Two groups both left at the default would both become 1, which
// makes is_upgrade (subscription_view.py:409-411) answer False in BOTH
// directions between them -- neither plan is an upgrade over the other, so the
// picker offers a change it then refuses to characterise. The rank has to be
// distinct per group, and it has to be derived from something that actually
// orders plans.
//
// The ordering key is normalised dollars-per-month ascending -- the same figure
// dollarsPerMonthDisplay reports, so a user reading the picker sees rows whose
// prices ascend in the order the ranks claim. Ties break on the tiers[] index,
// which is ListPlans' own BySortOrder order (payment_config_plans.go:127-129):
// an admin who DID set sort orders still gets their intent among equal-priced
// groups, and the result is total, so the rank is deterministic across requests
// on unchanged data. Ranks are 1..len(tiers) with no gaps.
//
// Sorting a slice of indices rather than the views themselves is deliberate:
// tiers[] keeps its first-seen (BySortOrder) emission order, which is what the
// panel and the previous behaviour both show. Only the rank is re-derived; the
// client sorts by it itself on every surface that cares
// (selectable_tiers, subscriptionOverlay.tsx:381,525).
func billingAssignTierOrder(tiers []BillingTierView, perMonth []float64) {
	idx := make([]int, len(tiers))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return perMonth[idx[a]] < perMonth[idx[b]]
	})
	for rank, i := range idx {
		tiers[i].TierOrder = rank + 1
	}
}

// billingRepresentativePlan picks the one plan that stands in for its whole
// group's tiers[] entry (ruling R-3.1; see resolveTiers's doc comment for
// why one entry per group, and why the id space cannot be the plan's own
// id). Returns the chosen plan and whether ANY plan in the group is for
// sale (the group's own isEnabled, independent of which plan was picked).
//
// plans must be non-empty (byGroup is built from at least one plan per key).
func billingRepresentativePlan(plans []*dbent.SubscriptionPlan) (*dbent.SubscriptionPlan, bool) {
	anyForSale := false
	for _, p := range plans {
		if p.ForSale {
			anyForSale = true
			break
		}
	}

	candidates := plans
	if anyForSale {
		candidates = make([]*dbent.SubscriptionPlan, 0, len(plans))
		for _, p := range plans {
			if p.ForSale {
				candidates = append(candidates, p)
			}
		}
	}

	best := candidates[0]
	bestPerMonth := billingDollarsPerMonth(best.Price, best.ValidityDays, best.ValidityUnit)
	for _, p := range candidates[1:] {
		perMonth := billingDollarsPerMonth(p.Price, p.ValidityDays, p.ValidityUnit)
		if perMonth < bestPerMonth || (perMonth == bestPerMonth && p.ID < best.ID) {
			best, bestPerMonth = p, perMonth
		}
	}
	return best, anyForSale
}

// billingValidityTotalDays mirrors TWO things that must agree with each
// other: inferno-frontend/src/components/payment/validity.ts's unit
// normalization (trim, lowercase, strip a trailing "s" -- the admin form
// saves plural units while some historical rows are singular) and Inferno's
// own billing conversion, service/payment_service.go's psComputeValidityDays
// (unexported, so not directly reusable here): a month/months unit means
// ValidityDays actually COUNTS MONTHS (x30 real days), a week/weeks unit
// means it counts WEEKS (x7), and everything else -- including the DB
// default "day" and any unrecognized unit -- is billed literally in days.
func billingValidityTotalDays(days int, unit string) int {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(unit)), "s")
	switch base {
	case "month":
		return days * 30
	case "week":
		return days * 7
	default:
		return days
	}
}

// billingDollarsPerMonth normalises a plan's total Price over its full
// validity period into a "$ per 30 days" figure (ruling R-3.2), so
// dollarsPerMonthDisplay -- a field whose NAME asserts a unit -- actually
// means what it says: an annual plan priced 1200 reports "100", not "1200".
//
// A plan with a non-positive total validity (<=0 days after
// billingValidityTotalDays) cannot be normalised at all -- CreatePlan/
// UpdatePlan validate ValidityDays > 0, so this is only reachable via
// malformed or historical data outside that validation, never a plan created
// through this codebase's own API. Rather than divide by zero or fabricate a
// number, this returns the plan's raw Price unmodified for that one
// pathological case: a known mislabel, not an invented figure.
func billingDollarsPerMonth(price float64, validityDays int, validityUnit string) float64 {
	totalDays := billingValidityTotalDays(validityDays, validityUnit)
	if totalDays <= 0 {
		return price
	}
	return price * 30 / float64(totalDays)
}

// ---------------------------------------------------------------------------
// The `subscription` object inside GET /api/oauth/account (Task 4).
//
// THIS IS A DIFFERENT WIRE CONTRACT FROM BillingSubscriptionView ABOVE, even
// though it describes the same underlying data. The only consumer is
// hermes_cli/nous_account.py's _subscription_from_payload (:705-716), reached
// from _info_from_account_payload's payload.get("subscription") (:660) --
// snake_case, a completely separate parser from Task 3's camelCase
// agent/subscription_view.py. Every field below is the exact key that
// function reads:
//
//	plan                _coerce_str    (:709)
//	tier                _coerce_int    (:710)
//	monthly_charge      _coerce_float  (:711) -- SEE THE WARNING ON THE FIELD
//	monthly_credits     _coerce_float  (:712)
//	current_period_end  _coerce_str    (:713)
//	credits_remaining   _coerce_float  (:714)
//	rollover_credits    _coerce_float  (:715) -- Inferno has no rollover
//	                                    concept; no Go field exists for this
//	                                    key at all, so there is nothing to
//	                                    accidentally set.
//
// A non-dict `subscription` (missing key included) makes
// _subscription_from_payload return None (:706-707) -- so, exactly like
// Task 1's precedent, omitting the whole key is the safe and correct
// representation of "no subscription", never an object of nulls.
// ---------------------------------------------------------------------------

// BillingAccountSubscriptionView is the snake_case `subscription` object
// embedded in GET /api/oauth/account's payload.
type BillingAccountSubscriptionView struct {
	// Plan -- _subscription_from_payload:709. Reuses
	// BillingCurrentSubscriptionView.TierName (the group's Name) verbatim.
	// omitempty costs nothing: _coerce_str already turns "" into None the
	// same as a missing key.
	Plan string `json:"plan,omitempty"`

	// Tier -- :710. The active group's rank in Task 3's tiers[] --
	// resolveTiers / billingAssignTierOrder (rulings R-3.1/R-3.2, C-2),
	// found by locating the tiers[] entry with IsCurrent==true. Sourcing it
	// from the SAME array GET /api/billing/subscription emits is the point:
	// the two endpoints cannot report different ranks for one subscription.
	// (It was SubscriptionPlan.SortOrder until C-2 -- see
	// BillingTierView.TierOrder.) Omitted, never a fabricated 0,
	// when the active group is not present in the plan catalog (a
	// data-integrity edge case: an active subscription pointing at a group
	// that ListPlans no longer returns any plan for).
	Tier *int `json:"tier,omitempty"`

	// MonthlyCharge is DELIBERATELY NEVER SET -- this field exists, always
	// nil, so that decision is visible instead of a key nobody thought
	// about. hermes_cli/models.py:685-707's is_nous_free_tier reads
	// monthly_charge == 0 as "free tier" (an old, currently-dead fallback
	// path -- see task-4-report.md for the citation proving it has zero
	// production call sites today, and why the ruling holds regardless).
	// Inferno has no honest RECURRING charge figure to report here: a
	// SubscriptionPlan's Price is a per-billing-period amount, and
	// normalising it to a monthly figure the way Task 3's
	// dollarsPerMonthDisplay does is a judgment call this task's brief
	// explicitly declines to make for this field ("omit unless we have a
	// real recurring charge figure" -- a normalised derivative is not
	// that). Omitted reads as "not free" at models.py:704
	// (`if charge is None: return False`), the safe direction. NEVER emit
	// 0 as filler for "unknown" -- that silently downgrades a paying user.
	MonthlyCharge *float64 `json:"monthly_charge,omitempty"`

	// MonthlyCredits / CreditsRemaining -- :711 (sic, :712) / :714, both ->
	// _coerce_float. Reuses Task 3's resolveCurrentSubscription figures
	// exactly: this parses BillingCurrentSubscriptionView's own decimal
	// STRING output (MonthlyCredits/CreditsRemaining) back to float64
	// rather than recomputing limit-minus-usage a second time, so the two
	// endpoints can never silently disagree about the same subscription.
	// The round trip is lossless: billingMoney encodes with
	// strconv.FormatFloat(v, 'f', -1, 64), the shortest representation
	// that parses back to the identical float64.
	MonthlyCredits   *float64 `json:"monthly_credits,omitempty"`
	CreditsRemaining *float64 `json:"credits_remaining,omitempty"`

	// CurrentPeriodEnd -- :713. Reuses
	// BillingCurrentSubscriptionView.CycleEndsAt verbatim (RFC3339,
	// sourced from sub.ExpiresAt).
	CurrentPeriodEnd string `json:"current_period_end,omitempty"`
}

// AccountSubscription builds the `subscription` object for
// GET /api/oauth/account (Task 4). Returns nil -- meaning "omit the key
// entirely" -- when the caller has no active subscription or the lookup
// degrades, exactly resolveCurrentSubscription's own "no plan" signal
// (Task 3's same degrade-not-500 philosophy: this endpoint has no single
// fatal dependency).
//
// Reuses Task 3's resolveCurrentSubscription and resolveTiers rather than
// re-deriving the group/plan logic a second time -- a second mapper that
// drifts from the first is exactly the defect class this task exists to
// avoid (see task-4-brief.md).
func (s *BillingContractService) AccountSubscription(ctx context.Context, userID int64) *BillingAccountSubscriptionView {
	current, activeGroupID, hasActive := s.resolveCurrentSubscription(ctx, userID)
	if !hasActive || current == nil {
		return nil
	}

	view := &BillingAccountSubscriptionView{
		Plan:             current.TierName,
		CurrentPeriodEnd: current.CycleEndsAt,
		MonthlyCredits:   billingMoneyStringToFloat(current.MonthlyCredits),
		CreditsRemaining: billingMoneyStringToFloat(current.CreditsRemaining),
	}

	for _, t := range s.resolveTiers(ctx, activeGroupID, hasActive) {
		if t.IsCurrent {
			order := t.TierOrder
			view.Tier = &order
			break
		}
	}

	return view
}

// billingMoneyStringToFloat parses one of billingMoney's own decimal-string
// outputs back to float64. The round trip is lossless because billingMoney
// uses strconv.FormatFloat(v, 'f', -1, 64) -- precision -1 is defined to
// produce the minimum number of digits necessary such that ParseFloat
// returns v exactly. A parse failure can only mean the string did not
// actually come from billingMoney (defensive only, not a reachable path
// through this codebase's own callers) -- omit rather than report a
// fabricated number.
func billingMoneyStringToFloat(s *string) *float64 {
	if s == nil {
		return nil
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		slog.Error("billing contract: could not parse money string back to float64", "value", *s, "error", err)
		return nil
	}
	return &v
}

// ---------------------------------------------------------------------------
// Task 5 -- the 7 write endpoints, all gated on billing:manage.
//
// Ruling R-5.1: 2 are IMPLEMENTED over Inferno's existing order flow, 5 are
// HONESTLY REFUSED. Every method/key/status below is cited to
// hermes_cli/nous_billing.py (task-5-brief.md, task-5-report.md).
//
// billing:manage IS GRANTABLE and these 7 ARE reachable in production. An
// earlier version of this comment said the opposite; it was false. It is
// refused only by the authorization_code grant
// (OAuthAuthorizeService.IssueCode, oauth_authorize_service.go:221); the
// DEVICE grant accepts it (OAuthDeviceService.RequestCode,
// oauth_device_service.go:144, validates with ValidateScope only, and
// knownScopes contains ScopeBillingManage, oauth_scope_vocabulary.go:56-64),
// and ExchangeDeviceCode mints the token with that scope verbatim
// (oauth_token_service.go:513,518,535). The client runs exactly that flow --
// hermes_cli/auth.py:9093 step_up_nous_billing_scope, triggered from
// hermes_cli/cli_billing_mixin.py:1303-1305 on a 403 insufficient_scope.
//
// So the handlers below are LIVE code paths, not dead ones behind a gate
// nobody can pass. Each body has to be right on its own merits; see Charge in
// particular (ruling D-2), which refuses rather than create an order nobody
// can pay.
// ---------------------------------------------------------------------------

const (
	// billingChargeIdempotencyScope namespaces this endpoint's idempotency
	// keys from every OTHER idempotent write in the app (redeem, batch image,
	// admin ops, ...) that already shares the same IdempotencyRepository
	// table. Two different users' -- or two different ENDPOINTS' -- literal
	// key strings must never collide.
	billingChargeIdempotencyScope = "billing_contract_charge"
)

// BillingChargeAcceptedView is POST /api/billing/charge's 202 body
// (hermes_cli/nous_billing.py:513-514 docstring, key read by
// cli_billing_mixin.py:1155 -- `result.get("chargeId")`).
//
// Money is NOT confirmed here: chargeId is Inferno's PaymentOrder id, and the
// caller polls GET /api/billing/charge/{id} (ChargeStatus below) until it
// resolves. Deliberately no amountUsd/status here -- the client's own code
// never reads either field off THIS response, only off the poll response.
type BillingChargeAcceptedView struct {
	ChargeID string `json:"chargeId"`
}

// billingChargeMissingIdempotencyKey is exactly the 400 the brief mandates:
// "a missing header is a 400, not a default" (nous_billing.py:511-521).
//
// This check happens BEFORE the idempotency coordinator is ever touched,
// deliberately: IdempotencyCoordinator.Execute's own RequireKey flag is
// suppressed by IdempotencyConfig.ObserveOnly (idempotency.go:213-221,
// defaulting ObserveOnly=true -- "observe first, enforce later" for the
// OTHER callers of that shared coordinator). Relying on that flag here would
// make this endpoint's 400 contingent on a config value owned by an
// unrelated feature; checking the header ourselves makes the 400
// unconditional, matching the brief's "not a default".
var ErrBillingChargeIdempotencyKeyRequired = infraerrors.BadRequest(
	"IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")

// Charge handles POST /api/billing/charge -- ruling R-5.1's first
// IMPLEMENTED write. It creates a "balance" PaymentOrder through the SAME
// CreateOrder path the panel's own recharge flow uses
// (service/payment_order.go:25), over whichever of this deployment's six
// enabled payment providers (EasyPay/Alipay/Wxpay/Stripe/Airwallex/Razorpay,
// per BillingChargeOrderSource's doc comment) actually has an instance
// configured -- PaymentType is left empty so the load balancer auto-selects
// across all enabled instances
// (load_balancer.go:114-117 documents this as supported cross-provider
// selection), because the client sends no payment-method choice of its own
// (its only field is amountUsd, :525) and there is no UI here to ask with.
//
// idempotencyKey MUST be non-empty (checked above); ctx-cancellation and
// provider failures propagate as-is -- CreateOrder's own errors (INVALID_AMOUNT,
// PAYMENT_DISABLED, ...) already carry the right HTTP status via infraerrors,
// so this method does not re-wrap them.
func (s *BillingContractService) Charge(ctx context.Context, userID int64, amountUSD float64, idempotencyKey string) (*BillingChargeAcceptedView, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, ErrBillingChargeIdempotencyKeyRequired
	}
	if s.idempotency == nil {
		return nil, infraerrors.ServiceUnavailable("SERVER_ERROR", "idempotency coordinator is not wired")
	}
	if s.chargeSvc == nil {
		return nil, infraerrors.ServiceUnavailable("SERVER_ERROR", "charge order source is not wired")
	}

	result, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope:          billingChargeIdempotencyScope,
		ActorScope:     "user:" + strconv.FormatInt(userID, 10),
		Method:         http.MethodPost,
		Route:          "/api/billing/charge",
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"amountUsd": amountUSD},
		TTL:            DefaultWriteIdempotencyTTL(),
		RequireKey:     true,
	}, func(execCtx context.Context) (any, error) {
		resp, createErr := s.chargeSvc.CreateOrder(execCtx, CreateOrderRequest{
			UserID:        userID,
			Amount:        amountUSD,
			OrderType:     payment.OrderTypeBalance,
			PaymentSource: "hermes_cli",
		})
		if createErr != nil {
			return nil, createErr
		}
		if resp == nil {
			// Defensive: BillingChargeOrderSource is an interface, and a
			// misbehaving implementation returning (nil, nil) must not crash
			// this request -- it is a server fault, not a client error.
			return nil, infraerrors.InternalServer("SERVER_ERROR", "order creation returned no response")
		}
		return &BillingChargeAcceptedView{ChargeID: strconv.FormatInt(resp.OrderID, 10)}, nil
	})
	if err != nil {
		return nil, err
	}
	return billingDecodeChargeAccepted(result.Data)
}

// billingDecodeChargeAccepted normalises IdempotencyExecuteResult.Data into
// *BillingChargeAcceptedView. On a FRESH execution the value is the typed
// struct the closure above returned; on a REPLAY it is whatever
// decodeStoredResponse produced from the persisted JSON
// (idempotency.go:473-482 -- a generic map[string]any), because the
// coordinator stores every response as JSON text and does not know this
// caller's Go type. Both are normalised through one JSON round trip so the
// two paths can never drift into different wire shapes.
func billingDecodeChargeAccepted(data any) (*BillingChargeAcceptedView, error) {
	if view, ok := data.(*BillingChargeAcceptedView); ok {
		return view, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("billing contract: encode charge-accepted for decode: %w", err)
	}
	var view BillingChargeAcceptedView
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, fmt.Errorf("billing contract: decode charge-accepted: %w", err)
	}
	return &view, nil
}

// BillingChargeStatusView is GET /api/billing/charge/{id}'s body
// (nous_billing.py:532-538). AmountUSD/Reason are optional: read by
// cli_billing_mixin.py:1189-1190 (settled) and :1200-1210 (failed) but not
// required by the parser (both `.get()` calls tolerate a missing key).
type BillingChargeStatusView struct {
	Status string `json:"status"`
	// AmountUSD is the decimal-STRING money encoding (billingMoney), matching
	// every other money field this adapter emits (Tasks 1/3) -- unlike the
	// REQUEST body's amountUsd, which is a raw JSON number because that is
	// the client's OWN encoding for what IT sends (nous_billing.py:525), not
	// something this codebase chose.
	AmountUSD *string `json:"amountUsd,omitempty"`
	// Reason is free text on a "failed" status. Deliberately never one of
	// the client's three RESERVED upgrade-endpoint values
	// ("authentication_required"/"payment_method_expired"/"card_declined",
	// cli_billing_mixin.py:1203-1208) -- those name Stripe 3DS/card outcomes
	// Inferno cannot produce (no card on file, R-5.1). Any other string
	// (including these below) falls through to the client's own generic
	// fallback copy ("didn't go through ({reason})"), which is honest.
	Reason *string `json:"reason,omitempty"`
}

// ChargeStatus handles GET /api/billing/charge/{id}.
//
// SECURITY-CRITICAL (brief, citing nous_billing.py:536-538): an unknown OR
// FOREIGN charge id returns {status:"pending"}, NEVER 404, NEVER another
// user's data. A 404 here would be a cross-tenant existence oracle -- a
// caller could probe sequential ids and learn which ones exist for OTHER
// users. So every error GetOrder can return (NotFound for an unknown id,
// Forbidden for one owned by someone else, and any other -- transient --
// failure) collapses to the SAME "pending" response, indistinguishable on
// the wire. This is deliberately not "loud" the way State() is: unlike an
// unresolvable balance, there is no safe way to surface more detail here
// without either leaking existence or downgrading a real error into a false
// "failed" the client would render as a lie.
//
// A malformed id (not a PaymentOrder integer id) is treated the same way --
// also pending, also never a distinguishable error -- for the identical
// oracle reason: responding differently to "malformed" vs "well-formed but
// unknown" would let a prober learn the id SHAPE even without learning
// existence.
func (s *BillingContractService) ChargeStatus(ctx context.Context, userID int64, chargeID string) *BillingChargeStatusView {
	pending := &BillingChargeStatusView{Status: "pending"}
	if s.chargeSvc == nil {
		return pending
	}

	orderID, err := strconv.ParseInt(strings.TrimSpace(chargeID), 10, 64)
	if err != nil {
		return pending
	}

	order, err := s.chargeSvc.GetOrder(ctx, orderID, userID)
	if err != nil {
		return pending
	}

	switch order.Status {
	case OrderStatusCompleted:
		// The only status the money has actually LANDED in the caller's
		// balance (payment_fulfillment.go:371-373's terminal happy path).
		amt := billingMoney(order.Amount)
		return &BillingChargeStatusView{Status: "settled", AmountUSD: &amt}
	case OrderStatusExpired:
		reason := "charge_expired"
		return &BillingChargeStatusView{Status: "failed", Reason: &reason}
	case OrderStatusCancelled:
		reason := "charge_cancelled"
		return &BillingChargeStatusView{Status: "failed", Reason: &reason}
	case OrderStatusFailed:
		// Crediting failed AFTER payment was received
		// (payment_fulfillment.go:819-822) -- a real terminal failure, not a
		// declined card (Inferno has none to decline).
		reason := "provider_declined"
		return &BillingChargeStatusView{Status: "failed", Reason: &reason}
	case OrderStatusPending, OrderStatusPaid, OrderStatusRecharging:
		// Pending: awaiting payment. Paid: payment received, not yet
		// credited. Recharging: crediting in progress. None of the three is
		// "done" -- money is not confirmed at THIS status the same way it is
		// at Completed, so all three stay "pending" and the client keeps
		// polling (nous_billing.py's 5-minute poll cap treats a stuck
		// "pending" as a timeout, never an error -- cli_billing_mixin.py's
		// final else-branch, "not a failure").
		return pending
	default:
		// Any refund-lane status (RefundRequested/Refunding/.../RefundFailed)
		// or a future status this contract has not been taught about.
		// "pending" is the SAFE default here: reporting "failed" for money
		// that actually settled (then got refunded) would be a worse lie
		// than an eventually-timing-out "pending", which the client already
		// treats as harmless.
		return pending
	}
}

// ---------------------------------------------------------------------------
// The 5 honest refusals (ruling R-5.1). Inferno has no card vault, no
// proration engine, and no end-of-period-intent model
// (grep -rniE 'cancel_at_period|pending_change|auto_renew' over ent/schema +
// subscription_service.go returns nothing) -- these bodies say so instead of
// inventing a Stripe-shaped success or a 404 that would read as "portal
// unreachable" (a worse lie).
// ---------------------------------------------------------------------------

// BillingSubscriptionPreviewView is POST /api/billing/subscription/preview's
// 200 body. "blocked" is the CLIENT's own vocabulary for "the commit would be
// refused" (nous_billing.py's post_subscription_preview docstring, the
// `effect` field), so this renders a clean message on the client rather than
// an error path.
type BillingSubscriptionPreviewView struct {
	Effect string `json:"effect"`
	Reason string `json:"reason"`
}

// SubscriptionPreview handles POST /api/billing/subscription/preview
// (nous_billing.py:573-591; body {"subscriptionTypeId": <str>} at :589,
// accepted but not required to be valid -- the answer is the same "blocked"
// regardless, since Inferno cannot preview ANY plan change honestly).
func (s *BillingContractService) SubscriptionPreview(context.Context, int64, string) *BillingSubscriptionPreviewView {
	return &BillingSubscriptionPreviewView{
		Effect: "blocked",
		Reason: "Inferno has no proration engine and no stored payment method to quote a change against -- manage your subscription on the billing portal.",
	}
}

// BillingRefusalView is the shared 501 body for the four writes this
// deployment cannot honestly perform: POST /subscription/upgrade, PUT and
// DELETE /subscription/pending-change, PATCH /auto-top-up.
//
// error/message are read generically by the client's own error mapper
// (nous_billing.py's _raise_for_error, :310-380): 501 matches none of that
// function's dedicated branches (400/401/403/429/503 only), so every one of
// these falls through to its final generic
// `raise BillingError(message or error or ...)` -- there is no reserved key
// vocabulary to avoid here, unlike the upgrade endpoint's own `status` field
// (see SubscriptionUpgrade's doc comment for the vocabulary that DOES matter).
type BillingRefusalView struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	PortalURL string `json:"portalUrl,omitempty"`
}

func (s *BillingContractService) billingRefusal(errCode, message string) *BillingRefusalView {
	return &BillingRefusalView{
		Error:     errCode,
		Message:   message,
		PortalURL: billingPortalURL(s.portalBaseURL),
	}
}

// SubscriptionUpgrade handles POST /api/billing/subscription/upgrade
// (nous_billing.py:648-675; body {"subscriptionTypeId"} at :672,
// Idempotency-Key mandatory per the client's own docstring, :657-670).
//
// 501, never a fake success and never the client's own
// "requires_action"/"payment_failed" statuses (:659-661): those name a 3DS
// pending state and a declined card. Inferno has no card on file at all --
// claiming one was declined would be a lie the user cannot act on (there is
// nothing to update). "no_payment_method" says the true thing instead.
//
// idempotencyKey is accepted but not enforced here: unlike Charge, a
// refused request performs no side effect to deduplicate, so there is
// nothing an idempotency key could protect against double-executing.
func (s *BillingContractService) SubscriptionUpgrade(context.Context, int64, string, string) *BillingRefusalView {
	return s.billingRefusal("no_payment_method",
		"Inferno has no stored payment method and no proration engine -- upgrade your plan on the billing portal.")
}

// PendingChangeSet handles PUT /api/billing/subscription/pending-change
// (nous_billing.py:594-627; body {"type":"cancellation"} or
// {"type":"tier_change","subscriptionTypeId":<str>}, :609-621).
func (s *BillingContractService) PendingChangeSet(context.Context, int64) *BillingRefusalView {
	return s.billingRefusal("unsupported_operation",
		"Inferno does not model an end-of-period pending change -- manage your subscription on the billing portal.")
}

// PendingChangeClear handles DELETE /api/billing/subscription/pending-change
// (nous_billing.py:631-644; no body).
func (s *BillingContractService) PendingChangeClear(context.Context, int64) *BillingRefusalView {
	return s.billingRefusal("unsupported_operation",
		"Inferno does not model an end-of-period pending change -- manage your subscription on the billing portal.")
}

// AutoTopUp handles PATCH /api/billing/auto-top-up (nous_billing.py:480-499;
// body {"enabled":bool,"threshold":num,"topUpAmount":num}, :495-499).
func (s *BillingContractService) AutoTopUp(context.Context, int64) *BillingRefusalView {
	return s.billingRefusal("unsupported_operation",
		"Auto top-up requires a stored payment method, which Inferno does not support -- recharge manually on the billing portal.")
}
