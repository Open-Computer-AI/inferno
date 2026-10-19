package service

import (
	"context"
	"errors"
	"fmt"
)

const (
	ConcurrencyModeShared = "shared"
	ConcurrencyModeSplit  = "split"
)

var ErrConcurrencyPolicyRuntimeNotApplied = errors.New("concurrency policy persisted but runtime publication failed")
var ErrConcurrencyPolicyGenerationStale = errors.New("concurrency policy generation is stale")
var ErrConcurrencyPolicyGenerationConflict = errors.New("concurrency policy generation conflicts with published policy")
var ErrConcurrencyPolicyAdmissionUnavailable = errors.New("concurrency policy authority is unavailable")
var ErrConcurrencyPolicyAdmissionStale = errors.New("concurrency policy admission snapshot is stale")

// ConcurrencyCapacityError carries the server-derived workload class and the
// Redis admission scope that rejected it. It is intentionally free of request
// IDs, credentials, and payload data so it is safe to return as diagnostics.
type ConcurrencyCapacityError struct {
	Class  ConcurrencyClass
	Scope  string
	Reason string
}

func (e *ConcurrencyCapacityError) Error() string {
	if e == nil {
		return "concurrency capacity exhausted"
	}
	if e.Scope == "" {
		return fmt.Sprintf("%s concurrency capacity exhausted", e.Class)
	}
	return fmt.Sprintf("%s concurrency capacity exhausted for %s", e.Class, e.Scope)
}

// ConcurrencyPolicyPublisher is the narrow DB-to-Redis acknowledgement fence
// used by administrative writes. Implementations must update the authoritative
// generation monotonically and reject stale/conflicting publications.
type ConcurrencyPolicyPublisher interface {
	PublishAccountConcurrencyPolicy(ctx context.Context, accountID int64, policy ConcurrencyPolicy, generation int64) error
	PublishUserConcurrencyPolicy(ctx context.Context, userID int64, policy ConcurrencyPolicy, generation int64) error
}

// ConcurrencyPolicyPatch is the explicit admin/API patch shape. A nil patch
// means the caller omitted concurrency_policy and the stored policy must be
// preserved. The mode is required when the object is present so JSON null and
// an omitted object cannot be confused with an explicit reset.
type ConcurrencyPolicyPatch struct {
	Mode     string `json:"mode"`
	Live     *int   `json:"live,omitempty"`
	Requests *int   `json:"requests,omitempty"`
}

func (p *ConcurrencyPolicyPatch) Validate() error {
	if p == nil {
		return fmt.Errorf("concurrency_policy patch is nil")
	}
	switch p.Mode {
	case ConcurrencyModeShared:
		if p.Live != nil || p.Requests != nil {
			return fmt.Errorf("shared concurrency policy must not include live or requests")
		}
		return nil
	case ConcurrencyModeSplit:
		if p.Live == nil {
			return fmt.Errorf("split concurrency policy requires live")
		}
		if p.Requests == nil {
			return fmt.Errorf("split concurrency policy requires requests")
		}
		if *p.Live <= 0 {
			return fmt.Errorf("split concurrency policy live must be > 0")
		}
		if *p.Requests <= 0 {
			return fmt.Errorf("split concurrency policy requests must be > 0")
		}
		return nil
	default:
		return fmt.Errorf("unsupported concurrency policy mode %q", p.Mode)
	}
}

// Resolve returns owned copies of the nullable fields to be persisted. Shared
// mode deliberately resolves to a NULL pair; its legacy Concurrency field is
// the shared limit and is not modified by this patch.
func (p *ConcurrencyPolicyPatch) Resolve() (live, requests *int, err error) {
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	if p.Mode == ConcurrencyModeShared {
		return nil, nil, nil
	}
	liveValue, requestValue := *p.Live, *p.Requests
	return &liveValue, &requestValue, nil
}

func applyConcurrencyPolicyPatch(patch *ConcurrencyPolicyPatch) (live, requests *int, err error) {
	return patch.Resolve()
}

// policyGenerationAfterPatch is kept small and deterministic so callers can
// test that omitted policy patches do not invalidate unrelated active leases.
func policyGenerationAfterPatch(current int64, patch *ConcurrencyPolicyPatch) int64 {
	if patch == nil {
		return current
	}
	return current + 1
}

