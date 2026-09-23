#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LEDGER="$ROOT/docs/superpowers/analysis/RECONCILIATION.md"
BASELINE_DIR="$ROOT/../inferno-local"

if [[ ! -f "$LEDGER" ]]; then
  echo "missing reconciliation ledger: $LEDGER" >&2
  exit 2
fi
if [[ ! -d "$BASELINE_DIR/.git" && ! -f "$BASELINE_DIR/.git" ]]; then
  echo "protected baseline checkout not found: $BASELINE_DIR" >&2
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
        gsub(/[[:space:]]/, "", value)
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

candidate_branch="$(git -C "$ROOT" branch --show-current)"
implementation_commit="$(ledger_value 'Candidate implementation commit')"
ledger_parent="$(git -C "$ROOT" rev-parse HEAD^)"
baseline_head="$(git -C "$BASELINE_DIR" rev-parse HEAD)"
upstream_local="$(git -C "$ROOT" rev-parse --verify refs/remotes/upstream/main)"
upstream_remote="$(git -C "$ROOT" ls-remote upstream refs/heads/main | awk 'NR == 1 { print $1 }')"
merge_base="$(git -C "$ROOT" merge-base "$implementation_commit" upstream/main)"

fail_if_different "candidate branch" "$(ledger_value 'Candidate branch')" "$candidate_branch"
fail_if_different "ledger commit parent / candidate implementation" "$implementation_commit" "$ledger_parent"
fail_if_different "protected baseline HEAD" "$(ledger_value 'Protected baseline HEAD')" "$baseline_head"
fail_if_different "merge base" "$(ledger_value 'Merge base')" "$merge_base"
fail_if_different "local upstream/main" "$(ledger_value 'Local upstream/main SHA')" "$upstream_local"
fail_if_different "live upstream/main" "$(ledger_value 'GitHub refs/heads/main SHA')" "$upstream_remote"

ledger_commit_files="$(git -C "$ROOT" diff-tree --no-commit-id --name-only -r HEAD)"
if [[ "$ledger_commit_files" != "docs/superpowers/analysis/RECONCILIATION.md" ]]; then
  printf 'STALE: expected the final commit to update only the live ledger; found:\n%s\n' "$ledger_commit_files" >&2
  exit 1
fi
printf 'PASS: final commit updates only RECONCILIATION.md\n'

candidate_status="$(git -C "$ROOT" status --porcelain=v1 --untracked-files=all)"
if [[ -n "$candidate_status" ]]; then
  printf 'DIRTY: candidate tracked and non-ignored untracked Git status is not clean:\n%s\n' "$candidate_status" >&2
  exit 1
fi
printf 'PASS: candidate tracked and non-ignored untracked Git status is clean\n'

if ! git -C "$BASELINE_DIR" diff --quiet HEAD -- ||
  ! git -C "$BASELINE_DIR" diff --cached --quiet; then
  echo "DIRTY: protected baseline has tracked changes" >&2
  exit 1
fi
printf 'PASS: protected baseline tracked files are unchanged\n'

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
