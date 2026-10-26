# Isolated Live controller gate

## Passed on 27 October 2026

- Actual hard controller kill followed by independent verification removed the
  owned containers, network, volume, temporary root and SSH forwards. Evidence:
  `live-independent-cleanup-2037f8a8fdf0.json` outside Git.
- Fresh empty candidate database, one minimal access-only account, synthetic
  user/key/group, fresh candidate JWT/TOTP/database secrets. No source database
  restore, history, refresh credentials or source writes.
- Exact current AWS executable hash matched the running production process:
  `4bb9f392de4cfc17586223d6e903c4b3403d5afaf87c8d07f21e254f8fc3267c`.
  Its clean source revision is `c595d020ed32152f515676ab7d738efb4cc589dd`.
- Effective `token_refresh.service_disabled` startup marker was observed on both
  boots; only approved static marker booleans were retained.
- Container-origin fresh proof succeeded. Both backup Macs' proofs were accepted
  in actual Live sessions with SDP, authenticated sideband and audible audio.
- Refusal, hanging primary, busy429, malformed response, recovery and established
  session with candidate helpers unavailable passed. All-down startup returned503.
- Seven successful sessions produced seven Live usage rows. All records closed;
  all account/user/API-key general and Live concurrency leases were zero.
- Normal stop of run `0ae8022331cb` removed its objects/root and returned a clean
  controller receipt. Independent verification is required as a separate check.
- 38 focused tests passed at this earlier gate;46 pass at the final source gate.

## Qualification outcome

The three-node native/Live fault and recovery cases, both1,920-proof native
soaks,600-proof AWS matching measurements, AWS crash/independent-cleanup gate,
and final phase credential teardown subsequently passed. See
`BACKUP-QUALIFICATION-20261027.md` for metrics, imperfect runs, topology,
monitoring coverage and remaining deployment gates.46 focused tests pass.

The draft is reviewable; permanent rollout remains no-go pending direct
AWS→Other-Pro authentication and reviewed unattended ownership/transport.
No production routing, queue, helper, credentials, firewall or persistent service
was changed. The original remaining-gate list is superseded by the final report.