// ConcurrencyClass identifies the workload class selected by the server.
// Callers must derive this from the endpoint/capability rather than accepting
// a client-provided class label.
type ConcurrencyClass string

const (
	ConcurrencyClassRequest ConcurrencyClass = "request"
	ConcurrencyClassLive    ConcurrencyClass = "live"
)

// ConcurrencyPolicy is either a legacy shared limit or an opted-in pair of
// independent positive limits. Live and Request are intentionally pointers so
// the absent pair can be distinguished from invalid zero-valued split data.
type ConcurrencyPolicy struct {
	Shared  int
	Live    *int
	Request *int
}

// ConcurrencySnapshot is the authoritative Redis occupancy for one account or
// user. Total is deliberately derived from the two class counts so callers do
// not have to infer whether a legacy shared cap is currently in use.
type ConcurrencySnapshot struct {
	Request int
	Live    int
	Total   int
}

// ConcurrencyAdmissionRequest is the server-derived admission input. The
// caller supplies the policy generation it read with the account/user record;
// Redis validates those generations against the published authority in the
// same Lua operation before it counts or writes a slot.
type ConcurrencyAdmissionRequest struct {
	AccountID         int64
	AccountPolicy     ConcurrencyPolicy
	AccountGeneration int64
	UserID            int64
	UserPolicy        ConcurrencyPolicy
	UserGeneration    int64
	APIKeyID          int64
	Class             ConcurrencyClass
	ReservationID     string
}

type ConcurrencyAdmissionResult struct {
	Acquired    bool
	DeniedScope string
	Account     ConcurrencySnapshot
	User        ConcurrencySnapshot
}

// UserConcurrencyAdmissionRequest is used before account selection, where the
// authenticated user is known but the provider account is not. It enforces
// the user policy on the existing regular-user slot namespace and deliberately
// does not impose an account limit.
type UserConcurrencyAdmissionRequest struct {
	UserID         int64
	UserPolicy     ConcurrencyPolicy
	UserGeneration int64
	APIKeyID       int64
	Class          ConcurrencyClass
	ReservationID  string
}

func (r UserConcurrencyAdmissionRequest) Validate() error {
	if r.UserID <= 0 {
		return fmt.Errorf("user concurrency admission identifier must be positive")
	}
	if r.ReservationID == "" {
		return fmt.Errorf("user concurrency admission reservation ID is required")
	}
	if r.Class != ConcurrencyClassRequest {
		return fmt.Errorf("user-only concurrency admission requires the request class")
	}
	if r.UserGeneration < 0 {
		return fmt.Errorf("user concurrency admission generation must be non-negative")
	}
	if err := ValidateConcurrencyPolicy(r.UserPolicy); err != nil {
		return fmt.Errorf("user policy: %w", err)
	}
	return nil
}

// ConcurrencyAdmissionCache is optional so legacy cache doubles and wrappers
// remain valid for callers that have not migrated to policy-aware admission.
// Implementations must make acquire/refresh/release atomic with Redis TIME.
type ConcurrencyAdmissionCache interface {
	AcquireConcurrency(context.Context, ConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error)
	RefreshConcurrency(context.Context, ConcurrencyAdmissionRequest) (bool, error)
	ReleaseConcurrency(context.Context, ConcurrencyAdmissionRequest) error
}

// UserConcurrencyAdmissionCache is the policy-aware pre-account gate used by
// ordinary requests on every provider/auth path.
type UserConcurrencyAdmissionCache interface {
	AcquireUserConcurrency(context.Context, UserConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error)
	RefreshUserConcurrency(context.Context, UserConcurrencyAdmissionRequest) (bool, error)
	ReleaseUserConcurrency(context.Context, UserConcurrencyAdmissionRequest) error
}

