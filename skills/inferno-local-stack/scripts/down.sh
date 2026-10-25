#!/usr/bin/env bash
# Stop the Inferno local stack. Leaves deploy/postgres_data alone, so data and
# the admin account survive. Pass --wipe to also drop the database volume.
set -uo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
SCRATCH="${TMPDIR:-/tmp}/inferno-local-stack"

echo "==> stopping vite (:3000) and backend (:8080)"
lsof -ti :3000 2>/dev/null | xargs kill 2>/dev/null
lsof -ti :8080 2>/dev/null | xargs kill 2>/dev/null
pkill -f 'exe/server' 2>/dev/null

echo "==> stopping containers"
if [ -f "$SCRATCH/ports.yml" ]; then
  ( cd "$REPO/deploy" && docker compose -f docker-compose.local.yml -f "$SCRATCH/ports.yml" down ) >/dev/null 2>&1
else
  ( cd "$REPO/deploy" && docker compose -f docker-compose.local.yml down ) >/dev/null 2>&1
fi

if [ "${1:-}" = "--wipe" ]; then
  echo "==> wiping postgres data and config.yaml (next up.sh re-runs setup)"
  rm -rf "$REPO/deploy/postgres_data" "$REPO/deploy/redis_data" "$REPO/backend/config.yaml"
fi

echo "done."
