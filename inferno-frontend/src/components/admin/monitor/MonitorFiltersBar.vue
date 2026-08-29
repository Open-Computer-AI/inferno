<template>
  <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
    <!-- Left: Search + Filters -->
    <div class="flex flex-1 flex-wrap items-center gap-3">
      <div class="relative w-full sm:w-64">
        <Icon
          name="search"
          size="md"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
        />
        <input
          v-model="search"
          type="text"
          :placeholder="t('admin.channelMonitor.searchPlaceholder')"
          class="field-control pl-10"
          @input="$emit('search-input')"
        />
      </div>

      <Select
        v-model="provider"
        :options="providerFilterOptions"
        :placeholder="t('admin.channelMonitor.allProviders')"
        class="w-44"
        @change="$emit('reload')"
      />

      <Select
        v-model="enabled"
        :options="enabledFilterOptions"
        :placeholder="t('admin.channelMonitor.enabledFilter')"
        class="w-40"
        @change="$emit('reload')"
      />
    </div>

    <!-- Right: Actions -->
    <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
      <Button
        :loading="loading"
        :disabled="loading"
        variant="secondary"
        size="sm"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="$emit('reload')"
      >
        <Icon v-if="!loading" name="refresh" size="md" />
      </Button>
      <Button
        variant="secondary"
        size="sm"
        @click="$emit('manage-templates')"
        :title="t('admin.channelMonitor.template.manageButton')"
      >
        <Icon name="cog" size="md" />
        {{ t('admin.channelMonitor.template.manageButton') }}
      </Button>
      <Button variant="solid" size="sm" @click="$emit('create')">
        <Icon name="plus" size="md" />
        {{ t('admin.channelMonitor.createButton') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/api/admin/channelMonitor'
import Select from '@/components/common/Select.vue'
import Button from '@/components/common/Button.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  PROVIDER_GROK,
  PROVIDER_ANTIGRAVITY,
  PROVIDER_KIMI,
  PROVIDER_ZHIPU,
  PROVIDER_DEEPSEEK,
} from '@/constants/channelMonitor'

defineProps<{
  loading: boolean
}>()

defineEmits<{
  (e: 'reload'): void
  (e: 'create'): void
  (e: 'manage-templates'): void
  (e: 'search-input'): void
}>()

const search = defineModel<string>('search', { required: true })
const provider = defineModel<Provider | ''>('provider', { required: true })
const enabled = defineModel<'' | 'true' | 'false'>('enabled', { required: true })

const { t } = useI18n()

const providerFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allProviders') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
  { value: PROVIDER_GROK, label: t('monitorCommon.providers.grok') },
  { value: PROVIDER_ANTIGRAVITY, label: t('monitorCommon.providers.antigravity') },
  { value: PROVIDER_KIMI, label: t('monitorCommon.providers.kimi') },
  { value: PROVIDER_ZHIPU, label: t('monitorCommon.providers.zhipu') },
  { value: PROVIDER_DEEPSEEK, label: t('monitorCommon.providers.deepseek') },
])

const enabledFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allStatus') },
  { value: 'true', label: t('admin.channelMonitor.onlyEnabled') },
  { value: 'false', label: t('admin.channelMonitor.onlyDisabled') },
])
</script>
