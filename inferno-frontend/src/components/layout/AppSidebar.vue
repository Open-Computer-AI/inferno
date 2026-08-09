<template>
  <aside
    class="rail"
    :class="{ 'rail--collapsed': sidebarCollapsed, 'rail--mobile-open': mobileOpen }"
  >
    <!-- Brand row: mark, name, collapse toggle. The toggle lives here (not the
         footer) per part 07 v2's anatomy: "brand row with a collapse toggle". -->
    <div class="rail__brand">
      <router-link :to="homePath" class="rail__mark" @click="handleMenuItemClick(homePath)">
        <img v-if="settingsLoaded && siteLogo" :src="siteLogo" alt="" class="rail__mark-img" />
        <span v-else class="rail__mark-fallback" aria-hidden="true">{{ markInitial }}</span>
      </router-link>
      <router-link
        :to="homePath"
        class="rail__brand-name"
        @click="handleMenuItemClick(homePath)"
        >{{ siteName }}</router-link
      >
      <button
        type="button"
        class="rail__icon-btn rail__collapse"
        :aria-label="sidebarCollapsed ? t('shell.expandSidebar') : t('shell.collapseSidebar')"
        :aria-expanded="!sidebarCollapsed"
        @click="toggleSidebar"
      >
        <i class="hgi-stroke hgi-sidebar-left" aria-hidden="true" />
      </button>
    </div>

    <!-- Admin only: the six-section switcher. Standard mode only -- simple mode's
         filtered item count is short enough that a flat list reads faster than a
         switcher, the same principle part 07 v2 applies to the customer shell. -->
    <div v-if="isAdmin && !isSimpleMode" class="rail__switcher">
      <button
        ref="switcherTriggerRef"
        type="button"
        class="rail__switcher-trigger"
        aria-haspopup="listbox"
        :aria-expanded="openPopover === 'switcher'"
        :aria-label="t('shell.switchSection')"
        @click="togglePopover('switcher')"
        @keydown="onTriggerKeydown('switcher', $event)"
      >
        <i :class="['hgi-stroke', activeSectionDef?.icon]" aria-hidden="true" />
        <span class="rail__switcher-label">{{ activeSectionDef?.label }}</span>
        <i class="hgi-stroke hgi-arrow-up-down" aria-hidden="true" />
      </button>

      <Teleport to="body">
        <div
          v-if="openPopover === 'switcher'"
          ref="switcherPanelRef"
          class="rail__popover"
          :style="popoverStyle"
          role="listbox"
          :aria-label="t('shell.sectionsListLabel')"
          @keydown="onPanelKeydown('switcher', $event)"
        >
          <button
            v-for="section in adminSections"
            :key="section.key"
            type="button"
            role="option"
            class="rail__popover-row"
            :aria-selected="section.key === activeSection"
            :data-active="section.key === activeSection || undefined"
            @click="selectSection(section.key)"
          >
            <i :class="['hgi-stroke', section.icon]" aria-hidden="true" />
            <span class="rail__popover-row-label">{{ section.label }}</span>
            <span class="rail__popover-row-count">{{ section.rows.length }}</span>
          </button>
        </div>
      </Teleport>
    </div>

    <!-- Navigation. Admin: the active section's rows. Customer and simple-mode
         admin: a flat list. Real landmark + aria-current on the active row. -->
    <nav ref="sidebarNavRef" class="rail__nav" :aria-label="t('shell.mainNavigation')">
      <template v-if="isAdmin && isSimpleMode">
        <div class="rail__group">
          <NavRowLink
            v-for="row in adminFlatSimple"
            :key="row.path"
            :row="row"
            :active="isActive(row.path)"
            @click="handleMenuItemClick(row.path)"
          />
        </div>
      </template>

      <template v-else-if="isAdmin">
        <div class="rail__group">
          <NavRowLink
            v-for="row in activeSectionDef?.rows ?? []"
            :key="row.path"
            :row="row"
            :active="isActive(row.path)"
            @click="handleMenuItemClick(row.path)"
          />
        </div>

        <!-- The admin's own account: same reduced shape as the customer nav,
             always visible regardless of which of the six sections is active. -->
        <div v-if="personalGroups.length" class="rail__personal">
          <div class="rail__personal-heading">{{ t('shell.myAccount') }}</div>
          <div v-for="group in personalGroups" :key="group.key" class="rail__group">
            <div v-if="group.heading" class="rail__group-heading">{{ group.heading }}</div>
            <NavRowLink
              v-for="row in group.rows"
              :key="row.path"
              :row="row"
              :active="isActive(row.path)"
              @click="handleMenuItemClick(row.path)"
            />
          </div>
        </div>
      </template>

      <template v-else>
        <div v-for="group in customerGroups" :key="group.key" class="rail__group">
          <div v-if="group.heading" class="rail__group-heading">{{ group.heading }}</div>
          <NavRowLink
            v-for="row in group.rows"
            :key="row.path"
            :row="row"
            :active="isActive(row.path)"
            @click="handleMenuItemClick(row.path)"
          />
        </div>
      </template>
    </nav>

    <!-- Footer: everything AppHeader used to carry. Notifications and the
         language/theme gear sit beside the identity row, which opens the
         avatar menu. See report for what each header item's new home is. -->
    <div class="rail__footer">
      <div class="rail__foot-utilities">
        <div class="rail__util-slot">
          <AnnouncementBell v-if="authStore.user" />
        </div>

        <button
          ref="prefsTriggerRef"
          type="button"
          class="rail__icon-btn"
          :aria-label="t('shell.preferences')"
          aria-haspopup="menu"
          :aria-expanded="openPopover === 'prefs'"
          @click="togglePopover('prefs')"
          @keydown="onTriggerKeydown('prefs', $event)"
        >
          <i class="hgi-stroke hgi-settings-01" aria-hidden="true" />
        </button>

        <Teleport to="body">
          <div
            v-if="openPopover === 'prefs'"
            ref="prefsPanelRef"
            class="rail__popover rail__popover--prefs"
            :style="popoverStyle"
            role="menu"
            :aria-label="t('shell.preferences')"
            @keydown="onPanelKeydown('prefs', $event)"
          >
            <div class="rail__pref-row" role="none">
              <span class="rail__pref-row-label">{{ t('shell.language') }}</span>
              <LocaleSwitcher />
            </div>
            <button
              type="button"
              role="menuitem"
              class="rail__popover-row"
              @click="toggleTheme"
            >
              <i class="hgi-stroke" :class="isDark ? 'hgi-sun-03' : 'hgi-moon-02'" aria-hidden="true" />
              <span class="rail__popover-row-label">{{ isDark ? t('shell.lightMode') : t('shell.darkMode') }}</span>
            </button>
          </div>
        </Teleport>
      </div>

      <button
        v-if="authStore.user"
        ref="avatarTriggerRef"
        type="button"
        class="rail__identity"
        aria-haspopup="menu"
        :aria-expanded="openPopover === 'avatar'"
        :aria-label="t('shell.accountMenu')"
        @click="togglePopover('avatar')"
        @keydown="onTriggerKeydown('avatar', $event)"
      >
        <span class="rail__avatar">
          <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="rail__avatar-img" />
          <span v-else aria-hidden="true">{{ userInitials }}</span>
        </span>
        <span class="rail__identity-text">
          <span class="rail__identity-name">{{ displayName }}</span>
          <span class="rail__identity-role">{{ roleLabel }}</span>
        </span>
      </button>

      <Teleport to="body">
        <div
          v-if="openPopover === 'avatar'"
          ref="avatarPanelRef"
          class="rail__popover"
          :style="popoverStyle"
          role="menu"
          :aria-label="t('shell.accountMenu')"
          @keydown="onPanelKeydown('avatar', $event)"
        >
          <router-link to="/profile" role="menuitem" class="rail__popover-row" @click="closePopovers">
            <i class="hgi-stroke hgi-user" aria-hidden="true" />
            <span class="rail__popover-row-label">{{ t('nav.profile') }}</span>
          </router-link>
          <router-link
            v-if="isAdmin"
            to="/admin/settings"
            role="menuitem"
            class="rail__popover-row"
            @click="closePopovers"
          >
            <i class="hgi-stroke hgi-settings-01" aria-hidden="true" />
            <span class="rail__popover-row-label">{{ t('nav.settings') }}</span>
          </router-link>
          <button type="button" role="menuitem" class="rail__popover-row rail__popover-row--rule" @click="handleLogout">
            <i class="hgi-stroke hgi-logout-01" aria-hidden="true" />
            <span class="rail__popover-row-label">{{ t('nav.logout') }}</span>
          </button>
        </div>
      </Teleport>
    </div>
  </aside>

  <!-- Mobile overlay, unchanged mechanism from the previous shell: the header's
       hamburger is gone, so AppLayout's corner button drives the same
       appStore.mobileOpen state. -->
  <transition name="rail-fade">
    <div v-if="mobileOpen" class="rail__scrim" @click="closeMobile"></div>
  </transition>
