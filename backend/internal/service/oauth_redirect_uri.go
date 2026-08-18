package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrInvalidRedirectURI is the sentinel every rejection in this file wraps.
// Callers use errors.Is against this single value; the wrapped detail (via
// fmt.Errorf("%w: ...")) is for logs/tests only and must never be echoed
// verbatim on the wire — handlers map it to a generic 400 invalid_request.
var ErrInvalidRedirectURI = errors.New("invalid redirect_uri")

// callbackSuffix is the required path suffix for a full redirect_uri
// presented at /oauth/authorize (Task 4). The design doc (D-3) requires it
// verbatim.
const callbackSuffix = "/auth/callback"

// oobRedirectOrigin is the conventional OAuth placeholder for a client that
// does not use redirects at all (RFC 8252-style "out of band"). It is NOT
// something a registration caller can ever supply — ValidateRedirectOrigin
// requires https and rejects a bare URN — it exists here only because
// migrations/905_hermes_cli_first_party_client.sql seeds the first-party
// `hermes-cli` client's redirect_uri_origin with exactly this value:
// hermes-cli is device-flow-only and has no redirect URI at all. That row
// was inserted by migration, not through RegisterSelfHosted, so it
// legitimately bypasses ValidateRedirectOrigin and is not drift.
const oobRedirectOrigin = "urn:ietf:wg:oauth:2.0:oob"

// IsOOBOrigin reports whether a stored oauth_client.redirect_uri_origin is
// the seeded out-of-band placeholder rather than a real https origin.
//
// This is exported for Task 4's authorize handler, which must check it
// BEFORE calling RedirectURIMatchesClient and reject such a client from the
// authorization_code flow with a clear, specific error (e.g. "this client
// has no redirect URI and cannot use the authorization_code grant").
// Without this check Task 4 would fall through to origin matching and fail
// with an opaque "redirect_uri did not match" — misleading for a client
// (hermes-cli) that never had a redirect URI to begin with. The predicate
// lives here, next to oobRedirectOrigin, so all redirect-URI reasoning
// stays in one file even though Task 4 is what consumes it.
func IsOOBOrigin(origin string) bool {
	return origin == oobRedirectOrigin
}

// parsedCandidate is the set of checks shared by both an ORIGIN (registration
// time) and a full redirect URI (authorize time): must parse, must be https,
// must have a non-empty host, no userinfo, no query, no fragment, and the
// host must not be loopback in any of its three forms.
func parsedCandidate(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidRedirectURI)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: unparseable: %v", ErrInvalidRedirectURI, err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme must be https", ErrInvalidRedirectURI)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidRedirectURI)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: userinfo not allowed", ErrInvalidRedirectURI)
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("%w: query string not allowed", ErrInvalidRedirectURI)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("%w: fragment not allowed", ErrInvalidRedirectURI)
	}
	if isLoopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("%w: loopback host not allowed — this is exactly why a "+
			"desktop client cannot talk to us directly and must broker through its gateway",
			ErrInvalidRedirectURI)
	}
	return u, nil
}

// isLoopbackHost covers all three forms a loopback host can take:
//   - an IPv4 literal anywhere in 127.0.0.0/8 (not just 127.0.0.1),
//   - the IPv6 loopback literal ::1,
//   - the literal name "localhost", compared case-insensitively.
//
// host is u.Hostname() — net/url already strips brackets from an IPv6
// literal and any port, so net.ParseIP sees a bare address either way.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ValidateRedirectOrigin validates a redirect_uri ORIGIN at client
// REGISTRATION time (RegisterSelfHosted, before the insert): https scheme,
// non-empty host, no userinfo/path/query/fragment, not loopback.
//
// The column this feeds, oauth_client.redirect_uri_origin, stores an
// ORIGIN, not a full redirect URI — every existing registration fixture
// passes a bare origin (e.g. "https://agent.example.com"), and the DTO
// field is literally named redirect_origin
// (handler/dto/oauth.go:SelfHostedClientRequest.RedirectOrigin). Applying
// ValidateRedirectURI's "/auth/callback" suffix requirement here would
// reject every one of those legitimate registrations.
//
// This validates INPUT at registration time only — it must never be run
// retroactively against rows already in the table. The migration-seeded
// hermes-cli row (redirect_uri_origin = the OOB placeholder, see
// IsOOBOrigin) legitimately fails this validator and is not drift: it was
// never passed through RegisterSelfHosted.
func ValidateRedirectOrigin(raw string) error {
	u, err := parsedCandidate(raw)
	if err != nil {
		return err
	}
	if u.Path != "" {
		return fmt.Errorf("%w: origin must not include a path", ErrInvalidRedirectURI)
	}
	return nil
}

// ValidateRedirectURI validates a FULL redirect_uri presented at
// /oauth/authorize (Task 4): everything ValidateRedirectOrigin checks, plus
// a path that must end in "/auth/callback".
//
// Kept as a separate exported function from ValidateRedirectOrigin rather
// than folded behind a boolean flag: the two call sites enforce genuinely
// different rules — an origin must have NO path at all, a redirect_uri must
// have exactly one specific path shape — and a flag would hide that the
// rules differ rather than express it.
func ValidateRedirectURI(raw string) error {
	u, err := parsedCandidate(raw)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(u.Path, callbackSuffix) {
		return fmt.Errorf("%w: path must end in %s", ErrInvalidRedirectURI, callbackSuffix)
	}
	return nil
}

// RedirectURIMatchesClient bridges the two validators above: it checks that
// a full redirect_uri presented at /oauth/authorize both (a) is itself
// valid per ValidateRedirectURI and (b) shares its scheme, host, and port
// with the client's registered origin.
//
// Comparison is PARSED scheme+host equality — net/url's u.Host already
// includes an explicit port — never strings.HasPrefix or any substring
// check. Prefix/substring matching is the classic hole here: it would
// accept "https://agent-abc.tryopencomputer.com.evil.com/auth/callback" as
// a match for the registered origin
// "https://agent-abc.tryopencomputer.com", and would ignore a port
// mismatch such as "https://host:8443/..." against a registered
// "https://host". Exact Host-field equality rejects both.
//
// registeredOrigin is expected to be a value ValidateRedirectOrigin already
// accepted (or the migration-seeded OOB placeholder). Callers MUST check
// IsOOBOrigin(registeredOrigin) before calling this function and reject
// that case explicitly with its own error — an OOB client has no origin to
// match against, and letting it reach here would fail closed but with a
// misleading "redirect_uri does not match" message.
func RedirectURIMatchesClient(registeredOrigin, redirectURI string) error {
	if err := ValidateRedirectURI(redirectURI); err != nil {
		return err
	}

	regURL, err := url.Parse(registeredOrigin)
	if err != nil || regURL.Scheme == "" || regURL.Host == "" {
		return fmt.Errorf("%w: registered origin is invalid", ErrInvalidRedirectURI)
	}

	reqURL, err := url.Parse(redirectURI)
	if err != nil {
		// Unreachable in practice: ValidateRedirectURI above already parsed
		// this string successfully. Fail closed rather than panic on a nil
		// reqURL if that invariant is ever broken by a future edit.
		return fmt.Errorf("%w: unparseable", ErrInvalidRedirectURI)
	}

	if regURL.Scheme != reqURL.Scheme || regURL.Host != reqURL.Host {
		return fmt.Errorf("%w: redirect_uri does not match the client's registered origin", ErrInvalidRedirectURI)
	}
	return nil
}
