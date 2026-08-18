package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
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
	replays  map[string]fakeReplayEntry
	families map[string]map[string]struct{}
	users    map[int64]map[string]struct{}
}

// fakeReplayEntry mirrors the refresh_replay:{hash} key the Redis
// implementation writes: the SEALED pair a rotation minted (opaque bytes —
// the fake, like the real cache, has no key to open it with), plus the
// moment Redis would expire it (service.RefreshReplayRetention after the
// rotation).
type fakeReplayEntry struct {
	sealed    []byte
	expiresAt time.Time
}

func newFakeRefreshTokenCache() *fakeRefreshTokenCache {
	return &fakeRefreshTokenCache{
		tokens:   make(map[string]*RefreshTokenData),
		replays:  make(map[string]fakeReplayEntry),
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
	delete(f.replays, tokenHash)
	return nil
}

func (f *fakeRefreshTokenCache) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash := range f.users[userID] {
		delete(f.tokens, hash)
		delete(f.replays, hash)
	}
	delete(f.users, userID)
	return nil
}

func (f *fakeRefreshTokenCache) DeleteTokenFamily(_ context.Context, familyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash := range f.families[familyID] {
		delete(f.tokens, hash)
		delete(f.replays, hash)
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

// MarkRotated mirrors the real Redis Lua script
// (repository.refreshTokenMarkRotatedScript) statement for statement: the
// whole read-decide-write happens while holding f.mu, so exactly one
// concurrent caller ever claims a given token, and the grace verdict is
// reached in the same critical section from the same injected clock. That is
// what makes TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins a
// meaningful test of OAuthTokenService.ExchangeRefreshToken's own
// serialization rather than of this fake's.
func (f *fakeRefreshTokenCache) MarkRotated(_ context.Context, tokenHash string, tombstoned *RefreshTokenData, sealedReplay []byte, now time.Time) (*RefreshRotationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.tokens[tokenHash]
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}
	cloned := *data
	if data.Rotated {
		graceSeconds := int64(RefreshReuseGracePeriod / time.Second)
		if data.RotatedAtUnix > 0 && now.Unix() >= data.RotatedAtUnix && now.Unix()-data.RotatedAtUnix <= graceSeconds {
			if entry, stored := f.replays[tokenHash]; stored && now.Before(entry.expiresAt) {
				return &RefreshRotationResult{Data: &cloned, Outcome: RefreshRotationGraceReplay, SealedReplay: entry.sealed}, nil
			}
		}
		return &RefreshRotationResult{Data: &cloned, Outcome: RefreshRotationReuse}, nil
	}
	tomb := *tombstoned
	f.tokens[tokenHash] = &tomb
	f.replays[tokenHash] = fakeReplayEntry{sealed: sealedReplay, expiresAt: now.Add(RefreshReplayRetention)}
	return &RefreshRotationResult{Data: &cloned, Outcome: RefreshRotationClaimed}, nil
}

// userLookupStub is a minimal OAuthUserLookup for tests: every ID not
// explicitly marked inactive/missing resolves to an active user, which is
// enough for tests that aren't specifically about account-status
// re-validation.
type userLookupStub struct {
	mu       sync.Mutex
	inactive map[int64]bool
	missing  map[int64]bool
	// passwordHash and email back the credential-invalidation tests:
	// resolvedTokenVersion fingerprints email+password_hash, so mutating either
	// is exactly what an in-app password or email change does to the value
	// stamped on a refresh token.
	passwordHash map[int64]string
	email        map[int64]string
}

func newUserLookupStub() *userLookupStub {
	return &userLookupStub{
		inactive:     map[int64]bool{},
		missing:      map[int64]bool{},
		passwordHash: map[int64]string{},
		email:        map[int64]string{},
	}
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
	hash, ok := u.passwordHash[id]
	if !ok {
		hash = "original-bcrypt-hash"
	}
	mail, ok := u.email[id]
	if !ok {
		mail = "user@example.com"
	}
	return &User{ID: id, Email: mail, PasswordHash: hash, Status: status}, nil
}

// setPasswordHash simulates UserService.ChangePassword: the new hash is
// written and nothing else happens — in particular RevokeAllUserSessions is
// NOT called, which is precisely the production behaviour the OAuth path has
// to defend itself against.
func (u *userLookupStub) setPasswordHash(id int64, hash string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.passwordHash[id] = hash
}

func (u *userLookupStub) setEmail(id int64, mail string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.email[id] = mail
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
	grant, err := devices.RequestCode(ctx, oc.ClientID, "inference:invoke")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	return ctx, tokens, devices, oc.ClientID, grant.DeviceCode, cache, users
}

// testClock is the codebase's way of crossing OAuthTokenService's
// refresh-reuse grace window without sleeping through a real minute: the
// service reads its clock through the injected OAuthTokenService.now, so a
// test can simply move time. Sleeping would make these tests take >60s each
// AND make them flaky under load, which is how a grace-window test quietly
// stops discriminating.
//
// Mutex-guarded because TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins
// reads it from N goroutines under -race.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// freezeClock puts the service on a stopped clock starting at the real
// current time, so unrelated real-time deadlines in the fixture (device code
// expiry, authorization code expiry) stay valid.
func freezeClock(tokens *OAuthTokenService) *testClock {
	clock := &testClock{t: time.Now()}
	tokens.now = clock.now
	return clock
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

func TestExchangeReturnsSignedRS256TokenAfterApproval(t *testing.T) {
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
	if got.Scope != "inference:invoke" {
		t.Fatalf("expected scope %q, got %q", "inference:invoke", got.Scope)
	}

	claims := jwt.MapClaims{}
	parsed, _, err := jwt.NewParser().ParseUnverified(got.AccessToken, claims)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if parsed.Method.Alg() != "RS256" {
		t.Fatalf("expected RS256, got %s", parsed.Method.Alg())
	}
	if parsed.Header["kid"] == nil || parsed.Header["kid"] == "" {
		t.Fatal("access token header must carry a kid")
	}

	// The two claims the gateway's own verifier reads
	// (plugins/dashboard_auth/nous/__init__.py in the read-only client repo).
	if v, _ := claims["oauth_contract_version"].(float64); v != 1 {
		t.Fatalf("expected oauth_contract_version claim 1, got %v", claims["oauth_contract_version"])
	}
	wantInstanceID, ok := strings.CutPrefix(clientID, "agent:")
	if !ok {
		t.Fatalf("test fixture clientID %q does not have the expected agent: prefix", clientID)
	}
	if got, _ := claims["agent_instance_id"].(string); got != wantInstanceID {
		t.Fatalf("expected agent_instance_id %q, got %q", wantInstanceID, got)
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
	clock := freezeClock(tokens)
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}
	if rotated.RefreshToken == original.RefreshToken {
		t.Fatal("rotation must issue a NEW refresh token, not return the same one")
	}

	// An attacker (or a retried request) replays the ORIGINAL, now-rotated
	// token — from OUTSIDE the reuse grace, which is what makes this the
	// reuse case rather than the forgiven one. A replay inside the grace is
	// a different, deliberately non-fatal outcome; see
	// TestRefreshReplayInsideGraceReturnsCurrentPair.
	//
	// 90 seconds is written as a literal, not as RefreshReuseGracePeriod+1:
	// a test whose clock is expressed in terms of the constant it is
	// checking moves with that constant, so widening the window would slide
	// the test along with it and the test would stop discriminating.
	clock.advance(90 * time.Second)
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

// grantedTokens runs the device flow to completion and returns the first
// token pair, so the refresh tests can start from a real issued credential.
func grantedTokens(t *testing.T, ctx context.Context, tokens *OAuthTokenService, devices *OAuthDeviceService, clientID, deviceCode string) *OAuthTokens {
	t.Helper()
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
	return got
}

// TestRefreshReplayInsideGraceReturnsCurrentPair is one half of the
// 60-second reuse grace.
//
// Instant revocation punishes benign clients: a desktop agent whose refresh
// response was lost in flight retries with the token it still holds, and two
// windows of one session refresh at the same moment. Both present an
// already-rotated token. Within RefreshReuseGracePeriod that must return the
// pair the rotation ALREADY minted — the same access token and the same
// refresh token, not a second pair (two live tokens in one family is the
// fork that permanently disables reuse detection) — and must leave the
// family alive.
//
// The assertions to keep: the returned pair is byte-identical to the
// rotation's, no additional token record was created, and the current token
// still works afterwards. An implementation that quietly minted a fresh pair
// would satisfy "the replay succeeded" but fails all three.
func TestRefreshReplayInsideGraceReturnsCurrentPair(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, cache, _ := newDeviceFlowFixtureWithDeps(t)
	original := grantedTokens(t, ctx, tokens, devices, clientID, deviceCode)

	clock := freezeClock(tokens)
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}

	cache.mu.Lock()
	recordsAfterRotation := len(cache.tokens)
	cache.mu.Unlock()

	clock.advance(30 * time.Second)

	replayed, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("replay 30s after rotation must be forgiven, got %v", err)
	}
	if replayed.RefreshToken != rotated.RefreshToken {
		t.Fatal("a replay inside the grace must return the CURRENT refresh token, not a newly minted one")
	}
	if replayed.AccessToken != rotated.AccessToken {
		t.Fatal("a replay inside the grace must return the CURRENT access token, not a newly minted one")
	}
	if replayed.Scope != rotated.Scope {
		t.Fatalf("scope must survive a grace replay unchanged: rotated %q, replayed %q", rotated.Scope, replayed.Scope)
	}
	// The access token was minted 30s ago, so what is left of it is 30s
	// less than a full TTL. Re-reporting the full TTL would leave the client
	// believing a dead token is still live.
	if want := rotated.ExpiresIn - 30; replayed.ExpiresIn != want {
		t.Fatalf("expected expires_in %d (the remaining lifetime of the already-minted access token), got %d", want, replayed.ExpiresIn)
	}

	// Nothing new was stored: minting on this path is exactly the family
	// fork the grace is designed NOT to create.
	cache.mu.Lock()
	recordsAfterReplay := len(cache.tokens)
	cache.mu.Unlock()
	if recordsAfterReplay != recordsAfterRotation {
		t.Fatalf("a grace replay must not create a token record: had %d before, %d after", recordsAfterRotation, recordsAfterReplay)
	}

	// The family is alive: the legitimate client's current token still
	// rotates. This is the assertion that fails if the grace path were to
	// revoke as a side effect.
	next, err := tokens.ExchangeRefreshToken(ctx, clientID, rotated.RefreshToken)
	if err != nil {
		t.Fatalf("the family must survive a forgiven replay, but the current token failed with %v", err)
	}
	if next.RefreshToken == rotated.RefreshToken {
		t.Fatal("rotation must issue a NEW refresh token")
	}
}

// TestRefreshReplayPairIsNeverStoredInPlaintext is the regression test for
// the one property the grace window threatened.
//
// Everything else in this system stores a refresh token as a SHA256 hash, so
// read access to Redis yields nothing usable. Answering a replay with the
// SAME pair means that pair has to be persisted — and persisting it in the
// clear would mean the newest tombstone in every family held that family's
// current live token, turning Redis read access into account takeover.
//
// So the stored blob is sealed under a key derived from the token being
// rotated. This test asserts all three halves of that: the blob contains
// neither token, the party holding the predecessor can open it, and nobody
// else can.
func TestRefreshReplayPairIsNeverStoredInPlaintext(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, cache, _ := newDeviceFlowFixtureWithDeps(t)
	original := grantedTokens(t, ctx, tokens, devices, clientID, deviceCode)

	clock := freezeClock(tokens)
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}

	cache.mu.Lock()
	entry, stored := cache.replays[hashToken(original.RefreshToken)]
	cache.mu.Unlock()
	if !stored {
		t.Fatal("expected the rotation to store a replay blob")
	}
	if len(entry.sealed) == 0 {
		t.Fatal("expected a non-empty sealed blob")
	}

	// (a) The blob leaks neither token. Checked as a raw byte search, not by
	// unmarshalling: a future implementation that stored the pair in some
	// other plaintext encoding would still be caught by this.
	blob := string(entry.sealed)
	if strings.Contains(blob, rotated.RefreshToken) {
		t.Fatal("the successor refresh token must never be stored in the clear")
	}
	if strings.Contains(blob, rotated.AccessToken) {
		t.Fatal("the successor access token must never be stored in the clear")
	}
	if strings.Contains(blob, original.RefreshToken) {
		t.Fatal("the presented refresh token must never be stored in the clear either")
	}

	// (b) The party holding the predecessor — the only party entitled to the
	// successor — can open it.
	opened, err := openRefreshReplay(original.RefreshToken, entry.sealed)
	if err != nil {
		t.Fatalf("the holder of the predecessor token must be able to open the sealed pair: %v", err)
	}
	if opened.RefreshToken != rotated.RefreshToken || opened.AccessToken != rotated.AccessToken {
		t.Fatal("the sealed pair must round-trip to exactly the pair the rotation minted")
	}

	// (c) Nobody else can. A Redis-only compromise yields this blob and a
	// hash, which is what it yielded before the grace existed.
	if _, err := openRefreshReplay(rotated.RefreshToken, entry.sealed); err == nil {
		t.Fatal("a different token must not open the sealed pair")
	}
	if _, err := openRefreshReplay(original.RefreshToken+"x", entry.sealed); err == nil {
		t.Fatal("a near-miss token must not open the sealed pair")
	}

	// And the end-to-end path still works: the grace replay unseals and
	// returns the same pair.
	clock.advance(30 * time.Second)
	replayed, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("grace replay: %v", err)
	}
	if replayed.RefreshToken != rotated.RefreshToken {
		t.Fatal("the unsealed replay must be the pair the rotation minted")
	}
}

