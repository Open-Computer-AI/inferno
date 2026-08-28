package service

import (
	"context"
	"errors"
	"testing"
)

func TestValidateRedirectURI(t *testing.T) {
	good := []string{
		"https://agent-abc.tryopencomputer.com/auth/callback",
		"https://gw.example.com/some/prefix/auth/callback",
	}
	for _, u := range good {
		if err := ValidateRedirectURI(u); err != nil {
			t.Errorf("%q must be accepted, got %v", u, err)
		}
	}

	bad := map[string]string{
		"http://agent.example.com/auth/callback":          "plaintext http",
		"https://127.0.0.1:8765/auth/callback":            "loopback — this is exactly why the desktop must broker through its gateway",
		"https://localhost:8765/auth/callback":            "loopback by name",
		"https://LOCALHOST:8765/auth/callback":            "loopback by name, mixed case",
		"https://localhost./auth/callback":                "loopback by name, trailing dot",
		"https://127.0.0.1./auth/callback":                "loopback IPv4, trailing dot",
		"https://[::1]:8765/auth/callback":                "loopback v6",
		"https://0.0.0.0/auth/callback":                   "unspecified IPv4 — kernel-delivered to 127.0.0.1 on Linux",
		"https://[::]/auth/callback":                      "unspecified IPv6",
		"https://[::ffff:127.0.0.1]/auth/callback":        "IPv4-mapped loopback",
		"https://agent.example.com/callback":              "does not end in /auth/callback",
		"https://agent.example.com/auth/callbackXYZ":      "suffix trick — extra chars after callback",
		"https://agent.example.com/notauth/callback":      "suffix trick — wrong prefix segment",
		"https://agent.example.com/auth/callback?x=1":     "query string",
		"https://agent.example.com/auth/callback#f":       "fragment",
		"https://user:pw@agent.example.com/auth/callback": "userinfo",
		"":          "empty",
		"not-a-url": "unparseable",
	}
	for u, why := range bad {
		if err := ValidateRedirectURI(u); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must be rejected (%s), got %v", u, why, err)
		}
	}
}

// TestValidateRedirectURIRejectsOtherLoopbackOctets pins the "not just
// 127.0.0.1" requirement: the entire 127.0.0.0/8 block is loopback.
func TestValidateRedirectURIRejectsOtherLoopbackOctets(t *testing.T) {
	for _, u := range []string{
		"https://127.0.0.2/auth/callback",
		"https://127.255.255.255/auth/callback",
	} {
		if err := ValidateRedirectURI(u); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must be rejected as loopback, got %v", u, err)
		}
	}
}

func TestRedirectURIMatchesClient(t *testing.T) {
	const origin = "https://agent-abc.tryopencomputer.com"

	if err := RedirectURIMatchesClient(origin, origin+"/auth/callback"); err != nil {
		t.Errorf("matching origin must be accepted, got %v", err)
	}
	// A different host that merely *contains* the registered origin as a
	// substring must not pass — prefix matching is the classic hole here.
	for _, bad := range []string{
		"https://evil.com/auth/callback",
		"https://agent-abc.tryopencomputer.com.evil.com/auth/callback",
		"https://agent-abc.tryopencomputer.com:8443/auth/callback",
	} {
		if err := RedirectURIMatchesClient(origin, bad); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must not match origin %q, got %v", bad, origin, err)
		}
	}
}

