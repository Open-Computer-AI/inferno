# Qualification completion audit — 27 October 2026

This audits the requested **isolated prototype and decision report**, not a
production installation. Production promotion remains a separate authorization.

|Requirement|Authoritative evidence|Assessment|
|---|---|---|
|Dedicated worktree/branch and reviewable PR|`test/attestation-backups-20261027`, PR35, scoped diff; no merge|Proven; final direct update committed/pushed at delivery|
|Preserve production|Guarded gateway/queue/listener start identities and hashes; authenticated health; monotonic queue error/retry deltas0 in documented windows|Proven for observed scope; independent gateway change between phases explicitly recorded|
|Repair/cleanup controllers before qualification|Actual controller SIGKILL gates and independent receipts, including direct AWS gateb61c7868e477e93e|Proven; initialization permission failure was cleaned before fresh retry|
|Both backup host/runtime identities|Pinned SSH host identities, Apple Silicon checks, native runtime/binary SHA preflight, authenticated helper paths|Proven for tested runtime versions|
|Each Mac generates fresh usable proofs|Native compatibility/uniqueness observations plus actual upstream Live acceptance on each helper|Proven; no proof cache|
|Shared deadline, cancellation, health/recovery, auth, bounds|Focused router tests; actual12-case fault matrix; regression for concurrent recovery; actual Live fault cases|Proven for tested cases; structure validation is not upstream cryptographic validation|
|Direct Other-Pro path without operator proof relay|Source identity/auth-method pinning, direct AWS SSH PID/health and real Live selection|Proven for backend path; test frontend/controller legs are separately labeled|
|Real Live sessions and eventual cleanup|Earlier phases7/5 usage rows; current phase7; closed records and five lease families0|Proven; synthetic virtual input and audible-level digital output, not physical speaker/microphone|
|Refused, dropped, hung, busy, malformed, cancellation, all-down|12 actual-helper cases, plus direct SSH shutdown, unavailable combinations,503 and reconnection|Proven; no physical production Air outage injected|
|Concurrency, uniqueness, sustained load|Both backups1,920-proof bounded soaks; direct Other1,920-proof/16 run, exact audit counts,0errors/duplicates/swap growth|Proven at16 in stated positions/conditions; no32/33/48 certification|
|CPU/RAM/pressure under load|109 concurrent direct-soak samples; extant helper PID CPU-time deltas; RSS, host CPU and pressure/swap|Proven for reported Mac samples; whole-host load is not attributed to helper|
|Matching current-Air/direct/routed latency|100 counterbalanced pairs/backend;600 fresh proofs/300 routed records; direct benchmarke3578a9715fd772c|Proven; intervals, sample limits, connection mode and observer handoff disclosed|
|Live setup/sideband/first audio separately|Current eight-case client artifacts; seven successful audio sessions and all-down503|Proven measurements;6.72sec post-readiness audio outlier and unknown cause retained|
|Cold/warm/topology/drawbacks|Direct report documents warmed health, first generation, no reboot, operator frontend/control, tailnet carrier unknown, expiry and common failures|Disclosed; no unrun reboot/roam/unattended claim|
|Access-only data/no refresh/no private publication|Fresh DB audits0refresh/1account/1testkey/0history; effective disabled marker; sanitized artifacts and source-only diff|Proven for staged inputs; source OAuth/DB remained read-only|
|Exact candidate stop/rollback readiness|`backup_phase_cleanup.py` active-phase command, independent verifiers, final successful phase receipt; working route retained|Proven candidate-only teardown; production rollback procedure is a later approval-dependent proposal|
|Final independent adversarial review|Review of direct transport, control, resource monitor, cleanup and final immutable source update|Passed: independent reviewer found no critical source blocker; draft qualification scope only, no production approval|
|Artifact manifest|Named sanitized artifact SHA256 manifest verified against current files|Passed: 99 artifact SHA256 entries independently recomputed against current local files; private credentials excluded|

## Deliberately unclaimed outcomes

Permanent LaunchAgents/systemd services, new production keys/firewall/policy,
unattended authorization across expiry, public-IP roam/reboot, multi-hour soak,
all deadline/load permutations, connection reuse and production cutover were
not performed. They are explicit prerequisites or future scope in the rollout
proposal, not missing evidence silently marked as passed here.

## Final adversarial decision

The independent transport reviewer approved shipment as a draft qualification PR,
conditional on a final immutable commit, repeating the57 focused tests/Node syntax/
diff checks against it, and a clean worktree. No remaining critical source blocker
was found. Permanent rollout remains no-go until separately approved and its real
supervisor/authentication/reconnect gates pass. Final commit and verification
results are recorded on PR35 and in the delivery response.