func (r ConcurrencyAdmissionRequest) Validate() error {
	if r.AccountID <= 0 || r.UserID <= 0 || r.APIKeyID <= 0 {
		return fmt.Errorf("concurrency admission identifiers must be positive")
	}
	if r.ReservationID == "" {
		return fmt.Errorf("concurrency admission reservation ID is required")
	}
	if r.Class != ConcurrencyClassRequest && r.Class != ConcurrencyClassLive {
		return fmt.Errorf("unsupported concurrency admission class %q", r.Class)
	}
	if r.AccountGeneration < 0 || r.UserGeneration < 0 {
		return fmt.Errorf("concurrency admission generations must be non-negative")
	}
	if err := ValidateConcurrencyPolicy(r.AccountPolicy); err != nil {
		return fmt.Errorf("account policy: %w", err)
	}
	if err := ValidateConcurrencyPolicy(r.UserPolicy); err != nil {
		return fmt.Errorf("user policy: %w", err)
	}
	return nil
}

func (p ConcurrencyPolicy) Mode() string {
	if p.Live != nil && p.Request != nil {
		return ConcurrencyModeSplit
	}
	return ConcurrencyModeShared
}

func (a *Account) ConcurrencyMode() string {
	if a != nil && a.LiveConcurrency != nil && a.RequestConcurrency != nil {
		return ConcurrencyModeSplit
	}
	return ConcurrencyModeShared
}

func (u *User) ConcurrencyMode() string {
	if u != nil && u.LiveConcurrency != nil && u.RequestConcurrency != nil {
		return ConcurrencyModeSplit
	}
	return ConcurrencyModeShared
}

// IsSplit reports whether the policy contains the complete split pair. It is
// only meaningful after ValidateConcurrencyPolicy succeeds.
func (p ConcurrencyPolicy) IsSplit() bool {
	return p.Live != nil && p.Request != nil
}

// ValidateConcurrencyPolicy rejects partial or non-positive split policies.
// Legacy shared policies retain the existing <= 0 unlimited convention.
func ValidateConcurrencyPolicy(policy ConcurrencyPolicy) error {
	if policy.Live == nil && policy.Request == nil {
		return nil
	}
	if policy.Live == nil || policy.Request == nil {
		return fmt.Errorf("split concurrency policy requires both live and request limits")
	}
	if *policy.Live <= 0 || *policy.Request <= 0 {
		return fmt.Errorf("split concurrency policy limits must be positive")
	}
	return nil
}

// EffectiveConcurrencyLimit returns the cap for a server-derived workload
// class and whether the cap is the legacy combined limit.
func EffectiveConcurrencyLimit(policy ConcurrencyPolicy, class ConcurrencyClass) (limit int, shared bool, err error) {
	if err := ValidateConcurrencyPolicy(policy); err != nil {
		return 0, false, err
	}
	if !policy.IsSplit() {
		switch class {
		case ConcurrencyClassRequest, ConcurrencyClassLive:
			return policy.Shared, true, nil
		default:
			return 0, false, fmt.Errorf("unsupported concurrency class %q", class)
		}
	}

	switch class {
	case ConcurrencyClassRequest:
		return *policy.Request, false, nil
	case ConcurrencyClassLive:
		return *policy.Live, false, nil
	default:
		return 0, false, fmt.Errorf("unsupported concurrency class %q", class)
	}
}

// accountAdmissionLimit returns the actual Redis cap for a server-derived
// scheduler class. Split policies use their independent class limit; legacy
// policies retain the combined Concurrency cap for both classes.
func accountAdmissionLimit(account *Account, class ConcurrencyClass) (int, bool) {
	if account == nil {
		return 0, false
	}
	if class == "" {
		class = ConcurrencyClassRequest
	}
	limit, _, err := EffectiveConcurrencyLimit(concurrencyPolicyForAccount(account), class)
	if err != nil {
		return 0, false
	}
	return limit, true
}

// accountLoadLimit is the denominator used by scheduler load snapshots. A
// legacy account keeps its existing load-factor behavior; a split account's
// class cap is authoritative so request and Live occupancy do not share a
// load bucket.
func accountLoadLimit(account *Account, class ConcurrencyClass) (int, bool, bool) {
	limit, valid := accountAdmissionLimit(account, class)
	if !valid {
		return 0, false, false
	}
	if account != nil && account.ConcurrencyMode() == ConcurrencyModeSplit {
		return limit, true, true
	}
	if account == nil {
		return 0, false, false
	}
	return account.EffectiveLoadFactor(), false, true
}
