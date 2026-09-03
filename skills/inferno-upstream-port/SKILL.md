---
name: inferno-upstream-port
description: Plan and verify taking commits from the sub2api upstream into the Inferno fork. Use when upstream has shipped work we have not taken, when deciding whether a commit can be copied or must be hand-merged, or when checking whether a port that has already been made is safe to ship. Read-only planning and independent verification; it never ports anything itself.
license: MIT
metadata:
  opencomputer:
    tags: [Inferno, Upstream, Porting, Review, Gates, Read-Only]
    related_skills: [inferno-dev-apply, eng-review]
---

# Inferno upstream port

Inferno is a fork of `Wei-Shaw/sub2api` whose frontend has been rewritten onto the
June design system. That rewrite is why taking an upstream commit is a judgement
call rather than a merge: for any file the June rewrite has touched, upstream's
lines no longer fit, and pasting them destroys work.

This skill covers the two ends of that job — **planning** a port and **verifying**
one. It does not perform ports. Every command below is read-only.

## The one rule that costs the most when broken

**A verbatim copy is safe only when the source commit descends from everything
already in our tree.**

On 2026-09-02 commit `aa7a811e6` measured *zero divergence* from upstream's
parent — provably carrying no June work, provably safe to copy. Copying it
silently reverted `7c01ec9be`, because the two sit on divergent branches and
neither is an ancestor of the other. `vue-tsc`, `june-lint` and `port-coverage`
all stayed green. Only upstream's own tests objected.

Zero divergence proves a file carries no June work. It says nothing about whose
lineage the replacement comes from. `port-prepare` checks ancestry first and
exits 1 rather than classify a batch it cannot vouch for.

## Planning a port

```bash
cd ~/OpenComputerV2/inferno
NODE_OPTIONS= node inferno-frontend/scripts/upstream-daily.mjs      # what shipped
NODE_OPTIONS= node inferno-frontend/scripts/port-prepare.mjs --pending
```

`port-prepare` classifies every file of every unported commit:

| verdict | meaning |
|---|---|
| `COPY` | byte-identical to upstream's parent — a wholesale copy destroys nothing |
| `HAND-MERGE` | diverges, or is a locale file — carries June work, apply hunks by hand |
| `NEW` | no counterpart here — take it whole |
| `DONE` | already matches upstream's post-image — nothing to do |

And each commit: `MECHANICAL` (all files copyable, ancestry clean),
`HAND` (needs judgement), `BLOCKED` (divergent ancestry — resolve first),
`DONE`.

**Exit 1 means BLOCKED. Do not copy anything in that batch.** Take upstream's
own merge of the divergent commits, or port each by hand additively — additive
hand-merges land on upstream's merged result correctly, which is exactly how
the types and locales survived on 09-02 while the copied files reverted.

## Verifying a port

Capture before the work, compare after:

```bash
cd ~/OpenComputerV2/inferno/inferno-frontend
NODE_OPTIONS= node scripts/port-verify.mjs --baseline /tmp/before.json
# ... the port happens ...
NODE_OPTIONS= node scripts/port-verify.mjs --against /tmp/before.json
```

It re-derives every number from the tree rather than trusting a report, and
exits 1 on `NO-SHIP`. The asymmetry it enforces:

- `conversion-status` may **not fall** — a fall means converted work was
  overwritten with upstream's Tailwind. Revert; do not investigate first.
- `behaviour-parity` may not gain gaps; `debt-ledger` may not reopen a row;
  the `port-coverage` set may not change; `tsc` and `vitest` must be clean.
- `june-lint` **may** rise — legitimate when porting verbatim into a file that
  is still Tailwind. It is reported as a NOTE needing a reason in the commit
  message, never an automatic block.

## Applying a build

Once a port is merged and the gateway should run it:

```bash
bash <this-skill-directory>/scripts/apply-verified.sh
```

It stamps the outgoing image as `inferno:rollback-<timestamp>`, builds,
recreates only the app service, waits for healthy, and then **exercises a real
authenticated round trip through hermes**. Success is `APPLY_STATUS=applied`;
anything else rolls back to the stamped image automatically and confirms the
previous build is serving again.

Health checks are deliberately not trusted. On 2026-09-03 a rebuild reported
every green signal it had — container healthy, `/health` 200, SPA served, auth
gate correct, zero errors — while OAuth token exchange returned 500 to every
client for hours, because the new code needed `server.frontend_url` as a JWT
issuer and nothing had set it. The container was answering a narrower question
than the one that mattered.

`APPLY_FORCE_FAIL=1` fails the first check on purpose so the rollback path can
be exercised. It exists to be tested, not used.

## What this skill will not do

- **It will not port.** No branch, no checkout, no copy, no commit. Its output
  is trustworthy precisely because it cannot have caused what it describes.
- **It will not advance the watermark.** `upstream-daily --record` means *seen
  and tracked*; running it before the manifest carries every row drops unported
  commits off the radar silently.
- **It will not decide.** `HAND` commits need someone who can read the enclosing
  code, know where June relocated a symbol, and tell a rewrite from a loss.
  Hand those back rather than guessing.

## Reporting

When reporting to a human, name the `REBUILD`/`HAND`/`NEW` commits individually
with the files they touch — those are the only ones that cost anything. Give
`MERGE` commits as a count; they arrive free with a backend merge and listing
them buries the signal. Lead with `BLOCKED` if there is any.
