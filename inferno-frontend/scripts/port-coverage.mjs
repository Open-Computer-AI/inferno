#!/usr/bin/env node
/**
 * port-coverage — did each ported upstream commit actually LAND?
 *
 * WHY THIS EXISTS: five separate ports partially applied and were recorded as
 * done. Not one was caught by a gate; every one was found later by a human or
 * an agent reading code.
 *
 *   684d9efb1   5 of 39 lines      391d69e08   0 of 6 lines
 *   e39fce270  12 of 31 lines      c4e46c3be  10 of 38, then 12 of 51
 *   b5827cfd5  frontend half never landed at all
 *
 * The first two were recorded green by a check that grepped for conflict
 * markers and never inspected `git apply`'s exit code, so a patch that failed
 * outright read as success. e39fce270 left a computed with three call sites and
 * no declaration -- caught by tsc, not by the ledger, which had already marked
 * the row PORTED. b5827cfd5's backend arrived by merge and its frontend half
 * was assumed to have come with it; a merge never touches inferno-frontend.
 *
 * Every other gate is blind to this by construction. vue-tsc only sees a port
 * that fails to compile. vitest only sees a branch some test covers. june-lint
 * reads class names. logic-drift compares us against upstream HEAD, so a hunk
 * that never landed looks identical to one we deliberately rewrote.
 *
 * WHAT IT DOES: for each upstream commit, take the substantive lines its
 * frontend/ diff ADDED and ask how many appear anywhere in inferno-frontend/.
 *
 * WHAT IT IS NOT: a pass/fail oracle. A port we deliberately rebuilt in the
 * June idiom SHOULD score low -- we took the logic and rewrote the markup, so
 * upstream's literal lines are correctly absent. Coverage is a ranked worklist:
 * a row expected VERBATIM sitting near zero is the shape of a patch that never
 * applied, and that is the signal to go and look.
 *
 *   node scripts/port-coverage.mjs                 rank every commit by coverage
 *   node scripts/port-coverage.mjs --below 40      only those under 40%
 *   node scripts/port-coverage.mjs <sha>           the missing lines for one commit
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const OURS = resolve(ROOT, 'inferno-frontend')
const MANIFEST = resolve(ROOT, 'docs/superpowers/analysis/COMMIT-MANIFEST.md')

const git = (args) =>
  execFileSync('git', args, { cwd: ROOT, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 })

/** Lines too generic to prove anything by their presence or absence. */
const isNoise = (l) => {
  const t = l.trim()
  if (t.length < 12) return true
  if (/^(import|export|\/\/|\/\*|\*|<\/|<template|<script|<style|}|\)|\]|,|;)/.test(t)) return true
  // a pure class-attribute change is exactly what a June rebuild is expected to
  // rewrite, so counting it would punish the correct outcome
  if (/^(class|:class)=/.test(t)) return true
  return false
}

const norm = (l) => l.replace(/\s+/g, ' ').trim()

let corpus = null
const ourCorpus = () => {
  if (corpus) return corpus
  const out = []
  const walk = (dir) => {
    for (const e of readdirSync(dir)) {
      if (e === 'node_modules' || e === 'dist' || e === '.git') continue
      const full = join(dir, e)
      if (statSync(full).isDirectory()) walk(full)
      else if (/\.(vue|ts|js|json|yaml|css)$/.test(e)) out.push(readFileSync(full, 'utf8'))
    }
  }
  walk(OURS)
  // Containment, not line membership: upstream's diff often splits a key and
  // its value across two lines where our file joins them, which is a reflow
  // and not a missing port. Line-set matching scored one such commit at 33%.
  corpus = norm(out.join('\n'))
  return corpus
}

const addedLines = (sha) => {
  let diff
  try {
    diff = git(['show', sha, '--unified=0', '--', 'frontend/'])
  } catch {
    return null
  }
  return diff
    .split('\n')
    .filter((l) => l.startsWith('+') && !l.startsWith('+++'))
    .map((l) => l.slice(1))
    .filter((l) => !isNoise(l))
    .map(norm)
}

const shas = [...readFileSync(MANIFEST, 'utf8').matchAll(/^\| \d+ \| `([0-9a-f]{7,})`/gm)].map((m) => m[1])

const only = process.argv.find((a) => /^[0-9a-f]{7,}$/.test(a))
const belowArg = process.argv.indexOf('--below')
const below = belowArg > -1 ? Number(process.argv[belowArg + 1]) : null

const ours = ourCorpus()
const results = []

for (const sha of only ? [only] : shas) {
  const added = addedLines(sha)
  if (added === null) { results.push({ sha, pct: null, hit: 0, total: 0, missing: [] }); continue }
  if (!added.length) continue // no substantive frontend lines to account for
  const missing = added.filter((l) => !ours.includes(l))
  const hit = added.length - missing.length
  results.push({ sha, pct: Math.round((hit / added.length) * 100), hit, total: added.length, missing })
}

if (only) {
  const r = results[0]
  if (!r || r.pct === null) { console.log(`${only}: no frontend diff`); process.exit(0) }
  console.log(`${r.sha}  ${r.pct}%  (${r.hit}/${r.total} substantive added lines present)\n`)
  if (!r.missing.length) console.log('  every line accounted for')
  else for (const l of r.missing.slice(0, 60)) console.log(`  missing: ${l.slice(0, 150)}`)
  if (r.missing.length > 60) console.log(`  ... and ${r.missing.length - 60} more`)
  process.exit(0)
}

results.sort((a, b) => (a.pct ?? 999) - (b.pct ?? 999))
const shown = below === null ? results : results.filter((r) => r.pct !== null && r.pct < below)

for (const r of shown) {
  if (r.pct === null) { console.log(`   --  ${r.sha}  no frontend diff`); continue }
  console.log(`${String(r.pct).padStart(4)}%  ${r.sha}  ${String(r.hit).padStart(3)}/${String(r.total).padEnd(3)}`)
}

const scored = results.filter((r) => r.pct !== null)
const low = scored.filter((r) => r.pct < 40).length
console.log(`\n${scored.length} commits scored · ${low} below 40%`)
console.log('Low coverage is a worklist, not a verdict: a June rebuild is expected to score low.')
console.log('Inspect one:  node scripts/port-coverage.mjs <sha>')
