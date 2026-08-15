import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { useDialogHost, useDialogService } from '../useDialogService'

describe('useDialogService', () => {
  const { confirm, prompt } = useDialogService()
  const { activeRequest, settle } = useDialogHost()

  afterEach(async () => {
    if (activeRequest.value) settle(null)
    await nextTick()
  })

  it('resolves confirmation requests through the shared active host', async () => {
    const pending = confirm({ message: 'Delete this?' })
    expect(activeRequest.value?.kind).toBe('confirm')

    settle(true)
    await expect(pending).resolves.toBe(true)
    expect(activeRequest.value).toBeNull()
  })

  it('queues requests and preserves prompt values and cancellation', async () => {
    const first = confirm({ message: 'First' })
    const second = prompt({ defaultValue: 'Draft' })
    expect(activeRequest.value?.kind).toBe('confirm')

    settle(false)
    await expect(first).resolves.toBe(false)
    await nextTick()
    expect(activeRequest.value?.kind).toBe('prompt')

    settle('Renamed')
    await expect(second).resolves.toBe('Renamed')
    expect(activeRequest.value).toBeNull()
  })
})
