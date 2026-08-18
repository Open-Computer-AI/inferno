import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')

// AppSidebar dropped its standalone logo mark entirely (c7906aaf6, "remove
// sidebar brand mark") -- covered by AppSidebar.spec.ts's own
// "keeps the wordmark without rendering a standalone logo mark" test, which
// asserts `siteLogo` is absent from the component. HomeView and KeyUsageView
// still render an admin-settable site_logo image and remain in scope here.
describe('site_logo sanitization', () => {
  it('AppSidebar no longer renders a site logo, so has nothing to sanitize', () => {
    expect(sidebarSource).not.toContain('siteLogo')
  })

  it('HomeView applies sanitizeUrl to siteLogo', () => {
    expect(homeViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('KeyUsageView applies sanitizeUrl to siteLogo', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('both remaining consumers pass allowRelative and allowDataUrl options', () => {
    for (const src of [homeViewSource, keyUsageViewSource]) {
      expect(src).toContain('allowRelative: true')
      expect(src).toContain('allowDataUrl: true')
    }
  })
})
