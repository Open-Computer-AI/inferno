#!/usr/bin/env node
/**
 * port-verify — the reviewer's checklist, run independently of whoever ported.
 *
 * WHY THIS EXISTS: the agent that produced work is never its own grader, and
 * this fork has the receipts. Every defect that shipped in the 2026-09 catch-up
 * passed every automated check and still looked correct:
 *
 *   a hunk that never applied at all   tsc clean, 2048 tests green
 *   a flipped default                  tsc, tests, june-lint, parity all green
 *   a verbatim copy that reverted the  everything green; only upstream's own
 *     previous commit                   tests objected
 *
 * So this does not read a report. It re-derives every number from the tree and
 * compares against a baseline captured BEFORE the work, then states a verdict.
 *
 * THE ASYMMETRY IT ENCODES: some gates may only move one way.
 *   conversion-status may not FALL      a fall means converted work was
 *                                       overwritten with upstream's Tailwind
 *   behaviour-parity may not GAIN gaps  upstream pins behaviour we dropped
 *   debt-ledger may not reopen a row    a closed row reopening is a regression
 *   port-coverage baseline may not gain entries
 *   tsc and vitest must be clean, absolutely
 *   june-lint may rise, but every point must be accounted for in the report
 *
 *   node scripts/port-verify.mjs --baseline out.json    capture, before work
 *   node scripts/port-verify.mjs --against out.json     verify, after work
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const OURS = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const arg = (f) => { const i = process.argv.indexOf(f); return i > -1 ? process.argv[i + 1] : null }

const run = (cmd, args) => {
  try {
    return { out: execFileSync(cmd, args, { cwd: OURS, encoding: 'utf8', maxBuffer: 64e6, env: { ...process.env, NODE_OPTIONS: '' } }), code: 0 }
  } catch (e) {
    return { out: (e.stdout || '') + (e.stderr || ''), code: e.status ?? 1 }
  }
}
const num = (s, re) => { const m = (s || '').match(re); return m ? Number(m[1]) : null }

function measure() {
  const tsc = run('npx', ['vue-tsc', '--noEmit'])
  const vitest = run('npx', ['vitest', 'run', '--reporter=dot'])
  const conv = run('node', ['scripts/conversion-status.mjs']).out
  const lint = run('node', ['scripts/june-lint.mjs']).out
  const par = run('node', ['scripts/behaviour-parity.mjs']).out
  const debt = run('node', ['scripts/debt-ledger.mjs', '--check'])
  const cov = run('node', ['scripts/port-coverage.mjs', '--check-baseline'])
  return {
    tscErrors: (tsc.out.match(/error TS/g) || []).length,
    testFiles: num(vitest.out, /Test Files\s+(\d+) passed/),
    tests: num(vitest.out, /Tests\s+(\d+) passed/),
    testsFailed: num(vitest.out, /Tests\s+(\d+) failed/) ?? 0,
    convertedFiles: num(conv, /conversion: (\d+)\//),
    legacyUtilities: num(conv, /, (\d+) legacy utilities/),
    lintViolations: num(lint, /june-lint: (\d+) violation/),
    lintFiles: num(lint, /across (\d+) converted file/),
    parityMissing: num(par, /(\d+) missing file/),
    parityShortfalls: num(par, /· (\d+) shortfall/),
    debtOpen: num(debt.out, /(\d+) open,/),
    debtReopened: /expected closed are open again/.test(debt.out),
    coverageUnaccounted: num(cov.out, /(\d+) unaccounted/),
    coverageChanged: cov.code === 1,
  }
}

const m = measure()

if (process.argv.includes('--baseline')) {
  writeFileSync(arg('--baseline'), JSON.stringify(m, null, 2))
  console.log(`baseline captured -> ${arg('--baseline')}`)
  for (const [k, v] of Object.entries(m)) console.log(`  ${k.padEnd(20)} ${v}`)
  process.exit(0)
}

const basePath = arg('--against')
if (!basePath) { console.error('usage: --baseline <file>  |  --against <file>'); process.exit(2) }
const b = JSON.parse(readFileSync(basePath, 'utf8'))

/*
 * Each rule is a hard failure, not a warning, EXCEPT june-lint. A rise there is
 * legitimate when a port lands upstream markup in a file that is still Tailwind
 * -- that happened deliberately four times in the catch-up -- so it is reported
 * loudly and left to the reviewer rather than blocking automatically.
 */
