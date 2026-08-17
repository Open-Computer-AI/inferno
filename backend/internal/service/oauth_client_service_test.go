package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRegisterSelfHostedAppliesAgentPrefix(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if !strings.HasPrefix(got.ClientID, "agent:") {
		t.Fatalf("expected an agent: prefix, got %q", got.ClientID)
	}
	if got.Status != ClientPending {
		t.Fatalf("expected status %q, got %q", ClientPending, got.Status)
	}
}

func TestRegisterSelfHostedNamesAreNotRequiredToBeUnique(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	// Two registrations must both succeed even if names collide. Upstream
	// treats the row id as the key; a name collision is harmless.
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://a.example.com", "same-name"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://b.example.com", "same-name"); err != nil {
		t.Fatalf("second register: %v", err)
	}
}

func TestByClientIDRoundTrips(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	created, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	found, err := svc.ByClientID(ctx, created.ClientID)
	if err != nil {
		t.Fatalf("ByClientID: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("round trip mismatch: %d vs %d", found.ID, created.ID)
	}
}

// TestRegisterSelfHostedUsesSuppliedName covers the fix for a review finding:
// the handler used to accept dto.SelfHostedClientRequest.Name and never read
// it, so `oc dashboard register --name my-laptop` silently discarded the
// user's explicit flag. A supplied name must now be persisted and returned
// verbatim.
func TestRegisterSelfHostedUsesSuppliedName(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com", "my-laptop")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if got.Name != "my-laptop" {
		t.Fatalf("expected supplied name %q, got %q", "my-laptop", got.Name)
	}
}

// TestRegisterSelfHostedFallsBackOnBlankName covers both the empty-string and
// whitespace-only cases: neither should ever be persisted as the name.
func TestRegisterSelfHostedFallsBackOnBlankName(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	for _, name := range []string{"", "   ", "\t\n"} {
		got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com", name)
		if err != nil {
			t.Fatalf("RegisterSelfHosted(name=%q): %v", name, err)
		}
		if strings.TrimSpace(got.Name) == "" {
			t.Fatalf("RegisterSelfHosted(name=%q): expected a generated fallback name, got %q", name, got.Name)
		}
	}
}

// TestRegisterSelfHostedRejectsOverLongName covers the schema's
// field.String("name").MaxLen(64): an over-length name must be rejected with
// a clean error, not truncated (truncation would silently give the client a
// name it never chose — the same class of bug as discarding it outright) and
// not surfaced as an opaque database error.
func TestRegisterSelfHostedRejectsOverLongName(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	tooLong := strings.Repeat("a", 65)
	_, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com", tooLong)
	if !errors.Is(err, ErrClientNameTooLong) {
		t.Fatalf("expected ErrClientNameTooLong, got %v", err)
	}
}
