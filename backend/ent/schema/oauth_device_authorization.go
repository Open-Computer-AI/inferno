package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthDeviceAuthorization is one in-flight RFC 8628 device-flow request.
type OAuthDeviceAuthorization struct {
	ent.Schema
}

func (OAuthDeviceAuthorization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_device_authorizations"},
	}
}

func (OAuthDeviceAuthorization) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OAuthDeviceAuthorization) Fields() []ent.Field {
	return []ent.Field{
		field.String("device_code").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("user_code").
			MaxLen(16).
			NotEmpty().
			Unique(),
		field.String("client_id").
			MaxLen(128).
			NotEmpty(),
		field.String("scope").
			MaxLen(255).
			Default(""),
		// pending | approved | denied | expired
		field.String("status").
			MaxLen(16).
			Default("pending"),
		field.Int64("approved_user_id").
			Optional().
			Nillable(),
		field.Time("expires_at"),
		field.Time("last_polled_at").
			Optional().
			Nillable(),
	}
}

func (OAuthDeviceAuthorization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
