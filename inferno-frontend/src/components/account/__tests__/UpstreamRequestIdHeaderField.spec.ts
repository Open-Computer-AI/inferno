import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UpstreamRequestIdHeaderField from '../UpstreamRequestIdHeaderField.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.platform ? `${key}:${String(params.platform)}` : key
    })
  }
})

function mountField(props: { platform?: string; type?: string; modelValue?: string }) {
  return mount(UpstreamRequestIdHeaderField, {
    props,
    global: { stubs: { Teleport: true } }
  })
}

describe('UpstreamRequestIdHeaderField', () => {
  it('shows relay and official header examples for API-key accounts', () => {
    const wrapper = mountField({ platform: 'openai', type: 'apikey' })

    expect(wrapper.text()).toContain('X-Client-Request-ID')
    expect(wrapper.text()).toContain('X-Oneapi-Request-Id')
    expect(wrapper.text()).toContain('admin.accounts.upstreamRequestIdHeaderHelp.official:OpenAI')
    expect(wrapper.text()).toContain('x-request-id')
  })

  it('shows only the official header for direct-platform OAuth accounts', () => {
    const wrapper = mountField({ platform: 'anthropic', type: 'oauth' })

    expect(wrapper.text()).toContain('admin.accounts.upstreamRequestIdHeaderHelp.official:Anthropic')
    expect(wrapper.text()).toContain('request-id')
    expect(wrapper.text()).not.toContain('X-Client-Request-ID')
    expect(wrapper.text()).not.toContain('X-Oneapi-Request-Id')
  })

  it('omits examples when the platform has no documented request-ID header', () => {
    const wrapper = mountField({ platform: 'kimi', type: 'oauth' })

    expect(wrapper.text()).toContain('admin.accounts.upstreamRequestIdHeaderHelp.intro')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamRequestIdHeaderHelp.examplesTitle')
    expect(wrapper.findAll('code')).toHaveLength(0)
  })

  it('binds the configured header through v-model', async () => {
    const wrapper = mountField({ platform: 'openai', type: 'apikey', modelValue: 'X-Request-ID' })
    const input = wrapper.get<HTMLInputElement>('[data-testid="upstream-request-id-header"]')
    expect(input.element.value).toBe('X-Request-ID')

    await input.setValue('X-Oneapi-Request-Id')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['X-Oneapi-Request-Id'])
  })
})
