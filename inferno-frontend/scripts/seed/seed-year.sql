-- Extends the local seed to a full year of DAILY aggregates so the donut's
-- four periods actually differ. The token mix drifts over the year (cache
-- adoption climbing from ~55% to ~90%) so Week / Month / Quarter / Year each
-- produce genuinely different slice shares -- otherwise the morph has nothing
-- to morph between.
BEGIN;

DELETE FROM usage_dashboard_daily WHERE bucket_date < current_date - interval '1 day';

INSERT INTO usage_dashboard_daily
  (bucket_date, total_requests, input_tokens, output_tokens,
   cache_creation_tokens, cache_read_tokens, total_cost, actual_cost,
   total_duration_ms, active_users, account_cost)
SELECT
  d::date,
  req,
  (req * 1850)::bigint,
  (req * 340)::bigint,
  (req * 900 * (1.9 - adopt))::bigint,          -- cache writes fall as the cache warms
  (req * 21000 * adopt)::bigint,                -- cache reads climb
  (req * 0.0261)::numeric(20,10),
  (req * 0.0223)::numeric(20,10),
  (req * 1420)::bigint,
  GREATEST(1, (req / 6)::int),
  (req * 0.0198)::numeric(20,10)
FROM (
  SELECT
    d,
    -- weekly rhythm plus slow growth
    GREATEST(40, (420
      * (0.55 + 0.45 * (EXTRACT(epoch FROM (d - (current_date - interval '364 days'))) / 86400.0 / 364.0))
      * (0.75 + 0.35 * sin(EXTRACT(dow FROM d) * 0.9))
    ))::int AS req,
    -- cache adoption 0.55 -> 0.92 across the year
    0.55 + 0.37 * (EXTRACT(epoch FROM (d - (current_date - interval '364 days'))) / 86400.0 / 364.0) AS adopt
  FROM generate_series(current_date - interval '364 days', current_date - interval '2 days', interval '1 day') AS d
) src;

COMMIT;
