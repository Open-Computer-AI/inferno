#!/usr/bin/env bash
#
# redeploy.sh — build, push, and ship a new Inferno image to oc-inference,
# then prove it works and roll back automatically if it doesn't.
#
# WHY THIS EXISTS: on 2026-09-03 a rebuild of this instance was green on every
# signal a health check can give -- container healthy, /health 200, SPA
# served, anonymous callers correctly rejected -- while OAuth token exchange
# 500'd for every real client for hours (SERVER_FRONTEND_URL, the JWT issuer,
# was never set). This script does not trust health checks: it gates success
# on scripts/verify.sh's probe() -- a real /v1/chat/completions completion --
# and rolls back to the previously-running image automatically if that probe
# fails.
#
# Usage:
#   redeploy.sh [--dry-run] [--ref <git-ref>] [-h|--help]
#
#   --dry-run     Print every command this script would run, in order,
#                 without running any of them (no ssh, no docker, no git
#                 network calls). Safe to run any time.
#   --ref <ref>   Build this git ref instead of HEAD. Always built via
#                 `git archive <ref>`, so uncommitted changes anywhere in the
#                 working tree -- including someone else's in-flight work --
#                 are never picked up.
#
# Env (never hardcoded, never echoed):
#   AWS_REGION            default us-east-1
#   OC_INTERNAL_HOST       default oc-internal
#   OC_INTERNAL_USER       default architsakri
#   INSTANCE_SSH_KEY       default ~/.ssh/oc-router-key.pem (on oc-internal)
#   INFERNO_API_KEY, INFERNO_PROBE_MODEL   forwarded to verify.sh's probe()
#   DEPLOY_FORCE_FAIL=1    fail the post-deploy probe ON PURPOSE, so the
#                          rollback path can be exercised. See below -- this
#                          is a separate hook from the probe itself so a
#                          successful rollback can still report success.
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Where to `git archive` the build context from.
#
# Two locations run this script and only one of them is inside the repo. Run
# from the checkout, scripts/../../.. IS the repo root. Run from an installed
# copy -- ~/.hermes/profiles/<profile>/skills/inferno-inference-deploy/ -- the
# same relative path lands on the profile directory, which is not a git repo
# at all, and every git call below fails. The installed copy refuses cleanly
# rather than shipping something wrong, but refusing is not doing the job.
#
# So: try the sibling checkout, and if it is not a repo, fall back to
# INFERNO_REPO_DIR and then to the known checkout on this host. Resolve it
# here, once, instead of letting each git call discover the problem
# separately.
_is_repo() { [ -n "${1:-}" ] && git -C "$1" rev-parse --git-dir >/dev/null 2>&1; }

REPO_DIR=""
for _candidate in \
  "${INFERNO_REPO_DIR:-}" \
  "$(cd "$SCRIPT_DIR/../../.." 2>/dev/null && pwd)" \
  "$HOME/OpenComputerV2/inferno" \
  "$HOME/OpenComputerV2/OpenComputerV2/inferno"
do
  if _is_repo "$_candidate"; then REPO_DIR="$_candidate"; break; fi
done

if [ -z "$REPO_DIR" ]; then
  echo "REFUSING: no Inferno git checkout found. Set INFERNO_REPO_DIR to one." >&2
  echo "  tried: \$INFERNO_REPO_DIR, $SCRIPT_DIR/../../.., \$HOME/OpenComputerV2/inferno" >&2
  exit 2
fi

# ---- fixed identity -----------------------------------------------------
#
# Hard-coded by instance id, not by a name string a later edit could quietly
# repoint at the wrong box.
ROUTER_INSTANCE_ID="i-0e4fe42fc3fadf277"    # oc-router -- PRODUCTION. Never targeted.
TARGET_INSTANCE_ID="i-0066a065c11a7b94d"    # oc-inference -- the only instance this script touches.
ROUTER_PUBLIC_IP="35.175.193.193"
INSTANCE_PUBLIC_IP_DEFAULT="3.82.43.139"

REGION="${AWS_REGION:-us-east-1}"
ECR_REGISTRY="133277694446.dkr.ecr.us-east-1.amazonaws.com"
ECR_REPO="oc-platform/inferno"
IMAGE="${ECR_REGISTRY}/${ECR_REPO}"

OC_INTERNAL_HOST="${OC_INTERNAL_HOST:-oc-internal}"
OC_INTERNAL_USER="${OC_INTERNAL_USER:-architsakri}"
INSTANCE_SSH_KEY="${INSTANCE_SSH_KEY:-~/.ssh/oc-router-key.pem}"

