import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * The landing page is gone: `/` opens on login, the way oc-router does it.
 *
 * This is a source-level assertion rather than a guard simulation because the
 * claim is about the ROUTE TABLE, not about guard logic -- `/` is a redirect
 * record, so vue-router resolves it before any guard runs and guards.spec's
 * simulateGuard can never see it. Importing router/index.ts for real would drag
 * in the stores and every guard side effect for two facts, so this follows the
 * precedent already set by siteLogoSanitization.spec.ts and reads the source.
 *
 * HomeView.vue deliberately stays on disk and unreferenced by the router: it is
 * ported work that port-coverage and conversion-status still count, and
 * deleting it would make those baselines fall as though the port had been lost.
 */
const src = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts'),
  'utf8'
)

describe('landing page removal', () => {
  it('/ redirects to /dashboard, not to a landing page', () => {
    expect(src).toMatch(/path:\s*'\/'\s*,\s*\n\s*redirect:\s*'\/dashboard'/)
  })

  it('declares no /home route', () => {
    expect(src).not.toMatch(/path:\s*'\/home'/)
  })

  it('declares no route named Home, which a named push would throw on', () => {
    expect(src).not.toMatch(/name:\s*'Home'/)
  })
})
