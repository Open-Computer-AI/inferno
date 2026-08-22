package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// The two halves of this file pin the ONE property the whole billing-write
// surface rests on, in the direction reality actually runs.
//
// The branch shipped the opposite claim in three places -- "billing:manage is
// refused outright and no other flow can grant it, so the seven writes are
// unreachable in production" -- and it was FALSE. It was written from
// OAuthAuthorizeService.IssueCode's refusal (oauth_authorize_service.go:221)
// alone; nobody read the DEVICE grant. Nothing on the branch could have caught
// it either: every write test mints its own token with
// env.mint(t, service.ScopeBillingManage), so the GATE was tested exhaustively
// and the ISSUANCE side -- whether such a token can exist at all -- was never
// asserted once.
//
// TestDeviceGrantIssuesBillingManageWhileAuthorizeCodeGrantRefusesIt is that
// missing assertion, and it deliberately asserts BOTH directions in one test:
// the point is not "the device flow works" or "authorize refuses", it is that
// the two grants DISAGREE, on purpose, and that the disagreement is what the
// design's "elevation needs a second, explicit act" rule is made of. Split
// across two tests, one of them could be deleted or skipped and the
// relationship would stop being pinned by anything.
//
// If the first half ever fails, `oc` can no longer elevate to billing:manage
// and /topup dead-ends at a 403 it has no recovery for
// (hermes_cli/cli_billing_mixin.py:1149-1153 routes BillingScopeRequired into
// _billing_handle_scope_required, which calls
// hermes_cli/auth.py:9093 step_up_nous_billing_scope, which re-runs THIS flow).
// If the second half ever fails, a silent redirect re-consent can hand out
// money-spending authority without a human ever seeing the scope.
func TestDeviceGrantIssuesBillingManageWhileAuthorizeCodeGrantRefusesIt(t *testing.T) {
	ctx := context.Background()
	entClient := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(entClient)
	keys := NewOAuthKeyService(entClient)
	devices := NewOAuthDeviceService(entClient, "https://portal.example.com")
	authorize := NewOAuthAuthorizeService(entClient)
	tokens := NewOAuthTokenService(entClient, keys, devices, newFakeRefreshTokenCache(), newUserLookupStub(), "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	// ---- Half 1: the DEVICE grant issues a token that really carries it. ----
	//
	// This is the exact scope string hermes_cli/auth.py:9119-9128 builds --
	// the prior login scope with billing:manage appended -- not a synthetic
	// one, so a vocabulary change that broke the real step-up would fail here.
	const stepUpScope = ScopeInferenceInvoke + " " + ScopeToolInvoke + " " + ScopeBillingManage

	grant, err := devices.RequestCode(ctx, oc.ClientID, stepUpScope)
	if err != nil {
		t.Fatalf("RequestCode with the client's real step-up scope must be accepted: %v", err)
	}

	// The human approval step RFC 8628 §3.3 requires, and the reason this
	// grant may carry a scope /oauth/authorize refuses: PendingByUserCode
	// shows the requested scopes to the user before Approve records anything.
	pending, err := devices.PendingByUserCode(ctx, grant.UserCode)
	if err != nil {
		t.Fatalf("PendingByUserCode: %v", err)
	}
	if !containsScope(pending.Scopes, ScopeBillingManage) {
		t.Fatalf("the approval screen must SHOW billing:manage before it can be approved, got %v", pending.Scopes)
	}

	if err := devices.Approve(ctx, grant.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	issued, err := tokens.ExchangeDeviceCode(ctx, oc.ClientID, grant.DeviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}
	if !ScopeContains(issued.Scope, ScopeBillingManage) {
		t.Fatalf("the token-endpoint response must report billing:manage, got %q", issued.Scope)
	}

	// The response field is not what the resource server reads -- middleware
	// reads the JWT's own `scope` claim -- so assert the CLAIM, which is the
	// thing that actually opens the seven write routes.
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(issued.AccessToken, claims); err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	claimScope, _ := claims["scope"].(string)
	if !ScopeContains(claimScope, ScopeBillingManage) {
		t.Fatalf("the ACCESS TOKEN's scope claim must carry billing:manage, got %q", claimScope)
	}
	// And it must carry the rest verbatim: the step-up is an addition, not a
	// replacement -- a token that traded inference:invoke away for
	// billing:manage would break every inference call the CLI makes next.
	if !ScopeContains(claimScope, ScopeInferenceInvoke) || !ScopeContains(claimScope, ScopeToolInvoke) {
		t.Fatalf("the step-up must PRESERVE the prior scopes, got %q", claimScope)
	}

	// ---- Half 2: the authorization_code grant still refuses it. ----
	_, challenge := pkcePair()
	if _, err := authorize.IssueCode(ctx, IssueCodeInput{
		ClientID:            oc.ClientID,
		RedirectURI:         "https://agent.example.com/auth/callback",
		Scope:               stepUpScope,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		UserID:              42,
	}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("the authorization_code grant must still refuse billing:manage, got %v", err)
	}
}

func containsScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
