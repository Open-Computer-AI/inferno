# MiniMax AC6 rebaseline evidence

Date: 2026-09-08
Candidate: `f1901a09339cd32c3d9352335bc65dc6da20538f`
Baseline command: `node inferno-frontend/scripts/port-coverage.mjs --baseline`
Verification command: `node inferno-frontend/scripts/port-coverage.mjs --check-baseline`

The pre-port baseline delta reported 19 newly changed coverage lines. Each was reviewed
against the current tree. The lines are accepted intended June-rewrite differences or
relocations, except for the MiniMax monitor validation omission corrected in source and
its capability test.

## Reviewed 19-line delta

| Upstream row | Line classification | Evidence |
|---|---|---|
| `302a10b88` `ChannelMonitorView.grok.spec.ts` provider button count | Accepted intended behavior | The product now exposes 9 providers, including MiniMax; `ChannelMonitorView.grok.spec.ts` asserts 9 and `PROVIDERS` contains MiniMax. |
| `85051616f` `CreateAccountModal.vue` adaptive reset helper | Accepted intended behavior | June account form generalized the CN preset union to include MiniMax; current implementation is the generalized equivalent. |
| `85051616f` `credentialsBuilder.ts` CN platform union | Accepted intended behavior | Current credential builder uses the generalized platform contract and MiniMax-specific defaults; the old three-provider literal is intentionally absent. |
| `8f6f45983` `ChannelsView.vue` platform order | Accepted intended behavior | Current platform order includes all nine supported providers, with MiniMax appended to the established order. |
| `901a0439f` `CreateAccountModal.vue` CN preset computed | Accepted intended behavior | Current computed preset includes MiniMax and preserves the same adaptive selection semantics. |
| `901a0439f` `CreateAccountModal.vue` CN selection helper | Accepted intended behavior | Current selection helper includes MiniMax and resets the matching adaptive URLs. |
| `901a0439f` `CreateAccountModal.vue` platform checks (3 lines) | Accepted intended behavior | Current checks are generalized to include MiniMax; the upstream three-provider expressions are deliberately replaced. |
| `901a0439f` `EditAccountModal.vue` preset computed | Accepted intended behavior | Current edit form includes MiniMax in the generalized preset contract. |
| `901a0439f` `EditAccountModal.vue` platform check | Accepted intended behavior | Current edit-form default selection handles MiniMax through the generalized branch. |
| `901a0439f` `EditAccountModal.vue` deepseek branch | Accepted intended behavior | The old DeepSeek-only expression was generalized; MiniMax is covered by the same current contract. |
| `901a0439f` `credentialsBuilder.ts` coding predicate | Accepted intended behavior | Current predicate intentionally distinguishes the expanded CN coding set and has dedicated MiniMax tests. |
| `b171bb0e4` `ChannelsView.vue` composite platform list | Accepted intended behavior | Current composite platform list includes MiniMax and preserves the composite scheduler. |
| `b171bb0e4` `GroupsView.compositePlatforms.spec.ts` option setup (4 lines) | Accepted intended behavior | Current test uses the shared concrete platform catalog and explicitly asserts MiniMax is present and composite is absent. |

## Actual omission corrected

The backend channel-monitor validation maps omitted `MonitorProviderMiniMax` even though
MiniMax had a registered chat adapter, frontend option, migration enum, and quota-mode
implementation. MiniMax therefore could not pass write-time provider validation. The
source fix adds MiniMax to both `monitorProviders` and `probeCapableProviders`; the
capability matrix test now asserts both `validateProvider(minimax)` and probe support.
The template provider error text was updated to match the supported provider contract.

## Result

After the correction, the documented baseline workflow recorded 730 unaccounted lines
with digest `de551038a225866f`; `--check-baseline` exits 0 and `debt-ledger --check`
reports 0 open / 11 closed with no reopened rows. The baseline update records reviewed
intentional June relocations; it does not suppress paths, reduce assertions, or alter
verifier semantics.
