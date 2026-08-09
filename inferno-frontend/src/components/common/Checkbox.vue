<script setup lang="ts">
/**
 * Checkbox — part 02, section 06 ("binary": toggle, checkbox, radio).
 *
 * Migration note from the prototype:
 *   "Checkbox and radio need extracting from about forty call sites; the
 *    mixed state, which bulk select needs, does not exist today and is
 *    drawn here."
 *
 * This resolves the prototype's open call ("Checkbox and radio have no
 * component today. Worth creating two, or keep them as documented raw
 * inputs with a shared class?") on the side of creating the two components.
 * The ~40 existing raw <input type="checkbox"> call sites are not touched
 * here — out of scope for this file set — but new and rewritten call sites
 * can now use a real component instead of ad hoc Tailwind.
 *
 * A real, visually hidden <input type="checkbox"> underneath: `indeterminate`
 * has no HTML attribute, only a DOM property, so it has to be set
 * imperatively on the element via a ref.
 */
import { onMounted, ref, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    /** The mixed/"some but not all" state bulk select needs. Visually takes
     *  the same filled treatment as checked; drawn as a dash instead of a
     *  tick. */
    indeterminate?: boolean
    disabled?: boolean
    id?: string
  }>(),
  { indeterminate: false, disabled: false }
)

const emit = defineEmits<{ 'update:modelValue': [boolean] }>()

const inputRef = ref<HTMLInputElement | null>(null)

const syncIndeterminate = () => {
  if (inputRef.value) inputRef.value.indeterminate = props.indeterminate
}
onMounted(syncIndeterminate)
watch(() => props.indeterminate, syncIndeterminate)

/* Listens on `click`, not `change`: `.checked` is already settled by the
 * time the click event fires (see Toggle.vue's comment for why), and this
 * keeps `wrapper.find(...).trigger('click')` working in jsdom, which does
 * not reliably synthesize `change` for a programmatically dispatched click. */
const onClick = (event: Event) => {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}
</script>

<template>
  <label class="chk" :data-disabled="disabled || undefined">
    <input
      ref="inputRef"
      :id="id"
      type="checkbox"
      class="chk__input"
      :checked="modelValue"
      :disabled="disabled"
      v-bind="$attrs"
      @click="onClick"
    />
    <span class="chk__box" :data-checked="modelValue || indeterminate || undefined" aria-hidden="true">
      <span v-if="indeterminate" class="chk__dash" />
      <i v-else-if="modelValue" class="hgi-stroke hgi-tick-01 chk__tick" aria-hidden="true" />
    </span>
    <span v-if="$slots.default" class="chk__label"><slot /></span>
  </label>
</template>

<style scoped>
.chk {
  position: relative;
  display: inline-grid;
  grid-template-columns: 15px minmax(0, 1fr);
  align-items: center;
  gap: 9px;
  cursor: pointer;
}

.chk[data-disabled] {
  opacity: 0.55;
  cursor: not-allowed;
}

/* Real, focusable, visually hidden. */
.chk__input {
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

/* 15 square, --r-xs: the badge/keycap radius tier, not --r-md. */
.chk__box {
  display: grid;
  place-items: center;
  width: 15px;
  height: 15px;
  border: 1px solid var(--border);
  border-radius: var(--r-xs);
  background: var(--card);
  color: var(--on-solid);
  /* Colour encodes state; never transition border-color. */
  transition: background var(--t-fast) var(--ease-out);
}

.chk__box[data-checked] {
  border-color: var(--brand);
  background: var(--brand);
}

.chk__tick {
  font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */
}

.chk__dash {
  width: 7px;
  height: 1.5px;
  border-radius: 1px;
  background: var(--on-solid);
}

.chk__input:focus-visible + .chk__box {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.chk__label {
  min-width: 0;
  font-size: var(--fs-md);
  color: var(--foreground);
}
</style>
