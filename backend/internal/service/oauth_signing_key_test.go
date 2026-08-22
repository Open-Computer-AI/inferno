package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
)

func TestActiveIsRSA(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	key, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if key.Private == nil || key.Private.N == nil {
		t.Fatal("expected an RSA private key")
	}
	if bits := key.Private.N.BitLen(); bits < 2048 {
		t.Fatalf("RSA key must be >= 2048 bits, got %d", bits)
	}
}

func TestJWKSEmitsRSAPublicKeyOnly(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	k := keys[0]
	for _, want := range []string{"kty", "n", "e", "kid", "use", "alg"} {
		if _, present := k[want]; !present {
			t.Errorf("JWKS entry missing %q", want)
		}
	}
	if k["kty"] != "RSA" || k["alg"] != "RS256" {
		t.Errorf("expected RSA/RS256, got %v/%v", k["kty"], k["alg"])
	}
	// The private half must never appear. For RSA that is more fields than ES256's single "d".
	for _, secret := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		if _, leaked := k[secret]; leaked {
			t.Fatalf("JWKS leaked private RSA parameter %q", secret)
		}
	}
}

// TestJWKSEncodesExponentWithoutLeadingZeros is the regression test for the
// usual 65537 public exponent: a big-endian encoding that still carries
// leading zero bytes is accepted by some JWT verifiers and rejected by
// others, which produces an intermittent, very hard to diagnose failure. The
// canonical encoding of 65537 is exactly 3 bytes (0x01 0x00 0x01), which
// base64url-encodes to "AQAB".
func TestJWKSEncodesExponentWithoutLeadingZeros(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	key, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if key.Private.E != 65537 {
		t.Fatalf("expected the standard public exponent 65537, got %d", key.Private.E)
	}

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	e, _ := keys[0]["e"].(string)
	if e != "AQAB" {
		t.Fatalf("expected e=%q for exponent 65537, got %q", "AQAB", e)
	}
}

// TestLegacyES256SecretNameMatchesMigration907 guards against
// legacyES256SecretName's value silently drifting from the literal
// migrations/907_oauth_rs256_key.sql deletes — in EITHER direction. It
// actually opens and reads that file (relative to this package,
// ../../migrations/907_oauth_rs256_key.sql) and asserts its DELETE
// statement contains the constant's current value, so this fails whether
// the Go constant changes without the SQL, or the SQL changes without the
// Go constant. Nothing else in this package references the constant (it
// exists purely as a documented, named source of truth for that migration's
// DELETE statement), so this is the only thing that keeps the two from
// silently diverging.
func TestLegacyES256SecretNameMatchesMigration907(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/907_oauth_rs256_key.sql")
	if err != nil {
		t.Fatalf("read migrations/907_oauth_rs256_key.sql: %v", err)
	}
	needle := fmt.Sprintf("key = '%s'", legacyES256SecretName)
	if !strings.Contains(string(sql), needle) {
		t.Fatalf("migrations/907_oauth_rs256_key.sql does not contain %q — it must delete the row named by legacyES256SecretName (%q); update whichever side drifted", needle, legacyES256SecretName)
	}
}

func TestByKidResolvesTheActiveKeyAndRejectsUnknown(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	active, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}

	got, err := svc.ByKid(ctx, active.Kid)
	if err != nil {
		t.Fatalf("ByKid(active): %v", err)
	}
	if got.Kid != active.Kid {
		t.Fatalf("ByKid returned %q, want %q", got.Kid, active.Kid)
	}

	if _, err := svc.ByKid(ctx, "not-a-real-kid"); !errors.Is(err, ErrUnknownKid) {
		t.Fatalf("expected ErrUnknownKid for an unknown kid, got %v", err)
	}
}

func TestActiveGeneratesAndPersistsKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthKeyService(client)

	first, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("first Active: %v", err)
	}
	if first.Kid == "" {
		t.Fatal("expected non-empty kid")
	}
	if _, ok := any(first.Private).(*rsa.PrivateKey); !ok {
		t.Fatal("expected an RSA private key")
	}

	second, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("second Active: %v", err)
	}
	if first.Kid != second.Kid {
		t.Fatalf("key not persisted: kid %q then %q", first.Kid, second.Kid)
	}
}

