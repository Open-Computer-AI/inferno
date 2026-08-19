package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
)

// activeKeySecretName is the security_secrets row holding the PEM-encoded
// RS256 private key used to sign OAuth-issued tokens. Deliberately separate
// from Inferno's HMAC session-token secret: a symmetric key cannot be
// published as a JWKS, and agents must verify signatures offline.
//
// RS256, not ES256: the gateway-brokered flow the Hermes Desktop client
// actually uses (plugins/dashboard_auth/nous/__init__.py:444 in the
// read-only client repo — the constrained consumer, so it decides) hard
// codes algorithms=["RS256"] and rejects anything else outright.
const activeKeySecretName = "oauth_rs256_active"

// legacyES256SecretName is the pre-RS256-migration security_secrets row
// name. It is not read or written by any code in this package any more —
// kept only as a named constant so migration 907's DELETE statement has a
// documented reason to reference "oauth_es256_active" instead of a bare
// string literal.
const legacyES256SecretName = "oauth_es256_active"

// rsaKeyBits is the RSA modulus size for newly generated OAuth signing
// keys. 2048 is the accepted floor for RS256 in 2026; anything smaller is a
// real cryptographic weakness, not a style choice.
const rsaKeyBits = 2048

// ErrUnknownKid is returned by ByKid when no signing key matches the
// requested kid — e.g. a token minted by a key that has since been rotated
// out, or a forged/garbage kid header.
var ErrUnknownKid = errors.New("oauth signing key: unknown kid")

type SigningKey struct {
	Kid     string
	Private *rsa.PrivateKey
}

// parsedSigningKey caches one PEM string's parse result, so a repeated read of
// the SAME stored key does not re-run pem.Decode + x509.ParsePKCS1PrivateKey.
//
// The cache key is the stored PEM itself, not the kid. That is the whole safety
// argument, and it is structural rather than event-driven: there is no rotation
// mechanism in this tree today for an invalidation hook to attach to, and a
// stale signing key is a security failure, not a performance one. Keying by the
// bytes means the database stays authoritative on every single request -- if
// the security_secrets row changes by any means at all (a future rotation task,
// an operator's UPDATE, a restore), the cached PEM no longer matches, the entry
// is missed, and the new key is parsed. A retired key can never be served from
// this cache, because nothing looks it up by name.
//
// One entry, not a map: security_secrets holds exactly one active OAuth signing
// key, so a map would only accumulate garbage across rotations.
type parsedSigningKey struct {
	pem string
	key *SigningKey
}

type OAuthKeyService struct {
	entClient *dbent.Client
	// parsed caches the last successfully parsed (pem, key) pair. See
	// parsedSigningKey for why the PEM is the cache key.
	parsed atomic.Pointer[parsedSigningKey]
}

func NewOAuthKeyService(entClient *dbent.Client) *OAuthKeyService {
	return &OAuthKeyService{entClient: entClient}
}

// signingKeyFor is signingKeyFromPEM with the parse memoised.
//
// # Why this exists
//
// ByKid -> Active is on the hot path of EVERY OAuth-authenticated inference
// request (middleware.OAuthOrAPIKeyAuth), and it had no cache of any kind:
// every request re-ran pem.Decode plus x509.ParsePKCS1PrivateKey on an RSA-2048
// PRIVATE key, which precomputes and validates the CRT parameters. Measured on
// this machine that parse is ~128 microseconds, against ~30 for the RS256
// verify the request actually needs -- so roughly four fifths of the crypto
// cost of a legitimate request was being spent re-deriving a key that had not
// changed, and an attacker spraying JWT-shaped garbage paid the server the same
// amplification for free.
//
// The SELECT is deliberately NOT cached. Caching it would open a staleness
// window in which a rotated-out key still verifies, and there is no rotation
// event in this tree to invalidate on. That half is left as a named residual;
// this half removes the dominant cost with no staleness window at all.
//
// The returned *SigningKey is shared by concurrent callers. rsa.PrivateKey is
// immutable after parsing and safe for concurrent signing and verification, and
// nothing in this package mutates a SigningKey after construction.
func (s *OAuthKeyService) signingKeyFor(value string) (*SigningKey, error) {
	if cached := s.parsed.Load(); cached != nil && cached.pem == value {
		return cached.key, nil
	}
	key, err := signingKeyFromPEM(value)
	if err != nil {
		return nil, err
	}
	s.parsed.Store(&parsedSigningKey{pem: value, key: key})
	return key, nil
}

