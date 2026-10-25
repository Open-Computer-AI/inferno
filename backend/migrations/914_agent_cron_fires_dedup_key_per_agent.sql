-- 913: scope agent_cron_fires.dedup_key's uniqueness to ONE agent -- ruling
-- T4-1 (task-4-report.md fix round 1).
--
-- 912 created:
--
--   dedup_key VARCHAR(300) NOT NULL UNIQUE
--
-- a single-column, GLOBALLY unique constraint. That is wrong on two counts:
--
--   1. Security: dedup_key is "{job_id}:{fire_at}" (plugins/cron_providers/
--      chronos/_nas_client.py:96-109), and job_id lives in the CALLING
--      AGENT's own namespace, not a global one. A global unique constraint
--      let AgentCronService.Provision's `INSERT ... ON CONFLICT (dedup_key)`
--      target a DIFFERENT agent's row: agent 2 provisioning a dedup_key
--      agent 1 already held returned HTTP 200 carrying agent 1's job_id and
--      schedule_id, no row of its own, and touched agent 1's updated_at.
--   2. Correctness: agent A arming "daily-report:2026-09-01T10:00:00Z"
--      permanently blocked agent B from EVER arming its own identically
--      named job -- nothing in the chronos contract asks for a global
--      namespace; "idempotent re-arm" is inherently per-agent.
--
-- Postgres names an inline column-level UNIQUE constraint
-- "<table>_<column>_key" by default, so 912's constraint is
-- agent_cron_fires_dedup_key_key. Dropped and replaced with a composite
-- UNIQUE index on (agent_row_id, dedup_key) -- named to match ent's
-- generated index (agentcronfire_agent_row_id_dedup_key,
-- ent/schema/agent_cron_fire.go) so ent's own migrate.Schema stays in sync
-- with what actually ran. This closes the cross-agent collision
-- STRUCTURALLY: a foreign agent's row can no longer be a conflict target
-- for this agent's upsert at all, rather than relying only on a
-- post-upsert ownership check that a future refactor could drop.
--
-- 912 is not edited: applied migrations are checksum-verified (see
-- migrations/migrations.go), so the constraint is dropped and the new index
-- created instead.
ALTER TABLE agent_cron_fires DROP CONSTRAINT IF EXISTS agent_cron_fires_dedup_key_key;

CREATE UNIQUE INDEX IF NOT EXISTS agentcronfire_agent_row_id_dedup_key
    ON agent_cron_fires (agent_row_id, dedup_key);
