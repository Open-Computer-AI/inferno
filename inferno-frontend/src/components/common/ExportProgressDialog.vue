<template>
  <BaseDialog :show="show" :title="t('usage.exporting')" width="narrow" @close="handleCancel">
    <div class="space-y-4">
      <div class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
        {{ t('usage.exportingProgress') }}
      </div>
      <div class="flex items-center justify-between text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">
        <span>{{ t('usage.exportedCount', { current, total }) }}</span>
        <span class="font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ normalizedProgress }}%</span>
      </div>
      <div class="h-2 w-full rounded-full bg-[var(--brand-tint)]">
        <div
          role="progressbar"
          :aria-valuenow="normalizedProgress"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-label="`${t('usage.exportingProgress')}: ${normalizedProgress}%`"
          class="h-2 rounded-full bg-[var(--brand)] transition-[background-color,color,opacity]"
          :style="{ width: `${normalizedProgress}%` }"
        ></div>
      </div>
      <div v-if="estimatedTime" class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]" aria-live="polite" aria-atomic="true">
        {{ t('usage.estimatedTime', { time: estimatedTime }) }}
      </div>
    </div>

    <template #footer>
      <Button
        @click="handleCancel"
        variant="secondary"
        size="sm"
      >
        {{ t('usage.cancelExport') }}
      </Button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'
import Button from './Button.vue'

interface Props {
  show: boolean
  progress: number
  current: number
  total: number
  estimatedTime: string
}

interface Emits {
  (e: 'cancel'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()

const normalizedProgress = computed(() => {
  const value = Number.isFinite(props.progress) ? props.progress : 0
  return Math.min(100, Math.max(0, Math.round(value)))
})

const handleCancel = () => {
  emit('cancel')
}
</script>
