<template>
  <div class="flex items-center gap-1">
    <IconButton
      icon="hgi-refresh"
      :label="t('admin.channelMonitor.runNow')"
      @click="$emit('run', row)"
      :disabled="running"
      :loading="running"
    />
    <IconButton
      data-testid="monitor-duplicate"
      icon="hgi-copy-01"
      :label="duplicateTitle"
      :disabled="duplicating || Boolean(row.api_key_decrypt_failed)"
      :loading="duplicating"
      @click="$emit('duplicate', row)"
    />
    <IconButton
      icon="hgi-edit-02"
      :label="t('common.edit')"
      @click="$emit('edit', row)"
    />
    <IconButton
      icon="hgi-delete-01"
      :label="t('common.delete')"
      tone="danger"
      @click="$emit('delete', row)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import IconButton from '@/components/common/IconButton.vue'

const props = defineProps<{
  row: ChannelMonitor
  running: boolean
  duplicating: boolean
}>()

defineEmits<{
  (e: 'run', row: ChannelMonitor): void
  (e: 'duplicate', row: ChannelMonitor): void
  (e: 'edit', row: ChannelMonitor): void
  (e: 'delete', row: ChannelMonitor): void
}>()

const { t } = useI18n()
const duplicateTitle = computed(() => {
  if (props.row.api_key_decrypt_failed) return t('admin.channelMonitor.duplicateKeyUnavailable')
  if (props.duplicating) return t('admin.channelMonitor.duplicating')
  return t('admin.channelMonitor.duplicate')
})
</script>
