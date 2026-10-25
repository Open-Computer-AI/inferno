package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Agent is one registered Hermes agent. Inferno owns IDENTITY only -- which
// user and org an agent belongs to, what to call it, and where its dashboard
// lives. VM lifecycle stays in oc-platform; oc_platform_user_id is the link
// between Inferno's int64 users and oc-platform's UUID users, which exists
// nowhere else.
type Agent struct {
	ent.Schema
}

func (Agent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agents"}}
}

func (Agent) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		// public_id is what GET /api/agents emits as `id`. The desktop
		// requires a STRING (apps/desktop/electron/main.ts:7922 drops any
		// agent whose id is not one), so this is never the int64 row id.
		field.String("public_id").MaxLen(64).NotEmpty().Unique(),
		field.Int64("user_id"),
		field.Int64("org_id"),
		field.String("name").MaxLen(200).NotEmpty(),
		field.String("dashboard_url").MaxLen(500).Default(""),
		field.String("oc_platform_user_id").MaxLen(64).Optional(),
		field.Time("last_seen_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (Agent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("org_id"),
	}
}
