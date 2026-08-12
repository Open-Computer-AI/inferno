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

import { execFileSync } from 'node:child_process'
import { readFileSync, existsSync, statSync } from 'node:fs'
import { readdir } from 'node:fs/promises'
import { join, relative, resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const REPO_ROOT = resolve(ROOT, '..')
const REFERENCE = resolve(REPO_ROOT, 'frontend')
const SRC = join(ROOT, 'src')
const ALL = process.argv.includes('--all')

/** Files the design system owns. Copied byte-identical from the bundle and
 *  re-synced, so they are spec, not our code, and are never linted. */
const EXEMPT = [/src\/design-system\//, /node_modules/, /\.spec\.ts$/, /__tests__/]

/**
 * Touched, but not yet converted.
 *
 * Deleting one line from a 6,000-line upstream component makes git call the
 * whole file ours, and the lint then reports every Tailwind violation upstream
 * wrote in it. That is noise: we changed one line, not the file.
 *
 * Each entry is a promise, not a permanent exemption -- REMOVE THE LINE when
 * that file is actually converted, and the lint starts holding it to June's
 * rules. All three below are already scheduled: the two account modals are
 * part 05's "two mega forms" (6,338 and 4,799 lines, called "a project rather
 * than a pass"), and ProxiesView lands with part 13.
 */
const TOUCHED_NOT_CONVERTED = [
  /src\/components\/account\/CreateAccountModal\.vue$/,
  /src\/components\/account\/EditAccountModal\.vue$/,
  /src\/views\/admin\/ProxiesView\.vue$/,
  // AccountsView: one native confirm() removed so the June ConfirmDialog is not
  // shown twice. The view itself is a large unconverted Tailwind screen and
  // lands with part 14; remove this line then.
  /src\/views\/admin\/AccountsView\.vue$/,
  // HomeView and KeyUsageView: touched only to swap a hard-coded 'Sub2API'
  // fallback for PRODUCT_NAME, so the fork does not ship upstream's name on a
  // customer-facing screen. Both are large unconverted Tailwind views carrying
  // their own transition-all, hand-coded font sizes and unguarded @keyframes.
  // They land with part 14; remove these two lines then.
  /src\/views\/HomeView\.vue$/,
  /src\/views\/KeyUsageView\.vue$/,
  // PlazaNavBar: same, touched only for PRODUCT_NAME. Its two `transition-all`
  // utilities are part 13's job; changing them here would be a visual change to
  // an unconverted screen made blind.
  /src\/components\/modelPlaza\/PlazaNavBar\.vue$/,
  // EmailVerifyView and LegalDocumentView: touched only for PRODUCT_NAME (and,
  // in the first, to narrow a `transition: all` that was a genuine
  // ground-rule-6 break). Their remaining weights and sizes are phase-4 work on
  // those screens; remove these when each is converted.
  // RegisterView came off this list when it was converted.
  /src\/views\/auth\/EmailVerifyView\.vue$/,
  /src\/views\/public\/LegalDocumentView\.vue$/,
  // SettingsView: 12,621 lines, part 14's largest view. Touched only to remove
  // a backdrop-blur glass bar (ground rule 7) from its sticky tab strip. Part
  // 14 splits it into nine routes as archetype C; remove this line then.
  /src\/views\/admin\/SettingsView\.vue$/,
  // AnnouncementBell (474 lines, in the shell so on every route) and the user
  // dashboard's quick actions: touched only to remove `transition-all`
  // (ground rule 6), `hover:scale` movement, and a backdrop-blur (rule 7).
  // Their remaining greys are a real conversion, not a patch; they land with
  // their parts. Remove these lines then.
  /src\/components\/common\/AnnouncementBell\.vue$/,
  /src\/components\/user\/dashboard\/UserDashboardQuickActions\.vue$/
]

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
  },
  {
    id: 'block-name-shadows-tailwind-utility',
    /*
     * Not a ground rule -- a trap this codebase is currently sitting in.
     *
     * Tailwind is still in the build while the rewrite runs, so a scoped BEM
     * block named after a bare utility is ALSO matched by that utility.
     * Scoped CSS does not help: the scope attribute narrows our rule, it does
     * not stop a global rule of the same name from matching the element.
     *
     * Found the hard way. OpsHealthRing's block was `ring`, so every instance
     * picked up Tailwind's `.ring` -- `box-shadow: var(--tw-ring-shadow)` with
     * the default blue-500/50 -- and painted a 3px blue box around the health
     * score, in a system with no blue in it. It shipped and survived three
     * commits because it looks deliberate.
     *
     * Matches the DECLARATION only -- a rule whose selector starts a line as a
     * bare utility name, e.g. `.ring {` or `.ring[data-tone='ok'] {`. Writing
     * `class="relative"` in a template is ordinary Tailwind use and is not a
     * collision; defining `.relative { }` in scoped CSS is. Checking the
     * declaration is what tells the two apart, and it is the half we control.
     *
     * Only bare, unprefixed utilities are listed: anything hyphenated (`flex-1`,
     * `border-t`) cannot be a BEM block anyway.
     */
    test: /^\.(?:ring|border|shadow|grid|flex|block|hidden|table|container|truncate|visible|invisible|static|fixed|absolute|relative|sticky|underline|italic|uppercase|lowercase|antialiased|isolate|contents|outline|transform|filter|blur|grayscale|invert)(?:__[a-z0-9-]+)?\s*[{,[:]/gm,
    files: /\.vue$/,
    message:
      'Block name shadows a bare Tailwind utility, which still matches it globally. Prefix it (ring -> hring).'
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

/**
 * Converted = a file WE changed. Asked of git, not computed by diffing trees.
 *
 * The obvious implementation -- "differs from ../frontend" -- is correct right
 * up until the first upstream sync, and wrong forever after. A sync advances
 * the mirror, so our untouched copy of any file upstream edited now differs
 * from it purely by being older. The first real sync put 578 violations on
 * screen that way, 284 of them upstream's own font-medium usage in stock
 * components. A lint that reports upstream's code as our violations gets muted,
 * and then it catches nothing.
 *
 * `git diff --name-only upstream/main..HEAD` lists exactly the files our own
 * commits touch, and stays correct across any number of rebases. One git call,
 * not one per file.
 */
function ourChangedFiles() {
  try {
    // The baseline is the commit that vendored the pristine copy, found by its
    // subject rather than its hash -- hashes change on every rebase, subjects
    // do not. Diffing from there gives exactly the files June has touched
    // since, which is the real definition of "converted".
    //
    // Not upstream/main: inferno-frontend/ does not exist there at all, so
    // every one of its ~730 files would count as changed.
    const base = execFileSync(
      'git',
      ['log', '--format=%H', '-1', '--grep=vendor upstream frontend as the redesign target'],
      { cwd: REPO_ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
    ).trim()
    if (!base) return null
    const out = execFileSync(
      'git',
      ['diff', '--name-only', base, '--', relative(REPO_ROOT, ROOT)],
      { cwd: REPO_ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
    )

    // Untracked files too. `git diff` cannot see them, so a brand-new component
    // -- exactly what a conversion adds most of -- was escaping the lint
    // entirely until it happened to be staged. Six page primitives were written
    // and reported clean while not being checked at all.
    const untracked = execFileSync(
      'git',
      ['ls-files', '--others', '--exclude-standard', '--', relative(REPO_ROOT, ROOT)],
      { cwd: REPO_ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
    )

    return new Set(
      (out + '\n' + untracked)
        .split('\n')
        .filter(Boolean)
        .map((p) => relative(ROOT, join(REPO_ROOT, p)))
    )
  } catch {
    return null
  }
}

/** Fallback for a non-git checkout: the old tree comparison, with its caveat. */
function differsFromMirror(abs) {
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
  (f) => !EXEMPT.some((re) => re.test(f)) && !TOUCHED_NOT_CONVERTED.some((re) => re.test(f)) && /\.(vue|ts|css)$/.test(f)
)
const ours = ALL ? null : ourChangedFiles()
if (!ALL && !ours) {
  console.warn(
    'june-lint: no git base found (upstream/main, origin/main, main). Falling back to a\n' +
    '           mirror comparison, which over-reports after an upstream sync.'
  )
}
const scoped = ALL
  ? files
  : ours
    // Both conditions, and both are needed:
    //   ours.has(...)        git says one of our commits touched it
    //   differsFromMirror()  it is not byte-identical to upstream right now
    //
    // Git alone is not enough. Porting an upstream file forward during a sync
    // is `cp frontend/x inferno-frontend/x` followed by a commit, so git
    // records it as ours even though every byte is upstream's. Ten such files
    // put 580 violations on screen after the first sync -- upstream's own
    // font-medium usage, attributed to us.
    //
    // The mirror check alone is not enough either: after a sync it flags files
    // we never touched, purely because our copy is older. Only the conjunction
    // means "we wrote this".
    ? files.filter((f) => ours.has(relative(ROOT, f)) && differsFromMirror(f))
    : files.filter(differsFromMirror)

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
