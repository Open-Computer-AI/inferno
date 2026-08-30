/**
 * logic-drift — which upstream conditions did the June rewrite drop?
 *
 * WHY THIS EXISTS: the June rewrite was supposed to change <template> and
 * <style> and leave <script setup> alone. It did not -- 243 of the 285 ported
 * files have script-region changes. Every defect the file-level sweep found has
 * the same shape: a line carrying a CONDITION exists upstream and has no
 * counterpart in ours. Examples that shipped:
 *
 *   used >= max          became  pct > 100      (saturation never turns red)
 *   mode === 'off'       dropped                ("Disabled" beside a green tick)
 *   429/529 counts       dropped                (drill-down dead during an outage)
 *   if (!value) return '-'  dropped             (blank cell, not a dash)
 *
 * None of the other gates can see this. vue-tsc type-checks both spellings,
 * vitest passes because no spec covered the branch, june-lint only reads class
 * names, and check-divergence does not cover inferno-frontend at all.
 *
 * This is a WORKLIST, not a pass/fail oracle: a line legitimately rewritten to
 * do the same thing differently shows up here too. It bounds the surface from
 * ~60k diff lines to a few hundred worth a human look, ranked worst-first.
 *
 * Usage:  node scripts/logic-drift.mjs            # ranked summary
 *         node scripts/logic-drift.mjs <path>     # the vanished lines for one file
 */
import { readFileSync, existsSync } from 'node:fs'
import { execSync } from 'node:child_process'
import { dirname, resolve, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const REPO = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const UP = resolve(REPO, 'frontend/src')
const OURS = resolve(REPO, 'inferno-frontend/src')

const PREDICATE = /(\bif\s*\(|\?\?|\|\||&&|>=|<=|===|!==|\breturn\b|\bcomputed\(|\bwatch\(|\.filter\(|\.some\(|\.every\()/
const NOISE = /^\s*(\/\/|\/\*|\*|import |export type|interface |type )/

// Locale files and specs change wording and coverage by design; a dropped line
// there is not a dropped condition.
const EXCLUDE = /(\/locales\/|__tests__\/|\.spec\.ts$)/

const scriptRegion = (file) => {
  const src = readFileSync(file, 'utf8')
  if (!file.endsWith('.vue')) return src
  const m = src.match(/<script[^>]*>([\s\S]*?)<\/script>/)
  return m ? m[1] : ''
}

const norm = (line) => line.replace(/\s+/g, ' ').trim()

const upstreamFiles = execSync(`find ${UP} -type f \\( -name '*.vue' -o -name '*.ts' \\)`)
  .toString().trim().split('\n')

const results = []
for (const upFile of upstreamFiles) {
  const rel = relative(UP, upFile)
  if (EXCLUDE.test(rel)) continue
  const ourFile = resolve(OURS, rel)
  if (!existsSync(ourFile)) continue

  const ours = new Set(scriptRegion(ourFile).split('\n').map(norm))
  const vanished = scriptRegion(upFile).split('\n')
    .filter((l) => PREDICATE.test(l) && !NOISE.test(l))
    .map(norm)
    .filter((l) => l.length > 12 && !ours.has(l))

  if (vanished.length) results.push({ rel, vanished })
}

results.sort((a, b) => b.vanished.length - a.vanished.length)

const only = process.argv[2]
if (only) {
  const hit = results.find((r) => r.rel === only || r.rel.endsWith(only))
  if (!hit) { console.log(`no vanished predicates for ${only}`); process.exit(0) }
  console.log(`${hit.rel}  (${hit.vanished.length})\n`)
  for (const l of hit.vanished) console.log(`  ${l}`)
  process.exit(0)
}

const total = results.reduce((n, r) => n + r.vanished.length, 0)
for (const r of results) console.log(`${String(r.vanished.length).padStart(5)}  ${r.rel}`)
console.log(`\n${total} vanished predicate line(s) across ${results.length} file(s)`)
console.log(`Inspect one:  node scripts/logic-drift.mjs <path>`)
