package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/oauthauthorizationcode"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// failOnceRefreshTokenCache wraps a *fakeRefreshTokenCache and, while armed,
// fails the very first StoreRefreshToken call (issueRefreshToken's first
// Redis write) — simulating a transient infrastructure fault (a Redis
// hiccup) downstream of a fully-validated authorization code redemption,
// with nothing wrong with the caller's credential.
type failOnceRefreshTokenCache struct {
	*fakeRefreshTokenCache
	armed bool
}

func (f *failOnceRefreshTokenCache) StoreRefreshToken(ctx context.Context, tokenHash string, data *RefreshTokenData, ttl time.Duration) error {
	if f.armed {
		f.armed = false
		return errors.New("injected: transient refresh-token store failure")
	}
	return f.fakeRefreshTokenCache.StoreRefreshToken(ctx, tokenHash, data, ttl)
}

// pkcePair returns a fixed, valid-shaped PKCE verifier and its RFC 7636 S256
// challenge (BASE64URL-NOPAD(SHA256(verifier))). The verifier's exact
// content is irrelevant to these tests — only that redemption re-derives
// the same challenge from it.
func pkcePair() (verifier, challenge string) {
	verifier = "test-code-verifier-with-enough-entropy-1234567890abcdefghijk"
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

// authCodeFixture bundles the services and a registered client an
// authorization-code test needs. redirectURI is the client's registered
// origin plus the required /auth/callback suffix (service.ValidateRedirectURI).
type authCodeFixture struct {
	ctx         context.Context
	entClient   *dbent.Client
	authorize   *OAuthAuthorizeService
	tokens      *OAuthTokenService
	cache       *fakeRefreshTokenCache
	users       *userLookupStub
	clientID    string
	redirectURI string
}

func newAuthCodeFixture(t *testing.T) *authCodeFixture {
	t.Helper()
	ctx := context.Background()
	entClient := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(entClient)
	keys := NewOAuthKeyService(entClient)
	devices := NewOAuthDeviceService(entClient, "https://portal.example.com")
	authorize := NewOAuthAuthorizeService(entClient)
	cache := newFakeRefreshTokenCache()
	users := newUserLookupStub()
	tokens := NewOAuthTokenService(entClient, keys, devices, cache, users, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	return &authCodeFixture{
		ctx:         ctx,
		entClient:   entClient,
		authorize:   authorize,
		tokens:      tokens,
		cache:       cache,
		users:       users,
		clientID:    oc.ClientID,
		redirectURI: "https://agent.example.com/auth/callback",
	}
}

// issue issues a code for f's registered client with a fresh PKCE pair,
// returning the raw code and the verifier that will redeem it.
func (f *authCodeFixture) issue(t *testing.T) (code, verifier string) {
	t.Helper()
	verifier, challenge := pkcePair()
	code, err := f.authorize.IssueCode(f.ctx, IssueCodeInput{
		ClientID:            f.clientID,
		RedirectURI:         f.redirectURI,
		Scope:               ScopeInferenceInvoke,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		UserID:              42,
	})
	if err != nil {
		t.Fatalf("IssueCode: %v", err)
	}
	return code, verifier
}

// TestAuthorizationCodeRoundTrip is the happy path: issue with a challenge,
// redeem with the matching verifier.
func TestAuthorizationCodeRoundTrip(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	got, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, verifier)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}
	if got.Scope != ScopeInferenceInvoke {
		t.Fatalf("expected scope %q, got %q", ScopeInferenceInvoke, got.Scope)
	}

	// The row must now carry the issued family, ready for a replay to
	// revoke it.
	row, err := f.entClient.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.Code(code)).
		Only(f.ctx)
	if err != nil {
		t.Fatalf("query authorization code row: %v", err)
	}
	if row.Status != authCodeStatusConsumed {
		t.Fatalf("expected status %q after redemption, got %q", authCodeStatusConsumed, row.Status)
	}
	if row.IssuedTokenFamily == nil || *row.IssuedTokenFamily == "" {
		t.Fatal("expected issued_token_family to be recorded on the redeemed row")
	}
}

