#!/usr/bin/env node
/**
 * upstream-daily — what did sub2api ship, and what is OUR route for each of it?
 *
 * WHY THIS EXISTS: catching up on upstream has, every single time, been a
 * research project. Someone fetches, reads 90 commits, decides file by file
 * whether a port is a copy or a rebuild, and writes the answers into a
 * manifest by hand. That is why the manifest header sat three counts behind
 * the rows, why five ports partially applied, and why b5827cfd5's frontend
 * half was assumed to have arrived with a merge that structurally cannot
 * carry it.
 *
 * None of that work was ever hard. It was mechanical, and mechanical work
 * done by hand once a fortnight is work done wrong. This runs it daily.
 *
 * THE ROUTING RULE, unchanged from port-classify.sh, just applied per commit:
 *
 *   no frontend/ files      -> MERGE     backend arrives free; a merge brings
 *                                        backend/ and the frontend/ mirror and
 *                                        never touches inferno-frontend/
 *   file absent in ours     -> NEW       nothing to preserve; take it whole
 *   file we never touched   -> VERBATIM  our copy IS upstream's old code, so a
 *                                        wholesale copy destroys nothing
 *   file we changed         -> REBUILD   it carries June work; a copy would
 *                                        silently revert it (this is exactly
 *                                        how four locale files were lost)
 *   any i18n/ file          -> REBUILD   always: locales accumulate June-only
 *                                        keys that no diff against an
 *                                        untouched file would reveal
 *
 * A commit's route is the most expensive route among its files, because the
 * cheap half of a mixed commit is not the half that costs you a day.
 *
 * WHAT IT IS NOT: a decision. It says what KIND of work each commit is and
 * what it touches. Whether we want the feature at all is still ours to say --
 * that is what the manifest's SKIPPED status is for.
 *
 *   node scripts/upstream-daily.mjs                 since the last recorded run
 *   node scripts/upstream-daily.mjs --day 2026-08-30
 *   node scripts/upstream-daily.mjs --since 2026-08-25
 *   node scripts/upstream-daily.mjs --json out.json
 *   node scripts/upstream-daily.mjs --html out.html
 *   node scripts/upstream-daily.mjs --record        advance the watermark
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const MANIFEST = resolve(ROOT, 'docs/superpowers/analysis/COMMIT-MANIFEST.md')
const STATE = resolve(ROOT, 'docs/superpowers/analysis/upstream-watch.json')

const git = (args, allowFail = false) => {
  try {
    return execFileSync('git', args, { cwd: ROOT, encoding: 'utf8', maxBuffer: 128 * 1024 * 1024 })
  } catch (e) {
    if (allowFail) return ''
    throw e
  }
}

const arg = (flag) => {
  const i = process.argv.indexOf(flag)
  return i > -1 ? process.argv[i + 1] : null
}
const has = (flag) => process.argv.includes(flag)

// ---------------------------------------------------------------- range

if (!has('--no-fetch')) {
  try { execFileSync('git', ['fetch', 'upstream', '--quiet'], { cwd: ROOT, stdio: 'ignore' }) } catch {}
}

if (!git(['rev-parse', '--verify', 'upstream/main'], true)) {
  console.error('no upstream/main ref. Run: git remote add upstream https://github.com/Wei-Shaw/sub2api.git && git fetch upstream')
  process.exit(2)
}

const state = existsSync(STATE) ? JSON.parse(readFileSync(STATE, 'utf8')) : {}
const day = arg('--day')
const since = arg('--since')

let range, label
if (day) {
  range = ['--since', `${day} 00:00`, '--until', `${day} 23:59`, 'upstream/main']
  label = day
} else if (since) {
  range = ['--since', since, 'upstream/main']
  label = `since ${since}`
} else {
  const from = state.lastSeen || git(['merge-base', 'HEAD', 'upstream/main']).trim()
  range = [`${from}..upstream/main`]
  label = `since ${from.slice(0, 9)}${state.lastSeenAt ? ` (last run ${state.lastSeenAt})` : ' (fork point — no watermark yet)'}`
}

/*
 * --no-merges WAS WRONG, and it cost a silent revert on 2026-09-02.
 *
 * upstream's 77729e272 is a merge that resolved a genuine conflict between
 * two divergent branches (7c01ec9be and aa7a811e6, neither an ancestor of
 * the other). Its resolution of ReasoningEffortPolicyFields.vue exists in
 * NEITHER parent -- it is the only place both features coexist. Filtering it
 * out meant the watcher reported two independent commits as if they were a
 * chain, and porting them in order reverted the first.
 *
 * So merges are listed, but only when they carry a resolution of their own:
 * `git show` on a merge prints a combined diff, which is empty for a merge
 * that took one side wholesale and non-empty exactly when the merge author
 * had to write something. That is the commit we need to see.
 */
