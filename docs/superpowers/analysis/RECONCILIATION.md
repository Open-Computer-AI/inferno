# Inferno selective upstream reconciliation — live source of truth

Last refreshed: 2026-09-27. This file is the active status, disposition, and
verification ledger for the selective upstream reconciliation. Historical
inventories and run logs are archived under `archive/2026-09-23/`; they are
evidence, not active queues. `upstream-watch.json` is only a watcher watermark.
The canonical development and publication ref is GitHub `main`. The separate
GitHub `inferno` ref is a legacy snapshot and is not required to match `main`.

## Exact source snapshot

| Item | Verified value |
|---|---|
| Canonical repository origin | `https://github.com/Open-Computer-AI/inferno.git` |
| Candidate working branch | `main` |
| Candidate implementation commit | `e4beef144e924d0b21a34fa66dc349397cc862a1` |
| Protected baseline commit | `7d3a6099bfa5d14d95253de7ac1864a9b330040e` |
| GitHub legacy inferno SHA | `3dd60d21d05e4613a3ee4003e0ec2e75f35f99ee` |
| Local upstream/main SHA | `a3eb7ef302961cba716dc78b39b93b60c467db0e` |
| GitHub upstream/main SHA | `a3eb7ef302961cba716dc78b39b93b60c467db0e` |
| Merge base | `5097b31457e6dc9f49e5f5c9c72b925ce79543b3` |

The Mac's two reconciliation-ledger/verifier commits and GitHub `main`'s
channel-monitor privacy UI and historical attestation audit are now joined by a
normal merge at the implementation commit above. GitHub `main` is the only
canonical source ref. The legacy `inferno` branch remains at `3dd60d21`; it was
not moved because it may have external consumers. The merge does not rewrite or
replace history. The root `.worktrees/` directory is ignored as local workspace
state, not committed as source.

At refresh, upstream `main` was `a3eb7ef3` (Sub2API `0.2.8`), and the standard
`frontend/` mirror still exactly matches that tree. The protected baseline
commit remains an ancestor of the candidate. The separate local
`/Users/saksham/OpenComputerV2/inferno-local` checkout is not this repository or
the active candidate; it remains at the protected commit with local edits and
artifacts preserved. Its absolute path is intentionally not required by the
portable verifier.

Run `./scripts/verify-inferno-reconciliation.sh` from any clean clone of this
repository on branch `main`. It checks repository identity, branch, baseline
commit ancestry, the implementation snapshot, live GitHub `main` and legacy
`inferno` refs, local/live upstream refs and merge base, standard-frontend
parity, clean tracked/untracked status, and the nine closed backend rows below.
It does not require the separate protected checkout to exist. The verifier
checks repository freshness and state; it does not establish runtime behavior
or guarantee future upstream freshness.

## Current disposition

| Surface | Status | Evidence / boundary |
|---|---|---|
| Backend upstream behavior | **Reconciled through `a3eb7ef3`; no omitted upstream-only backend path found.** | Backend review covered 3,100 upstream backend paths and 3,207 candidate paths: 2,985 byte-identical shared paths, 115 locally adapted shared paths, and 107 Inferno-specific paths. OAuth, backing-key routing, Razorpay/billing, refresh-token protections, GPT Live sideband/session identity, and local settings remain represented. The only change after `fd80b08c` was the version marker, now `0.2.8`. |
| Standard `frontend/` mirror | **Matches upstream/main and is tracked in the implementation commit.** | `git diff --quiet upstream/main -- frontend` passed. The three OpenCode Go files previously present but untracked are included in `c219bebb`. |
| June `inferno-frontend/` | **Current-tip comparison complete; Inferno design retained.** | Parity audit: 0 missing files; 1,049 upstream test cases examined across 148 touched specs; 22 remaining test-count shortfalls. Every shortfall was classified: two actual behavior mismatches were fixed with focused tests; the remaining 20 are test-count-only, consolidated/relocated coverage, or intentional design differences. No full June UI replacement was performed. |
| June channel-monitor privacy setting | **Present on canonical `main`.** | The upstream ranking-privacy option is implemented in the June admin settings UI, English/Chinese locales, and focused settings tests. This was merged from GitHub `main` during the 2026-09-27 reconciliation. |
| Live-attestation audit | **Historical documentation, not a runtime check.** | `docs/superpowers/analysis/2026-09-11-live-attestation-audit.md` records the previous source audit; it does not attest the current host or prove a Live call works. |
| June user-status toggle | **Fixed.** | When the list is idle, apply the API-returned status and `updated_at` in place. If a list load is active, abort/refetch so stale data cannot overwrite the mutation. Focused tests cover both paths. |
| June channel-status timer | **Fixed.** | Reload completion now calls `autoRefresh.resetCountdown()`, preserving the selected interval instead of reverting to the default. Focused test covers a 120-second selection. |
| Local app runtime smoke | **Not run.** | Read-only inspection found `127.0.0.1:3000` belongs to `athena-editor-reference-layout`, not Inferno; nothing was listening on `:8080`. Existing `inferno-local-postgres` and Redis containers are persistent local state and were not used, restarted, or altered. The login/OAuth-refresh/key/account UI/OpenAI/Anthropic/GPT Live sideband smoke therefore remains unverified against a full isolated app stack. |
| Production / canonical data | **Not touched.** | The repository integration test used its isolated test database/container. No production or canonical database, credentials, Docker app service, VM, or deployment was used or changed. |

### Backend behavior groups represented through `a3eb7ef3`

