#!/usr/bin/env bash
#
# Gate 5 — the divergence check.
#
# Inferno is a fork of Wei-Shaw/sub2api that is becoming its own product, so
# backend divergence is expected. What is NOT expected is divergence nobody
# declared: 84a3c4ac was a commit whose subject said "feat(ui):" and which
# quietly regenerated ent and added a migration. That is the failure this
# catches.
#
# Every file that differs from the upstream base under backend/, frontend/,
# deploy/ or docs/ must be listed in DECLARED below AND in GOAL.md's divergence
# ledger. The ledger is the explanation; this is the enforcement.
#
# WHY THE BASE AND NOT upstream/main:
#   Measuring against a freshly fetched upstream/main counts upstream's own
#   movement as our drift. On 2026-08-15 the same tree read 19 files against the
#   base and 246 against fresh upstream, 227 of which were upstream moving on.
#   merge-base is the only ref that answers "what have WE changed".
#
# PORTABILITY: macOS ships bash 3.2, which has no `mapfile` and no associative
# arrays. The first version of this script used `mapfile` and therefore read
# zero files and exited 0 -- a gate that passes by failing to run is worse than
# no gate, so this one is plain POSIX-ish and asserts that it actually ran.
#
set -uo pipefail

cd "$(dirname "$0")/../.."

if ! git rev-parse --verify upstream/main >/dev/null 2>&1; then
  echo "divergence: FAIL — no upstream/main ref. Run: git fetch upstream" >&2
  exit 2
fi

BASE="$(git merge-base HEAD upstream/main)" || {
  echo "divergence: FAIL — could not compute merge-base" >&2; exit 2; }

# Declared divergence. Keep in lockstep with GOAL.md's ledger table.
# One path per line. Blank lines and #-comments are ignored.
DECLARED="
# D1 -- avatar_seed on users
backend/ent/schema/user.go
backend/ent/migrate/schema.go
backend/ent/mutation.go
backend/ent/runtime/runtime.go
backend/ent/user.go
backend/ent/user/user.go
backend/ent/user/where.go
backend/ent/user_create.go
backend/ent/user_update.go
backend/internal/handler/dto/types.go
backend/internal/handler/dto/mappers.go
backend/internal/handler/user_handler.go
backend/internal/repository/api_key_repo.go
backend/internal/repository/user_repo.go
backend/internal/service/user.go
backend/internal/service/user_service.go
backend/migrations/900_add_user_avatar_seed.sql
# D2 -- English legal-document defaults
backend/internal/service/setting_public.go
backend/internal/server/api_contract_test.go
# D4 -- OpenComputer portal design specs + plans
docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md
docs/superpowers/plans/2026-08-17-inferno-oauth-authorization-server.md
# D5 -- org tenancy (Task 1: org + org_member tables, personal org on signup)
backend/ent/schema/org.go
backend/ent/schema/org_member.go
backend/ent/org.go
backend/ent/org/org.go
backend/ent/org/where.go
backend/ent/org_create.go
backend/ent/org_delete.go
backend/ent/org_query.go
backend/ent/org_update.go
backend/ent/orgmember.go
backend/ent/orgmember/orgmember.go
backend/ent/orgmember/where.go
backend/ent/orgmember_create.go
backend/ent/orgmember_delete.go
backend/ent/orgmember_query.go
backend/ent/orgmember_update.go
backend/ent/client.go
backend/ent/ent.go
backend/ent/group.go
backend/ent/hook/hook.go
backend/ent/intercept/intercept.go
backend/ent/migrate/schema.go
backend/ent/mutation.go
backend/ent/predicate/predicate.go
backend/ent/runtime/runtime.go
backend/ent/tx.go
backend/migrations/901_org_and_members.sql
backend/migrations/904_org_personal_user_id.sql
backend/internal/service/org_service.go
backend/internal/service/org_service_test.go
backend/internal/service/auth_service.go
backend/internal/service/wire.go
backend/cmd/server/wire_gen.go
backend/cmd/jwtgen/main.go
backend/internal/handler/auth_oauth_pending_flow_test.go
# D5 -- OAuth AS (Task 2: ES256 signing key + JWKS endpoint)
backend/internal/service/oauth_signing_key.go
backend/internal/service/oauth_signing_key_test.go
backend/internal/handler/oauth_handler.go
backend/internal/handler/handler.go
backend/internal/handler/wire.go
backend/internal/server/routes/oauth.go
backend/internal/server/routes/common.go
backend/internal/server/router.go
backend/internal/handler/passkey_handler_test.go
backend/internal/handler/auth_oauth_captcha_start_test.go
backend/internal/handler/auth_wechat_oauth_test.go
backend/internal/handler/auth_session_revocation_test.go
backend/internal/handler/user_handler_test.go
backend/internal/server/middleware/jwt_auth_test.go
backend/internal/server/middleware/optional_jwt_auth_test.go
backend/internal/server/middleware/admin_auth_test.go
backend/internal/service/auth_service_register_test.go
backend/internal/service/auth_service_identity_sync_test.go
backend/internal/service/auth_service_email_bind_test.go
backend/internal/service/auth_service_captcha_test.go
backend/internal/service/auth_email_oauth_auto_test.go
backend/internal/service/aliyun_captcha_service_test.go
backend/internal/service/auth_oauth_email_flow_test.go
backend/internal/service/auth_service_turnstile_register_test.go
# D5 -- OAuth AS (Task 3: oauth_client registry + self-hosted client registration)
backend/ent/schema/oauth_client.go
backend/ent/oauthclient.go
backend/ent/oauthclient/oauthclient.go
backend/ent/oauthclient/where.go
backend/ent/oauthclient_create.go
backend/ent/oauthclient_delete.go
backend/ent/oauthclient_query.go
backend/ent/oauthclient_update.go
backend/ent/client.go
backend/ent/ent.go
backend/ent/hook/hook.go
backend/ent/intercept/intercept.go
backend/ent/migrate/schema.go
backend/ent/mutation.go
backend/ent/predicate/predicate.go
backend/ent/runtime/runtime.go
backend/ent/tx.go
backend/migrations/902_oauth_client.sql
backend/internal/service/oauth_client_service.go
backend/internal/service/oauth_client_service_test.go
backend/internal/service/wire.go
backend/internal/handler/dto/oauth.go
backend/cmd/server/wire_gen.go
# D5 -- OAuth AS (Task 4: RFC 8628 device authorization request)
backend/ent/schema/oauth_device_authorization.go
backend/ent/oauthdeviceauthorization.go
backend/ent/oauthdeviceauthorization/oauthdeviceauthorization.go
backend/ent/oauthdeviceauthorization/where.go
backend/ent/oauthdeviceauthorization_create.go
backend/ent/oauthdeviceauthorization_delete.go
backend/ent/oauthdeviceauthorization_query.go
backend/ent/oauthdeviceauthorization_update.go
backend/migrations/903_oauth_device_authorization.sql
backend/internal/service/oauth_device_service.go
backend/internal/service/oauth_device_service_test.go
"

