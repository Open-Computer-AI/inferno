-- Paid orders, so the payment dashboard has something to draw.
--
-- WHY THIS EXISTS
-- `payment_enabled` is true on a fresh local stack but `payment_orders` is
-- empty, so /admin/payment-dashboard renders "No data" everywhere and
-- DailyRevenueChart cannot be verified at all. A chart with no series tells you
-- nothing about its axes, its legend, its stacking, or whether it survives a
-- day with zero orders in the middle of a run.
--
-- SHAPE
-- 45 days ending today, now()-relative like every other seed here. Volume
-- ramps and wobbles by weekday rather than being flat, so the trend line has
-- something to say. Two deliberate zero-order days are left in the middle: a
-- gap is the case a trend chart is most likely to get wrong (a line that
-- interpolates straight through a gap claims revenue that did not happen).
--
-- CURRENCY IS NOT A COLUMN
-- The backend derives it in PaymentOrderCurrency(): provider_snapshot->>currency
-- if present, else DefaultPaymentCurrency ("CNY"). So the chart's
-- multi-currency path -- the branch that decides how many strips render -- is
-- only reachable by writing a provider_snapshot. Three currencies are seeded
-- for exactly that reason; a single-currency seed would leave the interesting
-- branch untested.
--
-- STATUS IS UPPERCASE
-- payment_stats.go filters StatusIn("COMPLETED", "PAID", "RECHARGING") and on
-- paid_at, not created_at. Lowercase 'paid' silently matches nothing.
--
-- Idempotent: clears its own rows first, matched on the seed prefix.

DELETE FROM payment_orders WHERE out_trade_no LIKE 'seed-%';

INSERT INTO payment_orders (
  user_id, user_email, amount, pay_amount, fee_rate, recharge_code,
  payment_type, order_type, status, out_trade_no, provider_key,
  created_at, paid_at, expires_at, provider_snapshot
)
SELECT
  u.id,
  u.email,
  amt,
  amt,
  0,
  '',
  (ARRAY['alipay', 'wxpay', 'stripe'])[1 + (n % 3)],
  'recharge',
  'PAID',
  'seed-' || to_char(d, 'YYYYMMDD') || '-' || n,
  (ARRAY['alipay', 'wxpay', 'stripe'])[1 + (n % 3)],
  d + (n || ' minutes')::interval,
  d + (n || ' minutes')::interval,
  -- NOT NULL with no default. Already elapsed, which is correct for a paid order.
  d + (n || ' minutes')::interval + interval '30 minutes',
  jsonb_build_object(
    'schema_version', 1,
    'provider_key', (ARRAY['alipay', 'wxpay', 'stripe'])[1 + (n % 3)],
    'currency', (ARRAY['CNY', 'USD', 'EUR'])[1 + (n % 3)]
  )
FROM (
  SELECT (date_trunc('day', now()) - (offs || ' days')::interval) AS d, offs
  FROM generate_series(0, 44) AS offs
) days
CROSS JOIN LATERAL (
  SELECT id, email FROM users ORDER BY id LIMIT 1
) u
CROSS JOIN LATERAL generate_series(
  1,
  -- two dead days in the middle, then a weekday wobble on a rising base
  CASE
    WHEN days.offs IN (17, 18) THEN 0
    ELSE GREATEST(1, (14 - days.offs / 4) + (CASE WHEN EXTRACT(dow FROM days.d) IN (0, 6) THEN -4 ELSE 2 END))
  END
) AS n
CROSS JOIN LATERAL (
  SELECT ROUND((19 + ((days.offs * 7 + n * 13) % 9) * 10)::numeric, 2) AS amt
) money;

SELECT
  provider_snapshot->>'currency' AS currency,
  count(*) AS orders,
  count(DISTINCT paid_at::date) AS days_with_orders,
  min(paid_at)::date AS first_day,
  max(paid_at)::date AS last_day,
  sum(amount) AS total_amount
FROM payment_orders
WHERE out_trade_no LIKE 'seed-%'
GROUP BY 1
ORDER BY 1;
