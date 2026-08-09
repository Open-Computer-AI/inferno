<template>
  <BaseDialog :show="show" :title="title" width="narrow" @close="handleCancel">
    <div class="cf-body">
      <p class="cf-message">{{ message }}</p>
      <slot></slot>
    </div>

    <template #footer>
      <div class="cf-actions">
        <button type="button" class="cf-btn" data-variant="ghost" @click="handleCancel">
          {{ cancelText }}
        </button>
        <button
          type="button"
          class="cf-btn"
          :data-variant="danger ? 'danger' : 'solid'"
          @click="handleConfirm"
        >
          {{ confirmText }}
        </button>
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

/* Ghost/solid as data attributes, mirroring Button.vue's variant pattern:
   a footer here is two decisions (which action, how urgent), not a class
   per combination. */
.cf-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 32px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--r-md);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: 400;
  white-space: nowrap;
  cursor: pointer;
  /* Background only, never border-color or `all` (ground rule 6). */
  transition: background var(--motion-hover);
}
.cf-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.cf-btn[data-variant='ghost'] {
  background: transparent;
  color: var(--body-copy);
}
.cf-btn[data-variant='ghost']:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

/* Solid, not outlined: unlike a toolbar button, this action already is the
   one thing you can do here, so it can carry the fill. */
.cf-btn[data-variant='solid'],
.cf-btn[data-variant='danger'] {
  padding: 0 14px;
  color: var(--on-solid);
  font-weight: var(--fw-medium);
}
.cf-btn[data-variant='solid'] {
  background: var(--primary);
}
.cf-btn[data-variant='solid']:hover {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.cf-btn[data-variant='danger'] {
  background: var(--destructive);
}
.cf-btn[data-variant='danger']:hover {
  background: color-mix(in oklch, var(--destructive) 92%, var(--foreground));
}
</style>
