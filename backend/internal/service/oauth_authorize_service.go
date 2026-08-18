package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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
	if in.CodeChallenge == "" {
		return "", fmt.Errorf("%w: empty code_challenge", ErrInvalidRedirectURI)
	}
	if err := ValidateScope(in.Scope); err != nil {
		return "", err
	}

	client, err := s.clientSvc.UsableByClientID(ctx, in.ClientID)
	if err != nil {
		return "", fmt.Errorf("client_id %q not usable: %w", in.ClientID, err)
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
