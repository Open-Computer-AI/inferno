<script setup lang="ts">
/**
 * Toggle — part 02, section 06 ("binary": toggle, checkbox, radio).
 *
 * Migration note from the prototype:
 *   "Toggle goes from 44 by 24 to 34 by 20 and drops the focus ring offset."
 *
 * The old version painted a <button role="switch"> with no underlying form
 * control. Per the design system, toggle/checkbox/radio are form controls:
 * keyboard, screen readers and native label click-forwarding all expect a
 * real <input>. So the pill is decoration for a real, visually hidden
 * <input type="checkbox" role="switch">, driven off :checked.
 *
 * The prop and emit surface is unchanged: one prop, modelValue, plus
 * update:modelValue. `disabled` is declared explicitly now only because the
 * interactive element moved one level down inside a <label> — it worked
 * before purely via attribute fallthrough onto the old <button>. Every other
 * extra attribute (aria-label, data-test, ...) still lands on the control
 * itself: inheritAttrs is off and $attrs is bound to the <input>, not the
 * <label> wrapper, which is where assistive tech and tests look for it.
 *
 * Listens on `click`, not `change`: for a checkbox/switch input the DOM
 * flips `.checked` as part of the click's own pre-activation step, before
 * the click event is dispatched, for both mouse and keyboard (Space)
 * activation, and label-click forwarding re-dispatches a real click on this
 * input too. So `click` already carries the settled value. It also keeps
 * `wrapper.find(...).trigger('click')`, used throughout this codebase's
 * tests, working the same way it did against the old <button>.
 */
defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    modelValue: boolean
    disabled?: boolean
  }>(),
  { disabled: false }
)

const emit = defineEmits<{ 'update:modelValue': [boolean] }>()

const onClick = (event: Event) => {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}
</script>

<template>
  <label class="tgl" :data-disabled="disabled || undefined">
    <input
      type="checkbox"
      role="switch"
      class="tgl__input"
      :checked="modelValue"
      :aria-checked="modelValue ? 'true' : 'false'"
      :disabled="disabled"
      v-bind="$attrs"
      @click="onClick"
    />
    <span class="tgl__track" :data-checked="modelValue || undefined" aria-hidden="true">
      <span class="tgl__thumb" />
    </span>
  </label>
</template>

<style scoped>
.tgl {
  position: relative;
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  cursor: pointer;
}

.tgl[data-disabled] {
  opacity: 0.55;
  cursor: not-allowed;
}

/* Real, focusable, visually hidden. Not display:none: that would drop it
   from the accessibility tree and out of tab order. */
.tgl__input {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* 34 by 20, per the prototype's "Toggle, 34 by 20" caption. */
.tgl__track {
  position: relative;
  flex-shrink: 0;
  width: 34px;
  height: 20px;
  border-radius: var(--r-pill);
  background: var(--muted);
  /* Colour encodes state; background is the only thing that ever moves. */
  transition: background var(--t-fast) var(--ease-out);
}

.tgl__track[data-checked] {
  background: var(--brand);
}

/* 16px knob, 2px inset. */
.tgl__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--on-solid);
  box-shadow: 0 1px 2px oklch(0% 0 none / 18%);
  transition: left var(--t-fast) var(--ease-out);
}

.tgl__track[data-checked] .tgl__thumb {
  left: 16px;
}

/* 1px ring plus a 3px warm halo, on the real control. --t-fast collapses to
   0ms under prefers-reduced-motion (tokens/motion.css), so no separate media
   query is needed here. */
.tgl__input:focus-visible + .tgl__track {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
</style>
