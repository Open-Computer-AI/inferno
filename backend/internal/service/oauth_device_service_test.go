package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthdeviceauthorization"
)

func TestRequestCodeReturnsEveryRequiredField(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	// hermes_cli/auth.py hard-fails if ANY of these is missing.
	if grant.DeviceCode == "" {
		t.Error("missing device_code")
	}
	if grant.UserCode == "" {
		t.Error("missing user_code")
	}
	if grant.VerificationURI == "" {
		t.Error("missing verification_uri")
	}
	if grant.VerificationURIComplete == "" {
		t.Error("missing verification_uri_complete")
	}
	if grant.ExpiresIn <= 0 {
		t.Error("missing/invalid expires_in")
	}
	if grant.Interval <= 0 {
		t.Error("missing/invalid interval")
	}
	if !strings.Contains(grant.VerificationURIComplete, grant.UserCode) {
		t.Errorf("verification_uri_complete %q should embed user_code %q",
			grant.VerificationURIComplete, grant.UserCode)
	}
}

func TestUserCodeAvoidsAmbiguousCharacters(t *testing.T) {
	for _, bad := range []string{"0", "O", "1", "I", "L"} {
		if strings.Contains(UserCodeAlphabet, bad) {
			t.Errorf("user code alphabet must not contain %q — it is read aloud and typed", bad)
		}
	}
}

func TestApproveMarksTheRowApproved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	if err := svc.Approve(ctx, grant.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	row, err := svc.byDeviceCode(ctx, grant.DeviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("expected approved, got %q", row.Status)
	}
	if row.ApprovedUserID == nil || *row.ApprovedUserID != 42 {
		t.Fatalf("expected approved_user_id=42, got %v", row.ApprovedUserID)
	}
}

func TestDenyMarksTheRowDenied(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	if err := svc.Deny(ctx, grant.UserCode); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	row, err := svc.byDeviceCode(ctx, grant.DeviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if row.Status != "denied" {
		t.Fatalf("expected denied, got %q", row.Status)
	}
	if row.ApprovedUserID != nil {
		t.Fatalf("expected no approved_user_id on deny, got %v", *row.ApprovedUserID)
	}
}

func TestRequestCodeRejectsUnknownClient(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	if _, err := svc.RequestCode(ctx, "agent:does-not-exist", "inference:invoke"); err == nil {
		t.Fatal("expected an error for an unregistered client_id")
	}
}

func TestRequestCodeRejectsEmptyPortalBaseURL(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	_, err = svc.RequestCode(ctx, oc.ClientID, "inference:invoke")
	if !errors.Is(err, ErrPortalNotConfigured) {
		t.Fatalf("expected ErrPortalNotConfigured, got %v", err)
	}
}

func TestApproveUnknownUserCodeReturnsErrDeviceCodeNotFound(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	err := svc.Approve(ctx, "ZZZZ-ZZZZ", 1)
	if !errors.Is(err, ErrDeviceCodeNotFound) {
		t.Fatalf("expected ErrDeviceCodeNotFound, got %v", err)
	}
}

// byUserCodeForTest exposes the raw row so these tests can assert on the
// PERSISTED status/approved_user_id rather than only on the returned error —
// an illegal transition that returns an error but still writes would otherwise
// read as a pass.
func (s *OAuthDeviceService) byUserCodeForTest(ctx context.Context, userCode string) (*dbent.OAuthDeviceAuthorization, error) {
	return s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.UserCode(strings.ToUpper(strings.TrimSpace(userCode)))).
		Only(ctx)
}

// newDeviceApprovalFixture registers a client, starts a device flow, and
// returns the services plus the pending row's user_code.
func newDeviceApprovalFixture(t *testing.T, scope string) (context.Context, *dbent.Client, *OAuthClientService, *OAuthDeviceService, string, string) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, scope)
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	return ctx, client, clients, svc, oc.ClientID, grant.UserCode
}

