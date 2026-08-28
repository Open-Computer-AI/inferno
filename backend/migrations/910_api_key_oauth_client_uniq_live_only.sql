-- 910: scope the OAuth backing-row identity index to LIVE rows only.
--
-- 909 created:
--
--   CREATE UNIQUE INDEX api_keys_user_oauth_client_uniq
--       ON api_keys (user_id, oauth_client_id)
--       WHERE oauth_client_id IS NOT NULL;
--
-- That index does not filter deleted_at, but every read of api_keys does:
-- ent/schema/api_key.go mixes in mixins.SoftDeleteMixin, whose interceptor
-- unconditionally appends "deleted_at IS NULL" to every query. The two
-- disagree, and the disagreement is a trap:
--
--   1. a backing row is soft-deleted (tombstoned, deleted_at set);
--   2. OAuthBackingKeyService.Resolve looks it up and cannot see it;
--   3. Resolve inserts a replacement, and THIS index refuses it, because the
--      tombstone still occupies the (user_id, oauth_client_id) slot;
--   4. the recovery re-read cannot see the tombstone either.
--
-- That agent can then never resolve again -- no inference, no recovery short
-- of manual SQL. And the state is user-reachable, not merely operator-
-- reachable: APIKeyService.Delete authorizes on ownership alone and writes a
-- soft-delete tombstone, so a user who learns their backing row's id can
-- permanently brick their own agent. (That path is now refused in
-- APIKeyService.Delete as well -- this migration is the other half: the index
-- fix makes the state RECOVERABLE, the service guard stops it being reached.
-- Both are needed, because the index also has to cope with tombstones written
-- before the guard existed, and with admin/cascade paths that legitimately
-- tombstone a user's keys.)
--
-- Adding "AND deleted_at IS NULL" means a tombstone releases the slot: the
-- next Resolve inserts a fresh backing row and the agent self-heals. The
-- usage history that hung off the tombstoned row is untouched -- nothing is
-- hard-deleted here, and usage_logs_api_key_id_fkey (ON DELETE CASCADE) never
-- fires.
--
-- 909 is not edited: applied migrations are checksum-verified (see
-- migrations/migrations.go), so the index is dropped and recreated instead.
DROP INDEX IF EXISTS api_keys_user_oauth_client_uniq;

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL AND deleted_at IS NULL;
