-- Alert events seed for the ops dashboard.
--
-- The table ships with two rows, both P1/P2 and both resolved, both with a
-- NULL dimensions blob. That renders exactly one badge tone, one status tone
-- and a dash in the dimensions column, so most of the card cannot be seen at
-- all. This seeds the spread the component actually has to draw:
--
--   severity   P0 / P1 / P2 / P3      (the full ordinal ramp)
--   status     firing / resolved / manual_resolved
--   email      sent and ignored
--   dimensions platform+group+region, platform only, and none
--
-- Times run over the last 7 days so the 24h and 7d range filters both have
-- something to show, and the firing rows are recent so their live duration
-- counts up from minutes rather than days.

DELETE FROM ops_alert_events;
ALTER SEQUENCE ops_alert_events_id_seq RESTART WITH 1;

INSERT INTO ops_alert_events
  (rule_id, severity, status, title, description, metric_value, threshold_value,
   dimensions, fired_at, resolved_at, email_sent, created_at)
VALUES
  -- Firing right now, worst first ---------------------------------------
  (2, 'P0', 'firing', 'Success rate below 90%',
   'Success rate fell to 81.4% over the last 5 minutes, below the 90% floor. Upstream returned 529 on 14 of 75 requests.',
   81.4, 90,
   '{"platform":"anthropic","group_id":1,"region":"us-east"}',
   now() - interval '34 minutes', NULL, true, now() - interval '34 minutes'),

  (8, 'P0', 'firing', 'Error rate above 25%',
   'Error rate reached 31.2% on openai. Sustained for 3 consecutive evaluation windows.',
   31.2, 25,
   '{"platform":"openai","group_id":2}',
   now() - interval '3 hours 12 minutes', NULL, true, now() - interval '3 hours 12 minutes'),

  (7, 'P1', 'firing', 'Concurrency queue backing up',
   'Queue depth held at 46 waiting requests for over 10 minutes.',
   46, 20,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '1 hour 5 minutes', NULL, true, now() - interval '1 hour 5 minutes'),

  (1, 'P1', 'firing', 'Error rate above 10%',
   'Error rate at 13.8% on gemini.',
   13.8, 10,
   '{"platform":"gemini","group_id":3,"region":"eu-west"}',
   now() - interval '18 minutes', NULL, false, now() - interval '18 minutes'),

  (3, 'P2', 'firing', 'P99 latency above 8s',
   'P99 request duration at 11.4s.',
   11400, 8000,
   '{"platform":"openai"}',
   now() - interval '52 minutes', NULL, false, now() - interval '52 minutes'),

  -- Resolved in the last 24 hours ---------------------------------------
  (1, 'P1', 'resolved', 'Error rate above 10%',
   'Error rate peaked at 17.1% on anthropic, recovered without intervention.',
   17.1, 10,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '5 hours', now() - interval '4 hours 22 minutes', true, now() - interval '5 hours'),

  (6, 'P1', 'resolved', 'Memory usage above 85%',
   'Resident memory reached 91.3% of the container limit.',
   91.3, 85,
   NULL,
   now() - interval '7 hours', now() - interval '6 hours 41 minutes', true, now() - interval '7 hours'),

  (5, 'P2', 'resolved', 'CPU usage above 80%',
   'CPU held at 86.7% for 6 minutes during the evening peak.',
   86.7, 80,
   NULL,
   now() - interval '9 hours', now() - interval '8 hours 48 minutes', false, now() - interval '9 hours'),

  (4, 'P2', 'manual_resolved', 'P95 latency above 5s',
   'P95 at 6.9s on gemini. Closed by an operator after the upstream incident was confirmed.',
   6900, 5000,
   '{"platform":"gemini","group_id":3}',
   now() - interval '11 hours', now() - interval '9 hours 30 minutes', false, now() - interval '11 hours'),

  (2, 'P0', 'resolved', 'Success rate below 90%',
   'Success rate dipped to 88.2% for two windows during an upstream rollout.',
   88.2, 90,
   '{"platform":"anthropic","group_id":1,"region":"us-east"}',
   now() - interval '14 hours', now() - interval '13 hours 51 minutes', true, now() - interval '14 hours'),

  (4, 'P3', 'resolved', 'P95 latency above 3s',
   'Informational: P95 crossed the advisory threshold for one window.',
   3400, 3000,
   '{"platform":"openai","group_id":2}',
   now() - interval '16 hours', now() - interval '15 hours 55 minutes', false, now() - interval '16 hours'),

  (3, 'P2', 'resolved', 'P99 latency above 8s',
   'P99 at 9.2s on anthropic.',
   9200, 8000,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '20 hours', now() - interval '19 hours 12 minutes', false, now() - interval '20 hours'),

  -- Days 2 to 7 ---------------------------------------------------------
  (1, 'P1', 'resolved', 'Error rate above 10%',
   'Error rate at 12.4% on openai.',
   12.4, 10,
   '{"platform":"openai","group_id":2}',
   now() - interval '1 day 4 hours', now() - interval '1 day 3 hours 20 minutes', true, now() - interval '1 day 4 hours'),

  (1, 'P1', 'manual_resolved', 'Error rate above 10%',
   'Error rate at 11.1% on openai. Closed by an operator; the account was rotated.',
   11.1, 10,
   '{"platform":"openai","group_id":2}',
   now() - interval '1 day 9 hours', now() - interval '1 day 6 hours', true, now() - interval '1 day 9 hours'),

  (6, 'P1', 'resolved', 'Memory usage above 85%',
   'Resident memory reached 88.9%.',
   88.9, 85,
   NULL,
   now() - interval '1 day 18 hours', now() - interval '1 day 17 hours 44 minutes', false, now() - interval '1 day 18 hours'),

  (8, 'P0', 'resolved', 'Error rate above 25%',
   'Error rate spiked to 41.6% on gemini during a regional outage.',
   41.6, 25,
   '{"platform":"gemini","group_id":3,"region":"eu-west"}',
   now() - interval '2 days 3 hours', now() - interval '2 days 1 hour 12 minutes', true, now() - interval '2 days 3 hours'),

  (5, 'P2', 'resolved', 'CPU usage above 80%',
   'CPU at 82.1%.',
   82.1, 80,
   NULL,
   now() - interval '2 days 15 hours', now() - interval '2 days 14 hours 50 minutes', false, now() - interval '2 days 15 hours'),

  (3, 'P2', 'resolved', 'P99 latency above 8s',
   'P99 at 8.8s on openai.',
   8800, 8000,
   '{"platform":"openai"}',
   now() - interval '3 days 2 hours', now() - interval '3 days 1 hour 30 minutes', false, now() - interval '3 days 2 hours'),

  (7, 'P1', 'resolved', 'Concurrency queue backing up',
   'Queue depth reached 33 waiting requests.',
   33, 20,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '3 days 20 hours', now() - interval '3 days 19 hours 15 minutes', true, now() - interval '3 days 20 hours'),

  (4, 'P3', 'resolved', 'P95 latency above 3s',
   'Informational: P95 at 3.2s for one window.',
   3200, 3000,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '4 days 6 hours', now() - interval '4 days 5 hours 58 minutes', false, now() - interval '4 days 6 hours'),

  (2, 'P0', 'resolved', 'Success rate below 90%',
   'Success rate fell to 74.5% for 22 minutes. Every upstream account was rate limited at once.',
   74.5, 90,
   '{"platform":"anthropic","group_id":1,"region":"us-east"}',
   now() - interval '4 days 14 hours', now() - interval '4 days 13 hours 38 minutes', true, now() - interval '4 days 14 hours'),

  (1, 'P1', 'resolved', 'Error rate above 10%',
   'Error rate at 15.3% on anthropic.',
   15.3, 10,
   '{"platform":"anthropic","group_id":1}',
   now() - interval '5 days 8 hours', now() - interval '5 days 7 hours 5 minutes', true, now() - interval '5 days 8 hours'),

  (6, 'P1', 'resolved', 'Memory usage above 85%',
   'Resident memory reached 87.2%.',
   87.2, 85,
   NULL,
   now() - interval '6 days 1 hour', now() - interval '6 days 40 minutes', false, now() - interval '6 days 1 hour'),

  (5, 'P2', 'manual_resolved', 'CPU usage above 80%',
   'CPU at 84.4% during a batch import. Closed by an operator as expected load.',
   84.4, 80,
   NULL,
   now() - interval '6 days 12 hours', now() - interval '6 days 10 hours', false, now() - interval '6 days 12 hours');
