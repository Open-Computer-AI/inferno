# Upstream Reconciliation Inventory

Date: 2026-09-09

Base: `origin/inferno-redesign` at `7e527b8eddd301f8765bd9634a8808915fcbea45`

Target: `upstream/main` at `98d86915becae9fe9491a91ffc6defd5235c8d2b`

Scope: exact final-tree reconciliation. `.github/**` is intentionally excluded. History was used only to recover intent, provenance, tests, and migration ordering.

## Machine-derived final-tree facts

| Surface | Result |
| --- | --- |
| `backend/**` | Tree-identical (`git diff --quiet` exit 0); already equivalent |
| `frontend/**` | Tree-identical (`git diff --quiet` exit 0); upstream mirror already at target |
| `backend/migrations/**` | Tree-identical; no migration replay or reordering required |
| `inferno-frontend/**` | Inferno-owned final tree; upstream deletion is not a safe port |
| `.github/**` | Excluded; Inferno does not use GitHub Actions as a gate |
| OpenAI Image 2.5 | Already accounted by merged PR #25 at base `7e527b8ed` |

## Final classification

### Already ported or equivalent

- Backend runtime, gateway, proxy, WebSocket, ops, usage, billing, payment, providers, accounts, groups, model catalog, generated Ent code, and migrations.
- Standard frontend mirror at the exact upstream target.
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

- Added the existing production-wide Inferno static i18n key-resolution test (`inferno-frontend/src/i18n/__tests__/keyResolution.spec.ts`) to `FRONTEND_CRITICAL_VITEST` in the root Makefile. This preserves the upstream final-state requirement that statically referenced locale keys are a mandatory build/test gate while using Inferno’s stronger existing full-path scanner.

## Deploy exceptions reviewed

The six deploy-tree differences are intentional:

- `deploy/Dockerfile`: 3072 MB heap retained as a conservative Inferno build policy.
- `deploy/config.example.yaml`: OAuth backing-key documentation retained because it describes Inferno runtime behavior; upstream pricing-comment improvements are already present.
- `deploy/docker-compose.local.yml`: local Inferno build, names/network, and `SERVER_FRONTEND_URL` retained.
- `deploy/inference/{README.md,bootstrap.sh,ecr-push-policy.json}`: Inferno-specific deployment assets retained; upstream absence is not deletion evidence.

## Evidence

- Exact backend tree and migration tree comparisons: no delta.
- Exact standard frontend tree comparison: no delta.
- `git diff --check`: required before commit and final acceptance.
- Critical i18n test must execute through `make test` after this inventory/gate change.
- No claim is made that read-only `check-divergence.sh`, debt-ledger, or dependency-dependent tests passed during analysis; final acceptance must run in the writable configured checkout.
