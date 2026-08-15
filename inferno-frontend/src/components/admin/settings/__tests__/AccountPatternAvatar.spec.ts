import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountPatternAvatar from '../AccountPatternAvatar.vue'

describe('AccountPatternAvatar', () => {
  it('derives a stable pattern from the same account seed', () => {
    const first = mount(AccountPatternAvatar, { props: { seed: 'admin@example.com' } })
    const second = mount(AccountPatternAvatar, { props: { seed: 'admin@example.com' } })

    expect(first.attributes('style')).toBe(second.attributes('style'))
    expect(first.attributes('style')).toContain('--avatar-cloud-x:')
    expect(first.attributes('style')).toContain('--avatar-cloud-angle:')
  })

  it('changes the pattern when the account seed changes and respects size', () => {
    const first = mount(AccountPatternAvatar, {
      props: { seed: 'admin@example.com', size: 40 },
    })
    const second = mount(AccountPatternAvatar, {
      props: { seed: 'other@example.com', size: 40 },
    })

    expect(first.attributes('style')).not.toBe(second.attributes('style'))
    expect(first.attributes('style')).toContain('width: 40px')
    expect(first.attributes('style')).toContain('height: 40px')
  })
})