// TestDeniedAuthorizationCannotBeApproved: a Deny is a security decision and
// must be final. Before setStatusByUserCode guarded on status, re-submitting
// the same user_code with Approve silently overturned it.
func TestDeniedAuthorizationCannotBeApproved(t *testing.T) {
	ctx, _, _, svc, _, userCode := newDeviceApprovalFixture(t, "inference:invoke")

	if err := svc.Deny(ctx, userCode); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if err := svc.Approve(ctx, userCode, 42); !errors.Is(err, ErrDeviceCodeNotPending) {
		t.Fatalf("expected ErrDeviceCodeNotPending approving a denied code, got %v", err)
	}

	row, err := svc.byUserCodeForTest(ctx, userCode)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if row.Status != DeviceStatusDenied {
		t.Fatalf("a denied authorization must stay denied, got %q", row.Status)
	}
	if row.ApprovedUserID != nil {
		t.Fatalf("a denied authorization must not acquire an approved_user_id, got %v", *row.ApprovedUserID)
	}
}

// TestConsumedAuthorizationCannotBeReApproved is the serious one.
// ExchangeDeviceCode marks a redeemed row `expired`. Without a status guard,
// a human who approves, sees nothing happen, and approves again — the most
// natural recovery there is — RE-ARMED that consumed row, letting whoever holds
// the device_code redeem a SECOND access token and a second 30-day refresh
// family out of one human approval.
func TestConsumedAuthorizationCannotBeReApproved(t *testing.T) {
	ctx, entClient, _, svc, _, userCode := newDeviceApprovalFixture(t, "inference:invoke")

	if err := svc.Approve(ctx, userCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	// Simulate ExchangeDeviceCode's consuming update.
	row, err := svc.byUserCodeForTest(ctx, userCode)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := entClient.OAuthDeviceAuthorization.UpdateOneID(row.ID).
		SetStatus(DeviceStatusExpired).Save(ctx); err != nil {
		t.Fatalf("consume: %v", err)
	}

	if err := svc.Approve(ctx, userCode, 42); !errors.Is(err, ErrDeviceCodeNotPending) {
		t.Fatalf("expected ErrDeviceCodeNotPending re-approving a consumed code, got %v", err)
	}

	reloaded, err := svc.byUserCodeForTest(ctx, userCode)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != DeviceStatusExpired {
		t.Fatalf("a consumed authorization must stay consumed, got %q — the device_code is redeemable a second time", reloaded.Status)
	}
}

// TestApprovedAuthorizationCannotBeDenied closes the mirror case: an approval
// already handed out (or about to hand out) a credential, so flipping the row
// to `denied` would misrepresent state without revoking anything.
func TestApprovedAuthorizationCannotBeDenied(t *testing.T) {
	ctx, _, _, svc, _, userCode := newDeviceApprovalFixture(t, "inference:invoke")

	if err := svc.Approve(ctx, userCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := svc.Deny(ctx, userCode); !errors.Is(err, ErrDeviceCodeNotPending) {
		t.Fatalf("expected ErrDeviceCodeNotPending denying an approved code, got %v", err)
	}
}

// TestRequestCodeRejectsUnknownScope is half one of the phishing fix: without
// an allowlist, `scope` was pure pass-through, so an attacker could start a
// device flow for the well-known public `hermes-cli` client asking for
// billing:manage and agents:manage, phish the user_code, and poll out a token
// carrying scopes the victim never saw.
func TestRequestCodeRejectsUnknownScope(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	for _, bad := range []string{
		"admin",
		"inference",                   // the pre-Task-8 spelling; exact-match only
		"inference:invoke billing:*",  // no wildcards
		"inference:invoke everything", // one bad entry poisons the request
	} {
		if _, err := svc.RequestCode(ctx, oc.ClientID, bad); !errors.Is(err, ErrInvalidScope) {
			t.Errorf("scope %q: expected ErrInvalidScope, got %v", bad, err)
		}
	}

	// The documented vocabulary, and an omitted scope, must all be accepted.
	for _, ok := range []string{
		"",
		ScopeInferenceInvoke,
		ScopeBillingRead,
		ScopeBillingManage,
		ScopeAgentsRead,
		ScopeAgentsManage,
		ScopeToolInvoke,
		ScopeInferenceInvoke + " " + ScopeBillingRead,
		// The real client's billing step-up fallback, verbatim:
		// hermes_cli/auth.py:8561 builds this set when prior auth state carries
		// no scope string, and :8540 documents it as the standard set. If this
		// line ever fails, `oc` cannot elevate to billing:manage.
		ScopeInferenceInvoke + " " + ScopeToolInvoke + " " + ScopeBillingManage,
	} {
		if _, err := svc.RequestCode(ctx, oc.ClientID, ok); err != nil {
			t.Errorf("scope %q must be accepted, got %v", ok, err)
		}
	}
}

// TestRequestCodeRejectsRevokedClient: the status column has to gate the flow,
// or `revoked` is decoration.
func TestRequestCodeRejectsRevokedClient(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if _, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke"); err != nil {
		t.Fatalf("pending client must be able to start a device flow: %v", err)
	}

	if _, err := client.OAuthClient.UpdateOneID(oc.ID).SetStatus(ClientRevoked).Save(ctx); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke"); !errors.Is(err, ErrClientNotUsable) {
		t.Fatalf("expected ErrClientNotUsable for a revoked client, got %v", err)
	}
}

