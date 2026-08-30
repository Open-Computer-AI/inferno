/**
 * Does every static t('...') key in the app actually resolve against the en
 * locale?
 *
 * WHY THIS EXISTS: scripts/i18n-keycheck.mjs was supposed to cover this and
 * could not. It failed two ways at once -- with no file arguments it scanned
 * nothing and still printed "all t() keys resolve", and it matched only the
 * LEAF of a dotted path, so `dates.from` passed because some unrelated block
 * happened to declare a `from:`. Both `dates.from` and the three
 * `admin.accounts.todayStatsCell.*` keys shipped rendering raw dotted strings
 * at users while that gate reported clean.
 *
 * This resolves the FULL path against the real locale object, and it discovers
 * its own inputs, so a new file cannot opt out by not being passed in.
 *
 * Dynamic keys (t('a.b.' + x)) do not match the literal pattern and are simply
 * not checked -- same deliberate blind spot the old script had.
 */
import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, resolve, relative } from 'node:path'
import en from '../locales/en'

const SRC = resolve(__dirname, '../..')

function flatten(obj: unknown, prefix = '', out = new Set<string>()): Set<string> {
  if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      const path = prefix ? `${prefix}.${k}` : k
      out.add(path)
      flatten(v, path, out)
    }
  }
  return out
}

function walk(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === '__tests__' || entry === 'locales') continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, acc)
    else if (/\.(vue|ts)$/.test(entry) && !/\.spec\.ts$/.test(entry)) acc.push(full)
  }
  return acc
}

describe('i18n key resolution', () => {
  it('every static t() key referenced in src resolves against the en locale', () => {
    const keys = flatten(en)
    const missing: string[] = []

    for (const file of walk(SRC)) {
      const src = readFileSync(file, 'utf8')
      for (const m of src.matchAll(/[^A-Za-z0-9_$]t\(\s*'([A-Za-z0-9_.]+)'/g)) {
        // A trailing dot means the literal is a prefix being concatenated with a
        // runtime value (t('a.b.' + kind)) -- not a key, and not checkable here.
        if (m[1].endsWith('.')) continue
        if (!keys.has(m[1])) missing.push(`${relative(SRC, file)}  ${m[1]}`)
      }
    }

    expect(missing).toEqual([])
  })
})
