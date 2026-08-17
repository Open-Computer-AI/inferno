package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// fakeRefreshTokenCache is an in-memory RefreshTokenCache for tests in this
// package (which uses the SQLite-backed newPaymentConfigServiceTestClient
// and therefore has no live Redis). It mirrors the real Redis-backed
// implementation's family-set bookkeeping (internal/repository/refresh_token_cache.go)
// closely enough that DeleteTokenFamily has the same observable behavior:
// deleting every token hash ever added to that family, not just one.
type fakeRefreshTokenCache struct {
	mu       sync.Mutex
	tokens   map[string]*RefreshTokenData
	families map[string]map[string]struct{}
	users    map[int64]map[string]struct{}
}

func newFakeRefreshTokenCache() *fakeRefreshTokenCache {
	return &fakeRefreshTokenCache{
		tokens:   make(map[string]*RefreshTokenData),
		families: make(map[string]map[string]struct{}),
		users:    make(map[int64]map[string]struct{}),
	}
}

func (f *fakeRefreshTokenCache) StoreRefreshToken(_ context.Context, tokenHash string, data *RefreshTokenData, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := *data
	f.tokens[tokenHash] = &cloned
	return nil
}

func (f *fakeRefreshTokenCache) GetRefreshToken(_ context.Context, tokenHash string) (*RefreshTokenData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.tokens[tokenHash]
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}
	cloned := *data
	return &cloned, nil
}

func (f *fakeRefreshTokenCache) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tokens, tokenHash)
	return nil
}

func (f *fakeRefreshTokenCache) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash := range f.users[userID] {
		delete(f.tokens, hash)
	}
	delete(f.users, userID)
	return nil
}

func (f *fakeRefreshTokenCache) DeleteTokenFamily(_ context.Context, familyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash := range f.families[familyID] {
		delete(f.tokens, hash)
	}
	delete(f.families, familyID)
	return nil
}

func (f *fakeRefreshTokenCache) AddToUserTokenSet(_ context.Context, userID int64, tokenHash string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.users[userID] == nil {
		f.users[userID] = make(map[string]struct{})
	}
	f.users[userID][tokenHash] = struct{}{}
	return nil
}

func (f *fakeRefreshTokenCache) AddToFamilyTokenSet(_ context.Context, familyID, tokenHash string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.families[familyID] == nil {
		f.families[familyID] = make(map[string]struct{})
	}
	f.families[familyID][tokenHash] = struct{}{}
	return nil
}

func (f *fakeRefreshTokenCache) GetUserTokenHashes(_ context.Context, userID int64) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.users[userID]))
	for hash := range f.users[userID] {
		out = append(out, hash)
	}
	return out, nil
}

func (f *fakeRefreshTokenCache) GetFamilyTokenHashes(_ context.Context, familyID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.families[familyID]))
	for hash := range f.families[familyID] {
		out = append(out, hash)
	}
	return out, nil
}

func (f *fakeRefreshTokenCache) IsTokenInFamily(_ context.Context, familyID, tokenHash string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.families[familyID][tokenHash]
	return ok, nil
}

// MarkRotated mirrors the real Redis Lua script's atomicity: the whole
// get-decide-set happens while holding f.mu, so exactly one concurrent
// caller ever observes alreadyRotated=false for a given token. This is what
// makes TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins a
// meaningful test of OAuthTokenService.ExchangeRefreshToken's own locking
// (or lack of it) rather than of this fake's.
func (f *fakeRefreshTokenCache) MarkRotated(_ context.Context, tokenHash string, tombstoned *RefreshTokenData) (*RefreshTokenData, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.tokens[tokenHash]
	if !ok {
		return nil, false, ErrRefreshTokenNotFound
	}
	cloned := *data
	if data.Rotated {
		return &cloned, true, nil
	}
	tomb := *tombstoned
	f.tokens[tokenHash] = &tomb
	return &cloned, false, nil
}

// userLookupStub is a minimal OAuthUserLookup for tests: every ID not
// explicitly marked inactive/missing resolves to an active user, which is
// enough for tests that aren't specifically about account-status
// re-validation.
type userLookupStub struct {
	mu       sync.Mutex
	inactive map[int64]bool
	missing  map[int64]bool
}

func newUserLookupStub() *userLookupStub {
	return &userLookupStub{inactive: map[int64]bool{}, missing: map[int64]bool{}}
}

func (u *userLookupStub) GetByID(_ context.Context, id int64) (*User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.missing[id] {
		return nil, ErrUserNotFound
	}
	status := StatusActive
	if u.inactive[id] {
		status = "banned"
	}
	return &User{ID: id, Status: status}, nil
}

func (u *userLookupStub) setInactive(id int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.inactive[id] = true
}

func (u *userLookupStub) setMissing(id int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.missing[id] = true
}