// TestSealedRefreshReplayIsNonDeterministic guards the nonce: sealing the
// same pair under the same token twice must not produce the same bytes, or
// an observer of the store could tell that two rotations carried identical
// contents.
func TestSealedRefreshReplayIsNonDeterministic(t *testing.T) {
	pair := &RefreshReplayPair{
		AccessToken:         "access",
		RefreshToken:        "art_successor",
		Scope:               "inference:invoke",
		AccessExpiresAtUnix: 1700000000,
	}
	token := "art_" + strings.Repeat("a", 64)

	first, err := sealRefreshReplay(token, pair)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	second, err := sealRefreshReplay(token, pair)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same pair must differ — the nonce must be fresh each time")
	}

	// Both still open to the same plaintext.
	for _, sealed := range [][]byte{first, second} {
		opened, err := openRefreshReplay(token, sealed)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if opened.RefreshToken != pair.RefreshToken || opened.Scope != pair.Scope {
			t.Fatal("round trip changed the pair")
		}
	}

	// A truncated blob is rejected rather than panicking on the slice.
	if _, err := openRefreshReplay(token, first[:4]); err == nil {
		t.Fatal("a truncated blob must not open")
	}
	// A flipped bit in the ciphertext is caught by the GCM tag.
	tampered := append([]byte(nil), first...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := openRefreshReplay(token, tampered); err == nil {
		t.Fatal("a tampered blob must not open — the auth tag must be checked")
	}
}

