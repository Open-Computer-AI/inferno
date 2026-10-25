#!/bin/bash
# gateway_queue_v2.sh apply|rollback|status: T100-H step 5 on the Inferno gateway box (AWS, ubuntu). Installs queue v2
# (attest_queue.py, 68a7f38) in front of the attestation tunnel and turns on parallel forwarding (QUEUE_CONCURRENCY=16).
# Run as the box's ubuntu user from the unpacked bundle directory (attest_queue.py + attest-load-linux-arm64 + this file).
# Order: the Air helper v2 must already be live (done 10-23 01:17 UTC). Rollback: queue first, then the Air (README).
#   apply     guards: installed queue md5 = v1 06a02f7d... or already v2; bundle sha256 = 9205dfc2...; idle before every
#             restart. 5a: file swapped with switches off (= v1 behaviour), restart, health. 5b: drop-in
#             QUEUE_CONCURRENCY=16, restart, health. With GW_TF=<gateway token file>: attest-load K=1 after each, K=3/6/10 at
#             the end. Any failed check: STOP with the exact rollback command.
#   rollback  guard first (idle; skipped when the queue is down or silent, or with FORCE=1), then drop-in removed and
#             the v1 file restored from the backup, then one restart + health.
#   status    read-only: file md5, drop-in, unit state, /queue-health.
# Never prints the token (attest-load reads the file). Overridable for tests: QDIR, DROPDIR, UNIT, HEALTH_URL, SUDO, IDLE_WAIT.
set -uo pipefail
MODE=${1:?mode: apply|rollback|status}
HERE=$(cd "$(dirname "$0")" && pwd)
QDIR=${QDIR:-/opt/inferno-queue}; Q=$QDIR/attest_queue.py; BAK=$Q.v1-bak
UNIT=${UNIT:-inferno-attestation-queue}
DROPDIR=${DROPDIR:-/etc/systemd/system/$UNIT.service.d}; DROP=$DROPDIR/t100-v2.conf
HEALTH_URL=${HEALTH_URL:-http://127.0.0.1:19095/queue-health}
SUDO=${SUDO-sudo}; IDLE_WAIT=${IDLE_WAIT:-10}
V1_MD5=06a02f7d4b913f20088f69032650eb7c
V2_SHA=9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d
LOAD=$HERE/attest-load-linux-$(uname -m | sed -e s/aarch64/arm64/ -e s/x86_64/amd64/)

die() { echo "STOP: $*"; exit 4; }
md5() { md5sum "$1" 2>/dev/null | cut -c1-32; }
sha() { sha256sum "$1" 2>/dev/null | cut -c1-64; }
health() { curl -s -m 3 "$HEALTH_URL"; }
idle_check() {
  local a b
  a=$(health); sleep "$IDLE_WAIT"; b=$(health)
  if [ -n "$a" ] && [ "$a" = "$b" ] && ! echo "$b" | grep -Eq "in_flight['\"]?: *[1-9]"; then echo QUEUE_IDLE; return 0; fi
  echo "NOT IDLE (counters moved or in_flight > 0): wait and run again"; return 1
}
restart() {  # restart <hint> [noidle]
  [ "${2:-}" = noidle ] || idle_check || die "queue busy, nothing restarted"
  $SUDO systemctl daemon-reload && $SUDO systemctl restart "$UNIT" || die "restart failed; $1"
  sleep 2
  [ "$(systemctl is-active "$UNIT")" = active ] || die "$UNIT not active; $1"
  local h; h=$(health); [ -n "$h" ] || die "no /queue-health answer; $1"
  echo "health: $h"
}
load() {  # load <k> <rounds>: only with GW_TF; attest-load exits non-zero on any non-200 or repeated proof
  [ -n "${GW_TF:-}" ] || { echo "(GW_TF not set: load check skipped; watch the gateway log for Live creates)"; return 0; }
  [ -x "$LOAD" ] || die "$LOAD missing"
  $SUDO "$LOAD" -url "${HEALTH_URL%/queue-health}" -token-file "$GW_TF" -k "$1" -rounds "$2" -gap 3s || die "attest-load k=$1 failed; $3"
}
RB="bash $HERE/gateway_queue_v2.sh rollback"

case $MODE in
status)
  echo "queue file md5 $(md5 "$Q") (v1 $V1_MD5; v2 sha256 $(sha "$Q" | cut -c1-16)... want ${V2_SHA:0:16}...)"
  [ -f "$BAK" ] && echo "v1 backup: present ($(md5 "$BAK"))" || echo "v1 backup: none"
  [ -f "$DROP" ] && echo "drop-in: $(tr '\n' ' ' < "$DROP")" || echo "drop-in: none"
  echo "$UNIT: $(systemctl is-active "$UNIT")"; echo "health: $(health)"
  ;;