// TestAuthorizationCodeIsSingleUseAndReplayRevokes proves RFC 6749 §4.1.2's
// requirement: a replayed authorization code must not just fail for the
// replayer, it must kill the token family the FIRST redemption minted. A
// test that only checks the replay's own failure would pass against an
// implementation that merely marks the code consumed and leaves the stolen
// tokens live — assertion (b) below is the one that actually catches that.
func TestAuthorizationCodeIsSingleUseAndReplayRevokes(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	original, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, verifier)
	if err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	// (a) The replay itself must be rejected.
	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("(a) expected ErrInvalidGrant on replay, got %v", err)
	}

	// (b) The refresh token the FIRST redemption minted must now be dead —
	// the whole point of RFC 6749 §4.1.2's revoke-on-replay requirement.
	// A dead family means DeleteTokenFamily removed the cache entry
	// entirely, so the correct outcome is ErrInvalidGrant (not found), the
	// same as ExchangeRefreshToken's own reuse test asserts for its
	// equivalent case.
	if _, err := f.tokens.ExchangeRefreshToken(f.ctx, f.clientID, original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("(b) expected ErrInvalidGrant for the refresh token minted by the replayed code, got %v — "+
			"the tokens from a replayed authorization code must be revoked, not just the replay rejected", err)
	}
}

// TestAuthorizationCodeRejectsWrongVerifier: challenge derived from verifier
// A, redemption attempted with verifier B.
func TestAuthorizationCodeRejectsWrongVerifier(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, _ := f.issue(t)

	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, "a-completely-different-verifier-value"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a mismatched code_verifier, got %v", err)
	}
}

// TestAuthorizationCodeRejectsRedirectURIMismatch: redemption presents a
// different redirect_uri than the one bound at issue.
func TestAuthorizationCodeRejectsRedirectURIMismatch(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, "https://attacker.example.com/auth/callback", verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a mismatched redirect_uri, got %v", err)
	}
}

// TestAuthorizationCodeRejectsPlainChallengeMethod: code_challenge_method
// "plain" must be refused at issue time — RFC 7636 permits it, but it
// provides no protection, so this server only ever issues S256 codes.
func TestAuthorizationCodeRejectsPlainChallengeMethod(t *testing.T) {
	f := newAuthCodeFixture(t)
	_, challenge := pkcePair()

	_, err := f.authorize.IssueCode(f.ctx, IssueCodeInput{
		ClientID:            f.clientID,
		RedirectURI:         f.redirectURI,
		Scope:               ScopeInferenceInvoke,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "plain",
		UserID:              42,
	})
	if !errors.Is(err, ErrPlainChallengeMethodRejected) {
		t.Fatalf("expected ErrPlainChallengeMethodRejected, got %v", err)
	}
}

// TestAuthorizationCodeRejectsForeignClient: a code issued for client A is
// not redeemable by client B — the classic authorization-code interception
// vector RFC 6749 §10.5 exists to close.
func TestAuthorizationCodeRejectsForeignClient(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	clients := NewOAuthClientService(f.entClient)
	other, err := clients.RegisterSelfHosted(f.ctx, 1, 43, "https://other-agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted (other client): %v", err)
	}

	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, other.ClientID, code, f.redirectURI, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a code redeemed by a different client, got %v", err)
	}
}