// TestRefreshReplayOutsideGraceRevokesFamily is the other half: past
// RefreshReuseGracePeriod, a replay is indistinguishable from a stolen token
// and reuse detection must fire exactly as it did before the grace existed —
// revoking the whole family, not just the replayed token.
//
// Assertion (b) is the one that matters: an implementation that failed only
// the replayer would still pass (a).
func TestRefreshReplayOutsideGraceRevokesFamily(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)
	original := grantedTokens(t, ctx, tokens, devices, clientID, deviceCode)

	clock := freezeClock(tokens)
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}

	clock.advance(90 * time.Second)

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("(a) expected ErrRefreshTokenReused 90s after rotation, got %v", err)
	}
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, rotated.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("(b) expected the whole family to be revoked (ErrInvalidGrant for the current token), got %v", err)
	}
}

// TestRefreshGraceIsNotExtendedByReplay pins the narrowness of the window:
// the grace is anchored to the one rotation that opened it and is measured
// from that instant, so replaying inside it must not push the deadline out.
// A "reset the timer on every replay" implementation would let a stolen
// token be replayed forever, one minute at a time, and never trip reuse
// detection — which is the whole property rotation exists to provide.
func TestRefreshGraceIsNotExtendedByReplay(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)
	original := grantedTokens(t, ctx, tokens, devices, clientID, deviceCode)

	clock := freezeClock(tokens)
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}

	clock.advance(30 * time.Second)
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); err != nil {
		t.Fatalf("replay at t+30s must be forgiven, got %v", err)
	}

	// t+60s from the REPLAY, but t+90s from the rotation: outside.
	clock.advance(60 * time.Second)
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("the grace must be measured from the rotation, not restarted by a replay: got %v", err)
	}
}

