# Isolated backup qualification

Production remains unchanged: AWS gateway, queue19095, listener19085, Air jobs, credentials and firewall.

## Acceptance

1. Verify both Apple Silicon hosts, installed runtime, GUI signals, unused candidate ports.
2. Start bounded temporary helpers on each host loopback19485; tunnel the other helper to operator loopback19486. New independent bearer token per helper. Stop/EOF/deadline removes only owned processes and private temporary directories.
3. Measure 20 fresh proofs per backend; retain only durations, statuses, validity and duplicate counts, never proofs or tokens.
4. Candidate router loopback19495, total7sec, global concurrency16. Measure direct versus routed paired requests, refusal, hang, busy, malformed, both down, cooldown/recovery and caller cancellation.
5. Isolated Linux gateway/database/Redis initialized empty, with one approved access-only account projection, synthetic actors and effective refresh disabled BEFORE Live requests. No production account mutation. Real Live: accepted SDP, sideband, nonzero audio, final closed state, usage and zero leases.
6. Test real Live selection of each backend and a candidate primary failure followed by backup success. Retain selection and timing metadata.
7. AWS-origin transport measurements require candidate-only listeners and no production firewall change; report any relay hop separately.
8. Independent adversarial review; no deployment or promotion on partial acceptance.

## Rollback / stop

Touch artifact-root/private/stop-helpers to stop temporary helper controller. Its EOF watchdog kills only owned helper groups, other-host temporary root, and operator tunnel. Temporary router/gateway teardown uses exact owned PIDs/container names. No production restore is required because production is never modified. Verify candidate ports closed, helper PIDs absent, temporary roots removed; compare original production PID/source hashes.

Latency report must separate native helper duration, routing overhead on a healthy path, failed-attempt delay, total fallback duration, Live creation, sideband readiness and first nonzero audio. Paired tests use matching transport/connection mode.
