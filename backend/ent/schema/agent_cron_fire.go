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
		// _nas_client.py:96-109). It is UNIQUE PER AGENT (see the composite
		// index below), never globally -- job_id lives in the calling
		// agent's OWN namespace, so two different agents legitimately arm
		// the identically-named job "daily-report" on the same day. A
		// field-level (globally) unique dedup_key was ruling T4-1's plan
		// defect (task-1-brief.md originally specified it): it let one
		// agent's arm collide into another agent's row (a field-level
		// .Unique() IS an index on that column alone, so ON CONFLICT could
		// only ever target across every agent at once) and it permanently
		// blocked a second agent from ever arming a same-named job. The
		// composite index is what makes re-arming idempotent AT THE
		// DATABASE, scoped to one agent, rather than by a read-then-write
		// that races itself -- the agent's cold-start reconcile re-arms
		// everything it wants.
		field.String("dedup_key").MaxLen(300).NotEmpty(),
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
		// Composite UNIQUE on (agent_row_id, dedup_key) -- ruling T4-1.
		// Scopes idempotent re-arming to ONE agent's own namespace instead
		// of a global one: a foreign agent's row can no longer be a
		// conflict target for this agent's INSERT ... ON CONFLICT at all,
		// closing the cross-agent collision structurally rather than by a
		// check a future refactor could drop.
		index.Fields("agent_row_id", "dedup_key").Unique(),
	}
}
