<script setup lang="ts">
/**
 * LoadingSpinner — part 03, section 14/15.
 *
 * Migration note from the prototype:
 *   "The source has four colours: primary, secondary, white and gray. Two of
 *    them are the same grey and the teal one is the loudest thing on a
 *    loading screen. Neutral, or on-solid inside a filled control. Nothing
 *    else."
 *
 * `size` keeps its four values, now at 16 / 24 / 32 / 44 (was 16 / 32 / 48 /
 * 64). `color` changes shape from the four-value enum to 'neutral' | 'onSolid'
 * — an explicit prop type change the spec calls for, safe here because none of
 * the 16 call sites pass `color` at all; every one either takes the default or
 * sets only `size`.
 *
 * The ring reuses inferno.css's shared `s2a-spin` keyframe rather than
 * declaring its own: that file's prefers-reduced-motion guard matches on any
 * class containing "s2a-spinner", so this component carries that class
 * instead of a local @keyframes + media query.
 */
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'
type SpinnerColor = 'neutral' | 'onSolid'

withDefaults(
  defineProps<{
    size?: SpinnerSize
    color?: SpinnerColor
  }>(),
  { size: 'md', color: 'neutral' }
)
</script>

<template>
  <span class="spin2 s2a-spinner" :data-size="size" :data-color="color" role="status" :aria-label="t('common.loading')">
    <span class="spin2__sr">{{ t('common.loading') }}</span>
  </span>
</template>

<style scoped>
.spin2 {
  position: relative;
  display: inline-block;
  border-style: solid;
  border-radius: 50%;
  border-color: var(--spinner-neutral);
  border-top-color: transparent;
  animation: s2a-spin 0.7s linear infinite;
}

/* Diameter and ring width travel together, same as every other size scale in
 * this system. */
.spin2[data-size='sm'] {
  width: 16px;
  height: 16px;
  border-width: 2px;
}
.spin2[data-size='md'] {
  width: 24px;
  height: 24px;
  border-width: 2px;
}
.spin2[data-size='lg'] {
  width: 32px;
  height: 32px;
  border-width: 3px;
}
.spin2[data-size='xl'] {
  width: 44px;
  height: 44px;
  border-width: 4px;
}

/* The only other colour that survives: inside a filled control the ring has
 * to read against --primary (or another solid fill), not the page. */
.spin2[data-color='onSolid'] {
  border-color: var(--primary-foreground);
  border-top-color: transparent;
}

/* Visually hidden, not `sr-only` from Tailwind: this file has no Tailwind
 * utilities, only tokens. */
.spin2__sr {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
