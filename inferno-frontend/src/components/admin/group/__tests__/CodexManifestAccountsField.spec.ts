import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { CodexModelsManifestConfig } from '@/types'

const { listAccounts } = vi.hoisted(() => ({ listAccounts: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { list: listAccounts } }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import CodexManifestAccountsField from '../CodexManifestAccountsField.vue'

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:model-value'],
  template: '<button type="button" @click="$emit(\'update:model-value\', !modelValue)"></button>'
})

const initialConfig: CodexModelsManifestConfig = {
  enabled: true,
  account_ids: [],
  fallback_to_scheduler: true
}

const disabledConfig: CodexModelsManifestConfig = {
  enabled: false,
  account_ids: [],
  fallback_to_scheduler: false
}

function mountField(modelValue = initialConfig) {
  return mount(CodexManifestAccountsField, {
    props: { groupId: 42, modelValue },
    global: { stubs: { Toggle: ToggleStub, Icon: true } }
  })
}

describe('CodexManifestAccountsField', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listAccounts.mockReset().mockResolvedValue({
      items: [{ id: 17, name: 'Codex pool account' }]
    })
  })

  afterEach(() => vi.useRealTimers())

  it('hides account controls while disabled and emits the enable toggle', async () => {
    const wrapper = mountField(disabledConfig)

    expect(wrapper.find('[data-testid="codex-manifest-search"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="codex-manifest-fallback-toggle"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.groups.codexModelsManifest.disabledHint')

    await wrapper.get('[data-testid="codex-manifest-toggle"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({ enabled: true })

    await wrapper.setProps({ modelValue: { ...disabledConfig, enabled: true } })
    expect(wrapper.find('[data-testid="codex-manifest-search"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-manifest-fallback-toggle"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.groups.codexModelsManifest.enabledHint')
    wrapper.unmount()
  })

  it('searches only OpenAI accounts in the current group and emits the selected ID', async () => {
    const wrapper = mountField()
    const search = wrapper.get<HTMLInputElement>('[data-testid="codex-manifest-search"]')
    await search.trigger('focus')
    await search.setValue('codex')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      { search: 'codex', platform: 'openai', group: '42' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    await wrapper.get('[data-testid="codex-manifest-dropdown"] button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({
      ...initialConfig,
      account_ids: [17]
    })
    wrapper.unmount()
  })

  it('requires at least one account when enabled and allows removing selected accounts', async () => {
    const wrapper = mountField({
      ...initialConfig,
      account_ids: [17]
    })
    const exposed = wrapper.vm as unknown as { validate: () => boolean }

    expect(exposed.validate()).toBe(true)
    await wrapper.get('[aria-label="remove account 17"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual({
      ...initialConfig,
      account_ids: []
    })
    await wrapper.setProps({ modelValue: { ...initialConfig, account_ids: [] } })
    expect(exposed.validate()).toBe(false)
    await flushPromises()
    expect(wrapper.get('[data-testid="codex-manifest-validation-error"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows the empty-search state and closes the dropdown on an outside click', async () => {
    listAccounts.mockResolvedValueOnce({ items: [] })
    const wrapper = mountField()
    const search = wrapper.get<HTMLInputElement>('[data-testid="codex-manifest-search"]')
    await search.trigger('focus')
    await search.setValue('missing')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(wrapper.get('[data-testid="codex-manifest-search-empty"]').exists()).toBe(true)
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
    expect(wrapper.find('[data-testid="codex-manifest-dropdown"]').exists()).toBe(false)

    await search.trigger('focus')
    expect(wrapper.find('[data-testid="codex-manifest-dropdown"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
