package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Org is a tenant. One personal org is auto-created per user at signup.
type Org struct {
	ent.Schema
}

func (Org) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "orgs"},
	}
}

func (Org) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Org) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("name").
			MaxLen(128).
			NotEmpty(),
		field.Bool("is_personal").
			Default(false),
	}
}
