#!/usr/bin/env node
/**
 * behaviour-parity — is a June rebuild still doing what upstream's code does?
 *
 * WHY THIS EXISTS: port-coverage answers "did the port land" by matching
 * upstream's literal lines. That question is meaningless for the ports we
 * deliberately REBUILT in the June idiom: we take the logic and rewrite the
 * markup, so upstream's lines are correctly absent and coverage correctly reads
 * near zero. Twelve of the 103 ports are that kind, and nothing measured them.
 *
 * The oracle is upstream's own tests. They ship one with roughly 92% of
 * commits, and a test is a statement about behaviour rather than about markup,
 * so it survives a rewrite that class names do not. 61 of our 103 ported
 * commits carry upstream test cases; 212 cases in total.
 *
 * WHAT IT MEASURES, two things:
 *
 *   1. Every upstream spec file touched by a ported commit — does an equivalent
 *      path exist in ours at all? A missing file means behaviour upstream
 *      specified that nothing in our tree pins.
 *   2. For the specs that do exist — does ours hold at least as many cases as
 *      upstream's? Fewer is not automatically wrong (a rebuild can consolidate,
 *      and we rename some specs), but every shortfall needs a reason.
 *
 * WHAT IT IS NOT: proof of correctness. Matching counts can still test
 * different things. It answers "is there anything upstream pins that we do
 * not", which is the question that actually caught real losses: run against
 * this tree it independently reproduced four coverage gaps that a six-agent
 * file-by-file read had found separately.
 *
 *   node scripts/behaviour-parity.mjs            full report
 *   node scripts/behaviour-parity.mjs --gaps     only missing files and shortfalls
 */
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { dirname, resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const MANIFEST = resolve(ROOT, 'docs/superpowers/analysis/COMMIT-MANIFEST.md')

const git = (args) => {
  try {
    return execFileSync('git', args, { cwd: ROOT, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 })
  } catch {
    return ''
  }
}

/*
 * cwd matters more than it looks. An earlier version of this measurement ran
 * from the wrong directory, so every `git show` returned an empty string and it
 * reported a confident "0 upstream test cases" across all 103 commits. That is
 * the same failure as the old i18n-keycheck printing "all keys resolve" while
 * scanning nothing: a measurement that reports success because it looked at
 * nothing at all. Hence the explicit cwd above, and the assertion below.
 */
const countCases = (text) => (text.match(/^\s*(?:it|test)\s*\(/gm) || []).length

/**
 * Cases that moved to another file during the June rebuild, verified by
 * comparing the case lists rather than assumed. Recorded here so a known
 * relocation stops reading as a loss forever -- but each entry names the file
 * the case landed in, so the claim stays checkable.
 */
const RELOCATED = {
  'frontend/src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts': {
    n: 1,
    to: 'OpenAIQuotaResetCell.autoState.spec.ts',
    why: "upstream's 开关关闭时不显示历史运行态 is our \"renders nothing when auto-reset is switched off, even with a stale state\""
  }
}

const shas = [...readFileSync(MANIFEST, 'utf8').matchAll(/^\| \d+ \| `([0-9a-f]{7,})`/gm)].map((m) => m[1])
if (!shas.length) { console.error('no manifest rows found — wrong path?'); process.exit(1) }

const seen = new Set()
const missing = []
const shared = []
let upstreamCases = 0

for (const sha of shas) {
  const files = git(['show', '--name-only', '--format=', sha, '--', 'frontend/']).split('\n').filter(Boolean)
  for (const sp of files.filter((f) => f.includes('.spec.'))) {
    if (seen.has(sp)) continue
    seen.add(sp)

    const up = git(['show', `upstream/main:${sp}`])
    if (!up) continue
    const u = countCases(up)
    if (!u) continue
    upstreamCases += u

    const ourPath = join(ROOT, sp.replace('frontend/', 'inferno-frontend/'))
    let ours
    try { ours = readFileSync(ourPath, 'utf8') } catch { missing.push({ sha, sp, u }); continue }
    const reloc = RELOCATED[sp]
    shared.push({ sp, u, o: countCases(ours) + (reloc?.n ?? 0), reloc })
  }
}

if (!upstreamCases) {
  console.error('read 0 upstream test cases — the measurement is not looking at anything. Refusing to report.')
  process.exit(1)
}

const short = shared.filter((r) => r.o < r.u).sort((a, b) => a.o - a.u - (b.o - b.u))
const gapsOnly = process.argv.includes('--gaps')

if (missing.length) {
  console.log('Upstream specs with NO equivalent file in ours:\n')
  for (const m of missing) console.log(`  ${m.sha}  ${String(m.u).padStart(3)} cases  ${m.sp.replace('frontend/src/', '')}`)
  console.log()
}

if (short.length) {
  console.log('Shared specs holding fewer cases than upstream:\n')
  console.log(`  ${'Δ'.padStart(3)} ${'up'.padStart(3)} ${'ours'.padStart(4)}  spec`)
  for (const r of short) {
    console.log(`  ${String(r.o - r.u).padStart(3)} ${String(r.u).padStart(3)} ${String(r.o).padStart(4)}  ${r.sp.replace('frontend/src/', '')}`)
  }
  console.log()
}

const relocated = shared.filter((r) => r.reloc)
if (relocated.length) {
  console.log('Counted as present, relocated during the June rebuild:\n')
  for (const r of relocated) console.log(`  +${r.reloc.n}  ${r.sp.replace('frontend/src/', '')}\n      -> ${r.reloc.to}: ${r.reloc.why}`)
  console.log()
}

if (!gapsOnly) {
  const ourTotal = shared.reduce((n, r) => n + r.o, 0)
  const upTotal = shared.reduce((n, r) => n + r.u, 0)
  console.log(`${shared.length} shared specs · ours ${ourTotal} cases vs upstream ${upTotal}`)
}
console.log(`${missing.length} missing file(s) · ${short.length} shortfall(s) · ${upstreamCases} upstream cases examined`)
console.log('A shortfall is not automatically wrong — a rebuild can consolidate. It needs a reason.')
