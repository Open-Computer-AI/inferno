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
AUTONOMOUS=0
REF="HEAD"

usage() {
  cat <<'EOF'
Usage: redeploy.sh [--dry-run] [--autonomous] [--ref <git-ref>] [-h|--help]

Build the given git ref (default HEAD), ship it to oc-inference, verify with a
real completion, and archive it to ECR. Automatically rolls back to the
previously-running image if verification fails.

  --dry-run     print every command that would run; run none of them
  --ref <ref>   build this ref instead of HEAD (via `git archive`, so a dirty
                working tree is never picked up)
  --autonomous  run with no human watching: refuse unless a set of
                preconditions hold, and report the outcome to Slack either
                way. See "Autonomous mode" in SKILL.md.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --autonomous) AUTONOMOUS=1; shift ;;
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

# ---- autonomous mode: prove it is safe, or refuse ------------------------
#
# The gate for an unattended deploy belongs HERE, not in a permission prompt.
# A prompt only protects you while somebody is watching, which is exactly when
# protection matters least. So --autonomous refuses unless it can establish
# that this is a ROUTINE deploy, and reports the outcome either way.
#
# Every precondition below is something that, if false, means a human should
# be looking at it. None of them are style checks.
notify() {
  # $1 = one-line summary. Never fatal: a deploy must not fail because Slack
  # is unreachable. The line still goes to stdout, so the cron log has it even
  # when the post does not land.
  local msg="$1"
  echo "NOTIFY: ${msg}"
  [ "$AUTONOMOUS" = 1 ] || return 0
  [ -n "${SLACK_BOT_TOKEN:-}" ] && [ -n "${SLACK_DEV_CHANNEL:-}" ] || return 0
  curl -sS -m 10 -o /dev/null -X POST https://slack.com/api/chat.postMessage \
    -H "Authorization: Bearer ${SLACK_BOT_TOKEN}" \
    -H "Content-type: application/json; charset=utf-8" \
    --data "$(printf '{"channel":"%s","text":"inference redeploy: %s"}' \
      "$SLACK_DEV_CHANNEL" "$msg")" >/dev/null 2>&1 || true
}

refuse_autonomous() {
  echo "REFUSING (--autonomous): $1" >&2
  notify "REFUSED before touching anything - $1"
  exit 6
}

if [ "$AUTONOMOUS" = 1 ] && [ "$DRY_RUN" = 0 ]; then
  # It must be ABLE to shout before it is allowed to act. An unattended deploy
  # whose failures go nowhere is the exact thing this mode exists to prevent.
  { [ -n "${SLACK_BOT_TOKEN:-}" ] && [ -n "${SLACK_DEV_CHANNEL:-}" ]; } || \
    refuse_autonomous "SLACK_BOT_TOKEN/SLACK_DEV_CHANNEL unset - will not deploy with no way to report failure"

  # A dirty tree means somebody is mid-edit. git archive would not ship their
  # changes, so what goes live is silently different from what they are
  # looking at.
  [ -z "$(git -C "$REPO_DIR" status --porcelain 2>/dev/null)" ] || \
    refuse_autonomous "working tree at ${REPO_DIR} is dirty - somebody is mid-edit"

  # A real commit, reachable from a branch: never a detached scratch commit
  # or a tag that has been moved.
  AUTO_SHA="$(git -C "$REPO_DIR" rev-parse --verify "${REF}^{commit}" 2>/dev/null)" || \
    refuse_autonomous "'${REF}' does not resolve to a commit"
  [ -n "$(git -C "$REPO_DIR" branch --contains "$AUTO_SHA" 2>/dev/null)" ] || \
    refuse_autonomous "commit ${AUTO_SHA} is not on any branch"

  # The gates that already exist become a deploy precondition. Shipping code
  # that does not typecheck, or whose tests fail, is not a routine deploy --
  # and routine is the only thing this mode is allowed to do.
  #
  # ONLY true pass/fail gates belong here. `june-lint` is deliberately absent:
  # it exits 1 whenever any violation exists, and 1370 of them exist by
  # design -- the standing backlog of the June conversion, tracked so it goes
  # down over time, not a regression. Adding it would make --autonomous refuse
  # every single run, forever. port-verify.mjs already encodes the same
  # asymmetry (june-lint rising is a NOTE, never a FAIL). Do not "fix" this by
  # putting it back; if you want it enforced, gate on it not RISING, which is
  # a different check than the one this script needs.
  echo "DEPLOY_PHASE=autonomous-preflight"
  while IFS= read -r _gate; do
    [ -n "$_gate" ] || continue
    ( cd "${REPO_DIR}/inferno-frontend" && NODE_OPTIONS= eval "$_gate" ) >/dev/null 2>&1 || \
      refuse_autonomous "gate failed: ${_gate}"
    echo "  gate ok: ${_gate}"
  done <<'GATES'
