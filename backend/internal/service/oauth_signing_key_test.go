package service

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"sync"
	"testing"
)

// fixedShortYCoordinateKeyPEM is a TEST-ONLY, deliberately insecure P-256
// key fixture — NOT a real signing key, never used outside this test. Its
// private scalar is the small integer 43 (found by exhaustive deterministic
// search over d=1,2,3,... using crypto/elliptic.ScalarBaseMult, not
// crypto/rand, so this exact key is reproducible), chosen specifically
// because its Y coordinate's minimal big-endian encoding is 31 bytes, not
// 32: the high byte is genuinely zero. That makes it exercise the
// left-pad-to-32 path on every run, unlike a randomly generated per-test-run
// key, which only hits that path in about 1 run in 256.
//
//	D  = 43
//	X  = 986ae2506f1ff104d04230861d8f4b498f4bc4c6d009b30f7544dc129b82d28d (32 bytes, no padding needed)
//	Y  = 3cccc0a6460e0ae328a4d97d3c7b61d86fc6289c189f2525110c441bb07e97   (31 bytes — needs a leading 0x00)
const fixedShortYCoordinateKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAroAoGCCqGSM49
AwEHoUQDQgAEmGriUG8f8QTQQjCGHY9LSY9LxMbQCbMPdUTcEpuC0o0APMzApkYO
CuMopNl9PHth2G/GKJwYnyUlEQxEG7B+lw==
-----END EC PRIVATE KEY-----
`

// newTestEntClient is not defined in this package. The closest existing
// in-package helper with a matching signature — func(t *testing.T) *dbent.Client
// — is newPaymentConfigServiceTestClient (payment_config_service_test.go),
// established as the real helper to reuse for this in Task 1's
// org_service_test.go. Reused here for the same reason: use the real
// existing helper name rather than inventing a second one.

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
	if _, ok := any(first.Private).(*ecdsa.PrivateKey); !ok {
		t.Fatal("expected an ECDSA private key")
	}

	second, err := svc.Active(ctx)
	if err != nil {
		t.Fatalf("second Active: %v", err)
	}
	if first.Kid != second.Kid {
		t.Fatalf("key not persisted: kid %q then %q", first.Kid, second.Kid)
	}
}

func TestJWKSExposesPublicKeyOnly(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthKeyService(client)

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}

	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	k := keys[0]
	for _, want := range []string{"kty", "crv", "x", "y", "kid", "use", "alg"} {
		if _, present := k[want]; !present {
			t.Errorf("JWKS entry missing %q", want)
		}
	}
	if _, leaked := k["d"]; leaked {
		t.Fatal("JWKS leaked the private scalar 'd'")
	}
	if k["alg"] != "ES256" {
		t.Errorf("expected alg ES256, got %v", k["alg"])
	}
}

// TestJWKSPadsShortCoordinateTo32Bytes is the deterministic regression test
// for the fixed-width x/y encoding: P-256 JWK coordinates MUST be exactly 32
// bytes, and a coordinate whose big-endian value happens to have a zero
// leading byte would otherwise serialize one byte short. Uses
// fixedShortYCoordinateKeyPEM (a hardcoded fixture with a known 31-byte Y),
// so this exercises the padding path on every run instead of ~1 run in 256
// for a randomly generated key.
func TestJWKSPadsShortCoordinateTo32Bytes(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthKeyService(client)

	if _, err := client.SecuritySecret.Create().
		SetKey(activeKeySecretName).
		SetValue(fixedShortYCoordinateKeyPEM).
		Save(ctx); err != nil {
		t.Fatalf("seed fixture signing key: %v", err)
	}

	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	keys, ok := jwks["keys"].([]map[string]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected a non-empty keys array, got %#v", jwks["keys"])
	}
	k := keys[0]

	xEnc, _ := k["x"].(string)
	yEnc, _ := k["y"].(string)
	if xEnc == "" || yEnc == "" {
		t.Fatalf("expected non-empty x/y, got x=%q y=%q", xEnc, yEnc)
	}

	xDecoded, err := base64.RawURLEncoding.DecodeString(xEnc)
	if err != nil {
		t.Fatalf("decode x: %v", err)
	}
	if len(xDecoded) != 32 {
		t.Fatalf("x: expected exactly 32 bytes, got %d", len(xDecoded))
	}

	yDecoded, err := base64.RawURLEncoding.DecodeString(yEnc)
	if err != nil {
		t.Fatalf("decode y: %v", err)
	}
	if len(yDecoded) != 32 {
		t.Fatalf("y: expected exactly 32 bytes, got %d (this is the padding regression: a short coordinate must be left-padded with 0x00, not serialized as-is)", len(yDecoded))
	}
	// This fixture's Y is specifically 31 raw bytes, so a correct
	// implementation must have prepended exactly one 0x00 byte.
	if yDecoded[0] != 0x00 {
		t.Fatalf("y: expected a leading 0x00 pad byte for this fixture's known-short coordinate, got leading byte 0x%02x", yDecoded[0])
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