apply)
  [ "$(sha "$HERE/attest_queue.py")" = "$V2_SHA" ] || die "bundle attest_queue.py is not the reviewed v2 (sha256)"
  cur=$(md5 "$Q")
  if [ "$cur" = "$V1_MD5" ]; then
    idle_check || die "queue busy, nothing touched"
    $SUDO cp -p "$Q" "$BAK" || die "backup failed"
    $SUDO install -m 0644 "$HERE/attest_queue.py" "$Q" || die "install failed"
    [ "$(sha "$Q")" = "$V2_SHA" ] || die "installed file mismatch; $RB"
    echo "5a: v2 file installed (switches off = v1 behaviour)"
    restart "$RB"
    load 1 3 "$RB"
  elif [ "$(sha "$Q")" = "$V2_SHA" ]; then
    echo "5a: already v2"
  else
    die "installed queue is neither v1 ($V1_MD5) nor v2: another change; nothing touched"
  fi
  if [ -f "$DROP" ] && grep -q QUEUE_CONCURRENCY=16 "$DROP"; then
    echo "5b: drop-in already present"
  else
    $SUDO mkdir -p "$DROPDIR" && printf '[Service]\nEnvironment=QUEUE_CONCURRENCY=16\n' | $SUDO tee "$DROP" >/dev/null || die "drop-in write failed"
    echo "5b: QUEUE_CONCURRENCY=16"
    restart "$RB"
  fi
  systemctl show -p Environment "$UNIT" | grep -q QUEUE_CONCURRENCY=16 || die "unit does not see QUEUE_CONCURRENCY=16; $RB"
  load 1 2 "$RB"; for k in 3 6 10; do load $k 2 "$RB"; done
  echo "APPLY_OK. Watch 1 h: /queue-health failed not growing, busy_retries ~0, no new 503 on Live creates."
  ;;
rollback)
  # One guard before any file changes. A queue that is down or silent holds no calls, so it is not waited on (waiting
  # would leave the unit down); FORCE=1 skips the wait under traffic (proofs in flight then fail once).
  if [ "${FORCE:-0}" = 1 ]; then echo "FORCE=1: no idle wait"
  elif [ "$(systemctl is-active "$UNIT")" != active ] || [ -z "$(health)" ]; then echo "queue down or silent: no idle wait"
  else idle_check || die "queue busy, nothing touched; run again when idle, or FORCE=1 $RB"
  fi
  [ ! -f "$BAK" ] || [ "$(md5 "$BAK")" = "$V1_MD5" ] || die "backup is not v1; restore by hand"
  [ -f "$DROP" ] || [ -f "$BAK" ] || { echo "nothing to roll back (no drop-in, no v1 backup)"; exit 0; }
  # Both changes first, then one restart: the unit comes back on v1 even when v2 is what made it silent.
  if [ -f "$DROP" ]; then $SUDO rm -f "$DROP" && echo "drop-in removed" || die "drop-in removal failed; fix by hand"; fi
  if [ -f "$BAK" ]; then $SUDO cp -p "$BAK" "$Q" && echo "v1 file restored" || die "v1 restore failed; fix by hand"; fi
  restart "rolled back on disk but the unit did not come back: check journalctl -u $UNIT" noidle
  [ "$(md5 "$Q")" = "$V1_MD5" ] || die "queue file is not v1 after rollback; fix by hand"
  $SUDO rm -f "$BAK"; echo "ROLLBACK_OK"
  ;;
*) die "mode: apply|rollback|status" ;;
esac
