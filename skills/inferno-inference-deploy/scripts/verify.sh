#!/usr/bin/env bash
#
# verify.sh — prove the inference endpoint actually works, not just that it
# reports healthy.
#
# WHY THIS EXISTS: on 2026-09-03 a rebuild of this instance was green on every
# signal it had -- container healthy, /health 200, SPA served, anonymous
# callers correctly rejected with 401 -- while OAuth token exchange returned
# 500 to every real client for hours. SERVER_FRONTEND_URL (the JWT issuer)
# was never set. A health check answers a narrower question than "does this
# work". This script answers the real one: does a real client get a real
# completion, and did the redeploy leave the neighbour instance alone.
#
# Exit 0 only if ALL FOUR checks pass. Loud and non-zero on any failure.
#
# This file is also `source`d by redeploy.sh, which calls probe() and
# check_router() directly rather than re-implementing them -- one definition
# of "does it actually work", used by both the gate and the standalone check,
# so they cannot drift apart.
#
set -uo pipefail

INFERENCE_URL="${INFERENCE_URL:-https://inference.tryopencomputer.com}"
ROUTER_URL="${ROUTER_URL:-https://router.tryopencomputer.com}"
# The probe runs claude-opus-5 first and falls back to gpt-5.6-sol ONLY when
# the primary is out of capacity.
#
# Why a chain rather than just picking one: opus-5 is the model that actually
# matters, so the probe should exercise it -- but the anthropic pool holds
# only TWO accounts, and on 2026-09-04 both sat inside the same rate-limit
# window (reset 09:30) and the probe came back "All available accounts
# exhausted" on a perfectly healthy deploy. With a pool of two, one window
# takes out the whole platform. openai has SEVEN accounts, one of them
# limited, so it is the sturdier floor to fall back to.
#
# The fallback is gated on the failure being about CAPACITY (see
# _is_capacity_failure). A broken gateway must never be retried into a green
# probe.
#
# Effort values accepted by this gateway
# (backend/internal/service/gateway_request.go): OpenAI models take
# low / medium / high / xhigh; Claude models take low / medium / high / max.
# "extrahigh", "max" and "ultracode" are aliases onto the top tier.
INFERNO_PROBE_MODEL="${INFERNO_PROBE_MODEL:-claude-opus-5}"
INFERNO_PROBE_EFFORT="${INFERNO_PROBE_EFFORT:-low}"
INFERNO_PROBE_FALLBACK_MODEL="${INFERNO_PROBE_FALLBACK_MODEL:-gpt-5.6-sol}"
INFERNO_PROBE_FALLBACK_EFFORT="${INFERNO_PROBE_FALLBACK_EFFORT:-high}"
PROBE_MAX_TOKENS="${PROBE_MAX_TOKENS:-20}"
CURL_TIMEOUT="${CURL_TIMEOUT:-15}"

# Fallback path to fetch the probe key from the instance if it wasn't handed
# to us. Only used if INFERNO_API_KEY is unset. The key is never echoed,
# logged, or written anywhere by this script -- it only ever flows into the
# Authorization header of the probe request below.
OC_INTERNAL_HOST="${OC_INTERNAL_HOST:-oc-internal}"
OC_INTERNAL_USER="${OC_INTERNAL_USER:-architsakri}"
INSTANCE_IP="${INFERNO_INSTANCE_IP:-3.82.43.139}"
INSTANCE_SSH_KEY="${INSTANCE_SSH_KEY:-~/.ssh/oc-router-key.pem}"

# ---------------------------------------------------------------- key fetch
#
# Deliberately does NOT print the key. If this path is ever used, know that
# there is currently no known-good default -- config.yaml's default.api_key
# (the bootstrap DEFAULT_API_KEY) was tested live on 2026-09-04 and returned
# INVALID_API_KEY against /v1/models. This fallback exists for whichever key
# turns out to be the right one; it is not proven to find one today. See the
# Gaps section of SKILL.md.
#
_fetch_key_from_instance() {
  local remote='sudo python3 -c "import yaml; print(yaml.safe_load(open(\"/opt/inferno/data/config.yaml\"))[\"default\"][\"api_key\"])"'
  local b64
  b64="$(printf '%s' "$remote" | base64 | tr -d '\n')"
  local inner="ssh -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -i ${INSTANCE_SSH_KEY} ec2-user@${INSTANCE_IP} \"echo ${b64} | base64 -d | bash -s\""
  local ib64
  ib64="$(printf '%s' "$inner" | base64 | tr -d '\n')"
  ssh -o ConnectTimeout=15 "root@${OC_INTERNAL_HOST}" \
    "su - ${OC_INTERNAL_USER} -c 'echo ${ib64} | base64 -d | bash -s'" 2>/dev/null
}

_api_key() {
  if [ -n "${INFERNO_API_KEY:-}" ]; then
    printf '%s' "$INFERNO_API_KEY"
    return 0
  fi
  _fetch_key_from_instance
}

# ------------------------------------------------------------------- checks
#
# Each check prints "PASS  <name>  <detail>" or "FAIL  <name>  <detail>" and
# returns 0/1. main() aggregates. Nothing here is quiet on failure.

