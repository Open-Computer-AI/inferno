-- 909: api_keys.oauth_client_id — the backing row for OAuth-bearer inference.
--
-- NULL for every ordinary key. Non-NULL means this row exists only so an
-- OAuth access token has something to meter against (see spec 2026-08-19,
-- finding F-C: usage_logs.api_key_id is NOT NULL with an FK, and the quota
-- ledger IS the key row). Because usage_logs_api_key_id_fkey is ON DELETE
-- CASCADE, a backing row must never be hard-deleted -- doing so silently
-- erases that agent's whole usage history.
--
-- The unique index is the identity rule: at most ONE backing row per
-- (user, client).
--
-- It is partial, but NOT because a plain UNIQUE would break ordinary keys --
-- that claim was in this plan and is empirically false, verified against real
-- Postgres 18 both as a bare CREATE UNIQUE INDEX and through the production
-- migration runner. Postgres treats NULL as distinct from NULL in a unique
-- index, so ordinary keys (all NULL here) never collide with each other
-- either way.
--
-- The WHERE clause is kept for the reason 904 and 908 keep theirs: the index
-- covers only the rows the constraint is about, so it stays proportional to
-- the number of agents rather than to the number of API keys in the system.
-- Correctness would survive dropping it; size and intent would not.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS oauth_client_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL;
