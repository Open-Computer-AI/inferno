# Three-Mac attestation qualification — 27 October 2026

## Historical overnight decision

**Update:** direct AWS→Other authentication and tests subsequently completed.
See `BACKUP-DIRECT-QUALIFICATION-20261027.md` for the current results and
`BACKUP-ROLLOUT-PROPOSAL-20261027.md` for the later approval-dependent plan.
The following preserves the earlier snapshot and its then-pending gates.


**The isolated selector/failover candidate passed the tests below. Permanent
rollout remains NO-GO. No production route or persistent service was changed.**

The remaining deployment gate is a direct, independently authenticated
AWS-to-other-Pro path and reviewed unattended service/tunnel configuration.
The other Pro was tested through an operator relay. That relay is a common
failure point and must not be presented as the permanent three-node topology.
No aggregate48,32 or33 concurrency increase is qualified; global16 stays fixed.

## What was built and tested

A temporary Python/stdlib loopback selector with distinct frontend/backend
bearers, global16 and per-helper16 limits, a seven-second shared request budget
(gateway budget eight seconds), two-second earlier attempts, remaining time for
the last backend, three-second cooldown and a single recovery probe. It accepts
only authenticated health and empty proof requests. Proofs are generated fresh;
there is no proof cache. Helpers receive no provider credentials, SDP or audio.

A real recovery burst exposed15/16 rejected requests while a single half-open
probe ran. The repair makes concurrent admitted callers wait for that probe when
no alternative can run, within their original budget. A 16-caller regression
and subsequent native soaks passed. Pre-repair evidence is retained separately.

## Acceptance results

| Area | Evidence / result |
|---|---|
|Native compatibility|Both Apple Silicon backup Macs generated fresh distinct native proofs.|
|Helper faults|12 actual-helper scenarios passed: refusal, drop, hang,429, malformed envelope, all-down, cancellation, cooldown/recovery/failback.|
|Real Live through each backup|Accepted SDP, authenticated sideband, virtual input RTP and non-silent output with audible-level RMS; no hardware microphone or physical-speaker claim.|
|Real Live through three-node selector|Air selected; candidate Air tunnel closed and its processes independently absent → this Mac; Air plus this endpoint unavailable → other Pro; all-down →503; new candidate Air tunnel → Air recovery.|
|Session cleanup|First phase7 successful sessions/7 Live usage rows; three-node phase5/5. Every verified session closed its record; all five account/user/API-key lease families zero.|
|Established-session outage|An established session continued audible-level output after both candidate helper endpoints were unavailable.|
|Soak|Each backup:120 batches of16,1,920 distinct proofs,zero errors/duplicates/swapout growth, exact router audit counts.|
|AWS measurements|600 distinct proofs,300 exact routed records,100 counterbalanced direct/routed pairs per backend.|
|Crash cleanup|Actual SIGKILL of Live controller and separate AWS measurement controller; independent process/group/root/Docker/listener verification passed.|
|Normal cleanup|Both Live phases independently removed; all candidate helper groups/temp roots/forwards/ports removed; all phase credentials removed.|
|Source checks|46 focused Python tests and Node syntax check pass. No production deployment or CI success is implied.|

The Live gateway used the exact current AWS executable, copied read-only and
hash-matched to its running process:
`4bb9f392de4cfc17586223d6e903c4b3403d5afaf87c8d07f21e254f8fc3267c`;
clean source revision `c595d020ed32152f515676ab7d738efb4cc589dd`.

Candidate databases started empty. Only one approved account's access/identity
projection and synthetic user/key/group were seeded, with fresh candidate
JWT/TOTP/database secrets. Refresh credentials0, historical usage0. Effective
refresh-disabled startup marker verified. No source account write or refresh.
Raw tokens, proof bodies, SDP, audio and transcripts were not published.

### Imperfect runs and how acceptance handled them

- Initial memory-pressure gates stopped load before certification. Closing
  candidate Live containers freed test overhead; unrelated apps were left alone.
- One pre-repair concurrent recovery batch produced15 HTTP503s. It is a failure,
  not part of the successful soak results.
- One post-Live read-only SSH snapshot returned empty output. The sequence
  stopped. Independent retry confirmed closed records/zero leases, and a fresh
  full Live case passed. Read-only snapshot retries are bounded; Live creation
  is never silently retried by the harness.
