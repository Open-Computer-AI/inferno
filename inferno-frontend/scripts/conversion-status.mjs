#!/usr/bin/env node
/**
 * How far through the redesign are we, as a number.
 *
 * june-lint answers "is what we converted correct". It cannot answer "how much
 * is left", because an untouched file is out of its scope by design. This
 * counts the other half: files still carrying the dead Tailwind colour
 * palettes, which is the most reliable single proxy for "not yet converted"
 * (a converted file has scoped CSS on June tokens and no `bg-gray-700`).
 *
 * Exit code is 0 always -- this reports, it does not gate. The gate is
 * june-lint. Use --json for machine consumption, --top N to list the worst.
 *
 *   node scripts/conversion-status.mjs
 *   node scripts/conversion-status.mjs --json
 *   node scripts/conversion-status.mjs --top 20
 */
import { readFileSync } from 'node:fs'
import { readdir } from 'node:fs/promises'
import { join, relative, resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const SRC = join(ROOT, 'src')

const args = process.argv.slice(2)
const asJson = args.includes('--json')
const topIdx = args.indexOf('--top')
const topN = topIdx === -1 ? 12 : Number(args[topIdx + 1] || 12)

/* The dead palettes. Matches a utility only inside a class attribute, so prose
   in a comment explaining "we removed bg-gray-700" does not count itself. */
const LEGACY = /\b(?:bg|text|border|ring|divide|from|via|to)-(?:gray|red|green|blue|amber|slate|dark|yellow|purple|indigo|emerald|rose|teal|cyan|orange|pink|primary|accent)-\d{2,3}\b/g
const CLASS_ATTR = /class="([^"]*)"|:class="([^"]*)"/g

/* Design-system files are spec, not our code -- june-lint exempts them too. */
const EXEMPT = [/src\/design-system\//, /__tests__/, /\.spec\.ts$/]

async function walk(dir, out = []) {
  for (const e of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, e.name)
    if (e.isDirectory()) await walk(p, out)
    else if (p.endsWith('.vue')) out.push(p)
  }
  return out
}

const files = (await walk(SRC)).filter((f) => !EXEMPT.some((re) => re.test(f)))

const rows = []
for (const file of files) {
  const text = readFileSync(file, 'utf8')
  let hits = 0
  for (const m of text.matchAll(CLASS_ATTR)) {
    hits += ((m[1] ?? m[2] ?? '').match(LEGACY) ?? []).length
  }
  const lines = text.split('\n').length
  rows.push({ file: relative(ROOT, file), hits, lines })
}

const dirty = rows.filter((r) => r.hits > 0).sort((a, b) => b.hits - a.hits)
const totalHits = dirty.reduce((n, r) => n + r.hits, 0)
const totalLines = dirty.reduce((n, r) => n + r.lines, 0)
const pct = ((rows.length - dirty.length) / rows.length) * 100

const summary = {
  filesTotal: rows.length,
  filesConverted: rows.length - dirty.length,
  filesRemaining: dirty.length,
  percentConverted: Number(pct.toFixed(1)),
  legacyUtilitiesRemaining: totalHits,
  linesRemaining: totalLines,
  worst: dirty.slice(0, topN)
}

if (asJson) {
  console.log(JSON.stringify(summary, null, 2))
} else {
  console.log(
    `conversion: ${summary.filesConverted}/${summary.filesTotal} files (${summary.percentConverted}%), ` +
      `${summary.legacyUtilitiesRemaining} legacy utilities left across ${summary.linesRemaining} lines`
  )
  if (dirty.length) {
    console.log(`\n  worst ${Math.min(topN, dirty.length)}:`)
    for (const r of dirty.slice(0, topN)) {
      console.log(`    ${String(r.hits).padStart(5)}  ${String(r.lines).padStart(6)}L  ${r.file}`)
    }
  }
}
