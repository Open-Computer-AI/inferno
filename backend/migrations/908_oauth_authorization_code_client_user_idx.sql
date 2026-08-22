-- Index for the per-login supersession UPDATE in
-- OAuthAuthorizeService.IssueCode:
--
--   UPDATE oauth_authorization_codes SET status='consumed'
--    WHERE client_id = $1 AND user_id = $2 AND status = 'pending'
--
-- That statement runs on EVERY /oauth/authorize issuance, i.e. on the
-- primary login path. Migration 906 indexes only (status) and (expires_at),
-- so the planner's only choice was the status index -- whose selectivity
-- inverts over time, since after the first weeks almost every row is
-- 'consumed' rather than 'pending'. The table also has no reaper yet
-- (tracked separately as an operational task), so it grows without bound
-- and the query degrades monotonically.
--
-- PARTIAL on status='pending' deliberately: the predicate always carries
-- that status, the pending set is tiny and self-limiting (each login
-- consumes the previous code), and a partial index keeps the write cost on
-- the far larger consumed population at zero.
CREATE INDEX IF NOT EXISTS oauth_authorization_codes_client_user_pending_idx
    ON oauth_authorization_codes (client_id, user_id)
    WHERE status = 'pending';