1. OpenCode Go usage windows and account-group refresh.
2. Claude Code runtime-version synchronization.
3. GPT-6 Sol/Luna and Claude Opus 5.5 model capability/pricing support.
4. Backup archive retention and backup safety.
5. Gemini image routing and incompatible-account rejection.
6. Account/model selection, scheduler, picker, and API-key failover behavior.
7. Rolling log retention.
8. Affiliate offline withdrawal and idempotency.
9. Protocol-correct Responses stream errors.
10. Antigravity identity rewriting.
11. Invalid `null` entries in tool-schema `required` cleanup.
12. Simple-mode API-key spending windows.

Earlier backend groups, including OAuth preservation, Codex/Responses/sideband,
continuation, billing/quota/failover, and proxy attribution, remain in the
candidate. No wholesale upstream replacement was applied to those surfaces.

### Closed former unresolved backend rows

These nine SHAs are closed; the former inventory's `review` labels are not open
work:

| Upstream SHA | Disposition | Exact conclusion |
|---|---|---|
| `c4e6dcfd8` | CLOSED — already represented | Pinned Codex manifest payload, migration, and projection behavior already exist with focused manifest/group tests. |
| `55c5eed9f` | CLOSED — already represented | Group repository integration fixtures already persist model allowlists; this row adds fixture coverage, not a missing runtime feature. |
| `32bf3d0f3` | CLOSED — already applied | Grok cross-client mapping defaults to enabled; candidate `d4d38f0eb` applied the comment/default clarification and focused tests. |
| `88b697d5d` | CLOSED — already represented | `PgDumper.Dump` holds the migration advisory lock through dump completion and reader close. Restore locking is a separate design question, not part of this upstream change. |
| `10940ecf3` | CLOSED — no behavior | MiniMax allowlist comment/gofmt alignment only; no runtime behavior was missing. |
| `52f7bcaed` | CLOSED — already represented | CMv2 user-ranking hide setting/API/admin behavior and user-tab suppression are present with focused Go/Vitest coverage. |
| `e5e04de13` | CLOSED — already represented | Upstream config defaults and corresponding model/config tests are present. |
| `f78c4b241` | CLOSED — duplicate wrapper | Compact-model default is represented by candidate `af67f6a7a` / source `489968fd7`; the merge wrapper adds no independent behavior. |
| `7403a0117` | CLOSED — already represented | Reminder tags do not bypass moderation keyword checks; the candidate includes input-path behavior and a regression test. |

### OpenCode usage safety disposition

- Mounted-account eligibility is restricted to the official OpenCode Go host and
  supported paths. Refresh uses the fixed
  `https://opencode.ai/zen/go/v1/usage` endpoint, disables redirects, and does
  not treat `base_url` as an arbitrary bearer-token destination.
- Usage is keyed by the official subscription API key, not proxy ID. A proxy is
  only an egress route; changing egress invalidates the cached snapshot while
  retaining the operator's auto-refresh preference. Account-key/group changes
  clear the managed snapshot so state cannot follow a new key.
- Focused service/repository tests cover URL validation, identity changes,
  snapshot ownership, proxy fallback/expiry, and OpenCode activity scheduling.

## Final verification evidence

Commands were run on the committed candidate source before the ledger-only
commit:

- Backend: `go test -tags=unit ./...` — pass; `go vet -tags=unit ./...` — pass.
- Isolated DB: `go test -tags=integration ./internal/repository -count=1 -timeout=10m` — pass (15.6s); testcontainers isolation, not the persistent Inferno DB.
- Standard frontend: `pnpm run test:run` — 332 files / 2,484 tests pass; `pnpm run typecheck`, `pnpm run lint:check`, and direct Vite production build — pass.
- June frontend: `pnpm run test:run` — 362 files / 2,572 tests pass; `pnpm run typecheck`, `pnpm run lint:check`, and direct Vite production build — pass.
- June focused regression run: 2 files / 9 tests pass; independent read-only review found no correctness or test-quality blocker.
- Standard frontend comparison against `upstream/main` — exact match.
- `git diff --check` and `git diff --cached --check` — pass before implementation commit.
- Build outputs were directed under `/tmp`; no tracked or staged build output was included.

Both Vite builds emit existing advisories (stale Browserslist data, dynamic/static
import chunking, and large chunks); builds exit successfully. Full June lint is
green, so the older recorded 10-error/1-warning result is stale and no longer
describes this candidate.

## 2026-09-27 post-merge verification

The GitHub-`main` merge was checked after preserving both sides of the divergence:

- June frontend full suite: 362 files / 2,573 tests — pass, including the channel
  monitor privacy-setting tests.
- `pnpm run lint:check` — pass.
- `pnpm run typecheck` — pass.
- `pnpm run build` — pass. It emitted the existing stale-Browserslist,
  mixed-import, and large-chunk advisories. Vite regenerated the ignored embedded
  bundle under `backend/internal/web/dist`; no app process, container, database,
  or deployment was touched.
- `bash -n scripts/verify-inferno-reconciliation.sh` and `git diff --check` — pass.

These are build/test checks, not the separate isolated-runtime smoke listed above.

## Remaining limitation

The implementation and automated reconciliation checks are complete and
committed. The only unverified acceptance item is the live local app smoke for
login, OAuth refresh, key/account UI, OpenAI/Anthropic, and GPT Live sideband.
It requires a full app runtime wired to isolated data plus suitable test/mock
provider accounts; the currently running local resources are not that isolated
fixture. Do not use the persistent local DB or production to substitute for it.
