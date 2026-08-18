package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthauthorizationcode"
)

// Authorization-code lifecycle. Deliberately just two states, unlike the
// device flow's four: an authorization code has no human-in-the-loop
// approval step of its own (the browser-facing /oauth/authorize leg, Task
// 4, handles that before IssueCode is ever called), so there is nothing
// between "usable" and "used".
const (
	authCodeStatusPending  = "pending"
	authCodeStatusConsumed = "consumed"
)

// authorizationCodeTTL is RFC 6749 §4.1.2's "SHOULD expire shortly after
// issuance" -- ten minutes comfortably covers a human completing a login in
// a browser tab while still bounding how long a leaked code stays live.
const authorizationCodeTTL = 10 * time.Minute

// ErrPlainChallengeMethodRejected is returned by IssueCode when the caller
// requests code_challenge_method=plain. RFC 7636 permits "plain" but it
// provides no protection over a bare authorization code (the verifier IS
// the challenge), so this authorization server only ever issues S256 codes.
var ErrPlainChallengeMethodRejected = errors.New("oauth: code_challenge_method must be S256")

// ErrMissingCodeChallenge is returned by IssueCode when code_challenge is
// absent or is not shaped like a valid RFC 7636 S256 challenge.
//
// Deliberately its own sentinel, NOT ErrInvalidRedirectURI: this is an
// error about the request's PKCE parameters, not about where to send the
// browser. RFC 6749 §4.1.2.1 splits authorize-time errors into two classes
// with different handling — "redirect_uri itself is untrustworthy" MUST
// NOT redirect at all (render an error page, since redirecting would hand
// the browser to a URI the server never validated), while every other
// rejection MUST redirect back to the (now-validated) redirect_uri with
// `error=...` in the query string. Task 4's /oauth/authorize handler
// branches on ErrInvalidRedirectURI to pick the first arm; a client that
// simply forgot code_challenge needs the second arm (a redirect carrying
// invalid_request), not an opaque error page its state machine never
// expected.
var ErrMissingCodeChallenge = errors.New("oauth: code_challenge is required and must be a valid RFC 7636 S256 challenge")

// ErrUnknownClient is returned by IssueCode when client_id does not resolve
// to a usable oauth_client (unregistered, or registered but revoked/not yet
// active — see OAuthClientService.UsableByClientID).
//
// Its own sentinel for the same reason ErrMissingCodeChallenge is: RFC 6749
// §4.1.2.1 puts an invalid client_id in the same MUST-NOT-redirect class as
// an untrustworthy redirect_uri — the server has no registered origin to
// validate the caller's redirect_uri against, so it cannot safely send the
// browser anywhere the caller names. Before this existed, an unknown
// client_id fell through to a bare wrapped error with no sentinel Task 4
// could branch on; a handler written against this file's own guidance
// ("branches on ErrInvalidRedirectURI to pick the must-not-redirect arm")
// would have treated an unknown client_id as an ordinary rejection and
// redirected anyway — to a redirect_uri that was never validated against
// any registration. That is an open redirect: supply a bogus client_id
// alongside an attacker-controlled redirect_uri and the browser gets
// bounced there. Task 4 MUST treat ErrUnknownClient the same as
// ErrInvalidRedirectURI: render an error page, never redirect.
//
// IssueCode wraps this with BOTH the sentinel and the underlying
// UsableByClientID error via two %w verbs — never %v for the inner error —
// so the cause chain survives and Task 4 can tell "genuinely unknown/
// revoked" apart from "the client lookup itself failed" (a transient DB
// fault) without a second round trip: errors.Is(err, ErrClientNotUsable)
// is the deliberate policy collapse (stays an error page, no distinction on
// the wire — revoked and unknown read the same to the caller on purpose);
// anything else wrapped here that is NOT ErrClientNotUsable is an
// infrastructure fault and belongs on a logged 500, not silently rendered
// as "unknown client" — the same misclassification this sentinel was
// introduced to prevent for the redirect-uri case, and the reason it is a
// distinct check from the collapsed-on-the-wire ErrClientNotUsable case.
var ErrUnknownClient = errors.New("oauth: client_id is unknown or not usable")

// codeChallengeLen is the fixed length of a valid RFC 7636 S256 challenge:
// BASE64URL-NOPAD(SHA256(verifier)) is always 43 characters, since a SHA256
// digest is 32 bytes and base64url encodes 6 bits per output character (32*8
// = 256 bits -> ceil(256/6) = 43 characters, no padding).
const codeChallengeLen = 43

// ValidCodeChallengeShape reports whether s is shaped like a real RFC 7636
// S256 challenge: exactly codeChallengeLen unpadded base64url characters.
//
// This does NOT verify the challenge matches any particular verifier — that
// happens at redemption (OAuthTokenService.pkceChallengeMatches). A
// malformed challenge already fails closed today (it simply never matches
// any verifier at redemption), but validating its shape at issue time turns
// a silent invalid_grant several steps later into an immediate, diagnosable
// invalid_request at the point the caller actually made the mistake.
//
// Exported (Task 4) so /oauth/authorize's handler can apply the identical
// check before ever calling IssueCode: that lets it redirect a malformed
// PKCE request straight to redirect_uri with error=invalid_request without
// first forcing the browser through a login round trip to answer a
// question that doesn't depend on who is asking.
func ValidCodeChallengeShape(s string) bool {
	if len(s) != codeChallengeLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			continue
		default:
			return false
		}
	}
	return true
}

// IssueCodeInput is everything IssueCode needs to mint an authorization
// code. ClientID and RedirectURI are validated against the client's
// registered origin (service.RedirectURIMatchesClient); CodeChallenge is
// stored verbatim and compared at redemption via
// OAuthTokenService.ExchangeAuthorizationCode.
type IssueCodeInput struct {
	ClientID            string
	RedirectURI         string
	Scope               string
	CodeChallenge       string
	CodeChallengeMethod string
	UserID              int64
}

