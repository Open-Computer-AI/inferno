package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthClient is a registered OAuth client — one per gateway instance.
// These are PUBLIC clients: there is deliberately no client_secret column.
// PKCE is the protection.
type OAuthClient struct {
	ent.Schema
}

func (OAuthClient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_clients"},
	}
}

func (OAuthClient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		// Public client identifier. Server-generated only, "agent:{id}" for
		// SELF_HOSTED clients — callers never construct this value.
		field.String("client_id").
			MaxLen(128).
			NotEmpty().
			Unique(),
		// SELF_HOSTED | HOSTED
		field.String("kind").
			MaxLen(16).
			Default("SELF_HOSTED"),
		// Docker-style adjective_noun. NOT unique — the row id is the key;
		// a name collision is harmless.
		// MaxRuneLen, not MaxLen: the Postgres column is VARCHAR(64), which
		// counts CHARACTERS, while ent's MaxLen validator counts BYTES. The
		// mismatch is safe against the database but over-rejects — a 22-character
		// CJK name is 66 bytes and was refused, on a product whose primary
		// markets are CJK. MaxRuneLen sets the same Size(64) (so the generated
		// column is unchanged) and validates the unit the column actually uses.
		field.String("name").
			MaxRuneLen(64).
			NotEmpty(),
		field.Int64("owner_user_id"),
		field.Int64("org_id"),
		// Set by oc-platform once the VM exists. Nothing sets this in Task 3;
		// the column exists now so a later sub-project can make provisioning
		// idempotent on it without a migration.
		field.String("instance_id").
			MaxLen(128).
			Optional().
			Nillable().
			Unique(),
		// pending | active | revoked
		field.String("status").
			MaxLen(16).
			Default("pending"),
		field.String("redirect_uri_origin").
			MaxLen(255).
			NotEmpty(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
	}
}

func (OAuthClient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id"),
		index.Fields("owner_user_id"),
		index.Fields("status"),
	}
}