COMPOSE_DIR="/opt/inferno"
# Compose service key AND container name -- currently identical. Confirmed
# live 2026-09-04. NEVER trusted blindly: step 3 below checks this against
# `docker compose config --services` on the instance before recreating
# anything, because an earlier draft of this tooling hard-coded a service
# name that had drifted, and lost a 23-minute build at the very last step.
APP_SERVICE="inferno"
APP_CONTAINER="inferno"

DRY_RUN=0
REF="HEAD"

usage() {
  cat <<'EOF'
Usage: redeploy.sh [--dry-run] [--ref <git-ref>] [-h|--help]

Build the given git ref (default HEAD), push it to ECR, ship it to
oc-inference, and verify with a real completion. Automatically rolls back to
the previously-running image if verification fails.

  --dry-run     print every command that would run; run none of them
  --ref <ref>   build this ref instead of HEAD (via `git archive`, so a dirty
                working tree is never picked up)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --ref) REF="${2:?--ref needs a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# ---- the guard that must never be bypassed -------------------------------
guard_not_router() {
  if [ "$TARGET_INSTANCE_ID" = "$ROUTER_INSTANCE_ID" ]; then
    echo "REFUSING: TARGET_INSTANCE_ID equals ROUTER_INSTANCE_ID. This compares" >&2
    echo "the literal instance ids, not a name -- it cannot be fooled by an edited" >&2
    echo "label. Fix the constants in this script; do not work around this check." >&2
    exit 3
  fi
}
guard_not_router

# ---- remote execution helpers --------------------------------------------
#
# Both hops are base64-wrapped so nothing this script sends ever needs nested
# shell-quoting to survive `su - architsakri -c '...'` plus a second SSH hop.
# See SKILL.md's "quoting trap" -- this is that fix, applied automatically.

run_internal() {
  local remote_cmd="$1"
  if [ "$DRY_RUN" = 1 ]; then
    printf '+ [oc-internal as %s] %s\n' "$OC_INTERNAL_USER" "$remote_cmd"
    return 0
  fi
  local b64
  b64="$(printf '%s' "$remote_cmd" | base64 | tr -d '\n')"
  ssh -o ConnectTimeout=15 "root@${OC_INTERNAL_HOST}" \
    "su - ${OC_INTERNAL_USER} -c 'echo ${b64} | base64 -d | bash -s'"
}

run_instance() {
  local remote_cmd="$1"
  if [ "$DRY_RUN" = 1 ]; then
    printf '+ [oc-inference via oc-internal] %s\n' "$remote_cmd"
    return 0
  fi
  local b64
  b64="$(printf '%s' "$remote_cmd" | base64 | tr -d '\n')"
  local inner="ssh -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -i ${INSTANCE_SSH_KEY} ec2-user@${INSTANCE_PUBLIC_IP} \"echo ${b64} | base64 -d | bash -s\""
  run_internal "$inner"
}

# ---- resolve + re-guard the instance IP -----------------------------------
if [ "$DRY_RUN" = 1 ]; then
  INSTANCE_PUBLIC_IP="$INSTANCE_PUBLIC_IP_DEFAULT"
  echo "+ [oc-internal] aws ec2 describe-instances --region $REGION --instance-ids $TARGET_INSTANCE_ID --query 'Reservations[0].Instances[0].PublicIpAddress' --output text"
else
  INSTANCE_PUBLIC_IP="$(run_internal "aws ec2 describe-instances --region ${REGION} --instance-ids ${TARGET_INSTANCE_ID} --query 'Reservations[0].Instances[0].PublicIpAddress' --output text")"
  INSTANCE_PUBLIC_IP="$(printf '%s' "$INSTANCE_PUBLIC_IP" | tr -d '[:space:]')"
  if [ -z "$INSTANCE_PUBLIC_IP" ] || [ "$INSTANCE_PUBLIC_IP" = "$ROUTER_PUBLIC_IP" ]; then
    echo "REFUSING: resolved instance IP is empty or equals the router's IP ($ROUTER_PUBLIC_IP)." >&2
    exit 3
  fi
fi

echo "DEPLOY_TARGET=${TARGET_INSTANCE_ID} (${INSTANCE_PUBLIC_IP})"