const allShas = git(['log', '--format=%H', ...range]).trim().split('\n').filter(Boolean).reverse()
const isEmptyMerge = (sha) =>
  git(['rev-list', '--parents', '-n', '1', sha]).trim().split(' ').length > 2 &&
  !git(['show', '--format=', '--name-only', sha]).trim()
const shas = allShas.filter((sha) => !isEmptyMerge(sha))

// ---------------------------------------------------------------- routing inputs

// The vendor point is located by SUBJECT, never by hash: rebases rewrote it.
const VB = git(['log', '--format=%H', '-1', '--grep=vendor upstream frontend as the redesign target']).trim()
if (!VB) {
  console.error('cannot find the vendor-point commit by subject — routing would be a guess. Refusing to report.')
  process.exit(2)
}

/** Files under inferno-frontend/ we have deliberately changed since the vendor point. */
const touchedByUs = new Set(
  git(['diff', '--name-only', `${VB}..HEAD`, '--', 'inferno-frontend'])
    .split('\n').filter(Boolean).map((f) => f.replace(/^inferno-frontend\//, ''))
)
if (!touchedByUs.size) {
  console.error('read 0 files changed since the vendor point — routing would call everything VERBATIM, which is the exact failure that lost four locale files. Refusing to report.')
  process.exit(2)
}

const manifestText = existsSync(MANIFEST) ? readFileSync(MANIFEST, 'utf8') : ''
const inManifest = (sha) => manifestText.includes(sha.slice(0, 7))

const ROUTE_RANK = { MERGE: 0, VERBATIM: 1, NEW: 2, REBUILD: 3 }

const routeFile = (rel) => {
  if (rel.includes('/i18n/')) return 'REBUILD'
  if (!existsSync(resolve(ROOT, 'inferno-frontend', rel))) return 'NEW'
  return touchedByUs.has(rel) ? 'REBUILD' : 'VERBATIM'
}

// ---------------------------------------------------------------- collect

const commits = shas.map((sha) => {
  const meta = git(['show', '-s', '--format=%h%x00%ad%x00%an%x00%s', '--date=short', sha]).trim().split('\0')
  const files = git(['show', '--name-only', '--format=', sha]).split('\n').filter(Boolean)

  const fe = files.filter((f) => f.startsWith('frontend/')).map((f) => f.replace(/^frontend\//, ''))
  const be = files.filter((f) => f.startsWith('backend/'))
  const other = files.filter((f) => !f.startsWith('frontend/') && !f.startsWith('backend/'))

  const routed = fe.map((f) => ({ file: f, route: routeFile(f) }))
  const route = routed.length
    ? routed.reduce((w, r) => (ROUTE_RANK[r.route] > ROUTE_RANK[w] ? r.route : w), 'VERBATIM')
    : 'MERGE'

  const parents = git(['rev-list', '--parents', '-n', '1', sha]).trim().split(' ').length - 1

  return {
    sha: meta[0], date: meta[1], author: meta[2], subject: meta[3], merge: parents > 1,
    route, files: routed, backend: be.length, other: other.length,
    otherFiles: other,
    specs: fe.filter((f) => f.includes('.spec.')).length,
    known: inManifest(sha),
  }
})

// ---------------------------------------------------------------- report

const by = (r) => commits.filter((c) => c.route === r)
const counts = { MERGE: by('MERGE').length, VERBATIM: by('VERBATIM').length, NEW: by('NEW').length, REBUILD: by('REBUILD').length }
const days = [...new Set(commits.map((c) => c.date))].sort()

const BLURB = {
  MERGE: 'arrives free with a backend merge — no frontend work',
  VERBATIM: 'copy upstream\'s file wholesale; ours is still upstream\'s old code',
  NEW: 'file does not exist in our tree — take it whole',
  REBUILD: 'the file carries June work — take the LOGIC, rewrite the markup',
}

if (has('--json')) {
  writeFileSync(arg('--json'), JSON.stringify({ label, days, counts, commits }, null, 2))
}

if (has('--html')) {
  writeFileSync(arg('--html'), renderHtml({ label, days, counts, commits }))
}

/*
 * ANCESTRY, not just the watermark.
 *
 * The watermark records what we have SEEN. It says nothing about what we have
 * MERGED, and on 2026-09-02 those diverged: every frontend commit had been
 * hand-ported, so the watcher reported "nothing shipped" while
 * backend/cmd/server/VERSION sat at 0.1.185 against upstream's 0.2.0 and the
 * frontend/ mirror was four files stale. Hand-porting a frontend commit never
 * brings its backend sibling, and a stale mirror silently moves june-lint's
 * scope and port-classify's copy-vs-hand-merge verdict.
 *
 * So the report ends with the question the watermark cannot answer: is
 * upstream/main an ancestor of HEAD?
 */
const unmerged = git(['log', '--oneline', '--format=%h %s', 'upstream/main', '--not', 'HEAD'])
  .split('\n').filter(Boolean)

if (!has('--quiet')) {
  console.log(`\nsub2api — ${label}`)
  console.log(`${commits.length} commit(s) across ${days.length} day(s): ${days.join(', ') || '—'}\n`)
  if (!commits.length) console.log('  nothing shipped in this window.\n')

  for (const d of days) {
    console.log(`── ${d} ─────────────────────────────────────────`)
    for (const c of commits.filter((x) => x.date === d)) {
      const flag = (c.known ? ' [already in manifest]' : '') + (c.merge ? ' [CONFLICT RESOLUTION — port THIS, not its parents alone]' : '')
      console.log(`  ${c.route.padEnd(8)} ${c.sha}  ${c.subject.slice(0, 78)}${flag}`)
      if (c.route !== 'MERGE') {
        for (const f of c.files) console.log(`           ${f.route.padEnd(8)} ${f.file}`)
      } else if (c.backend) {
        console.log(`           ${c.backend} backend file(s)`)
      } else {
        // A "MERGE" with no backend files is not free -- it is a commit that
        // touched neither tree we track. Show the paths rather than let the
        // label imply it was handled.
        for (const f of c.otherFiles.slice(0, 6)) console.log(`           outside    ${f}`)
      }
    }
    console.log()
  }

  console.log('Route totals')
  for (const [r, n] of Object.entries(counts)) console.log(`  ${String(n).padStart(3)}  ${r.padEnd(8)} ${BLURB[r]}`)
  const work = counts.VERBATIM + counts.NEW + counts.REBUILD
  console.log(`\n  ${work} commit(s) need hands · ${counts.MERGE} arrive free`)
  console.log(`  ${commits.filter((c) => c.specs).length} ship upstream tests — those are the behaviour-parity oracle.`)

  console.log('\nAncestry')
  if (!unmerged.length) {
    console.log('  upstream/main is an ancestor of HEAD — backend and the frontend/ mirror are current.')
  } else {
    console.log(`  ${unmerged.length} upstream commit(s) are NOT in our history, even if their frontend was hand-ported:`)
    for (const l of unmerged.slice(0, 12)) console.log(`    ${l.slice(0, 92)}`)
    console.log('  A hand-port never brings a commit\'s backend half, and leaves the frontend/ mirror')
    console.log('  stale, which moves june-lint\'s scope. Run:  git merge upstream/main')
  }
}

if (has('--record')) {
  if (unmerged.length) {
    console.error(`\nrefusing to record: ${unmerged.length} upstream commit(s) are not in our history.`)
    console.error('The watermark means seen AND tracked. Merge upstream/main first, then record.')
    process.exit(1)
  }
  const head = git(['rev-parse', 'upstream/main']).trim()
  writeFileSync(STATE, JSON.stringify({ lastSeen: head, lastSeenAt: new Date().toISOString().slice(0, 10) }, null, 2) + '\n')
  console.log(`\nwatermark advanced to ${head.slice(0, 9)}`)
}

// ---------------------------------------------------------------- html

function renderHtml({ label, days, counts, commits }) {
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]))
  const HUE = { MERGE: 'merge', VERBATIM: 'verbatim', NEW: 'new', REBUILD: 'rebuild' }

  const dayBlocks = days.map((d) => {
    const rows = commits.filter((c) => c.date === d).map((c) => `
      <article class="c c--${HUE[c.route]}">
        <header class="c__h">
          <span class="c__route">${c.route}</span>
          <code class="c__sha">${esc(c.sha)}</code>
          <h3 class="c__subject">${esc(c.subject)}</h3>
          ${c.known ? '<span class="c__known">already tracked</span>' : ''}
          ${c.merge ? '<span class="c__known" style="border-color:var(--rebuild);color:var(--rebuild)">conflict resolution</span>' : ''}
        </header>
        ${c.route === 'MERGE'
          ? (c.backend
              ? `<p class="c__note">${c.backend} backend file${c.backend === 1 ? '' : 's'}${c.other ? ` · ${c.other} other` : ''} — arrives with the merge, no frontend work</p>`
              : `<p class="c__note">touches neither <code>backend/</code> nor <code>frontend/</code>: ${c.otherFiles.slice(0, 4).map(esc).join(', ')}</p>`)
          : `<ul class="c__files">${c.files.map((f) => `<li><span class="t t--${HUE[f.route]}">${f.route}</span><code>${esc(f.file)}</code></li>`).join('')}</ul>`}
        ${c.specs ? `<p class="c__specs">ships ${c.specs} upstream spec file${c.specs === 1 ? '' : 's'} — parity oracle</p>` : ''}
      </article>`).join('')
    return `<section class="day"><h2 class="day__h">${d}<span class="day__n">${commits.filter((c) => c.date === d).length} commits</span></h2>${rows}</section>`
  }).join('')

  return `<title>Upstream Watch</title>
<style>
:root{--bg:#faf8f6;--ink:#1d1a17;--dim:#6f665e;--line:#e3ddd6;--card:#fff;
--merge:#7c8b7a;--verbatim:#3f7d8c;--new:#8a6d3b;--rebuild:#b5551f;--accent:#b5551f}
:root:not([data-theme=light]) {}
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#161311;--ink:#f0ebe5;--dim:#9c9186;--line:#2e2925;--card:#1e1a17;
--merge:#9db09a;--verbatim:#69aab9;--new:#c49a5c;--rebuild:#e0763a;--accent:#e0763a}}
:root[data-theme=dark]{--bg:#161311;--ink:#f0ebe5;--dim:#9c9186;--line:#2e2925;--card:#1e1a17;
--merge:#9db09a;--verbatim:#69aab9;--new:#c49a5c;--rebuild:#e0763a;--accent:#e0763a}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);
font:15px/1.6 ui-sans-serif,-apple-system,"Segoe UI",sans-serif;padding:48px 24px 96px}
.wrap{max-width:960px;margin:0 auto;display:flex;flex-direction:column;gap:40px}
h1{font-size:34px;margin:0;letter-spacing:-.02em;text-wrap:balance}
.sub{color:var(--dim);margin:6px 0 0}
.legend{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px}
.l{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px 16px;border-left:3px solid var(--line)}
.l--merge{border-left-color:var(--merge)}.l--verbatim{border-left-color:var(--verbatim)}
.l--new{border-left-color:var(--new)}.l--rebuild{border-left-color:var(--rebuild)}
.l__n{font-size:26px;font-variant-numeric:tabular-nums;font-weight:600}
.l__k{font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:var(--dim);margin:2px 0 4px}
.l__d{font-size:12.5px;color:var(--dim);line-height:1.45}
.day__h{font-size:13px;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);
border-bottom:1px solid var(--line);padding-bottom:8px;display:flex;justify-content:space-between;margin:0 0 14px}
.day{display:flex;flex-direction:column;gap:10px}
.c{background:var(--card);border:1px solid var(--line);border-left:3px solid var(--line);border-radius:10px;padding:13px 16px}
.c--merge{border-left-color:var(--merge)}.c--verbatim{border-left-color:var(--verbatim)}
.c--new{border-left-color:var(--new)}.c--rebuild{border-left-color:var(--rebuild)}
.c__h{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}
.c__route{font-size:10.5px;letter-spacing:.09em;font-weight:700}
.c--merge .c__route{color:var(--merge)}.c--verbatim .c__route{color:var(--verbatim)}
.c--new .c__route{color:var(--new)}.c--rebuild .c__route{color:var(--rebuild)}
.c__sha{font-family:ui-monospace,SFMono-Regular,monospace;font-size:12px;color:var(--dim)}
.c__subject{font-size:14.5px;font-weight:500;margin:0;flex:1 1 320px}
.c__known{font-size:11px;color:var(--dim);border:1px solid var(--line);border-radius:99px;padding:1px 8px}
.c__note,.c__specs{margin:6px 0 0;font-size:12.5px;color:var(--dim)}
.c__files{list-style:none;margin:9px 0 0;padding:0;display:flex;flex-direction:column;gap:3px}
.c__files li{display:flex;gap:9px;align-items:baseline;font-size:12.5px;overflow-x:auto}
.c__files code{font-family:ui-monospace,SFMono-Regular,monospace;color:var(--dim);white-space:nowrap}
.t{font-size:9.5px;letter-spacing:.07em;font-weight:700;min-width:60px}
.t--verbatim{color:var(--verbatim)}.t--new{color:var(--new)}.t--rebuild{color:var(--rebuild)}
</style>
<div class="wrap">
<header>
  <h1>Upstream Watch</h1>
  <p class="sub">sub2api, ${esc(label)} — ${commits.length} commits across ${days.length} day${days.length === 1 ? '' : 's'}. Each row carries the route our tree takes for it.</p>
</header>
<section class="legend">
  ${Object.entries(counts).map(([r, n]) => `<div class="l l--${HUE[r]}"><div class="l__n">${n}</div><div class="l__k">${r}</div><div class="l__d">${BLURB[r]}</div></div>`).join('')}
</section>
${dayBlocks}
</div>`
}
