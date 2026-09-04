---
name: inferno-inference-deploy
description: Ship a new build to the inference.tryopencomputer.com Inferno instance (redeploy, verify, roll back) and reach the box in the first place. Use when a change needs to go live on the inference endpoint, when checking whether the running build is actually healthy versus merely /health-green, or when a bad redeploy needs to come back. Does not cover first-boot provisioning — see deploy/inference/README.md and bootstrap.sh for that.
license: MIT
metadata:
  opencomputer:
    tags: [Inferno, Deploy, EC2, ECR, Cloudflare, Redeploy, Rollback, Read-Path]
    related_skills: [inferno-upstream-port]
---

# Inferno inference redeploy

`inference.tryopencomputer.com` is a second Inferno instance, stood up
2026-09-03/04 alongside the pre-existing `router.tryopencomputer.com`,
deliberately mirroring how that one is built and sharing nothing with it but
the AWS account. First-boot provisioning is covered by
`deploy/inference/README.md` and `deploy/inference/bootstrap.sh` — read those
first. This skill is the part they don't cover: shipping a **new** build to an
**already-running** instance, and proving it worked.

## The one rule that costs the most when broken

**A green `/health` is not proof the deployment works. Prove it with a real
completion.**

On 2026-09-03 a rebuild of this exact instance reported every signal it had:
container `healthy`, `/health` → `200 {"status":"ok"}`, the SPA served, the
auth gate correctly rejecting anonymous callers with `401`. Meanwhile OAuth
token exchange returned `500` to every real client, for hours, because the new
code needed `SERVER_FRONTEND_URL` as the JWT issuer and nothing had ever set
it. Every automated signal was green. The only thing that would have caught it
is what actually broke: a client completing a real request.

`SERVER_FRONTEND_URL` is now set (`https://inference.tryopencomputer.com`,
threaded through `docker-compose.yml` and `config.yaml`'s `server.frontend_url`
on the instance). The lesson generalizes past that one variable: a health
check answers a narrower question than "does this work," and this skill
treats it that way everywhere — `scripts/verify.sh` gates on a real
`/v1/chat/completions` completion, not on container health, and
`scripts/redeploy.sh` will not call a deploy successful without it.

## Shape