# ---- step 2: record the rollback target -----------------------------------
echo "DEPLOY_PHASE=record-previous"
if [ "$DRY_RUN" = 1 ]; then
  echo "+ [oc-inference] docker inspect ${APP_CONTAINER} --format '{{index .RepoDigests 0}}'"
  PREV_IMAGE_REF="<resolved-at-runtime>"
else
  PREV_IMAGE_REF="$(run_instance "docker inspect ${APP_CONTAINER} --format '{{index .RepoDigests 0}}'")"
  PREV_IMAGE_REF="$(printf '%s' "$PREV_IMAGE_REF" | tr -d '[:space:]')"
  if [ -z "$PREV_IMAGE_REF" ]; then
    echo "DEPLOY_STATUS=no-previous-image (is ${APP_CONTAINER} actually running?)" >&2
    exit 2
  fi
fi
echo "DEPLOY_ROLLBACK_TARGET=${PREV_IMAGE_REF}"

# ---- step 3: confirm the compose service name before trusting it ----------
echo "DEPLOY_PHASE=confirm-service-name"
if [ "$DRY_RUN" = 1 ]; then
  echo "+ [oc-inference] cd ${COMPOSE_DIR} && docker compose config --services"
else
  SERVICES="$(run_instance "cd ${COMPOSE_DIR} && docker compose config --services")"
  if ! printf '%s\n' "$SERVICES" | grep -qx "$APP_SERVICE"; then
    echo "REFUSING: compose service '${APP_SERVICE}' (hard-coded in this script) is not" >&2
    echo "among the instance's actual services: [${SERVICES}]. Update APP_SERVICE in" >&2
    echo "this script rather than guessing -- this exact drift once killed a build at" >&2
    echo "the very last step. See SKILL.md Traps." >&2
    exit 4
  fi
fi

# ---- step 4: build from a pinned ref, not the working tree ----------------
COMMIT="$(git -C "$REPO_DIR" rev-parse "$REF" 2>/dev/null || true)"
if [ -z "$COMMIT" ]; then
  echo "REFUSING: '${REF}' does not resolve to a commit in ${REPO_DIR}." >&2
  exit 2
fi
TAG="$(git -C "$REPO_DIR" rev-parse --short "$COMMIT")"
STAGE="/tmp/inferno-deploy-${TAG}"

# Print the checkout too, not just the ref. "HEAD" means nothing without
# knowing which working copy resolved it -- the checkout on this host and the
# one on a laptop are routinely several commits apart.
echo "DEPLOY_REPO=${REPO_DIR}"
echo "DEPLOY_REF=${REF} (${COMMIT})"
echo "DEPLOY_TAG=${TAG}"
echo "DEPLOY_PHASE=stage-build-context"

if [ "$DRY_RUN" = 1 ]; then
  echo "+ [oc-internal] rm -rf ${STAGE} && mkdir -p ${STAGE}"
  echo "+ [local->oc-internal] git -C ${REPO_DIR} archive ${COMMIT} | ssh root@${OC_INTERNAL_HOST} \"su - ${OC_INTERNAL_USER} -c 'tar -x -C ${STAGE}'\""
else
  run_internal "rm -rf ${STAGE} && mkdir -p ${STAGE}"
  git -C "$REPO_DIR" archive "$COMMIT" | \
    ssh -o ConnectTimeout=15 "root@${OC_INTERNAL_HOST}" \
      "su - ${OC_INTERNAL_USER} -c 'mkdir -p ${STAGE} && tar -x -C ${STAGE}'"
fi

# ---- step 5: build (native amd64 on oc-internal) and push -----------------
echo "DEPLOY_PHASE=build-and-push"
run_internal "cd ${STAGE} && docker build -t ${IMAGE}:${TAG} -t ${IMAGE}:latest -f Dockerfile ."
run_internal "aws ecr get-login-password --region ${REGION} | docker login --username AWS --password-stdin ${ECR_REGISTRY}"
run_internal "docker push ${IMAGE}:${TAG}"
run_internal "docker push ${IMAGE}:latest"
run_internal "rm -rf ${STAGE}"

# ---- step 6: pull + recreate on the instance -------------------------------
echo "DEPLOY_PHASE=pull-and-recreate"
run_instance "aws ecr get-login-password --region ${REGION} | docker login --username AWS --password-stdin ${ECR_REGISTRY}"
run_instance "docker pull ${IMAGE}:${TAG}"
run_instance "docker tag ${IMAGE}:${TAG} ${IMAGE}:latest"
run_instance "cd ${COMPOSE_DIR} && docker compose up -d --no-deps --force-recreate ${APP_SERVICE}"

