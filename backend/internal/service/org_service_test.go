package service

import (
	"context"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// newTestEntClient is not defined in this package. The closest existing
// in-package helper with a matching signature — func(t *testing.T) *dbent.Client
// — is newPaymentConfigServiceTestClient (payment_config_service_test.go),
// which derives a unique in-memory sqlite DB name from t.Name(). Reused here
// per instruction to use the real existing helper name rather than inventing
// a second one.

func TestEnsurePersonalOrgIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOrgService(client)

	first, err := svc.EnsurePersonalOrg(ctx, 42, "saksham")
	if err != nil {
		t.Fatalf("first EnsurePersonalOrg: %v", err)
	}
	if !first.IsPersonal {
		t.Fatalf("expected IsPersonal=true, got false")
	}

	second, err := svc.EnsurePersonalOrg(ctx, 42, "saksham")
	if err != nil {
		t.Fatalf("second EnsurePersonalOrg: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("not idempotent: created org %d then %d", first.ID, second.ID)
	}
}

func TestEnsurePersonalOrgMakesUserOwner(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOrgService(client)

	org, err := svc.EnsurePersonalOrg(ctx, 7, "archit")
	if err != nil {
		t.Fatalf("EnsurePersonalOrg: %v", err)
	}

	role, err := svc.RoleIn(ctx, org.ID, 7)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != OrgRoleOwner {
		t.Fatalf("expected %q, got %q", OrgRoleOwner, role)
	}
}

// TestEnsurePersonalOrgIsRaceSafe fires N goroutines at EnsurePersonalOrg for
// the SAME user id, coordinated to genuinely overlap via a start channel, and
// asserts every call succeeds and every call returns the same org. This is
// the regression test for the double-fired-OAuth-callback / two-tabs bug:
// a naive read-then-create (no DB-level constraint spanning the invariant)
// lets two concurrent callers both miss the "does a personal org already
// exist" check and both create one. TestEnsurePersonalOrgIsIdempotent alone
// does not catch this — it calls the function twice serially, so it never
// exercises the race window.
func TestEnsurePersonalOrgIsRaceSafe(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOrgService(client)

	const n = 8
	const userID = int64(4242)

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*dbent.Org, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // released together so calls genuinely overlap
			org, err := svc.EnsurePersonalOrg(ctx, userID, "racer")
			results[i] = org
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: EnsurePersonalOrg returned error: %v", i, err)
		}
	}

	firstID := results[0].ID
	for i, org := range results {
		if org == nil {
			t.Fatalf("call %d: returned nil org with no error", i)
		}
		if org.ID != firstID {
			t.Fatalf("not race-safe: call 0 returned org %d, call %d returned org %d — user has two personal orgs", firstID, i, org.ID)
		}
	}

	// Exactly one personal org should exist for this user, with exactly one
	// OWNER membership — not one org per goroutine.
	orgs, err := svc.OrgsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("OrgsForUser: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected exactly 1 org for user, got %d", len(orgs))
	}

	role, err := svc.RoleIn(ctx, firstID, userID)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != OrgRoleOwner {
		t.Fatalf("expected %q, got %q", OrgRoleOwner, role)
	}
}

func TestRoleInReturnsEmptyForNonMember(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOrgService(client)

	org, err := svc.EnsurePersonalOrg(ctx, 7, "archit")
	if err != nil {
		t.Fatalf("EnsurePersonalOrg: %v", err)
	}

	role, err := svc.RoleIn(ctx, org.ID, 999)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != "" {
		t.Fatalf("expected empty role for non-member, got %q", role)
	}
}
