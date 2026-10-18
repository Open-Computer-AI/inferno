package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConcurrencyPolicyPatchJSONAndValidation(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantPresent bool
		wantMode    string
		wantErr     string
	}{
		{name: "omitted preserves", payload: `{}`, wantPresent: false},
		{name: "shared clears", payload: `{"concurrency_policy":{"mode":"shared"}}`, wantPresent: true, wantMode: "shared"},
		{name: "split accepts both positive limits", payload: `{"concurrency_policy":{"mode":"split","live":4,"requests":2}}`, wantPresent: true, wantMode: "split"},
		{name: "split rejects missing request limit", payload: `{"concurrency_policy":{"mode":"split","live":4}}`, wantPresent: true, wantMode: "split", wantErr: "requests"},
		{name: "unknown mode rejects", payload: `{"concurrency_policy":{"mode":"borrow"}}`, wantPresent: true, wantMode: "borrow", wantErr: "mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request struct {
				ConcurrencyPolicy *ConcurrencyPolicyPatch `json:"concurrency_policy"`
			}
			if err := json.Unmarshal([]byte(tt.payload), &request); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if (request.ConcurrencyPolicy != nil) != tt.wantPresent {
				t.Fatalf("presence = %t, want %t", request.ConcurrencyPolicy != nil, tt.wantPresent)
			}
			if request.ConcurrencyPolicy == nil {
				return
			}
			if request.ConcurrencyPolicy.Mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", request.ConcurrencyPolicy.Mode, tt.wantMode)
			}
			if err := request.ConcurrencyPolicy.Validate(); tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			} else if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestConcurrencyPolicyPatchResolvesWithoutBorrowing(t *testing.T) {
	live, requests := 4, 2
	currentLive, currentRequests := 9, 7

	shared := ConcurrencyPolicyPatch{Mode: "shared"}
	gotLive, gotRequests, err := shared.Resolve()
	if err != nil {
		t.Fatalf("shared Resolve() error = %v", err)
	}
	if gotLive != nil || gotRequests != nil {
		t.Fatalf("shared Resolve() = (%v, %v), want both nil", gotLive, gotRequests)
	}

	split := ConcurrencyPolicyPatch{Mode: "split", Live: &live, Requests: &requests}
	gotLive, gotRequests, err = split.Resolve()
	if err != nil {
		t.Fatalf("split Resolve() error = %v", err)
	}
	if gotLive == nil || gotRequests == nil || *gotLive != live || *gotRequests != requests {
		t.Fatalf("split Resolve() = (%v, %v), want (%d, %d)", gotLive, gotRequests, live, requests)
	}

	if currentLive == *gotLive || currentRequests == *gotRequests {
		t.Fatal("test setup must exercise independent limits")
	}
}

func TestConcurrencyPolicyGenerationOnlyChangesForExplicitPolicyPatch(t *testing.T) {
	if got := policyGenerationAfterPatch(17, nil); got != 17 {
		t.Fatalf("omitted patch generation = %d, want 17", got)
	}
	shared := &ConcurrencyPolicyPatch{Mode: "shared"}
	if got := policyGenerationAfterPatch(17, shared); got != 18 {
		t.Fatalf("explicit shared patch generation = %d, want 18", got)
	}
}

func TestAccountPolicyPatchEligibilityAndPreservation(t *testing.T) {
	live, requests := 4, 2
	split := &ConcurrencyPolicyPatch{Mode: ConcurrencyModeSplit, Live: &live, Requests: &requests}

	openAI := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	if err := applyAccountConcurrencyPolicy(openAI, split); err != nil {
		t.Fatalf("Live-capable OpenAI split policy: %v", err)
	}
	if openAI.LiveConcurrency == nil || openAI.RequestConcurrency == nil {
		t.Fatal("split policy did not persist both nullable limits")
	}

	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	if err := applyAccountConcurrencyPolicy(apiKey, split); err == nil {
		t.Fatal("API-key OpenAI account unexpectedly accepted split policy")
	}

	shared := &ConcurrencyPolicyPatch{Mode: ConcurrencyModeShared}
	if err := applyAccountConcurrencyPolicy(openAI, shared); err != nil {
		t.Fatalf("shared reset: %v", err)
	}
	if openAI.LiveConcurrency != nil || openAI.RequestConcurrency != nil {
		t.Fatal("shared reset did not clear both nullable limits")
	}
}
