# Direct-path qualification addendum — 27 October 2026

## Current decision

**The isolated direct-path prototype passed the tests below. Production has not
been switched.** The human authentication gate is resolved: AWS can reach the
other Pro directly over Tailscale SSH, without this Mac relaying that proof path.

This report supersedes the earlier report's outstanding direct-authentication
condition. Permanent services, unattended authorization and an eventual route
switch still require the user's production greenlight and the staged gates in
`BACKUP-ROLLOUT-PROPOSAL-20261027.md`. The qualification harnesses are temporary
foreground actors, not deployable production services.

## Current gateway and scope

When work resumed, the running gateway had changed independently of this task:
PID2439627, executable SHA256
`6890e38552c77ee0ebc086c75f729597a173e732a353f09d078afd53a050bc2d`.
Its embedded VCS revision is `288064cecefa08a85f005632243f2cc3a1832290`, marked
clean, linux/arm64. The matching commit was unavailable in this checkout; the
sandbox used the exact hash-matched executable, not a reconstructed build.
The previous c595 executable and all overnight evidence remain separately
identified. No cause is assigned to the external gateway change.

The production queue remained PID1038746 and SHA256
`9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d`;
the existing Air reverse listener remained PID2282787. Current queue mode was
confirmed read-only: concurrency16, keepalive disabled. Fresh HTTP is therefore
a matching connection mode for this comparison; connection reuse is not promoted.

The new sandbox database started empty, with only the approved account28
access/identity projection and synthetic user/key/group. Refresh credentials0,
credential accounts1, active test keys1, historical usage0. The current
executable emitted the effective refresh-disabled marker before Live calls.
No source account, provider credential, production database or customer key was
modified. Test bearer/API-key/JWT/database files were removed after teardown.

## What the direct test means

AWS selector loopback19495 used:

- Air: existing AWS loopback19085 → existing production Air reverse tunnel.
- This Mac: separate AWS loopback19485 → temporary operator-origin SSH reverse
  forward → this Mac's temporary native helper19485.
- Other Pro: AWS loopback19486 → **AWS-origin SSH directly to100.96.58.114** →
  other Pro's temporary native helper19485. No operator relay in this backend.

“Direct” describes the application SSH endpoints. Whether the underlying
Tailscale transport used peer-to-peer UDP or DERP during every sample was not
recorded; no physical-path guarantee is implied.

The test Linux gateway ran on the other Pro. Its proof request still entered
the AWS selector through the operator's separate test forward. That extra
frontend leg is absent from a future AWS-local gateway→selector call. Foreground
controllers also remained operator-owned for safe qualification cleanup; this
is not proof of unattended operation after this laptop disappears.