// OAuthAuthorizeService issues RFC 6749 §4.1 authorization codes, PKCE-bound
// per RFC 7636. It does NOT redeem them -- that is
// OAuthTokenService.ExchangeAuthorizationCode, which owns the atomic
// single-use consumption and replay-revocation logic. Splitting issue from
// redeem across two services mirrors OAuthDeviceService (issues device
// codes) vs OAuthTokenService (redeems them).
type OAuthAuthorizeService struct {
	entClient *dbent.Client
	clientSvc *OAuthClientService
}

// NewOAuthAuthorizeService constructs the authorization-code issuance
// service.
func NewOAuthAuthorizeService(entClient *dbent.Client) *OAuthAuthorizeService {
	return &OAuthAuthorizeService{
		entClient: entClient,
		// Constructed here rather than injected, matching OAuthDeviceService
		// and OAuthTokenService: all three live in package service over the
		// same ent client, and threading a wire dependency through for one
		// lookup buys nothing.
		clientSvc: NewOAuthClientService(entClient),
	}
}

// IssueCode validates the request and persists a pending authorization
// code.
//
// Validation order is deliberate: the code_challenge_method check runs
// first and requires no database round trip, so a request with the wrong
// method (a caller that hasn't implemented PKCE properly, or is probing)
// is rejected before touching the client registry at all. Scope is
// validated against the same closed vocabulary RequestCode uses, for the
// same reason: an authorization code is exactly as capable as the scope
// string embedded in the access token it will mint, and an unvalidated
// scope here would let a caller mint a token with a scope nothing at
// /oauth/authorize (Task 4) actually offered the human.
func (s *OAuthAuthorizeService) IssueCode(ctx context.Context, in IssueCodeInput) (string, error) {
	if in.CodeChallengeMethod != "S256" {
		return "", ErrPlainChallengeMethodRejected
	}
	if !ValidCodeChallengeShape(in.CodeChallenge) {
		return "", ErrMissingCodeChallenge
	}
	if err := ValidateScope(in.Scope); err != nil {
		return "", err
	}

	client, err := s.clientSvc.UsableByClientID(ctx, in.ClientID)
	if err != nil {
		return "", fmt.Errorf("%w: client_id %q: %w", ErrUnknownClient, in.ClientID, err)
	}

	// A client whose registered redirect is the device-flow OOB placeholder
	// has no real origin to match against -- IsOOBOrigin MUST be checked
	// before RedirectURIMatchesClient, exactly as the design doc for that
	// function requires, or the OOB client falls through to origin matching
	// and fails with a misleading "redirect_uri does not match" instead of
	// the specific reason.
	if IsOOBOrigin(client.RedirectURIOrigin) {
		return "", fmt.Errorf("%w: client has no redirect URI and cannot use the authorization_code grant", ErrInvalidRedirectURI)
	}
	if err := RedirectURIMatchesClient(client.RedirectURIOrigin, in.RedirectURI); err != nil {
		return "", err
	}

	// Invalidate any still-pending code already issued for this exact
	// (client, user) pair before minting a new one. Without this, every
	// call to this function -- a remount of AuthorizeConsentView.vue
	// (Back-navigation, a refresh, a duplicated mount), or a repeat
	// auto-approve GET -- adds ANOTHER independently redeemable
	// credential on top of whatever is already outstanding, rather than
	// replacing it: N calls means N live codes, each good for its own
	// token exchange, not one. Task 3's single-use/replay-revocation
	// logic (ExchangeAuthorizationCode) operates per-code and does
	// nothing to bound that count.
	//
	// Marked "consumed" with issued_token_family left nil -- the exact
	// state this schema already defines for "a presentation that failed
	// validation, so nothing was ever issued and a later replay has
	// nothing to revoke" (see the field's doc comment on
	// ent/schema/oauth_authorization_code.go). A superseded-but-never-
	// redeemed code fits that same description. Deliberately not wrapped
	// in a transaction with the Create below: the invalidation window is
	// a few milliseconds against a 10-minute TTL, and the property this
	// closes is "how many codes can be outstanding at once", not
	// single-use (which the CAS in ExchangeAuthorizationCode already
	// guarantees regardless).
	if _, err := s.entClient.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.ClientID(in.ClientID),
			oauthauthorizationcode.UserID(in.UserID),
			oauthauthorizationcode.Status(authCodeStatusPending),
		).
		SetStatus(authCodeStatusConsumed).
		Save(ctx); err != nil {
		return "", fmt.Errorf("invalidate prior pending codes: %w", err)
	}

	code, err := newAuthorizationCode()
	if err != nil {
		return "", fmt.Errorf("authorization code entropy: %w", err)
	}

	_, err = s.entClient.OAuthAuthorizationCode.Create().
		SetCode(code).
		SetClientID(in.ClientID).
		SetUserID(in.UserID).
		SetRedirectURI(in.RedirectURI).
		SetScope(in.Scope).
		SetCodeChallenge(in.CodeChallenge).
		SetCodeChallengeMethod(in.CodeChallengeMethod).
		SetStatus(authCodeStatusPending).
		SetExpiresAt(time.Now().Add(authorizationCodeTTL)).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("persist authorization code: %w", err)
	}

	return code, nil
}

// newAuthorizationCode draws 32 bytes (256 bits) of crypto/rand, hex-encoded
// -- the same entropy shape as device_code and the OAuth refresh token. A
// predictable code is a free login, so the error from rand.Read is checked
// rather than silently degrading to whatever the buffer happened to contain.
func newAuthorizationCode() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
