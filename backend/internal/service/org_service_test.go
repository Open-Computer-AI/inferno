package service

import (
	"context"
	"testing"
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