</template>

<script setup lang="ts">
/**
 * AppSidebar -- part 07 v2, sections 03 and 04.
 *
 * One component, two configurations (part 07 v2 "shellCompare"): admin gets a
 * six-section switcher over ~23 destinations; the customer shell omits the
 * switcher and renders a flat 7-row, 3-label nav. `isAdmin` picks the branch.
 *
 * The onboarding tour drives three sidebar selectors directly:
 * `#sidebar-group-manage`, `#sidebar-channel-manage` (composables/useOnboardingTour.ts
 * via components/Guide/steps.ts) and `[data-tour="sidebar-my-keys"]`. All three
 * are preserved verbatim below. The first two live inside the Accounts section,
 * which is why a fresh install defaults the switcher to Accounts rather than
 * Overview -- see the comment on `activeSection`.
 */
import { computed, nextTick, onBeforeUnmount, onMounted, onUnmounted, ref, watch, h, defineComponent, type PropType } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import { sanitizeSvg } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import AnnouncementBell from '@/components/common/AnnouncementBell.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

interface NavRow {
  path: string
  label: string
  icon: string
  iconSvg?: string
  id?: string
  dataTour?: string
}
interface NavGroup {
  key: string
  heading?: string
  rows: NavRow[]
}
interface SectionDef {
  key: SectionKey
  label: string
  icon: string
  rows: NavRow[]
}
type SectionKey = 'overview' | 'accounts' | 'traffic' | 'billing' | 'people' | 'system'
const SECTION_KEYS: SectionKey[] = ['overview', 'accounts', 'traffic', 'billing', 'people', 'system']

