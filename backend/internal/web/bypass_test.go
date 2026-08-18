package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Deliberately untagged -- see bypass.go's doc comment for why. This file
// must compile and run under a plain `go test ./...`, the standing backend
// gate, with no `-tags embed` and no embedded dist required.

func TestEmbeddedFrontendBypassesBareVideoAPIRoutes(t *testing.T) {
	for _, path := range []string{
		"/videos/generations",
		"/videos/edits",
		"/videos/extensions",
		"/videos/request-123",
	} {
		require.True(t, shouldBypassEmbeddedFrontend(path), "path=%s", path)
	}
}

// TestEmbeddedFrontendBypassesOAuthAuthorize is Task 4's regression guard on
// the one line that makes GET/POST /oauth/authorize reachable at all in a
// production embed build: without it, the SPA fallback would serve
// index.html for this path (indistinguishable from any other unmatched
// route) and OAuthHandler.Authorize -- the handler that owns the RFC 6749
// §4.1.2.1 error-page/redirect split -- would never run.
func TestEmbeddedFrontendBypassesOAuthAuthorize(t *testing.T) {
	require.True(t, shouldBypassEmbeddedFrontend("/oauth/authorize"))
	// Sibling paths must NOT bypass -- only the exact literal path is a real
	// backend route; anything else at this prefix is still SPA territory.
	require.False(t, shouldBypassEmbeddedFrontend("/oauth/authorized"))
	require.False(t, shouldBypassEmbeddedFrontend("/oauth/authorize/consent"))
}

// TestEmbeddedFrontendBypassesJWKS regression-guards the fix for the
// /.well-known/jwks.json omission review found and the parent ruled back
// in scope: verified independently that a production embed build was
// answering every JWKS fetch with the SPA's index.html (200 text/html)
// instead of the key set, breaking every RS256 verifier reading it,
// including the real hermes client's PyJWKClient.
func TestEmbeddedFrontendBypassesJWKS(t *testing.T) {
	require.True(t, shouldBypassEmbeddedFrontend("/.well-known/jwks.json"))
	// Scoped to exactly this path, per the parent's ruling -- sweeping the
	// rest of RegisterCommonRoutes for the same gap is a separate job.
	require.False(t, shouldBypassEmbeddedFrontend("/.well-known/other"))
}
