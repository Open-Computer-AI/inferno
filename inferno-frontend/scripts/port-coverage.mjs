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
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs'
import { join, dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const OURS = resolve(ROOT, 'inferno-frontend')
const MANIFEST = resolve(ROOT, 'docs/superpowers/analysis/COMMIT-MANIFEST.md')
const BASELINE = resolve(ROOT, 'docs/superpowers/analysis/port-coverage-missing.txt')

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

/**
 * Added lines, each tagged with the file upstream put it in.
 *
 * WHY THE FILE MATTERS: matching a line against the whole corpus answers
 * "does this text exist somewhere in our tree", which is not the question.
 * Measured on 2026-09-02: flipping SettingsView's openai_ttft_mode default
 * from "semantic" to "visible" left coverage at 85%, because the literal
 * `openai_ttft_mode: "semantic",` also lives in SettingsView.spec.ts and the
 * corpus match found it there. tsc, 2049 tests, june-lint and
 * behaviour-parity all passed too. Nothing saw it.
 */
const addedLines = (sha) => {
  let diff
  try {
    diff = git(['show', sha, '--unified=0', '--', 'frontend/'])
  } catch {
    return null
  }
  const out = []
  let file = null
  for (const raw of diff.split('\n')) {
    const m = /^\+\+\+ b\/frontend\/(.+)$/.exec(raw)
    if (m) { file = m[1]; continue }
    if (!raw.startsWith('+') || raw.startsWith('+++')) continue
    const l = raw.slice(1)
    if (isNoise(l)) continue
    out.push({ file, line: norm(l) })
  }
  return out
}

/** Our copy of one upstream path, normalised, or null when we do not have it. */
const fileCache = new Map()
const ourFile = (rel) => {
  if (fileCache.has(rel)) return fileCache.get(rel)
  let v = null
  try { v = norm(readFileSync(join(OURS, rel), 'utf8')) } catch { v = null }
  fileCache.set(rel, v)
  return v
}

const shas = [...readFileSync(MANIFEST, 'utf8').matchAll(/^\| \d+ \| `([0-9a-f]{7,})`/gm)].map((m) => m[1])

const only = process.argv.find((a) => /^[0-9a-f]{7,}$/.test(a))
const belowArg = process.argv.indexOf('--below')
const below = belowArg > -1 ? Number(process.argv[belowArg + 1]) : null

const ours = ourCorpus()
const results = []

for (const sha of only ? [only] : shas) {
  const added = addedLines(sha)
  if (added === null) { results.push({ sha, pct: null, hit: 0, total: 0, missing: [], moved: [] }); continue }
  if (!added.length) continue // no substantive frontend lines to account for

  /*
   * Three outcomes per line, not two:
   *   IN PLACE   present in the file upstream put it in
   *   RELOCATED  absent there, present elsewhere in our tree -- real and
   *              common (c66e700f0's control lives in our extracted
   *              AdminGatewaySettingsPage), but it is a claim needing a
   *              reason, not a silent pass
   *   MISSING    nowhere at all
   */
  const inPlace = [], moved = [], missing = []
  for (const { file, line } of added) {
    const own = file ? ourFile(file) : null
    if (own && own.includes(line)) inPlace.push(line)
    else if (ours.includes(line)) moved.push({ file, line })
    else missing.push({ file, line })
  }
  const hit = inPlace.length
  results.push({
    sha, pct: Math.round((hit / added.length) * 100),
    hit, total: added.length, missing, moved,
  })
}

if (only) {
  const r = results[0]
  if (!r || r.pct === null) { console.log(`${only}: no frontend diff`); process.exit(0) }
  console.log(`${r.sha}  ${r.pct}%  (${r.hit}/${r.total} added lines in the file upstream put them in)\n`)
  if (r.moved.length) {
    console.log(`  RELOCATED — elsewhere in our tree, not in upstream's file (${r.moved.length}):`)
    for (const m of r.moved.slice(0, 30)) console.log(`    ${m.file}\n      ${m.line.slice(0, 130)}`)
    console.log()
  }
  if (!r.missing.length && !r.moved.length) console.log('  every line accounted for, in place')
  else if (!r.missing.length) console.log('  nothing missing outright')
  else {
    console.log(`  MISSING — nowhere in our tree (${r.missing.length}):`)
    for (const m of r.missing.slice(0, 40)) console.log(`    ${m.file}\n      ${m.line.slice(0, 130)}`)
    if (r.missing.length > 40) console.log(`    ... and ${r.missing.length - 40} more`)
  }
  process.exit(0)
}

/*
 * --digest: a stable fingerprint of the exact set of MISSING lines across every
 * manifest row.
 *
 * WHY A SET AND NOT A COUNT: an explained file used to become a blind spot.
 * The old debt-ledger probe pinned "5 rows below 40%", so a sixth defect
 * inside an already-explained file moved nothing. Measured: flipping
 * SettingsView's TTFT default took that file from 19% to 13% -- the tool SAW
 * it -- and the headline number did not move, because it counted files.
 * A digest over the line set moves for any new missing line anywhere.
 */
if (process.argv.includes('--baseline') || process.argv.includes('--check-baseline')) {
  /*
   * The baseline covers MISSING **and** RELOCATED.
   *
   * A first cut tracked only missing, and it let the motivating defect through
   * again: flipping SettingsView's TTFT default made
   * `openai_ttft_mode: "semantic",` absent from SettingsView.vue, but the same
   * literal exists in SettingsView.spec.ts, so it landed in the relocated
   * bucket and the digest never moved. Three buckets had merely renamed the
   * escape hatch.
   *
   * Relocation is a CLAIM -- "upstream put this line in file A and we keep it
   * in file B" -- and claims get reviewed, not auto-accepted. Both buckets are
   * legitimate and both belong in the reviewed set.
   */
  const all = [
    ...results.flatMap((r) => r.missing.map((m) => `MISSING   ${r.sha} ${m.file} ${m.line}`)),
    ...results.flatMap((r) => r.moved.map((m) => `RELOCATED ${r.sha} ${m.file} ${m.line}`)),
  ].sort()
  const digest = createHash('sha256').update(all.join('\n')).digest('hex').slice(0, 16)

  if (process.argv.includes('--baseline')) {
    writeFileSync(BASELINE, all.join('\n') + '\n')
    console.log(`recorded ${all.length} unaccounted line(s) · digest ${digest}`)
    console.log(BASELINE)
    process.exit(0)
  }

  let prev
  try { prev = readFileSync(BASELINE, 'utf8').split('\n').filter(Boolean) } catch {
    console.error(`no baseline at ${BASELINE} — run: node scripts/port-coverage.mjs --baseline`)
    process.exit(2)
  }
  const before = new Set(prev), after = new Set(all)
  const appeared = all.filter((l) => !before.has(l))
  const resolved = prev.filter((l) => !after.has(l))

  // Printing the DELTA, not a pass/fail number, is the point. A bare count made
  // an explained file a permanent blind spot; a diff makes every new line
  // visible and every review cheap.
  if (appeared.length) {
    console.log(`${appeared.length} line(s) changed state — newly missing, or newly only-elsewhere:\n`)
    for (const l of appeared.slice(0, 40)) console.log(`  + ${l.slice(0, 150)}`)
    if (appeared.length > 40) console.log(`  ... and ${appeared.length - 40} more`)
    console.log()
  }
  if (resolved.length) {
    console.log(`${resolved.length} line(s) previously absent are now present (re-baseline if intended):\n`)
    for (const l of resolved.slice(0, 20)) console.log(`  - ${l.slice(0, 150)}`)
    console.log()
  }
  console.log(`${all.length} unaccounted line(s) (missing or relocated) · digest ${digest}`)
  process.exit(appeared.length ? 1 : 0)
}

results.sort((a, b) => (a.pct ?? 999) - (b.pct ?? 999))
const shown = below === null ? results : results.filter((r) => r.pct !== null && r.pct < below)

for (const r of shown) {
  if (r.pct === null) { console.log(`   --  ${r.sha}  no frontend diff`); continue }
  console.log(`${String(r.pct).padStart(4)}%  ${r.sha}  ${String(r.hit).padStart(3)}/${String(r.total).padEnd(3)}`)
}

const scored = results.filter((r) => r.pct !== null)
const low = scored.filter((r) => r.pct < 40).length
const missTotal = results.reduce((n, r) => n + r.missing.length, 0)
const movedTotal = results.reduce((n, r) => n + r.moved.length, 0)
console.log(`\n${scored.length} commits scored · ${low} below 40%`)
console.log(`${missTotal} line(s) missing outright · ${movedTotal} relocated to another file of ours`)
console.log('Low coverage is a worklist, not a verdict: a June rebuild is expected to score low.')
console.log('Inspect one:  node scripts/port-coverage.mjs <sha>')