/** One row, shared by every nav list. A tiny local component rather than a
 *  template #slot repeated five times, so the tour attributes and
 *  aria-current live in exactly one place. */
const NavRowLink = defineComponent({
  name: 'NavRowLink',
  props: {
    row: { type: Object as PropType<NavRow>, required: true },
    active: { type: Boolean, default: false }
  },
  emits: ['click'],
  setup(props, { emit }) {
    return () =>
      h(
        RouterLink,
        {
          to: props.row.path,
          class: 'rail__row',
          'data-active': props.active || undefined,
          'aria-current': props.active ? 'page' : undefined,
          id: props.row.id,
          'data-tour': props.row.dataTour,
          onClick: () => emit('click')
        },
        () => [
          props.row.iconSvg
            ? h('span', {
                class: 'rail__row-icon rail__row-icon--svg sidebar-svg-icon',
                innerHTML: sanitizeSvg(props.row.iconSvg)
              })
            : h('i', { class: ['hgi-stroke', props.row.icon, 'rail__row-icon'], 'aria-hidden': 'true' }),
          h('span', { class: 'rail__row-label' }, props.row.label)
        ]
      )
  }
})

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const isAdmin = computed(() => authStore.isAdmin)
const isSimpleMode = computed(() => authStore.isSimpleMode)
const sidebarNavRef = ref<HTMLElement | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))

const homePath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const markInitial = computed(() => (siteName.value || 'S').trim().charAt(0).toUpperCase() || 'S')

const user = computed(() => authStore.user)
const avatarUrl = computed(() => user.value?.avatar_url?.trim() || '')
const displayName = computed(() => user.value?.username || user.value?.email?.split('@')[0] || '')
const userInitials = computed(() => {
  const name = user.value?.username || user.value?.email?.split('@')[0] || ''
  return name.slice(0, 2).toUpperCase()
})
const roleLabel = computed(() => (isAdmin.value ? t('shell.roleAdministrator') : t('shell.roleMember')))

// ---- feature flags -------------------------------------------------------
const flagOpsMonitoring = () => adminSettingsStore.opsMonitoringEnabled
const flagAdminPayment = () => adminSettingsStore.paymentEnabled

// ---- custom menu items ----------------------------------------------------
const customMenuItemsForAdmin = computed(() =>
  adminSettingsStore.customMenuItems.filter((item) => item.visibility === 'admin').sort((a, b) => a.sort_order - b.sort_order)
)
const customMenuItemsForUser = computed(() => {
  const items = appStore.cachedPublicSettings?.custom_menu_items ?? []
  return items.filter((item) => item.visibility === 'user').sort((a, b) => a.sort_order - b.sort_order)
})

