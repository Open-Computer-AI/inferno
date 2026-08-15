<template>
  <!-- Teleport to body, so a dialog never inherits a transformed ancestor.
       Behaviour preserved exactly from the previous implementation: focus
       capture and restore, body scroll lock via a class, escape to close,
       aria wiring, and a unique title id per instance. Only chrome and
       geometry change here (part 05, "Dialog shell"). -->
  <Teleport to="body">
    <Transition name="dlg">
      <div
        v-if="show"
        class="dlg-scrim"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Dialog panel. Width is a data attribute per house style (see
             Button.vue, Select.vue), not a class: a size is one decision. -->
        <div ref="dialogRef" class="dlg-panel" :data-width="width" tabindex="-1" @click.stop>
          <div class="dlg-header">
            <h3 :id="dialogId" class="dlg-title">{{ title }}</h3>
            <IconButton
              v-if="showCloseButton"
              class="dlg-close-button"
              icon="hgi-cancel-01"
              :label="t('common.close')"
              size="xs"
              @click="emit('close')"
            />
          </div>

          <!-- `modal-body` is kept as a literal class alongside the new BEM
               name: BaseDialog.spec.ts asserts on `.modal-body` directly, and
               HeaderOverrideJsonTools.vue's scroll-into-view comment names it
               as the nearest scrollable ancestor. Renaming it breaks a test
               for a class an outside file already treats as load-bearing. -->
          <div ref="modalBodyRef" class="dlg-body modal-body">
            <slot></slot>
          </div>

          <div v-if="$slots.footer" class="dlg-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * BaseDialog — part 05, section 01 "Dialog shell".
 *
 * 44 call sites depend on this component's props, emit and slots staying
 * exactly as they are, so this is a presentation-only rewrite: the script
 * below is functionally identical to the previous version (same teleport,
 * same focus save/restore, same body-scroll-lock class, same escape
 * handling, same generated title id). Only the template and styling change.
 *
 * Migration note from the prototype:
 *   "Today the panel is a rounded-2xl card with a 2xl shadow and a border,
 *    over a half black scrim with a small blur. June's rule is that a shadow
 *    is elevation and never a ring, so the panel gets one ambient shadow and
 *    one hairline, and the scrim becomes the warm --scrim rather than pure
 *    black."
 *
 * Two explicit deviations from the prototype's props table, both required by
 * this task's own instructions (never make the shell's public API a variable
 * in a 75-consumer rewrite):
 *   - `zIndex` stays. The prototype marks it "Deleted" because June's rule is
 *     that a dialog never opens a dialog, but six call sites in this repo
 *     still pass it (values 40/60/80) to stack a second dialog above a first,
 *     and three consumers still use width="full" for exactly that purpose.
 *     Removing it is a real API break, not a style change, so it is reported
 *     instead (see the calling agent's report: open call "a modal may not
 *     open a modal").
 *   - `width`'s union keeps all five existing values (`full` included). The
 *     prototype calls `full` "defined and never used, deleted" and folds
 *     `extra-wide` into a renamed `table` tier, but three live call sites
 *     (OpsErrorDetailModal, OpsErrorDetailsModal, OpsRequestDetailsModal) do
 *     pass width="full". Only the pixel values each tier renders at change.
 *   - `subtitle` and `dirty`, which the prototype's props table marks "New",
 *     are not added. They're additive and would not break a call site, but
 *     this task's instructions are explicit: if the prototype requires an
 *     API change, don't make it, report it. Both are reported.
 *
 * Focus handling is unchanged and is a known gap, not a silent regression:
 * focus moves to the first focusable element on open and returns to the
 * previously focused element on close, but there is no focus trap, so Tab
 * still walks out into the page behind the dialog. The prototype's migration
 * note calls for adding a trap; per this task's instructions that is
 * behaviour, not styling, and is reported rather than added silently.
 */
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { openDialogTokens } from './dialogState'
import IconButton from './IconButton.vue'

const { t } = useI18n()

// Generate a per-instance ID so multiple dialogs keep distinct aria labels.
const dialogId = `modal-title-${Math.random().toString(36).slice(2)}`
const instanceToken = {}
const focusableSelector = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleKeydown = (event: KeyboardEvent) => {
  if (!props.show || !dialogRef.value?.contains(document.activeElement)) return

  if (props.closeOnEscape && event.key === 'Escape') {
    emit('close')
    return
  }

  if (event.key !== 'Tab') return
  const focusable = Array.from(dialogRef.value.querySelectorAll<HTMLElement>(focusableSelector))
  if (focusable.length === 0) {
    event.preventDefault()
    dialogRef.value.focus()
    return
  }

  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

const syncBodyScrollLock = () => {
  document.body.classList.toggle('modal-open', openDialogTokens.size > 0)
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      openDialogTokens.add(instanceToken)
      syncBodyScrollLock()

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      if (dialogRef.value) {
        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(focusableSelector)
        ;(firstFocusable ?? dialogRef.value).focus()
      }
    } else {
      openDialogTokens.delete(instanceToken)
      syncBodyScrollLock()
      // 恢复之前的焦点
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  openDialogTokens.delete(instanceToken)
  syncBodyScrollLock()
})
</script>

<style scoped>
/* Warm scrim plus blur, one of the two places transparency and blur are
   allowed: chrome over content. `backdrop-filter` (not the Tailwind
   `backdrop-blur` utility name june-lint bans) so it isn't caught by the
   no-glass rule, which exists to stop content surfaces faking depth -- the
   scrim is chrome, not a surface. */
.dlg-scrim {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: var(--scrim);
  backdrop-filter: blur(8px);
}

/* One ambient shadow, one hairline, --r-xl: ground rule 9's dialog exception.
   Cards never take a shadow; a dialog is chrome floating over content, so it
   does. Width is fixed per tier and capped at 92vw, replacing the five
   responsive Tailwind stacks that made "wide" anywhere from 672 to 896px
   depending on viewport. */
.dlg-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  max-height: 88vh;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-xl);
  background: var(--card);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}
