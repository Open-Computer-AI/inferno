# Upstream Reconciliation Inventory

Date: 2026-09-09

Base: `origin/inferno-redesign` at `7e527b8eddd301f8765bd9634a8808915fcbea45`

Target: `upstream/main` at `98d86915becae9fe9491a91ffc6defd5235c8d2b`

Scope: exact final-tree reconciliation. `.github/**` is intentionally excluded. History was used only to recover intent, provenance, tests, and migration ordering.

## Machine-derived final-tree facts

| Surface | Result |
| --- | --- |
| `backend/**` | Tree-identical (`git diff --quiet` exit 0); already equivalent |
| `frontend/**` | Runtime source is target-equivalent; package manifest/lockfile deliberately diverge for the security repair below |
| `backend/migrations/**` | Tree-identical; no migration replay or reordering required |
| `inferno-frontend/**` | Inferno-owned final tree; upstream deletion is not a safe port |
| `.github/**` | Excluded; Inferno does not use GitHub Actions as a gate |
| OpenAI Image 2.5 | Already accounted by merged PR #25 at base `7e527b8ed` |

## Final classification

### Already ported or equivalent

- Backend runtime, gateway, proxy, WebSocket, ops, usage, billing, payment, providers, accounts, groups, model catalog, generated Ent code, and migrations.
- Standard frontend runtime source at the exact upstream target before the dependency-only security repair.
- OpenAI Image 2.5 routing, pricing, OAuth override, tests, whitelist, version, and Compose/env wiring.

### Inferno-specific; retain

- `inferno-frontend/**`: June design system, accessibility, routing, components, product behavior, reconciliation scripts, and regression tests.
- `GOAL.md`, `INFERNO-BUILD.md`, `SPEC.md`, `docs/superpowers/**`, and `skills/inferno-*/**`.
- `deploy/inference/**`, local Inferno image/service/network names, `SERVER_FRONTEND_URL`, OAuth backing-key policy, and the 3072 MB frontend build heap.

### Superseded or intentionally excluded

- Upstream deletion/replacement of `inferno-frontend/**` with `frontend/**`.
- Upstream deployment branding/image/network changes that would replace Inferno operational policy.
- Upstream deletion of Inferno’s inference deployment assets.
- `.github/**` workflows.

### Ported in this final remainder

- Added an Inferno-native locale completeness test (`inferno-frontend/src/i18n/__tests__/localeKeyCompleteness.spec.ts`) alongside the existing filename-reporting `keyResolution.spec.ts` in `FRONTEND_CRITICAL_VITEST`. Together they preserve complementary coverage for single-, double-, and template-quoted `t`, `$t`, `i18n.t`, `i18n.global.t`, metadata/keypath references, `<i18n-t>`, English/Chinese schema equality, non-empty locale leaves, and static production references.
- Wired the locale completeness test into `inferno-frontend`'s direct production build, matching the upstream build-gate contract rather than relying only on the root Makefile.
- Raised Vitest's test and hook timeout to 30 seconds in both frontend trees. This preserves assertions while accommodating the locale compilation/source-discovery tests and existing async tests under the upgraded Vitest runtime.
- Updated both frontend package manifests and lockfiles to patched compatible Vite/Vitest and transitive dependency versions. Live audit fell from 31 high plus 1 critical finding to the two known `xlsx` highs for which npm publishes no patched package; those remain under the unexpired admin-export exceptions.
- Updated the Compose security scanner to recognize both the upstream `sub2api` service and the deliberate Inferno-local `inferno` service name.

## Deploy exceptions reviewed

The six deploy-tree differences are intentional:

- `deploy/Dockerfile`: 3072 MB heap retained as a conservative Inferno build policy.
- `deploy/config.example.yaml`: OAuth backing-key documentation retained because it describes Inferno runtime behavior; upstream content-hash hot-reload documentation is ported alongside it.
- `deploy/docker-compose.local.yml`: local Inferno build, names/network, and `SERVER_FRONTEND_URL` retained.
- `deploy/inference/{README.md,bootstrap.sh,ecr-push-policy.json}`: Inferno-specific deployment assets retained; upstream absence is not deletion evidence.

## Evidence

- Exact backend tree and migration tree comparisons: no delta.
- Exact standard frontend runtime-source comparison: no delta; `package.json` and `pnpm-lock.yaml` differ only for the documented security repair.
- `git diff --check`: required before commit and final acceptance.
- Critical i18n test must execute through `make test` after this inventory/gate change.
- No claim is made that read-only `check-divergence.sh`, debt-ledger, or dependency-dependent tests passed during analysis; final acceptance must run in the writable configured checkout.
