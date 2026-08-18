-- 909: api_keys.oauth_client_id — the backing row for OAuth-bearer inference.
--
-- NULL for every ordinary key. Non-NULL means this row exists only so an
-- OAuth access token has something to meter against (see spec 2026-08-19,
-- finding F-C: usage_logs.api_key_id is NOT NULL with an FK, and the quota
-- ledger IS the key row). Because usage_logs_api_key_id_fkey is ON DELETE
-- CASCADE, a backing row must never be hard-deleted -- doing so silently
-- erases that agent's whole usage history.
--
-- The partial unique index is the identity rule: at most ONE backing row per
-- (user, client). It is partial so ordinary keys, which all carry NULL, are
-- unaffected -- a plain UNIQUE would collapse every user's keys to one.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS oauth_client_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_oauth_client_uniq
    ON api_keys (user_id, oauth_client_id)
    WHERE oauth_client_id IS NOT NULL;
