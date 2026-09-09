# 1. HEADER

- reqid: e9f71e23
- repo: /Users/architsakri/OpenComputerV2/inferno/.worktrees/inferno-minimax-20260908
- branch: port/inferno-minimax-20260908
- BASE_SHA: 9b1673a10aaa9e5f96c8a1233cdd5fb531a78c2a
- date: 2026-09-08
- COMPONENT: general
- upstream merge: dbe92a1c241a03c77e2b218761369ff988b8356b
- semantic feature commits: 19382f275e8fdc05a655e3e20fbd688bb9a7ec2a, 6f2295bfc7a29367e3a300909c75bc79720804f1, 3495635a52bb0009ad15b4a6990ac3d2b1c43811, 10940ecf39bf67000c668b07facd6321ec7fb21e
- merge-side version commit: 772a0382f079676983c06f24b0d41e09139a8462 (already represented by BASE version 0.2.3; do not replay independently)

PRIOR LESSONS
- UNAVAILABLE: the pinned lessons utility rejected its documented `--repo` interface. No retry was made under the planning budget.
- Repository port evidence says divergent ancestry forbids whole-tree or commit copying even when a file appears zero-divergence. Additive hand-porting or the upstream merge post-image is required.
END PRIOR LESSONS

# 2. PROBLEM

At BASE_SHA, MiniMax model-specific pricing and protocol handling exist, but MiniMax is not a first-class account, group, composite-routing, quota-monitoring, or scheduling-threshold platform. The upstream merge adds that platform contract across database constraints, generated/runtime backend surfaces, tests, the pristine frontend, and product UI. Inferno must expose the same MiniMax behavior without replacing June-designed `inferno-frontend` files with upstream markup. The observable delta is that operators can configure MiniMax PAYG or Coding accounts, route MiniMax models through ordinary and composite groups, inspect Coding-plan quota, and apply threshold stopping while existing platforms and June UI behavior remain intact.

# 3. GOALS / NON-GOALS

Goals:
- Accept `minimax` everywhere the backend platform contract permits a concrete CN provider, including persisted constraints and generated schema metadata.
- Route MiniMax/ABAB models and expose MiniMax defaults through ordinary and composite groups without changing other platform selection.
- Support MiniMax CN/international PAYG and Coding credentials, native protocol endpoints, quota parsing, monitoring, and scheduling thresholds.
- Expose the same behavior in `inferno-frontend` using its existing June components/tokens and preserve `frontend/` as the pristine upstream mirror.
- Prove the stateful migration, backend behavior, frontend behavior, and mirror/June preservation independently.

Non-goals:
- No production deploy, migration execution, watermark advance, unrelated pending upstream commits, or MiniMax live-network calls.
- No redesign of composite scheduling, quota abstractions, June components, or existing provider UX.
- No wholesale copy from `frontend/` into `inferno-frontend/`.
- No replay of the unrelated 0.2.3 version commit; BASE already contains that version.

# 4. RISK

RISK: HIGH
- Blast radius: HIGH — public platform unions and persisted CHECK constraints span shared routing, account, monitoring, quota, and UI surfaces (upstream post-image touches 109 behavior files plus the merge).
- Reversibility: HIGH — migration 237 widens live database constraints; rollback compatibility must be explicit.
- Novelty: MEDIUM — MiniMax follows the existing Kimi/Zhipu/DeepSeek CN-provider pattern, but adds a new persisted platform and quota response parser.

# 5. ALTERNATIVES

1. Selected: pin backend plus pristine `frontend/` paths to upstream merge `dbe92a1c`, then hand-port only MiniMax semantics into corresponding `inferno-frontend/` paths; most consistent and auditable, with bounded June risk.
2. Cherry-pick the four feature commits and resolve conflicts in place; rejected because ancestry is divergent and can silently revert Inferno-only backend/frontend decisions.
3. Reimplement a narrow backend-only MiniMax provider first; rejected because it leaves the user-visible account/group contract incomplete and creates a second incompatible rollout.

# 6. APPROACH

