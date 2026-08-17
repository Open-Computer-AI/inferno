package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrgMember binds a user to an org with a role.
type OrgMember struct {
	ent.Schema
}

func (OrgMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "org_members"},
	}
}

func (OrgMember) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OrgMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("org_id"),
		field.Int64("user_id"),
		// OWNER | ADMIN | MEMBER. These exact strings are read by the desktop
		// (apps/desktop/electron/main.ts trimCloudOrg) — do not change casing.
		field.String("role").
			MaxLen(16).
			Default("MEMBER"),
	}
}

func (OrgMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("org_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
