// Package web provides embedded web assets for the application.
package web

import "strings"

// shouldBypassEmbeddedFrontend reports whether path is a real backend route
// that must skip FrontendServer.Middleware()'s SPA fallback (embed_on.go)
// and reach its registered gin handler instead of being served
// index.html.
//
// Deliberately in an UNTAGGED file (no `//go:build embed`), unlike the
// middleware that calls it. Review (Task 4 fix round 1, F4) found the
// regression test for this exact function -- the allowlist protecting the
// one line that makes GET/POST /oauth/authorize reachable in a production
// build at all -- carried the same `//go:build embed` tag as
// FrontendServer itself, which means `go test ./...` (the standing backend
// gate) never compiled it, and running it required BOTH `-tags embed` AND
// a real `internal/web/dist/index.html`, which is gitignored. A
// regression guard nothing in CI executes is not a regression guard. This
// file's split from embed_on.go exists solely so `shouldBypassEmbeddedFrontend`
// -- and the test asserting its contents -- compile and run under a plain
// `go test ./...`, independent of the embed tag or an embedded dist.
//
// This is the same root cause as the /.well-known/jwks.json gap fixed
// below: a tagged-out test is why neither omission was caught.
func shouldBypassEmbeddedFrontend(path string) bool {
	trimmed := strings.TrimSpace(path)
	return strings.HasPrefix(trimmed, "/api/") ||
		strings.HasPrefix(trimmed, "/v1/") ||
		strings.HasPrefix(trimmed, "/v1beta/") ||
		strings.HasPrefix(trimmed, "/backend-api/") ||
		strings.HasPrefix(trimmed, "/antigravity/") ||
		strings.HasPrefix(trimmed, "/setup/") ||
		trimmed == "/health" ||
		trimmed == "/models" ||
		trimmed == "/responses" ||
		strings.HasPrefix(trimmed, "/responses/") ||
		trimmed == "/alpha/search" ||
		strings.HasPrefix(trimmed, "/images/") ||
		strings.HasPrefix(trimmed, "/videos/") ||
		// GET/POST /oauth/authorize (Task 4 of the authorization_code+PKCE
		// plan): a real backend endpoint the hermes client's system browser
		// navigates to directly (see OAuthHandler.Authorize's doc comment),
		// not a Vue route -- without this it would be served index.html
		// like any other unmatched SPA path and the handler that does the
		// RFC 6749 §4.1.2.1 error-page/redirect split would never run.
		// The trailing-slash form too: gin's RedirectTrailingSlash never
		// gets a chance, because the frontend middleware runs BEFORE
		// registerRoutes (internal/server/router.go), so `/oauth/authorize/`
		// was answered with a 200 text/html SPA shell -- the same failure
		// mode as the JWKS gap below, one character over.
		trimmed == "/oauth/authorize" || trimmed == "/oauth/authorize/" ||
		// GET /.well-known/jwks.json (RegisterCommonRoutes, mounted at the
		// server root by Task 2 of the OAuth-AS plan). Same failure mode as
		// /oauth/authorize above and found the same way: verified that
		// deploy/Dockerfile builds with `-tags embed`, that
		// frontendServer.Middleware() is installed before registerRoutes
		// (internal/server/router.go), and that this path is not a file
		// under dist/ -- so every production build was falling through to
		// serveIndexHTML and answering every JWKS fetch with a 200
		// text/html body instead of the key set. Every RS256 verifier
		// reading this endpoint -- including the real hermes client's
		// PyJWKClient -- breaks in production. Scoped to this one path
		// deliberately; auditing the rest of RegisterCommonRoutes for the
		// same gap is a separate job, not swept in here.
		trimmed == "/.well-known/jwks.json"
}
