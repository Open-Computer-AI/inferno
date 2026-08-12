<script setup lang="ts">
import { computed, ref } from 'vue'

/**
 * Input — part 02, section 03.
 *
 * Migration note from the prototype:
 *   "Height moves from py-2.5 to an explicit 36px. Drop the
 *    ring-2 ring-primary-500/30 focus and ring-red-500/20 error ring for the
 *    token composite. The disabled state stops repainting the background grey."
 *
 * The prop, emit, slot and expose surface is byte-for-byte what it was — the
 * spec marks every prop "unchanged" except `readonly`, which only changes how
 * it looks. This is a presentation rewrite, so every existing call site keeps
 * working untouched.
 *
 * Two states that used to look identical and no longer do:
 *
 *   disabled  the control is off. Dimmed, not repainted. Repainting the fill
 *             grey said "this is a different kind of field" when it is the same
 *             field, switched off.
 *   readonly  the value is real and worth reading, you just cannot edit it.
 *             --surface-subtle fill with --border-subtle, so it reads as
 *             presented data rather than a broken input.
 */
interface Props {
  modelValue: string | number | null | undefined
  type?: string
  label?: string
  placeholder?: string
  disabled?: boolean
  required?: boolean
  readonly?: boolean
  error?: string
  hint?: string
  id?: string
  autocomplete?: string
  /* Forwarded to the inner control. Purely additive -- the root element is a
     div, so a fallthrough `min` would land on the wrapper and never reach the
     input that has to enforce it. No existing call site passes these. */
  min?: number | string
  max?: number | string
  step?: number | string
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  disabled: false,
  required: false,
  readonly: false
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
  (e: 'blur', event: FocusEvent): void
  (e: 'focus', event: FocusEvent): void
  (e: 'enter', event: KeyboardEvent): void
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const placeholderText = computed(() => props.placeholder || '')

const onInput = (event: Event) => {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}

defineExpose({
  focus: () => inputRef.value?.focus(),
  select: () => inputRef.value?.select()
})
</script>

<template>
  <div class="fld">
    <label v-if="label" :for="id" class="fld__label">
      {{ label }}
      <!-- Asterisk after the label, in --destructive. It marks a requirement,
           it is not an error, so it never carries the error styling. -->
      <span v-if="required" class="fld__req" aria-hidden="true">*</span>
    </label>

    <div class="fld__wrap">
      <span v-if="$slots.prefix" class="fld__prefix"><slot name="prefix" /></span>

      <input
        :id="id"
        ref="inputRef"
        class="fld__input"
        :type="type"
        :value="modelValue"
        :disabled="disabled"
        :required="required"
        :readonly="readonly"
        :placeholder="placeholderText"
        :autocomplete="autocomplete"
        :min="min"
        :max="max"
        :step="step"
        :data-has-prefix="$slots.prefix ? '' : undefined"
        :data-has-suffix="$slots.suffix ? '' : undefined"
        :data-invalid="error ? '' : undefined"
        :aria-invalid="error ? true : undefined"
        @input="onInput"
        @change="$emit('change', ($event.target as HTMLInputElement).value)"
        @blur="$emit('blur', $event)"
        @focus="$emit('focus', $event)"
        @keyup.enter="$emit('enter', $event)"
      />

      <span v-if="$slots.suffix" class="fld__suffix"><slot name="suffix" /></span>
    </div>

    <p v-if="error" class="fld__msg fld__msg--error">{{ error }}</p>
    <p v-else-if="hint" class="fld__msg">{{ hint }}</p>
  </div>
</template>

<style scoped>
.fld {
  width: 100%;
}

/* Label, 6px, field, 6px, hint. The gaps are equal so the group reads as one
   object rather than a label sitting closer to the field above it. */
.fld__label {
  display: block;
  margin-bottom: 6px;
  font-size: var(--fs-md);
  color: var(--foreground);
}

.fld__req {
  color: var(--destructive);
}

.fld__wrap {
  position: relative;
  display: flex;
  align-items: center;
}

.fld__input {
  width: 100%;
  height: var(--s2a-h-md); /* 36px, explicit. Was py-2.5, which drifted. */
  padding: 0 10px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  /* Background only, never border-color. */
  transition: background var(--motion-hover);
}

.fld__input::placeholder {
  color: var(--muted-foreground);
}

/* Prefix pads to 32, suffix to 34. Not symmetric: a suffix is usually an
   interactive affordance (clear, reveal) and needs the extra breathing room. */
.fld__input[data-has-prefix] {
  padding-left: 32px;
}
.fld__input[data-has-suffix] {
  padding-right: 34px;
}

.fld__prefix,
.fld__suffix {
  position: absolute;
  display: flex;
  align-items: center;
  color: var(--muted-foreground);
  /* These slots carry an icon. 01-TOKENS: glyphs are fixed chrome and must NOT
     scale with --font-scale, so a --fs-* token would be the wrong tool here. */
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
}
.fld__prefix {
  left: 10px;
  pointer-events: none;
}
.fld__suffix {
  right: 8px;
}

/* The token composite: a 1px ring plus a 3px warm halo. Replaces
   ring-2 ring-primary-500/30, which was a cool blue at two different alphas. */
.fld__input:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.fld__input[data-invalid] {
  border-color: var(--destructive);
}
.fld__input[data-invalid]:focus {
  box-shadow: 0 0 0 3px color-mix(in oklch, var(--destructive) 18%, transparent);
}

/* Off, not different. The fill stays --card. */
.fld__input:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* Real data you cannot edit. Distinct from disabled on purpose. */
.fld__input:read-only:not(:disabled) {
  background: var(--surface-subtle);
  border-color: var(--border-subtle);
}

.fld__msg {
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  line-height: 1.45;
}

.fld__msg--error {
  color: var(--destructive);
}
</style>
