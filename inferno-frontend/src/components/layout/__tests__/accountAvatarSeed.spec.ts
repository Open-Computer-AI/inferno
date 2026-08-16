import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppSidebar from '@/components/layout/AppSidebar.vue'
import type { User } from '@/types'

/* The rail's generated identicon must be derived from the same seed the profile
   page uses. ProfileAvatarCard writes `avatar_seed` when the user regenerates,
   so a rail that only ever hashed id/email kept painting the old pattern and the
   same user appeared with two different avatars on screen at once. */

const { authState } = vi.hoisted(() => ({
  authState: {
    user: null as User | null,
    isAdmin: false,
    isSimpleMode: false,
    logout: vi.fn()
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    mobileOpen: false,
    sidebarScrollTop: 0,
    siteName: 'Inferno',
    siteLogo: '',
    publicSettingsLoaded: true,
    cachedPublicSettings: null,
    setMobileOpen: vi.fn(),
    toggleSidebar: vi.fn()
  }),
  useAuthStore: () => authState,
  useOnboardingStore: () => ({
    isCurrentStep: () => false,
    nextStep: vi.fn()
  }),
  useAdminSettingsStore: () => ({
    customMenuItems: [],
    opsMonitoringEnabled: false,
    paymentEnabled: false,
    fetch: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard', name: 'dashboard' }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  RouterLink: defineComponent({
    name: 'RouterLink',
    props: { to: { type: [String, Object], default: '' } },
    setup: (_props, { slots }) => () => h('a', {}, slots.default?.())
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      stubs: {
        AnnouncementBell: true,
        LocaleSwitcher: true,
        AccountPatternAvatar: {
          name: 'AccountPatternAvatar',
          props: ['seed', 'size'],
          template: '<span class="account-pattern-avatar" :data-seed="seed" />'
        },
        Teleport: true
      }
    }
  })
}

function seedOf(wrapper: ReturnType<typeof mountSidebar>) {
  return wrapper.find('.account-pattern-avatar').attributes('data-seed')
}

describe('AppSidebar account avatar seed', () => {
  beforeEach(() => {
    // isFeatureFlagEnabled reaches for the real app store, so the nav rows need
    // a live pinia even though the sidebar's own stores are mocked above.
    setActivePinia(createPinia())
    authState.user = null
  })

  it('prefers avatar_seed over id and email', () => {
    authState.user = {
      id: 42,
      username: 'ada',
      email: 'ada@example.com',
      avatar_seed: 'regenerated-seed-abc'
    } as unknown as User

    const wrapper = mountSidebar()
    expect(seedOf(wrapper)).toBe('regenerated-seed-abc')
  })

  it('falls back to id when avatar_seed is absent', () => {
    authState.user = {
      id: 42,
      username: 'ada',
      email: 'ada@example.com'
    } as unknown as User

    const wrapper = mountSidebar()
    expect(seedOf(wrapper)).toBe('42')
  })

  it('falls back to email when there is no id and no avatar_seed', () => {
    authState.user = {
      id: 0,
      username: '',
      email: 'ada@example.com'
    } as unknown as User

    const wrapper = mountSidebar()
    expect(seedOf(wrapper)).toBe('ada@example.com')
  })

  it('keeps the inferno-account fallback when nothing identifies the user', () => {
    authState.user = { id: 0, username: '', email: '' } as unknown as User

    const wrapper = mountSidebar()
    expect(seedOf(wrapper)).toBe('inferno-account')
  })

  it('derives the rail seed the same way the profile components do', () => {
    // ProfileAvatarCard and ProfileSettingsSidebar are the two other readers of
    // avatar_seed; if any of the three drifts the user sees mismatched avatars.
    authState.user = {
      id: 7,
      username: 'grace',
      email: 'grace@example.com',
      avatar_seed: 'shared-seed'
    } as unknown as User

    const profileSeed =
      authState.user.avatar_seed ||
      String(authState.user.id || authState.user.email || authState.user.username)

    const wrapper = mountSidebar()
    expect(seedOf(wrapper)).toBe(profileSeed)
  })
})
