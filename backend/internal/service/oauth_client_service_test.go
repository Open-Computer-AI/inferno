package service

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterSelfHostedAppliesAgentPrefix(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com")
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
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://a.example.com"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://b.example.com"); err != nil {
		t.Fatalf("second register: %v", err)
	}
}

func TestByClientIDRoundTrips(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	created, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com")
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