npx vue-tsc --noEmit
npx vitest run
node scripts/port-coverage.mjs --check-baseline
GATES

  # The lock is acquired further down, once run_instance() exists -- it is a
  # remote operation and these helpers are defined below.
fi

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

  # `su - user -c '...'` does NOT propagate the child's exit status on this
  # host: a remote `exit 42` comes back to us as 0. Verified directly on
  # 2026-09-04, and it is why must() sat there doing nothing while push,
  # describe-images and pull all failed in sequence -- every one of them
  # "succeeded" as far as the local shell could tell.
  #
  # So the remote side reports its own status explicitly, on the last line,
  # and we parse it back out. Output is captured rather than streamed, which
  # costs live build progress and buys an exit code that is actually true.
  local out rc
  out="$(ssh -o ConnectTimeout=15 "root@${OC_INTERNAL_HOST}" \
    "su - ${OC_INTERNAL_USER} -c 'echo ${b64} | base64 -d | bash -s; echo __RC__:\$?'" 2>&1)"
  rc="$(printf '%s\n' "$out" | sed -n 's/^__RC__:\([0-9][0-9]*\)$/\1/p' | tail -1)"
  printf '%s\n' "$out" | grep -v '^__RC__:[0-9]*$'
  # No marker at all means the hop itself broke (ssh/su failed) -- that is a
  # failure, not a pass. Never default this to 0.
  return "${rc:-1}"
}