# Check 1 (the one that matters): a real completion, with real content.
# This is probe() -- the same function redeploy.sh calls to gate a deploy and
# to confirm a rollback actually restored service. Do not duplicate this
# logic anywhere else; change it here and both callers pick it up.
# _one_completion <model> <effort> -- returns 0 and echoes the content on
# success; echoes the raw response and returns 1 otherwise.
_one_completion() {
  local model="$1" effort="$2" key body resp content
  key="$(_api_key)"
  [ -n "$key" ] || { echo "no API key available (set INFERNO_API_KEY)"; return 1; }

  body=$(printf '{"model":"%s","messages":[{"role":"user","content":"Reply with exactly one word: OK"}],"max_tokens":%s,"reasoning_effort":"%s"}' \
    "$model" "$PROBE_MAX_TOKENS" "$effort")

  resp="$(curl -sS -m "$CURL_TIMEOUT" -X POST "${INFERENCE_URL}/v1/chat/completions" \
    -H "Authorization: Bearer ${key}" \
    -H "Content-Type: application/json" \
    --data "$body" 2>&1)"

  # Pull the completion content out without a JSON library dependency: look
  # for a non-empty "content" field anywhere in the response.
  content="$(printf '%s' "$resp" | grep -o '"content":"[^"]\{1,\}"' | head -1)"
  if [ -n "$content" ]; then
    printf '%s' "$content"
    return 0
  fi
  printf '%s' "${resp:0:300}"
  return 1
}

# Is this failure about CAPACITY rather than about the gateway being broken?
# Only these fall through to the fallback model. Anything else -- a 500, an
# auth error, a malformed response -- is a real failure and must stay one:
# retrying it on another platform would turn a broken gateway into a green
# probe, which is precisely the class of lie this whole script exists to stop.
_is_capacity_failure() {
  printf '%s' "$1" | grep -qiE 'accounts exhausted|rate.?limit|quota|too many requests|429|529|overloaded'
}

probe() {
  local out fb_out
  if out="$(_one_completion "$INFERNO_PROBE_MODEL" "$INFERNO_PROBE_EFFORT")"; then
    echo "PASS  completion  got content ($out) via ${INFERNO_PROBE_MODEL}"
    return 0
  fi

  # The primary platform is out of capacity, not broken. With only two
  # anthropic accounts a single rate-limit window takes the whole platform
  # out, and on 2026-09-04 exactly that made a healthy deploy look failed.
  # Fall back to the deeper openai pool rather than call the deploy bad.
  if [ -n "$INFERNO_PROBE_FALLBACK_MODEL" ] && _is_capacity_failure "$out"; then
    echo "NOTE  completion  ${INFERNO_PROBE_MODEL} is out of capacity, trying ${INFERNO_PROBE_FALLBACK_MODEL}: ${out:0:120}"
    if fb_out="$(_one_completion "$INFERNO_PROBE_FALLBACK_MODEL" "$INFERNO_PROBE_FALLBACK_EFFORT")"; then
      echo "PASS  completion  got content ($fb_out) via ${INFERNO_PROBE_FALLBACK_MODEL} (primary was rate-limited)"
      return 0
    fi
    echo "FAIL  completion  both ${INFERNO_PROBE_MODEL} and ${INFERNO_PROBE_FALLBACK_MODEL} failed. Fallback said: ${fb_out:0:200}"
    return 1
  fi

  echo "FAIL  completion  no content in response: ${out}"
  return 1
}

# Check 2: the SPA is served, not just a health endpoint.
check_spa() {
  local status body
  body="$(curl -sS -m "$CURL_TIMEOUT" -D /tmp/verify-spa-headers.$$ "${INFERENCE_URL}/" 2>&1)"
  status="$(head -1 /tmp/verify-spa-headers.$$ 2>/dev/null | tr -d '\r')"
  rm -f /tmp/verify-spa-headers.$$

  if printf '%s' "$status" | grep -q " 200 " \
     && printf '%s' "$body" | grep -qi "inferno"; then
    echo "PASS  spa  ${status}"
    return 0
  fi
  echo "FAIL  spa  status='${status}' body_snippet='${body:0:150}'"
  return 1
}

# Check 3: an anonymous protected call is rejected, not silently 500'd or
# silently allowed. 401/API_KEY_REQUIRED is correct; 500 is exactly the kind
# of failure that looked fine everywhere except here on 2026-09-03.
check_anon_rejected() {
  local status resp
  resp="$(curl -sS -m "$CURL_TIMEOUT" -o /tmp/verify-anon.$$ -w '%{http_code}' \
    -X POST "${INFERENCE_URL}/v1/chat/completions" \
    -H "Content-Type: application/json" --data '{}' 2>&1)"
  status="$resp"
  body="$(cat /tmp/verify-anon.$$ 2>/dev/null)"
  rm -f /tmp/verify-anon.$$

  if [ "$status" = "401" ]; then
    echo "PASS  anon-rejected  401 ${body:0:150}"
    return 0
  fi
  echo "FAIL  anon-rejected  expected 401, got ${status}: ${body:0:150}"
  return 1
}

# Check 4: the neighbour. This redeploy never touches oc-router directly, but
# every redeploy checks it anyway -- shared account, shared Cloudflare zone,
# and a typo'd hostname have each been the kind of thing that silently
# affects a neighbour before. Public HTTPS only, same as any customer.
check_router() {
  local status
  status="$(curl -sS -m "$CURL_TIMEOUT" -o /dev/null -w '%{http_code}' "${ROUTER_URL}/" 2>&1)"
  if [ "$status" = "200" ]; then
    echo "PASS  router-neighbour  ${ROUTER_URL} -> ${status}"
    return 0
  fi
  echo "FAIL  router-neighbour  ${ROUTER_URL} -> ${status} (neighbour may be disturbed)"
  return 1
}

# ------------------------------------------------------------------- doctor
#
# Which LAYER is broken? probe() answers "does a real client get a completion",
# which is the right question for "did the deploy work" and the wrong one for
# "what do I do about it". A failed completion has at least five causes and
# only one of them is fixed by rolling back the build:
#
#   container not running      -> restart it
#   app not answering in-box   -> the build really is bad -> roll back
#   tunnel down                -> restart cloudflared; the build is fine
#   app fine, provider failing -> NOTHING to fix here; rolling back is wrong
#   everything up              -> transient; retry
#
# The discriminator that matters most is the fourth. If the SPA serves and an
# anonymous call is correctly rejected, the application is working -- the
# completion is failing somewhere upstream of us, in a provider account. A
# rollback there swaps a good build for an older one and fixes nothing, while
# looking like it addressed the problem.
#
# PRESUPPOSES A FAILED probe(). It does not re-test the completion -- it works
# out which layer beneath the completion is broken. Called on a healthy system
# it therefore returns "provider-failing", which is correct in context (every
# layer we can see is fine, so the fault is above them) and meaningless out of
# it. Do not use it as a health check; that is what main() is for.
#
# Echoes one token on stdout. Requires run_instance() from redeploy.sh, so it
# is only usable when sourced from there; standalone verify.sh does not call
# it, and says so rather than pretending to diagnose.
doctor() {
  if ! declare -F run_instance >/dev/null 2>&1; then
    echo "doctor-unavailable"
    return 0
  fi

  local state rc local_health tunnel spa_code anon_code

  # "Cannot reach the box" and "the container is down" are different problems
  # with different responses, and they look identical if you only inspect the
  # output. An ssh failure returns an empty string, which read as
  # container-down and would send the ladder off restarting services it has no
  # connection to. Check the exit status, not just the value.
  state="$(run_instance "docker inspect inferno --format '{{.State.Status}}'" 2>/dev/null)"; rc=$?
  state="$(printf '%s' "$state" | tr -d '[:space:]')"
  if [ "$rc" != 0 ]; then
    echo "unreachable"
    return 0
  fi
  if [ "$state" != "running" ]; then
    echo "container-down"
    return 0
  fi

  # Inside the box, bypassing the tunnel entirely. This is what separates "our
  # app is broken" from "the path to our app is broken".
  local_health="$(run_instance "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health" 2>/dev/null | tr -d '[:space:]')"
  tunnel="$(run_instance "systemctl is-active cloudflared" 2>/dev/null | tr -d '[:space:]')"

  if [ "$local_health" != "200" ]; then
    echo "app-down-in-box"
    return 0
  fi

  # App answers locally. Anything still failing is between us and the client.
  if [ "$tunnel" != "active" ]; then
    echo "tunnel-down"
    return 0
  fi

  spa_code="$(curl -sS -m "$CURL_TIMEOUT" -o /dev/null -w '%{http_code}' "${INFERENCE_URL}/" 2>/dev/null)"
  if [ "$spa_code" != "200" ]; then
    echo "tunnel-down"
    return 0
  fi

  anon_code="$(curl -sS -m "$CURL_TIMEOUT" -o /dev/null -w '%{http_code}' \
    -X POST "${INFERENCE_URL}/v1/chat/completions" \
    -H 'Content-Type: application/json' --data '{}' 2>/dev/null)"
  if [ "$anon_code" = "401" ]; then
    # Serving, routing and auth all work. The completion is failing upstream.
    echo "provider-failing"
    return 0
  fi

  echo "app-degraded"
}

# --------------------------------------------------------------------- main
main() {
  local rc=0
  echo "=== verify: ${INFERENCE_URL} ==="
  probe               || rc=1
  check_spa           || rc=1
  check_anon_rejected || rc=1
  check_router        || rc=1

  if [ "$rc" = 0 ]; then
    echo "VERIFY_STATUS=ok"
  else
    echo "VERIFY_STATUS=FAILED"
  fi
  return "$rc"
}

# Only run main when executed directly. When sourced (by redeploy.sh), this
# file just defines probe()/check_spa()/check_anon_rejected()/check_router()
# and the caller decides what to run and when.
if [ "${BASH_SOURCE[0]:-}" = "${0}" ]; then
  main "$@"
  exit $?
fi
