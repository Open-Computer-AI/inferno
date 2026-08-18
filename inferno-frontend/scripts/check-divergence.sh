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
#
# A line may be a glob (`case` patterns, so `*` also crosses `/`). Use one ONLY
# for files that are unambiguously ours by name -- a razorpay_* file cannot be
# an upstream file we drifted from, so a new one appearing is not the accident
# this gate exists to catch. NEVER glob a directory that also holds upstream
# files: `service/payment_*` would swallow upstream's own payment files and this
# gate would stop seeing the modified-file conflict seams, which are the
# expensive half of a reconcile. When in doubt, list the exact path.
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
# D5 -- Razorpay payment provider. Files that are ours by name may glob;
# the upstream files we MODIFIED to add the provider are listed exactly,
# because those are the seams that will actually conflict on a reconcile.
backend/internal/payment/provider/razorpay*
backend/internal/payment/razorpay_events.go
backend/internal/service/payment_razorpay_*
backend/internal/service/payment_client_verification.go
backend/internal/payment/provider/factory.go
backend/internal/payment/types.go
backend/internal/handler/payment_handler.go
backend/internal/handler/payment_webhook_handler.go
backend/internal/server/routes/payment.go
backend/internal/service/payment_service.go
backend/internal/service/payment_order.go
backend/internal/service/payment_currency.go
backend/internal/service/payment_fulfillment.go
backend/internal/service/payment_config_providers.go
backend/internal/service/payment_order_provider_snapshot.go
backend/internal/service/payment_webhook_provider.go
# D6 -- NODE_OPTIONS heap bump for the larger Inferno bundle
deploy/Dockerfile
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
# `case` does the matching so a DECLARED line may be an exact path or a glob;
# with no metacharacters a glob match IS an exact match, so both work uniformly.
undeclared=""
if [ "$n_changed" -gt 0 ]; then
  undeclared="$(printf '%s\n' "$CHANGED" | grep . | while IFS= read -r f; do
    hit=0
    while IFS= read -r d; do
      [ -n "$d" ] || continue
      case "$f" in ($d) hit=1; break ;; esac
    done <<INNER
$(declared_list)
INNER
    [ "$hit" -eq 0 ] && printf '%s\n' "$f"
  done)"
fi

# Stale: declared but nothing matches it any more. Not fatal, but the ledger
# should shrink when a divergence is retired or a glob stops covering anything.
stale="$(declared_list | while IFS= read -r d; do
  [ -n "$d" ] || continue
  hit=0
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    case "$f" in ($d) hit=1; break ;; esac
  done <<INNER
$CHANGED
INNER
  [ "$hit" -eq 0 ] && printf '%s\n' "$d"
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