func newDeviceFlowFixtureWithDeps(t *testing.T) (context.Context, *OAuthTokenService, *OAuthDeviceService, string, string, *fakeRefreshTokenCache, *userLookupStub) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	keys := NewOAuthKeyService(client)
	devices := NewOAuthDeviceService(client, "https://portal.example.com")
	cache := newFakeRefreshTokenCache()
	users := newUserLookupStub()
	tokens := NewOAuthTokenService(client, keys, devices, cache, users, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := devices.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	return ctx, tokens, devices, oc.ClientID, grant.DeviceCode, cache, users
}

func newDeviceFlowFixture(t *testing.T) (context.Context, *OAuthTokenService, *OAuthDeviceService, string, string) {
	t.Helper()
	ctx, tokens, devices, clientID, deviceCode, _, _ := newDeviceFlowFixtureWithDeps(t)
	return ctx, tokens, devices, clientID, deviceCode
}

func TestExchangeReturnsAuthorizationPendingBeforeApproval(t *testing.T) {
	ctx, tokens, _, clientID, deviceCode := newDeviceFlowFixture(t)

	_, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("expected ErrAuthorizationPending, got %v", err)
	}
}

func TestExchangeReturnsSlowDownOnFastRepoll(t *testing.T) {
	ctx, tokens, _, clientID, deviceCode := newDeviceFlowFixture(t)

	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("first poll: expected ErrAuthorizationPending, got %v", err)
	}
	// Immediate repoll, well inside the 5s interval.
	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); !errors.Is(err, ErrSlowDown) {
		t.Fatalf("second poll: expected ErrSlowDown, got %v", err)
	}
}

func TestExchangeReturnsSignedES256TokenAfterApproval(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}
	if got.Scope != "inference" {
		t.Fatalf("expected scope %q, got %q", "inference", got.Scope)
	}

	parsed, _, err := jwt.NewParser().ParseUnverified(got.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if parsed.Method.Alg() != "ES256" {
		t.Fatalf("expected ES256, got %s", parsed.Method.Alg())
	}
	if parsed.Header["kid"] == nil || parsed.Header["kid"] == "" {
		t.Fatal("access token header must carry a kid")
	}
}

func TestDeviceCodeIsSingleUse(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); err == nil {
		t.Fatal("a device code must not be redeemable twice")
	}
}

func TestExchangeRejectsMismatchedClient(t *testing.T) {
	ctx, tokens, devices, _, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if _, err := tokens.ExchangeDeviceCode(ctx, "agent:someone-else", deviceCode); err == nil {
		t.Fatal("a device code must only be redeemable by the client that requested it")
	}
}

// TestRefreshTokenReuseIsRejected proves rotation's entire point: replaying
// an already-rotated refresh token must not just fail for the replayer, it
// must kill the session for the LEGITIMATE client too. A single-token-delete
// implementation would pass a test that only checked the replay's own
// failure — assertion (b) below is the one that catches that bug.
func TestRefreshTokenReuseIsRejected(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	// Legitimate client rotates once.
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}
	if rotated.RefreshToken == original.RefreshToken {
		t.Fatal("rotation must issue a NEW refresh token, not return the same one")
	}

	// An attacker (or a retried request) replays the ORIGINAL, now-rotated
	// token.
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("(a) expected ErrRefreshTokenReused on replay, got %v", err)
	}

	// The token the LEGITIMATE client received from the rotation must also
	// be dead now — reuse detection revokes the whole family, not just the
	// replayed token. Checking the exact sentinel (not just "any error")
	// matters: an unrelated failure (e.g. a broken test double) must not be
	// able to masquerade as successful family revocation. A dead family
	// means the token's cache entry is gone entirely, so the correct
	// outcome is ErrInvalidGrant (not found), not ErrRefreshTokenReused.
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, rotated.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("(b) expected ErrInvalidGrant for the legitimately-rotated token after reuse revoked its family, got %v", err)
	}
}

func TestRefreshPreservesScope(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}
	if original.Scope != "inference" {
		t.Fatalf("expected initial scope %q, got %q", "inference", original.Scope)
	}

	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("ExchangeRefreshToken: %v", err)
	}
	if rotated.Scope != original.Scope {
		t.Fatalf("scope must be preserved across refresh: got %q, want %q", rotated.Scope, original.Scope)
	}

	parsed, _, err := jwt.NewParser().ParseUnverified(rotated.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse rotated access token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected MapClaims")
	}
	if claims["scope"] != "inference" {
		t.Fatalf("access token scope claim = %v, want %q", claims["scope"], "inference")
	}
}

func TestRefreshRejectsMismatchedClient(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	if _, err := tokens.ExchangeRefreshToken(ctx, "agent:someone-else", original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a refresh_token presented by the wrong client, got %v", err)
	}
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	ctx, tokens, _, clientID, _ := newDeviceFlowFixture(t)

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, "art_does-not-exist"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for an unknown refresh token, got %v", err)
	}
}

