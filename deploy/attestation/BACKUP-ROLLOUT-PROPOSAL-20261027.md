# Proposed permanent rollout — requires later human greenlight

This is a proposal, not executed deployment instructions. The existing Air route
is working. Qualified foreground test actors have been removed. The temporary
harnesses contain lifetime/stop controls and must not be installed as production
services unchanged.

## Proposed permanent scope

1. Keep the Air's existing helper, token and Tailscale reverse tunnel intact.
2. Install the same qualified native helper behavior on this Mac and the other
   Pro under their proper logged-in users, using independent bearer tokens,
   loopback-only19485 and max-concurrent16. Use user LaunchAgents independent
   of Codex, Chrome and Docker. Review app update, login and sleep/power behavior.
3. Give each backup its own supervised Tailscale SSH tunnel and AWS loopback
   listener19485/19486. Prefer Mac-originated restricted reverse tunnels matching
   the established Air pattern: separate keys, pinned AWS host key, fixed
   tailnet destination, no shell/PTY/agent/X11 rights, explicit allowed listener
   and source. A forced failing command is needed to deny shell/exec; OpenSSH
   `restrict` alone does not deny shell execution.
4. If this preferred profile needs a new restricted authorized-key entry and a
   TCP22 allowance for Other-Pro's fixed tailnet source, those are explicit
   production security changes requiring the greenlight. No public-IP allowlist
   updates or broad ingress rules. Do not alter/remove unrelated rules or keys.
5. The tested alternative AWS-origin Tailscale SSH path needs a reviewed
   unattended authorization design: current human check authorization can
   expire. Do not globally disable checks or broaden all-user access. Qualify
   whichever exact persistent profile is chosen; do not equate a current login
   with unattended reconnect behavior.
6. Package the reviewed selector core as a separately supervised AWS service on
   loopback19495: Air19085 first, this Mac19485 second, Other19486 third;
   frontend token distinct from all backend tokens; total7sec budget; global16;
   bounded diagnostics/metrics and log rotation. No new package-manager/runtime
   dependency is required by the stdlib prototype, but permanent packaging and
   ownership still need review.
7. Generate only the new backup/pool transport and bearer credentials. Preserve
   the existing Air secret for rollback. Do not rotate OAuth/provider credentials,
   alter account limits, change balances/billing or import customer data.

The preferred reverse profile differs in direction and security configuration
from the direct-client qualification. It must pass isolated proof, Live,
reconnect and cleanup checks on separate listeners before customer routing.
No final reverse-profile latency or reboot/login behavior is claimed today.

## Parallel installation gates after approval, before route change

- Capture exact current gateway executable/container/deployment identity and
  secure copies of the two existing attestation env settings, secret mount and
  launch configuration. Determine the actual deployment controller and exact
  restore command; do not guess a generic restart command.
- Install new services/tunnels on separate ports; preserve working19095→19085.
- Prove start/restart/reconnect under the real supervisors, login/native runtime,
  independent authentication, expiry policy and absence of broad shell/forward
  permissions. Never fault the production Air to test this.
- Repeat authenticated fresh proofs and a dedicated approved-key Live test
  through the permanent candidate path: SDP, sideband, non-silent audio, closed
  records, usage and zero leaked leases. Recheck16-load/deadline behavior and
  the measured>2sec early-attempt limitation before choosing unchanged budgets.
- Compare matched timings using the exact installed connection mode. No32/33
  or connection-reuse promotion is included. Stop on identity drift, elevated
  error/retry counters, resource pressure, cleanup failure or expired auth.

## Gateway cutover gate: drain established Live sessions

The inspected service construction calls `liveattestation.NewProvider()` once;
that factory reads `LIVE_ATTESTATION_URL`/`LIVE_ATTESTATION_TOKEN_FILE`. Do not
assume changing an environment file reconfigures a running process. Confirm the
current deployment's exact behavior before acting.

If applying the new endpoint/token requires gateway replacement or restart,
**drain all established Live sessions and other affected long-lived requests**,
not merely attestation queue `in_flight`. Verify authoritative active session
records/leases and connection state; agree the admission/maintenance mechanism.
If a safe drain cannot be demonstrated, do not restart or cut over.

Under the approved window, change only the attestation endpoint to the candidate
AWS19495 service and its independently mounted frontend bearer file. Keep the
same approved gateway binary and all unrelated configuration. Apply through the
actual deployment controller after the full drain. Keep customer admission held
until fresh proof/Live/closure/usage/lease checks pass and error deltas stay flat.
Then observe normal traffic for an agreed period with the old route retained.

## Rollback

Before the cutover, have the exact saved launch configuration, secret mount,
endpoint19095 and original Air-token path ready. Candidate-only failures before
routing need only candidate stop/removal; the working route is still in place.

After routing, hold new admissions and safely drain affected Live sessions
before any rollback that restarts the gateway. Restore the saved two attestation
settings/mount and original launch configuration through the verified controller;
verify19095→19085 authenticated health, fresh proof and an approved-key real Live
session, closed records, usage, zero leases and flat queue error/retry counters.
Do not kill customer sessions or force a restart as a shortcut. Preserve the
new service evidence until rollback is independently verified, then remove only
owned candidate/pool artifacts as authorized.

No rollback was needed during qualification because production was not changed.
