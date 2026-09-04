#!/usr/bin/env bash
#
# apply-verified — rebuild the local Inferno image, prove it actually serves,
# and roll back automatically if it does not.
#
# WHY THIS EXISTS: on 2026-09-03 a rebuild reported every green signal it had
# and was broken for every real client. The container was healthy, /health
# returned 200, the SPA served, the auth gate rejected anonymous callers
# exactly as it should, and zero errors appeared in the log. OAuth token
# exchange returned 500 to every client for hours, because the new code needed
# server.frontend_url as a JWT issuer and nothing had ever set it.
#
# Health checks answered a narrower question than the one being asked. So this
# script does not trust them: it exercises a REAL authenticated path end to end
# and treats anything else as failure.
#
# It also stamps the outgoing image before replacing it, so rollback is one
# retag rather than a rebuild, and performs that rollback itself on failure
# rather than leaving a broken gateway up while someone reads logs.
#
set -uo pipefail

REPO="${INFERNO_REPO:-$HOME/OpenComputerV2/inferno}"
COMPOSE="$REPO/deploy/docker-compose.local.yml"
PROJECT=deploy
SERVICE=inferno
IMAGE=inferno:local
PROFILE="${HERMES_PROFILE_NAME:-engineering}"

cd "$REPO" || { echo "APPLY_STATUS=no-repo"; exit 2; }
COMMIT="$(git rev-parse --short HEAD)"
STAMP="inferno:rollback-$(date +%Y%m%d-%H%M%S)"

echo "APPLY_COMMIT=$COMMIT"

# ---- stamp the outgoing image so rollback is a retag, not a rebuild ----------
if docker image inspect "$IMAGE" >/dev/null 2>&1; then
  docker tag "$IMAGE" "$STAMP" || { echo "APPLY_STATUS=stamp-failed"; exit 2; }
  echo "APPLY_ROLLBACK_TAG=$STAMP"
else
  echo "APPLY_ROLLBACK_TAG=none (no current $IMAGE to stamp)"
  STAMP=""
fi

# ---- build -------------------------------------------------------------------
echo "APPLY_PHASE=build"
if ! docker compose -f "$COMPOSE" --project-name "$PROJECT" build "$SERVICE" >/tmp/apply-build.log 2>&1; then
  echo "APPLY_STATUS=build-failed  (see /tmp/apply-build.log)"
  tail -5 /tmp/apply-build.log
  exit 1
fi

# ---- swap --------------------------------------------------------------------
echo "APPLY_PHASE=recreate"
docker compose -f "$COMPOSE" --project-name "$PROJECT" up -d --no-deps --force-recreate "$SERVICE" >/dev/null 2>&1

for i in $(seq 1 30); do
  s="$(docker inspect "$SERVICE" --format '{{.State.Health.Status}}' 2>/dev/null || echo none)"
  [ "$s" = "healthy" ] && break
  sleep 3
done
echo "APPLY_HEALTH=${s:-unknown}"

# ---- the check that actually matters ----------------------------------------
#
# Not /health. A real authenticated round trip, driven by hermes itself, which
# is the same client that was broken and silent for hours on 09-03. If this
# fails the build is not usable no matter what the container reports.
#
echo "APPLY_PHASE=real-path"
HERMES="$HOME/.hermes/hermes-agent/venv/bin/hermes"
# One definition, used for both the check and the post-rollback confirmation.
# Keeping them separate let a sabotaged check report a rollback failure that had
# not happened -- the service was serving fine; only the confirmation string was
# unsatisfiable. A verifier that can lie about a recovery is worse than none.
probe() { "$HERMES" -p "$PROFILE" -z "$PROBE_PROMPT" 2>&1 | tail -1; }
PROBE_PROMPT="${PROBE_PROMPT:-Reply with exactly: APPLY_PATH_OK}"
PROBE_EXPECT="${PROBE_EXPECT:-APPLY_PATH_OK}"

REAL="$(probe)"
echo "APPLY_REAL_PATH=$REAL"

# APPLY_FORCE_FAIL=1 fails ONLY this first check, leaving the post-rollback
# confirmation honest. Without a separate injection point the rollback path is
# untestable: sabotaging the probe or the expectation breaks both checks, so a
# rollback that genuinely restored service still reports failure. Exists to be
# exercised, not to be used.
if [ "${APPLY_FORCE_FAIL:-0}" = "1" ]; then
  echo "APPLY_FORCE_FAIL=1 — treating the real-path check as failed on purpose"
  REAL="__forced_failure__"
fi

if ! printf '%s' "$REAL" | grep -q "$PROBE_EXPECT"; then
  echo "APPLY_STATUS=real-path-failed"
  if [ -n "$STAMP" ]; then
    echo "APPLY_PHASE=rollback"
    docker tag "$STAMP" "$IMAGE"
    docker compose -f "$COMPOSE" --project-name "$PROJECT" up -d --no-deps --force-recreate "$SERVICE" >/dev/null 2>&1
    sleep 8
    back="$(probe)"
    if printf '%s' "$back" | grep -q "$PROBE_EXPECT"; then
      echo "APPLY_ROLLBACK=ok — the previous image is serving again"
    else
      echo "APPLY_ROLLBACK=FAILED — the gateway is down and rollback did not restore it. Escalate."
    fi
  else
    echo "APPLY_ROLLBACK=impossible — nothing was stamped"
  fi
  exit 1
fi

echo "APPLY_IMAGE=$(docker inspect "$SERVICE" --format '{{.Image}}' | cut -c1-19)"
# Keep the three most recent rollback points and drop the rest. Without this
# every apply leaves a tag forever; four accumulated during one afternoon of
# testing. Three is enough to step back through a bad week and few enough that
# the list stays readable.
docker images --format '{{.Repository}}:{{.Tag}}' \
  | grep -E '^inferno:rollback-' | sort -r | tail -n +4 \
  | while read -r old_tag; do docker rmi "$old_tag" >/dev/null 2>&1; done

echo "APPLY_STATUS=applied"
