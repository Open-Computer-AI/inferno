package service

import "fmt"

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
