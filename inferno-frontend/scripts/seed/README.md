# Dev seeds

Every file is `now()`-relative, so re-running re-anchors it to the current clock.
That matters: the ops screens read a rolling window, and the container clock
drifts ahead of whatever was seeded earlier. **If a page reads "Idle" or
"No data", reseed before concluding the code is broken** — that has cost time
more than once.

```bash
cd inferno-frontend/scripts/seed
for f in seed-dashboard seed-year seed-models seed-ops seed-ops-dense seed-alert-events seed-users; do
  docker cp $f.sql sub2api-postgres:/tmp/
  docker exec sub2api-postgres psql -U sub2api -d sub2api -q -f /tmp/$f.sql
done
```

| file | what it makes | verifies |
|------|---------------|----------|
| `seed-dashboard.sql` | base dashboard totals | the stat tiles |
| `seed-year.sql` | a year of daily + hourly usage, cache adoption 18%→96% | trend charts, day-over-day deltas |
| `seed-models.sql` | per-model usage, 4 models × 365 days | token mix donut, tokens by model |
| `seed-ops.sql` | ops hourly metrics + error logs | ops header, error trend |
| `seed-ops-dense.sql` | per-minute traffic for 4 hours | ops charts at the 1h default, which bucket by minute |
| `seed-alert-events.sql` | 24 alerts, full P0–P3 ramp, all 3 statuses, both email states | alert table tones, severity ramp, the open/closed rule |
| `seed-users.sql` | 10 extra users + 30 days of usage each | the stacked user-trend chart, incl. its top-7 + "Other" fold |

Seed the *state space*, not just "some rows". The alert table shipped with two
rows that were both resolved with a null dimensions blob — which renders almost
none of what the component actually draws.

Local dev login: `admin@sub2api.local` / `36232e0cd5be929e4004e4ca025b100e`