Conflict-resolution contract:
1. Treat `dbe92a1c` as the authoritative merged MiniMax post-image; do not apply `772a0382f` separately and do not cherry-pick any source commit.
2. Pin every changed `backend/**` path in `git diff --name-only 9b1673a10..dbe92a1c -- backend` to the exact `dbe92a1c` blob, including generated Ent files and `backend/migrations/237_add_minimax_platform.sql`; retain BASE-only files not named by that diff. The migration number is 237, not the feature branch's earlier 235.
3. Pin every changed `frontend/**` path in that same diff to the exact `dbe92a1c` blob. `frontend/` remains the pristine reference mirror and is not the shipped product frontend.
4. For each changed `frontend/src/**` path, inspect the matching `inferno-frontend/src/**` file and add only the MiniMax semantic delta. Preserve Inferno-only code, June component structure, design tokens, tests, accessibility, and local naming. Never replace a product file wholesale based only on a COPY verdict: `port-prepare` reports the batch BLOCKED by divergent ancestry.
5. Add MiniMax to the established CN-provider contract: domain constants/default URLs, account credentials and validation, concrete/composite routing, model detection/defaults, channel monitor provider/quota mode, Coding-plan quota fetch/parser, scheduling thresholds, error passthrough, API allowlist, and tests. Preserve the existing composite-group scheduler; do not introduce account selection UI.
6. MiniMax quota semantics are: Bearer auth; CN `api.minimaxi.com` and international `api.minimax.io`; `model_name=general`; used percent is `100 - remaining`; 5h and active weekly windows; seconds or milliseconds reset timestamps normalized to RFC3339; nonzero `base_resp.status_code` is a probe error, not success.
7. In `inferno-frontend`, translate account create/edit, credentials builder/presets, quota display, monitor filters/forms, platform icons/badges/colors, group/channel/proxy options, user usage, model whitelist, locales, and tests. Use existing June controls/classes at each target; upstream Tailwind is evidence, not copy material.
8. Keep `docs/COMPOSITE_GROUPS.md` unchanged in this slice: it is upstream documentation outside the pinned backend/mirror/product contract and its MiniMax statement is covered by executable tests.

Exact path groups:
- Backend pin: all 64 paths (62 modified plus new `backend/migrations/237_add_minimax_platform.sql` and `backend/migrations/minimax_platform_migration_test.go`) reported by `git diff --name-status 9b1673a10..dbe92a1c -- backend`.
- Pristine mirror pin: all 44 paths reported by `git diff --name-only 9b1673a10..dbe92a1c -- frontend`.
- Product hand-port: corresponding 44 paths under `inferno-frontend/src/`, with no file-wide replacement; preserve any additional Inferno tests and components.

ASSUMPTIONS:
- The source range means the four MiniMax feature commits as merged by `dbe92a1c`; `772a0382f` is merge-side version history already present at BASE and is not a second deliverable.
- Existing provider/platform registries are the convention; no feature flag exists for adding a concrete provider, so rollout is additive and immediately available after migration.
- Exact backend pinning is authorized by the request; frontend pinning applies only to pristine `frontend/`, never wholesale to `inferno-frontend/`.

# 7. TASKS

1. Backend post-image pin — apply exact `dbe92a1c` blobs for the named backend diff, retain unrelated BASE files, and record the path manifest; satisfies AC1, AC2, AC3.
2. Stateful platform contract — verify migration 237, Ent schema/generated allowlists, constants, API validation, and additive rollback compatibility; satisfies AC1.
3. Routing/quota/scheduler behavior — preserve the composite scheduler while adding MiniMax detection, model exposure, account modes, quota parsing/monitoring, and threshold evaluation; satisfies AC2 and AC3.
4. Mirror sync — pin the 44 named `frontend/` paths exactly to `dbe92a1c` without touching unrelated mirror paths; satisfies AC4.
5. Product hand-port — translate MiniMax semantics into matching `inferno-frontend` modules with June structure/tokens and targeted tests; satisfies AC5 and AC6.
6. Luna verification bundle — run the named backend/frontend tests plus full repository gates and `port-verify`; attach commands, exit codes, changed-path manifest, BASE/candidate comparison, and limitations for independent Sol review/security and Luna eval; satisfies AC1-AC6.

# 8. ACCEPTANCE CRITERIA

Planning did not execute behavioral tests, per task instruction. Backend commands run from `backend/`; all other commands run from the repository root. The criteria use only existing Inferno toolchain commands and actual upstream/product test paths. No generic test wrapper may be added.

| id | command | baseline expected/observed | proves |
|---|---|---|---|
| AC1 | `go test ./migrations` | RED(missing-test) / not run | actual upstream `minimax_platform_migration_test.go` proves migration 237 widens all persisted constraints to MiniMax and remains additive for old rows |
| AC2 | `go test ./internal/service ./internal/handler/...` | RED / not run | existing upstream tests prove MiniMax endpoints, quota/error parsing, model detection, ordinary/composite routing, scheduling thresholds, defaults, and gateway behavior |
| AC3 | `git diff --exit-code dbe92a1c241a03c77e2b218761369ff988b8356b HEAD -- backend frontend` | RED / not run | every backend and pristine `frontend/` path equals the authoritative merged MiniMax post-image, with no invented parity wrapper |
| AC4 | `pnpm --dir inferno-frontend exec vitest run src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/credentialsBuilder.cnAdaptive.spec.ts src/views/admin/__tests__/GroupsView.compositePlatforms.spec.ts src/views/admin/__tests__/channelPlatformOptions.spec.ts` | RED / not run | actual product tests prove MiniMax PAYG/Coding endpoints and ordinary/composite platform exposure while retaining Inferno test structure |
| AC5 | `pnpm --dir inferno-frontend exec vue-tsc --noEmit` | GREEN / not run | the additive product hand-port remains type-correct across the June frontend |
| AC6 | `NODE_OPTIONS= node inferno-frontend/scripts/port-verify.mjs --against /tmp/inferno-minimax-port-baseline-9b1673a10.json` | GREEN / baseline captured with degraded npm probes | conversion, parity, debt, coverage, type, and Vitest metrics do not regress relative to the captured pre-port baseline |