// TestActiveIsConcurrencySafe fires N goroutines at Active() with no key yet
// persisted, coordinated to genuinely overlap via a start channel. This is
// the regression test for the class of bug Task 1's EnsurePersonalOrg hit:
// two simultaneous first-calls both find no stored key, both generate a
// keypair, and both try to persist under the same unique
// security_secrets.key. A lost race must return the WINNING key, never an
// error and never a second key — a token signed with a discarded key would
// fail verification forever.
func TestActiveIsConcurrencySafe(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthKeyService(client)

	const n = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*SigningKey, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // released together so calls genuinely overlap
			key, err := svc.Active(ctx)
			results[i] = key
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: Active returned error: %v", i, err)
		}
	}

	firstKid := results[0].Kid
	for i, key := range results {
		if key == nil {
			t.Fatalf("call %d: returned nil key with no error", i)
		}
		if key.Kid != firstKid {
			t.Fatalf("not race-safe: call 0 returned kid %q, call %d returned kid %q — two keys were generated", firstKid, i, key.Kid)
		}
	}

	// Exactly one security_secrets row should exist for this key, not one
	// per goroutine.
	count, err := client.SecuritySecret.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count security_secrets: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 persisted signing key row, got %d", count)
	}
}

// TestSigningKeyCacheNeverServesARotatedKey is the safety proof for the parse
// cache added under Task 4's fix round.
//
// The cache is keyed by the stored PEM, not by kid, precisely so this holds
// without any rotation hook: replacing the security_secrets row -- by any
// means, since there is no rotation code in this tree to hook -- must make the
// old key unresolvable and the new one authoritative on the very next call, not
// after a TTL. A cache keyed by kid, or one that also cached the SELECT, would
// keep verifying tokens signed by the retired key.
func TestSigningKeyCacheNeverServesARotatedKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthKeyService(client)

	before, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	// Warm the cache the way every request does.
	if _, err := svc.ByKid(ctx, before.Kid); err != nil {
		t.Fatalf("ByKid before rotation: %v", err)
	}

	// Rotate: overwrite the row with a different key.
	replacement, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		t.Fatalf("generate replacement: %v", err)
	}
	replacementPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(replacement),
	})
	if _, err := client.SecuritySecret.Update().
		Where(securitysecret.Key(activeKeySecretName)).
		SetValue(string(replacementPEM)).
		Save(ctx); err != nil {
		t.Fatalf("rotate stored key: %v", err)
	}

	after, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("Active after rotation: %v", err)
	}
	if after.Kid == before.Kid {
		t.Fatal("rotation did not change the resolved key: the cache served a stale key")
	}
	if after.Private.N.Cmp(replacement.N) != 0 {
		t.Fatal("Active returned a key that is not the newly stored one")
	}
	// The retired kid must no longer resolve at all — this is the security
	// property. A kid-keyed cache would still answer here.
	if _, err := svc.ByKid(ctx, before.Kid); !errors.Is(err, ErrUnknownKid) {
		t.Fatalf("a rotated-out kid must be unknown, got %v", err)
	}
	if _, err := svc.ByKid(ctx, after.Kid); err != nil {
		t.Fatalf("ByKid for the new key: %v", err)
	}
}

// TestSigningKeyCacheReturnsTheSameParsedKeyForAnUnchangedRow pins that the
// cache is actually consulted. Without it every call re-parses and returns a
// distinct *rsa.PrivateKey, so pointer identity is the observable.
func TestSigningKeyCacheReturnsTheSameParsedKeyForAnUnchangedRow(t *testing.T) {
	ctx := context.Background()
	svc := NewOAuthKeyService(newPaymentConfigServiceTestClient(t))

	first, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("first Active: %v", err)
	}
	second, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("second Active: %v", err)
	}
	if first.Private != second.Private {
		t.Fatal("the stored key was re-parsed even though the row did not change")
	}
}

// BenchmarkSigningKeyParse and BenchmarkSigningKeyCached are the before/after
// for the parse cache. The first is what every OAuth-authenticated request used
// to pay in ByKid -> Active; the second is what it pays now.
func BenchmarkSigningKeyParse(b *testing.B) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		b.Fatal(err)
	}
	encoded := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := signingKeyFromPEM(encoded); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSigningKeyCached(b *testing.B) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		b.Fatal(err)
	}
	encoded := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}))
	svc := &OAuthKeyService{}
	if _, err := svc.signingKeyFor(encoded); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.signingKeyFor(encoded); err != nil {
			b.Fatal(err)
		}
	}
}
