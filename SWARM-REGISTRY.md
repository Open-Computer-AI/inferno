# Inferno reconciliation status pointer

The current candidate, exact upstream snapshot, closed backend dispositions,
frontend status, verification results, and pending work are maintained only in
[`docs/superpowers/analysis/RECONCILIATION.md`](docs/superpowers/analysis/RECONCILIATION.md).

The previous June-rewrite agent registry is preserved as dated history at
[`docs/superpowers/analysis/archive/2026-09-23/SWARM-REGISTRY-20260829.md`](docs/superpowers/analysis/archive/2026-09-23/SWARM-REGISTRY-20260829.md).
Its agent counts and raw `diff -rq` command are not current upstream status or
proof of behavioral parity.

## 2026-09-04 registry entries from inferno-redesign (merged into stage 2026-10-25)

These rows were appended to the old June-rewrite registry on `inferno-redesign` after
the archived copy above was taken; kept here verbatim so the history is not lost.

---

## 2026-09-04 — landing-page removal + inference deploy skill (2 agents, parallel)

Two independent agents, no shared files. Spawned from the main thread; results
are reviewed and committed by the main thread, never self-merged.

| # | Purpose | Owns | Status | Distilled output |
|---|---------|------|--------|------------------|
| 1 | Remove the public landing page so `/` opens on login, mirroring `oc-router` | `inferno-frontend/src/router/`, `views/KeyUsageView.vue`, `views/public/LegalDocumentView.vue` | see below | working tree diff (uncommitted) |
| 2 | Capture the `inference.tryopencomputer.com` deploy + redeploy runbook as a skill | `skills/inferno-inference-deploy/` (new) | see below | `skills/inferno-inference-deploy/SKILL.md` + `scripts/` |

Agent 2 is explicitly **read-only against production** — it inspects
`i-0066a065c11a7b94d` and writes files locally; it deploys nothing and never
touches `i-0e4fe42fc3fadf277` (oc-router).

### Recover

Neither agent commits. If the session dies before review, the work survives as:

- agent 1 — uncommitted changes in `inferno-frontend/src`; `git diff` shows the
  whole of it. Re-derive the scope with
  `grep -rn "/home" inferno-frontend/src` (8 sites across 4 files) and the
  reference implementation at
  `../oc-router/frontend/src/router/index.ts` (same change, already made).
- agent 2 — files under `skills/inferno-inference-deploy/`. Source material is
  `deploy/inference/README.md` and `deploy/inference/bootstrap.sh`.

Live facts both depend on, so they need not be rediscovered:
AWS `133277694446` / us-east-1, creds only on `oc-internal`
(`ssh root@oc-internal "su - architsakri -c '<aws ...>'"`);
`i-0066a065c11a7b94d` = oc-inference (3.82.43.139),
`i-0e4fe42fc3fadf277` = oc-router (35.175.193.193, off-limits);
ECR repo `oc-platform/inferno`, linux/amd64; hermes profiles on oc-internal at
`/Users/architsakri/.hermes/profiles/<name>/skills/`.

**Outcome (2026-09-04).** Both agents completed. Agent 1's code change shipped
as `5e8da015a` — its two guard-test rewrites were rejected and redone by the
main thread (one was a duplicate of a test three lines above it; the other had
replaced the backend-mode allowlist's *negative* case with a plain-auth case
the block already covered). Agent 2's skill shipped as `cfdabf7c2` and is
installed and enabled in the `engineering-ops` hermes profile on oc-internal.

The redeploy itself is NOT done: `verify.sh`'s probe needs a working
`INFERNO_API_KEY`, and the bootstrap `DEFAULT_API_KEY` in the instance's
`config.yaml` returns `INVALID_API_KEY`. Running the deploy without one would
make the probe fail and auto-roll-back a perfectly good build. Resume with:

```bash
cd skills/inferno-inference-deploy
INFERNO_API_KEY=<key> INFERNO_PROBE_MODEL=claude-opus-5 ./scripts/verify.sh
INFERNO_API_KEY=<key> INFERNO_PROBE_MODEL=claude-opus-5 ./scripts/redeploy.sh
```