if [ "$DRY_RUN" = 1 ]; then
  echo "+ [oc-inference] wait for ${APP_CONTAINER} to report healthy (poll docker inspect .State.Health.Status)"
  echo "+ run scripts/verify.sh's probe() and check_router() against https://inference.tryopencomputer.com"
  echo "DEPLOY_STATUS=dry-run-ok (nothing was executed)"
  exit 0
fi

# ---- step 7: wait for healthy ----------------------------------------------
echo "DEPLOY_PHASE=wait-healthy"
HEALTH="unknown"
for _ in $(seq 1 30); do
  HEALTH="$(run_instance "docker inspect ${APP_CONTAINER} --format '{{.State.Health.Status}}' 2>/dev/null || echo none")"
  HEALTH="$(printf '%s' "$HEALTH" | tr -d '[:space:]')"
  [ "$HEALTH" = "healthy" ] && break
  sleep 5
done
echo "DEPLOY_HEALTH=${HEALTH}"

# ---- step 8: the check that actually matters -------------------------------
#
# source, not copy: probe() and check_router() are defined once, in
# verify.sh, and used identically here and standalone. Sourcing (rather than
# executing) verify.sh runs none of its checks -- its BASH_SOURCE guard at
# the bottom keeps main() from firing.
# shellcheck source=./verify.sh
source "${SCRIPT_DIR}/verify.sh"

echo "DEPLOY_PHASE=verify"
PROBE_RESULT="$(probe)"
echo "$PROBE_RESULT"

# DEPLOY_FORCE_FAIL=1 fails ONLY this first evaluation, on purpose, so the
# rollback path below can be exercised without touching probe() itself. If
# probe() were sabotaged directly, the post-rollback re-check would ALSO
# fail even after a genuinely successful rollback -- a verifier that can lie
# about a recovery is worse than none. Keeping the injection point separate
# from probe() is what keeps the post-rollback confirmation honest.
PROBE_OK=1
printf '%s' "$PROBE_RESULT" | grep -q '^PASS' || PROBE_OK=0
if [ "${DEPLOY_FORCE_FAIL:-0}" = "1" ]; then
  echo "DEPLOY_FORCE_FAIL=1 -- treating this deploy's probe as failed on purpose"
  PROBE_OK=0
fi

if [ "$PROBE_OK" = 1 ]; then
  echo "DEPLOY_IMAGE=${IMAGE}:${TAG}"
  echo "DEPLOY_STATUS=deployed"
  ROLLED_BACK=0
else
  echo "DEPLOY_STATUS=probe-failed"
  echo "DEPLOY_PHASE=rollback"
  run_instance "docker tag ${PREV_IMAGE_REF} ${IMAGE}:latest"
  run_instance "cd ${COMPOSE_DIR} && docker compose up -d --no-deps --force-recreate ${APP_SERVICE}"
  for _ in $(seq 1 30); do
    HEALTH="$(run_instance "docker inspect ${APP_CONTAINER} --format '{{.State.Health.Status}}' 2>/dev/null || echo none")"
    HEALTH="$(printf '%s' "$HEALTH" | tr -d '[:space:]')"
    [ "$HEALTH" = "healthy" ] && break
    sleep 5
  done
  # The real, unmodified probe -- never subject to DEPLOY_FORCE_FAIL. This is
  # what makes the rollback's own success claim trustworthy.
  BACK_RESULT="$(probe)"
  echo "$BACK_RESULT"
  if printf '%s' "$BACK_RESULT" | grep -q '^PASS'; then
    echo "DEPLOY_STATUS=rolled-back-ok"
    ROLLED_BACK=1
  else
    echo "DEPLOY_STATUS=rollback-FAILED -- the gateway may be down. Escalate now." >&2
    ROLLED_BACK=1
  fi
fi

# ---- step 9: always check the neighbour, regardless of outcome ------------
echo "DEPLOY_PHASE=check-neighbour"
ROUTER_RESULT="$(check_router)"
echo "$ROUTER_RESULT"
printf '%s' "$ROUTER_RESULT" | grep -q '^PASS' || \
  echo "WARNING: router.tryopencomputer.com did not answer as expected -- this deploy did not touch it directly, investigate anyway." >&2

if [ "$PROBE_OK" = 1 ]; then
  exit 0
elif [ "${ROLLED_BACK:-0}" = 1 ] && printf '%s' "$BACK_RESULT" | grep -q '^PASS'; then
  exit 1
else
  exit 5
fi
