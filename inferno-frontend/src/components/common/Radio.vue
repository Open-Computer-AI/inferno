<script setup lang="ts">
/**
 * Radio — part 02, section 06 ("binary": toggle, checkbox, radio).
 *
 * Migration note from the prototype:
 *   "Checkbox and radio need extracting from about forty call sites."
 *
 * Resolves the prototype's open call the same way Checkbox.vue does: a real
 * component rather than documented raw inputs. One <Radio> represents one
 * option in a group; give every instance in the group the same `name` so
 * the native <input type="radio"> grouping does the keyboard (arrow key)
 * behaviour for free, and bind the same v-model to each so `modelValue`
 * is the group's current value and `value` is this option's own value.
 *
 * Radio rows may carry a one-line description; checkboxes do not, so this
 * component (unlike Checkbox) has a dedicated `note` slot of its own rather
 * than a single free-form default slot.
 */
defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue: string | number | boolean | null
    value: string | number | boolean
    /** Required: native grouping (and its keyboard behaviour) only works
     *  when every option in the group shares one name. */
    name: string
    label?: string
    note?: string
    variant?: 'default' | 'pill'
    disabled?: boolean
  }>(),
  { variant: 'default', disabled: false }
)

const emit = defineEmits<{ 'update:modelValue': [string | number | boolean] }>()

let skipNextChange = false

/* Listens on `click`, not `change`: see Toggle.vue's comment. `.checked` is
 * already settled by the time click fires, for both mouse and keyboard
 * activation, and it is what `wrapper.find(...).trigger('click')` needs. */
const onClick = (event: Event) => {
  const input = event.target as HTMLInputElement
  if (input.checked) {
    skipNextChange = true
    emit('update:modelValue', props.value)
  }
}

const onChange = (event: Event) => {
  const input = event.target as HTMLInputElement
  if (skipNextChange) {
    skipNextChange = false
    return
  }
  if (input.checked) emit('update:modelValue', props.value)
}
</script>

<template>
  <label class="rdo" :class="`rdo--${variant}`" :data-disabled="disabled || undefined" :data-variant="variant">
    <input
      type="radio"
      class="rdo__input"
      :name="name"
      :checked="modelValue === value"
      :disabled="disabled"
      v-bind="$attrs"
      @click="onClick"
      @change="onChange"
    />
    <span class="rdo__dot-box" :data-checked="modelValue === value || undefined" aria-hidden="true">
      <span class="rdo__dot" />
    </span>
    <span v-if="label || note || $slots.default" class="rdo__text">
      <span v-if="label" class="rdo__label">{{ label }}</span>
      <slot />
      <span v-if="note" class="rdo__note">{{ note }}</span>
    </span>
  </label>
</template>

<style scoped>
.rdo {
  position: relative;
  display: grid;
  grid-template-columns: 15px minmax(0, 1fr);
  align-items: start;
  gap: 9px;
  cursor: pointer;
}

.rdo[data-disabled] {
  opacity: 0.55;
  cursor: not-allowed;
}

.rdo--pill {
  display: inline-flex;
  align-items: center;
  gap: 0;
  border: 1px solid var(--brand-line);
  border-radius: 999px;
  padding: 6px 12px;
  color: var(--muted-foreground);
  transition: background-color 140ms ease, color 140ms ease;
}

.rdo--pill:has(.rdo__input:checked) {
  border-color: var(--destructive);
  background: var(--destructive-soft);
  color: var(--destructive);
}

.rdo--pill .rdo__dot-box {
  display: none;
}

.rdo--pill .rdo__text {
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
}

/* Real, focusable, visually hidden. */
.rdo__input {
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

/* 15 round, aligned to the first line of the label: 2px top margin puts its
   centre on the label's cap height rather than the row's centre. */
.rdo__dot-box {
  display: grid;
  place-items: center;
  width: 15px;
  height: 15px;
  margin-top: 2px;
  border: 1px solid var(--border);
  border-radius: 50%;
  background: var(--card);
}

/* Only the border and the inner dot encode state; the box fill is always
   --card, unlike checkbox's solid fill. Never transition border-color. */
.rdo__dot-box[data-checked] {
  border-color: var(--brand);
}

.rdo__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: transparent;
}

.rdo__dot-box[data-checked] .rdo__dot {
  background: var(--brand);
}

.rdo__input:focus-visible + .rdo__dot-box {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.rdo__text {
  min-width: 0;
}

.rdo__label {
  display: block;
  font-size: var(--fs-md);
  color: var(--foreground);
}

.rdo__note {
  display: block;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  line-height: 1.45;
}
</style>
