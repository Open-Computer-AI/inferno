#!/usr/bin/env bash
# Monitor source for the sub2api upstream watch.
#
# Hermes hashes this output each tick. Identical bytes => the agent does not run
# at all, which is the point: upstream ships nothing on most days and an
# expensive model should not wake up to say so.
#
# Output must therefore be STABLE — no timestamps, no durations, no ordering
# that can vary. Only the set of unported commits and their routes.
set -uo pipefail
cd "$HOME/OpenComputerV2/inferno" || exit 0
git fetch upstream --quiet 2>/dev/null
NODE_OPTIONS= node inferno-frontend/scripts/upstream-daily.mjs --no-fetch 2>/dev/null \
  | grep -E "^  (MERGE|VERBATIM|NEW|REBUILD) |^  upstream/main is|^  [0-9]+ upstream commit" \
  | sort