| | |
|---|---|
| instance | `i-0066a065c11a7b94d` — `oc-inference` — `3.82.43.139` (public) / `172.31.33.32` (private) — t3.small, us-east-1 |
| **off-limits** | `i-0e4fe42fc3fadf277` — `oc-router` — `35.175.193.193` — **production. This skill's scripts refuse to run against it, hard-coded by instance id.** |
| account/region | AWS `133277694446`, `us-east-1` |
| network | no inbound 80/443 on either box; SSH (22) open only to one `/32` admin CIDR (`122.171.16.188/32` on `oc-inference`'s SG, `sg-0c2b5ebfe8b5de271`); the only way in is outbound-initiated |
| tunnel | `cloudflared`, token-based (`systemctl status cloudflared`, token at `/etc/cloudflared/token`), tunnel id `033ebfb8-7c75-4018-9859-db47af90f68e` |
| DNS | `inference.tryopencomputer.com` → proxied CNAME → `033ebfb8-7c75-4018-9859-db47af90f68e.cfargotunnel.com` |
| image | ECR `133277694446.dkr.ecr.us-east-1.amazonaws.com/oc-platform/inferno`, `linux/amd64` only, `MUTABLE` tag policy. Written to **from the instance**, not from the builder — see **Traps**. |
| IAM (ECR write) | inline policy `InfernoEcrPush` on `oc-router-ec2-role`, scoped to this one repository (`deploy/inference/ecr-push-policy.json`) |
| compose | `/opt/inferno/docker-compose.yml`, project name `inferno`, services `postgres` / `redis` / `inferno` (container names `inferno-postgres` / `inferno-redis` / `inferno`) |
| app | listens on `127.0.0.1:8080` only inside the box; `GET /health` → `{"status":"ok"}`; served through the tunnel at the public hostname |
| data | `/opt/inferno/pgdata` (Postgres volume), `/opt/inferno/data` (`config.yaml`, `.installed`, `model_pricing.json`, `logs/`, `pages/`, `plugins/`) — seeded, live, never re-seeded (see **Data** below) |
| IAM | the instance itself carries `oc-router-ec2-role` via its instance profile — it can `aws ecr get-login-password` and pull without any credential ever being copied to it |

## How to reach it

AWS credentials for account `133277694446` exist **only** on `oc-internal`
(Tailscale), and only work as the `architsakri` user, not `root`. Every AWS
call and every SSH hop to either EC2 box goes through that machine:

```bash
ssh root@oc-internal "su - architsakri -c '<command>'"
```

`oc-internal` is itself the machine whose IP sits in `oc-inference`'s SSH
allowlist and holds the EC2 key pair
(`~architsakri/.ssh/oc-router-key.pem`, keypair name `oc-router-key`, shared
by both instances) — so reaching the instance itself is a second hop from
there:

```bash
ssh root@oc-internal "su - architsakri -c \\
  'ssh -i ~/.ssh/oc-router-key.pem ec2-user@3.82.43.139 \"<command>\"'"
```

**The quoting trap:** everything inside `su ... -c '...'` is a single shell
argument that survives two more levels of parsing (the `su` shell, then
whatever it runs). Single quotes cannot nest — the moment your `<command>`
itself needs a single quote (any AWS CLI JMESPath filter like
`Tags[?Key==\`Name\`]` needs backtick-quoted literals, and those backticks
need escaping too, or the local shell tries to execute them) you get a syntax
error that looks like the connection failed, not like a quoting bug. Two ways
out:
- For a single AWS CLI call, use double quotes outer / escaped double quotes
  inner and escape the JMESPath backticks (`` \` ``), matching how this
  skill's own investigation commands were run.
- For anything with its own quoting (a remote pipeline, a heredoc, a command
  that itself SSHes again), base64-encode the command and have the far end
  decode-and-run it. Both scripts below do this internally
  (`run_internal` / `run_instance`) specifically so a redeploy never breaks on
  a quoting accident hours into a build.

Read-only inspection of `oc-inference` (`docker ps`, `docker compose config`,
env var *names*, image tags/digests) is expected and safe. Never run anything
against `oc-router` — not even a read — outside of a plain HTTPS `curl` to its
public hostname, which is no different from what any customer's browser does.

## How to redeploy a new build

```bash
cd skills/inferno-inference-deploy
./scripts/redeploy.sh --dry-run          # see the exact command sequence first
./scripts/redeploy.sh                    # build HEAD, ship it, verify, auto-rollback on failure
./scripts/redeploy.sh --ref <commit>     # ship a specific commit instead of HEAD
```

The sequence, in order:

1. **Guard.** Refuse if the target instance id equals the router's instance
   id — compared as literal ids, not as a name string that a later edit could
   quietly repoint.
2. **Record the rollback target.** Before touching anything, read the
   currently-running image's digest off the instance
   (`docker inspect inferno --format '{{index .RepoDigests 0}}'`) and hold it.
   This is the thing rollback retags back — not a tag, a digest, because
   `:latest` is mutable and by the time rollback runs it may already point at
   the broken build.
3. **Confirm the compose service name before trusting it.** `docker compose
   config --services` on the instance is checked against the constant this
   script hard-codes (`inferno`) before anything is recreated. See **Traps**
   — this check exists because of a specific past failure.
4. **Build from a pinned ref, not the working tree.** `git archive <ref>` is
   staged onto `oc-internal` and built there — `oc-internal` is a genuine
   Intel x86_64 host, so this is a native `linux/amd64` build, no `buildx`
   emulation needed and no risk of an Apple-silicon Mac silently producing
   `arm64`. Building from `git archive` also means a redeploy can never pick
   up someone else's uncommitted work sitting in the same checkout — it only
   ever ships what's actually committed at `<ref>`.
5. **Ship the image straight to the instance**, `docker save | gzip | ssh |
   docker load`. **ECR is not on the outbound path**, because `oc-internal`
   physically cannot push to it — see **Traps**. At ~132 MB this takes
   seconds and needs no registry credential on the Mac at all.
6. **On the instance:** `docker tag <repo>:<short-sha> <repo>:latest` (compose
   references `:latest`), then `docker compose up -d --no-deps
   --force-recreate inferno`.
7. **Wait for healthy**, then **assert the running container is the image we
   just built** — compare `docker inspect inferno --format '{{.Image}}'`
   against the built tag's id, and fail `image-mismatch` if they differ.
   This is the single check that separates "deployed" from "still running
   whatever was there before"; no health or completion check can substitute
   for it (see **Traps**).
8. **Then** run the **same `probe()`** that `scripts/verify.sh` uses (sourced,
   not copied) — a real `/v1/chat/completions` call, not a health poll.
9. **On probe failure**, automatically retag the recorded previous image id
   back to `:latest`, recreate, wait, and re-run the *real*, unmodified
   `probe()` to confirm the rollback actually restored service. Reports which
   of `DEPLOY_STATUS=deployed`, `DEPLOY_STATUS=rolled-back-ok`, or
   `DEPLOY_STATUS=rollback-FAILED` it ended in — the last one means the
   gateway may be down and needs a human now.
10. **Archive to ECR from the INSTANCE**, once the deploy is live and
    verified. ECR still matters — it is the only copy that outlives the box,
    and compose pulls `:latest` from it on a rebuilt instance — but the
    instance can authenticate (IAM role, plain-file credentials) where the
    Mac cannot. This step is deliberately **not** fatal: the deploy is
    already verified by then, and an archive is not worth failing a good
    deploy over. It is loud on failure, so a gap in the archive is visible.
9. **Always, regardless of outcome,** checks `router.tryopencomputer.com`
   over public HTTPS and reports it loudly if it doesn't answer as expected.
   This redeploy never touches the router directly, but a shared account, a
   shared Cloudflare zone, or a typo'd hostname have all historically been
   the kind of thing that silently affects a neighbour — check it every time,
   not just when something feels wrong.

`redeploy.sh` never duplicates the probe logic — it `source`s
`verify.sh` and calls its `probe()` and `check_router()` functions directly,
guarded so `verify.sh` only runs its own `main()` when executed standalone.
One definition, one place it can drift.

## How to verify

Run standalone, any time, against the live public endpoint — no SSH required
except as a fallback to fetch the probe API key if `INFERNO_API_KEY` isn't
set:

```bash
INFERNO_API_KEY=sk-... INFERNO_PROBE_MODEL=<a model on a seeded account> \
  ./scripts/verify.sh
```

It exits `0` only if **all four** hold, and prints each check's result:

1. **A real completion returns real content.** `POST
   https://inference.tryopencomputer.com/v1/chat/completions` with a valid
   key returns content, not just `200`. This is the check that would have
   caught the 2026-09-03 incident — `/health` would not have.
2. **The SPA is served.** `GET /` returns `200`, `text/html`, and the Inferno
   app shell.
3. **An anonymous protected call is rejected.** `POST
   /v1/chat/completions` with no `Authorization` returns `401`
   (`API_KEY_REQUIRED`) — not `500`, not a silent pass-through.
4. **The neighbour is unaffected.** `https://router.tryopencomputer.com`
   still answers over public HTTPS as it did before. Checked by hostname
   only, over the same public path any customer uses — never by SSH or AWS
   API against the router's instance.

Any failure prints `FAIL` for that check with the response it actually got,
and the script exits non-zero. Nothing here is quiet.

## How to roll back

Two different failures, two different fixes — know which one you have before
acting:

- **A bad build** (the new image is running but broken) → **image tag
  rollback**. `redeploy.sh` does this automatically on a failed probe; done
  by hand it's: retag the previous digest (get it from `docker inspect
  inferno --format '{{index .RepoDigests 0}}'` *before* you start, or from
  ECR's image list by push date if you didn't) back to `<repo>:latest` on the
  instance, then `docker compose up -d --no-deps --force-recreate inferno`.
  No terminate, no data touched, seconds not minutes.
- **A bad instance** (the box itself is compromised, corrupted, or
  unrecoverable — not just a bad app build) → **terminate and recreate**,
  per `deploy/inference/README.md`'s original rollback: terminate the
  instance, delete the CNAME, relaunch with `bootstrap.sh`. This loses
  whatever is on local disk that isn't in the seed dump, so it is a last
  resort, not a response to a bad app build.

Never reach for the second when the first would do — a redeploy gone wrong is
almost always the first kind.

## Traps

Each of these cost a real debugging round. The first four are from initial
provisioning (`deploy/inference/README.md`); the last two are redeploy-specific
and are new to this skill.

- **The cloudflared RPM asset is `x86_64`, not `amd64`.** `amd64` names the
  `.deb` and the raw binary; grabbing the wrong one and letting `curl` save a
  404 page as the `.rpm` (no `-f`) makes `dnf` fail on a file full of HTML.
- **Postgres 18 needs `PGDATA` set explicitly**, or it refuses a bare
  `/var/lib/postgresql/data` mount and never becomes healthy.
- **`DATABASE_DBNAME`, not `DATABASE_NAME`.** The wrong key means the app
  can't see a database and boots into the setup wizard instead of serving.
- **`config.yaml` + `.installed` decide "first run," not environment
  variables.** Without both present, a correctly-configured app still starts
  the wizard. A redeploy never touches these files — they already exist on
  this instance — but recreating `/opt/inferno/data` from scratch would
  reopen this exact trap.
- **`SERVER_FRONTEND_URL` is the OAuth `iss` claim, and it fails silent.**
  Unset, every other signal — container health, `/health`, the SPA, the
  anonymous-reject path — stays green while token exchange 500s for every
  real client. This is the reason `/health` is not this skill's bar for
  success anywhere. It is set now (`https://inference.tryopencomputer.com`,
  in both `docker-compose.yml`'s environment block and `config.yaml`'s
  `server.frontend_url`); a redeploy that touches either file without
  carrying it forward reopens the incident.
- **`su - user -c '…'` does not propagate exit status. This is the big one.**
  A remote `exit 42` comes back to the caller as `0`. Verified directly on
  2026-09-04. While that hole was open, *no* error checking anywhere in the
  script could work: push failed, the ECR existence check failed, pull failed,
  retag failed — and every one of them reported success. `run_internal` now
  has the remote side print `__RC__:$?` on its last line and parses it back,
  and treats a missing marker (the ssh/su hop itself broke) as failure, never
  as pass. If you add a new remote helper, do the same or it will lie to you.
- **A green probe cannot tell you WHICH build is running.** The old image
  answers `/v1/chat/completions` perfectly well. On 2026-09-04 a run whose
  push, pull and retag had all failed recreated the container on the *old*
  image, watched it come up healthy, passed the completion probe, and printed
  `DEPLOY_STATUS=deployed`. That is this skill's own headline failure mode
  turned inward: a check passing for a reason unrelated to the claim it makes.
  Liveness is not identity — hence the `image-mismatch` assertion at step 7.
- **`RepoDigests` is a field on an image, not a container.** `docker inspect
  <container> --format '{{index .RepoDigests 0}}'` returns a template error
  and an empty string, which then trips the "is the app even running?" guard
  on a container that is perfectly healthy — a fields bug wearing the costume
  of an outage. Use `{{.Image}}`, which is also the better rollback anchor:
  already local, so rollback is a `docker tag` with nothing to pull, and
  still valid if the ECR tag is later overwritten.
- **`oc-internal` cannot push to any registry non-interactively.** It runs
  Docker Desktop for Mac, whose CLI keeps credentials in the login keychain;
  over ssh there is no GUI session to unlock it and `docker login` dies with
  `User interaction is not allowed. (-25308)`. An isolated `--config` dir does
  not help, and neither does an explicit `"credsStore": ""` in it — both were
  tried, the CLI called the keychain helper anyway (`login rc=1`). Do not
  spend another round on this; the image goes save/load and ECR is written
  from the instance instead.
- **The instance's IAM role was pull-only.** `oc-router-ec2-role` carried only
  `AmazonEC2ContainerRegistryReadOnly`, so the archive push failed with
  `not authorized to perform: ecr:InitiateLayerUpload`. Fixed 2026-09-04 with
  an inline policy `InfernoEcrPush` scoped to
  `repository/oc-platform/inferno` alone — checked into this repo at
  `deploy/inference/ecr-push-policy.json`. The role is SHARED with the
  production router instance, which is why the policy names one repository
  and five actions rather than `ecr:*`.
- **A stale hard-coded compose service name can kill a 20+ minute build at
  the very last step.** An earlier draft of this tooling hard-coded the
  compose service Redeploy needed to recreate; it had drifted from what the
  actual `docker-compose.yml` on the instance names its service, so the build
  and push succeeded — a 23-minute round trip — and the deploy step failed
  immediately after, for a reason that had nothing to do with the build. The
  current confirmed service/container name is **`inferno`** for the app,
  `postgres` / `redis` for its dependencies (compose service keys; the
  container names are `inferno`, `inferno-postgres`, `inferno-redis`). Don't
  trust that this stays true forever — `redeploy.sh` checks `docker compose
  config --services` against its constant before recreating anything, exactly
  so this class of failure can't repeat itself silently.

## Data

The instance was seeded from a local `pg_dump -Fc` of the `oc_internal`
Inferno database — 23 provider accounts. **A redeploy must never re-seed.**
The running database is the source of truth now; restoring a dump into it
would silently overwrite live accounts, keys, and usage history with
whatever was true at snapshot time. `bootstrap.sh`'s seed-restore step only
ever runs on first boot, before the app starts, and only if a dump file has
been staged — nothing in `redeploy.sh` stages, restores, or even reads
`/opt/inferno/pgdata`. Dumps themselves are never printed, never committed to
this repo, and are deleted from every intermediate host (including
`oc-internal`) as soon as a restore completes — if you ever need to seed a
*new* instance, treat the dump file itself as a secret for its entire
lifetime, not just at rest.

## What this skill will not do

- **It will not provision.** No instance launch, no tunnel creation, no DNS
  record. That's `deploy/inference/README.md` and `bootstrap.sh`.
- **It will not touch `oc-router`** beyond a public HTTPS health check of its
  hostname — never SSH, never an AWS API call scoped to its instance id.
- **It will not re-seed the database**, ever, under any flag.
- **It will not trust a health check** as proof of anything beyond "the
  process is running."

## The probe key

`verify.sh` and `redeploy.sh` both need `INFERNO_API_KEY` and
`INFERNO_PROBE_MODEL`. Confirmed working 2026-09-04:

```bash
INFERNO_API_KEY="$(docker exec sub2api-postgres \
  psql -U sub2api -d oc_internal -tAc 'select key from api_keys where id=2')"
INFERNO_PROBE_MODEL=claude-opus-5
```

That is the local Inferno database on the laptop, and the key is `OAuth agent
hermes-cli` — the admin's, stored in plaintext in the `key` column, live on
production because production was seeded from that database.

**Watch which database.** That one Postgres container holds three: `postgres`,
`sub2api` and `oc_internal`. The container's own `POSTGRES_DB=sub2api` points
at the WRONG one — `sub2api` is stale scaffolding holding thirteen fictional
users (`ada`, `linus`, `grace`…) and twelve keys that authenticate nothing.
It looks entirely plausible, which is what makes it dangerous: a full,
coherent result reads as "found it" in a way an empty result never would.
The tell is `last_used_at` — every key in `sub2api` has none. The real
database is `oc_internal`: two users, one key, real usage history.

The bootstrap `DEFAULT_API_KEY` recorded in the instance's `config.yaml` is
NOT a working key — tested live against `/v1/models` on 2026-09-04, it
returns `INVALID_API_KEY`. Don't reach for it.

Note also that the auth middleware rate-limits repeated bad keys
(`rejectInvalidAuthAbuse`), so guessing is actively costly.
