#!/usr/bin/env bash
#
# port-classify.sh — for every file that differs between the upstream mirror and
# our June tree, say whether a wholesale COPY is safe or a HAND-MERGE is required.
#
# WHY THIS EXISTS:
#   On 2026-08-11 four locale files were ported with a wholesale `cp`. It silently
#   reverted June i18n work: it dropped a key a converted component renders, and
#   reintroduced an em dash and an emoji that ground rules 2 and 8 had removed.
#   Nothing failed. The only tell was june-lint's converted FILE COUNT dropping
#   96 -> 92, because copying made the files byte-identical to the mirror again
#   and took them OUT of lint scope. A health signal that improves when work is
#   destroyed is the one to distrust.
#
# THE RULE, and it is mechanical:
#   If we have deliberately changed a file since the vendor point, it carries June
#   work and MUST be hand-merged. If we have not, our copy is upstream's old code
#   and a wholesale copy is safe.
#
#   Locale files are ALWAYS hand-merge regardless -- they accumulate June-only
#   keys that no diff against an untouched file would reveal.
#
# PORTABILITY: macOS bash 3.2. No mapfile, no associative arrays.
#
set -uo pipefail
cd "$(dirname "$0")/../.."

VB="$(git log --format=%H -1 --grep='vendor upstream frontend as the redesign target')"
if [ -z "$VB" ]; then
  echo "classify: FAIL — cannot find the vendor-point commit by subject." >&2
  echo "  It is located by SUBJECT, never by hash: rebases rewrote the hash." >&2
  exit 2
fi

git diff --name-only "$VB..HEAD" -- inferno-frontend \
  | sed 's|^inferno-frontend/||' | sort -u > /tmp/.pc_ours
diff -rq frontend/src inferno-frontend/src -x node_modules -x dist 2>/dev/null \
  | grep '^Files' | sed 's|Files frontend/||; s| and .*||' | sort -u > /tmp/.pc_differ

n_differ=$(grep -c . /tmp/.pc_differ || true)
n_ours=$(grep -c . /tmp/.pc_ours || true)
echo "classify: vendor point $(git rev-parse --short "$VB") · ${n_differ} file(s) differ from the mirror · ${n_ours} we changed"
echo

copy=0; merge=0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  case "$f" in
    *i18n/*) printf '  HAND-MERGE  %s   (locale: always, June-only keys)\n' "$f"; merge=$((merge+1)); continue ;;
  esac
  if grep -qxF "$f" /tmp/.pc_ours; then
    printf '  HAND-MERGE  %s\n' "$f"; merge=$((merge+1))
  else
    printf '  copy-safe   %s\n' "$f"; copy=$((copy+1))
  fi
done < /tmp/.pc_differ

echo
echo "  ${merge} require a hand merge · ${copy} are copy-safe"
echo
echo "  Before AND after any batch, record june-lint's converted FILE COUNT."
echo "  If it DROPS, a copy overwrote converted work. Revert, do not investigate."