// TestValidateRedirectOrigin covers the registration-time validator: an
// ORIGIN (no path at all), everything else ValidateRedirectURI enforces
// except the /auth/callback suffix.
func TestValidateRedirectOrigin(t *testing.T) {
	good := []string{
		"https://agent.example.com",
		"https://agent-abc.tryopencomputer.com",
	}
	for _, u := range good {
		if err := ValidateRedirectOrigin(u); err != nil {
			t.Errorf("%q must be accepted, got %v", u, err)
		}
	}

	bad := map[string]string{
		"http://agent.example.com":                "plaintext http",
		"https://127.0.0.1:8765":                  "loopback",
		"https://localhost:8765":                  "loopback by name",
		"https://localhost.":                      "loopback by name, trailing dot",
		"https://127.0.0.1.":                      "loopback IPv4, trailing dot",
		"https://[::1]:8765":                      "loopback v6",
		"https://0.0.0.0":                         "unspecified IPv4 — kernel-delivered to 127.0.0.1 on Linux",
		"https://[::]":                            "unspecified IPv6",
		"https://agent.example.com/auth/callback": "origin must not include a path",
		"https://agent.example.com/foo":           "origin must not include a non-trivial path",
		"https://agent.example.com?x=1":           "query string",
		"https://agent.example.com#f":             "fragment",
		"https://user:pw@agent.example.com":       "userinfo",
		"":                                        "empty",
		"not-a-url":                               "unparseable",
		"urn:ietf:wg:oauth:2.0:oob":               "the OOB placeholder is not a registrable origin",
	}
	for u, why := range bad {
		if err := ValidateRedirectOrigin(u); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must be rejected (%s), got %v", u, why, err)
		}
	}
}

// TestNormalizeRedirectOriginAcceptsAndNormalisesTrailingSlash covers fix 3
// from the security review: a bare trailing slash is semantically identical
// to no path (RedirectURIMatchesClient compares only Scheme+Host, never
// Path), so a human pasting a browser-address-bar URL should not be
// rejected for it — but the stored form must be canonical, not verbatim,
// so the column never accumulates two spellings of one origin.
func TestNormalizeRedirectOriginAcceptsAndNormalisesTrailingSlash(t *testing.T) {
	got, err := NormalizeRedirectOrigin("https://x.example.com/")
	if err != nil {
		t.Fatalf("trailing slash must be accepted, got %v", err)
	}
	if got != "https://x.example.com" {
		t.Fatalf("expected the trailing slash normalised away, got %q", got)
	}

	// Any other non-empty path is still rejected outright.
	if _, err := NormalizeRedirectOrigin("https://x.example.com/foo"); !errors.Is(err, ErrInvalidRedirectURI) {
		t.Fatalf("a real path must still be rejected, got %v", err)
	}
}

// TestNormalizeRedirectOriginStripsTrailingDot is the round-2 security
// review fix: isLoopbackHost's trailing-dot handling (added for F2) only
// ever operated on a local copy used for the loopback decision -- it never
// reached the string NormalizeRedirectOrigin actually returns/persists.
// "https://example.com." and "https://example.com" are one origin to DNS,
// but a client registered with the dotted spelling would never match its
// own authorize requests (nobody types the trailing dot) under
// RedirectURIMatchesClient's exact-Host-equality check. This must fail
// against the pre-fix code, which returned "https://example.com." verbatim.
func TestNormalizeRedirectOriginStripsTrailingDot(t *testing.T) {
	got, err := NormalizeRedirectOrigin("https://example.com./")
	if err != nil {
		t.Fatalf("a trailing-dot host must be accepted, got %v", err)
	}
	if got != "https://example.com" {
		t.Fatalf("expected the trailing dot normalised away, got %q", got)
	}
}

// TestNormalizeRedirectOriginStripsTrailingDotWithPort pins the fiddlier
// case the review specifically warned about: u.Host for
// "example.com.:8443" is "example.com.:8443" as a single string, so a
// naive strings.TrimSuffix(u.Host, ".") is a silent no-op whenever a port
// is present -- the dot is no longer at the end of the string. The fix
// must rebuild from u.Hostname() (port already split off) and reattach the
// port via net.JoinHostPort, not trim u.Host directly.
func TestNormalizeRedirectOriginStripsTrailingDotWithPort(t *testing.T) {
	got, err := NormalizeRedirectOrigin("https://example.com.:8443/")
	if err != nil {
		t.Fatalf("a trailing-dot host with a port must be accepted, got %v", err)
	}
	if got != "https://example.com:8443" {
		t.Fatalf("expected the trailing dot normalised away with the port preserved, got %q", got)
	}
}

