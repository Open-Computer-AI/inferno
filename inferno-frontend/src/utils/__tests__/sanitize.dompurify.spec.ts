import { describe, expect, it } from 'vitest'
import DOMPurify from 'dompurify'

// dompurify guards every admin-authored surface that renders HTML: the
// announcement bell and popup (shown on every route), the public legal
// document and custom pages, the model plaza description, and the compliance
// dialog. 4a1da2950 bumps it past a sanitizer-bypass range, so this pins the
// floor and keeps a few basic escapes covered.
describe('dompurify sanitizer', () => {
  it('is at or past the patched version', () => {
    expect(DOMPurify.version >= '3.4.14').toBe(true)
  })

  it.each([
    ['<img src=x onerror=alert(1)>', 'onerror'],
    ['<script>alert(1)</script>', 'script'],
    ['<a href="javascript:alert(1)">x</a>', 'javascript:'],
    ['<svg><animate onbegin=alert(1) attributeName=x></svg>', 'onbegin'],
    ['<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>', 'onerror'],
  ])('strips %s', (payload, forbidden) => {
    expect(DOMPurify.sanitize(payload).toLowerCase()).not.toContain(forbidden)
  })
})
