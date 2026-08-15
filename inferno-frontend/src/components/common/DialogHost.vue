<template>
  <ConfirmDialog
    v-if="request?.kind === 'confirm'"
    :key="request.id"
    :show="true"
    :title="request.options.title || t('common.confirm')"
    :message="request.options.message"
    :confirm-text="request.options.confirmText || t('common.confirm')"
    :cancel-text="request.options.cancelText || t('common.cancel')"
    :danger="request.options.danger"
    @confirm="settle(true)"
    @cancel="settle(false)"
  />

  <PromptDialog
    v-else-if="request?.kind === 'prompt'"
    :key="request.id"
    :show="true"
    :title="request.options.title || t('common.field-control')"
    :message="request.options.message"
    :default-value="request.options.defaultValue"
    :placeholder="request.options.placeholder"
    :confirm-text="request.options.confirmText || t('common.confirm')"
    :cancel-text="request.options.cancelText || t('common.cancel')"
    :danger="request.options.danger"
    @confirm="settle"
    @cancel="settle(null)"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from './ConfirmDialog.vue'
import PromptDialog from './PromptDialog.vue'
import { useDialogHost } from '@/composables/useDialogService'

const { t } = useI18n()
const { activeRequest, settle } = useDialogHost()
const request = computed(() => activeRequest.value)
</script>
