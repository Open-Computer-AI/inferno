<template>
  <BaseDialog :show="show" :title="title" width="narrow" @close="handleCancel">
    <div class="cf-body">
      <p class="cf-message">{{ message }}</p>
      <slot></slot>
    </div>

    <template #footer>
      <div class="cf-actions">
        <Button variant="ghost" size="sm" @click="handleCancel">
          {{ cancelText }}
        </Button>
        <Button :variant="danger ? 'danger' : 'solid'" size="sm" @click="handleConfirm">
          {{ confirmText }}
        </Button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * ConfirmDialog — part 05, section 02 "Confirm". Archetype A, 4 modals in the
 * map but roughly 30 call sites in this repo.
 *
 * Presentation-only rewrite: props, emits and the default slot (used by
 * AccountsView's export dialog to add a checkbox below the message) stay
 * exactly as they were. The footer buttons move from raw Tailwind classes to
 * scoped, token-driven styles that match the prototype's mock: ghost cancel,
 * solid action. The action is --destructive when `danger`, --primary
 * otherwise, and nothing else changes between the two, per the prototype's
 * migration note: "Swaps --primary for --destructive on the action and
 * nothing else."
 *
 * Not implemented, and reported rather than added silently:
 *   - `warning`, a new prop in the prototype's table that renders a detail
 *     box ("Three of the twelve are currently serving traffic...") only when
 *     something is in flight or unrecoverable. Adding it is an API change,
 *     and this task's instructions are to report a required API change
 *     rather than make it.
 *   - Removing `confirmText`'s implicit fallback to t('common.confirm') so a
 *     caller is forced to name the verb. The prototype's migration note asks
 *     for this, but it is a behaviour change for every call site that omits
 *     confirmText today, so it stays as a fallback.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'
import Button from './Button.vue'

const { t } = useI18n()

interface Props {
  show: boolean
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

interface Emits {
  (e: 'confirm'): void
  (e: 'cancel'): void
}

const props = withDefaults(defineProps<Props>(), {
  danger: false
})

const confirmText = computed(() => props.confirmText || t('common.confirm'))
const cancelText = computed(() => props.cancelText || t('common.cancel'))

const emit = defineEmits<Emits>()

const handleConfirm = () => {
  emit('confirm')
}

const handleCancel = () => {
  emit('cancel')
}
</script>

<style scoped>
.cf-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.cf-message {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.55;
}

.cf-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  width: 100%;
}

</style>
