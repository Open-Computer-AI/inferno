-- Local dev seed. Two days of hourly aggregates + the matching daily rows.
-- Deliberately leaves 13:00-15:00 empty today so the sparkline's zero-fill is
-- visible: raw rows would draw 12:00 and 16:00 adjacent and erase the dip.
BEGIN;

DELETE FROM usage_dashboard_hourly WHERE bucket_start >= current_date - interval '1 day';
DELETE FROM usage_dashboard_daily  WHERE bucket_date  >= current_date - interval '1 day';

WITH shape(h, w) AS (VALUES
  (0,4),(1,3),(2,2),(3,2),(4,3),(5,6),(6,12),(7,22),(8,38),(9,52),(10,61),(11,58),
  (12,44),(13,0),(14,0),(15,0),(16,49),(17,63),(18,55),(19,41),(20,28),(21,18),(22,11),(23,6)
),
gen AS (
  SELECT d.day, s.h,
         GREATEST(0, (s.w * d.mult)::int) AS req
  FROM (VALUES
          (current_date - interval '1 day', 1.00),
          (current_date,                     1.21)
       ) AS d(day, mult)
  CROSS JOIN shape s
  -- Today only exists up to the current hour.
  WHERE d.day < current_date OR s.h <= EXTRACT(hour FROM now())::int
)
INSERT INTO usage_dashboard_hourly
  (bucket_start, total_requests, input_tokens, output_tokens,
   cache_creation_tokens, cache_read_tokens, total_cost, actual_cost,
   total_duration_ms, active_users, account_cost)
SELECT
  gen.day + (gen.h || ' hours')::interval,
  gen.req,
  gen.req * 1850,
  gen.req * 340,
  gen.req * 900,
  gen.req * 21000,                      -- cache read dominates, as in real traffic
  (gen.req * 0.0261)::numeric(20,10),   -- standard list price
  (gen.req * 0.0223)::numeric(20,10),   -- actual, after the group discount
  gen.req * 1420,
  GREATEST(1, (gen.req / 6)::int),
  (gen.req * 0.0198)::numeric(20,10)
FROM gen
WHERE gen.req > 0;

INSERT INTO usage_dashboard_daily
  (bucket_date, total_requests, input_tokens, output_tokens,
   cache_creation_tokens, cache_read_tokens, total_cost, actual_cost,
   total_duration_ms, active_users, account_cost)
SELECT
  (bucket_start AT TIME ZONE current_setting('TimeZone'))::date,
  SUM(total_requests), SUM(input_tokens), SUM(output_tokens),
  SUM(cache_creation_tokens), SUM(cache_read_tokens),
  SUM(total_cost), SUM(actual_cost), SUM(total_duration_ms),
  MAX(active_users), SUM(account_cost)
FROM usage_dashboard_hourly
WHERE bucket_start >= current_date - interval '1 day'
GROUP BY 1;

COMMIT;
