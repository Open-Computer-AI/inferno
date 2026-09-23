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

---

# Phase 1 — after the merge (`0dc970bdd`)

The merge landed 223 backend files, 69 mirror files, 4 deploy, 4 root, and
**0 files under `inferno-frontend/`**, as promised. Re-measured, not assumed.

| gate | before | after | verdict |
|---|---|---|---|
| `vue-tsc` | 0 errors | 0 errors | unchanged |
| `vitest` | 278 / 2008 | 278 / 2008 | unchanged |
| `conversion-status` | 211/352 · 9461 | 211/352 · 9461 | unchanged |
| `debt-ledger --check` | 0 open / 10 | 0 open / 10 | unchanged |
| `behaviour-parity` | 3 shortfalls | 3 shortfalls | unchanged |
| `june-lint` | 1353 / 308 files | **1387 / 316 files** | **moved — see below** |

## june-lint rose 34 while zero of our files changed

Worth writing down, because it is the 2026-08-11 lesson running backwards and
it will happen at every future merge.

`june-lint`'s scope is `ours.has(f) && differsFromMirror(f)` — a file counts as
converted only if we changed it since the vendor point **and** it still differs
from the `frontend/` mirror. The first half is vendor-point-relative and stable
across a merge. The second half is not: a merge advances the mirror.

Twelve files flipped `same -> diff` when the mirror moved:

    src/api/admin/channels.ts                  src/components/account/CNProviderQuotaCell.vue
    src/api/admin/settings.ts                  src/components/account/credentialsBuilder.ts
    src/components/admin/channel/types.ts      src/components/common/MonitorQuotaView.vue
    src/components/keys/UseKeyModal.vue        src/utils/format.ts
    + 4 of their spec files

Every one of them is a file we ported **verbatim** — the copy made it
byte-identical to the mirror, which took it OUT of lint scope. Upstream has now
edited all twelve, so they differ again and re-entered scope carrying
**upstream's own Tailwind** with them (`UseKeyModal` 99 utilities,
`MonitorQuotaView` 25, `CNProviderQuotaCell` 21).

So the +34 is upstream's code being counted, not ours regressing. The
converted FILE COUNT rose 308 -> 316; it did not drop, and a drop is still the
signal that work was destroyed.

**The standing rule gains a second half.** The 08-11 rule was: if the count
DROPS, revert. The corollary is now measured: across a merge the count can RISE
for the same structural reason, and that rise is equally not about our quality.
Compare june-lint only between runs with the mirror held still — which means
the post-merge number, 1387 / 316, is the baseline the seven stories are
measured against, not 1353 / 308.

## The migration collision is benign — proven, not argued

The merge brought five migrations, three numbered `232`. All three are
`ADD COLUMN IF NOT EXISTS` on distinct columns (two on `groups`, one on
`channel_model_pricing` / `channel_pricing_intervals`), so they commute and are
idempotent.

Verified against the real `oc_internal` schema rather than reasoned about: all
five concatenated in **deliberately reverse order**, run inside a transaction,
`ON_ERROR_STOP=1`. Every statement applied, all four new columns appeared, and
the transaction was rolled back — `oc_internal` is byte-for-byte untouched.

    force_openai_fast · free_openai_fast · max_reasoning_effort_over_limit
    cache_write_1h_price

Nothing blocks the seven stories.