// TestAuthorizationCodeInternalFailureRollsBackAndFamilyIsRecordedBeforeMint
// covers two of the review's Important findings at once:
//
//  1. An internal failure downstream of the consuming CAS (here: a
//     transient refresh-token-store fault, the exact scenario the review
//     used — "OAuthKeyService.Active blips... the user is bounced through a
//     full browser re-login for a fault that had nothing to do with their
//     credential") must roll the code back to "pending" rather than burn it
//     — proven by redeeming the SAME code a second time, after the
//     injected fault clears, and getting real tokens instead of
//     ErrInvalidGrant.
//  2. issued_token_family is recorded on the row BEFORE minting is
//     attempted, not after — proven because the row still carries a
//     non-nil IssuedTokenFamily immediately after the failed (and rolled
//     back) first attempt, even though that attempt never produced a
//     token. The old ordering (record-after-mint) would have left
//     IssuedTokenFamily nil here, since the failure happens before the
//     record-family step ever runs in that ordering.
func TestAuthorizationCodeInternalFailureRollsBackAndFamilyIsRecordedBeforeMint(t *testing.T) {
	ctx := context.Background()
	entClient := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(entClient)
	keys := NewOAuthKeyService(entClient)
	devices := NewOAuthDeviceService(entClient, "https://portal.example.com")
	authorize := NewOAuthAuthorizeService(entClient)
	cache := &failOnceRefreshTokenCache{fakeRefreshTokenCache: newFakeRefreshTokenCache(), armed: true}
	users := newUserLookupStub()
	tokens := NewOAuthTokenService(entClient, keys, devices, cache, users, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	redirectURI := "https://agent.example.com/auth/callback"
	verifier, challenge := pkcePair()
	code, err := authorize.IssueCode(ctx, IssueCodeInput{
		ClientID:            oc.ClientID,
		RedirectURI:         redirectURI,
		Scope:               ScopeInferenceInvoke,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		UserID:              42,
	})
	if err != nil {
		t.Fatalf("IssueCode: %v", err)
	}

	// First attempt: the code is fully valid, but the refresh-token store
	// fails with an injected transient error.
	if _, err := tokens.ExchangeAuthorizationCode(ctx, oc.ClientID, code, redirectURI, verifier); err == nil {
		t.Fatal("expected the injected store failure to surface as an error")
	} else if errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("an internal infrastructure failure must NOT be folded into ErrInvalidGrant, got %v", err)
	}

	row, err := entClient.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.Code(code)).
		Only(ctx)
	if err != nil {
		t.Fatalf("query authorization code row after failed attempt: %v", err)
	}
	if row.Status != authCodeStatusPending {
		t.Fatalf("expected status %q after an internal failure (rolled back), got %q", authCodeStatusPending, row.Status)
	}
	if row.IssuedTokenFamily == nil || *row.IssuedTokenFamily == "" {
		t.Fatal("expected issued_token_family to already be recorded on the row even though minting failed — " +
			"it must be written BEFORE mintAccessToken/issueRefreshToken run, not after")
	}

	// Second attempt, fault cleared: the SAME code must still redeem
	// successfully — this is the whole point of rolling back rather than
	// leaving the code permanently burned by an unrelated infrastructure
	// blip.
	got, err := tokens.ExchangeAuthorizationCode(ctx, oc.ClientID, code, redirectURI, verifier)
	if err != nil {
		t.Fatalf("retry after the fault cleared should succeed, got %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens on the successful retry")
	}
}

// TestAuthorizationCodeReplayRevocationIsGatedOnClientID proves the
// Minor-4 review fix: a client_id that is NOT the one an authorization code
// was bound to at issue must not be able to revoke the family a DIFFERENT,
// legitimate client already redeemed, merely by replaying that other
// client's (leaked, e.g. via proxy logs or Referer) code with its own
// client_id. Revocation must be gated on row.ClientID == the presented
// client_id, not fire for any replay of any code.
func TestAuthorizationCodeReplayRevocationIsGatedOnClientID(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	original, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, verifier)
	if err != nil {
		t.Fatalf("legitimate exchange: %v", err)
	}

	clients := NewOAuthClientService(f.entClient)
	foreign, err := clients.RegisterSelfHosted(f.ctx, 1, 43, "https://foreign-agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted (foreign client): %v", err)
	}

	// The foreign client replays the ALREADY-CONSUMED code under its own
	// client_id. It must be rejected exactly like any other replay...
	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, foreign.ClientID, code, f.redirectURI, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a cross-client replay, got %v", err)
	}

	// ...but it must NOT have been able to revoke the legitimate client's
	// family: the original refresh token must still work. Without the
	// client_id gate, the line above would have called DeleteTokenFamily on
	// f.clientID's family, and this call would fail with ErrInvalidGrant
	// instead of succeeding.
	if _, err := f.tokens.ExchangeRefreshToken(f.ctx, f.clientID, original.RefreshToken); err != nil {
		t.Fatalf("expected the legitimate client's refresh token to still be live after a cross-client replay attempt, got %v", err)
	}
}

// TestAuthorizationCodeExpires: past expires_at must reject redemption, and
// no tokens must be minted.
func TestAuthorizationCodeExpires(t *testing.T) {
	f := newAuthCodeFixture(t)
	code, verifier := f.issue(t)

	if _, err := f.entClient.OAuthAuthorizationCode.Update().
		Where(oauthauthorizationcode.Code(code)).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		Save(f.ctx); err != nil {
		t.Fatalf("backdate expires_at: %v", err)
	}

	if _, err := f.tokens.ExchangeAuthorizationCode(f.ctx, f.clientID, code, f.redirectURI, verifier); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for an expired code, got %v", err)
	}

	// No tokens must have been minted: the row was consumed (single-use is
	// unconditional, see ExchangeAuthorizationCode's docs) but never got a
	// family recorded.
	row, err := f.entClient.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.Code(code)).
		Only(f.ctx)
	if err != nil {
		t.Fatalf("query authorization code row: %v", err)
	}
	if row.IssuedTokenFamily != nil {
		t.Fatal("expected no issued_token_family for an expired code — no tokens should have been minted")
	}
}
