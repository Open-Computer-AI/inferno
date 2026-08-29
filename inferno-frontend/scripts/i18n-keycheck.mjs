/**
 * i18n-keycheck — does every t('...') key a file references actually resolve?
 *
 * WHY THIS EXISTS: twice now a port has added UI that calls t() for keys we
 * never brought across, and the page rendered raw dotted strings at the user.
 *   - BackupView.vue        (Phase 1.1) — 5 keys, multipart download UI
 *   - MonitorTemplateManagerDialog.vue  — 4 keys, the 8-provider tab list
 *
 * Nothing else catches it. vue-tsc does not resolve i18n keys, and vitest
 * cannot either: specs routinely stub t() to echo the key back, so they pass
 * green against entirely empty locale files.
 *
 * Usage:  node scripts/i18n-keycheck.mjs <file...>
 *         node scripts/i18n-keycheck.mjs $(git diff --name-only HEAD | grep -E '\.(vue|ts)$')
 *
 * Leaf-name matching, so it is deliberately permissive: it will not catch a key
 * nested under the wrong parent, only one that is entirely absent. Dynamic keys
 * (t('a.b.' + x)) are reported as a bare prefix and are expected noise.
 */
import { readFileSync } from 'node:fs'
import { execSync } from 'node:child_process'

// Flatten our locale files into a key set
const localeRoot = 'inferno-frontend/src/i18n/locales/en'
const files = execSync(`find ${localeRoot} -name '*.ts'`).toString().trim().split('\n')
const keys = new Set()
for (const f of files) {
  const src = readFileSync(f, 'utf8')
  // collect every "identifier:" at any depth; good enough to test leaf existence
  for (const m of src.matchAll(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:/gm)) keys.add(m[1])
}

// Every t('...') key referenced in the files under test
const targets = process.argv.slice(2)
const missing = []
for (const f of targets) {
  let src
  try { src = readFileSync(f, 'utf8') } catch { continue }
  for (const m of src.matchAll(/[^A-Za-z]t\(\s*['"`]([A-Za-z0-9_.]+)['"`]/g)) {
    const leaf = m[1].split('.').pop()
    if (!keys.has(leaf)) missing.push(`${f}  ${m[1]}`)
  }
}
if (missing.length) { console.log(missing.join('\n')); console.log(`\n${missing.length} unresolved key reference(s)`) }
else console.log('all t() keys resolve')
