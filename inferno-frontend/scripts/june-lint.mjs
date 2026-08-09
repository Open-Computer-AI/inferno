#!/usr/bin/env node
/**
 * June ground-rule lint.
 *
 * The README calls its ten ground rules "not preferences. A violation is a
 * review failure." Seven of them are mechanically checkable, so they are
 * checked here rather than in review.
 *
 * SCOPE is the interesting part: this lints only files that have actually been
 * converted, and works that out by diffing against ../frontend, which is a
 * pristine copy of upstream. A file that still matches upstream byte for byte
 * has not been touched yet and is not held to June's rules; the moment it
 * differs, it is. New files are always in scope.
 *
 * That means the lint scope widens automatically as the rewrite proceeds, with
 * no list to maintain and no way to forget to add a file.
 *
 *   node scripts/june-lint.mjs           lint converted files
 *   node scripts/june-lint.mjs --all     lint everything (will be very noisy)
 */

import { readFileSync, existsSync, statSync } from 'node:fs'
import { readdir } from 'node:fs/promises'
import { join, relative, resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const REFERENCE = resolve(ROOT, '../frontend')
const SRC = join(ROOT, 'src')
const ALL = process.argv.includes('--all')

/** Files the design system owns. Copied byte-identical from the bundle and
 *  re-synced, so they are spec, not our code, and are never linted. */
const EXEMPT = [/src\/design-system\//, /node_modules/, /\.spec\.ts$/, /__tests__/]

const RULES = [
  {
    id: 'ground-rule-1-sentence-case',
    // Sentence case cannot be fully linted, but the mechanical half can:
    // nothing in June is ever set in capitals.
    test: /text-transform:\s*uppercase|\buppercase\b(?![^\n]*\/\/ june-allow)/g,
    files: /\.(vue|css)$/,
    message: 'ALL CAPS is banned (ground rule 1). Remove uppercase.'
  },
  {
    id: 'ground-rule-3-two-weights',
    // The family ships 400 and 600 with font-synthesis:none, so 500 renders as
    // 400 and 700 renders as 600. Those declarations lie about what paints.
    test: /font-weight:\s*(100|200|300|500|700|800|900|bold|lighter|bolder)\b|\bfont-(thin|extralight|light|medium|bold|extrabold|black)\b/g,
    files: /\.(vue|css)$/,
    message: 'Only 400 and var(--fw-medium) (600) exist. Other weights silently resolve to one of those.'
  },
  {
    id: 'ground-rule-4-token-font-sizes',
    // "Sizes come ONLY from this scale — never hand-code a font-size."
    test: /font-size:\s*(?!var\(--fs-|inherit|0\b)[0-9.]+(px|rem|em|pt)/g,
    files: /\.(vue|css)$/,
    message: 'Hand-coded font-size. Use a --fs-* token.'
  },
  {
    id: 'ground-rule-6-static-borders',
    // "Borders are static chrome." transition-all is included because it
    // sweeps border-color in without anyone deciding to.
    // `transition: all` and `transition-all` both sweep border-color in without
    // anyone choosing to, which is how a static-border rule quietly dies.
    test: /transition:\s*all\b|transition:[^;]*border-color|transition-property:[^;]*\ball\b|transition-property:[^;]*border-color|\btransition-all\b/g,
    files: /\.(vue|css)$/,
    message: 'Never transition border-color (ground rule 6). Hover changes background only.'
  },
  {
    id: 'ground-rule-7-no-gradients-glass-glow',
    test: /\b(shadow-glass|shadow-glow|inner-glow|bg-gradient-(?!to-)|mesh-gradient|backdrop-blur|animate-(glow|shimmer|pulse-slow))\b|linear-gradient|radial-gradient/g,
    files: /\.(vue|css)$/,
    message: 'No gradients, glass, glow or mesh (ground rule 7).'
  },
  {
    id: 'dead-teal-palette',
    // Not a numbered ground rule, but 01-TOKENS is explicit that the teal
    // scale is contrary to June and dead on the redesign.
    test: /\b(bg|text|border|ring|from|to|via)-(primary|accent|dark)-[0-9]{2,3}\b/g,
    files: /\.(vue|css)$/,
    message: 'The teal/accent/dark Tailwind palettes are dead. Use June tokens.'
  },
  {
    id: 'ground-rule-8-no-emoji',
    test: /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}]/gu,
    files: /\.(vue|ts)$/,
    message: 'No emoji (ground rule 8). Iconography carries that load.'
  },
  {
    id: 'ground-rule-2-no-dashes',
    // Applies to "every i18n string you touch".
    test: /[–—]/g,
    files: /i18n\/.*\.ts$|\.vue$/,
    message: 'No en or em dashes in user-facing copy (ground rule 2). Use a hyphen or the word "to".'
  }
]

/**
 * Blank out comments, preserving line count and column positions.
 *
 * Without this the lint reads its own explanations: a comment saying "delete
 * every shadow-glow" trips the no-glow rule, and an em dash in prose trips the
 * no-dashes rule, which is about user-facing copy. Every finding was a false
 * positive on the first run, which is how a lint gets switched off.
 */
function stripComments(text) {
  const blank = (m) => m.replace(/[^\n]/g, ' ')
  return text
    .replace(/\/\*[\s\S]*?\*\//g, blank) // /* css + js block */
    .replace(/<!--[\s\S]*?-->/g, blank) // <!-- html -->
    .replace(/(^|[^:])\/\/[^\n]*/g, (m, p) => p + blank(m.slice(p.length))) // // line, not http://
}

/**
 * Selector context for the line at `idx`: the nearest preceding line that opens
 * a rule. Used to tell an icon glyph size from a text size -- 01-TOKENS
 * requires icons to be explicitly sized in px and says they must NOT scale with
 * the text-size preference, so --fs-* would be wrong for them.
 */
function selectorFor(lines, idx) {
  for (let i = idx; i >= 0 && idx - i < 30; i--) {
    const m = lines[i].match(/^\s*([^@{}]+)\{\s*$/)
    if (!m) continue
    // Selectors are often split across lines by comma:
    //   .fld__prefix,
    //   .fld__suffix {
    // Only checking the line with the brace would miss the first half, which
    // is exactly where the "is this an icon?" signal usually lives.
    let sel = m[1]
    for (let j = i - 1; j >= 0 && /,\s*$/.test(lines[j].trim()); j--) {
      sel = lines[j].trim() + ' ' + sel
    }
    return sel
  }
  return ''
}

const ICON_SELECTOR = /icon|hgi|glyph|spinner|mark\b/i

async function walk(dir, out = []) {
  for (const e of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, e.name)
    if (e.isDirectory()) await walk(p, out)
    else out.push(p)
  }
  return out
}

/** Converted = differs from the pristine upstream copy, or is new. */
function isConverted(abs) {
  const rel = relative(ROOT, abs)
  const ref = join(REFERENCE, rel)
  if (!existsSync(ref)) return true
  if (statSync(abs).size !== statSync(ref).size) return true
  return readFileSync(abs).compare(readFileSync(ref)) !== 0
}

/** @keyframes without a prefers-reduced-motion guard in the same file. */
function checkReducedMotion(text, rel, findings) {
  if (!/@keyframes/.test(text)) return
  if (/prefers-reduced-motion/.test(text)) return
  findings.push({
    file: rel,
    line: text.split('\n').findIndex((l) => l.includes('@keyframes')) + 1,
    rule: 'ground-rule-10-reduced-motion',
    message: 'Defines @keyframes with no prefers-reduced-motion: reduce guard.',
    snippet: '@keyframes'
  })
}

const files = (await walk(SRC)).filter(
  (f) => !EXEMPT.some((re) => re.test(f)) && /\.(vue|ts|css)$/.test(f)
)
const scoped = ALL ? files : files.filter(isConverted)

const findings = []
for (const abs of scoped) {
  const raw = readFileSync(abs, 'utf8')
  const text = stripComments(raw)
  const rel = relative(ROOT, abs)
  const lines = text.split('\n')
  // Marker lookups read the RAW source: stripComments blanks comments, which
  // would otherwise make every june-lint-disable annotation invisible to the
  // very check it exists to suppress.
  const rawLines = raw.split('\n')

  for (const rule of RULES) {
    if (!rule.files.test(rel)) continue
    lines.forEach((line, i) => {
      if (rawLines[i]?.includes('june-lint-disable')) return
      const m = line.match(rule.test)
      if (!m) return
      // Icons are explicitly sized px by design and must not scale with the
      // text-size preference, so --fs-* is the wrong tool for them.
      if (rule.id === 'ground-rule-4-token-font-sizes' && ICON_SELECTOR.test(selectorFor(lines, i))) return
      findings.push({ file: rel, line: i + 1, rule: rule.id, message: rule.message, snippet: m[0] })
    })
  }
  checkReducedMotion(text, rel, findings)
}

const scopeLabel = ALL ? 'all files' : `${scoped.length} converted file(s)`
if (!findings.length) {
  console.log(`june-lint: clean across ${scopeLabel}.`)
  process.exit(0)
}

const byRule = new Map()
for (const f of findings) byRule.set(f.rule, [...(byRule.get(f.rule) ?? []), f])

console.log(`june-lint: ${findings.length} violation(s) across ${scopeLabel}.\n`)
for (const [rule, items] of [...byRule].sort((a, b) => b[1].length - a[1].length)) {
  console.log(`  ${rule}  (${items.length})`)
  console.log(`  ${items[0].message}`)
  for (const it of items.slice(0, 8)) {
    console.log(`    ${it.file}:${it.line}  ${JSON.stringify(it.snippet).slice(0, 60)}`)
  }
  if (items.length > 8) console.log(`    ... and ${items.length - 8} more`)
  console.log()
}
process.exit(1)
