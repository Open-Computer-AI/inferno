import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

/* site_logo is operator-controlled and lands in an <img :src>, so every consumer
   must run it through sanitizeUrl. These are source assertions rather than mount
   tests on purpose: the risk is someone adding a NEW consumer that binds the raw
   value, and only reading the source catches that.

   AppSidebar was a consumer until 7009d86ac ("fix(ui): remove sidebar brand
   mark") replaced the logo with the site name as text. It has no <img :src> left
   to sanitize, so it is deliberately not asserted here. Its own spec covers the
   text-only rail. If a logo ever returns to the sidebar, add it back to BOTH
   lists below. */

const dir = dirname(fileURLToPath(import.meta.url))
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')

describe('site_logo sanitization', () => {
  it('HomeView applies sanitizeUrl to siteLogo', () => {
    expect(homeViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('KeyUsageView applies sanitizeUrl to siteLogo', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('every consumer passes allowRelative and allowDataUrl', () => {
    for (const src of [homeViewSource, keyUsageViewSource]) {
      expect(src).toContain('allowRelative: true')
      expect(src).toContain('allowDataUrl: true')
    }
  })

  it('AppSidebar still renders no logo, so it needs no sanitizer', () => {
    // Guards the inverse: if someone reintroduces a site_logo binding here
    // without a sanitizer, this fails and points them at the block above.
    expect(sidebarSource).not.toContain('siteLogo')
  })
})
