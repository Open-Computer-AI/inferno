<template>
  <aside class="settings-sidebar">
    <RouterLink class="settings-sidebar__back" to="/admin/dashboard">
      <Icon name="chevronLeft" size="sm" aria-hidden="true" />
      <span>{{ t('admin.settings.backToApp') }}</span>
    </RouterLink>

    <nav class="settings-sidebar__nav" :aria-label="t('admin.settings.title')">
      <section v-for="group in groups" :key="group.key" class="settings-sidebar__group">
        <h2 class="settings-sidebar__group-title">
          {{ t(`admin.settings.groups.${group.key}`) }}
        </h2>
        <div class="settings-sidebar__items">
          <RouterLink
            v-for="section in group.sections"
            :key="section.key"
            :to="{ name: 'AdminSettings', params: { section: section.key } }"
            class="settings-sidebar__item"
            :class="{ 'settings-sidebar__item--active': activeTab === section.key }"
            :aria-current="activeTab === section.key ? 'page' : undefined"
            @click="selectTabOnClick(section.key)"
          >
            <span class="settings-sidebar__icon">
              <Icon :name="section.icon" size="sm" aria-hidden="true" />
            </span>
            <span class="settings-sidebar__label">{{ t(`admin.settings.tabs.${section.key}`) }}</span>
          </RouterLink>
        </div>
      </section>
    </nav>

    <div class="settings-sidebar__account">
      <AccountPatternAvatar :seed="accountSeed" :size="22" />
      <span class="settings-sidebar__account-copy">
        <strong>{{ displayName }}</strong>
        <small>{{ t('shell.roleAdministrator') }}</small>
      </span>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { getActivePinia } from 'pinia'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores/auth'
import { useAccountSeed } from '@/composables/useAccountSeed'
import AccountPatternAvatar from './AccountPatternAvatar.vue'
import {
  SETTINGS_GROUPS,
  type SettingsSectionKey,
} from './settingsRegistry'

const { activeTab } = defineProps<{
  activeTab: SettingsSectionKey
}>()

const emit = defineEmits<{
  select: [section: SettingsSectionKey]
}>()

const { t } = useI18n()
// SettingsView's legacy unit tests mount the view without Pinia. The real app
// always has Pinia installed, but keeping this leaf presentation component
// optional makes those isolated tests—and Storybook-style mounts—safe.
const authStore = getActivePinia() ? useAuthStore() : null

const groups = computed(() => SETTINGS_GROUPS)

const displayName = computed(() => authStore?.user?.username || authStore?.user?.email?.split('@')[0] || 'admin')
// `authStore?.` is load-bearing, not defensive noise: authStore is genuinely
// null whenever this component is mounted without Pinia (see above).
const accountSeed = useAccountSeed(() => authStore?.user, displayName)

function selectTabOnClick(section: SettingsSectionKey): void {
  emit('select', section)
}

</script>

<style scoped>
.settings-sidebar {
  display: flex;
  flex: 0 0 var(--sidebar-w);
  width: var(--sidebar-w);
  flex-direction: column;
  min-height: 100%;
  padding: 10px 8px;
  gap: 8px;
  background: var(--sidebar);
  color: var(--sidebar-foreground);
}

.settings-sidebar__back {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 30px;
  padding: 0 8px;
  border-radius: var(--r-sm);
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-decoration: none;
  transition: background-color var(--t-fast) var(--ease-out), color var(--t-fast) var(--ease-out);
}

.settings-sidebar__back:hover,
.settings-sidebar__back:focus-visible {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.settings-sidebar__nav {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 10px;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.settings-sidebar__nav::-webkit-scrollbar {
  width: 0;
  height: 0;
  display: none;
}

.settings-sidebar__group {
  display: grid;
  gap: 1px;
}

.settings-sidebar__group-title {
  margin: 0;
  padding: 8px 8px 3px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.settings-sidebar__items {
  display: grid;
  gap: 1px;
}

.settings-sidebar__item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 30px;
  padding: 0 8px;
  border: 0;
  border-radius: var(--r-md);
  background: transparent;
  color: var(--muted-foreground);
  font: inherit;
  font-size: var(--fs-md);
  text-align: left;
  text-decoration: none;
  cursor: pointer;
  transition: background-color var(--t-fast) var(--ease-out), color var(--t-fast) var(--ease-out);
}

.settings-sidebar__item:hover,
.settings-sidebar__item:focus-visible {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.settings-sidebar__item--active {
  background: var(--sidebar-accent);
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.settings-sidebar__icon {
  display: inline-grid;
  flex: 0 0 18px;
  place-items: center;
  color: currentColor;
}

.settings-sidebar__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-sidebar__account {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  padding: 7px 6px 0;
  border-top: 1px solid var(--sidebar-border);
}

.settings-sidebar__account-copy {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.settings-sidebar__account-copy strong,
.settings-sidebar__account-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-sidebar__account-copy strong {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.settings-sidebar__account-copy small {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

@media (max-width: 900px) {
  .settings-sidebar {
    flex-basis: var(--sidebar-w);
  }
}

@media (max-width: 700px) {
  .settings-sidebar {
    min-height: auto;
    width: auto;
    padding: 10px 8px;
  }

  .settings-sidebar__nav {
    display: flex;
    flex-direction: row;
    align-items: flex-start;
    gap: 16px;
    margin-top: 2px;
    overflow-x: auto;
    overflow-y: hidden;
    padding-bottom: 2px;
  }

  .settings-sidebar__group {
    min-width: max-content;
  }

  .settings-sidebar__items {
    display: flex;
  }

  .settings-sidebar__group-title {
    padding-left: 8px;
  }

}
</style>