const findings = []
const fail = (t) => findings.push({ level: 'FAIL', t })
const note = (t) => findings.push({ level: 'NOTE', t })

if (m.tscErrors > 0) fail(`vue-tsc reports ${m.tscErrors} error(s) — was ${b.tscErrors}`)
if (m.testsFailed > 0) fail(`${m.testsFailed} test(s) failing`)
if (m.tests !== null && b.tests !== null && m.tests < b.tests)
  fail(`test count FELL ${b.tests} -> ${m.tests}: coverage was removed, not added`)
if (m.convertedFiles !== null && b.convertedFiles !== null && m.convertedFiles < b.convertedFiles)
  fail(`converted file count FELL ${b.convertedFiles} -> ${m.convertedFiles}. A copy overwrote converted work — revert, do not investigate first`)
if (m.legacyUtilities !== null && b.legacyUtilities !== null && m.legacyUtilities > b.legacyUtilities)
  note(`legacy Tailwind utilities rose ${b.legacyUtilities} -> ${m.legacyUtilities} (+${m.legacyUtilities - b.legacyUtilities}). Expected only when porting verbatim into an unconverted file — say which`)
if (m.parityMissing > b.parityMissing || m.parityShortfalls > b.parityShortfalls)
  fail(`behaviour-parity worsened: ${b.parityMissing}/${b.parityShortfalls} -> ${m.parityMissing}/${m.parityShortfalls}. Upstream pins behaviour this tree no longer has`)
if (m.debtReopened) fail('a closed debt-ledger row has reopened')
if (m.debtOpen > b.debtOpen) fail(`debt rows open rose ${b.debtOpen} -> ${m.debtOpen}`)
if (m.coverageChanged) fail('port-coverage: the unaccounted-line set changed — run --check-baseline and read the delta')
if (m.lintViolations > b.lintViolations)
  note(`june-lint rose ${b.lintViolations} -> ${m.lintViolations} (+${m.lintViolations - b.lintViolations}). Legitimate only for verbatim markup in an unconverted file`)

console.log('\nport-verify — measured from the tree, not from anyone\'s report\n')
const rows = [
  ['tsc errors', b.tscErrors, m.tscErrors], ['tests passing', b.tests, m.tests],
  ['converted files', b.convertedFiles, m.convertedFiles], ['legacy utilities', b.legacyUtilities, m.legacyUtilities],
  ['june-lint', b.lintViolations, m.lintViolations], ['parity missing', b.parityMissing, m.parityMissing],
  ['parity shortfalls', b.parityShortfalls, m.parityShortfalls], ['debt open', b.debtOpen, m.debtOpen],
  ['coverage unaccounted', b.coverageUnaccounted, m.coverageUnaccounted],
]
console.log(`  ${'gate'.padEnd(22)} ${'before'.padStart(8)} ${'after'.padStart(8)}`)
for (const [k, x, y] of rows) {
  const mark = x === y ? ' ' : y > x ? '↑' : '↓'
  console.log(`  ${k.padEnd(22)} ${String(x).padStart(8)} ${String(y).padStart(8)}  ${mark}`)
}

console.log()
if (!findings.length) {
  console.log('SHIP — every gate held or improved.')
  process.exit(0)
}
for (const f of findings) console.log(`  ${f.level}  ${f.t}`)
const hard = findings.filter((f) => f.level === 'FAIL').length
console.log()
console.log(hard ? `NO-SHIP — ${hard} gate(s) moved the wrong way.` : 'SHIP WITH CONDITIONS — no gate failed, but the notes above need a reason in the commit message.')
process.exit(hard ? 1 : 0)
