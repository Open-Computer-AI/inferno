<script setup lang="ts">
/**
 * Icon button — part 02, section 02.
 *
 * Migration note from the prototype:
 *   "The source uses .btn-icon with p-2.5 and rounded-xl, which yields a 42px
 *    control that does not line up with a 32px toolbar. Fix the size and the
 *    radius together."
 *
 * Radius is --r-sm at every size, including 28px. June picks radius by what an
 * element IS, not by how big it is: dropping a small square button to --r-xs
 * makes it read as a badge, which is a different kind of thing.
 *
 * `label` is required, not optional. The prototype: "Always carries a tooltip,
 * because an icon alone is never self evident." Making it required means an
 * unlabelled icon button is a type error rather than an accessibility bug
 * found later.
 */
withDefaults(
  defineProps<{
    /** Hugeicons glyph, e.g. "hgi-arrow-reload-horizontal". */
    icon: string
    /** Tooltip and accessible name. Required by design, not by convention. */
    label: string
    /** 28 / 32 / 36, glyph 14 / 15 / 16. */
    size?: 'xs' | 'sm' | 'md'
    /** Destructive hover tint, for row actions like revoke. */
    tone?: 'neutral' | 'danger'
    disabled?: boolean
  }>(),
  { size: 'xs', tone: 'neutral', disabled: false }
)

defineEmits<{ click: [MouseEvent] }>()
</script>

<template>
  <button
    type="button"
    class="iconbtn"
    :data-size="size"
    :data-tone="tone"
    :title="label"
    :aria-label="label"
    :disabled="disabled"
    :aria-disabled="disabled || undefined"
    @click="$emit('click', $event)"
  >
    <i :class="['hgi-stroke', icon]" aria-hidden="true" />
  </button>
</template>

<style scoped>
.iconbtn {
  display: grid;
  place-items: center;
  border: 0;
  /* --r-sm at every size. Not --r-xs, which is the badge tier. */
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

/* Square: width and height come from one token so it cannot drift into a
   rectangle when padding changes. */
.iconbtn[data-size='xs'] {
  width: var(--s2a-h-xs);
  height: var(--s2a-h-xs);
  font-size: 14px;
}
.iconbtn[data-size='sm'] {
  width: var(--s2a-h-sm);
  height: var(--s2a-h-sm);
  font-size: 15px;
}
.iconbtn[data-size='md'] {
  width: var(--s2a-h-md);
  height: var(--s2a-h-md);
  font-size: 16px;
}

/* Hover lifts the glyph from --muted-foreground to --foreground and tints the
   background neutral grey. Colour is not spent here: an icon button is chrome,
   so brand tint would encode importance it does not have. */
.iconbtn:hover:not(:disabled) {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.iconbtn[data-tone='danger']:hover:not(:disabled) {
  background: var(--destructive-soft);
  color: var(--destructive);
}

.iconbtn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.iconbtn:disabled,
.iconbtn[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}
</style>
