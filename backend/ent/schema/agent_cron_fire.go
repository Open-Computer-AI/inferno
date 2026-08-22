package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AgentCronFire is ONE armed one-shot. Chronos is not a cron engine: the agent
// arms a single fire_at and re-arms after each fire, so there is no recurrence
// rule here by design.
//
// This table is the TRUTH. TimingWheelService is in-memory, so a restart drops
// every pending timer; rows are rehydrated into the wheel on boot. Nothing
// errors when that is broken -- the work simply never happens.
type AgentCronFire struct {
	ent.Schema
}

func (AgentCronFire) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agent_cron_fires"}}
}

func (AgentCronFire) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (AgentCronFire) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("agent_row_id"),
		field.String("job_id").MaxLen(200).NotEmpty(),
		field.Time("fire_at"),
		field.String("callback_url").MaxLen(500).NotEmpty(),
		// dedup_key is "{job_id}:{fire_at}" (plugins/cron_providers/chronos/
		// _nas_client.py:96-109). UNIQUE is what makes re-arming idempotent AT
		// THE DATABASE rather than by a read-then-write that races itself --
		// the agent's cold-start reconcile re-arms everything it wants.
		field.String("dedup_key").MaxLen(300).NotEmpty().Unique(),
		field.String("schedule_id").MaxLen(64).NotEmpty(),
		field.Enum("state").Values("armed", "fired", "cancelled").Default("armed"),
		field.Int("attempts").Default(0),
		field.String("last_error").MaxLen(500).Default(""),
	}
}

func (AgentCronFire) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_row_id"),
		index.Fields("state", "fire_at"),
	}
}
