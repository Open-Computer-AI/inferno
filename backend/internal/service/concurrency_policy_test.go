package service

import "testing"

func policyIntPtr(value int) *int {
	return &value
}

func TestConcurrencyPolicyLegacyAndSplit(t *testing.T) {
	tests := []struct {
		name       string
		policy     ConcurrencyPolicy
		class      ConcurrencyClass
		wantLimit  int
		wantShared bool
		wantErr    bool
	}{
		{
			name:       "legacy shared account policy",
			policy:     ConcurrencyPolicy{Shared: 4},
			class:      ConcurrencyClassRequest,
			wantLimit:  4,
			wantShared: true,
		},
		{
			name:       "legacy shared user policy",
			policy:     ConcurrencyPolicy{Shared: 4},
			class:      ConcurrencyClassLive,
			wantLimit:  4,
			wantShared: true,
		},
		{
			name:       "nil split fields retain legacy shared mode",
			policy:     ConcurrencyPolicy{Shared: 0},
			class:      ConcurrencyClassRequest,
			wantLimit:  0,
			wantShared: true,
		},
		{
			name:       "split live limit",
			policy:     ConcurrencyPolicy{Shared: 99, Live: policyIntPtr(4), Request: policyIntPtr(2)},
			class:      ConcurrencyClassLive,
			wantLimit:  4,
			wantShared: false,
		},
		{
			name:       "split request limit",
			policy:     ConcurrencyPolicy{Shared: 99, Live: policyIntPtr(4), Request: policyIntPtr(2)},
			class:      ConcurrencyClassRequest,
			wantLimit:  2,
			wantShared: false,
		},
		{
			name:    "live field without request field is invalid",
			policy:  ConcurrencyPolicy{Live: policyIntPtr(4)},
			class:   ConcurrencyClassLive,
			wantErr: true,
		},
		{
			name:    "request field without live field is invalid",
			policy:  ConcurrencyPolicy{Request: policyIntPtr(2)},
			class:   ConcurrencyClassRequest,
			wantErr: true,
		},
		{
			name:    "zero split limit is invalid",
			policy:  ConcurrencyPolicy{Live: policyIntPtr(0), Request: policyIntPtr(2)},
			class:   ConcurrencyClassLive,
			wantErr: true,
		},
		{
			name:    "negative split limit is invalid",
			policy:  ConcurrencyPolicy{Live: policyIntPtr(4), Request: policyIntPtr(-1)},
			class:   ConcurrencyClassRequest,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, shared, err := EffectiveConcurrencyLimit(tt.policy, tt.class)
			if tt.wantErr {
				if err == nil {
					t.Fatal("EffectiveConcurrencyLimit() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("EffectiveConcurrencyLimit() error = %v", err)
			}
			if limit != tt.wantLimit {
				t.Fatalf("EffectiveConcurrencyLimit() limit = %d, want %d", limit, tt.wantLimit)
			}
			if shared != tt.wantShared {
				t.Fatalf("EffectiveConcurrencyLimit() shared = %v, want %v", shared, tt.wantShared)
			}
		})
	}
}

func TestAccountAndUserExposePairedConcurrencyPolicy(t *testing.T) {
	account := Account{Concurrency: 8, LiveConcurrency: policyIntPtr(4), RequestConcurrency: policyIntPtr(2)}
	user := User{Concurrency: 6, LiveConcurrency: policyIntPtr(3), RequestConcurrency: policyIntPtr(1)}

	if account.LiveConcurrency == nil || account.RequestConcurrency == nil {
		t.Fatal("account split policy fields must be present")
	}
	if user.LiveConcurrency == nil || user.RequestConcurrency == nil {
		t.Fatal("user split policy fields must be present")
	}
}
