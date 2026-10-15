import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

import OpsAlertRulesCard from '../OpsAlertRulesCard.vue'
import { ref } from 'vue'
const { createAlertRule } = vi.hoisted(() => ({ createAlertRule: vi.fn() }))
vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ref(true) }))
vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listAlertRules: vi.fn().mockResolvedValue([]), createAlertRule } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))

async function openRule() {
  const wrapper = shallowMount(OpsAlertRulesCard, {
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } },
  })
  await flushPromises()
  const vm = wrapper.vm as unknown as {
    openCreate: () => void
    draft: { name: string; threshold: number; sustained_minutes: number; cooldown_minutes: number }
    editorValidation: { valid: boolean; errors: string[] }
    save: () => Promise<void>
  }
  vm.openCreate()
  await wrapper.vm.$nextTick()
  vm.draft.name = 'Error rate'
  return { wrapper, vm }
}
describe('alert rule integer durations', () => {
  beforeEach(() => { vi.clearAllMocks() })
  it.each(['1.5', '0.5'])('rejects fractional duration %s before submitting', async value => {
    const { vm } = await openRule()
    if (value === '1.5') vm.draft.sustained_minutes = Number(value)
    else vm.draft.cooldown_minutes = Number(value)
    expect(vm.editorValidation.errors).toContain(value === '1.5' ? 'admin.ops.alertRules.validation.sustainedRange' : 'admin.ops.alertRules.validation.cooldownRange')
    await vm.save()
    expect(createAlertRule).not.toHaveBeenCalled()
  })
  it('allows whole-minute boundaries and a fractional metric threshold', async () => {
    const { vm } = await openRule()
    vm.draft.sustained_minutes = 1440
    vm.draft.cooldown_minutes = 0
    vm.draft.threshold = 1.25
    expect(vm.editorValidation.valid).toBe(true)
    await vm.save()
    await flushPromises()
    expect(createAlertRule).toHaveBeenCalledWith(expect.objectContaining({ sustained_minutes: 1440, cooldown_minutes: 0, threshold: 1.25 }))
  })
})
