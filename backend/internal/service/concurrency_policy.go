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