- The first final phase cleanup did not pass its helper verifier and retained
  credentials. A direct independent helper verification then passed, followed
  by a successful full phase cleanup and credential removal. No first-attempt
  cleanup success is claimed.
- A later optional Air recheck encountered its600sec test-forward lifetime
  expiry and stopped before proof requests. Independent process/port absence
  was verified; a new bounded forward and fresh guarded recheck passed.

## Latency

### AWS-origin matching comparison

Every direct/routed pair targets the same native helper over the same transport,
with fresh HTTP connections, counterbalanced order,100 pairs per backend.
Air reference rate stayed at most one proof/sec. Bootstrap intervals describe
uncertainty of the paired mean, not a per-request worst-case guarantee.

|Helper|Direct p50 / p95|Routed p50 / p95|Mean routed-minus-direct|Bootstrap95% interval|
|---|---:|---:|---:|---:|
|Air|560 /949ms|575 /1019ms|+16.97ms|[-6.74,+43.75]ms|
|This Mac|705 /738ms|706 /744ms|-0.49ms|[-18.45,+14.84]ms|
|Other Pro|744 /812ms|743 /783ms|-6.97ms|[-17.09,+4.07]ms|

**No statistically resolved positive selector cost in these measurements.**
Negative deltas are noise, not an acceleration claim. These bounds are for this
sample/window. With100 observations per route, p99 is one tail observation and
must not be treated as a stable tail-SLA estimate. Raw timings include maxima.

Topology matters:

- Air: AWS loopback19085 → existing production Air reverse tunnel → native helper.
- This Mac: separate AWS loopback19485 → operator SSH reverse-forward → native helper.
- Other Pro: AWS loopback19486 → operator reverse-forward → operator local19486
  → Other-Pro SSH forward → native helper. This extra relay remains unqualified
  as a permanent topology. It cannot prove performance when this Mac is offline.
- This measures the existing Air helper reference with fresh HTTP; it does not
  certify a new queue connection-reuse mode or an end-to-end customer SLA.

Earlier operator-origin matching100-pair measurements found mean selector
costs0.587ms (95%[-1.491,2.769]) and0.758ms ([-9.103,8.418]). Those also span zero,
used matching proxies, and occurred during operator memory-pressure warning.

### Failover and real Live startup

A hanging first helper adds about2sec before fallback; the actual helper fault
matrix observed~2092ms total. Two hanging earlier backends can consume~4sec.
All attempts share7sec; there is no new7sec allowance for each backend.

Selected real Live examples (sequential, different upstream warm/cold state;
not a controlled microphone-to-speaker or host-speed comparison):

|Scenario|SDP response|Media/sideband ready|First audible-level output after text append|
|---|---:|---:|---:|
|First this-Mac session|2619ms|4374ms|1115ms|
|Other Pro after hanging primary|3762ms|4994ms|1069ms|
|Three-node Air healthy|2056ms|3284ms|1169ms|
|Air candidate tunnel down → this Mac|901ms|2040ms|1308ms|
|Two paths down → other Pro|1117ms|2295ms|1379ms|
|Air recovered|1825ms|3021ms|1875ms|

The helper is involved in session startup. Healthy selection adds no extra hop
to ongoing audio. That does not guarantee survival of unrelated upstream,
network or session failures.

## Sustained native resources

|Backup|Proofs / elapsed|p50 /p95 /p99 /max|Whole-host CPU range|Peak helper-group RSS|
|---|---:|---:|---:|---:|
|This Mac|1920 /169.409sec|159 /285 /319 /648ms|19.03–71.05%|49.77MiB|
|Other Pro|1920 /160.389sec|220 /347 /408 /520ms|23.05–62.10%|51.50MiB|

Both had25 normal-pressure observations and zero swapout growth. The probe
issued batches of16, at most one batch/sec plus telemetry overhead. This is a
bounded paced soak, not maximum-throughput or multi-hour soak certification.
CPU is the whole host; per-process `ps` CPU uses a different averaging basis.
RSS is resident memory, not physical footprint or total host RAM used.
During the later low-rate AWS benchmark Other-Pro pressure began at warning and
ended normal, with no swapout growth; that benchmark is not a clean capacity run.