// kidFor derives a stable key id from the public key's DER encoding, so the
// same key always produces the same kid without storing it separately.
func kidFor(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("oauth signing key: encode public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:16]), nil
}

// signingKeyFromPEM parses a stored PEM-encoded RSA private key row into a
// SigningKey. Shared by the read path and the post-race re-read path so both
// produce an identically-derived Kid.
func signingKeyFromPEM(value string) (*SigningKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, fmt.Errorf("oauth signing key: stored PEM is undecodable")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: parse: %w", err)
	}
	kid, err := kidFor(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	return &SigningKey{Kid: kid, Private: priv}, nil
}

// Active returns the current signing key, generating and persisting one on
// first use. Safe for concurrent callers: if two callers both observe no
// stored key and both generate one, the security_secrets.key UNIQUE
// constraint lets only one INSERT win. The loser does not error and does not
// keep its freshly generated (and now-discarded) key — it re-reads and
// returns the winner's persisted key, because a token signed with a
// discarded key would fail verification forever.
func (s *OAuthKeyService) Active(ctx context.Context) (*SigningKey, error) {
	row, err := s.entClient.SecuritySecret.Query().
		Where(securitysecret.Key(activeKeySecretName)).
		Only(ctx)
	if err == nil {
		return s.signingKeyFor(row.Value)
	}
	if !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("oauth signing key: query: %w", err)
	}

	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: generate: %w", err)
	}
	der := x509.MarshalPKCS1PrivateKey(priv)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})

	if _, err := s.entClient.SecuritySecret.Create().
		SetKey(activeKeySecretName).
		SetValue(string(encoded)).
		Save(ctx); err != nil {
		if dbent.IsConstraintError(err) {
			// Lost the race: another caller's INSERT for the same key
			// committed between our lookup and our insert. Re-read and
			// return the winner's key rather than erroring or minting a
			// second, unpersisted key.
			winner, getErr := s.entClient.SecuritySecret.Query().
				Where(securitysecret.Key(activeKeySecretName)).
				Only(ctx)
			if getErr != nil {
				return nil, fmt.Errorf("oauth signing key: re-read after race: %w", getErr)
			}
			return s.signingKeyFor(winner.Value)
		}
		return nil, fmt.Errorf("oauth signing key: persist: %w", err)
	}

	kid, err := kidFor(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	// Memoise under the exact PEM that was just persisted, so the next read of
	// this row hits the cache rather than re-parsing what we already hold.
	key := &SigningKey{Kid: kid, Private: priv}
	s.parsed.Store(&parsedSigningKey{pem: string(encoded), key: key})
	return key, nil
}

// ByKid resolves a signing key by its kid, for verification. Today it only
// ever has the one active key to resolve to, but it is deliberately the ONLY
// lookup RequireOAuthScope (and anything else that verifies a token) uses —
// never Active() directly — so a second key can be added later (rotation)
// by changing this one method, without touching any verifier.
func (s *OAuthKeyService) ByKid(ctx context.Context, kid string) (*SigningKey, error) {
	active, err := s.Active(ctx)
	if err != nil {
		return nil, err
	}
	if active.Kid != kid {
		return nil, ErrUnknownKid
	}
	return active, nil
}

// JWKS projects the active key to its public JWK form. Never includes any of
// the private RSA parameters (d, p, q, dp, dq, qi).
func (s *OAuthKeyService) JWKS(ctx context.Context) (map[string]any, error) {
	key, err := s.Active(ctx)
	if err != nil {
		return nil, err
	}
	pub := key.Private.PublicKey

	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigEndianExponent(pub.E)),
			"kid": key.Kid,
			"use": "sig",
			"alg": "RS256",
		}},
	}, nil
}

// bigEndianExponent encodes an RSA public exponent as a minimal big-endian
// byte slice, with no leading zero bytes. big.Int.Bytes() already returns
// the minimal (unpadded) big-endian encoding, which for the usual exponent
// 65537 is exactly 3 bytes (0x01 0x00 0x01) — base64url "AQAB". A padded
// encoding is accepted by some JWT verifiers and rejected by others, which
// produces an intermittent, hard-to-diagnose failure, so this must never
// pad.
func bigEndianExponent(e int) []byte {
	return big.NewInt(int64(e)).Bytes()
}
