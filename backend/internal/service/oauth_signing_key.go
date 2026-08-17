package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
)

// activeKeySecretName is the security_secrets row holding the PEM-encoded
// ES256 private key used to sign OAuth-issued tokens. Deliberately separate
// from Inferno's HMAC session-token secret: a symmetric key cannot be
// published as a JWKS, and agents must verify signatures offline.
const activeKeySecretName = "oauth_es256_active"

type SigningKey struct {
	Kid     string
	Private *ecdsa.PrivateKey
}

type OAuthKeyService struct {
	entClient *dbent.Client
}

func NewOAuthKeyService(entClient *dbent.Client) *OAuthKeyService {
	return &OAuthKeyService{entClient: entClient}
}

// kidFor derives a stable key id from the public key bytes, so the same key
// always produces the same kid without storing it separately.
func kidFor(pub *ecdsa.PublicKey) string {
	raw := elliptic.Marshal(pub.Curve, pub.X, pub.Y) //nolint:staticcheck // JWK thumbprint input
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

// signingKeyFromPEM parses a stored PEM-encoded EC private key row into a
// SigningKey. Shared by the read path and the post-race re-read path so both
// produce an identically-derived Kid.
func signingKeyFromPEM(value string) (*SigningKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, fmt.Errorf("oauth signing key: stored PEM is undecodable")
	}
	priv, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: parse: %w", err)
	}
	return &SigningKey{Kid: kidFor(&priv.PublicKey), Private: priv}, nil
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
		return signingKeyFromPEM(row.Value)
	}
	if !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("oauth signing key: query: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: generate: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("oauth signing key: marshal: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

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
			return signingKeyFromPEM(winner.Value)
		}
		return nil, fmt.Errorf("oauth signing key: persist: %w", err)
	}

	return &SigningKey{Kid: kidFor(&priv.PublicKey), Private: priv}, nil
}

func b64uint(i *big.Int, size int) string {
	b := i.Bytes()
	if len(b) < size {
		padded := make([]byte, size)
		copy(padded[size-len(b):], b)
		b = padded
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// JWKS projects the active key to its public JWK form. Never includes "d".
func (s *OAuthKeyService) JWKS(ctx context.Context) (map[string]any, error) {
	key, err := s.Active(ctx)
	if err != nil {
		return nil, err
	}
	pub := key.Private.PublicKey
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "EC",
			"crv": "P-256",
			"x":   b64uint(pub.X, 32), //nolint:staticcheck // JWK x/y encoding needs the raw coordinates; big.Int is fine for a public value
			"y":   b64uint(pub.Y, 32), //nolint:staticcheck // JWK x/y encoding needs the raw coordinates; big.Int is fine for a public value
			"kid": key.Kid,
			"use": "sig",
			"alg": "ES256",
		}},
	}, nil
}
