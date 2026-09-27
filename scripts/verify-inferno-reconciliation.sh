#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LEDGER="$ROOT/docs/superpowers/analysis/RECONCILIATION.md"

if [[ ! -f "$LEDGER" ]]; then
  echo "missing reconciliation ledger: $LEDGER" >&2
  exit 2
fi

ledger_value() {
  awk -F'|' -v key="$1" '
    {
      label = $2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", label)
      if (label == key) {
        value = $3
        gsub(sprintf("%c", 96), "", value)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        print value
        exit
      }
    }
  ' "$LEDGER"
}

fail_if_different() {
  local label="$1" expected="$2" actual="$3" shown_expected shown_actual
  shown_expected="$expected"
  shown_actual="$actual"
  if [[ -z "$expected" ]]; then shown_expected="<missing>"; fi
  if [[ -z "$actual" ]]; then shown_actual="<missing>"; fi
  if [[ -z "$expected" || "$expected" != "$actual" ]]; then
    printf 'STALE: %s\n  ledger: %s\n  actual: %s\n' "$label" "$shown_expected" "$shown_actual" >&2
    exit 1
  fi
  printf 'PASS: %s = %s\n' "$label" "$actual"
}

canonical_origin="$(git -C "$ROOT" remote get-url origin 2>/dev/null || true)"
candidate_branch="$(git -C "$ROOT" branch --show-current)"
local_main="$(git -C "$ROOT" rev-parse --verify refs/heads/main)"
implementation_commit="$(ledger_value 'Candidate implementation commit')"
protected_baseline="$(ledger_value 'Protected baseline commit')"
upstream_local="$(git -C "$ROOT" rev-parse --verify refs/remotes/upstream/main 2>/dev/null || true)"
fork_main_local="$(git -C "$ROOT" rev-parse --verify refs/remotes/origin/main 2>/dev/null || true)"
fork_main_remote="$(git -C "$ROOT" ls-remote origin refs/heads/main | awk 'NR == 1 { print $1 }')"
legacy_inferno_remote="$(git -C "$ROOT" ls-remote origin refs/heads/inferno | awk 'NR == 1 { print $1 }')"
upstream_remote="$(git -C "$ROOT" ls-remote upstream refs/heads/main | awk 'NR == 1 { print $1 }')"
merge_base="$(git -C "$ROOT" merge-base "$implementation_commit" "$upstream_remote" 2>/dev/null || true)"

fail_if_different "canonical repository origin" "$(ledger_value 'Canonical repository origin')" "$canonical_origin"
fail_if_different "candidate working branch" "$(ledger_value 'Candidate working branch')" "$candidate_branch"
fail_if_different "GitHub legacy inferno SHA" "$(ledger_value 'GitHub legacy inferno SHA')" "$legacy_inferno_remote"
fail_if_different "merge base" "$(ledger_value 'Merge base')" "$merge_base"
fail_if_different "local origin/main" "$fork_main_remote" "$fork_main_local"
fail_if_different "local upstream/main" "$(ledger_value 'Local upstream/main SHA')" "$upstream_local"
fail_if_different "live upstream/main" "$(ledger_value 'GitHub upstream/main SHA')" "$upstream_remote"

if [[ "$candidate_branch" != "main" || "$local_main" != "$(git -C "$ROOT" rev-parse HEAD)" ]]; then
  echo "STALE: run this verifier from the canonical main checkout" >&2
  exit 1
fi
printf 'PASS: HEAD is the canonical local main branch\n'

if ! git -C "$ROOT" cat-file -e "$protected_baseline^{commit}" 2>/dev/null; then
  printf 'STALE: protected baseline commit is absent: %s\n' "$protected_baseline" >&2
  exit 1
fi
if ! git -C "$ROOT" merge-base --is-ancestor "$protected_baseline" "$implementation_commit"; then
  printf 'STALE: protected baseline %s is not an ancestor of the implementation snapshot\n' "$protected_baseline" >&2
  exit 1
fi
printf 'PASS: protected baseline commit exists and remains an implementation ancestor\n'

if ! git -C "$ROOT" merge-base --is-ancestor "$implementation_commit" "$fork_main_remote"; then
  printf 'STALE: live GitHub main does not contain the implementation snapshot %s\n' "$implementation_commit" >&2
  exit 1
fi
printf 'PASS: live GitHub main contains the implementation snapshot\n'

if ! git -C "$ROOT" merge-base --is-ancestor "$implementation_commit" HEAD; then
  printf 'STALE: candidate implementation %s is not an ancestor of HEAD\n' "$implementation_commit" >&2
  exit 1
fi
printf 'PASS: candidate implementation is an ancestor of HEAD\n'

fork_source_changes="$(git -C "$ROOT" diff --name-only "$implementation_commit" "$fork_main_remote" -- . \
  ':(exclude)docs/superpowers/analysis/RECONCILIATION.md' \
  ':(exclude)scripts/verify-inferno-reconciliation.sh')"
if [[ -n "$fork_source_changes" ]]; then
  printf 'STALE: GitHub fork main differs from the implementation snapshot outside the ledger:\n%s\n' "$fork_source_changes" >&2
  exit 1
fi
printf 'PASS: GitHub fork main source files match the implementation snapshot\n'

if ! git -C "$ROOT" merge-base --is-ancestor "$fork_main_remote" "$local_main"; then
  printf 'STALE: local main is not a fast-forward descendant of live GitHub main\n' >&2
  exit 1
fi
printf 'PASS: local main contains the live GitHub main snapshot\n'

source_changes="$(git -C "$ROOT" diff --name-only "$implementation_commit" HEAD -- . \
  ':(exclude)docs/superpowers/analysis/RECONCILIATION.md' \
  ':(exclude)scripts/verify-inferno-reconciliation.sh')"
if [[ -n "$source_changes" ]]; then
  printf 'STALE: non-ledger/verifier files changed after the implementation snapshot:\n%s\n' "$source_changes" >&2
  exit 1
fi
printf 'PASS: source tree is unchanged after the implementation snapshot\n'

if ! git -C "$ROOT" diff --quiet "$upstream_local" HEAD -- frontend; then
  echo "STALE: standard frontend no longer exactly matches the recorded upstream snapshot" >&2
  exit 1
fi
printf 'PASS: standard frontend exactly matches upstream/main\n'

candidate_status="$(git -C "$ROOT" status --porcelain=v1 --untracked-files=all)"
if [[ -n "$candidate_status" ]]; then
  printf 'DIRTY: candidate tracked and non-ignored untracked Git status is not clean:\n%s\n' "$candidate_status" >&2
  exit 1
fi
printf 'PASS: candidate tracked and non-ignored untracked Git status is clean\n'

closed_rows="$(awk '
  /^### Closed former unresolved backend rows/ { in_rows = 1; next }
  /^### / { in_rows = 0 }
  in_rows && /\| CLOSED —/ { count++ }
  END { print count + 0 }
' "$LEDGER")"
if [[ "$closed_rows" != 9 ]]; then
  printf 'STALE: expected 9 closed former unresolved backend rows; found %s\n' "$closed_rows" >&2
  exit 1
fi
printf 'PASS: all %s former unresolved backend rows remain accounted for\n' "$closed_rows"
echo 'SNAPSHOT VERIFIED: this confirms the recorded commit/ref state, not future upstream changes or runtime behavior.'