// TestPendingByUserCodeShowsWhoIsAskingAndForWhat is half two of the phishing
// fix: RFC 8628 §5.4 requires the AS to display client and authorization
// information at the verification URI. The screen used to show a bare code box.
func TestPendingByUserCodeShowsWhoIsAskingAndForWhat(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "my-laptop")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference:invoke billing:read")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	// Lowercase and untrimmed, the way a person types it.
	pending, err := svc.PendingByUserCode(ctx, "  "+strings.ToLower(grant.UserCode)+" ")
	if err != nil {
		t.Fatalf("PendingByUserCode: %v", err)
	}
	if pending.ClientName != "my-laptop" {
		t.Fatalf("expected the client's name, got %q", pending.ClientName)
	}
	if pending.ClientID != oc.ClientID {
		t.Fatalf("expected client_id %q, got %q", oc.ClientID, pending.ClientID)
	}
	if len(pending.Scopes) != 2 || pending.Scopes[0] != "inference:invoke" || pending.Scopes[1] != "billing:read" {
		t.Fatalf("expected the requested scopes, got %v", pending.Scopes)
	}
	if pending.ExpiresAt.IsZero() {
		t.Fatal("expected an expiry")
	}
}

// TestPendingByUserCodeIsNotAnOracle: an unknown code and a code that has
// already been decided must not be distinguishable from each other by their
// error class alone in a way that lets a caller enumerate live codes. Unknown
// reads as not-found; a decided code reads as not-pending and exposes no
// client or scope information either way.
func TestPendingByUserCodeIsNotAnOracle(t *testing.T) {
	ctx, _, _, svc, _, userCode := newDeviceApprovalFixture(t, "inference:invoke")

	if _, err := svc.PendingByUserCode(ctx, "ZZZZ-ZZZZ"); !errors.Is(err, ErrDeviceCodeNotFound) {
		t.Fatalf("expected ErrDeviceCodeNotFound, got %v", err)
	}

	if err := svc.Approve(ctx, userCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	pending, err := svc.PendingByUserCode(ctx, userCode)
	if !errors.Is(err, ErrDeviceCodeNotPending) {
		t.Fatalf("expected ErrDeviceCodeNotPending for a decided code, got %v", err)
	}
	if pending != nil {
		t.Fatalf("no authorization details may be returned alongside an error, got %+v", pending)
	}
}

// TestRequestCodeValidatesScopeBeforeClientLookup: if the client lookup ran
// first, a caller sending a deliberately-bogus scope would get invalid_scope
// for a real client_id and invalid_client for a fake one — turning the
// difference between two error codes into a client_id existence oracle. Both
// must report the SAME class of failure.
func TestRequestCodeValidatesScopeBeforeClientLookup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	const bogusScope = "definitely-not-a-real-scope"

	_, realErr := svc.RequestCode(ctx, oc.ClientID, bogusScope)
	_, fakeErr := svc.RequestCode(ctx, "agent:does-not-exist-at-all", bogusScope)

	if !errors.Is(realErr, ErrInvalidScope) {
		t.Fatalf("registered client with a bad scope: expected ErrInvalidScope, got %v", realErr)
	}
	if !errors.Is(fakeErr, ErrInvalidScope) {
		t.Fatalf("unknown client with a bad scope: expected ErrInvalidScope too — "+
			"a different error here leaks whether the client_id exists; got %v", fakeErr)
	}
}