// TestNormalizeRedirectOriginPreservesIPv6BracketsWithPort covers the other
// half of the fiddliness the review flagged: net.JoinHostPort re-brackets
// an IPv6 literal automatically, but a hand-rolled host+":"+port would
// produce an invalid, unbracketed authority. Uses a non-loopback IPv6
// literal so the loopback/unspecified rejection doesn't mask this check.
func TestNormalizeRedirectOriginPreservesIPv6BracketsWithPort(t *testing.T) {
	got, err := NormalizeRedirectOrigin("https://[2001:db8::1]:8443/")
	if err != nil {
		t.Fatalf("a non-loopback IPv6 host with a port must be accepted, got %v", err)
	}
	if got != "https://[2001:db8::1]:8443" {
		t.Fatalf("expected the IPv6 literal to keep its brackets, got %q", got)
	}
}

// TestRegisterSelfHostedStoresTrailingDotNormalized is the storage-level
// round trip: registering with the dotted form must store the dot-free
// canonical form, the same invariant TestRegisterSelfHostedNormalizesTrailingSlashOrigin
// pins for the trailing-slash case.
func TestRegisterSelfHostedStoresTrailingDotNormalized(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com./", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if got.RedirectURIOrigin != "https://agent-abc.example.com" {
		t.Fatalf("expected the stored origin to be normalised (no trailing dot), got %q", got.RedirectURIOrigin)
	}
}

// TestValidateRedirectURIRejectsUnspecifiedAddress pins fix 1: 0.0.0.0 and
// the IPv6 unspecified address are a DIFFERENT net.IP predicate
// (IsUnspecified, not IsLoopback) and were not covered before this fix,
// despite 0.0.0.0 being kernel-delivered to 127.0.0.1 on Linux — the same
// loopback-delivery vector this validator exists to close.
func TestValidateRedirectURIRejectsUnspecifiedAddress(t *testing.T) {
	for _, u := range []string{
		"https://0.0.0.0/auth/callback",
		"https://[::]/auth/callback",
	} {
		if err := ValidateRedirectURI(u); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("%q must be rejected as unspecified, got %v", u, err)
		}
	}
}

// TestIsOOBOrigin covers the predicate Task 4 uses to reject hermes-cli
// (migrations/905, redirect_uri_origin = the OOB placeholder) from the
// authorization_code flow explicitly, rather than falling through to
// RedirectURIMatchesClient and failing with a misleading "did not match".
func TestIsOOBOrigin(t *testing.T) {
	if !IsOOBOrigin("urn:ietf:wg:oauth:2.0:oob") {
		t.Error("the seeded OOB placeholder must be recognized")
	}
	for _, origin := range []string{
		"https://agent.example.com",
		"",
		"urn:ietf:wg:oauth:2.0:oob ", // trailing space is a different string, not the sentinel
	} {
		if IsOOBOrigin(origin) {
			t.Errorf("%q must not be treated as the OOB placeholder", origin)
		}
	}
}

// TestRegisterSelfHostedRejectsLoopbackOrigin is the Step 4 registration
// test the brief asks for: a loopback origin must never reach the database.
func TestRegisterSelfHostedRejectsLoopbackOrigin(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	for _, origin := range []string{
		"https://127.0.0.1",
		"https://localhost",
		"https://0.0.0.0",
		"http://agent.example.com", // plaintext, not loopback, but must also be rejected here
	} {
		if _, err := svc.RegisterSelfHosted(ctx, 1, 42, origin, ""); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("RegisterSelfHosted(origin=%q): expected ErrInvalidRedirectURI, got %v", origin, err)
		}
	}
}

// TestRegisterSelfHostedNormalizesTrailingSlashOrigin covers fix 3: a
// registrant pasting a browser-address-bar URL (trailing slash included)
// must be accepted, and the row must store the canonical spelling, not the
// verbatim input, so the column never accumulates two spellings of one
// origin.
func TestRegisterSelfHostedNormalizesTrailingSlashOrigin(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthClientService(client)

	got, err := svc.RegisterSelfHosted(ctx, 1, 42, "https://agent-abc.example.com/", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	if got.RedirectURIOrigin != "https://agent-abc.example.com" {
		t.Fatalf("expected the stored origin to be normalised (no trailing slash), got %q", got.RedirectURIOrigin)
	}
}