## Production preservation evidence

Gateway PID481177 and start ticks43877907; queue PID1038746/start61158650;
queue SHA`9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d`;
Air reverse-listener PID2282787: stable across guarded pool cases and final
checks. Authenticated19085/19095 health remained200/supported.

Operational queue counters were sampled21 times from26 Oct23:10:50.914 to
23:15:49.449UTC (~299sec): failure, unreachable-retry and busy-retry deltas all0.
The first totals already contained24 historical failures and461 unreachable
retries. Do not attribute those historical counts to this run. Counter coverage
started after the AWS benchmark began; earlier identity/health checks do not
prove absence of all transient degradation between observations.

Air tests use the existing production helper at low reference rate. They are
shared-helper reference traffic, not a separately installed Air helper. No
production helper/tunnel was stopped, routed away, reconfigured or restarted.

## Drawbacks and remaining gates

1. Direct AWS→Other-Pro Tailscale SSH requires a separate human identity check.
   The earlier check authenticated the operator source. Do not bypass this by
   weakening SSH policy or changing the production firewall. A fresh auth URL
   must be generated when a human is available; the prior terminated-probe URL
   must not be reused.
2. Permanent launchd/systemd ownership, restricted transport credentials,
   boot/login/reconnect behavior, power/sleep policy, expiry and monitoring are
   proposal work requiring later approval. Foreground test supervisors are not
   a permanent deployment. Interactive Tailscale checks must not become an
   unattended production reconnect dependency.
3. Macs on one power/network connection have correlated failures. AWS,
   selector/queue and tailnet policy remain shared dependencies. An operator
   relay introduces another failure point and is not the desired final route.
4. Helpers depend on installed native ChatGPT files and the correct user/runtime
   environment. App/macOS updates and simultaneous sleep can affect every node.
5. The selector validates envelope structure, not Apple/OpenAI cryptography.
   A structurally valid proof rejected upstream does not currently feed back
   into helper selection or trigger another backend attempt.
6. Network cancellation propagates and late native responses are discarded.
   The existing provider does not prove that an already issued Apple operation
   itself can be aborted; released HTTP/router slots do not prove zero native
   background work.
7. Global16 and per-helper16 protect different resources. Three helpers do not
   automatically approve48 simultaneous proofs or32/33. Upstream/account Live
   concurrency is separate from proof generation. Connection reuse remains a
   separate unqualified change and was not promoted.
8. No reboot, physical network roam/public-IP change, simultaneous three-Mac
   failure or multi-hour soak is claimed. Existing Tailscale transport is the
   basis for IP stability; this task did not modify it.

## Reproduce, stop and proposed later rollout

Tests (no remote work):

```sh
python3 -m unittest discover -s deploy/attestation -p 'test_backup*.py' -q
node --check deploy/attestation/backup_live_smoke.mjs
```

Remote harnesses require owned0700 private staging and exact approved identity
inputs. `backup_live_prepare.py` creates a fresh DB phase; helpers use independent
native runtime hashes and tokens. `backup_pool_prepare.py` is the explicit
three-node wiring/verification step. Do not run these as production entrypoints.
Sanitized evidence filenames and SHA256s are in `BACKUP-EVIDENCE-20261027.json`;
private authorization and binaries are outside Git.

Candidate-only stop:

```sh
python3 deploy/attestation/backup_phase_cleanup.py /Users/saksham/Documents/Codex/2026-10-27/attestation-backups
```

The cleaner creates only candidate markers, invokes independent verifiers,
checks exact actor identities and ports, and removes phase secrets only after
success. Failure retains inputs and fails acceptance. Completed receipts show
candidate-only teardown passed. No production restoration was required because
production configuration was never changed.

For a later human-approved rollout: first qualify direct unattended backup
transports on separate listeners, retain the exact working Air path and queue
config, install separately reviewed service ownership, validate isolated Live,
then fully drain affected established Live sessions before any gateway restart
required for the route switch; draining attestation work alone is insufficient. Monitor fresh proof,
real Live, record/usage/lease cleanup and queue error deltas. Restore the saved
Air route immediately on failure; retain it until the new route passes an
approved observation period. Do not merge, deploy, or perform that switch under
this qualification task.
