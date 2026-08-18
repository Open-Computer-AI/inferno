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
docs/superpowers/specs/2026-08-18-authorization-code-pkce-design.md
docs/superpowers/plans/2026-08-18-authorization-code-pkce.md
docs/superpowers/specs/2026-08-19-oauth-bearer-inference-design.md
docs/superpowers/plans/2026-08-19-oauth-bearer-inference.md
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
# D5 -- OAuth AS (Task 5: token endpoint -- device_code and refresh_token grants)
backend/internal/service/oauth_token_service.go
backend/internal/service/oauth_token_service_test.go
backend/internal/service/refresh_token_cache.go
backend/internal/service/wire.go
backend/cmd/server/wire_gen.go
backend/internal/handler/oauth_handler.go
backend/internal/server/routes/oauth.go
# D5 -- OAuth AS (Task 5 review fixes: atomic refresh-token rotation,
# account re-validation on refresh, 500-on-internal-error, wire contract tests)
backend/internal/repository/refresh_token_cache.go
backend/internal/repository/refresh_token_cache_test.go
backend/internal/handler/oauth_handler_test.go
backend/internal/server/routes/oauth_token_route_test.go
# D5 -- OAuth AS (Task 6: scope-enforcement middleware + GET /api/oauth/account)
# oauth_handler.go, oauth_handler_test.go, routes/oauth.go, and router.go are
# already declared above (Tasks 2/3/5) -- not re-listed here, same files.
backend/internal/server/middleware/oauth_scope.go
backend/internal/server/middleware/oauth_scope_test.go
backend/internal/server/routes/oauth_account_route_test.go
# D5 -- OAuth AS (Task 7: device approval screen -- ApproveDevice/DenyDevice)
# oauth_handler.go and routes/oauth.go are already declared above (Tasks
# 2/5) -- not re-listed here, same files. inferno-frontend/ is out of this
# script's diff scope entirely (CHANGED below is scoped to backend frontend
# deploy docs from the repo root; frontend there is the pristine upstream
# mirror, not inferno-frontend/), so the Vue/TS files this task added
# (views/oauth/DeviceApprovalView.vue + its spec, api/oauth.ts,
# router/index.ts, i18n/locales/en+zh/misc.ts) never appear in this gate
# and are not listed here -- see GOAL.md's D5 row for the full file list.
backend/internal/handler/oauth_device_decision_test.go
# D5 -- OAuth AS (Task 8: end-to-end conformance against the real hermes
# client -- seeds the first-party hermes-cli client, corrects the scope
# vocabulary to inference:invoke, and adds the conformance runbook). Test
# files whose scope literals changed from inference to inference:invoke
# (oauth_handler_test.go, oauth_device_service_test.go,
# oauth_token_service_test.go, oauth_scope_test.go,
# refresh_token_cache_test.go) and the design doc
# (docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md)
# are already declared above (Tasks 3/4/5/6 and D4) -- not re-listed here,
# same files.
backend/migrations/905_hermes_cli_first_party_client.sql
backend/scripts/oauth-conformance.md
# D5 -- OAuth AS (final whole-branch fix wave: personal org on every login,
# TokenVersion on OAuth refresh tokens, oauth_client.status enforcement,
# device-authorization terminal states, the scope allowlist + consent screen,
# and the /api/oauth/account contract reshape). Every other file this wave
# touched -- auth_service.go, oauth_{client,device,token}_service{,_test}.go,
# handler/oauth_handler{,_test}.go, oauth_device_decision_test.go,
# handler/wire.go, server/routes/oauth{,_account_route_test}.go,
# ent/schema/oauth_client.go and cmd/server/wire_gen.go -- is already declared
# above (Tasks 1-8), same files. The inferno-frontend Vue/TS side
# (views/oauth/DeviceApprovalView.vue + its spec, api/oauth.ts,
# components/layout/InterstitialState.vue, i18n/locales/en+zh/misc.ts) is out
# of this script's diff scope entirely -- see GOAL.md's D5 row for that list.
backend/internal/service/oauth_scope_vocabulary.go
backend/internal/service/auth_service_personal_org_login_test.go
# D5 -- OAuth AS (new plan, Task 1: RS256 signing key migration, kid-dispatched
# verification). oauth_signing_key.go, oauth_signing_key_test.go,
# oauth_scope.go, oauth_scope_test.go, and oauth_token_service.go are already
# declared above (Tasks 2/5/6) -- not re-listed here, same files.
backend/migrations/907_oauth_rs256_key.sql
# D5 -- OAuth AS (authorization_code + PKCE plan, Task 2: redirect_uri
# validation). oauth_client_service.go and oauth_handler.go are already
# declared above (Tasks 3/2) -- not re-listed here, same files.
backend/internal/service/oauth_redirect_uri.go
backend/internal/service/oauth_redirect_uri_test.go
# D5 -- OAuth AS (authorization_code + PKCE plan, Task 3: authorization
# codes -- issue and redeem). oauth_handler.go, oauth_token_service.go, and
# oauth_scope_vocabulary.go are already declared above (Tasks 2/5/8) -- not
# re-listed here, same files. The shared ent codegen files this task's
# 'go generate ./ent' run also touched (client.go, ent.go, hook/hook.go,
# intercept/intercept.go, migrate/schema.go, mutation.go,
# predicate/predicate.go, runtime/runtime.go, tx.go) are already declared
# above (Task 3's oauth_client registration) -- not re-listed here, same
# files.
backend/ent/schema/oauth_authorization_code.go
backend/ent/oauthauthorizationcode.go
backend/ent/oauthauthorizationcode/oauthauthorizationcode.go
backend/ent/oauthauthorizationcode/where.go
backend/ent/oauthauthorizationcode_create.go
backend/ent/oauthauthorizationcode_delete.go
backend/ent/oauthauthorizationcode_query.go
backend/ent/oauthauthorizationcode_update.go
backend/migrations/906_oauth_authorization_code.sql
backend/migrations/908_oauth_authorization_code_client_user_idx.sql
backend/internal/service/oauth_authorize_service.go
backend/internal/service/oauth_authorize_service_test.go
# D5 -- OAuth AS (authorization_code + PKCE plan, Task 4: GET/POST
# /oauth/authorize and the consent screen). oauth_handler.go,
# oauth_authorize_service.go (ValidCodeChallengeShape exported, no behavior
# change), handler/wire.go, service/wire.go, server/routes/oauth.go,
# server/router.go, and cmd/server/wire_gen.go are already declared above
# (Tasks 1-3 / org tenancy) -- not re-listed here, same files. The frontend
# half of this task (views/oauth/AuthorizeConsentView.vue + its spec,
# api/oauth.ts, router/index.ts, i18n/locales/en+zh/misc.ts) is out of this
# script's diff scope entirely, same as Task 7's device screen -- see
# GOAL.md's D5 row for that list.
backend/internal/handler/oauth_authorize_handler.go
backend/internal/handler/oauth_authorize_handler_test.go
backend/internal/web/embed_on.go
# D5 -- OAuth AS (authorization_code + PKCE plan, Task 4 fix round 1: F3/F4).
# embed_test.go was already modified by da357017 (a new
# TestEmbeddedFrontendBypassesOAuthAuthorize test) but was never added here or
# to GOAL.md -- review caught this exact gate failing on its own introduced
# file. F4's fix also moved shouldBypassEmbeddedFrontend and its tests out of
# the two embed-tagged files into two new untagged ones, so the allowlist
# assertion actually runs under a plain go test ./... instead of requiring
# -tags embed plus a gitignored dist/index.html that CI without a frontend
# build never has.
backend/internal/web/embed_test.go
backend/internal/web/bypass.go
backend/internal/web/bypass_test.go
# D5 -- OAuth AS (authorization_code + PKCE plan, Task 4 fix round 2:
# NEW-4, NEW-8, from task-4-rereview.md). oauth_authorize_handler.go,
# oauth_authorize_handler_test.go, oauth_authorize_service.go, server/router.go,
# server/routes/oauth.go, and handler/oauth_handler.go are already declared
# above (Tasks 1-4) -- not re-listed here, same files. NEW-4 gave
# /oauth/authorize its own rate-limit bucket key (PublicIPScoped, alongside the
# existing PublicIP) instead of sharing one counter with Model Plaza's public
# group; NEW-8 removed the duplicate package doc comment embed_off.go picked up
# once bypass.go (fix round 1) also carried one.
backend/internal/server/middleware/panel_rate_limit.go
backend/internal/server/middleware/panel_rate_limit_test.go
backend/internal/web/embed_off.go
# D5 -- OAuth AS (oauth-bearer-inference plan, Task 2: api_keys.oauth_client_id
# and its partial unique index -- the backing-row column and constraint that
# let an OAuth access token resolve to a real api_keys row to meter against).
# ent/schema/api_key.go's 3-line field addition regenerated 8 shared ent
# files (apikey.go, apikey/{apikey,where}.go, apikey_{create,update}.go,
# migrate/schema.go, mutation.go, runtime/runtime.go); apikey_query.go and
# apikey_delete.go were untouched by this run.
backend/ent/schema/api_key.go
backend/ent/apikey.go
backend/ent/apikey/apikey.go
backend/ent/apikey/where.go
backend/ent/apikey_create.go
backend/ent/apikey_update.go
backend/ent/migrate/schema.go
backend/ent/mutation.go
backend/ent/runtime/runtime.go
backend/migrations/909_api_key_oauth_client_id.sql
backend/internal/repository/api_key_oauth_client_id_integration_test.go
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
