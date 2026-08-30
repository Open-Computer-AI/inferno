/**
 * The OAuth callback adopt-name / adopt-avatar panels stay fully clickable.
 *
 * These were `<label class="auth-panel">` wrapping a raw checkbox, so clicking
 * the text toggled. Converting to the design system's Checkbox meant the
 * wrapper had to stop being a label -- Checkbox renders its own, and labels
 * cannot nest -- which would have shrunk the click target to the 16px box on a
 * screen a user meets once, at signup. The panel forwards its click instead.
 */
import { describe, it, expect, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const VIEWS = ['OidcCallbackView', 'WechatCallbackView', 'DingTalkCallbackView', 'LinuxDoCallbackView']
const read = (name: string) => readFileSync(resolve(__dirname, `../${name}.vue`), 'utf8')

describe.each(VIEWS)('%s adopt panels', (name) => {
  const src = read(name)

  it('uses the design system Checkbox, not a raw input', () => {
    expect(src).not.toContain('type="checkbox"')
    expect(src).toContain("import Checkbox from '@/components/common/Checkbox.vue'")
  })

  it('does not nest a label inside Checkbox own label', () => {
    // The panel must be a div: <label><label> is invalid and breaks click
    // forwarding in both directions.
    expect(src).not.toMatch(/<label[^>]*\n\s*v-if="suggested/)
  })

  it('keeps the whole panel clickable', () => {
    expect(src).toContain('auth-panel--clickable')
    expect(src.match(/@click="togglePanel/g)?.length).toBe(2)
  })

  it('ignores clicks that came from the checkbox itself', () => {
    // Otherwise the panel handler and the control both fire and it toggles twice.
    expect(src).toContain("closest('.chk')")
  })
})

describe('togglePanel behaviour', () => {
  const togglePanel = (event: MouseEvent, toggle: () => void) => {
    if ((event.target as HTMLElement | null)?.closest('.chk')) return
    toggle()
  }

  it('toggles when the click came from the panel body', () => {
    const toggle = vi.fn()
    const panel = document.createElement('div')
    const text = document.createElement('span')
    panel.appendChild(text)
    togglePanel({ target: text } as unknown as MouseEvent, toggle)
    expect(toggle).toHaveBeenCalledOnce()
  })

  it('does not double-toggle when the click came from the checkbox', () => {
    const toggle = vi.fn()
    const panel = document.createElement('div')
    const chk = document.createElement('label')
    chk.className = 'chk'
    const input = document.createElement('input')
    chk.appendChild(input)
    panel.appendChild(chk)
    togglePanel({ target: input } as unknown as MouseEvent, toggle)
    expect(toggle).not.toHaveBeenCalled()
  })
})
