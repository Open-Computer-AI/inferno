<template>
  <div class="settings-tabs-shell">
    <nav class="settings-tabs-scroll" role="tablist" :aria-label="t('admin.settings.title')">
      <div class="settings-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          :id="`settings-tab-${tab.key}`"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.key"
          :tabindex="activeTab === tab.key ? 0 : -1"
          :class="['settings-tab', activeTab === tab.key && 'settings-tab-active']"
          @click="selectTab(tab.key)"
          @keydown="handleTabKeydown($event, tab.key)"
        >
          <span class="settings-tab-icon">
            <Icon :name="tab.icon" size="sm" />
          </span>
          <span class="settings-tab-label">{{ t(`admin.settings.tabs.${tab.key}`) }}</span>
        </button>
      </div>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

export type SettingsTabKey =
  | 'general'
  | 'agreement'
  | 'features'
  | 'security'
  | 'users'
  | 'gateway'
  | 'payment'
  | 'email'
  | 'backup'

type SettingsTabIcon =
  | 'home'
  | 'document'
  | 'bolt'
  | 'shield'
  | 'user'
  | 'server'
  | 'creditCard'
  | 'mail'
  | 'database'

export interface SettingsTab {
  key: SettingsTabKey
  icon: SettingsTabIcon
}

const props = defineProps<{
  tabs: SettingsTab[]
  activeTab: SettingsTabKey
}>()

const emit = defineEmits<{
  select: [tab: SettingsTabKey]
}>()

const { t } = useI18n()

const keyboardActions: Record<string, number | 'first' | 'last'> = {
  ArrowLeft: -1,
  ArrowUp: -1,
  ArrowRight: 1,
  ArrowDown: 1,
  Home: 'first',
  End: 'last',
}

function selectTab(tab: SettingsTabKey): void {
  emit('select', tab)
}

function handleTabKeydown(event: KeyboardEvent, tab: SettingsTabKey): void {
  const action = keyboardActions[event.key]
  if (action === undefined) return

  event.preventDefault()
  const currentIndex = props.tabs.findIndex((item) => item.key === tab)
  let nextIndex = currentIndex < 0 ? 0 : currentIndex

  if (action === 'first') nextIndex = 0
  else if (action === 'last') nextIndex = props.tabs.length - 1
  else nextIndex = (nextIndex + action + props.tabs.length) % props.tabs.length

  const nextTab = props.tabs[nextIndex]?.key
  if (!nextTab) return

  selectTab(nextTab)
  window.requestAnimationFrame(() => {
    document.getElementById(`settings-tab-${nextTab}`)?.focus()
  })
}
</script>

<style scoped>
.settings-tabs-shell {
  @apply sticky z-20 -mx-1 p-1.5;
  top: 4.75rem;
  border-bottom: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
}

.settings-tabs-scroll {
  @apply overflow-x-auto;
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.settings-tabs-scroll::-webkit-scrollbar { display: none; }
.settings-tabs { @apply flex min-w-max items-center gap-1; }

.settings-tab {
  @apply relative isolate flex h-10 min-w-[6.75rem] shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-transparent px-3 text-sm text-gray-600 outline-none transition-colors duration-200 ease-out dark:text-gray-300;
  font-weight: var(--fw-medium);
}

@media (min-width: 768px) {
  .settings-tabs { @apply min-w-full; }
  .settings-tab { @apply min-w-0 flex-1 basis-0 overflow-hidden px-2 text-[13px]; }
  .settings-tab-icon { @apply h-6 w-6; }
}

.settings-tab::before {
  @apply absolute inset-0 -z-10 rounded-xl opacity-0 transition-opacity duration-200;
  content: '';
  background: var(--sidebar-accent);
}

.settings-tab:hover::before,
.settings-tab:focus-visible::before { opacity: 1; }
.settings-tab:focus-visible { @apply ring-2 ring-offset-2 ring-offset-white dark:ring-offset-dark-900; box-shadow: 0 0 0 2px color-mix(in oklch, var(--ring) 40%, transparent); }
.settings-tab-active {
  @apply border shadow-sm;
  background: var(--card);
  border-color: color-mix(in oklch, var(--primary) 35%, var(--border));
  color: var(--primary);
  box-shadow:
    0 8px 18px rgb(15 23 42 / 0.08),
    0 1px 0 rgb(255 255 255 / 0.92) inset;
}
.settings-tab-active::before { opacity: 0; }
.settings-tab-active::after { position: absolute; right: 0.75rem; bottom: 0.25rem; left: 0.75rem; height: 2px; border-radius: 9999px; content: ''; background: var(--primary); }
.settings-tab-icon { @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors duration-200 dark:text-gray-400; }
.settings-tab:hover .settings-tab-icon,
.settings-tab:focus-visible .settings-tab-icon { color: var(--foreground); }
.settings-tab-active .settings-tab-icon { background: color-mix(in oklch, var(--primary) 12%, transparent); color: var(--primary); }
.settings-tab-label { @apply min-w-0 overflow-hidden text-ellipsis whitespace-nowrap leading-none; }
</style>

<style>
.dark .settings-tab::before { background: var(--sidebar-accent); }
.dark .settings-tab-active { box-shadow: 0 12px 26px rgb(0 0 0 / 0.22), 0 1px 0 rgb(255 255 255 / 0.08) inset; }
</style>
