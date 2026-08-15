<script setup lang="ts">
/**
 * Button — part 02, section 01.
 *
 * Replaces the `.btn` CSS family in style.css: nine variants and four size
 * classes combined freely, which gave the app fourteen effective heights and
 * five gradients. Same nine variants here, no gradients, one height set.
 *
 * Migration note from the prototype:
 *   "Delete every gradient, every shadow-glow and the active:scale-[0.98].
 *    Replace the four size classes with the height tokens. btn-stripe,
 *    btn-alipay, btn-wxpay and btn-airwallex keep the provider mark but lose
 *    the brand fill."
 *
 * Two rules worth stating because they are easy to undo by accident:
 *
 *   Press does not scale. June's press feedback is the solid darkening by
 *   mixing 8% foreground into --primary. `active:scale-[0.98]` is deleted, not
 *   ported.
 *
 *   Only background transitions. Ground rule 6: borders are static chrome, so
 *   the transition names `background` explicitly rather than using `all`.
 */
withDefaults(
  defineProps<{
    /** Today these are separate CSS classes (.btn-primary, .btn-danger, ...). */
    variant?: 'solid' | 'secondary' | 'ghost' | 'danger' | 'success' | 'warning'
    /** 28 / 32 / 36 / 40. Toolbar (sm) is the common case. */
    size?: 'xs' | 'sm' | 'md' | 'lg'
    /** Leading Hugeicons glyph, e.g. "hgi-add-01". Left side only: a trailing
     *  chevron means the button opens a menu and nothing else. */
    icon?: string
    /** Spinner replaces the leading icon; the label does not change, so the
     *  button keeps its width and the row does not reflow. */
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'secondary', size: 'sm', icon: '', loading: false, disabled: false, type: 'button' }
)

defineEmits<{ click: [MouseEvent] }>()
</script>

<template>
  <button
    :type="type"
    class="btn2"
    :data-variant="variant"
    :data-size="size"
    :disabled="disabled"
    :aria-disabled="disabled || undefined"
    :aria-busy="loading || undefined"
    @click="$emit('click', $event)"
  >
    <span v-if="loading" class="btn2__spinner s2a-spinner" aria-hidden="true" />
    <i v-else-if="icon" :class="['hgi-stroke', icon]" class="btn2__icon" aria-hidden="true" />
    <span class="btn2__label"><slot /></span>
  </button>
</template>

<style scoped>
.btn2 {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: var(--r-md);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: 400;
  cursor: pointer;
  /* Background only. Never border-color, never `all`. */
  transition: background var(--motion-hover);
}

/* Heights and inline padding travel together: the padding is tuned per height,
   so a size is one decision rather than two that can disagree. */
.btn2[data-size='xs'] {
  height: var(--s2a-h-xs);
  padding: 0 10px;
  font-size: var(--fs-sm);
}
.btn2[data-size='sm'] {
  height: var(--s2a-h-sm);
  padding: 0 12px;
}
.btn2[data-size='md'] {
  height: var(--s2a-h-md);
  padding: 0 14px;
}
.btn2[data-size='lg'] {
  height: var(--s2a-h-lg);
  padding: 0 18px;
  font-size: var(--fs-lg);
}

/* --- variants ------------------------------------------------------------ */

/* Solid is the only variant that takes the medium weight. Everything else at
   600 would make a toolbar read as six primary actions. */
.btn2[data-variant='solid'] {
  background: var(--primary);
  color: var(--primary-foreground);
  font-weight: var(--fw-medium);
}
.btn2[data-variant='solid']:hover:not(:disabled) {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.btn2[data-variant='solid']:active:not(:disabled) {
  background: color-mix(in oklch, var(--primary) 84%, var(--foreground));
}

.btn2[data-variant='secondary'] {
  border: 1px solid var(--border);
  background: var(--card);
  color: var(--foreground);
}
.btn2[data-variant='secondary']:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.btn2[data-variant='ghost'] {
  background: transparent;
  color: var(--body-copy);
}
.btn2[data-variant='ghost']:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

/* Destructive, success and warning are outlined rather than filled. A filled
   red in a toolbar reads as the primary action, which it never is. The
   hairlines are opaque mixes rather than alpha fades of the accent, which
   composite into a muddy grey veil on light surfaces. */
.btn2[data-variant='danger'] {
  border: 1px solid color-mix(in oklch, var(--destructive) 30%, transparent);
  background: transparent;
  color: var(--destructive);
}
.btn2[data-variant='danger']:hover:not(:disabled) {
  background: var(--destructive-soft);
}

.btn2[data-variant='success'] {
  border: 1px solid color-mix(in oklch, var(--success) 26%, transparent);
  background: color-mix(in oklch, var(--success) 12%, var(--card));
  color: var(--success);
}
.btn2[data-variant='success']:hover:not(:disabled) {
  background: color-mix(in oklch, var(--success) 18%, var(--card));
}

.btn2[data-variant='warning'] {
  border: 1px solid color-mix(in oklch, var(--s2a-attn-bg) 26%, transparent);
  background: color-mix(in oklch, var(--s2a-attn-bg) 12%, var(--card));
  color: var(--s2a-attn);
}
.btn2[data-variant='warning']:hover:not(:disabled) {
  background: color-mix(in oklch, var(--s2a-attn-bg) 18%, var(--card));
}

/* --- states -------------------------------------------------------------- */

/* 1px ring plus a 3px warm halo. Never a cool macOS blue. */
.btn2:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* Still focusable: `disabled` on the element would drop it out of tab order and
   take its tooltip with it, and part 02's rule is that a gated control has to
   be able to explain its gate. */
.btn2:disabled,
.btn2[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}

.btn2[aria-busy='true'] {
  cursor: progress;
}

.btn2__icon {
  font-size: 14px;
}

/* Slotted icons are SVG elements from Icon.vue. The global SVG/span baseline
   rules intentionally make them block-level elsewhere; buttons need their
   label contents to be an explicit inline flex row so the glyph cannot stack
   above the text. */
.btn2__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  line-height: 1;
}

.btn2__spinner {
  display: inline-block;
  width: 13px;
  height: 13px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  opacity: 0.7;
  animation: s2a-spin 0.7s linear infinite;
}
</style>
