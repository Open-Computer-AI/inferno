package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// newPersonalOrgLoginTestAuthService builds the smallest AuthService that can
// run GenerateTokenPair: a real ent client (SQLite in-memory), a real
// OrgService over it, the in-package fake RefreshTokenCache, and just enough
// JWT config to sign an HS256 access token. Every other collaborator is nil —
// GenerateTokenPair touches none of them, and passing nil is the established
// pattern for AuthService's optional dependencies in this package.
func newPersonalOrgLoginTestAuthService(t *testing.T) (*AuthService, *OrgService) {
	t.Helper()
	client := newPaymentConfigServiceTestClient(t)
	orgSvc := NewOrgService(client)
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-secret-for-personal-org-login-hook-0000000000"
	cfg.JWT.AccessTokenExpireMinutes = 15
	cfg.JWT.RefreshTokenExpireDays = 30
	svc := NewAuthService(
		client,
		nil, // userRepo
		nil, // redeemRepo
		newFakeRefreshTokenCache(),
		cfg,
		nil, // settingService
		nil, // emailService
		nil, // turnstileService
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner
		nil, // affiliateService
		nil, // userPlatformQuotaRepo
		orgSvc,
	)
	return svc, orgSvc
}

// TestLoginEnsuresPersonalOrgForPreExistingUser is the regression test for the
// deploy-day breaker: EnsurePersonalOrg used to be called ONLY on the three
// registration paths, so every user who signed up before the org tables
// existed had zero org_members rows forever — POST /api/oauth/self-hosted-client
// 500s for them (oauth_handler.go's len(orgs)==0 branch) and GET
// /api/oauth/account reports no org. Migration 904 added personal_user_id with
// no backfill, so nothing else would ever repair them.
//
// The fix is a login-time hook, which also covers the second hole: the
// registration-time call is deliberately fail-open (warn and continue), so a
// transient DB fault at signup used to leave even a BRAND NEW user permanently
// org-less despite the log line promising "will retry on next login" — there
// was no next-login retry.
//
// This test asserts the property at the funnel every authenticated session
// passes through, GenerateTokenPair, rather than at any single caller, so
// adding a new login path cannot silently reintroduce the gap.
func TestLoginEnsuresPersonalOrgForPreExistingUser(t *testing.T) {
	ctx := context.Background()
	svc, orgSvc := newPersonalOrgLoginTestAuthService(t)

	// A user who predates the org tables: a row in users, nothing in orgs.
	user := &User{
		ID:       4242,
		Email:    "legacy@example.com",
		Username: "legacy",
		Role:     RoleUser,
		Status:   StatusActive,
	}

	before, err := orgSvc.OrgsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("OrgsForUser (pre-check): %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("fixture setup bug: expected the user to start with no orgs, got %d", len(before))
	}

	if _, err := svc.GenerateTokenPair(ctx, user, ""); err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	after, err := orgSvc.OrgsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("OrgsForUser (post-check): %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected exactly 1 personal org after login, got %d — "+
			"POST /api/oauth/self-hosted-client 500s and GET /api/oauth/account "+
			"reports no org for this user", len(after))
	}
	if !after[0].IsPersonal {
		t.Fatalf("expected the auto-created org to be personal")
	}

	role, err := orgSvc.RoleIn(ctx, after[0].ID, user.ID)
	if err != nil {
		t.Fatalf("RoleIn: %v", err)
	}
	if role != OrgRoleOwner {
		t.Fatalf("expected %q membership after login, got %q", OrgRoleOwner, role)
	}
}

// TestLoginPersonalOrgHookIsIdempotentAcrossSessions proves the hook does not
// mint a second org on every subsequent login (which would make
// OrgsForUser(...)[0] — how the self-hosted-client handler picks the owning
// org — non-deterministic, registering an agent under one org and billing it
// against another).
func TestLoginPersonalOrgHookIsIdempotentAcrossSessions(t *testing.T) {
	ctx := context.Background()
	svc, orgSvc := newPersonalOrgLoginTestAuthService(t)

	user := &User{
		ID:       77,
		Email:    "repeat@example.com",
		Username: "repeat",
		Role:     RoleUser,
		Status:   StatusActive,
	}

	for i := 0; i < 3; i++ {
		if _, err := svc.GenerateTokenPair(ctx, user, ""); err != nil {
			t.Fatalf("GenerateTokenPair (login %d): %v", i+1, err)
		}
	}

	orgs, err := orgSvc.OrgsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("OrgsForUser: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected exactly 1 org after 3 logins, got %d", len(orgs))
	}
}

// TestLoginSucceedsWhenPersonalOrgProvisioningFails locks the fail-open
// posture: the org hook must never be able to block a login. A nil OrgService
// is the cheapest stand-in for "org provisioning is unavailable" that does not
// require a broken ent client — the guard that skips a nil OrgService is the
// same branch that must swallow a real failure.
func TestLoginSucceedsWhenPersonalOrgProvisioningFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-secret-for-personal-org-login-hook-0000000000"
	cfg.JWT.AccessTokenExpireMinutes = 15
	cfg.JWT.RefreshTokenExpireDays = 30
	svc := NewAuthService(client, nil, nil, newFakeRefreshTokenCache(), cfg,
		nil, nil, nil, nil, nil, nil, nil, nil, nil /* orgService */)

	pair, err := svc.GenerateTokenPair(ctx, &User{
		ID: 5, Email: "noorg@example.com", Role: RoleUser, Status: StatusActive,
	}, "")
	if err != nil {
		t.Fatalf("GenerateTokenPair must not fail when org provisioning is unavailable: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected a usable token pair, got %+v", pair)
	}
}