// ---- self / personal nav ---------------------------------------------------
// Mirrors buildSelfNavItems from the previous shell, reduced from eleven rows
// to six per part 07 v2 section 04: profile moves to the avatar menu, key
// usage becomes a child of a key row, channel status folds into the channels
// table, redeem becomes a Billing action, orders becomes Billing's lower
// half. Three named runs -- Access, Usage, Billing -- exactly as picked over
// the two-label fallback (see report, "groupOptions").
function buildSelfGroups(): NavGroup[] {
  const access: NavRow[] = [
    { path: '/keys', label: t('shell.apiKeys'), icon: 'hgi-key-01', dataTour: 'sidebar-my-keys' }
  ]
  if (!isSimpleMode.value && isFeatureFlagEnabled(FeatureFlags.availableChannels)) {
    access.push({ path: '/available-channels', label: t('nav.channels'), icon: 'hgi-plug-socket' })
  }
  if (isFeatureFlagEnabled(FeatureFlags.modelPlaza)) {
    access.push({ path: '/model-plaza', label: t('shell.modelPlaza'), icon: 'hgi-store-01' })
  }

  const usage: NavRow[] = isSimpleMode.value
    ? []
    : [{ path: '/usage', label: t('nav.usage'), icon: 'hgi-dashboard-speed-01' }]

  const billing: NavRow[] = []
  if (!isSimpleMode.value) {
    billing.push({ path: '/subscriptions', label: t('shell.billing'), icon: 'hgi-invoice-01' })
    if (isFeatureFlagEnabled(FeatureFlags.affiliate)) {
      billing.push({ path: '/affiliate', label: t('shell.referrals'), icon: 'hgi-share-08' })
    }
  }

  for (const item of customMenuItemsForUser.value) {
    access.push({ path: `/custom/${item.id}`, label: item.label, icon: '', iconSvg: item.icon_svg })
  }

  return [
    { key: 'access', heading: t('shell.accessGroup'), rows: access },
    { key: 'usage', heading: t('nav.usage'), rows: usage },
    { key: 'billing', heading: t('shell.billing'), rows: billing }
  ].filter((g) => g.rows.length > 0)
}

const customerGroups = computed((): NavGroup[] => {
  const dashboardRow: NavGroup = { key: 'dashboard', rows: [{ path: '/dashboard', label: t('nav.dashboard'), icon: 'hgi-dashboard-square-01' }] }
  return [dashboardRow, ...buildSelfGroups()]
})

// Admin's own account, same shape as the customer nav, always visible below
// the active section regardless of the switcher -- standard mode only, same
// gate the previous shell used for its "My account" block.
const personalGroups = computed((): NavGroup[] => (isSimpleMode.value ? [] : buildSelfGroups()))

// ---- admin: six sections ----------------------------------------------------
// Grouping is a judgment call, not a product decision (03-OPEN-CALLS.md #1):
// only "Channels under Accounts" and "Risk control under System" were given.
// The rest is documented in the report for the product owner to re-sort.
function buildAdminSections(): SectionDef[] {
  const overview: NavRow[] = [{ path: '/admin/dashboard', label: t('nav.dashboard'), icon: 'hgi-dashboard-square-01' }]
  if (flagOpsMonitoring()) overview.push({ path: '/admin/ops', label: t('nav.ops'), icon: 'hgi-pulse-01' })

  const accounts: NavRow[] = [
    { path: '/admin/accounts', label: t('nav.accounts'), icon: 'hgi-globe-02', id: 'sidebar-channel-manage' },
    { path: '/admin/groups', label: t('nav.groups'), icon: 'hgi-folder-01', id: 'sidebar-group-manage' },
    { path: '/admin/channels/pricing', label: t('nav.channels'), icon: 'hgi-plug-socket' }
  ]
  if (isFeatureFlagEnabled(FeatureFlags.channelMonitor)) {
    accounts.push({ path: '/admin/channels/monitor', label: t('shell.monitor'), icon: 'hgi-activity-01' })
  }
  accounts.push({ path: '/admin/proxies', label: t('nav.proxies'), icon: 'hgi-internet' })

  const traffic: NavRow[] = [
    { path: '/admin/usage', label: t('nav.usage'), icon: 'hgi-dashboard-speed-01' },
    { path: '/admin/audit-logs', label: t('shell.auditLogs'), icon: 'hgi-audit-01' },
    { path: '/admin/prompt-audit', label: t('shell.promptAudit'), icon: 'hgi-message-programming' }
  ]

  const billing: NavRow[] = [
    { path: '/admin/subscriptions', label: t('nav.subscriptions'), icon: 'hgi-credit-card' },
    { path: '/admin/redeem', label: t('shell.redeemCodes'), icon: 'hgi-gift', id: 'sidebar-wallet' },
    { path: '/admin/promo-codes', label: t('shell.promoCodes'), icon: 'hgi-tag-01' }
  ]
  if (flagAdminPayment()) {
    billing.push(
      { path: '/admin/orders/dashboard', label: t('shell.paymentDashboard'), icon: 'hgi-chart-increase' },
      { path: '/admin/orders', label: t('nav.orderManagement'), icon: 'hgi-receipt-text' },
      { path: '/admin/orders/plans', label: t('nav.paymentPlans'), icon: 'hgi-package-01' }
    )
  }

  const people: NavRow[] = [
    { path: '/admin/users', label: t('nav.users'), icon: 'hgi-user-multiple-02' },
    { path: '/admin/announcements', label: t('nav.announcements'), icon: 'hgi-notification-01' }
  ]
  if (isFeatureFlagEnabled(FeatureFlags.affiliate)) {
    people.push(
      { path: '/admin/affiliates/invites', label: t('shell.affiliateInvites'), icon: 'hgi-user-add-01' },
      { path: '/admin/affiliates/rebates', label: t('shell.affiliateRebates'), icon: 'hgi-money-receive-01' },
      { path: '/admin/affiliates/transfers', label: t('shell.affiliateTransfers'), icon: 'hgi-exchange-01' }
    )
  }

  const system: NavRow[] = []
  if (isFeatureFlagEnabled(FeatureFlags.riskControl)) {
    system.push({ path: '/admin/risk-control', label: t('shell.riskControl'), icon: 'hgi-shield-01' })
  }
  system.push({ path: '/admin/settings', label: t('nav.settings'), icon: 'hgi-settings-01' })
  for (const cm of customMenuItemsForAdmin.value) {
    system.push({ path: `/custom/${cm.id}`, label: cm.label, icon: '', iconSvg: cm.icon_svg })
  }

  return [
    { key: 'overview', label: t('shell.overview'), icon: 'hgi-dashboard-square-01', rows: overview },
    { key: 'accounts', label: t('nav.accounts'), icon: 'hgi-globe-02', rows: accounts },
    { key: 'traffic', label: t('shell.traffic'), icon: 'hgi-activity-01', rows: traffic },
    { key: 'billing', label: t('shell.billing'), icon: 'hgi-invoice-01', rows: billing },
    { key: 'people', label: t('shell.people'), icon: 'hgi-user-multiple-02', rows: people },
    { key: 'system', label: t('shell.system'), icon: 'hgi-preference-horizontal', rows: system }
  ]
}

