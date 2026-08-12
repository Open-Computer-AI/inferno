-- Ten extra users with usage, so the stacked trend actually has bands to draw.
-- The chart caps at 8 (the categorical ramp's length): 7 named + "Other".
-- Eleven users total therefore exercises the fold, which one user cannot.
BEGIN;

INSERT INTO users (email, password_hash, role, username, status)
SELECT
  'user' || g || '@inferno.local',
  '$2a$10$seedseedseedseedseedseedseedseedseedseedseedseedseedse',
  'user',
  (ARRAY['ada','grace','alan','edsger','barbara','donald','ken','dennis','linus','margaret'])[g],
  'active'
FROM generate_series(1, 10) AS g
WHERE NOT EXISTS (SELECT 1 FROM users x WHERE x.email = 'user' || g || '@inferno.local');

-- One API key per new user so usage_logs has a valid FK target.
INSERT INTO api_keys (user_id, name, key, status)
SELECT u.id, u.username || '-key', 'sk-seed-' || md5(u.email), 'active'
FROM users u
WHERE u.email LIKE 'user%@inferno.local'
  AND NOT EXISTS (SELECT 1 FROM api_keys k WHERE k.user_id = u.id);

-- 30 days of daily usage, each user on a different scale so the ranking is
-- unambiguous and the tail is genuinely smaller than the head.
INSERT INTO usage_logs
  (user_id, api_key_id, account_id, request_id, model,
   input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
   total_cost, actual_cost, stream, duration_ms, first_token_ms, created_at, rate_multiplier)
SELECT
  u.id,
  k.id,
  1,
  'seeduser-' || u.id || '-' || to_char(d, 'YYYYMMDD') || '-' || s,
  (ARRAY['claude-sonnet-4-5','claude-opus-4-1','claude-haiku-4-5','gpt-5.1'])[1 + (u.id % 4)],
  (w * 900)::int, (w * 160)::int, (w * 420)::int, (w * 5200)::int,
  (w * 0.011)::numeric(20,10), (w * 0.009)::numeric(20,10),
  true, 800, 200,
  d + (s * interval '37 minutes'),
  1
FROM users u
JOIN api_keys k ON k.user_id = u.id
CROSS JOIN generate_series(now() - interval '29 days', now() - interval '1 hour', interval '1 day') AS d
CROSS JOIN generate_series(1, 2) AS s
CROSS JOIN LATERAL (SELECT (12 - (u.id - 1)) * (1 + (extract(day from d)::int % 3)) AS w) AS weight
WHERE u.email LIKE 'user%@inferno.local';

COMMIT;