// TestRefreshFailsAfterPasswordChange is the regression test for the
// credential-revocation gap between the panel and the OAuth path.
//
// UserService.ChangePassword writes the new password hash and returns; it does
// NOT call RevokeAllUserSessions. Panel sessions still die because
// AuthService.RefreshTokenPair compares the stored TokenVersion against
// resolvedTokenVersion(user) — a fingerprint of email+password_hash. The OAuth
// path used to have NEITHER half: issueRefreshToken never stamped TokenVersion
// (it stayed 0) and ExchangeRefreshToken never read it, so an exfiltrated
// ~/.hermes/auth.json kept self-rotating for its full 30 days after the victim
// changed their password — and there is no UI listing OAuth grants to tell
// them.
//
// The assertion is deliberately on the whole FAMILY, not just the presented
// token: killing one token would leave any concurrently-rotated sibling alive.
func TestRefreshFailsAfterPasswordChange(t *testing.T) {
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

	// Sanity: the credential works before the password changes. Without this,
	// a test that broke the fixture would still "pass" the assertion below for
	// entirely the wrong reason.
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("pre-change rotation should succeed: %v", err)
	}

	// The victim changes their password in the panel. Nothing else happens —
	// no explicit revocation call, exactly as in production.
	users.setPasswordHash(42, "new-bcrypt-hash-after-the-user-changed-it")

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, rotated.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant after password change, got %v — "+
			"a stolen agent credential would keep rotating for 30 more days", err)
	}

	// And the family is gone, not just the one token: re-presenting the
	// already-rotated original must now read as "unknown", proving the whole
	// rotation chain was revoked rather than a single entry invalidated.
	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected the whole family to be revoked, got %v", err)
	}
}