const adminSections = computed((): SectionDef[] => (isAdmin.value && !isSimpleMode.value ? buildAdminSections() : []))

// Simple-mode admin: the same flat, reduced list the previous shell built
// (dashboard, ops, accounts, announcements, proxies, usage, then keys and
// settings appended) -- unchanged, just restyled. No switcher: the filtered
// count is short enough that grouping would cost a click for nothing.
const adminFlatSimple = computed((): NavRow[] => {
  if (!(isAdmin.value && isSimpleMode.value)) return []
  const rows: NavRow[] = [{ path: '/admin/dashboard', label: t('nav.dashboard'), icon: 'hgi-dashboard-square-01' }]
  if (flagOpsMonitoring()) rows.push({ path: '/admin/ops', label: t('nav.ops'), icon: 'hgi-pulse-01' })
  rows.push(
    { path: '/admin/accounts', label: t('nav.accounts'), icon: 'hgi-globe-02', id: 'sidebar-channel-manage' },
    { path: '/admin/announcements', label: t('nav.announcements'), icon: 'hgi-notification-01' },
    { path: '/admin/proxies', label: t('nav.proxies'), icon: 'hgi-internet' },
    { path: '/admin/usage', label: t('nav.usage'), icon: 'hgi-dashboard-speed-01' },
    { path: '/keys', label: t('shell.apiKeys'), icon: 'hgi-key-01', dataTour: 'sidebar-my-keys' },
    { path: '/admin/settings', label: t('nav.settings'), icon: 'hgi-settings-01' }
  )
  for (const cm of customMenuItemsForAdmin.value) {
    rows.push({ path: `/custom/${cm.id}`, label: cm.label, icon: '', iconSvg: cm.icon_svg })
  }
  return rows
})

// ---- active section --------------------------------------------------------
const ACTIVE_SECTION_STORAGE_KEY = 'inferno_admin_active_section'
const ADMIN_SECTION_BY_PATH: Array<[string, SectionKey]> = [
  ['/admin/dashboard', 'overview'],
  ['/admin/ops', 'overview'],
  ['/admin/accounts', 'accounts'],
  ['/admin/groups', 'accounts'],
  ['/admin/channels', 'accounts'],
  ['/admin/proxies', 'accounts'],
  ['/admin/usage', 'traffic'],
  ['/admin/audit-logs', 'traffic'],
  ['/admin/prompt-audit', 'traffic'],
  ['/admin/subscriptions', 'billing'],
  ['/admin/redeem', 'billing'],
  ['/admin/promo-codes', 'billing'],
  ['/admin/orders', 'billing'],
  ['/admin/users', 'people'],
  ['/admin/announcements', 'people'],
  ['/admin/affiliates', 'people'],
  ['/admin/risk-control', 'system'],
  ['/admin/settings', 'system']
]