# must <label> <command...> -- run it, and abort the deploy if it fails.
#
# WHY: this script runs `set -uo pipefail` WITHOUT -e, deliberately, because
# the probe and rollback paths need to inspect failures rather than die on
# them. The cost of that choice was that every remote step's exit status was
# simply discarded. On 2026-09-04 the push failed ("no basic auth
# credentials"), the pull then failed ("manifest unknown"), the retag failed
# ("No such image") -- and the script recreated the container on the OLD
# image, watched it come up healthy, ran a probe that passed because the old
# build works fine, and printed DEPLOY_STATUS=deployed. Three consecutive
# hard failures and a green result.
#
# That is the exact failure this skill was written to prevent, reproduced
# inside the skill: a check that passes for a reason unrelated to the claim
# it is making. A probe proves the endpoint works; it cannot prove the thing
# it is testing is the thing you just built. So every step that MUST succeed
# is wrapped here, and the deploy stops at the first one that doesn't.
must() {
  local label="$1"; shift
  # Capture straight off the command. Testing it with `if "$@"` and reading $?
  # in the else branch reports the exit status of the *if*, not of the step --
  # so a step that died with 137 gets reported as "exit 0", which is a
  # confusing thing to hand someone at 2am.
  local rc=0
  "$@" || rc=$?
  [ "$rc" = 0 ] && return 0
  echo "DEPLOY_STATUS=FAILED at step '${label}' (exit ${rc})" >&2
  echo "Nothing was rolled back: the running container was not replaced." >&2
  exit 4
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

# ---- autonomous: take the deploy lock (needs run_instance, defined above) --
#
# Two concurrent deploys is the one way to reach a genuinely bad state: both
# retag :latest, and whichever recreates second wins silently. The lock lives
# on the INSTANCE, so it is shared by every machine that could start a deploy
# rather than being local to whichever one happened to go first. `mkdir` is
# the atomic primitive -- it either creates or fails, with no check-then-act
# window for a second deploy to slip through.
if [ "$AUTONOMOUS" = 1 ] && [ "$DRY_RUN" = 0 ]; then
  run_instance "mkdir /opt/inferno/.deploy.lock" >/dev/null 2>&1 || \
    refuse_autonomous "another deploy holds /opt/inferno/.deploy.lock (if stale: rmdir it by hand)"
  trap 'run_instance "rmdir /opt/inferno/.deploy.lock" >/dev/null 2>&1 || true' EXIT
fi

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
# The anchor is the local IMAGE ID of what the container is running right now.
#
# The obvious-looking `docker inspect <container> --format '{{index
# .RepoDigests 0}}'` does not work and fails in a way that reads like the app
# is down: RepoDigests is a field on an *image*, and inspecting a container
# returns container JSON, which has .Image (the image id) and .Config.Image
# (the tag) but no RepoDigests at all. Docker answers with a template error
# and an empty string, and the empty string then trips the "is it running?"
# guard even though the container is perfectly healthy.
#
# .Image is also the better anchor than a repo digest would be. It is already
# on the box, so a rollback is a local `docker tag` with nothing to pull, and
# it stays valid even if the ECR tag is later overwritten or the repo pruned.
if [ "$DRY_RUN" = 1 ]; then
  echo "+ [oc-inference] docker inspect ${APP_CONTAINER} --format '{{.Image}}'"
  PREV_IMAGE_REF="<resolved-at-runtime>"
else
  PREV_IMAGE_REF="$(run_instance "docker inspect ${APP_CONTAINER} --format '{{.Image}}'")"
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

# ---- step 5: build on oc-internal, ship the image straight to the instance --
#
# The image does NOT travel via ECR on the way out, because oc-internal
# physically cannot push from a non-interactive session.
#
# Its Docker is Docker Desktop for Mac, whose CLI stores registry credentials
# in the login keychain. Over ssh there is no GUI session to unlock it, so
# `docker login` dies with `User interaction is not allowed. (-25308)`. That
# is not fixable from this side: an isolated --config dir, and even an
# explicit "credsStore": "" in it, were both tried on 2026-09-04 and the CLI
# still called the keychain helper (login rc=1 either way).
#
# So the image goes `docker save` -> ssh -> `docker load`, directly from the
# builder to the instance. At ~132 MB gzipped this is seconds, needs no
# registry credentials at all on the Mac, and removes ECR from the critical
# path of a deploy entirely.
echo "DEPLOY_PHASE=build-and-ship"
must "build" run_internal "cd ${STAGE} && docker build -t ${IMAGE}:${TAG} -t ${IMAGE}:latest -f Dockerfile ."
must "ship-image" run_internal "set -o pipefail
docker save ${IMAGE}:${TAG} | gzip -1 | \
  ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -i ${INSTANCE_SSH_KEY} \
  ec2-user@${INSTANCE_PUBLIC_IP} 'gunzip | docker load'"
run_internal "rm -rf ${STAGE}"

# ---- step 6: retag + recreate on the instance ------------------------------
echo "DEPLOY_PHASE=retag-and-recreate"
must "retag" run_instance "docker tag ${IMAGE}:${TAG} ${IMAGE}:latest"
must "recreate" run_instance "cd ${COMPOSE_DIR} && docker compose up -d --no-deps --force-recreate ${APP_SERVICE}"

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

# ---- step 7b: is the container running the image we just built? ------------
#
# The probe below proves the endpoint works. It cannot prove the endpoint is
# running the NEW build -- the old one works too, which is exactly how the
# 2026-09-04 run passed every check while having shipped nothing. So compare
# identity directly: the image id the container is on must equal the image id
# of the tag we just pushed. This is the one assertion that distinguishes
# "deployed" from "still running whatever was there before", and no amount of
# health or completion checking can substitute for it.
BUILT_ID="$(run_instance "docker inspect ${IMAGE}:${TAG} --format '{{.Id}}'" | tr -d '[:space:]')"
RUNNING_ID="$(run_instance "docker inspect ${APP_CONTAINER} --format '{{.Image}}'" | tr -d '[:space:]')"
if [ -z "$BUILT_ID" ] || [ "$BUILT_ID" != "$RUNNING_ID" ]; then
  echo "DEPLOY_STATUS=FAILED image-mismatch" >&2
  notify "FAILED image-mismatch on ${TAG} - container is NOT running the new build"
  echo "  built   ${IMAGE}:${TAG} -> ${BUILT_ID:-<none>}" >&2
  echo "  running ${APP_CONTAINER}          -> ${RUNNING_ID:-<none>}" >&2
  echo "The container is NOT running the build this deploy produced." >&2
  exit 5
fi
echo "DEPLOY_IMAGE_ID=${RUNNING_ID}"

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
  notify "deployed ${TAG} - verified by real completion"
  ROLLED_BACK=0

  # Archive the image to ECR from the INSTANCE, not from oc-internal: the
  # instance authenticates with its IAM role and stores the token in a plain
  # file, so it has none of the Mac keychain problem that took ECR out of the
  # deploy path above.
  #
  # Deliberately NOT wrapped in must(): the deploy is already live and
  # verified by this point, and a registry that is merely a historical record
  # is not worth failing a good deploy over. It is loud when it fails so the
  # gap in the archive is visible rather than silent.
  echo "DEPLOY_PHASE=archive-to-ecr"
  if run_instance "set -o pipefail
aws ecr get-login-password --region ${REGION} | docker login --username AWS --password-stdin ${ECR_REGISTRY}
docker push ${IMAGE}:${TAG}
docker push ${IMAGE}:latest" >/dev/null 2>&1; then
    echo "DEPLOY_ARCHIVE=ok (${IMAGE}:${TAG} in ECR)"
  else
    echo "DEPLOY_ARCHIVE=FAILED -- the deploy is live and fine, but this build is NOT in ECR." >&2
    notify "WARN ${TAG} is live but NOT archived to ECR - a rebuilt instance would come back on the old image"
  fi
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
    notify "probe failed on ${TAG} - rolled back, service confirmed restored"
    ROLLED_BACK=1
  else
    echo "DEPLOY_STATUS=rollback-FAILED -- the gateway may be down. Escalate now." >&2
    notify "@here ROLLBACK FAILED after ${TAG} - the gateway may be DOWN, needs a human now"
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
