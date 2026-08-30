import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('rail__nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain('const sidebarNavRef = ref<HTMLElement | null>(null)')
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar branding', () => {
  it('keeps the wordmark without rendering a standalone logo mark', () => {
    expect(componentSource).toContain('rail__brand-name')
    expect(componentSource).toContain('{{ siteName }}')
    expect(componentSource).not.toContain('rail__mark')
    expect(componentSource).not.toContain('siteLogo')
    expect(componentSource).not.toContain('markInitial')
  })
})

describe('AppSidebar popover positioning', () => {
  // Part 07 v2 moves the version badge's dropdown-clipping fix (the reason
  // this file originally existed) into a structural guarantee instead of a
  // one-off style rule: every interactive popover -- the section switcher,
  // the preferences menu, the avatar menu -- is teleported to <body> and
  // positioned off its trigger's own bounding rect, the same pattern
  // Select.vue uses. A popover living outside the rail's DOM subtree cannot
  // be clipped by an ancestor's overflow, whatever that ancestor's CSS says.
  it('teleports every popover to <body> instead of nesting it under an overflow ancestor', () => {
    // One popover now, not three. The section switcher is gone (it hid 11 of 16
    // nav rows behind a mode), and the preferences menu was folded into the
    // avatar menu -- language and theme are account-scoped, and a second gear in
    // the footer read as "Settings" beside the real one in the nav.
    const teleportCount = (componentSource.match(/<Teleport to="body">/g) ?? []).length
    expect(teleportCount).toBe(1)
  })

  it('positions each popover from its own trigger rect, not a static offset', () => {
    expect(componentSource).toContain('getBoundingClientRect()')
    expect(componentSource).toContain("position: 'fixed'")
  })
})

describe('AppSidebar route reachability', () => {
  // The June reduction planned to fold channel status into the channels table
  // and orders into Billing's lower half. Neither fold was built, so all three
  // routes shipped with no entry point anywhere in the app -- /monitor linked
  // from nowhere at all. Remove a row here only once its destination is
  // genuinely reachable somewhere else.
  it.each(['/monitor', '/purchase', '/orders'])('keeps a nav entry for %s', (path) => {
    expect(componentSource).toContain(`path: '${path}'`)
  })

  it('gates /admin/prompt-audit on the same flag its router guard uses', () => {
    // The route carries `requiresRiskControl`, so an ungated row is a dead link
    // that bounces the admin to /admin/settings.
    const gated = componentSource.matchAll(
      /isFeatureFlagEnabled\(FeatureFlags\.riskControl\)\)\s*\{([\s\S]*?)\n  \}/g
    )
    const blocks = Array.from(gated, (m) => m[1])
    expect(blocks.length).toBeGreaterThan(0)
    for (const occurrence of componentSource.matchAll(/path: '\/admin\/prompt-audit'/g)) {
      expect(blocks.some((b) => b.includes("path: '/admin/prompt-audit'"))).toBe(true)
      expect(occurrence).toBeTruthy()
    }
  })
})