// TestRefreshFailsAfterEmailChange covers the other half of what
// resolvedTokenVersion fingerprints. It is a separate test rather than a
// subtest of the password case because the two are different revocation
// triggers that happen to share one mechanism — if that mechanism is ever
// narrowed to password_hash only, this is the test that notices.
func TestRefreshFailsAfterEmailChange(t *testing.T) {
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

	users.setEmail(42, "attacker-cannot-follow-this@example.com")

	if _, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant after email change, got %v", err)
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
	if original.Scope != "inference:invoke" {
		t.Fatalf("expected initial scope %q, got %q", "inference:invoke", original.Scope)
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
	if claims["scope"] != "inference:invoke" {
		t.Fatalf("access token scope claim = %v, want %q", claims["scope"], "inference:invoke")
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
// verification: N goroutines present the SAME refresh token at once.
//
// The failure it guards against is a FORK. Without an atomic claim
// (RefreshTokenCache.MarkRotated), a read-then-write ExchangeRefreshToken
// lets several goroutines all observe "not yet rotated" before any of them
// writes, so several mint — leaving the family with multiple live,
// independently-rotating branches that never again present an
// already-rotated token to each other, i.e. reuse detection silently dead
// for the rest of the family's 30 days.
//
// Since the 60-second reuse grace landed, the losers no longer see an error:
// simultaneous presentations are by definition inside the grace, so each
// loser is handed the pair the winner already minted. That is the intended
// outcome — it is the two-windows-at-once case the grace exists for — and it
// does NOT weaken this test, because the invariant being checked was never
// "the losers get an error". It is that exactly ONE token pair exists
// afterwards. Hence the assertions below: every caller that succeeded got
// the byte-identical refresh token, and exactly one new record was stored. A
// forking implementation fails both.
//
// Run with -race; see task-5-report.md for the mutation evidence.
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

	originalHash := hashToken(original.RefreshToken)
	cache.mu.Lock()
	familyID := cache.tokens[originalHash].FamilyID
	recordsBefore := len(cache.tokens)
	cache.mu.Unlock()
	if familyID == "" {
		t.Fatal("expected the original token's cache record to carry a family id")
	}

	const n = 20
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		issued      = map[string]int{}
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
				issued[got.RefreshToken]++
			default:
				otherErrors = append(otherErrors, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(otherErrors) != 0 {
		t.Fatalf("every concurrent presentation is inside the reuse grace and must succeed, got errors: %v", otherErrors)
	}
	// THE anti-fork assertion: one token, handed to everyone.
	if len(issued) != 1 {
		t.Fatalf("expected all %d concurrent presentations to receive the SAME refresh token (a fork would produce several), got %d distinct tokens", n, len(issued))
	}
	for token := range issued {
		if token == original.RefreshToken {
			t.Fatal("rotation must issue a NEW refresh token, not return the presented one")
		}
	}

	// Exactly one record was added — the single successor. A fork would have
	// stored one per winning goroutine, which is the state that kills reuse
	// detection for good.
	cache.mu.Lock()
	recordsAfter := len(cache.tokens)
	familySize := len(cache.families[familyID])
	cache.mu.Unlock()
	if recordsAfter != recordsBefore+1 {
		t.Fatalf("expected exactly one new token record after %d concurrent presentations, had %d before and %d after", n, recordsBefore, recordsAfter)
	}
	if familySize != 2 {
		t.Fatalf("expected the family to hold exactly 2 tokens (the rotated original and its single successor), got %d", familySize)
	}
	// The family was NOT revoked: a simultaneous double-presentation is the
	// benign case, and killing the session for it is the behavior the grace
	// replaced.
	cache.mu.Lock()
	_, originalStillPresent := cache.tokens[originalHash]
	cache.mu.Unlock()
	if !originalStillPresent {
		t.Fatal("a concurrent double-presentation must not revoke the family; the rotated original should still be tombstoned in cache")
	}
}

// TestConcurrentReuseRevocationAndRotationErrorClasses exists to replace
// coverage that the rewrite of
// TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins removed.
//
// Before the grace, that test's losers all took the reuse branch, so
// DeleteTokenFamily ran concurrently with another goroutine's rotation
// (StoreRefreshToken + AddToUserTokenSet + AddToFamilyTokenSet) on every
// run. Under the grace, simultaneous presentations are all forgiven, so
// nothing in that test revokes anything any more and that interleaving
// stopped being exercised at all — including under -race.
//
// This restores it: N goroutines replay a token from OUTSIDE the grace (so
// they revoke) while one goroutine legitimately rotates the family's current
// token (so it writes).
//
// It asserts ERROR CLASSES ONLY, deliberately. Whether the concurrently
// rotated successor survives the revocation depends on whether
// AddToFamilyTokenSet lands before or after DeleteTokenFamily reads the
// family set — a known, pre-existing ordering hazard that Task 5 did not fix
// and that this test must not pretend to have fixed. Asserting "nothing in
// the family survives" here would be asserting a property the code does not
// have, which is how a flaky test gets born.
func TestConcurrentReuseRevocationAndRotationErrorClasses(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode := newDeviceFlowFixture(t)
	original := grantedTokens(t, ctx, tokens, devices, clientID, deviceCode)

	clock := freezeClock(tokens)
	rotated, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
	if err != nil {
		t.Fatalf("legitimate rotation: %v", err)
	}
	clock.advance(90 * time.Second)

	const replayers = 8
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		reuseSeen   int
		otherErrors []error
	)
	start := make(chan struct{})

	wg.Add(replayers)
	for i := 0; i < replayers; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := tokens.ExchangeRefreshToken(ctx, clientID, original.RefreshToken)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrRefreshTokenReused):
				reuseSeen++
			case errors.Is(err, ErrInvalidGrant):
				// An earlier replayer's DeleteTokenFamily already removed the
				// tombstone, so this one never reaches the reuse branch.
			default:
				otherErrors = append(otherErrors, fmt.Errorf("replayer: %w", err))
			}
		}()
	}

	// The legitimate client, rotating at the same moment its family is being
	// torn down. Either outcome is correct; a third would not be.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := tokens.ExchangeRefreshToken(ctx, clientID, rotated.RefreshToken)
		mu.Lock()
		defer mu.Unlock()
		if err != nil && !errors.Is(err, ErrInvalidGrant) && !errors.Is(err, ErrRefreshTokenReused) {
			otherErrors = append(otherErrors, fmt.Errorf("rotator: %w", err))
		}
	}()

	close(start)
	wg.Wait()

	if len(otherErrors) != 0 {
		t.Fatalf("unexpected error classes during concurrent revocation: %v", otherErrors)
	}
	// The revocation path must genuinely have run — otherwise this test would
	// pass while exercising nothing, which is exactly the coverage it exists
	// to restore.
	if reuseSeen == 0 {
		t.Fatal("expected at least one replayer to take the reuse branch and revoke the family")
	}
}

