import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupSelector from '../GroupSelector.vue'

const authState = { isSimpleMode: false }

vi.mock('@/stores', () => ({ useAuthStore: () => authState }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const groups = [
  {
    id: 1,
    name: 'Basic',
    platform: 'anthropic',
    status: 'active',
    rate_multiplier: 1,
    account_count: 1,
    rate_limited_account_count: 0
  },
  {
    id: 2,
    name: 'Composite',
    platform: 'composite',
    status: 'active',
    rate_multiplier: 1,
    account_count: 1,
    rate_limited_account_count: 0
  }
] as any

const mountSelector = (modelValue: number[] = []) =>
  mount(GroupSelector, {
    props: { modelValue, groups },
    global: {
      stubs: {
        Checkbox: {
          props: ['modelValue'],
          template: '<span class="checkbox-stub" />'
        }
      }
    }
  })

describe('GroupSelector simple-mode binding policy', () => {
  beforeEach(() => {
    authState.isSimpleMode = false
  })

  it('hides composite groups in simple mode and preserves basic groups', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector()

    expect(wrapper.text()).toContain('Basic')
    expect(wrapper.text()).not.toContain('Composite')
  })

  it('keeps composite groups available in advanced mode', () => {
    const wrapper = mountSelector()

    expect(wrapper.text()).toContain('Composite')
  })

  it('cleans hidden historical composite IDs while preserving visible selections', () => {
    authState.isSimpleMode = true
    const wrapper = mountSelector([1, 2])

    expect(wrapper.emitted('update:modelValue')).toEqual([[[1]]])
  })
})