declared_list() { printf '%s\n' "$DECLARED" | grep -v '^[[:space:]]*#' | grep -v '^[[:space:]]*$'; }

CHANGED="$(git diff --name-only "$BASE..HEAD" -- backend frontend deploy docs)"
git_status=$?
if [ $git_status -ne 0 ]; then
  echo "divergence: FAIL — git diff errored ($git_status)" >&2
  exit 2
fi

n_changed=$(printf '%s' "$CHANGED" | grep -c . || true)
n_declared=$(declared_list | grep -c . || true)

echo "divergence: base $(git rev-parse --short "$BASE") · ${n_changed} file(s) differ · ${n_declared} declared"

# Undeclared: differs but is not in the ledger. This is the failure condition.
undeclared=""
if [ "$n_changed" -gt 0 ]; then
  undeclared="$(printf '%s\n' "$CHANGED" | grep . | while IFS= read -r f; do
    declared_list | grep -qxF "$f" || printf '%s\n' "$f"
  done)"
fi

# Stale: declared but no longer differs. Not fatal, but the ledger should shrink.
stale="$(declared_list | while IFS= read -r d; do
  printf '%s\n' "$CHANGED" | grep -qxF "$d" || printf '%s\n' "$d"
done)"

if [ -n "$stale" ]; then
  echo
  echo "  note: declared file(s) that no longer differ — prune the ledger:"
  printf '%s\n' "$stale" | sed 's/^/    /'
fi

if [ -n "$undeclared" ]; then
  echo
  echo "  UNDECLARED DIVERGENCE:"
  printf '%s\n' "$undeclared" | sed 's/^/    /'
  echo
  echo "  Either revert these, or add them to DECLARED here and to GOAL.md's"
  echo "  ledger with a reason and a re-apply strategy. Do not add a row just to"
  echo "  make this pass — every entry is a permanent tax on future reconciles."
  exit 1
fi

echo "  all divergence declared"
exit 0
