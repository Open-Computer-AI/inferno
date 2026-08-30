---
name: inferno-local-stack
description: Bring up the Inferno local stack (Postgres, Redis, Go backend, Vite dev server) and log in as admin for browser testing or verification. Use when you need to run Inferno locally, verify a frontend change against a live app, drive the UI with Claude in Chrome, query the local database, or when someone says "bring up the stack", "verify in the browser", "test it locally", or "log in to Inferno".
---

# Inferno local stack

Brings up the 4 pieces and logs in. **Do not use `docker compose up inferno`** — see Gotcha 1.

```bash
skills/inferno-local-stack/scripts/up.sh          # start everything
skills/inferno-local-stack/scripts/up.sh --fresh  # also re-run the setup wizard
skills/inferno-local-stack/scripts/down.sh        # stop everything
```

| Piece | Where | Port |
|---|---|---|
| Postgres | container `sub2api-postgres` | **5433** (host) |
| Redis | container `sub2api-redis` | **6380** (host) |
| Go backend | native `go run ./cmd/server` | **8080** |
| Frontend | native `pnpm run dev` (Vite) | **3000** ← use this |

Open **http://localhost:3000**, not 8080. Credentials are in `deploy/.env`
and the script prints them.

## Logging in (browser)

The login is **two-step** and has three traps. Follow exactly:

1. Go to `http://localhost:3000/login`.
2. **Type** the email — do not use `form_input`. See Gotcha 5.
3. Press Return. The password field appears.
4. **Clear the field first** (`cmd+a`), then type the password. Chrome
   autofills a stale password and you get "invalid email or password".
5. Press Return.
6. An onboarding tour modal ("Welcome to Inferno", 1 of 21) covers the UI on
   admin pages. Press **Escape** to dismiss, then re-click what you wanted.

You land on a 404 if the URL carried `?redirect=`. That is normal — the login
worked. Navigate to a real route.

## Routes that are easy to get wrong

```
/admin/channels/monitor      NOT /admin/channel-monitor
/admin/accounts   /admin/channels   /admin/groups   /admin/usage
/admin/dashboard  /admin/ops        /admin/users    /admin/proxies
```

Full list: `grep -oE "path: '/admin/[a-z0-9-]+'" inferno-frontend/src/router/index.ts`

## Querying the database

```bash
docker exec sub2api-postgres psql -U sub2api -d sub2api -c '\dt'
docker exec sub2api-postgres psql -U sub2api -d sub2api -Atc 'SELECT count(*) FROM channels;'
```

A fresh install is **empty** — 0 channels, 0 accounts, 0 pricing rows. It can
verify that a feature *works*; it can never tell you what production data
looks like. Do not use it to answer "is anything mispriced in prod".

## Gotchas (each of these cost real time)

1. **`docker compose build inferno` fails with exit 134.** Docker Desktop is
   capped around 2 GB and `pnpm run build` needs more; it dies with
   "JavaScript heap out of memory". The host has plenty. Build natively
   (`cd inferno-frontend && pnpm run build`) or just use the Vite dev server,
   which is what `up.sh` does.
2. **Postgres and Redis publish no host ports** by default — the compose file
   deliberately keeps them internal. `up.sh` layers a scratch override that
   maps 5433/6380. Never edit `deploy/docker-compose.local.yml` to do this.
3. **`go run` does not serve the frontend.** The embed is behind the release
   build tag, so `http://localhost:8080/` returns 404 while the API on
   `:8080/api/v1/...` works fine. Use the Vite dev server on :3000; it proxies
   `/api`, `/v1` and `/setup` to :8080 automatically.
4. **After the setup wizard the process exits.** It logs "Service restart via
   exit only works on Linux with systemd" and dies. Start it again. Also kill
   the wizard process first or the restart hits
   "bind: address already in use" — `lsof -ti :8080 | xargs kill`.
5. **`form_input` does not trigger Vue's `v-model`.** It sets the DOM value and
   the app never sees it, so the form looks filled and the button does nothing.
   Click the field and use the `type` action instead.
6. **`atlas_schema_revisions` is not the migration ledger.** It holds a single
   baseline row, so it always reads like the DB is stuck at an old migration.
   Check for an actual column instead:
   `SELECT count(*) FROM information_schema.columns WHERE table_name='channel_model_pricing' AND column_name='time_pricing';`

## Verifying a frontend change

The dev server hot-reloads, so edit and re-check without restarting anything.
Prefer asserting on the DOM over reading screenshots when the claim is
structural:

```js
// via mcp__claude-in-chrome__javascript_tool
document.querySelectorAll('[data-testid="auto-reset-credit-settings"] input').length
```

Most components carry `data-testid` attributes — grep the component for them
rather than clicking blind.
