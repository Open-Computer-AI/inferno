<template>
  <SettingsGroup
    :title="t('admin.settings.appearance.theme')"
    :description="t('admin.settings.appearance.themeHint')"
  >
    <SettingsRow :label="t('admin.settings.appearance.theme')">
      <Segmented
        v-model="themePreference"
        :items="themeOptions"
        :aria-label="t('admin.settings.appearance.theme')"
        @update:model-value="applyThemePreference"
      />
    </SettingsRow>
  </SettingsGroup>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Segmented from '@/components/common/Segmented.vue'
import SettingsGroup from '@/components/layout/SettingsGroup.vue'
import SettingsRow from '@/components/layout/SettingsRow.vue'

type ThemePreference = 'system' | 'light' | 'dark'

const { t } = useI18n()

function normalizeThemePreference(value: string): ThemePreference {
  return value === 'dark' || value === 'light' || value === 'system' ? value : 'system'
}

const themePreference = ref<ThemePreference>(normalizeThemePreference(localStorage.getItem('theme') || 'system'))
const themeOptions = computed(() => [
  { value: 'system', label: t('admin.settings.appearance.system') },
  { value: 'light', label: t('admin.settings.appearance.light') },
  { value: 'dark', label: t('admin.settings.appearance.dark') },
])

function applyThemePreference(value: string): void {
  const preference = normalizeThemePreference(value)
  themePreference.value = preference
  const shouldUseDark = preference === 'dark' ||
    (preference === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
  if (preference === 'system') localStorage.removeItem('theme')
  else localStorage.setItem('theme', preference)
}
</script>
