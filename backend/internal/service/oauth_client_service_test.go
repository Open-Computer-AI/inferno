package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
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

// TestRegisterSelfHostedAcceptsMultiByteNameUpToColumnWidth is the regression
// test for the byte-vs-character length unit. The column is VARCHAR(64), which
// counts CHARACTERS; the check used to count BYTES via Go's len(), so a
// perfectly legal 24-character Chinese name (72 bytes) was refused with a bare
// 400 and `oc dashboard register --name` simply did not work for the CJK
// markets this product explicitly serves.
//
// The name is round-tripped through the database, not just past the service
// check: ent's own MaxLen validator is byte-based too, so a service-only fix
// would have relocated the same rejection into an opaque 500.
func TestRegisterSelfHostedAcceptsMultiByteNameUpToColumnWidth(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	for _, name := range []string{
		strings.Repeat("我的笔记本", 4),            // 20 chars, 60 bytes
		strings.Repeat("字", maxClientNameLen), // exactly 64 chars, 192 bytes
		strings.Repeat("😀", 25),               // 25 chars, 100 bytes
	} {
		got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", name)
		if err != nil {
			t.Fatalf("RegisterSelfHosted(%q, %d runes/%d bytes): %v",
				name, utf8.RuneCountInString(name), len(name), err)
		}
		if got.Name != name {
			t.Fatalf("name must persist verbatim: got %q, want %q", got.Name, name)
		}

		reloaded, err := svc.ByClientID(ctx, got.ClientID)
		if err != nil {
			t.Fatalf("ByClientID: %v", err)
		}
		if reloaded.Name != name {
			t.Fatalf("name must survive a round trip: got %q, want %q", reloaded.Name, name)
		}
	}
}

// TestRegisterSelfHostedRejectsOverLongMultiByteName pins the other side: the
// limit is still enforced, just in the right unit. 65 characters is over the
// column width regardless of how many bytes they occupy.
func TestRegisterSelfHostedRejectsOverLongMultiByteName(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	tooLong := strings.Repeat("字", maxClientNameLen+1)
	if _, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", tooLong); !errors.Is(err, ErrClientNameTooLong) {
		t.Fatalf("expected ErrClientNameTooLong, got %v", err)
	}
}

// TestStatusUsableFailsClosed locks the allowlist's shape rather than its
// current contents: `revoked` must never be usable, and a status nobody has
// defined yet must be refused rather than silently permitted. A future
// migration adding a status without touching this list should tighten
// behaviour, not loosen it.
func TestStatusUsableFailsClosed(t *testing.T) {
	if !StatusUsable(ClientActive) {
		t.Error("active clients must be usable")
	}
	if !StatusUsable(ClientPending) {
		t.Error("pending clients must be usable today — self-hosted registration creates them pending and nothing activates them yet")
	}
	for _, status := range []string{ClientRevoked, "", "suspended", "ACTIVE"} {
		if StatusUsable(status) {
			t.Errorf("status %q must not be usable", status)
		}
	}
}

// TestUsableByClientIDRejectsRevoked is the enforcement half: before this,
// oauth_client.status was written and never read, so the column advertised a
// kill switch that did not exist.
func TestUsableByClientIDRejectsRevoked(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	oc, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	// Usable while pending.
	if _, err := svc.UsableByClientID(ctx, oc.ClientID); err != nil {
		t.Fatalf("a pending client must be usable today: %v", err)
	}

	if _, err := client.OAuthClient.UpdateOneID(oc.ID).SetStatus(ClientRevoked).Save(ctx); err != nil {
		t.Fatalf("revoke client: %v", err)
	}

	if _, err := svc.UsableByClientID(ctx, oc.ClientID); !errors.Is(err, ErrClientNotUsable) {
		t.Fatalf("expected ErrClientNotUsable for a revoked client, got %v", err)
	}
	// ByClientID itself must stay status-blind — it answers "does this row
	// exist", and callers that need the gate must ask for it explicitly.
	if _, err := svc.ByClientID(ctx, oc.ClientID); err != nil {
		t.Fatalf("ByClientID must still resolve a revoked client: %v", err)
	}
}
