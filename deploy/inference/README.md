# Inference endpoint deployment

`inference.tryopencomputer.com` — a second Inferno instance alongside
`router.tryopencomputer.com`, deployed the same way and sharing nothing with it
except the AWS account and the provider accounts in its seeded data.

## Shape

Modelled on the oc-router box, deliberately:

| | |
|---|---|
| instance | EC2 t3.small, AL2023 x86_64, 30 GB gp3 |
| network | **no inbound 80/443** — Cloudflare is reached *outbound* by a cloudflared tunnel |
| SSH | one admin CIDR only |
| data | Postgres + Redis containers on the instance, not RDS |
| image | ECR `oc-platform/inferno`, built linux/amd64 |
| DNS | proxied CNAME → `<tunnel-id>.cfargotunnel.com` |

No inbound web port is the load-bearing part: the box is not addressable from
the internet at all, and the tunnel is the only path in.

## Deploying

1. Create the tunnel (Cloudflare API, `config_src: cloudflare`) and set its
   ingress to `hostname → http://localhost:8080`.
2. Add the proxied CNAME for the hostname.
3. Push the image to ECR. Build it somewhere x86_64 — an Apple-silicon Mac
   produces arm64 and the instance will not run it.
4. Launch the instance with `bootstrap.sh` as user-data, with
   `TUNNEL_TOKEN`, `PG_PASSWORD`, `JWT_SECRET`, `DEFAULT_API_KEY`,
   `PUBLIC_URL` and `IMAGE` set. Generate every secret per instance.
5. To seed from an existing database, stage a `pg_dump -Fc` at
   `/opt/inferno/seed.dump` before first boot. The script restores it *before*
   starting the app, so migrations run against the real data, then shreds it.

## Validating

Not `/health`. A health check answers a narrower question than "does this
work" — on 2026-09-03 a container reported healthy, served the SPA and
correctly rejected anonymous callers while OAuth token exchange returned 500
to every real client. Prove it with a completion:

```bash
curl -X POST https://inference.tryopencomputer.com/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  --data '{"model":"claude-opus-5","messages":[{"role":"user","content":"ping"}],"max_tokens":20}'
```

Then confirm `router.tryopencomputer.com` still answers as it did before.

## Four things that will bite

Each of these cost a debugging round on the first deploy.

- **The cloudflared RPM is `x86_64`, not `amd64`.** `amd64` names the .deb and
  the raw binary. And `curl` needs `-f`: without it a 404 page is saved *as*
  the .rpm and `dnf` fails on a file full of HTML.
- **Postgres 18 needs `PGDATA`** set explicitly, or it refuses a bare
  `/var/lib/postgresql/data` mount and never becomes healthy.
- **`DATABASE_DBNAME`, not `DATABASE_NAME`.** With the wrong key the app
  cannot see a database and boots into the setup wizard.
- **`config.yaml` + `.installed` decide "first run"**, not environment
  variables. Without both, a correctly-configured app still starts the wizard
  and ignores a perfectly good database.

## Rolling back

Terminate the instance and delete the CNAME. Nothing else is shared, so the
router is unaffected either way.