func TestRefreshTokenRawValueIsNeverStored(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	cache, ok := tokens.refreshCache.(*fakeRefreshTokenCache)
	if !ok {
		t.Fatal("expected fakeRefreshTokenCache")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for hash := range cache.tokens {
		if hash == got.RefreshToken {
			t.Fatal("the raw refresh token must never be used as its own storage key — only its SHA256 hash")
		}
	}
	if len(cache.tokens) != 1 {
		t.Fatalf("expected exactly one stored refresh token record, got %d", len(cache.tokens))
	}
}

// TestRefreshRejectsInactiveUser proves a banned/disabled user's already-
// issued refresh token stops minting new access tokens immediately, not
// just at its natural 30-day expiry. UserService.UpdateStatus invalidates an
// auth cache but never calls RevokeAllUserSessions, so this re-validation on
// every refresh is the only thing that closes that gap for OAuth sessions.
func TestRefreshRejectsInactiveUser(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, _, users := newDeviceFlowFixtureWithDeps(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	users.setInactive(42)

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a refresh token belonging to an inactive user, got %v", err)
	}
}

// TestRefreshRejectsMissingUser covers the deleted-user case separately from
// inactive: GetByID returns ErrUserNotFound, not an active-but-banned User.
func TestRefreshRejectsMissingUser(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, _, users := newDeviceFlowFixtureWithDeps(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	users.setMissing(42)

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant for a refresh token belonging to a deleted user, got %v", err)
	}
}

// TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins is C1's
// verification: N goroutines present the SAME refresh token at once. Without
// an atomic claim (RefreshTokenCache.MarkRotated), a read-then-write
// ExchangeRefreshToken lets multiple goroutines all observe "not yet
// rotated" before any of them writes, so multiple would succeed — forking
// the family into live, independently-rotating branches that never again
// trip the reuse detector. Exactly one must succeed; every other caller must
// see ErrRefreshTokenReused, and the family must have been revoked exactly
// once (not once per loser).
//
// Run with -race; see task-5-report.md for the before/after mutation
// evidence proving this test actually discriminates the fix from the race
// it replaces.
func TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, cache, _ := newDeviceFlowFixtureWithDeps(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	original, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode)
	if err != nil {
		t.Fatalf("ExchangeDeviceCode: %v", err)
	}

	// Captured BEFORE the race: once reuse detection fires, DeleteTokenFamily
	// removes every token in the family from the cache, so there is nothing
	// left to recover FamilyID from afterward.
	originalHash := hashToken(original.RefreshToken)
	cache.mu.Lock()
	familyID := cache.tokens[originalHash].FamilyID
	cache.mu.Unlock()
	if familyID == "" {
		t.Fatal("expected the original token's cache record to carry a family id")
	}

	const n = 20
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		successes   int
		losers      int
		otherErrors []error
	)
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			got, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && got != nil:
				successes++
			case errors.Is(err, ErrRefreshTokenReused):
				// This goroutine's own MarkRotated call was the one that
				// observed alreadyRotated=true.
				losers++
			case errors.Is(err, ErrInvalidGrant):
				// Also a legitimate loser outcome: by the time this
				// goroutine reached MarkRotated, an EARLIER loser's
				// DeleteTokenFamily had already removed the cache entry
				// entirely, so this call sees "not found" rather than
				// "already rotated". Which of the two a given loser sees is
				// a race between its own MarkRotated and another loser's
				// cleanup — both mean "you did not win", which is the
				// invariant this test actually cares about.
				losers++
			default:
				otherErrors = append(otherErrors, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(otherErrors) != 0 {
		t.Fatalf("unexpected errors (want only nil, ErrRefreshTokenReused, or ErrInvalidGrant): %v", otherErrors)
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 winner out of %d concurrent presentations of one refresh token, got %d", n, successes)
	}
	if losers != n-1 {
		t.Fatalf("expected %d losers, got %d", n-1, losers)
	}

	// A real DeleteTokenFamily call must actually have fired (revoked, not
	// merely reported as revoked) — otherwise a loser observing
	// ErrRefreshTokenReused without any real cleanup would still pass the
	// assertions above. Checking specifically for the ORIGINAL token's entry
	// is deliberate and race-free: unlike the winner's newly-minted token
	// (whose own AddToFamilyTokenSet call can interleave with a loser's
	// DeleteTokenFamily in either order — a narrower, separate ordering
	// hazard noted in task-5-report.md and out of scope for this test),
	// nothing ever re-adds the original hash once MarkRotated tombstones it,
	// so its presence/absence unambiguously reflects whether revocation ran.
	cache.mu.Lock()
	_, originalStillPresent := cache.tokens[originalHash]
	cache.mu.Unlock()
	if originalStillPresent {
		t.Fatal("expected DeleteTokenFamily to have actually removed the original token's cache entry")
	}
}