// TestRevokedClientCannotRedeemOrRefresh proves oauth_client.status is a real
// kill switch on BOTH grants, not just at flow start.
//
// Checking status only at RequestCode would leave every already-issued refresh
// family rotating for up to 30 more days after a client is revoked — i.e. a
// switch that stops new logins and nothing that matters. Sub-project #1's
// reconcile job is being designed against this column, so it has to mean
// something before that lands.
func TestRevokedClientCannotRedeemOrRefresh(t *testing.T) {
	ctx := context.Background()
	entClient := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(entClient)
	keys := NewOAuthKeyService(entClient)
	devices := NewOAuthDeviceService(entClient, "https://portal.example.com")
	cache := newFakeRefreshTokenCache()
	tokens := NewOAuthTokenService(entClient, keys, devices, cache, newUserLookupStub(), "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	// Flow 1: approved, then the client is revoked BEFORE redemption.
	grant, err := devices.RequestCode(ctx, oc.ClientID, ScopeInferenceInvoke)
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if err := devices.Approve(ctx, grant.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Flow 2: fully redeemed while still usable, so we hold a live refresh
	// token whose family must also die on revocation.
	grant2, err := devices.RequestCode(ctx, oc.ClientID, ScopeInferenceInvoke)
	if err != nil {
		t.Fatalf("RequestCode (2): %v", err)
	}
	if err := devices.Approve(ctx, grant2.UserCode, 42); err != nil {
		t.Fatalf("Approve (2): %v", err)
	}
	live, err := tokens.ExchangeDeviceCode(ctx, oc.ClientID, deviceCodeFor(t, devices, ctx, grant2.UserCode))
	if err != nil {
		t.Fatalf("ExchangeDeviceCode (2): %v", err)
	}
	if _, err := tokens.ExchangeRefreshToken(ctx, oc.ClientID, live.RefreshToken); err != nil {
		t.Fatalf("refresh must work while the client is usable: %v", err)
	}

	if _, err := entClient.OAuthClient.UpdateOneID(oc.ID).SetStatus(ClientRevoked).Save(ctx); err != nil {
		t.Fatalf("revoke client: %v", err)
	}

	// device_code grant: an approved-but-unredeemed code is now worthless.
	if _, err := tokens.ExchangeDeviceCode(ctx, oc.ClientID, deviceCodeFor(t, devices, ctx, grant.UserCode)); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied redeeming for a revoked client, got %v", err)
	}

	// refresh_token grant: the already-issued family stops rotating.
	if _, err := tokens.ExchangeRefreshToken(ctx, oc.ClientID, live.RefreshToken); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant refreshing for a revoked client, got %v", err)
	}
}

