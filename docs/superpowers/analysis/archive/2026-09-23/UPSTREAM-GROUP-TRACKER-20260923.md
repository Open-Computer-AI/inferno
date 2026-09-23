# Feature-grouped upstream tracker — 2026-09-23

> Historical snapshot only. Its 119-row table below was generated at
> `033047b7d` and is superseded by the current-tip counts and implementation
> state in [UPSTREAM-GROUP-PLAN-20260923.md](UPSTREAM-GROUP-PLAN-20260923.md).

## Scope and safety boundary

- Candidate only: `/Users/saksham/OpenComputerV2/inferno-port-20260922`, branch `port/inferno-selective-upstream-20260922`.
- Protected baseline: `/Users/saksham/OpenComputerV2/inferno-local`, commit `7d3a6099bfa5d14d95253de7ac1864a9b330040e`; do not modify it.
- Upstream snapshot: `upstream/main` at `033047b7d445ef0af7d9963276773df85ea0f13a`.
- The candidate was clean at `118f7b0f49a360aa802e1a82d12bf33c09c3368e` when this inventory was counted.
- Preserve OAuth/device flow, account routing, GPT Live/sideband/continuation behavior, Hermes routing, billing/quota/failover, and the June Inferno UI. No production deployment, container restart, database reset, credential changes, or destructive cleanup.

## Why the unit of work changed

Review and integration are now organized by behavior, not by individual Git commits. Commits are source evidence: several may make up one feature, and merge wrappers may add no independent code. One feature group can include backend behavior, the shared `frontend/**` mirror, and/or the separate `inferno-frontend/**` June product surface; those surfaces remain separately implemented and tested.

For every source change, record TAKE, HAND-MERGE, or SKIP with its upstream SHA, target paths, candidate evidence, and focused verification. A TAKE disposition alone does not prove integration: verify the candidate commit/equivalent implementation and tests before marking the feature complete.

## Current grouped inventory

Snapshot: 119 non-empty upstream changes through `033047b7`; 50 empty merge wrappers are excluded from implementation work. The initial grouping uses source subject plus affected paths and may be refined when a feature is inspected.

| Behavior group | Rows | TAKE | SKIP | REVIEW | UNSET | Surface-specific |
|---|---:|---:|---:|---:|---:|---:|
| Gateway, protocol, provider adapters, streaming/WS | 49 | 3 | 14 | 20 | 12 | 0 |
| Accounts, OAuth/auth, account selection and routing | 20 | 3 | 0 | 10 | 7 | 0 |
| Billing, quota, balance, pricing and subscriptions | 16 | 3 | 1 | 9 | 3 | 0 |
| Admin, operations, proxy and configuration | 18 | 8 | 0 | 5 | 5 | 0 |
| Frontend product UX not classified above | 16 | 6 | 0 | 2 | 6 | 2 |
| **Total** | **119** | **23** | **15** | **46** | **33** | **2** |

The two surface-specific rows are DateRangePicker dismissal and BaseDialog scroll locking: take the upstream behavior in the shared frontend mirror, but skip the June runtime change because June already has equivalent behavior and regression coverage. These are different target surfaces, not unresolved code conflicts.

Thus 79/119 rows (66.4%) still need a decision. This is only the unresolved-disposition rate, not overall project completion. The earlier “15% complete / 85% remaining” statement was invalid: it ignored already-integrated work and used an unsuitable denominator. The candidate already contains 250 commits since baseline, but commit count by itself also does not prove feature completeness.

## Execution protocol

1. Process one behavior group at a time. For all rows in that group, inspect the upstream diff and the actual candidate consumer/implementation together; identify existing equivalent behavior before editing.
2. Resolve all rows in the group as TAKE, HAND-MERGE, or SKIP. Group related backend/shared-frontend/June-frontend changes by behavior, while retaining separate target-path ownership and not wholesale-copying the June UI.
3. Implement the selected group as one coherent checkpoint. Add/fix focused tests, then run that group’s relevant backend/frontend checks. For gateway, OAuth, WS, sideband, account continuation, and billing/failover intersections, do not combine parallel edits to overlapping contracts; integrate those serially.
4. Update the disposition ledger and this tracker once per completed group with source SHAs, candidate commit/equivalence evidence, tests, and any intentionally deferred cross-group dependency.
5. After all groups are dispositioned and integrated, run the full required verification and local smoke checks in `INFERNO-BUILD.md`. Do not claim completion from inventory decisions alone.

## Immediate next action

Start with **Accounts / auth / routing**: reconcile its 17 REVIEW/UNSET rows together, compare against candidate OAuth, account selection, reauthorization and continuation paths, and test the cross-layer routing contract before moving to the Gateway group. Do not edit the protected baseline or resolve unclear behavior by taking upstream wholesale.

### Account-group audit started

No application code has been changed in this pass. Two concrete candidate gaps are confirmed and need to be included in the account checkpoint:

- `63c079d87` — candidate `ApplyOAuthCredentials` sanitizes and stores the new OAuth credential map without first preserving absent existing credential settings. The upstream fix merges old/new credentials before sanitization. Hand-merge only that behavior; the source diff also references an upstream validation helper not present in this candidate, so do not cherry-pick the whole commit.
- `7f18f3e9a` — candidate `UserApiKeysModal` watches only whether the dialog is open and accepts every fetch response. Switching users or closing/reopening while a request is in flight can show stale key data. Upstream adds request-version invalidation and clears old rows before loading; add/port its focused race test with the behavior.

Both remain unmodified pending the rest of the group audit and focused verification. The remaining account-group rows still need the same candidate/source comparison before any implementation batch.
