package service

import (
	"errors"
	"strings"
)

// The OAuth scope vocabulary. These exact strings are the contract: the real
// hermes client hardcodes `inference:invoke`
// (hermes_cli/auth.py's NOUS_INFERENCE_INVOKE_SCOPE), and
// middleware.RequireOAuthScope matches whitespace-split tokens by EXACT
// equality — substring matching would be a privilege-escalation bug — so a
// near-miss spelling silently satisfies nothing.
const (
	ScopeInferenceInvoke = "inference:invoke"
	// ScopeToolInvoke is requested by the real client's billing step-up
	// fallback (hermes_cli/auth.py:8561) and is named in its own docstring at
	// :8540 as part of the standard "inference:invoke tool:invoke
	// billing:manage" set. Nothing in THIS branch gates on it — no route
	// demands it — but the client may legitimately ask for it, and the client
	// is the specification. Omitting it made the entire step-up fail with
	// invalid_scope on the branch where prior state carries no scope string.
	ScopeToolInvoke    = "tool:invoke"
	ScopeBillingRead   = "billing:read"
	ScopeBillingManage = "billing:manage"
	ScopeAgentsRead    = "agents:read"
	ScopeAgentsManage  = "agents:manage"
)

// knownScopes is the closed vocabulary a client may request.
//
// Before this existed, `scope` was pure pass-through: whatever a caller put on
// POST /api/oauth/device/code was stored verbatim, echoed into the minted
// access token, and read back by the resource-server middleware. Combined with
// an approval screen that showed the human nothing but a code box, that made
// the following work: `hermes-cli` is a well-known public client with no
// secret (seeded active by migration 905), so an attacker requests a device
// code for it with `scope=billing:manage agents:manage inference:invoke`,
// phishes the victim into pasting the user_code into their dashboard, and
// polls out an access token plus a 30-day refresh family carrying scopes the
// victim never saw.
//
// An allowlist is also what makes the design's own rule — that `billing:manage`
// is never granted at initial login and must be elevated to via a second device
// flow — expressible at all: you cannot enforce a policy over an open set.
var knownScopes = map[string]struct{}{
	ScopeInferenceInvoke: {},
	ScopeToolInvoke:      {},
	ScopeBillingRead:     {},
	ScopeBillingManage:   {},
	ScopeAgentsRead:      {},
	ScopeAgentsManage:    {},
}

// ErrInvalidScope is RFC 6749 §4.1.2.1 / RFC 8628 §3.2's `invalid_scope`: the
// request asked for a scope the authorization server does not recognise.
var ErrInvalidScope = errors.New("invalid_scope")

// ValidateScope accepts a whitespace-delimited scope request and rejects it if
// any entry is outside the vocabulary.
//
// An EMPTY scope is allowed: RFC 6749 §3.3 makes the parameter optional, and
// the server is entitled to apply a default. Rejecting it would break any
// client that omits the parameter, and an empty scope grants nothing — every
// RequireOAuthScope-guarded route demands a specific scope and an empty claim
// satisfies none of them, so "no scope" fails closed already.
//
// Validation is deliberately whole-request: one unknown entry rejects the
// whole request rather than being silently dropped. Silently narrowing a scope
// request produces a credential the client believes is more capable than it
// is, and the failure then surfaces much later as an opaque 403 from a
// completely different endpoint.
func ValidateScope(scope string) error {
	for _, s := range strings.Fields(scope) {
		if _, ok := knownScopes[s]; !ok {
			return ErrInvalidScope
		}
	}
	return nil
}

// ScopeList splits a stored scope string into its individual entries, for
// display on the approval screen. Returns a non-nil empty slice for an empty
// scope so JSON encodes it as [] rather than null.
func ScopeList(scope string) []string {
	out := strings.Fields(scope)
	if out == nil {
		return []string{}
	}
	return out
}
