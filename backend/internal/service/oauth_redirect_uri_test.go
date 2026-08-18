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
		"https://[::1]:8765/auth/callback":                "loopback v6",
		"https://agent.example.com/callback":              "does not end in /auth/callback",
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
		"https://[::1]:8765":                      "loopback v6",
		"https://agent.example.com/auth/callback": "origin must not include a path",
		"https://agent.example.com/":              "origin must not include a path (even bare slash)",
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
		"http://agent.example.com", // plaintext, not loopback, but must also be rejected here
	} {
		if _, err := svc.RegisterSelfHosted(ctx, 1, 42, origin, ""); !errors.Is(err, ErrInvalidRedirectURI) {
			t.Errorf("RegisterSelfHosted(origin=%q): expected ErrInvalidRedirectURI, got %v", origin, err)
		}
	}
}