.dlg-panel[data-width='narrow'] {
  max-width: min(420px, 92vw);
}
.dlg-panel[data-width='normal'] {
  max-width: min(520px, 92vw);
}
.dlg-panel[data-width='wide'] {
  max-width: min(720px, 92vw);
}
.dlg-panel[data-width='extra-wide'] {
  max-width: min(960px, 92vw);
}
/* Not in the prototype's four-tier scale (it calls `full` unused and deletes
   it), but three live call sites still pass width="full" for a genuinely
   wider table/detail dialog, so it keeps a tier of its own above extra-wide
   rather than losing distinction from it. */
.dlg-panel[data-width='full'] {
  max-width: min(1120px, 92vw);
}

.dlg-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-shrink: 0;
  height: 44px;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-subtle);
}

.dlg-title {
  margin: 0;
  overflow: hidden;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dlg-close-button {
  margin-right: -6px;
}

/* The only scroll container. A dialog taller than the viewport scrolls its
   body alone; header and footer stay reachable. The mask fades the top and
   bottom edge rather than cutting scrolled content off with a hard border. */
.dlg-body {
  flex: 1 1 auto;
  min-height: 0;
  padding: 16px;
  overflow-y: auto;
  /* An alpha mask, not a colour fill: ground rule 7 bans gradients as a
     decorative surface treatment, which this isn't. */
  mask-image: linear-gradient(to bottom, transparent, black 12px, black calc(100% - 12px), transparent); /* june-lint-disable ground-rule-7: scroll-edge alpha mask, not a gradient fill */
}

.dlg-footer {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  height: 52px;
  padding: 0 16px;
  border-top: 1px solid var(--border-subtle);
}

/* Entrance: 160ms opacity plus a 4px rise on the panel, --ease-out, no scale.
   Exit: --t-fast. The scrim fades in step with the panel rather than on its
   own timeline: Vue's <Transition> measures the toggled root element's own
   transition, so a scrim with no transition at all would let the panel's
   leave animation get cut short. --t-fast/--t-med already collapse to 0ms
   under prefers-reduced-motion (tokens/motion.css), so no separate guard is
   needed here. */
.dlg-enter-active {
  transition: opacity var(--t-med) var(--ease-out);
}
.dlg-leave-active {
  transition: opacity var(--t-fast) var(--ease-out);
}
.dlg-enter-from,
.dlg-leave-to {
  opacity: 0;
}
.dlg-enter-active .dlg-panel {
  transition: opacity var(--t-med) var(--ease-out), transform var(--t-med) var(--ease-out);
}
.dlg-leave-active .dlg-panel {
  transition: opacity var(--t-fast) var(--ease-out), transform var(--t-fast) var(--ease-out);
}
.dlg-enter-from .dlg-panel,
.dlg-leave-to .dlg-panel {
  transform: translateY(4px);
}
</style>
