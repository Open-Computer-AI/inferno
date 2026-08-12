-- Per-model usage_logs so the stacked bar has columns. Model stats are
-- computed from the raw log table, not from the dashboard aggregates, so the
-- daily/hourly seed alone left the chart empty.
-- One row per model per day: enough for the aggregate query, tiny to insert.
BEGIN;

DELETE FROM usage_logs WHERE user_id = 1 AND request_id LIKE 'seed-%';

INSERT INTO usage_logs
  (user_id, api_key_id, account_id, request_id, model,
   input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
   total_cost, actual_cost, stream, duration_ms, created_at, rate_multiplier)
SELECT
  1, 1, 1,
  'seed-' || m.name || '-' || to_char(d, 'YYYYMMDD'),
  m.name,
  (m.weight * 1850 * daily.req / 100)::int,
  (m.weight * 340  * daily.req / 100)::int,
  (m.weight * 900  * daily.req / 100)::int,
  (m.weight * 21000 * daily.req * adopt / 100)::int,
  (m.weight * daily.req * 0.000261)::numeric(20,10),
  (m.weight * daily.req * 0.000223)::numeric(20,10),
  true,
  (1200 + m.weight * 6)::int,
  d + interval '12 hours',
  1
FROM generate_series(current_date - interval '364 days', current_date, interval '1 day') AS d
CROSS JOIN (VALUES
  ('claude-sonnet-4-5', 52),
  ('claude-opus-4-1',   26),
  ('claude-haiku-4-5',  15),
  ('gpt-5.1',            7)
) AS m(name, weight)
CROSS JOIN LATERAL (
  SELECT GREATEST(40, (420 * (0.55 + 0.45 * (EXTRACT(epoch FROM (d - (current_date - interval '364 days'))) / 86400.0 / 364.0)))::int) AS req
) daily
CROSS JOIN LATERAL (
  SELECT 0.18 + 0.78 * (EXTRACT(epoch FROM (d - (current_date - interval '364 days'))) / 86400.0 / 364.0) AS adopt
) a;

COMMIT;