The direct client was tested with ordinary SSH agent/key/password/keyboard/
GSSAPI/host-based authentication disabled. It used the human-approved Tailscale
identity, pinned AWS node ID/user fingerprint, fixed AWS Tailscale address and
pinned Other-Pro host key. Tailscale documents its identity-based SSH protocol
as using the SSH `none` authentication method after tailnet authentication:
[Tailscale SSH](https://tailscale.com/docs/features/tailscale-ssh).

## Actual Live and fault results

Eight current-binary cases passed:

|Case|Selected helper / result|
|---|---|
|Direct Other healthy|Other Pro|
|Actual direct Other SSH process stopped;19486 verified closed|Air|
|Other tunnel stopped plus Air candidate endpoint unavailable|This Mac|
|All three candidate paths unavailable|HTTP503; no Live usage/session leak|
|New direct SSH process; authenticated helper health before acknowledgement|Other Pro recovered|
|Air-first healthy order|Air|
|Air-first candidate endpoint unavailable|This Mac|
|Air and this candidate endpoints unavailable|Other Pro over direct AWS path|

The Air failures used reserved, non-listening **candidate** endpoints; production
19085 was never stopped or reconfigured. Source control transitions were
acknowledged with case/run/sequence and actual direct-listener state. Reconnection
was tested within the approved Tailscale check window, not across check expiry.

The seven successful cases had accepted SDP, authenticated sideband, virtual
input RTP, audible-level non-silent output, close acknowledgement and closed
client transports. There were seven Live usage rows, every call record closed,
and zero members in all five general/Live account/user/API-key lease families.
No hardware microphone or physical-speaker measurement is claimed.

|Case|SDP response|Media/sideband ready|Text append→first audible-level output|
|---|---:|---:|---:|
|Other healthy|2362ms|3548ms|949ms|
|Other tunnel down→Air|2131ms|3607ms|1009ms|
|Other+Air down→this Mac|2562ms|4210ms|957ms|
|Other recovered|1749ms|2996ms|929ms|
|Air-first healthy|2092ms|3929ms|**6722ms**|
|Air-first down→this Mac|2125ms|3504ms|1067ms|
|Air+this down→Other|1900ms|3678ms|1108ms|

The6.72sec first-audio outlier occurred after readiness and after the only
attestation proof request completed. Its cause was not isolated. Do not
attribute it to the selector or promise that a pool removes such audio delays.
These are small, sequential, differently warmed Live samples, not a voice SLA
or microphone-to-speaker benchmark.

## Direct16 proof soak and concurrent resources

The direct Other-Pro route passed120 batches of16: **1,920 distinct proofs,
zero errors/duplicates, exact1,920 router audit records and zero swapout growth**.
The paced run took320.270sec; its independent concurrent monitor took327.912sec.
All successful selections were Other Pro. Earlier Air/this candidate endpoints
were refused; this was the **last-backend** position.

|Timing scope|p50|p95|p99|max|
|---|---:|---:|---:|---:|
|Operator test client→AWS selector→Other|1726ms|2256ms|2597ms|3110ms|
|AWS→Other backend attempt only|1028ms|1466ms|1851ms|2277ms|

**12/1,920 backend attempts exceeded2sec.** There was no measured fallback in
this soak because Other was last and had the remaining shared budget. The same
latencies would exceed an earlier backend's2sec cap and could cause additional
failover. Earlier-position behavior under this exact load was not certified;
no “zero extra delay across all positions” claim is made. Combining timings
from separate tests is not proof of a full two-hanging-backend load scenario.

The independent monitor collected109 resource observations while requests ran:

- Other-Pro memory pressure remained normal; swapout growth0.
- Whole-host CPU ranged8.85–81.92% during sampled intervals, below the85% gate.
- Maximum sampled sum of helper-group RSS:77.219MiB. RSS is not physical
  footprint or total host RAM and should not be added to wired/compressed RAM.
- Matching extant Go+Node PID cumulative CPU deltas peaked at1.696% of **one
  core** across sample intervals on an8-logical-CPU host. Exited child CPU and
  separate OS/native services are excluded. This is not instantaneous CPU peak.
- Do not attribute whole-host CPU to attestation. The native processes' `ps`
  lifetime/decayed CPU percentage was not used as the under-load CPU measure.

The earlier two1,920-proof backup soaks remain valid for their stated topology
and conditions. No32/33/48 limit, multi-hour soak or universal capacity is proven.

## Final matching AWS-origin latency

Run `e3578a9715fd772c`:600 fresh distinct proofs,300 routed records,100
counterbalanced direct/router pairs **per helper**, same helper and transport
within every pair. Air reference rate≤1proof/sec. Other Pro used the direct
AWS-origin SSH path. All requests succeeded.

|Helper|Direct p50 /p95|Routed p50 /p95|Mean routed−direct|Bootstrap95% interval|
|---|---:|---:|---:|---:|
|Air|564 /872ms|565 /940ms|+10.28ms|[-15.57,+39.20]ms|
|This Mac|717 /794ms|718 /790ms|+10.91ms|[-8.38,+28.63]ms|
|Other Pro, direct|446 /653ms|447 /461ms|-12.86ms|[-26.20,+0.43]ms|

All intervals span zero. Routing overhead was not statistically separable from
noise in this sample. Negative differences do not mean the selector accelerates
proof generation. The routed observer also includes a≥1ms scheduling handoff
before audit validation, so no exact submillisecond selector-cost claim is made.
Raw p99/max observations are retained;100 observations per route make p99 a
single tail observation, not a stable tail SLA. The three helpers were sampled
sequentially, so differences between Macs are not a controlled host comparison.

New temporary native processes had authenticated health warmed before proof
measurement; their first generation pair is included. The OS/native service
was not rebooted or cold-booted. These are fresh HTTP/SSH-channel observations,
not connection reuse qualification. Backend network latency remains part of
setup; selection does not add a proof-proxy hop to media packets after startup.

## Preservation and cleanup

Current gateway/queue hashes, start identities, original Air listener and
19085/19095 authenticated health were stable throughout the guarded direct run.
The58 operational samples during the Live/soak actor cover27 Oct03:33:39–
03:57:50UTC;17 samples during the paired run cover04:05:49–04:12:34UTC.
Failed/unreachable-retry/busy-retry deltas were0 in both windows. Historical
counters remained24/461/0. This does not establish a customer latency SLA or
continuous observation between windows.

Actual direct-controller SIGKILL gate `b61c7868e477e93e` passed independent
verification: remote actor gone; local actor/tunnel groups gone; direct Other
SSH group gone; candidate ports closed; temporary known-host root removed.
Normal direct Live/soak actor `cc57e933a38fcccf` and final paired actor
`e3578a9715fd772c` also passed independent verification. The current Live sandbox
`c8b57b69486a` and all its Docker storage/objects were independently removed.
Final phase cleanup passed for helpers, all candidate ports/roots/groups and
phase credentials. The approved tailnet login/check cache itself was not revoked.

One preparation attempt `f385ebb72e78` was blocked before containers started:
the copied executable was0600. The preparation now verifies its hash and applies
0700 before staging. The failed root was independently cleaned before a fresh
phase. The failed receipts remain identified. A separate macOS process-group
exit race was exposed by QA: only a reaped/terminated-child `EPERM` is ignored;
a live-child permission failure still fails cleanup. Regression tests cover both.

Final source gate:57 focused tests plus Node syntax and diff whitespace checks.
Independent adversarial review and immutable commit/PR status are recorded with
the final delivery. No production routing, firewall, persistent service,
provider credentials or customer database were changed by this task.
