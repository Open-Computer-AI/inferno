# Phase 0 baseline — captured before merging upstream/main

    HEAD      9deb478a9
    upstream  47d818e83 (merge target)
    captured  2026-09-02

These are the alarm for the upstream catch-up. Nothing in it is allowed to make
any of them worse.

`conversion-status` is the one that matters most. `vue-tsc` and `vitest` catch a
port that BREAKS. Only `conversion-status` catches a port that SUCCEEDS by
overwriting converted work with upstream's Tailwind — which is precisely what
happened on 2026-08-11, when four locale files were `cp`-ed wholesale and the
only tell was the converted file count falling 96 -> 92. A health signal that
improves because work was destroyed is the one to distrust.

**If conversion-status DROPS, revert the batch. Do not investigate first.**

| gate | baseline | exit |
|---|---|---|
| `npx vue-tsc --noEmit` | 0 errors | 0 |
| `npx vitest run` | 278 files · 2008 tests, all passing | 0 |
| `node scripts/conversion-status.mjs` | **211/352 files (59.9%) · 9461 legacy utilities · 89350 lines** | — |
| `node scripts/debt-ledger.mjs --check` | 0 open, 10 closed of 10 | 0 |
| `node scripts/behaviour-parity.mjs` | 67 shared specs · ours 643 vs upstream 637 · 0 missing · **3 shortfalls** | — |
| `node scripts/june-lint.mjs` | **1353 violations across 308 converted files** | **1** |

## Two entries above are not green, and both are honest standing state

**june-lint exits 1 with 1353 violations.** This is not new and was not caused by
the catch-up — the script has not changed since `f127794f4` and the tree has not
changed since `bcba4d37b`. It is a standing backlog, dominated by one rule:

    ground-rule-3-two-weights          416
    ground-rule-6-static-borders        51
    ground-rule-2-no-dashes             41
    ground-rule-1-sentence-case         28
    ground-rule-7-no-gradients          9
    ground-rule-8-no-emoji              3
    ground-rule-4-token-font-sizes      1

The catch-up rule for it is therefore **do not let the count rise**, not "get it
to zero". Zero is conversion work, not port work.

**behaviour-parity shows 3 shortfalls where last week showed 0.** Nothing of ours
regressed. The gate reads spec files from `upstream/main` at run time, so when
upstream adds cases to a spec we already hold, the gap opens automatically. All
three are unported commits from this catch-up window and close when their story
lands:

| shortfall | opened by | closes in |
|---|---|---|
| `admin/usage/__tests__/UsageTable.spec.ts` −1 | `1cc6999ad` | story 2 |
| `views/admin/__tests__/UsageView.spec.ts` −1 | `1cc6999ad` | story 2 |
| `account/__tests__/credentialsBuilder.cnAdaptive.spec.ts` −1 | `e377c4358` | story 6 |

That the instrument surfaced upstream's new work unprompted, on the first run
after upstream moved, is the behaviour it was built for.