function sectionForRoute(path: string): SectionKey | null {
  const hit = ADMIN_SECTION_BY_PATH.filter(([prefix]) => path === prefix || path.startsWith(prefix + '/')).sort(
    (a, b) => b[0].length - a[0].length
  )[0]
  return hit ? hit[1] : null
}

function loadPersistedSection(): SectionKey | null {
  try {
    const v = localStorage.getItem(ACTIVE_SECTION_STORAGE_KEY)
    return v && (SECTION_KEYS as string[]).includes(v) ? (v as SectionKey) : null
  } catch {
    return null
  }
}

function persistSection(key: SectionKey) {
  try {
    localStorage.setItem(ACTIVE_SECTION_STORAGE_KEY, key)
  } catch {
    // localStorage unavailable (private browsing, quota) -- the switcher still
    // works for the session, it just does not survive a reload.
  }
}

// Cold start (no persisted preference) defaults to Accounts, not Overview or
// the current route's section. Two reasons, and both have to hold together:
//   1. It is the section the prototype's own mockup shows pre-selected
//      (part 07 v2 section 03, the `sections` array).
//   2. The admin onboarding tour's first two interactive steps target
//      `#sidebar-group-manage` and `#sidebar-channel-manage`, both inside
//      Accounts, and they fire before any navigation happens -- a fresh
//      admin's landing route (Overview) would otherwise hide both targets
//      and silently break the tour, the exact failure this task was warned
//      about. Once an admin picks a section it persists, and route changes
//      keep the switcher honest from then on (see the watcher below) --
///     this default only governs the very first paint.
const activeSection = ref<SectionKey>(loadPersistedSection() ?? 'accounts')
const activeSectionDef = computed(() => adminSections.value.find((s) => s.key === activeSection.value) ?? adminSections.value[0])

function selectSection(key: SectionKey) {
  activeSection.value = key
  persistSection(key)
  closePopovers()
}

// After the first paint, follow navigation: if the admin lands on a route
// belonging to a different section (a bookmark, the tour's own clicks, a
// deep link), the switcher snaps to match. `watch` without `immediate` never
// fires on mount, so the tour-safe default above is never fought on load.
watch(
  () => route.path,
  (path) => {
    if (!isAdmin.value) return
    const match = sectionForRoute(path)
    if (match && match !== activeSection.value) {
      activeSection.value = match
      persistSection(match)
    }
  }
)

// ---- popovers: switcher, preferences (language + theme), avatar menu ------
type PopoverName = 'switcher' | 'prefs' | 'avatar' | null
const openPopover = ref<PopoverName>(null)
const switcherTriggerRef = ref<HTMLElement | null>(null)
const switcherPanelRef = ref<HTMLElement | null>(null)
const prefsTriggerRef = ref<HTMLElement | null>(null)
const prefsPanelRef = ref<HTMLElement | null>(null)
const avatarTriggerRef = ref<HTMLElement | null>(null)
const avatarPanelRef = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({})

function refsFor(name: PopoverName): [HTMLElement | null, HTMLElement | null] {
  if (name === 'switcher') return [switcherTriggerRef.value, switcherPanelRef.value]
  if (name === 'prefs') return [prefsTriggerRef.value, prefsPanelRef.value]
  if (name === 'avatar') return [avatarTriggerRef.value, avatarPanelRef.value]
  return [null, null]
}

function closePopovers() {
  openPopover.value = null
}

function positionPopover(trigger: HTMLElement) {
  const rect = trigger.getBoundingClientRect()
  const viewportBottom = window.innerHeight - 8
  const opensUp = rect.bottom > viewportBottom - 200 && rect.top > 200
  popoverStyle.value = {
    position: 'fixed',
    left: `${Math.max(8, rect.left)}px`,
    minWidth: `${Math.max(180, rect.width)}px`,
    zIndex: '100000020',
    ...(opensUp ? { bottom: `${window.innerHeight - rect.top + 6}px` } : { top: `${rect.bottom + 6}px` })
  }
}

function togglePopover(name: Exclude<PopoverName, null>) {
  if (openPopover.value === name) {
    closePopovers()
    return
  }
  const [trigger] = refsFor(name)
  if (trigger) positionPopover(trigger)
  openPopover.value = name
}

function onTriggerKeydown(name: Exclude<PopoverName, null>, e: KeyboardEvent) {
  if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (openPopover.value !== name) togglePopover(name)
  }
}

function focusableRowsIn(panel: HTMLElement | null): HTMLElement[] {
  if (!panel) return []
  return Array.from(panel.querySelectorAll<HTMLElement>('[role="option"], [role="menuitem"], button, a'))
}

