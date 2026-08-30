/**
 * At-capacity must read as at-capacity.
 *
 * The June rewrite turned upstream's `used >= max` into `pct > 100` in both of
 * these components. An enforced cap never exceeds 100%, so the destructive tone
 * became unreachable and a fully saturated group or user rendered as merely
 * "attention" -- losing the one signal an operator scans the table for.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UserConcurrencyCell from '../../user/UserConcurrencyCell.vue'

const stubs = { CapacityBar: true }

describe('UserConcurrencyCell tone', () => {
  it('renders the destructive tone at exactly the cap, not only above it', () => {
    const full = mount(UserConcurrencyCell, { props: { current: 5, max: 5 }, global: { stubs } })
    expect(full.find('.ucc__value').attributes('style')).toContain('--destructive')
  })

  it('stays out of the destructive tone below the cap', () => {
    const partial = mount(UserConcurrencyCell, { props: { current: 4, max: 5 }, global: { stubs } })
    expect(partial.find('.ucc__value').attributes('style')).not.toContain('--destructive')
  })

  it('falls back to neutral when no cap is set', () => {
    const unset = mount(UserConcurrencyCell, { props: { current: 3, max: 0 }, global: { stubs } })
    expect(unset.find('.ucc__value').attributes('style')).toContain('--muted-foreground')
  })
})
