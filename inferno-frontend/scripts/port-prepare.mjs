#!/usr/bin/env node
/**
 * port-prepare — everything up to the first judgement call, then stop.
 *
 * WHY THIS EXISTS: porting an upstream commit is roughly twenty minutes of
 * setup and then some minutes of actual thought. The setup is identical every
 * time and was done by hand every time, which is how the one check that
 * mattered got skipped.
 *
 * On 2026-09-02, aa7a811e6 measured ZERO divergence from upstream's parent --
 * provably carrying no June work, provably safe to copy. Copying it silently
 * reverted 7c01ec9be outright, because the two commits sit on divergent
 * branches and neither is an ancestor of the other. vue-tsc stayed clean,
 * june-lint stayed clean, port-coverage would have read 100%. Only upstream's
 * own tests objected.
 *
 * The lesson is narrow and mechanical: 0-divergence proves a file carries no
 * June work. It says NOTHING about whose lineage the replacement comes from.
 * So ancestry is checked FIRST here, before any per-file classification is
 * even printed, and a failure is fatal rather than advisory.
 *
 * WHAT IT DOES NOT DO: edit anything. No branch, no checkout, no copy, no
 * commit. It reads and it reports. Every mutation this could perform is one a
 * human or a reviewed agent should perform deliberately, and the whole value
 * of the tool is that its output is trustworthy precisely because it cannot
 * have caused what it is describing.
 *
 *   node scripts/port-prepare.mjs <sha> [<sha>...]   plan these commits
 *   node scripts/port-prepare.mjs --pending          plan everything unported
 *   node scripts/port-prepare.mjs --json out.json    machine-readable plan
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const OURS = resolve(ROOT, 'inferno-frontend')
const STATE = resolve(ROOT, 'docs/superpowers/analysis/upstream-watch.json')

const git = (args, allowFail = true) => {
  try {
    // stderr piped, not inherited: `git show <sha>^:<path>` legitimately fails
    // for a file the commit ADDED, and that fatal: line is noise in a report.
    return execFileSync('git', args, {
      cwd: ROOT, encoding: 'utf8', maxBuffer: 128 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
  } catch (e) {
    if (allowFail) return ''
    throw e
  }
}
const arg = (f) => { const i = process.argv.indexOf(f); return i > -1 ? process.argv[i + 1] : null }
const has = (f) => process.argv.includes(f)

// ---------------------------------------------------------------- inputs

/*
 * The vendor point is located by SUBJECT, never by hash: rebases rewrote it,
 * and a hardcoded hash would silently resolve to nothing and make every file
 * look unchanged -- which classifies everything as copy-safe, the exact
 * failure that lost four locale files on 2026-08-11.
 */
const VENDOR = git(['log', '--format=%H', '-1', '--grep=vendor upstream frontend as the redesign target']).trim()
if (!VENDOR) {
  console.error('cannot locate the vendor-point commit by subject — every classification below would be a guess. Refusing to plan.')
  process.exit(2)
}