function onPanelKeydown(name: Exclude<PopoverName, null>, e: KeyboardEvent) {
  const [, panel] = refsFor(name)
  const items = focusableRowsIn(panel)
  if (!items.length) return
  const currentIndex = items.indexOf(document.activeElement as HTMLElement)
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    items[(currentIndex + 1 + items.length) % items.length]?.focus()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    items[(currentIndex - 1 + items.length) % items.length]?.focus()
  }
}

function handleDocClick(e: MouseEvent) {
  if (!openPopover.value) return
  const target = e.target as Node
  const [trigger, panel] = refsFor(openPopover.value)
  if (trigger?.contains(target) || panel?.contains(target)) return
  closePopovers()
}

function handleDocKeydown(e: KeyboardEvent) {
  if (!openPopover.value || e.key !== 'Escape') return
  const name = openPopover.value
  const [trigger] = refsFor(name)
  closePopovers()
  trigger?.focus()
}

watch(openPopover, async (name) => {
  if (!name) return
  await nextTick()
  const [, panel] = refsFor(name)
  focusableRowsIn(panel)[0]?.focus()
})

// ---- theme, mobile, tour, scroll restore (unchanged mechanics) -----------
function toggleSidebar() {
  appStore.toggleSidebar()
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

async function handleLogout() {
  closePopovers()
  try {
    await authStore.logout()
  } catch (error) {
    console.error('Logout error:', error)
  }
  await router.push('/login')
}

function handleMenuItemClick(itemPath: string) {
  closePopovers()
  if (mobileOpen.value) {
    setTimeout(() => appStore.setMobileOpen(false), 150)
  }
  const pathToSelector: Record<string, string> = {
    '/admin/groups': '#sidebar-group-manage',
    '/admin/accounts': '#sidebar-channel-manage',
    '/keys': '[data-tour="sidebar-my-keys"]'
  }
  const selector = pathToSelector[itemPath]
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
  isDark.value = true
  document.documentElement.classList.add('dark')
}

watch(
  isAdmin,
  (v) => {
    if (v) adminSettingsStore.fetch()
  },
  { immediate: true }
)

onMounted(() => {
  if (isAdmin.value) adminSettingsStore.fetch()
  document.addEventListener('click', handleDocClick)
  document.addEventListener('keydown', handleDocKeydown)
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
    })
  }
})

onBeforeUnmount(() => {
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})

onUnmounted(() => {
  document.removeEventListener('click', handleDocClick)
  document.removeEventListener('keydown', handleDocKeydown)
})
</script>

<style scoped>
/* --- rail shell ------------------------------------------------------- */
.rail {
  display: flex;
  flex-direction: column;
  width: var(--sidebar-w);
  height: 100vh;
  position: sticky;
  top: 0;
  align-self: start;
  padding: 10px 8px;
  gap: 8px;
  overflow: hidden;
  /* Layout change, not a hover: the width collapse uses --motion-layout. */
  transition: width var(--motion-layout);
}
.rail--collapsed {
  width: var(--sidebar-w-collapsed);
}

/* --- brand -------------------------------------------------------------- */
.rail__brand {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-shrink: 0;
  padding: 2px 4px;
}
.rail__mark {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  border-radius: var(--r-sm);
  background: var(--brand);
  color: var(--on-solid);
  font-size: 10px; /* june-lint-disable ground-rule-4: icon-scale mark, not text */
  font-weight: var(--fw-medium);
  overflow: hidden;
}
.rail__mark-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.rail__brand-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}
.rail--collapsed .rail__brand-name {
  display: none;
}
.rail--collapsed .rail__collapse {
  display: none;
}

/* --- shared icon button, popover row, group heading ---------------------- */
.rail__icon-btn {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.rail__icon-btn i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
}
.rail__icon-btn:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}
.rail__icon-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.rail__group-heading {
  padding: 8px 8px 3px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}
.rail--collapsed .rail__group-heading,
.rail--collapsed .rail__personal-heading {
  display: none;
}

/* --- switcher ------------------------------------------------------------ */
.rail__switcher {
  flex-shrink: 0;
}
.rail__switcher-trigger {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.rail__switcher-trigger:hover {
  background: var(--sidebar-accent);
}
.rail__switcher-trigger:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.rail__switcher-trigger i:first-child {
  flex-shrink: 0;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}
.rail__switcher-trigger i:last-child {
  flex-shrink: 0;
  margin-left: auto;
  font-size: 11px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}
.rail__switcher-label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: left;
}
.rail--collapsed .rail__switcher-trigger {
  padding: 0;
  justify-content: center;
}
.rail--collapsed .rail__switcher-label,
.rail--collapsed .rail__switcher-trigger i:last-child {
  display: none;
}

