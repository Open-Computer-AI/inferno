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
INFERNO_PROBE_MODEL="${INFERNO_PROBE_MODEL:-}"
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
probe() {
  local key model body resp content
  key="$(_api_key)"
  if [ -z "$key" ]; then
    echo "FAIL  completion  no API key available (set INFERNO_API_KEY)"
    return 1
  fi
  model="${INFERNO_PROBE_MODEL}"
  if [ -z "$model" ]; then
    echo "FAIL  completion  no model set (set INFERNO_PROBE_MODEL to a model on a live seeded provider account)"
    return 1
  fi

  body=$(printf '{"model":"%s","messages":[{"role":"user","content":"Reply with exactly one word: OK"}],"max_tokens":%s}' \
    "$model" "$PROBE_MAX_TOKENS")

  resp="$(curl -sS -m "$CURL_TIMEOUT" -X POST "${INFERENCE_URL}/v1/chat/completions" \
    -H "Authorization: Bearer ${key}" \
    -H "Content-Type: application/json" \
    --data "$body" 2>&1)"

  # Pull the completion content out without a JSON library dependency: look
  # for a non-empty "content" field anywhere in the response.
  content="$(printf '%s' "$resp" | grep -o '"content":"[^"]\{1,\}"' | head -1)"

  if [ -n "$content" ]; then
    echo "PASS  completion  got content ($content)"
    return 0
  fi
  echo "FAIL  completion  no content in response: ${resp:0:300}"
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
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  main "$@"
  exit $?
fi
