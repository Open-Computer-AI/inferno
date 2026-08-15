<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.template.applyPickerTitle', { name: templateName })"
    @close="$emit('close')"
  >
    <p class="mb-3 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
      {{ t('admin.channelMonitor.template.applyPickerHint') }}
    </p>

    <div v-if="loading" class="py-6 text-center text-sm text-[var(--muted-foreground)]">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="monitors.length === 0" class="py-6 text-center text-sm text-[var(--muted-foreground)]">
      {{ t('admin.channelMonitor.template.applyPickerEmpty') }}
    </div>

    <div v-else>
      <!-- 全选/全不选 -->
      <div class="mb-2 flex items-center gap-3 text-xs">
        <AppButton
          type="button"
          variant="ghost"
          size="xs"
          @click="selectAll"
        >
          {{ t('common.selectAll') }}
        </AppButton>
        <AppButton
          type="button"
          variant="ghost"
          size="xs"
          @click="selectNone"
        >
          {{ t('admin.channelMonitor.template.selectNone') }}
        </AppButton>
        <span class="ml-auto text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
          {{ t('admin.channelMonitor.template.selectedCount', {
            n: selectedIds.length,
            total: monitors.length,
          }) }}
        </span>
      </div>

      <ul class="max-h-80 divide-y divide-[var(--brand-line)] overflow-y-auto rounded-lg border border-[var(--brand-line)] divide-[var(--brand-line)] border-[var(--brand-line)]">
        <li
          v-for="m in monitors"
          :key="m.id"
          class="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-[var(--brand-tint)] dark:hover:bg-[var(--card)]"
          @click="toggle(m.id)"
        >
          <Checkbox
            :model-value="selectedSet.has(m.id)"
            :aria-label="m.name"
            @click.stop
            @update:model-value="toggle(m.id)"
          />
          <span class="font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ m.name }}</span>
          <span class="text-xs text-[var(--muted-foreground)]">{{ m.provider }}</span>
          <span v-if="m.provider === 'openai'" class="text-xs text-[var(--muted-foreground)]">{{ m.api_mode }}</span>
          <span
            v-if="!m.enabled"
            class="ml-auto rounded bg-[var(--brand-tint)] px-1.5 py-0.5 text-xs text-[var(--muted-foreground)] bg-[var(--brand-tint)] text-[var(--muted-foreground)]"
          >
            {{ t('admin.channelMonitor.onlyDisabled').replace(/^仅|^Only /, '') }}
          </span>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <AppButton variant="secondary" @click="$emit('close')">
          {{ t('common.cancel') }}
        </AppButton>
        <AppButton
          variant="solid"
          :disabled="submitting || selectedIds.length === 0"
          :loading="submitting"
          @click="handleApply"
        >
          {{ submitting
            ? t('common.submitting')
            : t('admin.channelMonitor.template.applyPickerConfirm', { n: selectedIds.length }) }}
        </AppButton>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { AssociatedMonitorBrief } from '@/api/admin/channelMonitorTemplate'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AppButton from '@/components/common/Button.vue'
import Checkbox from '@/components/common/Checkbox.vue'

const props = defineProps<{
  show: boolean
  templateId: number | null
  templateName: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'applied', affected: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const monitors = ref<AssociatedMonitorBrief[]>([])
const selectedIds = ref<number[]>([])

const selectedSet = computed(() => new Set(selectedIds.value))

watch(
  () => [props.show, props.templateId] as const,
  ([show, id]) => {
    if (!show || id == null) return
    void fetchMonitors(id)
  },
  { immediate: true },
)

async function fetchMonitors(id: number) {
  loading.value = true
  monitors.value = []
  selectedIds.value = []
  try {
    const { items } = await adminAPI.channelMonitorTemplate.listAssociatedMonitors(id)
    monitors.value = items
    // 默认全选
    selectedIds.value = items.map((m) => m.id)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

function toggle(id: number) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
}

function selectAll() {
  selectedIds.value = monitors.value.map((m) => m.id)
}

function selectNone() {
  selectedIds.value = []
}

async function handleApply() {
  if (props.templateId == null || selectedIds.value.length === 0 || submitting.value) return
  submitting.value = true
  try {
    const { affected } = await adminAPI.channelMonitorTemplate.apply(
      props.templateId,
      [...selectedIds.value],
    )
    appStore.showSuccess(t('admin.channelMonitor.template.applySuccess', { n: affected }))
    emit('applied', affected)
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}
</script>
