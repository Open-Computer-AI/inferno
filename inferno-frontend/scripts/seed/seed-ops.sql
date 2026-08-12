-- Local ops seed. Three sources feed that page and none were populated:
--   usage_logs        -> the latency histogram (reads raw rows, not aggregates)
--   ops_error_logs    -> error distribution + error trend
--   ops_metrics_hourly-> throughput trend + the overview counters
BEGIN;

DELETE FROM usage_logs      WHERE request_id LIKE 'seedops-%';
DELETE FROM ops_error_logs  WHERE request_id LIKE 'seedops-%';
DELETE FROM ops_metrics_hourly WHERE bucket_start > now() - interval '8 days';

-- 1. Dense recent traffic, 48h at 15-minute steps, with a realistic latency
--    spread so the histogram actually has buckets rather than one spike.
INSERT INTO usage_logs
  (user_id, api_key_id, account_id, request_id, model,
   input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
   total_cost, actual_cost, stream, duration_ms, first_token_ms, created_at, rate_multiplier)
SELECT
  1, 1, 1,
  'seedops-' || to_char(ts, 'YYYYMMDDHH24MI') || '-' || m.name,
  m.name,
  (m.w * 90)::int, (m.w * 17)::int, (m.w * 45)::int, (m.w * 1050)::int,
  (m.w * 0.0013)::numeric(20,10), (m.w * 0.0011)::numeric(20,10),
  true,
  -- log-ish spread: most fast, a long tail
  GREATEST(120, (380 * power(2.4, (abs(hashtext(to_char(ts,'HH24MI') || m.name)) % 100) / 33.0)))::int,
  GREATEST(60, (140 * power(1.9, (abs(hashtext(m.name || to_char(ts,'MI'))) % 100) / 40.0)))::int,
  ts,
  1
FROM generate_series(now() - interval '47 hours', now() - interval '2 minutes', interval '15 minutes') AS ts
CROSS JOIN (VALUES ('claude-sonnet-4-5',52),('claude-opus-4-1',26),('claude-haiku-4-5',15),('gpt-5.1',7)) AS m(name, w);

-- 2. Errors: four classes so the distribution donut has real slices.
INSERT INTO ops_error_logs
  (request_id, user_id, api_key_id, account_id, platform, model,
   error_phase, error_type, severity, status_code, is_business_limited,
   error_message, duration_ms, created_at)
SELECT
  'seedops-err-' || to_char(ts, 'YYYYMMDDHH24MI') || '-' || e.code,
  1, 1, 1, 'anthropic', 'claude-sonnet-4-5',
  e.phase, e.etype, e.sev, e.code, false,
  e.msg,
  (400 + (abs(hashtext(e.msg || to_char(ts,'HH24MI'))) % 2600))::int,
  ts
FROM generate_series(now() - interval '47 hours', now() - interval '5 minutes', interval '35 minutes') AS ts
CROSS JOIN (VALUES
  ('upstream','upstream_error','error',   503, 'upstream temporarily unavailable'),
  ('upstream','upstream_error','error',   529, 'upstream overloaded'),
  ('request', 'client_error',  'warning', 429, 'rate limit exceeded'),
  ('request', 'client_error',  'warning', 400, 'invalid request payload'),
  ('internal','system_error',  'error',   500, 'internal gateway error')
) AS e(phase, etype, sev, code, msg)
-- Thin the tail out so the classes have clearly different weights.
WHERE (abs(hashtext(to_char(ts,'YYYYMMDDHH24MI') || e.code)) % 100) < e.code / 8;

-- 3. Hourly rollups for throughput + overview.
INSERT INTO ops_metrics_hourly
  (bucket_start, platform, group_id, success_count, error_count_total,
   business_limited_count, error_count_sla, upstream_error_count_excl_429_529,
   upstream_429_count, upstream_529_count, token_consumed,
   duration_p50_ms, duration_p90_ms, duration_p95_ms, duration_p99_ms,
   ttft_p50_ms, ttft_p90_ms, ttft_p95_ms, ttft_p99_ms,
   duration_avg_ms, duration_max_ms, ttft_avg_ms, ttft_max_ms, ttft_sample_count)
SELECT
  h, 'anthropic', NULL,
  base, (base * 0.045)::bigint,
  (base * 0.006)::bigint, (base * 0.030)::bigint, (base * 0.018)::bigint,
  (base * 0.012)::bigint, (base * 0.008)::bigint, (base * 5200)::bigint,
  420, 980, 1450, 3100,
  180, 430,  620, 1400,
  610.0, 5200, 240.0, 2600, base
FROM (
  SELECT h,
    GREATEST(20, (150 * (0.55 + 0.45 * sin(EXTRACT(hour FROM h) * 0.26 - 1.6))))::bigint AS base
  FROM generate_series(date_trunc('hour', now() - interval '7 days'), date_trunc('hour', now()), interval '1 hour') AS h
) src;

COMMIT;