/* --- nav rows -------------------------------------------------------- */
.rail__nav {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.rail__group {
  display: grid;
  gap: 1px;
}
.rail__personal {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}
.rail__personal-heading {
  padding: 0 8px 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

:deep(.rail__row) {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
  min-height: 30px;
  padding: 0 8px;
  border-radius: var(--r-md);
  color: var(--body-copy);
  font-size: var(--fs-md);
  text-decoration: none;
  transition: background var(--motion-hover);
}
:deep(.rail__row:hover) {
  background: var(--sidebar-accent);
  color: var(--foreground);
}
:deep(.rail__row[data-active]) {
  background: var(--sidebar-accent);
  color: var(--foreground);
  font-weight: var(--fw-medium);
}
:deep(.rail__row:focus-visible) {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
:deep(.rail__row-icon) {
  flex-shrink: 0;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}
:deep(.rail__row[data-active] .rail__row-icon) {
  color: var(--foreground);
}
:deep(.rail__row-label) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rail--collapsed :deep(.rail__row) {
  grid-template-columns: 18px;
  padding: 0 8px;
}
.rail--collapsed :deep(.rail__row-label) {
  display: none;
}

/* Custom uploaded SVG icon: sized without repainting the uploader's own fill
   or stroke. Kept as its own rule (not folded into .rail__row-icon) so an
   admin-supplied brand mark is never recoloured by the row hover state. */
:deep(.sidebar-svg-icon) {
  display: block;
  color: currentColor;
}
:deep(.sidebar-svg-icon svg) {
  display: block;
  width: 15px;
  height: 15px;
}

/* --- footer ---------------------------------------------------------- */
.rail__footer {
  flex-shrink: 0;
  padding-top: 7px;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.rail__foot-utilities {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 2px 2px;
}
.rail__util-slot {
  display: grid;
  place-items: center;
}
.rail__identity {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 5px 6px;
  border: 0;
  border-radius: var(--r-md);
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition: background var(--motion-hover);
}
.rail__identity:hover {
  background: var(--sidebar-accent);
}
.rail__identity:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.rail__avatar {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--muted);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  overflow: hidden;
}
.rail__avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.rail__identity-text {
  min-width: 0;
  flex: 1 1 auto;
}
.rail__identity-name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--foreground);
  font-size: var(--fs-sm);
}
.rail__identity-role {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}
.rail--collapsed .rail__identity-text {
  display: none;
}

/* --- popovers, teleported to body (Select.vue's established pattern: a
   popover anchored off the trigger's own rect can never be clipped by an
   ancestor's overflow, which is what the old version-badge dropdown bug was
   -- see AppSidebar.spec.ts). --- */
.rail__popover {
  min-width: 200px;
  max-width: 280px;
  padding: 5px;
  border: 1px solid var(--popover-border);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-md);
  display: grid;
  gap: 1px;
}
.rail__popover-row {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-height: 30px;
  width: 100%;
  padding: 0 8px;
  border: 0;
  border-top: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--foreground);
  font-size: var(--fs-md);
  text-align: left;
  text-decoration: none;
  cursor: pointer;
  transition: background var(--motion-hover);
}
.rail__popover-row--rule {
  margin-top: 3px;
  padding-top: 3px;
  border-top: 1px solid var(--border-subtle);
  border-radius: 0 0 var(--r-sm) var(--r-sm);
}
.rail__popover-row:hover,
.rail__popover-row:focus-visible,
.rail__popover-row[data-active] {
  background: var(--sidebar-accent);
}
.rail__popover-row:focus-visible {
  outline: none;
}
.rail__popover-row i {
  flex-shrink: 0;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}
.rail__popover-row-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rail__popover-row-count {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}
.rail__popover--prefs {
  min-width: 220px;
}
.rail__pref-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-height: 30px;
  padding: 0 8px;
  border-radius: var(--r-sm);
  color: var(--foreground);
  font-size: var(--fs-md);
}
.rail__pref-row-label {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- mobile: the rail becomes an overlay below the shell's own breakpoint,
   see AppLayout.vue's .shell for the matching column collapse. --- */
@media (max-width: 1023px) {
  .rail {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 40;
    width: var(--sidebar-w);
    box-shadow: var(--shadow-lg);
    transform: translateX(-100%);
  }
  .rail--mobile-open {
    transform: translateX(0);
  }
}
.rail__scrim {
  position: fixed;
  inset: 0;
  z-index: 39;
  background: var(--scrim);
}
@media (min-width: 1024px) {
  .rail__scrim {
    display: none;
  }
}
.rail-fade-enter-active,
.rail-fade-leave-active {
  transition: opacity var(--motion-hover);
}
.rail-fade-enter-from,
.rail-fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .rail,
  .rail--mobile-open,
  .rail-fade-enter-active,
  .rail-fade-leave-active {
    transition: none;
  }
}
</style>