// deviceCodeFor resolves a user_code back to its device_code, which the CLI
// would be holding.
func deviceCodeFor(t *testing.T, devices *OAuthDeviceService, ctx context.Context, userCode string) string {
	t.Helper()
	row, err := devices.byUserCodeForTest(ctx, userCode)
	if err != nil {
		t.Fatalf("byUserCodeForTest: %v", err)
	}
	return row.DeviceCode
}

// TestDeviceGrantRejectsBannedApprover: an approval is not a session. A user
// banned between clicking Approve in the browser and the CLI's next poll must
// not receive a 30-day credential.
func TestDeviceGrantRejectsBannedApprover(t *testing.T) {
	ctx, tokens, devices, clientID, deviceCode, _, users := newDeviceFlowFixtureWithDeps(t)

	row, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if err := devices.Approve(ctx, row.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	users.setInactive(42)

	if _, err := tokens.ExchangeDeviceCode(ctx, clientID, deviceCode); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for a banned approver, got %v", err)
	}

	// The approval must NOT have been burned: the user-status check runs
	// before the consuming update, so an unban leaves the code redeemable.
	reloaded, err := devices.byDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != DeviceStatusApproved {
		t.Fatalf("a rejected redemption must not consume the approval, status is %q", reloaded.Status)
	}
}
