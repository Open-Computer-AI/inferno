import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SupportedModelChip from '../SupportedModelChip.vue'
import type { UserSupportedModel } from '@/api/channels'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const tokenFields = {
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  per_request_price: null
}

describe('SupportedModelChip pricing', () => {
  it.each(['availableChannels.pricing', 'admin.availableChannels.pricing'])(
    'shows video prices per second using %s translations',
    async (pricingKeyPrefix) => {
      const model: UserSupportedModel = {
        name: 'video-test',
        platform: '',
        pricing: {
          billing_mode: 'video',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          image_input_price: null,
          image_output_price: null,
          per_request_price: 0.05,
          intervals: [
            { ...tokenFields, min_tokens: 0, max_tokens: null, tier_label: '480p', per_request_price: 0 },
            { ...tokenFields, min_tokens: 0, max_tokens: null, tier_label: '720p', per_request_price: 0.12 }
          ]
        }
      }
      const wrapper = mount(SupportedModelChip, {
        attachTo: document.body,
        props: { pricingKeyPrefix, model },
        global: { stubs: { PlatformIcon: true } }
      })

      try {
        await wrapper.find('[tabindex="0"]').trigger('mouseenter')
        await nextTick()
        const tooltip = document.body.querySelector('[role="tooltip"]')
        expect(tooltip?.textContent).toContain(`${pricingKeyPrefix}.billingModeVideo`)
        expect(tooltip?.textContent).toContain(`${pricingKeyPrefix}.videoPrice`)
        expect(tooltip?.textContent).toContain(`$0.05 ${pricingKeyPrefix}.unitPerSecond`)
        expect(tooltip?.textContent).toContain('480p')
        expect(tooltip?.textContent).toContain(`$0 ${pricingKeyPrefix}.unitPerSecond`)
        expect(tooltip?.textContent).toContain('720p')
        expect(tooltip?.textContent).toContain(`$0.12 ${pricingKeyPrefix}.unitPerSecond`)

        await wrapper.setProps({
          model: { ...model, pricing: { ...model.pricing!, per_request_price: null } }
        })
        expect(tooltip?.textContent).not.toContain(`${pricingKeyPrefix}.videoPrice`)
        expect(tooltip?.textContent).toContain(`$0.12 ${pricingKeyPrefix}.unitPerSecond`)

        await wrapper.setProps({
          model: { ...model, pricing: { ...model.pricing!, per_request_price: 0 } }
        })
        expect(tooltip?.textContent).toContain(`${pricingKeyPrefix}.videoPrice`)
        expect(wrapper.findComponent({ name: 'PricingRow' }).text()).toContain(`$0 ${pricingKeyPrefix}.unitPerSecond`)
      } finally {
        wrapper.unmount()
      }
    }
  )

  it('resolves multiplier-only token intervals against the model base prices', async () => {
    const wrapper = mount(SupportedModelChip, {
      attachTo: document.body,
      props: {
        model: {
          name: 'gpt-test',
          platform: '',
          pricing: {
            billing_mode: 'token',
            input_price: 10e-6,
            output_price: 50e-6,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: null,
            intervals: [{
              min_tokens: 272000,
              max_tokens: null,
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              input_multiplier: 2,
              output_multiplier: 1.5,
              per_request_price: null
            }]
          }
        },
        showPlatform: false
      },
      global: { stubs: { PlatformIcon: true } }
    })

    await wrapper.find('[tabindex="0"]').trigger('mouseenter')
    await nextTick()

    expect(document.body.textContent).toContain('$20 / $75')
    wrapper.unmount()
  })
})