const ourChanged = new Set(
  git(['diff', '--name-only', `${VENDOR}..HEAD`, '--', 'inferno-frontend'])
    .split('\n').filter(Boolean).map((f) => f.replace(/^inferno-frontend\//, ''))
)
if (!ourChanged.size) {
  console.error('read 0 files changed since the vendor point — that would classify the whole tree copy-safe. Refusing to plan.')
  process.exit(2)
}

let shas = process.argv.slice(2).filter((a) => /^[0-9a-f]{7,40}$/.test(a))
if (has('--pending')) {
  const from = existsSync(STATE) ? JSON.parse(readFileSync(STATE, 'utf8')).lastSeen : null
  const base = from || git(['merge-base', 'HEAD', 'upstream/main']).trim()
  shas = git(['log', '--no-merges', '--format=%H', `${base}..upstream/main`, '--', 'frontend/'])
    .split('\n').filter(Boolean).reverse()
}
if (!shas.length) {
  console.log(has('--pending')
    ? 'nothing pending — every upstream commit since the watermark is already taken.'
    : 'no commits given. Use:  port-prepare.mjs <sha>...   or   --pending')
  process.exit(0)
}

// ---------------------------------------------------------------- ancestry

/*
 * THE GATE. A verbatim copy replaces our file with the source commit's version.
 * That is safe only when the source descends from everything already in our
 * tree -- otherwise the copy carries the source branch's state, including the
 * ABSENCE of work that reached us another way.
 *
 * Checked pairwise across the batch as well as against HEAD, because a batch
 * can contain two commits from divergent branches even when each is
 * individually fine against HEAD. That is precisely the 7c01ec9be / aa7a811e6
 * shape.
 */
/** True when `a` is an ancestor of `b`. git exits 1 for "no", which is not an error. */
const isAncestor = (a, b) => {
  try { execFileSync('git', ['merge-base', '--is-ancestor', a, b], { cwd: ROOT, stdio: 'ignore' }); return true }
  catch { return false }
}

const ancestry = shas.map((sha) => ({
  sha,
  inHead: isAncestor(sha, 'HEAD'),
  // Commits in this batch that share no ancestry with `sha` in either direction.
  conflicts: shas.filter((o) => o !== sha && !isAncestor(sha, o) && !isAncestor(o, sha)).map((o) => o.slice(0, 9)),
}))

const divergent = ancestry.filter((a) => a.conflicts.length)

// ---------------------------------------------------------------- classify

const isLocale = (f) => f.includes('/i18n/locales/')
const norm = (s) => s.replace(/\s+/g, ' ').trim()

const plan = shas.map((sha) => {
  const meta = git(['show', '-s', '--format=%h%x00%ad%x00%s', '--date=short', sha]).trim().split('\0')
  const files = git(['show', '--name-only', '--format=', sha, '--', 'frontend/']).split('\n').filter(Boolean)
  const rows = []
  for (const f of files) {
    const rel = f.replace(/^frontend\//, '')
    const ourPath = join(OURS, rel)
    const parent = git(['show', `${sha}^:${f}`])

    if (!existsSync(ourPath)) { rows.push({ rel, action: 'NEW', why: 'no counterpart in our tree — take it whole' }); continue }
    if (isLocale(rel)) { rows.push({ rel, action: 'HAND-MERGE', why: 'locale — accumulates June-only keys a diff cannot reveal' }); continue }
    if (!parent) { rows.push({ rel, action: 'NEW', why: 'file did not exist upstream before this commit' }); continue }

    const ours = readFileSync(ourPath, 'utf8')

    /*
     * Already applied? Compare against the POST image first.
     *
     * Without this the tool is actively misleading on any commit already
     * ported: our file legitimately no longer matches upstream's PARENT, so
     * every file reads HAND-MERGE and the commit looks like unstarted work.
     * Checked before divergence for exactly that reason.
     */
    const post = git(['show', `${sha}:${f}`])
    if (post && norm(ours) === norm(post)) {
      rows.push({ rel, action: 'DONE', why: 'already matches upstream\'s post-image — this file is ported' })
      continue
    }

    const identical = norm(ours) === norm(parent)
    if (identical && !ourChanged.has(rel)) {
      rows.push({ rel, action: 'COPY', why: 'byte-identical to upstream\'s parent and untouched since the vendor point' })
    } else if (identical) {
      rows.push({ rel, action: 'COPY', why: 'we changed it, but it currently matches upstream\'s parent exactly' })
    } else {
      rows.push({ rel, action: 'HAND-MERGE', why: 'diverges from upstream\'s parent — carries June work' })
    }
  }
  const anc = ancestry.find((a) => a.sha === sha)
  const copyable = rows.filter((r) => r.action === 'COPY').length
  const hand = rows.filter((r) => r.action === 'HAND-MERGE').length
  const fresh = rows.filter((r) => r.action === 'NEW').length
  const done = rows.filter((r) => r.action === 'DONE').length
  return {
    sha: meta[0], date: meta[1], subject: meta[2],
    ancestry: anc, rows, copyable, hand, fresh, done,
    verdict: anc.conflicts.length ? 'BLOCKED'
      : done === rows.length && rows.length ? 'DONE'
      : hand ? 'HAND' : 'MECHANICAL',
  }
})

// ---------------------------------------------------------------- baseline

const gates = has('--no-baseline') ? null : (() => {
  const run = (cmd, args) => { try { return execFileSync(cmd, args, { cwd: OURS, encoding: 'utf8', maxBuffer: 32e6, env: { ...process.env, NODE_OPTIONS: '' } }) } catch (e) { return e.stdout || '' } }
  const one = (out, re) => (out.match(re) || [])[0] || '?'
  return {
    conversion: one(run('node', ['scripts/conversion-status.mjs']), /conversion: .*/),
    juneLint: one(run('node', ['scripts/june-lint.mjs']), /june-lint: .*/),
    parity: one(run('node', ['scripts/behaviour-parity.mjs']), /\d+ missing file.*/),
    debt: one(run('node', ['scripts/debt-ledger.mjs', '--check']), /\d+ open, \d+ closed.*/),
    coverage: one(run('node', ['scripts/port-coverage.mjs', '--check-baseline']), /\d+ unaccounted.*/),
  }
})()

// ---------------------------------------------------------------- report

if (has('--json')) writeFileSync(arg('--json'), JSON.stringify({ vendor: VENDOR, plan, gates }, null, 2))

console.log(`\nport-prepare · ${plan.length} commit(s) · vendor point ${VENDOR.slice(0, 9)}\n`)

if (divergent.length) {
  console.log('BLOCKED — divergent branches in this batch\n')
  for (const d of divergent) {
    console.log(`  ${d.sha.slice(0, 9)} shares no ancestry with ${d.conflicts.join(', ')}`)
  }
  console.log('\n  Neither is an ancestor of the other, so a verbatim copy of one can revert')
  console.log('  the other even when both measure 0 divergence. Take upstream\'s own merge')
  console.log('  of them instead, or port each by hand additively. Do NOT copy either.\n')
}

for (const p of plan) {
  const tag = { BLOCKED: 'BLOCKED   ', MECHANICAL: 'MECHANICAL', DONE: 'DONE      ', HAND: 'HAND      ' }[p.verdict]
  console.log(`${tag} ${p.sha}  ${p.date}  ${p.subject.slice(0, 62)}`)
  for (const r of p.rows) console.log(`             ${r.action.padEnd(10)} ${r.rel}`)
  if (p.rows.length) console.log(`             -> ${p.copyable} copy · ${p.hand} hand-merge · ${p.fresh} new · ${p.done} already done`)
  console.log()
}

const mech = plan.filter((p) => p.verdict === 'MECHANICAL')
const hand = plan.filter((p) => p.verdict === 'HAND')
const blocked = plan.filter((p) => p.verdict === 'BLOCKED')
const alreadyDone = plan.filter((p) => p.verdict === 'DONE')
console.log('Summary')
console.log(`  ${String(mech.length).padStart(3)}  MECHANICAL  every file copy-safe and ancestry clean — safe to apply`)
console.log(`  ${String(hand.length).padStart(3)}  HAND        at least one file carries June work — needs judgement`)
console.log(`  ${String(blocked.length).padStart(3)}  BLOCKED     divergent ancestry — do not copy, resolve first`)
console.log(`  ${String(alreadyDone.length).padStart(3)}  DONE        every file already matches upstream — nothing to do`)

if (gates) {
  console.log('\nBaseline to compare against after any port')
  for (const [k, v] of Object.entries(gates)) console.log(`  ${k.padEnd(11)} ${v}`)
  console.log('\n  conversion-status is the one to watch: tsc and vitest catch a port that')
  console.log('  BREAKS; only this catches a port that SUCCEEDS by overwriting converted')
  console.log('  work with upstream\'s Tailwind. If it drops, revert rather than investigate.')
}

console.log('\nThis tool changed nothing. Every action above is yours to take deliberately.')
process.exit(blocked.length ? 1 : 0)
