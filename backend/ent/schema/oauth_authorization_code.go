package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthAuthorizationCode is one in-flight RFC 6749 §4.1 authorization_code
// grant, PKCE-bound (RFC 7636). It is a one-time bearer credential: a code
// converts into a 30-day refresh-token family exactly once. See
// OAuthAuthorizeService.IssueCode and OAuthTokenService.ExchangeAuthorizationCode
// for the issue/redeem lifecycle and the replay-revocation invariant.
type OAuthAuthorizationCode struct {
	ent.Schema
}

func (OAuthAuthorizationCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_authorization_codes"},
	}
}

func (OAuthAuthorizationCode) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OAuthAuthorizationCode) Fields() []ent.Field {
	return []ent.Field{
		// 32 bytes of crypto/rand, hex-encoded (64 chars) -- see
		// newAuthorizationCode in oauth_authorize_service.go. MaxLen(128)
		// mirrors oauth_device_authorizations.device_code, the closest
		// analogue: a bearer credential of the same entropy shape.
		field.String("code").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("client_id").
			MaxLen(128).
			NotEmpty(),
		field.Int64("user_id"),
		// The FULL redirect_uri presented at /oauth/authorize (Task 4), not
		// an origin -- ValidateRedirectURI's callback-suffix path plus a
		// registered origin comfortably fits within 512.
		field.String("redirect_uri").
			MaxLen(512).
			NotEmpty(),
		field.String("scope").
			MaxLen(255).
			Default(""),
		// BASE64URL-NOPAD(SHA256(code_verifier)) per RFC 7636 -- 43
		// characters for a SHA256 digest; 128 leaves headroom without
		// inviting a padded/oversized value to silently truncate.
		field.String("code_challenge").
			MaxLen(128).
			NotEmpty(),
		// S256 only -- see oauth_authorize_service.go's IssueCode, which
		// rejects "plain" (RFC 7636 permits it; it provides no protection)
		// before a row is ever created. Stored anyway, not hardcoded, so a
		// redemption-time defensive check has something real to compare
		// against rather than trusting the invariant blindly.
		field.String("code_challenge_method").
			MaxLen(8).
			NotEmpty(),
		// pending | consumed
		field.String("status").
			MaxLen(16).
			Default("pending"),
		// Set at redemption, once tokens have actually been minted from this
		// code -- NOT at issue time. A replayed code (second presentation
		// after the row is already "consumed") uses this to revoke the
		// refresh-token family the FIRST presentation minted, per RFC 6749
		// §4.1.2: a replay means one of the two arrivals is an attacker, and
		// the safe move is to kill what was already issued. Nil when the
		// code was consumed by a presentation that itself failed validation
		// (wrong PKCE verifier, mismatched client/redirect_uri, expired) --
		// nothing was ever issued from it, so a later replay has nothing to
		// revoke.
		field.String("issued_token_family").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Time("expires_at"),
	}
}

func (OAuthAuthorizationCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
