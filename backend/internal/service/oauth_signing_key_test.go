package service

import (
	"context"
	"crypto/ecdsa"
	"sync"
	"testing"
)

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
