package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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

func NewBillingContractService(
	balanceSvc BillingBalanceSource,
	orgSvc BillingOrgSource,
	usageSvc BillingUsageSource,
	paymentSvc BillingPaymentSource,
	planSvc BillingPlanSource,
	subSvc BillingSubscriptionSource,
	portalBaseURL string,
) *BillingContractService {
	return &BillingContractService{
		balanceSvc:    balanceSvc,
		orgSvc:        orgSvc,
		usageSvc:      usageSvc,
		paymentSvc:    paymentSvc,
		planSvc:       planSvc,
		subSvc:        subSvc,
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
	// TierOrder is the SubscriptionPlan's SortOrder, the same field the panel's
	// own /api/v1/payment/plans already orders by.
	TierOrder int `json:"tierOrder"`
	// DollarsPerMonthDisplay is the plan's Price as-is. Inferno's SubscriptionPlan
	// has no notion of "normalize to a monthly rate" -- the panel's own
	// /api/v1/payment/plans response (payment_handler.go's GetPlans) shows the
	// same raw Price with no such normalization -- so a non-monthly-validity
	// plan's price is shown unmodified, consistent with how the rest of this
	// product already displays it.
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

	view := &BillingCurrentSubscriptionView{
		TierID:            strconv.FormatInt(sub.GroupID, 10),
		TierName:          sub.Group.Name,
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
// -- see BillingPlanSource.ListPlans), marking the caller's own group current
// and every non-ForSale plan disabled.
//
// DollarsPerMonthDisplay is normalised via billingDollarsPerMonth (ruling
// R-3.2): a field whose NAME asserts "per month" must not emit a plan's raw
// total price when that plan bills annually or weekly.
func (s *BillingContractService) resolveTiers(ctx context.Context, activeGroupID int64, hasActive bool) []BillingTierView {
	plans, err := s.planSvc.ListPlans(ctx)
	if err != nil {
		slog.Error("billing contract: plan catalog lookup failed", "error", err)
		return []BillingTierView{}
	}

	groupInfo := s.planSvc.GetGroupInfoMap(ctx, plans)

	tiers := make([]BillingTierView, 0, len(plans))
	for _, p := range plans {
		view := BillingTierView{
			TierID:                 strconv.FormatInt(p.GroupID, 10),
			Name:                   p.Name,
			TierOrder:              p.SortOrder,
			DollarsPerMonthDisplay: billingMoney(billingDollarsPerMonth(p.Price, p.ValidityDays, p.ValidityUnit)),
			IsCurrent:              hasActive && p.GroupID == activeGroupID,
			IsEnabled:              p.ForSale,
		}
		if gi, ok := groupInfo[p.GroupID]; ok && gi.MonthlyLimitUSD != nil {
			mc := billingMoney(*gi.MonthlyLimitUSD)
			view.MonthlyCredits = &mc
		}
		tiers = append(tiers, view)
	}
	return tiers
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
