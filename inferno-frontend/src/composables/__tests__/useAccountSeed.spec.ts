import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { useAccountSeed, type AccountSeedUser } from '@/composables/useAccountSeed'

/* This derivation used to be hand-copied into four components and drifted:
   AppSidebar was missing the `avatar_seed` branch, so a regenerated avatar
   showed on the profile page and the old one stayed in the rail. These cases
   pin the precedence chain that all four now share. */

describe('useAccountSeed', () => {
  it('prefers avatar_seed over everything else', () => {
    const seed = useAccountSeed(
      { id: 42, email: 'ada@example.com', avatar_seed: 'regenerated-abc' },
      'ada'
    )
    expect(seed.value).toBe('regenerated-abc')
  })

  it('falls back to the id, stringified, when there is no avatar_seed', () => {
    const seed = useAccountSeed({ id: 42, email: 'ada@example.com' }, 'ada')
    expect(seed.value).toBe('42')
  })

  it('falls back to the email when the id is absent or a placeholder zero', () => {
    expect(useAccountSeed({ email: 'ada@example.com' }, 'ada').value).toBe('ada@example.com')
    // id 0 is a placeholder, not an identity: `||` must fall through it rather
    // than seed every such user with the shared string '0'.
    expect(useAccountSeed({ id: 0, email: 'ada@example.com' }, 'ada').value).toBe('ada@example.com')
  })

  it('falls back to the display name when there is no id and no email', () => {
    expect(useAccountSeed({ id: 0, email: '' }, 'ada').value).toBe('ada')
  })

  it('returns an empty seed when nothing identifies the user and no fallback is given', () => {
    expect(useAccountSeed({ id: 0, email: '' }, '').value).toBe('')
  })

  it('honours a custom final fallback only once everything else is exhausted', () => {
    expect(useAccountSeed({ id: 0, email: '' }, '', 'inferno-account').value).toBe('inferno-account')
    // The fallback must not outrank a real display name.
    expect(useAccountSeed({ id: 0, email: '' }, 'ada', 'inferno-account').value).toBe('ada')
    expect(useAccountSeed({ id: 7, email: '' }, '', 'inferno-account').value).toBe('7')
  })

  it('handles a null or undefined user', () => {
    expect(useAccountSeed(null, 'ada').value).toBe('ada')
    expect(useAccountSeed(undefined, 'ada').value).toBe('ada')
    expect(useAccountSeed(null, '').value).toBe('')
    expect(useAccountSeed(null, '', 'inferno-account').value).toBe('inferno-account')
  })

  it('accepts a ref source and tracks it reactively', () => {
    const user = ref<AccountSeedUser | null>(null)
    const seed = useAccountSeed(user, 'ada')
    expect(seed.value).toBe('ada')

    user.value = { id: 42, email: 'ada@example.com' }
    expect(seed.value).toBe('42')

    user.value = { id: 42, email: 'ada@example.com', avatar_seed: 'regenerated-abc' }
    expect(seed.value).toBe('regenerated-abc')
  })

  it('accepts a getter source, so a component can pass a prop or a store', () => {
    const store = { user: { id: 9, email: 'grace@example.com' } as AccountSeedUser | null }
    const seed = useAccountSeed(() => store.user, 'grace')
    expect(seed.value).toBe('9')
  })

  it('tracks a reactive display name', () => {
    const name = ref('')
    const seed = useAccountSeed(null, name, 'inferno-account')
    expect(seed.value).toBe('inferno-account')

    name.value = 'ada'
    expect(seed.value).toBe('ada')
  })

  it('accepts a computed display name, which is how every call site passes it', () => {
    const user = ref<AccountSeedUser | null>({ id: 0, email: '', avatar_seed: '' })
    const displayName = computed(() => user.value?.email || 'fallback-name')
    const seed = useAccountSeed(user, displayName)
    expect(seed.value).toBe('fallback-name')
  })
})
