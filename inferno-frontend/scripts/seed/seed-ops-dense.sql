-- The ops page defaults to "Last 1 hour", and the first pass stepped every 15
-- minutes -- so the throughput and error trends drew five spikes separated by
-- zeros instead of a curve. This fills the recent window at minute
-- granularity, which is what those charts bucket by at that range.
BEGIN;

DELETE FROM usage_logs     WHERE request_id LIKE 'seedmin-%';
DELETE FROM ops_error_logs WHERE request_id LIKE 'seedmin-%';

-- ~1 request per model per minute for 4 hours, with a gentle rhythm so the
-- line has shape rather than being flat.
INSERT INTO usage_logs
  (user_id, api_key_id, account_id, request_id, model,
   input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
   total_cost, actual_cost, stream, duration_ms, first_token_ms, created_at, rate_multiplier)
SELECT
  1, 1, 1,
  'seedmin-' || to_char(ts, 'YYYYMMDDHH24MI') || '-' || m.name || '-' || g,
  m.name,
  (m.w * 95)::int, (m.w * 18)::int, (m.w * 48)::int, (m.w * 1120)::int,
  (m.w * 0.0014)::numeric(20,10), (m.w * 0.0012)::numeric(20,10),
  true,
  GREATEST(120, (360 * power(2.5, (abs(hashtext(to_char(ts,'HH24MISS') || m.name || g)) % 100) / 32.0)))::int,
  GREATEST(55, (130 * power(2.0, (abs(hashtext(m.name || to_char(ts,'MISS') || g)) % 100) / 38.0)))::int,
  ts + (g * interval '7 seconds'),
  1
FROM generate_series(now() - interval '4 hours', now() - interval '20 seconds', interval '1 minute') AS ts
CROSS JOIN (VALUES ('claude-sonnet-4-5',52),('claude-opus-4-1',26),('claude-haiku-4-5',15),('gpt-5.1',7)) AS m(name, w)
CROSS JOIN generate_series(0, 2) AS g
-- Thin it with a deterministic rhythm so throughput rises and falls.
WHERE (abs(hashtext(to_char(ts,'HH24MI') || m.name)) % 100)
      < 45 + 40 * (0.5 + 0.5 * sin(EXTRACT(epoch FROM ts) / 900.0));

-- Errors every couple of minutes, weighted per class.
INSERT INTO ops_error_logs
  (request_id, user_id, api_key_id, account_id, platform, model,
   error_phase, error_type, severity, status_code, is_business_limited,
   error_message, duration_ms, created_at)
SELECT
  'seedmin-err-' || to_char(ts, 'YYYYMMDDHH24MI') || '-' || e.code,
  1, 1, 1, 'anthropic', 'claude-sonnet-4-5',
  e.phase, e.etype, e.sev, e.code, false, e.msg,
  (400 + (abs(hashtext(e.msg || to_char(ts,'HH24MI'))) % 2600))::int,
  ts
FROM generate_series(now() - interval '4 hours', now() - interval '1 minute', interval '2 minutes') AS ts
CROSS JOIN (VALUES
  ('upstream','upstream_error','error',   503, 'upstream temporarily unavailable'),
  ('upstream','upstream_error','error',   529, 'upstream overloaded'),
  ('request', 'client_error',  'warning', 429, 'rate limit exceeded'),
  ('request', 'client_error',  'warning', 400, 'invalid request payload'),
  ('internal','system_error',  'error',   500, 'internal gateway error')
) AS e(phase, etype, sev, code, msg)
WHERE (abs(hashtext(to_char(ts,'YYYYMMDDHH24MI') || e.code)) % 100) < e.code / 14;

COMMIT;