# 9. TEST STRATEGY

- Add the actual upstream `backend/migrations/minimax_platform_migration_test.go` and preserve/port MiniMax cases in existing upstream Go tests, especially `backend/internal/service/cn_providers_test.go`, `backend/internal/service/composite_platform_test.go`, `backend/internal/service/channel_monitor_quota_fetcher_test.go`, `backend/internal/service/channel_monitor_quota_mode_test.go`, `backend/internal/handler/composite_platform_test.go`, and `backend/internal/handler/gateway_models_test.go`.
- Add MiniMax cases to the existing Inferno test files named by AC4; do not invent parallel MiniMax-only wrapper tests unless the implementation exposes behavior those files cannot exercise.
- Existing upstream tests changed by the post-image guard account/platform allowlists, scheduler snapshot cardinality, quota modes, gateway model listings, composite routes, error rules, monitor validation, and migration SQL. Existing Inferno tests guard June controls, locale keys, account modal behavior, group options, monitor presentation, and quota rendering.
- Luna build must additionally run the repository's real gates: `make test` and `make build`, then rerun AC6 against `/tmp/inferno-minimax-port-baseline-9b1673a10.json`. Record exact commands and exit codes; these are verification evidence.
- The baseline capture exited 0 but its internal `npx vue-tsc`/Vitest probes could not use the root-owned npm cache, so `testFiles` and `tests` are null. AC4 and AC5 are therefore mandatory independent gates; evaluator must not interpret AC6 alone as behavioral or typecheck evidence.
- Changed-path lint/type scope is all hand-ported `.ts` and `.vue` files under `inferno-frontend/src`; the full `make test` invokes lint, typecheck, critical Vitest, backend Go tests, and golangci-lint.
- macOS-direct evaluation is the current authorized mode. A fresh Luna evaluator reruns the allowlisted local checks read-only, records BASE/candidate and exit codes, and states that the result is not Linux-authoritative attestation.
- Sol review checks this SPEC, exact source/mirror manifests, the branch diff, June preservation, and test evidence. A separate fresh Sol security review checks SSRF allowlists/custom base URLs, bearer-secret handling/logging, migration safety, parser bounds/error semantics, and that no production credentials or live MiniMax calls were used.
- existing tests are immutable — weakening or deleting one is an automatic REJECT

# 10. ROLLOUT / ROLLBACK

- Deployment is only through the fixed root-owned `/opt/opencomputer/deploy-control/eng_deploy` launcher after the human approval gate. This planning card performs no deploy.
- Revert unit: the ordered 1-3 build commits produced for this SPEC, identified in the build handoff; no source commit is cherry-picked independently. Recovery is `git revert <commit>` for the entire unit, full pipeline rerun, and a FRESH human gate; there is no fast-rollback lane.
- Stateful compatibility window: migration 237 is additive-only for existing rows but new `minimax` rows become unreadable by reverted code because old CHECK constraints and older binaries do not recognize the platform. Therefore code rollback is safe only before any MiniMax account, route, quota, or monitor row is written, or after those rows are removed/translated under a separately approved data operation. Never automatically restore the narrower constraints while MiniMax rows exist.
- Down path is not exercised by an automated criterion because destructive deletion/constraint narrowing would violate this request. AC1 proves the additive up-path only. FOR-HUMAN must treat any post-adoption rollback as a data-touch requiring separate approval.
- Soak signals after an approved deployment: migration success; zero CHECK/unknown-platform errors; MiniMax account create/edit success; MiniMax gateway success/error/latency split; quota probe success and parse-error rates by CN/international host; threshold pause/resume events; composite scheduler availability and no change in other-platform selection; frontend console/API errors; `port-verify` conversion/parity/debt/coverage deltas.

# 11. OUT OF SCOPE

- The 35 other pending/divergent upstream commits surfaced by `port-prepare` and any watermark update.
- Production deployment, live credentials, live MiniMax API probing, destructive rollback, or database cleanup.
- Upstream `docs/COMPOSITE_GROUPS.md`, unrelated VERSION history, broad frontend reconciliation, design-system conversion, and refactors.
- Linux-authoritative attestation; macOS-direct eval is explicitly a limited local gate.
