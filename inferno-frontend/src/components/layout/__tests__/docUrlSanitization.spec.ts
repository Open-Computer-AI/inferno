import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
// AppHeader.vue was deleted in the part 07 v2 shell rewrite (no header at
// all -- see AppLayout.vue and AppSidebar.vue). Its `docUrl` link had no new
// home in the redesign (part 07 v2 lists the header's whole inventory as
// four items -- breadcrumb, notifications, language/theme, avatar menu --
// and a docs link is not one of them), so the shell renders no doc_url
// anywhere any more. The two tests below replace the old
// "AppHeader applies sanitizeUrl to docUrl" pair: they guard against the
// regression the original bug class actually was -- a raw, unsanitized
// doc_url reaching the DOM -- now scoped to the files that make up the
// shell instead of a file that no longer exists.
const layoutSource = readFileSync(resolve(dir, '../AppLayout.vue'), 'utf8')
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')

describe('doc_url sanitization', () => {
  it('AppLayout does not render a raw docUrl', () => {
    expect(layoutSource).not.toMatch(/\bdocUrl\b/)
  })

  it('AppSidebar does not render a raw docUrl', () => {
    expect(sidebarSource).not.toMatch(/\bdocUrl\b/)
  })

  it('HomeView imports sanitizeUrl', () => {
    expect(homeViewSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('HomeView applies sanitizeUrl to docUrl', () => {
    expect(homeViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl')
  })

  it('KeyUsageView imports sanitizeUrl', () => {
    expect(keyUsageViewSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('KeyUsageView applies sanitizeUrl to docUrl', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl')
  })
})
